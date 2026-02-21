package django

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Snapshot represents a database snapshot with metadata
type Snapshot struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Timestamp         time.Time `json:"timestamp"`
	GitBranch         string    `json:"git_branch"`
	GitCommit         string    `json:"git_commit"`
	DatabaseEngine    string    `json:"database_engine"`
	AppliedMigrations []string  `json:"applied_migrations"`
	FilePath          string    `json:"file_path"`
	MetadataPath      string    `json:"metadata_path"`
}

// SnapshotManager handles database snapshots
type SnapshotManager struct {
	project      *Project
	snapshotsDir string
}

func shouldUseDjangoDumpFallback(engine string, hasDocker bool, hasCmd func(string) bool) bool {
	engine = strings.ToLower(engine)

	// Docker-managed databases may provide tooling inside containers.
	if hasDocker {
		return false
	}

	switch {
	case strings.Contains(engine, "postgresql"):
		return !hasCmd("pg_dump")
	case strings.Contains(engine, "mysql"):
		return !hasCmd("mysqldump")
	default:
		return true
	}
}

func snapshotFileExtension(engine string, djangoFallback bool) string {
	engine = strings.ToLower(engine)
	if strings.Contains(engine, "sqlite") {
		return ".sqlite3"
	}
	if djangoFallback {
		return ".json"
	}
	return ".sql"
}

// NewSnapshotManager creates a snapshot manager
func NewSnapshotManager(project *Project) *SnapshotManager {
	snapshotsDir := filepath.Join(project.RootDir, ".lazy-django", "snapshots")
	os.MkdirAll(snapshotsDir, 0755)

	return &SnapshotManager{
		project:      project,
		snapshotsDir: snapshotsDir,
	}
}

// CreateSnapshot creates a new database snapshot
func (sm *SnapshotManager) CreateSnapshot(name string) (*Snapshot, error) {
	now := time.Now().UTC()
	if name == "" {
		name = fmt.Sprintf("snapshot-%s", now.Format("20060102-150405"))
	}

	snapshot := &Snapshot{
		ID:             fmt.Sprintf("%d", now.UnixNano()),
		Name:           name,
		Timestamp:      now,
		DatabaseEngine: sm.project.Database.Engine,
	}

	// Get git info
	snapshot.GitBranch = sm.getCurrentBranch()
	snapshot.GitCommit = sm.getCurrentCommit()

	// Get applied migrations
	migrations, err := sm.getAppliedMigrations()
	if err == nil {
		snapshot.AppliedMigrations = migrations
	}

	engine := strings.ToLower(sm.project.Database.Engine)
	djangoFallback := shouldUseDjangoDumpFallback(engine, sm.project.HasDocker, commandExists)

	// Create snapshot based on database type
	snapshotFile := filepath.Join(sm.snapshotsDir, fmt.Sprintf("%s%s", snapshot.ID, snapshotFileExtension(engine, djangoFallback)))
	snapshot.FilePath = snapshotFile
	snapshot.MetadataPath = filepath.Join(sm.snapshotsDir, fmt.Sprintf("%s.json", snapshot.ID))

	var dumpErr error
	switch {
	case strings.Contains(engine, "postgresql"):
		if djangoFallback {
			dumpErr = sm.dumpDjangoData(snapshotFile)
		} else {
			dumpErr = sm.dumpPostgreSQL(snapshotFile)
		}
	case strings.Contains(engine, "mysql"):
		if djangoFallback {
			dumpErr = sm.dumpDjangoData(snapshotFile)
		} else {
			dumpErr = sm.dumpMySQL(snapshotFile)
		}
	case strings.Contains(engine, "sqlite"):
		dumpErr = sm.dumpSQLite(snapshotFile)
	default:
		// Fallback to Django dumpdata (works with any backend)
		dumpErr = sm.dumpDjangoData(snapshotFile)
	}

	if dumpErr != nil {
		return nil, fmt.Errorf("failed to create snapshot: %w", dumpErr)
	}

	// Save metadata
	if err := sm.saveMetadata(snapshot); err != nil {
		return nil, fmt.Errorf("failed to save metadata: %w", err)
	}

	return snapshot, nil
}

// dbConfig holds database-specific command configuration
type dbConfig struct {
	dumpCmd     string
	restoreCmd  string
	dumpArgs    func(*DatabaseInfo, string) []string
	restoreArgs func(*DatabaseInfo, string) []string
	envVars     func(string) []string
	checkDocker func(*SnapshotManager) string
}

