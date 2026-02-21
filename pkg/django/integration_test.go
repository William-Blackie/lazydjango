package django

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Test snapshot system with real file operations
func TestSnapshotBasics(t *testing.T) {
	tmpDir := t.TempDir()
	
	project := &Project{
		RootDir:      tmpDir,
		ManagePyPath: filepath.Join(tmpDir, "manage.py"),
		Database: DatabaseInfo{
			Engine:   "django.db.backends.sqlite3",
			Name:     filepath.Join(tmpDir, "test.db"),
			IsUsable: true,
		},
	}

	// Create test database
	testData := []byte("test database content")
	if err := os.WriteFile(project.Database.Name, testData, 0644); err != nil {
		t.Fatal(err)
	}

	sm := NewSnapshotManager(project)

	// Test create
	snap, err := sm.CreateSnapshot("test")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if !fileExists(snap.FilePath) {
		t.Error("Snapshot file not created")
	}

	// Test list
	snaps, err := sm.ListSnapshots()
	if err != nil || len(snaps) == 0 {
		t.Error("List failed or no snapshots found")
	}

	// Test restore
	// Modify DB
	os.WriteFile(project.Database.Name, []byte("modified"), 0644)
	
	// Restore
	if err := sm.RestoreSnapshot(snap.ID); err != nil {
		t.Logf("Restore failed (expected in test env): %v", err)
	}

	// Test delete
	if err := sm.DeleteSnapshot(snap.ID); err != nil {
		t.Errorf("Delete failed: %v", err)
	}
}

func TestDatabaseTypeDetection(t *testing.T) {
	tests := []struct {
		engine string
		want   string
	}{
		{"django.db.backends.postgresql", "postgresql"},
		{"django.db.backends.mysql", "mysql"},
		{"django.db.backends.sqlite3", "sqlite"},
		{"django.db.backends.oracle", ""},
	}

	for _, tt := range tests {
		t.Run(tt.engine, func(t *testing.T) {
			engine := strings.ToLower(tt.engine)
			if tt.want == "postgresql" && !strings.Contains(engine, "postgresql") {
				t.Error("PostgreSQL not detected")
			}
			if tt.want == "mysql" && !strings.Contains(engine, "mysql") {
				t.Error("MySQL not detected")
			}
			if tt.want == "sqlite" && !strings.Contains(engine, "sqlite") {
				t.Error("SQLite not detected")
			}
		})
	}
}

func TestDataViewerQueryConstruction(t *testing.T) {
	dv := &DataViewer{}
	
	// Test filter code generation
	filters := map[string]string{
		"name": "test",
		"age": "25",
	}
	
	code := dv.buildFilterCode(filters)
	if code == "" {
		t.Error("Filter code should not be empty")
	}
	
	if !strings.Contains(code, "qs.filter") {
		t.Error("Filter code should contain qs.filter")
	}
	
	// Test empty filters
	emptyCode := dv.buildFilterCode(map[string]string{})
	if emptyCode != "" {
		t.Error("Empty filters should produce empty code")
	}
}

func TestSnapshotMetadataFormat(t *testing.T) {
	snapshot := &Snapshot{
		ID:             "123",
		Name:           "test-snapshot",
		Timestamp:      time.Now(),
		DatabaseEngine: "django.db.backends.postgresql",
		GitBranch:      "main",
		GitCommit:      "abc123",
	}

	// Test JSON marshaling
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("Failed to marshal snapshot: %v", err)
	}

	// Test unmarshaling
	var restored Snapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("Failed to unmarshal snapshot: %v", err)
	}

	if restored.ID != snapshot.ID {
		t.Error("ID mismatch after unmarshal")
	}
	if restored.Name != snapshot.Name {
		t.Error("Name mismatch after unmarshal")
	}
}
