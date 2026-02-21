package django

import (
	"testing"
)

// Test the new helper functions work correctly
func TestDataViewerHelpers(t *testing.T) {
	// Create a mock project
	project := &Project{
		RootDir:      "/tmp/test",
		ManagePyPath: "/tmp/test/manage.py",
		Database: DatabaseInfo{
			Engine: "django.db.backends.sqlite3",
			Name:   "db.sqlite3",
		},
	}
	
	dv := NewDataViewer(project)
	
	// Test that DataViewer is created properly
	if dv == nil {
		t.Fatal("NewDataViewer returned nil")
	}
	
	if dv.project != project {
		t.Error("DataViewer project not set correctly")
	}
}

func TestSerializeFieldsCode(t *testing.T) {
	// Just verify the constant exists and has expected content
	if serializeFieldsCode == "" {
		t.Error("serializeFieldsCode is empty")
	}
	
	// Check it contains expected keywords
	expectedKeywords := []string{"fields", "getattr", "isoformat", "pk"}
	for _, keyword := range expectedKeywords {
		found := false
		for i := 0; i <= len(serializeFieldsCode)-len(keyword); i++ {
			if i+len(keyword) <= len(serializeFieldsCode) && 
				serializeFieldsCode[i:i+len(keyword)] == keyword {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("serializeFieldsCode missing expected keyword: %s", keyword)
		}
	}
}

func TestSnapshotManagerHelpers(t *testing.T) {
	project := &Project{
		RootDir:      "/tmp/test",
		ManagePyPath: "/tmp/test/manage.py",
		Database: DatabaseInfo{
			Engine: "django.db.backends.postgresql",
			Name:   "testdb",
			Host:   "localhost",
			Port:   "5432",
			User:   "testuser",
		},
	}
	
	sm := NewSnapshotManager(project)
	
	if sm == nil {
		t.Fatal("NewSnapshotManager returned nil")
	}
	
	if sm.project != project {
		t.Error("SnapshotManager project not set correctly")
	}
}

func TestDbConfigStructure(t *testing.T) {
	// Verify dbConfigs map is properly initialized
	if len(dbConfigs) == 0 {
		t.Error("dbConfigs map is empty")
	}
	
	// Check PostgreSQL config
	if pgConfig, ok := dbConfigs["postgresql"]; !ok {
		t.Error("PostgreSQL config missing")
	} else {
		if pgConfig.dumpCmd != "pg_dump" {
			t.Errorf("Expected pg_dump, got %s", pgConfig.dumpCmd)
		}
		if pgConfig.restoreCmd != "psql" {
			t.Errorf("Expected psql, got %s", pgConfig.restoreCmd)
		}
	}
	
	// Check MySQL config
	if mysqlConfig, ok := dbConfigs["mysql"]; !ok {
		t.Error("MySQL config missing")
	} else {
		if mysqlConfig.dumpCmd != "mysqldump" {
			t.Errorf("Expected mysqldump, got %s", mysqlConfig.dumpCmd)
		}
		if mysqlConfig.restoreCmd != "mysql" {
			t.Errorf("Expected mysql, got %s", mysqlConfig.restoreCmd)
		}
	}
}

func TestBuildDockerCommand(t *testing.T) {
	project := &Project{
		RootDir:           "/tmp/test",
		ManagePyPath:      "/tmp/test/manage.py",
		HasDocker:         true,
		DockerService:     "web",
		DockerComposeFile: "/tmp/test/compose.yaml",
	}
	
	cmd := project.buildDockerCommand("migrate", "--fake")
	
	if cmd == nil {
		t.Fatal("buildDockerCommand returned nil")
	}
	
	// Verify command structure
	args := cmd.Args
	if len(args) == 0 {
		t.Fatal("Command has no args")
	}
	
	// Should contain compose, exec, service name, and our arguments
	expectedParts := []string{"compose", "exec", "web", "migrate", "--fake"}
	for _, part := range expectedParts {
		found := false
		for _, arg := range args {
			if arg == part {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Command missing expected part: %s", part)
		}
	}
}

// Helper function
func joinArgs(args []string) string {
	result := ""
	for _, arg := range args {
		result += arg + " "
	}
	return result
}
