package django

import (
	"os"
	"path/filepath"
	"testing"
)

// TestModelString tests Model string representation
func TestModelStructure(t *testing.T) {
	model := Model{
		App:     "blog",
		Name:    "Post",
		Fields:  5,
		HasMeta: true,
	}

	if model.App != "blog" {
		t.Errorf("Expected App to be blog, got %s", model.App)
	}

	if model.Name != "Post" {
		t.Errorf("Expected Name to be Post, got %s", model.Name)
	}

	if model.Fields != 5 {
		t.Errorf("Expected 5 fields, got %d", model.Fields)
	}
}

// TestMigrationStructure tests Migration struct
func TestMigrationStructure(t *testing.T) {
	migration := Migration{
		App:     "blog",
		Name:    "0001_initial",
		Applied: true,
	}

	if migration.App != "blog" {
		t.Errorf("Expected App to be blog, got %s", migration.App)
	}

	if migration.Name != "0001_initial" {
		t.Errorf("Expected Name to be 0001_initial, got %s", migration.Name)
	}

	if !migration.Applied {
		t.Error("Expected Applied to be true")
	}
}

// TestDatabaseInfoValidation tests database info validation
func TestDatabaseInfoValidation(t *testing.T) {
	tests := []struct {
		name     string
		db       DatabaseInfo
		isUsable bool
	}{
		{
			name: "Valid SQLite",
			db: DatabaseInfo{
				Engine:   "django.db.backends.sqlite3",
				Name:     "/path/to/db.sqlite3",
				IsUsable: true,
			},
			isUsable: true,
		},
		{
			name: "Valid PostgreSQL",
			db: DatabaseInfo{
				Engine:   "django.db.backends.postgresql",
				Name:     "mydb",
				Host:     "localhost",
				Port:     "5432",
				User:     "postgres",
				IsUsable: true,
			},
			isUsable: true,
		},
		{
			name: "Empty database",
			db: DatabaseInfo{
				IsUsable: false,
			},
			isUsable: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.db.IsUsable != tt.isUsable {
				t.Errorf("Expected IsUsable=%v, got %v", tt.isUsable, tt.db.IsUsable)
			}
		})
	}
}

// TestProjectStructure tests Project struct initialization
func TestProjectStructure(t *testing.T) {
	project := &Project{
		ManagePyPath: "/project/manage.py",
		HasDocker:    false,
		Models:       []Model{},
		Migrations:   []Migration{},
		Database: DatabaseInfo{
			IsUsable: false,
		},
	}

	if project.ManagePyPath != "/project/manage.py" {
		t.Errorf("Expected ManagePyPath to be /project/manage.py, got %s", project.ManagePyPath)
	}

	if len(project.Models) != 0 {
		t.Error("Expected empty Models slice")
	}

	if len(project.Migrations) != 0 {
		t.Error("Expected empty Migrations slice")
	}

	if project.Database.IsUsable {
		t.Error("Expected database to be unusable initially")
	}
}

// TestModelRecordStructure tests ModelRecord struct
func TestModelRecordStructure(t *testing.T) {
	record := ModelRecord{
		PK: 1,
		Fields: map[string]interface{}{
			"title":   "Test",
			"content": "Content",
			"active":  true,
		},
		Model: "blog.Post",
	}

	if record.PK != 1 {
		t.Errorf("Expected PK to be 1, got %v", record.PK)
	}

	if len(record.Fields) != 3 {
		t.Errorf("Expected 3 fields, got %d", len(record.Fields))
	}

	if record.Fields["title"] != "Test" {
		t.Errorf("Expected title to be Test, got %v", record.Fields["title"])
	}

	if record.Model != "blog.Post" {
		t.Errorf("Expected Model to be blog.Post, got %s", record.Model)
	}
}

// TestQueryResultStructure tests QueryResult struct
func TestQueryResultStructure(t *testing.T) {
	result := QueryResult{
		Total: 100,
		Records: []ModelRecord{
			{PK: 1, Fields: map[string]interface{}{"name": "Test1"}, Model: "app.Model"},
			{PK: 2, Fields: map[string]interface{}{"name": "Test2"}, Model: "app.Model"},
		},
		Page:     1,
		PageSize: 20,
		HasNext:  true,
		HasPrev:  false,
	}

	if result.Total != 100 {
		t.Errorf("Expected total to be 100, got %d", result.Total)
	}

	if len(result.Records) != 2 {
		t.Errorf("Expected 2 records, got %d", len(result.Records))
	}

	if !result.HasNext {
		t.Error("Expected HasNext to be true")
	}

	if result.HasPrev {
		t.Error("Expected HasPrev to be false")
	}
}

// TestSnapshotMetadataStructure tests Snapshot struct
func TestSnapshotStructure(t *testing.T) {
	snapshot := Snapshot{
		ID:        "snap-001",
		Name:      "test-snapshot",
		GitBranch: "main",
		GitCommit: "abc123",
	}

	if snapshot.Name != "test-snapshot" {
		t.Errorf("Expected name to be test-snapshot, got %s", snapshot.Name)
	}

	if snapshot.GitBranch != "main" {
		t.Errorf("Expected branch to be main, got %s", snapshot.GitBranch)
	}

	if snapshot.ID != "snap-001" {
		t.Errorf("Expected ID to be snap-001, got %s", snapshot.ID)
	}
}

// TestDiscoverProject tests basic project discovery
func TestDiscoverProject(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a minimal Django project structure
	managePyPath := filepath.Join(tmpDir, "manage.py")
	if err := os.WriteFile(managePyPath, []byte("#!/usr/bin/env python\nimport sys"), 0755); err != nil {
		t.Fatal(err)
	}

	project, err := DiscoverProject(tmpDir)
	if err != nil {
		t.Fatalf("DiscoverProject failed: %v", err)
	}

	if project == nil {
		t.Fatal("DiscoverProject returned nil")
	}

	if project.ManagePyPath != managePyPath {
		t.Errorf("Expected ManagePyPath to be %s, got %s", managePyPath, project.ManagePyPath)
	}
}

// TestDataViewerStructure tests DataViewer initialization
func TestDataViewerStructure(t *testing.T) {
	project := &Project{
		ManagePyPath: "/tmp/manage.py",
	}

	dv := NewDataViewer(project)

	if dv == nil {
		t.Fatal("NewDataViewer returned nil")
	}

	if dv.project != project {
		t.Error("DataViewer project not set correctly")
	}
}

// TestSnapshotManagerStructure tests SnapshotManager initialization
func TestSnapshotManagerStructure(t *testing.T) {
	tmpDir := t.TempDir()

	project := &Project{
		ManagePyPath: filepath.Join(tmpDir, "manage.py"),
		Database: DatabaseInfo{
			Engine:   "django.db.backends.sqlite3",
			Name:     filepath.Join(tmpDir, "db.sqlite3"),
			IsUsable: true,
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

func TestExtractLikelySettingsModule(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "setdefault with single quotes",
			line: "os.environ.setdefault('DJANGO_SETTINGS_MODULE', 'mysite.settings')",
			want: "mysite.settings",
		},
		{
			name: "setdefault with double quotes",
			line: "os.environ.setdefault(\"DJANGO_SETTINGS_MODULE\", \"config.settings.dev\")",
			want: "config.settings.dev",
		},
		{
			name: "missing module",
			line: "os.environ.setdefault('DJANGO_SETTINGS_MODULE', '')",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractLikelySettingsModule(tt.line); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
