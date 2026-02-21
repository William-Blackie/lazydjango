package django

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestSnapshotManager tests the snapshot system with various database types
func TestSnapshotManager(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create mock project
	project := &Project{
		RootDir:      tmpDir,
		ManagePyPath: filepath.Join(tmpDir, "manage.py"),
		Database: DatabaseInfo{
			Engine:   "django.db.backends.sqlite3",
			Name:     filepath.Join(tmpDir, "db.sqlite3"),
			IsUsable: true,
		},
	}

	// Create fake database file
	os.WriteFile(project.Database.Name, []byte("fake db content"), 0644)

	sm := NewSnapshotManager(project)

	t.Run("CreateSQLiteSnapshot", func(t *testing.T) {
		snapshot, err := sm.CreateSnapshot("test-snapshot")
		if err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		if snapshot.Name != "test-snapshot" {
			t.Errorf("Expected name 'test-snapshot', got '%s'", snapshot.Name)
		}

		if !fileExists(snapshot.FilePath) {
			t.Error("Snapshot file was not created")
		}

		if !fileExists(snapshot.MetadataPath) {
			t.Error("Metadata file was not created")
		}
	})

	t.Run("ListSnapshots", func(t *testing.T) {
		snapshots, err := sm.ListSnapshots()
		if err != nil {
			t.Fatalf("Failed to list snapshots: %v", err)
		}

		if len(snapshots) < 1 {
			t.Error("Expected at least one snapshot")
		}
	})

	t.Run("GetSnapshot", func(t *testing.T) {
		// Create a test snapshot first
		created, err := sm.CreateSnapshot("get-test")
		if err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		retrieved, err := sm.GetSnapshot(created.ID)
		if err != nil {
			t.Fatalf("Failed to get snapshot: %v", err)
		}

		if retrieved.ID != created.ID {
			t.Errorf("Expected ID '%s', got '%s'", created.ID, retrieved.ID)
		}

		if retrieved.Name != created.Name {
			t.Errorf("Expected name '%s', got '%s'", created.Name, retrieved.Name)
		}
	})

	t.Run("RestoreSQLiteSnapshot", func(t *testing.T) {
		// Create snapshot
		snapshot, err := sm.CreateSnapshot("restore-test")
		if err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		// Modify database
		os.WriteFile(project.Database.Name, []byte("modified content"), 0644)

		// Restore snapshot
		err = sm.RestoreSnapshot(snapshot.ID)
		if err != nil {
			t.Fatalf("Failed to restore snapshot: %v", err)
		}

		// Verify restoration
		content, err := os.ReadFile(project.Database.Name)
		if err != nil {
			t.Fatalf("Failed to read restored db: %v", err)
		}

		if string(content) != "fake db content" {
			t.Error("Database was not properly restored")
		}
	})

	t.Run("DeleteSnapshot", func(t *testing.T) {
		// Create snapshot
		snapshot, err := sm.CreateSnapshot("delete-test")
		if err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		// Delete it
		err = sm.DeleteSnapshot(snapshot.ID)
		if err != nil {
			t.Fatalf("Failed to delete snapshot: %v", err)
		}

		// Verify deletion
		if fileExists(snapshot.FilePath) {
			t.Error("Snapshot file still exists after deletion")
		}

		if fileExists(snapshot.MetadataPath) {
			t.Error("Metadata file still exists after deletion")
		}
	})

	t.Run("GitIntegration", func(t *testing.T) {
		// Initialize git repo in temp dir
		os.WriteFile(filepath.Join(tmpDir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644)
		os.MkdirAll(filepath.Join(tmpDir, ".git", "refs", "heads"), 0755)
		os.WriteFile(filepath.Join(tmpDir, ".git", "refs", "heads", "main"), []byte("abc123def456"), 0644)

		snapshot, err := sm.CreateSnapshot("git-test")
		if err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		// Git branch should be captured (if git is available)
		if snapshot.GitBranch == "" && fileExists(filepath.Join(tmpDir, ".git")) {
			t.Log("Git branch not captured (git may not be available)")
		}
	})

	t.Run("SnapshotMetadata", func(t *testing.T) {
		snapshot, err := sm.CreateSnapshot("metadata-test")
		if err != nil {
			t.Fatalf("Failed to create snapshot: %v", err)
		}

		if snapshot.Timestamp.IsZero() {
			t.Error("Snapshot timestamp is zero")
		}

		if snapshot.DatabaseEngine != project.Database.Engine {
			t.Errorf("Expected engine '%s', got '%s'", project.Database.Engine, snapshot.DatabaseEngine)
		}

		if time.Since(snapshot.Timestamp) > time.Minute {
			t.Error("Snapshot timestamp seems incorrect")
		}
	})
}

// TestPostgreSQLSnapshot tests PostgreSQL-specific snapshot functionality
func TestPostgreSQLSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping PostgreSQL test in short mode")
	}

	tmpDir := t.TempDir()

	project := &Project{
		RootDir:      tmpDir,
		ManagePyPath: filepath.Join(tmpDir, "manage.py"),
		Database: DatabaseInfo{
			Engine:   "django.db.backends.postgresql",
			Name:     "testdb",
			Host:     "localhost",
			Port:     "5432",
			User:     "testuser",
			IsUsable: true,
		},
		HasDocker: false,
	}

	sm := NewSnapshotManager(project)

	t.Run("PostgreSQLDumpCommand", func(t *testing.T) {
		// This will fail without actual postgres, but tests command construction
		err := sm.dumpPostgreSQL(filepath.Join(tmpDir, "test.sql"))
		if err == nil {
			t.Log("Unexpected success - postgres may be running")
		} else {
			t.Logf("Expected failure without postgres: %v", err)
		}
	})
}