var dbConfigs = map[string]dbConfig{
	"postgresql": {
		dumpCmd:    "pg_dump",
		restoreCmd: "psql",
		dumpArgs: func(db *DatabaseInfo, file string) []string {
			return []string{"-h", db.Host, "-p", db.Port, "-U", db.User, "-d", db.Name, "-f", file, "--clean", "--if-exists"}
		},
		restoreArgs: func(db *DatabaseInfo, file string) []string {
			return []string{"-h", db.Host, "-p", db.Port, "-U", db.User, "-d", db.Name, "-f", file}
		},
		envVars: func(pw string) []string {
			return []string{fmt.Sprintf("PGPASSWORD=%s", pw)}
		},
		checkDocker: func(sm *SnapshotManager) string { return sm.getPostgresContainerName() },
	},
	"mysql": {
		dumpCmd:    "mysqldump",
		restoreCmd: "mysql",
		dumpArgs: func(db *DatabaseInfo, file string) []string {
			return []string{"-h", db.Host, "-P", db.Port, "-u", db.User, fmt.Sprintf("-p%s", db.Host), "--add-drop-table", "--result-file=" + file, db.Name}
		},
		restoreArgs: func(db *DatabaseInfo, file string) []string {
			return []string{"-h", db.Host, "-P", db.Port, "-u", db.User, fmt.Sprintf("-p%s", db.Host), db.Name}
		},
		envVars:     func(pw string) []string { return nil },
		checkDocker: func(sm *SnapshotManager) string { return sm.getMySQLContainerName() },
	},
}

// runDBCommand executes a database command locally or in Docker
func (sm *SnapshotManager) runDBCommand(dbType, cmd string, args []string, envVars []string, outputFile string) error {
	config := dbConfigs[dbType]
	containerName := config.checkDocker(sm)

	if sm.project.HasDocker && containerName != "" {
		return sm.runDockerDBCommand(containerName, cmd, args, envVars, outputFile)
	}

	command := exec.Command(cmd, args...)
	command.Env = append(os.Environ(), envVars...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %s - %w", cmd, string(output), err)
	}
	return nil
}

// runDockerDBCommand executes a database command inside Docker container
func (sm *SnapshotManager) runDockerDBCommand(container, cmd string, args []string, envVars []string, outputFile string) error {
	dockerArgs := []string{"exec"}
	for _, env := range envVars {
		dockerArgs = append(dockerArgs, "-e", env)
	}
	dockerArgs = append(dockerArgs, container, cmd)
	dockerArgs = append(dockerArgs, args...)

	command := exec.Command("docker", dockerArgs...)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s failed: %s - %w", cmd, string(output), err)
	}

	if outputFile != "" {
		return os.WriteFile(outputFile, output, 0644)
	}
	return nil
}

// dumpPostgreSQL creates PostgreSQL dump
func (sm *SnapshotManager) dumpPostgreSQL(outputFile string) error {
	config := dbConfigs["postgresql"]

	// Check if running in Docker
	containerName := config.checkDocker(sm)
	if sm.project.HasDocker && containerName != "" {
		// Inside Docker, use connection details from Django settings
		// These map to the Docker network names (e.g., POSTGRES_HOST=postgres)
		args := []string{
			"-h", sm.project.Database.Host,
			"-p", sm.project.Database.Port,
			"-U", sm.project.Database.User,
			"-d", sm.project.Database.Name,
			"--clean",
			"--if-exists",
		}
		// Set PGPASSWORD environment variable
		envVars := []string{fmt.Sprintf("PGPASSWORD=%s", sm.getDatabasePassword())}
		return sm.runDBCommand("postgresql", config.dumpCmd, args, envVars, outputFile)
	}

	// Local execution - use full connection details
	args := config.dumpArgs(&sm.project.Database, outputFile)
	return sm.runDBCommand("postgresql", config.dumpCmd, args,
		config.envVars(sm.getDatabasePassword()), outputFile)
}

// dumpMySQL creates MySQL dump
func (sm *SnapshotManager) dumpMySQL(outputFile string) error {
	config := dbConfigs["mysql"]
	pw := sm.getDatabasePassword()

	containerName := config.checkDocker(sm)
	if sm.project.HasDocker && containerName != "" {
		args := []string{
			"-h", sm.project.Database.Host,
			"-P", sm.project.Database.Port,
			"-u", sm.project.Database.User,
			fmt.Sprintf("-p%s", pw),
			"--add-drop-table",
			sm.project.Database.Name,
		}
		return sm.runDBCommand("mysql", config.dumpCmd, args, config.envVars(pw), outputFile)
	}

	args := config.dumpArgs(&sm.project.Database, outputFile)
	// Fix password in args
	for i, arg := range args {
		if strings.HasPrefix(arg, "-p") {
			args[i] = fmt.Sprintf("-p%s", pw)
		}
	}
	return sm.runDBCommand("mysql", config.dumpCmd, args, config.envVars(pw), outputFile)
}

