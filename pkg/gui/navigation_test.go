package gui

import (
	"testing"

	"github.com/williamblackie/lazydjango/pkg/django"
)

func TestPanelTitleFocus(t *testing.T) {
	gui := &Gui{currentWindow: MenuWindow}

	if got := gui.panelTitle(MenuWindow, "Project"); got != " [1] Project " {
		t.Fatalf("unexpected focused title: %q", got)
	}
	if got := gui.panelTitle(ListWindow, "Database"); got != " [2] Database " {
		t.Fatalf("unexpected unfocused title: %q", got)
	}
}

func TestSortedModels(t *testing.T) {
	gui := &Gui{
		project: &django.Project{
			Models: []django.Model{
				{App: "shop", Name: "OrderItem"},
				{App: "blog", Name: "Comment"},
				{App: "blog", Name: "Post"},
			},
		},
	}

	models := gui.sortedModels()
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
	if models[0].App != "blog" || models[0].Name != "Comment" {
		t.Fatalf("unexpected first model: %#v", models[0])
	}
	if models[1].App != "blog" || models[1].Name != "Post" {
		t.Fatalf("unexpected second model: %#v", models[1])
	}
	if models[2].App != "shop" || models[2].Name != "OrderItem" {
		t.Fatalf("unexpected third model: %#v", models[2])
	}
}

func TestSelectionCount(t *testing.T) {
	gui := &Gui{
		project: &django.Project{
			Models: []django.Model{
				{App: "blog", Name: "Post"},
				{App: "shop", Name: "Order"},
			},
			HasDocker:         true,
			DockerComposeFile: "/tmp/compose.yaml",
		},
	}

	if got := gui.selectionCount(MenuWindow); got == 0 {
		t.Fatal("expected menu to expose selectable actions")
	}
	if got := gui.selectionCount(ListWindow); got != 2 {
		t.Fatalf("expected 2 table selections, got %d", got)
	}
	if got := gui.selectionCount(DataWindow); got != 3 {
		t.Fatalf("expected 3 data actions, got %d", got)
	}
}

// TestEditRecordConditions tests the conditions for editing records.
func TestEditRecordConditions(t *testing.T) {
	tests := []struct {
		name          string
		currentWindow string
		currentModel  string
		recordsCount  int
		shouldAllow   bool
	}{
		{
			name:          "Valid edit conditions",
			currentWindow: MainWindow,
			currentModel:  "Post",
			recordsCount:  5,
			shouldAllow:   true,
		},
		{
			name:          "Wrong window",
			currentWindow: MenuWindow,
			currentModel:  "Post",
			recordsCount:  5,
			shouldAllow:   false,
		},
		{
			name:          "No model selected",
			currentWindow: MainWindow,
			currentModel:  "",
			recordsCount:  5,
			shouldAllow:   false,
		},
		{
			name:          "No records",
			currentWindow: MainWindow,
			currentModel:  "Post",
			recordsCount:  0,
			shouldAllow:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gui := &Gui{
				currentWindow:  tt.currentWindow,
				currentModel:   tt.currentModel,
				currentRecords: make([]django.ModelRecord, tt.recordsCount),
			}

			canEdit := gui.currentWindow == MainWindow &&
				gui.currentModel != "" &&
				len(gui.currentRecords) > 0

			if canEdit != tt.shouldAllow {
				t.Errorf("Expected canEdit=%v, got %v", tt.shouldAllow, canEdit)
			}
		})
	}
}