// TestMySQLSnapshot tests MySQL-specific snapshot functionality
func TestMySQLSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping MySQL test in short mode")
	}

	tmpDir := t.TempDir()

	project := &Project{
		RootDir:      tmpDir,
		ManagePyPath: filepath.Join(tmpDir, "manage.py"),
		Database: DatabaseInfo{
			Engine:   "django.db.backends.mysql",
			Name:     "testdb",
			Host:     "localhost",
			Port:     "3306",
			User:     "testuser",
			IsUsable: true,
		},
		HasDocker: false,
	}

	sm := NewSnapshotManager(project)

	t.Run("MySQLDumpCommand", func(t *testing.T) {
		// This will fail without actual mysql, but tests command construction
		err := sm.dumpMySQL(filepath.Join(tmpDir, "test.sql"))
		if err == nil {
			t.Log("Unexpected success - mysql may be running")
		} else {
			t.Logf("Expected failure without mysql: %v", err)
		}
	})
}

// TestDockerSnapshot tests Docker-based snapshot operations
func TestDockerSnapshot(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Docker test in short mode")
	}

	tmpDir := t.TempDir()
	composeFile := filepath.Join(tmpDir, "compose.yaml")

	// Create mock compose file
	composeContent := `services:
  postgres:
    image: postgres:16-alpine
    container_name: test-postgres
`
	os.WriteFile(composeFile, []byte(composeContent), 0644)

	project := &Project{
		RootDir:           tmpDir,
		ManagePyPath:      filepath.Join(tmpDir, "manage.py"),
		HasDocker:         true,
		DockerComposeFile: composeFile,
		Database: DatabaseInfo{
			Engine:   "django.db.backends.postgresql",
			Name:     "testdb",
			User:     "testuser",
			IsUsable: true,
		},
	}

	sm := NewSnapshotManager(project)

	t.Run("DetectDockerContainer", func(t *testing.T) {
		containerName := sm.getPostgresContainerName()
		t.Logf("Found postgres container: %s", containerName)
		// Container may or may not exist - just testing detection logic
	})

	t.Run("DockerPostgresDump", func(t *testing.T) {
		// Test the new consolidated method
		err := sm.dumpPostgreSQL(filepath.Join(tmpDir, "test.sql"))
		if err != nil {
			t.Logf("Expected failure without running container: %v", err)
		}
	})
}

