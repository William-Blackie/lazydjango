package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/williamblackie/lazydjango/pkg/django"
)

func TestNewProjectStateStore(t *testing.T) {
	tmp := t.TempDir()
	store := newProjectStateStore(tmp)
	if store == nil {
		t.Fatal("expected state store for non-empty project root")
	}

	want := filepath.Join(tmp, projectStateDirName, projectStateFileName)
	if store.path != want {
		t.Fatalf("unexpected state path: got %q want %q", store.path, want)
	}

	history := newProjectHistoryStore(tmp)
	if history == nil {
		t.Fatal("expected history store for non-empty project root")
	}
	wantHistory := filepath.Join(tmp, projectStateDirName, projectHistoryFileName)
	if history.path != wantHistory {
		t.Fatalf("unexpected history path: got %q want %q", history.path, wantHistory)
	}

	if got := newProjectStateStore("   "); got != nil {
		t.Fatal("expected nil store for empty project root")
	}
	if got := newProjectHistoryStore("   "); got != nil {
		t.Fatal("expected nil history store for empty project root")
	}
}

func TestSaveAndLoadProjectState(t *testing.T) {
	root := t.TempDir()

	gui := &Gui{
		project:          &django.Project{RootDir: root},
		stateStore:       newProjectStateStore(root),
		historyStore:     newProjectHistoryStore(root),
		currentWindow:    DataWindow,
		menuSelection:    1,
		listSelection:    0,
		dataSelection:    2,
		commandHistory:   []string{"python manage.py check"},
		favoriteCommands: []string{"python manage.py check", "make test"},
		recentModels: []persistedRecentModel{{
			App:           "blog",
			Model:         "Post",
			LastPage:      3,
			LastRecordIdx: 4,
			LastRecordPK:  "42",
		}},
		recentErrors: []persistedRecentError{{
			Key:     "ui|bad",
			Source:  "ui",
			Message: "bad",
			Count:   2,
		}},
		outputTabs:   make(map[string]*outputTabState),
		outputOrder:  make([]string, 0),
		outputRoutes: make(map[string]string),
	}

	commandTab := gui.startCommandOutputTab("Run Checks")
	gui.appendOutput(commandTab, strings.Repeat("x", maxPersistedOutputTextBytes+128))
	logsTab := gui.startLogsOutputTab("Dev Server")
	gui.appendOutput(logsTab, "line-1\nline-2\n")
	gui.switchOutputTab(logsTab)
	gui.stateDirty = true

	if err := gui.saveProjectState(); err != nil {
		t.Fatalf("saveProjectState failed: %v", err)
	}
	if gui.stateDirty {
		t.Fatal("expected stateDirty=false after save")
	}

	statePath := filepath.Join(root, projectStateDirName, projectStateFileName)
	if _, err := os.Stat(statePath); err != nil {
		t.Fatalf("expected state file to exist: %v", err)
	}

	loaded := &Gui{
		project:      &django.Project{RootDir: root},
		stateStore:   newProjectStateStore(root),
		historyStore: newProjectHistoryStore(root),
		outputTabs:   make(map[string]*outputTabState),
		outputOrder:  make([]string, 0),
		outputRoutes: make(map[string]string),
	}
	if err := loaded.loadProjectState(); err != nil {
		t.Fatalf("loadProjectState failed: %v", err)
	}

	if loaded.currentWindow != DataWindow {
		t.Fatalf("expected currentWindow=%s, got %s", DataWindow, loaded.currentWindow)
	}
	if loaded.menuSelection != 1 {
		t.Fatalf("expected menuSelection=1, got %d", loaded.menuSelection)
	}
	if loaded.dataSelection != 2 {
		t.Fatalf("expected dataSelection=2, got %d", loaded.dataSelection)
	}
	if len(loaded.commandHistory) != 1 {
		t.Fatalf("expected 1 command history entry, got %d", len(loaded.commandHistory))
	}
	if len(loaded.favoriteCommands) != 2 {
		t.Fatalf("expected 2 favorites, got %d", len(loaded.favoriteCommands))
	}
	if len(loaded.recentModels) != 1 {
		t.Fatalf("expected 1 recent model, got %d", len(loaded.recentModels))
	}
	if len(loaded.recentErrors) != 1 {
		t.Fatalf("expected 1 recent error, got %d", len(loaded.recentErrors))
	}
	if len(loaded.outputOrder) != 2 {
		t.Fatalf("expected 2 restored output tabs, got %d", len(loaded.outputOrder))
	}

	activeID := loaded.resolveOutputTabID("", false)
	if activeID == "" {
		t.Fatal("expected an active restored output tab")
	}
	if loaded.outputTabs[activeID].route != OutputTabLogs {
		t.Fatalf("expected logs tab to be active, got route=%q", loaded.outputTabs[activeID].route)
	}

	var restoredCommandText string
	for _, id := range loaded.outputOrder {
		tab := loaded.outputTabs[id]
		if tab.route == OutputTabCommand {
			restoredCommandText = tab.text
			break
		}
	}
	if restoredCommandText == "" {
		t.Fatal("expected restored command tab text")
	}
	if len(restoredCommandText) > maxPersistedOutputTextBytes {
		t.Fatalf("expected truncated command text <= %d bytes, got %d", maxPersistedOutputTextBytes, len(restoredCommandText))
	}
	if !strings.Contains(restoredCommandText, "truncated from previous session") {
		t.Fatal("expected truncation marker in restored command text")
	}
}