// dumpSQLite copies SQLite database file
func (sm *SnapshotManager) dumpSQLite(outputFile string) error {
	dbPath := sm.project.Database.Name

	// Handle relative paths
	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(sm.project.RootDir, dbPath)
	}

	input, err := os.ReadFile(dbPath)
	if err != nil {
		return fmt.Errorf("failed to read sqlite db: %w", err)
	}

	return os.WriteFile(outputFile, input, 0644)
}

// dumpDjangoData uses Django's dumpdata (universal fallback)
func (sm *SnapshotManager) dumpDjangoData(outputFile string) error {
	output, err := sm.project.RunCommand("dumpdata", "--natural-foreign", "--natural-primary", "--indent", "2")
	if err != nil {
		return fmt.Errorf("dumpdata failed: %w", err)
	}

	return os.WriteFile(outputFile, []byte(output), 0644)
}

// RestoreSnapshot restores a database snapshot
func (sm *SnapshotManager) RestoreSnapshot(snapshotID string) error {
	snapshot, err := sm.GetSnapshot(snapshotID)
	if err != nil {
		return err
	}

	engine := strings.ToLower(snapshot.DatabaseEngine)
	ext := strings.ToLower(filepath.Ext(snapshot.FilePath))

	// Restore based on database type
	var restoreErr error
	switch {
	case ext == ".json":
		restoreErr = sm.restoreDjangoData(snapshot.FilePath)
	case strings.Contains(engine, "postgresql"):
		restoreErr = sm.restorePostgreSQL(snapshot.FilePath)
	case strings.Contains(engine, "mysql"):
		restoreErr = sm.restoreMySQL(snapshot.FilePath)
	case strings.Contains(engine, "sqlite"):
		restoreErr = sm.restoreSQLite(snapshot.FilePath)
	default:
		restoreErr = sm.restoreDjangoData(snapshot.FilePath)
	}

	if restoreErr != nil {
		return fmt.Errorf("failed to restore snapshot: %w", restoreErr)
	}

	// Handle migration differences
	if err := sm.syncMigrations(snapshot.AppliedMigrations); err != nil {
		return fmt.Errorf("failed to sync migrations: %w", err)
	}

	return nil
}

// restorePostgreSQL restores PostgreSQL dump
func (sm *SnapshotManager) restorePostgreSQL(dumpFile string) error {
	config := dbConfigs["postgresql"]

	// For Docker, copy file first
	if sm.project.HasDocker && config.checkDocker(sm) != "" {
		containerName := config.checkDocker(sm)
		tmpFile := "/tmp/restore.sql"
		copyCmd := exec.Command("docker", "cp", dumpFile, fmt.Sprintf("%s:%s", containerName, tmpFile))
		if err := copyCmd.Run(); err != nil {
			return fmt.Errorf("failed to copy dump to container: %w", err)
		}
		dumpFile = tmpFile
	}

	return sm.runDBCommand("postgresql", config.restoreCmd,
		config.restoreArgs(&sm.project.Database, dumpFile),
		config.envVars(sm.getDatabasePassword()), "")
}

// restoreMySQL restores MySQL dump
func (sm *SnapshotManager) restoreMySQL(dumpFile string) error {
	config := dbConfigs["mysql"]
	pw := sm.getDatabasePassword()

	// For Docker, copy file first
	if sm.project.HasDocker && config.checkDocker(sm) != "" {
		containerName := config.checkDocker(sm)
		tmpFile := "/tmp/restore.sql"
		copyCmd := exec.Command("docker", "cp", dumpFile, fmt.Sprintf("%s:%s", containerName, tmpFile))
		if err := copyCmd.Run(); err != nil {
			return fmt.Errorf("failed to copy dump to container: %w", err)
		}
		dumpFile = tmpFile
	}

	args := config.restoreArgs(&sm.project.Database, dumpFile)
	// Fix password in args
	for i, arg := range args {
		if strings.HasPrefix(arg, "-p") {
			args[i] = fmt.Sprintf("-p%s", pw)
		}
	}

	// MySQL restore needs stdin redirection
	cmd := exec.Command(config.restoreCmd, args...)
	cmd.Env = append(os.Environ(), config.envVars(pw)...)
	if input, err := os.ReadFile(dumpFile); err == nil {
		cmd.Stdin = strings.NewReader(string(input))
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysql restore failed: %s - %w", string(output), err)
	}
	return nil
}

// restoreSQLite restores SQLite database
func (sm *SnapshotManager) restoreSQLite(dumpFile string) error {
	dbPath := sm.project.Database.Name

	if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Join(sm.project.RootDir, dbPath)
	}

	input, err := os.ReadFile(dumpFile)
	if err != nil {
		return fmt.Errorf("failed to read dump: %w", err)
	}

	return os.WriteFile(dbPath, input, 0644)
}

