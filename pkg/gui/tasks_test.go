package gui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/williamblackie/lazydjango/pkg/django"
)

func TestLoadProjectTasksCreatesFileOnFirstInit(t *testing.T) {
	root := t.TempDir()
	gui := &Gui{
		project: &django.Project{
			RootDir: root,
		},
		makeTargetsLoaded: true,
		makeTargets: []makeTarget{
			{name: "test", description: "Run tests"},
		},
	}

	tasks, err := gui.loadProjectTasks(false)
	if err != nil {
		t.Fatalf("loadProjectTasks failed: %v", err)
	}
	if len(tasks) == 0 {
		t.Fatal("expected initial tasks to be generated")
	}

	found := false
	for _, task := range tasks {
		if task.Command == "make test" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected generated tasks to include make target command, got %+v", tasks)
	}

	path := filepath.Join(root, ".lazy-django", projectTasksFileName)
	if _, statErr := os.Stat(path); statErr != nil {
		t.Fatalf("expected project tasks file to exist: %v", statErr)
	}
}

func TestLoadProjectTasksUsesExistingFile(t *testing.T) {
	root := t.TempDir()
	gui := &Gui{
		project: &django.Project{
			RootDir: root,
		},
		makeTargetsLoaded: true,
		makeTargets: []makeTarget{
			{name: "test", description: "Run tests"},
		},
	}

	if _, err := gui.loadProjectTasks(false); err != nil {
		t.Fatalf("initial loadProjectTasks failed: %v", err)
	}

	custom := []projectTaskEntry{
		{Label: "Custom Check", Command: "python manage.py check --deploy"},
	}
	cfg := projectTaskConfig{
		Version: projectTasksVersion,
		Tasks:   custom,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal custom tasks: %v", err)
	}

	path := filepath.Join(root, ".lazy-django", projectTasksFileName)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write custom tasks: %v", err)
	}

	gui.makeTargets = []makeTarget{
		{name: "lint", description: "Run lint checks"},
	}

	tasks, err := gui.loadProjectTasks(true)
	if err != nil {
		t.Fatalf("loadProjectTasks(force=true) failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 custom task, got %d", len(tasks))
	}
	if tasks[0].Command != "python manage.py check --deploy" {
		t.Fatalf("expected custom command to be preserved, got %q", tasks[0].Command)
	}
}