func TestLoadProjectStateInvalidJSON(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, projectStateDirName, projectStateFileName)
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		t.Fatalf("failed to create state directory: %v", err)
	}
	if err := os.WriteFile(statePath, []byte("{invalid"), 0644); err != nil {
		t.Fatalf("failed to write invalid state file: %v", err)
	}

	gui := &Gui{
		project:      &django.Project{RootDir: root},
		stateStore:   newProjectStateStore(root),
		historyStore: newProjectHistoryStore(root),
		outputTabs:   make(map[string]*outputTabState),
		outputOrder:  make([]string, 0),
		outputRoutes: make(map[string]string),
	}
	if err := gui.loadProjectState(); err == nil {
		t.Fatal("expected loadProjectState to fail for invalid JSON")
	}
}

func TestRememberCommandKeepsRecentEntriesAndFavorites(t *testing.T) {
	gui := &Gui{}
	for i := 0; i < maxPersistedCommandHistoryLen+25; i++ {
		gui.rememberCommand(fmt.Sprintf("cmd-%03d", i))
	}

	if len(gui.commandHistory) != maxPersistedCommandHistoryLen {
		t.Fatalf("expected %d history entries, got %d", maxPersistedCommandHistoryLen, len(gui.commandHistory))
	}
	if gui.commandHistory[0] != "cmd-025" {
		t.Fatalf("expected oldest retained command to be cmd-025, got %q", gui.commandHistory[0])
	}
	if gui.commandHistory[len(gui.commandHistory)-1] != fmt.Sprintf("cmd-%03d", maxPersistedCommandHistoryLen+24) {
		t.Fatalf("unexpected newest command: %q", gui.commandHistory[len(gui.commandHistory)-1])
	}
	if len(gui.favoriteCommands) != maxPersistedFavoriteCommands {
		t.Fatalf("expected %d favorites, got %d", maxPersistedFavoriteCommands, len(gui.favoriteCommands))
	}
	if gui.favoriteCommands[0] != fmt.Sprintf("cmd-%03d", maxPersistedCommandHistoryLen+24) {
		t.Fatalf("unexpected favorite head: %q", gui.favoriteCommands[0])
	}
}

func TestSanitizeCommandRedactsSecrets(t *testing.T) {
	raw := `python manage.py check --token abc123 PASSWORD=hunter2 https://u:pw@example.com`
	safe := sanitizeCommand(raw)
	if strings.Contains(safe, "abc123") {
		t.Fatalf("expected token to be redacted: %q", safe)
	}
	if strings.Contains(strings.ToLower(safe), "hunter2") {
		t.Fatalf("expected password assignment to be redacted: %q", safe)
	}
	if strings.Contains(safe, ":pw@") {
		t.Fatalf("expected URI password to be redacted: %q", safe)
	}
	if !strings.Contains(safe, "[REDACTED]") {
		t.Fatalf("expected redaction marker in sanitized command: %q", safe)
	}
}