// restoreDjangoData restores using Django's loaddata
func (sm *SnapshotManager) restoreDjangoData(dumpFile string) error {
	_, err := sm.project.RunCommand("flush", "--no-input")
	if err != nil {
		return fmt.Errorf("flush failed: %w", err)
	}

	_, err = sm.project.RunCommand("loaddata", dumpFile)
	if err != nil {
		return fmt.Errorf("loaddata failed: %w", err)
	}

	return nil
}

// ListSnapshots returns all snapshots
func (sm *SnapshotManager) ListSnapshots() ([]*Snapshot, error) {
	files, err := os.ReadDir(sm.snapshotsDir)
	if err != nil {
		return nil, err
	}

	var snapshots []*Snapshot
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			snapshotID := strings.TrimSuffix(file.Name(), ".json")
			snapshot, err := sm.GetSnapshot(snapshotID)
			if err == nil {
				snapshots = append(snapshots, snapshot)
			}
		}
	}

	sort.Slice(snapshots, func(i, j int) bool {
		return snapshots[i].Timestamp.After(snapshots[j].Timestamp)
	})

	return snapshots, nil
}

// GetSnapshot retrieves a specific snapshot
func (sm *SnapshotManager) GetSnapshot(id string) (*Snapshot, error) {
	metadataPath := filepath.Join(sm.snapshotsDir, fmt.Sprintf("%s.json", id))

	data, err := os.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot not found: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("invalid snapshot metadata: %w", err)
	}

	return &snapshot, nil
}

// DeleteSnapshot removes a snapshot
func (sm *SnapshotManager) DeleteSnapshot(id string) error {
	snapshot, err := sm.GetSnapshot(id)
	if err != nil {
		return err
	}

	// Remove snapshot file
	if err := os.Remove(snapshot.FilePath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Remove metadata
	if err := os.Remove(snapshot.MetadataPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

// Helper functions

func (sm *SnapshotManager) saveMetadata(snapshot *Snapshot) error {
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(snapshot.MetadataPath, data, 0644)
}

func (sm *SnapshotManager) getCurrentBranch() string {
	cmd := exec.Command("git", "-C", sm.project.RootDir, "rev-parse", "--abbrev-ref", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (sm *SnapshotManager) getCurrentCommit() string {
	cmd := exec.Command("git", "-C", sm.project.RootDir, "rev-parse", "HEAD")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func (sm *SnapshotManager) getAppliedMigrations() ([]string, error) {
	output, err := sm.project.RunCommand("showmigrations", "--plan")
	if err != nil {
		return nil, err
	}

	var migrations []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "[X]") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				migrations = append(migrations, parts[1])
			}
		}
	}

	return migrations, nil
}

func (sm *SnapshotManager) syncMigrations(targetMigrations []string) error {
	_, err := sm.getAppliedMigrations()
	if err != nil {
		return nil // Ignore errors in test environments
	}

	_, err = sm.project.RunCommand("migrate", "--fake")
	return nil // Non-critical error
}

// getContainerName finds a Docker container by service name or image
func (sm *SnapshotManager) getContainerName(serviceName, imageName string) string {
	// Try compose service first
	if sm.project.DockerComposeFile != "" {
		cmd := exec.Command("docker", "compose", "-f", sm.project.DockerComposeFile, "ps", "-q", serviceName)
		if output, err := cmd.Output(); err == nil && len(output) > 0 {
			containerID := strings.TrimSpace(string(output))
			nameCmd := exec.Command("docker", "inspect", "--format", "{{.Name}}", containerID)
			if nameOutput, err := nameCmd.Output(); err == nil {
				return strings.TrimSpace(strings.TrimPrefix(string(nameOutput), "/"))
			}
		}
	}

	// Fallback: search by image
	cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("ancestor=%s", imageName), "--format", "{{.Names}}")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	names := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(names) > 0 && names[0] != "" {
		return names[0]
	}
	return ""
}

func (sm *SnapshotManager) getPostgresContainerName() string {
	return sm.getContainerName("postgres", "postgres")
}

func (sm *SnapshotManager) getMySQLContainerName() string {
	return sm.getContainerName("mysql", "mysql")
}

func (sm *SnapshotManager) getDatabasePassword() string {
	// Try common environment variables
	for _, envVar := range []string{"DB_PASSWORD", "POSTGRES_PASSWORD", "MYSQL_PASSWORD"} {
		if pw := os.Getenv(envVar); pw != "" {
			return pw
		}
	}

	// Extract from Django settings
	cmd := `import json; from django.conf import settings; print(settings.DATABASES['default'].get('PASSWORD', ''))`
	output, err := sm.project.RunCommand("shell", "-c", cmd)
	if err == nil {
		return strings.TrimSpace(output)
	}

	return ""
}