// TestSnapshotEdgeCases tests error handling and edge cases
func TestSnapshotEdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	project := &Project{
		RootDir:      tmpDir,
		ManagePyPath: filepath.Join(tmpDir, "manage.py"),
		Database: DatabaseInfo{
			Engine:   "django.db.backends.sqlite3",
			Name:     filepath.Join(tmpDir, "db.sqlite3"),
			IsUsable: true,
		},
	}

	sm := NewSnapshotManager(project)

	t.Run("GetNonExistentSnapshot", func(t *testing.T) {
		_, err := sm.GetSnapshot("nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent snapshot")
		}
	})

	t.Run("DeleteNonExistentSnapshot", func(t *testing.T) {
		err := sm.DeleteSnapshot("nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent snapshot")
		}
	})

	t.Run("RestoreNonExistentSnapshot", func(t *testing.T) {
		err := sm.RestoreSnapshot("nonexistent")
		if err == nil {
			t.Error("Expected error for nonexistent snapshot")
		}
	})

	t.Run("EmptySnapshotName", func(t *testing.T) {
		// Create DB file first
		os.WriteFile(project.Database.Name, []byte("test"), 0644)

		snapshot, err := sm.CreateSnapshot("")
		if err != nil {
			t.Fatalf("Failed to create snapshot with empty name: %v", err)
		}

		if snapshot.Name == "" {
			t.Error("Snapshot name should be auto-generated")
		}
	})

	t.Run("MissingDatabaseFile", func(t *testing.T) {
		project.Database.Name = filepath.Join(tmpDir, "nonexistent.db")
		err := sm.dumpSQLite(filepath.Join(tmpDir, "test.sql"))
		if err == nil {
			t.Error("Expected error for missing database file")
		}
	})
}

func TestSnapshotIDsAreUniqueAndOrdered(t *testing.T) {
	tmpDir := t.TempDir()
	project := &Project{
		RootDir:      tmpDir,
		ManagePyPath: filepath.Join(tmpDir, "manage.py"),
		Database: DatabaseInfo{
			Engine:   "django.db.backends.sqlite3",
			Name:     filepath.Join(tmpDir, "db.sqlite3"),
			IsUsable: true,
		},
	}
	if err := os.WriteFile(project.Database.Name, []byte("seed"), 0644); err != nil {
		t.Fatal(err)
	}

	sm := NewSnapshotManager(project)

	first, err := sm.CreateSnapshot("first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := sm.CreateSnapshot("second")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("snapshot IDs must be unique")
	}

	snaps, err := sm.ListSnapshots()
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) < 2 {
		t.Fatalf("expected at least 2 snapshots, got %d", len(snaps))
	}
	if snaps[0].Timestamp.Before(snaps[1].Timestamp) {
		t.Fatal("snapshots should be sorted newest-first")
	}
}

// BenchmarkSnapshotOperations benchmarks snapshot performance
func BenchmarkSnapshotOperations(b *testing.B) {
	tmpDir := b.TempDir()

	project := &Project{
		RootDir:      tmpDir,
		ManagePyPath: filepath.Join(tmpDir, "manage.py"),
		Database: DatabaseInfo{
			Engine:   "django.db.backends.sqlite3",
			Name:     filepath.Join(tmpDir, "db.sqlite3"),
			IsUsable: true,
		},
	}

	// Create fake database
	dbContent := make([]byte, 1024*1024) // 1MB fake data
	os.WriteFile(project.Database.Name, dbContent, 0644)

	sm := NewSnapshotManager(project)

	b.Run("CreateSnapshot", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sm.CreateSnapshot("bench-test")
		}
	})

	b.Run("ListSnapshots", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sm.ListSnapshots()
		}
	})
}

func TestShouldUseDjangoDumpFallback(t *testing.T) {
	yes := func(string) bool { return true }
	no := func(string) bool { return false }

	if !shouldUseDjangoDumpFallback("django.db.backends.postgresql", false, no) {
		t.Fatal("expected postgres fallback when pg_dump is missing")
	}
	if shouldUseDjangoDumpFallback("django.db.backends.postgresql", false, yes) {
		t.Fatal("expected no postgres fallback when pg_dump is available")
	}
	if !shouldUseDjangoDumpFallback("django.db.backends.mysql", false, no) {
		t.Fatal("expected mysql fallback when mysqldump is missing")
	}
	if shouldUseDjangoDumpFallback("django.db.backends.mysql", true, no) {
		t.Fatal("docker projects should prefer native-in-container tooling")
	}
}

func TestSnapshotFileExtension(t *testing.T) {
	if got := snapshotFileExtension("django.db.backends.sqlite3", false); got != ".sqlite3" {
		t.Fatalf("expected sqlite extension, got %q", got)
	}
	if got := snapshotFileExtension("django.db.backends.postgresql", true); got != ".json" {
		t.Fatalf("expected json extension for fallback snapshots, got %q", got)
	}
	if got := snapshotFileExtension("django.db.backends.postgresql", false); got != ".sql" {
		t.Fatalf("expected sql extension for native postgres snapshots, got %q", got)
	}
}