func TestRememberErrorDedupes(t *testing.T) {
	gui := &Gui{}
	gui.rememberError("docker", "container failed")
	gui.rememberError("docker", "container failed")
	gui.rememberError("docker", "other error")

	if len(gui.recentErrors) != 2 {
		t.Fatalf("expected 2 deduped errors, got %d", len(gui.recentErrors))
	}
	if gui.recentErrors[0].Message != "other error" {
		t.Fatalf("expected newest error first, got %q", gui.recentErrors[0].Message)
	}
	if gui.recentErrors[1].Count != 2 {
		t.Fatalf("expected deduped count=2, got %d", gui.recentErrors[1].Count)
	}
}

func TestRememberModelAccessMRU(t *testing.T) {
	gui := &Gui{}
	gui.rememberModelAccess("blog", "Post", 2, 4, 11)
	gui.rememberModelAccess("shop", "Order", 1, 0, "x")
	gui.rememberModelAccess("blog", "Post", 3, 2, 13)

	if len(gui.recentModels) != 2 {
		t.Fatalf("expected 2 model entries, got %d", len(gui.recentModels))
	}
	if gui.recentModels[0].App != "blog" || gui.recentModels[0].Model != "Post" {
		t.Fatalf("expected blog.Post to be MRU, got %s.%s", gui.recentModels[0].App, gui.recentModels[0].Model)
	}
	if gui.recentModels[0].LastPage != 3 || gui.recentModels[0].LastRecordIdx != 2 {
		t.Fatalf("unexpected model state: %+v", gui.recentModels[0])
	}
}

func TestProjectHistoryStoreAppendAndTail(t *testing.T) {
	root := t.TempDir()
	store := newProjectHistoryStore(root)
	if store == nil {
		t.Fatal("expected history store")
	}

	oldEvent := historyEvent{
		Time:   time.Now().UTC().AddDate(0, 0, -(historyRetentionDays + 1)).Format(time.RFC3339),
		Type:   "command",
		Status: "success",
	}
	if err := store.append(oldEvent); err != nil {
		t.Fatalf("append old event failed: %v", err)
	}

	for i := 0; i < 6; i++ {
		event := historyEvent{
			Time:    time.Now().UTC().Format(time.RFC3339),
			Type:    "command",
			Status:  "success",
			Command: fmt.Sprintf("cmd-%d", i),
		}
		if err := store.append(event); err != nil {
			t.Fatalf("append event %d failed: %v", i, err)
		}
	}

	events, err := store.tail(3)
	if err != nil {
		t.Fatalf("tail failed: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 tail events, got %d", len(events))
	}
	if events[0].Command != "cmd-3" || events[2].Command != "cmd-5" {
		t.Fatalf("unexpected tail ordering/content: %+v", events)
	}

	all, err := store.tail(100)
	if err != nil {
		t.Fatalf("tail all failed: %v", err)
	}
	for _, e := range all {
		if e.Command == "" && e.Type == "command" {
			t.Fatal("expected old event to be compacted out")
		}
	}
}

func TestRecordCommandExecutionWritesHistoryAndErrors(t *testing.T) {
	root := t.TempDir()
	gui := &Gui{
		project:      &django.Project{RootDir: root},
		stateStore:   newProjectStateStore(root),
		historyStore: newProjectHistoryStore(root),
		outputTabs:   make(map[string]*outputTabState),
		outputOrder:  make([]string, 0),
		outputRoutes: make(map[string]string),
	}
	tabID := gui.startCommandOutputTab("Command")

	gui.recordCommandExecution("python manage.py check --token abc", tabID, time.Now().Add(-100*time.Millisecond), fmt.Errorf("exit status 1"))

	if len(gui.commandHistory) != 1 {
		t.Fatalf("expected command history entry, got %d", len(gui.commandHistory))
	}
	if strings.Contains(gui.commandHistory[0], "abc") {
		t.Fatalf("expected sanitized command history, got %q", gui.commandHistory[0])
	}
	if len(gui.recentErrors) != 1 {
		t.Fatalf("expected command error captured, got %d", len(gui.recentErrors))
	}

	events, err := gui.historyStore.tail(1)
	if err != nil {
		t.Fatalf("tail failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 history event, got %d", len(events))
	}
	if events[0].Type != "command" || events[0].Status != "error" {
		t.Fatalf("unexpected command event: %+v", events[0])
	}
	if strings.Contains(events[0].Command, "abc") {
		t.Fatalf("expected sanitized event command, got %q", events[0].Command)
	}
}
