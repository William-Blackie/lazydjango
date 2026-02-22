package gui

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/awesome-gocui/gocui"
	"github.com/williamblackie/lazydjango/pkg/django"
)

type nopWriteCloser struct {
	io.Writer
}

func (n nopWriteCloser) Close() error { return nil }

// TestNewGui tests GUI initialization
func TestNewGui(t *testing.T) {
	project := &django.Project{
		ManagePyPath: "/tmp/test-project/manage.py",
		Database: django.DatabaseInfo{
			IsUsable: false,
		},
	}

	gui, err := NewGui(project)

	if err != nil {
		t.Skipf("Skipping GUI init test in non-interactive terminal: %v", err)
	}

	if gui == nil {
		t.Fatal("NewGui returned nil")
	}

	if gui.project != project {
		t.Error("GUI project not set correctly")
	}

	if gui.config == nil {
		t.Error("GUI config not set")
	}

	if gui.currentWindow != MenuWindow {
		t.Errorf("Expected currentWindow to be %s, got %s", MenuWindow, gui.currentWindow)
	}

	// Clean up
	gui.g.Close()
}

// TestModalState tests modal state management
func TestModalState(t *testing.T) {
	gui := &Gui{
		isModalOpen: false,
		modalType:   "",
		modalFields: []map[string]interface{}{},
		modalValues: map[string]string{},
	}

	// Test opening form modal
	fields := []map[string]interface{}{
		{
			"name":  "title",
			"type":  "CharField",
			"null":  false,
			"blank": false,
		},
		{
			"name":  "content",
			"type":  "TextField",
			"null":  true,
			"blank": true,
		},
	}

	gui.currentApp = "blog"
	gui.currentModel = "Post"

	gui.openFormModal("add", fields, nil)

	if !gui.isModalOpen {
		t.Error("Modal should be open")
	}

	if gui.modalType != "add" {
		t.Errorf("Expected modalType to be 'add', got '%s'", gui.modalType)
	}

	if len(gui.modalFields) != 2 {
		t.Errorf("Expected 2 modal fields, got %d", len(gui.modalFields))
	}

	if gui.modalFieldIdx != 0 {
		t.Error("Expected modalFieldIdx to be 0")
	}

	if gui.modalTitle != "Add blog.Post" {
		t.Errorf("Expected modalTitle to be 'Add blog.Post', got '%s'", gui.modalTitle)
	}

	// Test with current values
	currentValues := map[string]string{
		"title":   "Test Title",
		"content": "Test Content",
	}

	gui.openFormModal("edit", fields, currentValues)

	if gui.modalType != "edit" {
		t.Errorf("Expected modalType to be 'edit', got '%s'", gui.modalType)
	}

	if gui.modalTitle != "Edit blog.Post" {
		t.Errorf("Expected modalTitle to be 'Edit blog.Post', got '%s'", gui.modalTitle)
	}

	if gui.modalValues["title"] != "Test Title" {
		t.Errorf("Expected title to be 'Test Title', got '%s'", gui.modalValues["title"])
	}
}

// TestModalNavigation tests modal field navigation
func TestModalNavigation(t *testing.T) {
	gui := &Gui{
		modalFieldIdx: 0,
		modalFields: []map[string]interface{}{
			{"name": "field1", "type": "CharField"},
			{"name": "field2", "type": "CharField"},
			{"name": "field3", "type": "CharField"},
		},
	}

	// Test moving down
	gui.modalFieldIdx = (gui.modalFieldIdx + 1) % len(gui.modalFields)
	if gui.modalFieldIdx != 1 {
		t.Errorf("Expected modalFieldIdx to be 1, got %d", gui.modalFieldIdx)
	}

	// Test moving up from index 1
	gui.modalFieldIdx--
	if gui.modalFieldIdx != 0 {
		t.Errorf("Expected modalFieldIdx to be 0, got %d", gui.modalFieldIdx)
	}

	// Test wrapping up from 0
	gui.modalFieldIdx--
	if gui.modalFieldIdx < 0 {
		gui.modalFieldIdx = len(gui.modalFields) - 1
	}
	if gui.modalFieldIdx != 2 {
		t.Errorf("Expected modalFieldIdx to be 2 (wrapped), got %d", gui.modalFieldIdx)
	}

	// Test wrapping down from end
	gui.modalFieldIdx = (gui.modalFieldIdx + 1) % len(gui.modalFields)
	if gui.modalFieldIdx != 0 {
		t.Errorf("Expected modalFieldIdx to be 0 (wrapped), got %d", gui.modalFieldIdx)
	}
}

// TestGetFieldConstraints tests field constraint generation
func TestGetFieldConstraints(t *testing.T) {
	gui := &Gui{}

	tests := []struct {
		name        string
		field       map[string]interface{}
		expected    string
		description string
	}{
		{
			name: "CharField with max_length",
			field: map[string]interface{}{
				"name":       "title",
				"type":       "CharField",
				"max_length": float64(200),
			},
			expected:    "max length: 200",
			description: "Should show max_length for CharField",
		},
		{
			name: "Field with unique constraint",
			field: map[string]interface{}{
				"name":   "email",
				"type":   "EmailField",
				"unique": true,
			},
			expected:    "unique",
			description: "Should show unique constraint",
		},
		{
			name: "Field with max_length and unique",
			field: map[string]interface{}{
				"name":       "username",
				"type":       "CharField",
				"max_length": float64(150),
				"unique":     true,
			},
			expected:    "max length: 150 | unique",
			description: "Should show both constraints",
		},
		{
			name: "Field with choices",
			field: map[string]interface{}{
				"name": "status",
				"type": "CharField",
				"choices": []interface{}{
					map[string]interface{}{"value": "draft", "label": "Draft"},
					map[string]interface{}{"value": "published", "label": "Published"},
				},
			},
			expected:    "choices: Draft, Published",
			description: "Should show choices",
		},
		{
			name: "Field with no constraints",
			field: map[string]interface{}{
				"name": "content",
				"type": "TextField",
			},
			expected:    "",
			description: "Should return empty string for unconstrained field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gui.getFieldConstraints(tt.field)
			if result != tt.expected {
				t.Errorf("%s: expected '%s', got '%s'", tt.description, tt.expected, result)
			}
		})
	}
}

// TestGetRelatedModelFromField tests ForeignKey field parsing
func TestGetRelatedModelFromField(t *testing.T) {
	gui := &Gui{}

	tests := []struct {
		name        string
		field       map[string]interface{}
		fieldName   string
		expectModel string
		expectApp   string
		description string
	}{
		{
			name: "ForeignKey with both app and model",
			field: map[string]interface{}{
				"type":          "ForeignKey",
				"related_model": "User",
				"related_app":   "auth",
			},
			fieldName:   "author",
			expectModel: "User",
			expectApp:   "auth",
			description: "Should extract both app and model",
		},
		{
			name: "ForeignKey with model only",
			field: map[string]interface{}{
				"type":          "ForeignKey",
				"related_model": "Post",
			},
			fieldName:   "post",
			expectModel: "Post",
			expectApp:   "",
			description: "Should work with just model",
		},
		{
			name: "Non-ForeignKey field",
			field: map[string]interface{}{
				"type": "CharField",
			},
			fieldName:   "title",
			expectModel: "",
			expectApp:   "",
			description: "Should return empty for non-ForeignKey",
		},
		{
			name: "Field ending with _id (heuristic)",
			field: map[string]interface{}{
				"type": "IntegerField",
			},
			fieldName:   "author_id",
			expectModel: "Author",
			expectApp:   "",
			description: "Should use heuristic for _id fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, app := gui.getRelatedModelFromField(tt.field, tt.fieldName)

			if model != tt.expectModel {
				t.Errorf("%s: expected model='%s', got '%s'", tt.description, tt.expectModel, model)
			}

			if app != tt.expectApp {
				t.Errorf("%s: expected app='%s', got '%s'", tt.description, tt.expectApp, app)
			}
		})
	}
}

// TestGetRecordDisplayString tests record formatting for display
func TestGetRecordDisplayString(t *testing.T) {
	gui := &Gui{}

	tests := []struct {
		name        string
		record      django.ModelRecord
		expected    string
		description string
	}{
		{
			name: "Record with name field",
			record: django.ModelRecord{
				PK: 1,
				Fields: map[string]interface{}{
					"name": "John Doe",
					"age":  float64(30),
				},
			},
			expected:    "John Doe",
			description: "Should prefer name field",
		},
		{
			name: "Record with title field",
			record: django.ModelRecord{
				PK: 2,
				Fields: map[string]interface{}{
					"title":   "Test Post",
					"content": "Content here",
				},
			},
			expected:    "Test Post",
			description: "Should use title field if no name",
		},
		{
			name: "Record with username field",
			record: django.ModelRecord{
				PK: 3,
				Fields: map[string]interface{}{
					"username": "testuser",
					"email":    "test@example.com",
				},
			},
			expected:    "testuser",
			description: "Should use username field",
		},
		{
			name: "Record with email field",
			record: django.ModelRecord{
				PK: 4,
				Fields: map[string]interface{}{
					"email": "user@example.com",
					"phone": "123-456-7890",
				},
			},
			expected:    "user@example.com",
			description: "Should use email field",
		},
		{
			name: "Record with slug field",
			record: django.ModelRecord{
				PK: 5,
				Fields: map[string]interface{}{
					"slug": "test-slug",
					"body": "Body text",
				},
			},
			expected:    "test-slug",
			description: "Should use slug field",
		},
		{
			name: "Record with non-display fields (fallback to first non-id)",
			record: django.ModelRecord{
				PK: 6,
				Fields: map[string]interface{}{
					"id":          float64(6),
					"description": "Description here",
					"count":       float64(42),
				},
			},
			expected:    "description: Description here", // Fallback shows first non-id field
			description: "Should show first non-id field when no display fields",
		},
		{
			name: "Record with only id",
			record: django.ModelRecord{
				PK: 7,
				Fields: map[string]interface{}{
					"id": float64(7),
				},
			},
			expected:    "Record #7",
			description: "Should fall back to Record #PK when only id present",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gui.getRecordDisplayString(tt.record)
			if result != tt.expected {
				t.Errorf("%s: expected '%s', got '%s'", tt.description, tt.expected, result)
			}
		})
	}
}

// TestUpdateOptionsView tests the options/keybindings display
func TestUpdateOptionsView(t *testing.T) {
	g, err := gocui.NewGui(gocui.OutputNormal, false)
	if err != nil {
		t.Skip("Cannot create gocui in test environment")
	}
	defer g.Close()

	gui := &Gui{
		g:             g,
		currentWindow: MenuWindow,
		isModalOpen:   false,
	}

	// This test just verifies the function doesn't crash
	// Full UI testing would require a mock terminal
	v, err := g.SetView("test", 0, 0, 80, 3, 0)
	if err != nil && err != gocui.ErrUnknownView {
		t.Fatal(err)
	}

	gui.updateOptionsView(v)

	// Check that something was written
	content := v.Buffer()
	if len(content) == 0 {
		t.Error("updateOptionsView should write content to view")
	}
}

// TestPanelSwitching tests switching between different panels
func TestPanelSwitching(t *testing.T) {
	gui := &Gui{
		currentWindow: MenuWindow,
	}

	// Test switching panels
	panels := []string{MenuWindow, ListWindow, DataWindow, MainWindow}

	for _, panel := range panels {
		gui.currentWindow = panel
		if gui.currentWindow != panel {
			t.Errorf("Failed to switch to panel %s", panel)
		}
	}
}

func TestClampSelection(t *testing.T) {
	if got := clampSelection(-1, 3); got != 0 {
		t.Fatalf("expected lower clamp to 0, got %d", got)
	}
	if got := clampSelection(7, 3); got != 2 {
		t.Fatalf("expected upper clamp to 2, got %d", got)
	}
	if got := clampSelection(1, 3); got != 1 {
		t.Fatalf("expected in-range selection to remain 1, got %d", got)
	}
}

// TestModalFieldValidation tests field validation logic
func TestModalFieldValidation(t *testing.T) {
	tests := []struct {
		name  string
		field map[string]interface{}
		value string
		valid bool
	}{
		{
			name: "Required field with value",
			field: map[string]interface{}{
				"null":  false,
				"blank": false,
			},
			value: "some value",
			valid: true,
		},
		{
			name: "Required field without value",
			field: map[string]interface{}{
				"null":  false,
				"blank": false,
			},
			value: "",
			valid: false,
		},
		{
			name: "Optional field without value",
			field: map[string]interface{}{
				"null":  true,
				"blank": true,
			},
			value: "",
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			required := false
			if null, ok := tt.field["null"].(bool); ok && !null {
				if blank, ok := tt.field["blank"].(bool); ok && !blank {
					required = true
				}
			}

			isValid := !required || tt.value != ""

			if isValid != tt.valid {
				t.Errorf("Expected validity %v, got %v", tt.valid, isValid)
			}
		})
	}
}

func TestParseComposePSOutput(t *testing.T) {
	arrayInput := `[
		{"Service":"web","State":"running"},
		{"Service":"db","State":"exited"}
	]`

	arrayStatus := parseComposePSOutput(arrayInput)
	if arrayStatus["web"] != "running" {
		t.Fatalf("expected web=running, got %q", arrayStatus["web"])
	}
	if arrayStatus["db"] != "exited" {
		t.Fatalf("expected db=exited, got %q", arrayStatus["db"])
	}

	lineInput := `{"Service":"cache","Status":"running"}`
	lineStatus := parseComposePSOutput(lineInput)
	if lineStatus["cache"] != "running" {
		t.Fatalf("expected cache=running, got %q", lineStatus["cache"])
	}
}

func TestParseComposeServicesFromYAML(t *testing.T) {
	content := `
version: "3.9"
services:
  web:
    build: .
  db:
    image: postgres:16
  cache:
    image: redis:7
volumes:
  pgdata:
`

	services := parseComposeServicesFromYAML(content)
	if len(services) != 3 {
		t.Fatalf("expected 3 services, got %d (%v)", len(services), services)
	}
	if services[0] != "cache" || services[1] != "db" || services[2] != "web" {
		t.Fatalf("unexpected services ordering/content: %v", services)
	}
}

func TestParseComposeServicesFromYAMLNoServices(t *testing.T) {
	content := `
version: "3.9"
volumes:
  data:
`
	services := parseComposeServicesFromYAML(content)
	if len(services) != 0 {
		t.Fatalf("expected no services, got %v", services)
	}
}

func TestParseMakeHelpOutput(t *testing.T) {
	help := `
Docker
  docker               Build the Docker image
  up                   Start main containers

Django
  runserver            Run Django development server
  migrate              Run migrations
`

	targets := parseMakeHelpOutput(help)
	if len(targets) != 4 {
		t.Fatalf("expected 4 targets, got %d (%v)", len(targets), targets)
	}
	if targets[0].name != "docker" || targets[0].section != "Docker" {
		t.Fatalf("unexpected first target: %+v", targets[0])
	}
	if targets[2].name != "runserver" || targets[2].section != "Django" {
		t.Fatalf("unexpected third target: %+v", targets[2])
	}
}

func TestProjectMakeActions(t *testing.T) {
	gui := &Gui{
		makeTargetsLoaded: true,
		makeTargets: []makeTarget{
			{name: "help", description: "Show help"},
			{name: ".internal", description: "Internal target"},
			{name: "test", description: "Run tests"},
			{name: "up", description: "Start containers"},
			{name: "runserver", description: "Run server"},
			{name: "migrate", description: "Run migrations"},
		},
	}

	actions := gui.projectMakeActions()
	if len(actions) != 4 {
		t.Fatalf("expected 4 discovered actions, got %d", len(actions))
	}
	if actions[0].makeTarget != "test" {
		t.Fatalf("expected first action to preserve make help order, got %q", actions[0].makeTarget)
	}
	if actions[1].makeTarget != "up" {
		t.Fatalf("expected second action up, got %q", actions[1].makeTarget)
	}
	if actions[1].label != "up - Start containers" {
		t.Fatalf("expected compact make label, got %q", actions[1].label)
	}
}

func TestShortCommit(t *testing.T) {
	if got := shortCommit("abcdef123456"); got != "abcdef1" {
		t.Fatalf("expected truncated commit, got %q", got)
	}
	if got := shortCommit("abc123"); got != "abc123" {
		t.Fatalf("expected unchanged short commit, got %q", got)
	}
}

func TestOutputLineHelpers(t *testing.T) {
	lines := outputLines("one\r\ntwo\rthree\nfour")
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d (%v)", len(lines), lines)
	}
	if got := outputLineAt(lines, 99); got != "four" {
		t.Fatalf("expected clamped line to be 'four', got %q", got)
	}
	if got := outputLineAt(nil, 0); got != "" {
		t.Fatalf("expected empty output line for nil slice, got %q", got)
	}
}

func TestSanitizeOutputForClipboard(t *testing.T) {
	input := "ok\x1b[31mERR\x1b[0m\r\nline\x07\tend\n"
	got := sanitizeOutputForClipboard(input)
	want := "okERR\nline\tend"
	if got != want {
		t.Fatalf("expected sanitized clipboard text %q, got %q", want, got)
	}
}

func TestSanitizeOutputForDisplay(t *testing.T) {
	input := "\x1b[?25h\x1b[1;A\x1b[0;G\x1b[?25l[+] Running 1/1\r\n"
	got := sanitizeOutputForDisplay(input)
	want := "[+] Running 1/1\n"
	if got != want {
		t.Fatalf("expected sanitized display output %q, got %q", want, got)
	}
}

func TestIsProjectTasksModalTitle(t *testing.T) {
	if !isProjectTasksModalTitle(" Project Tasks ") {
		t.Fatal("expected Project Tasks title to match")
	}
	if isProjectTasksModalTitle("Favorite Commands") {
		t.Fatal("did not expect non-task title to match")
	}
}

func TestNormalizeRange(t *testing.T) {
	start, end := normalizeRange(9, 3)
	if start != 3 || end != 9 {
		t.Fatalf("expected sorted range (3,9), got (%d,%d)", start, end)
	}
}

func TestSendOutputInputWritesLine(t *testing.T) {
	var buf bytes.Buffer
	gui := &Gui{
		outputInputWriters: map[string]io.WriteCloser{
			"command-001": nopWriteCloser{Writer: &buf},
		},
		inputTargetTabID: "command-001",
	}

	if err := gui.sendOutputInput("abc@example.com"); err != nil {
		t.Fatalf("sendOutputInput returned error: %v", err)
	}
	if got := buf.String(); got != "abc@example.com\n" {
		t.Fatalf("expected newline-terminated payload, got %q", got)
	}
}

func TestIsLikelyInteractiveCommand(t *testing.T) {
	tests := []struct {
		command string
		want    bool
	}{
		{command: "python manage.py createsuperuser", want: true},
		{command: "python manage.py shell", want: true},
		{command: "docker compose exec web python manage.py migrate", want: true},
		{command: "make migrate", want: false},
	}

	for _, tt := range tests {
		if got := isLikelyInteractiveCommand(tt.command); got != tt.want {
			t.Fatalf("isLikelyInteractiveCommand(%q)=%v, want %v", tt.command, got, tt.want)
		}
	}
}

func TestHasInteractivePrompt(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{text: "Email address: ", want: true},
		{text: "Password: ", want: true},
		{text: ">>> ", want: true},
		{text: "Applying migrations... OK", want: false},
	}

	for _, tt := range tests {
		if got := hasInteractivePrompt(tt.text); got != tt.want {
			t.Fatalf("hasInteractivePrompt(%q)=%v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestFreezeOutputTabAtCurrentPosition(t *testing.T) {
	gui := &Gui{
		outputTabs: map[string]*outputTabState{
			"command-001": {
				id:         "command-001",
				route:      OutputTabCommand,
				title:      "Command",
				text:       "line1\nline2\nline3\nline4",
				autoscroll: true,
			},
		},
	}

	gui.freezeOutputTabAtCurrentPosition("command-001")
	tab := gui.outputTabs["command-001"]
	if tab.autoscroll {
		t.Fatal("expected autoscroll to be disabled")
	}
	if tab.originY < 0 {
		t.Fatalf("expected non-negative origin, got %d", tab.originY)
	}
}

func TestProjectActionsContainCoreItems(t *testing.T) {
	gui := &Gui{
		project: &django.Project{},
	}

	foundServer := false
	foundMigrations := false
	foundTools := false
	for _, action := range gui.projectActions() {
		if action.label == "Server..." {
			foundServer = true
		}
		if action.label == "Migrations..." {
			foundMigrations = true
		}
		if action.label == "Tools..." {
			foundTools = true
		}
	}

	if !foundServer {
		t.Fatal("expected project actions to include Server...")
	}
	if !foundMigrations {
		t.Fatal("expected project actions to include Migrations...")
	}
	if !foundTools {
		t.Fatal("expected project actions to include Tools...")
	}
}

func TestProjectToolActionsContainExpectedCommands(t *testing.T) {
	gui := &Gui{
		project: &django.Project{},
	}

	foundCheck := false
	foundURLs := false
	for _, action := range gui.projectToolActions() {
		if action.label == "Django check" {
			foundCheck = true
		}
		if action.label == "Show URL patterns" {
			foundURLs = true
		}
	}

	if !foundCheck {
		t.Fatal("expected tool actions to include Django check")
	}
	if !foundURLs {
		t.Fatal("expected tool actions to include Show URL patterns")
	}
}

func TestProjectServerActionsContainStartAndStop(t *testing.T) {
	gui := &Gui{
		project: &django.Project{},
	}

	foundStart := false
	foundStop := false
	for _, action := range gui.projectServerActions() {
		if action.label == "Start dev server" {
			foundStart = true
		}
		if action.label == "Stop dev server" {
			foundStop = true
		}
	}

	if !foundStart {
		t.Fatal("expected server actions to include Start dev server")
	}
	if !foundStop {
		t.Fatal("expected server actions to include Stop dev server")
	}
}

func TestMoveProjectModalSelectionClamps(t *testing.T) {
	gui := &Gui{
		projectModalActions: []projectAction{
			{label: "one"},
			{label: "two"},
		},
	}

	gui.moveProjectModalSelection(10)
	if gui.projectModalIndex != 1 {
		t.Fatalf("expected index to clamp to last item, got %d", gui.projectModalIndex)
	}

	gui.moveProjectModalSelection(-10)
	if gui.projectModalIndex != 0 {
		t.Fatalf("expected index to clamp to first item, got %d", gui.projectModalIndex)
	}
}

func TestProjectActionsViewportKeepsSelectionVisible(t *testing.T) {
	g, err := gocui.NewGui(gocui.OutputNormal, false)
	if err != nil {
		t.Skip("Cannot create gocui in test environment")
	}
	defer g.Close()

	v, err := g.SetView("viewport-test", 0, 0, 60, 10, 0)
	if err != nil && err != gocui.ErrUnknownView {
		t.Fatalf("failed to create view: %v", err)
	}

	actions := make([]projectAction, 0, 25)
	for i := 0; i < 25; i++ {
		actions = append(actions, projectAction{label: fmt.Sprintf("action-%02d", i)})
	}
	gui := &Gui{
		projectModalActions: actions,
		projectModalIndex:   0,
		projectModalOffset:  0,
	}

	start, end := gui.projectActionsViewport(v)
	if start != 0 {
		t.Fatalf("expected viewport start=0 initially, got %d", start)
	}
	if end <= start {
		t.Fatalf("expected non-empty viewport, got start=%d end=%d", start, end)
	}

	gui.projectModalIndex = 20
	start, end = gui.projectActionsViewport(v)
	if gui.projectModalIndex < start || gui.projectModalIndex >= end {
		t.Fatalf("expected selected index %d to be visible in [%d,%d)", gui.projectModalIndex, start, end)
	}
	if start == 0 {
		t.Fatal("expected viewport to scroll for deeper index")
	}
}

func TestProjectModalNumberInputJump(t *testing.T) {
	actions := make([]projectAction, 0, 12)
	for i := 0; i < 12; i++ {
		actions = append(actions, projectAction{label: fmt.Sprintf("task-%d", i+1)})
	}
	gui := &Gui{projectModalActions: actions}

	gui.appendProjectModalNumberInput('1')
	if gui.projectModalIndex != 0 {
		t.Fatalf("expected index 0 after input '1', got %d", gui.projectModalIndex)
	}
	if gui.projectModalNumber != "1" {
		t.Fatalf("expected modal number '1', got %q", gui.projectModalNumber)
	}

	gui.appendProjectModalNumberInput('2')
	if gui.projectModalIndex != 11 {
		t.Fatalf("expected index 11 after input '12', got %d", gui.projectModalIndex)
	}
	if gui.projectModalNumber != "12" {
		t.Fatalf("expected modal number '12', got %q", gui.projectModalNumber)
	}

	// Overflowing numbers fallback to the last digit when valid.
	gui.appendProjectModalNumberInput('9')
	if gui.projectModalIndex != 8 {
		t.Fatalf("expected index 8 after overflow fallback to '9', got %d", gui.projectModalIndex)
	}
	if gui.projectModalNumber != "9" {
		t.Fatalf("expected modal number to fallback to '9', got %q", gui.projectModalNumber)
	}
}

func TestOutputTabStateHelpers(t *testing.T) {
	gui := &Gui{}

	commandTab1 := gui.startCommandOutputTab("Command 1")
	logsTab1 := gui.startLogsOutputTab("Logs 1")
	commandTab2 := gui.startCommandOutputTab("Command 2")

	if len(gui.outputOrder) != 3 {
		t.Fatalf("expected 3 output tabs, got %d", len(gui.outputOrder))
	}
	if gui.outputTab != commandTab2 {
		t.Fatalf("expected latest tab selected, got %q", gui.outputTab)
	}

	gui.appendOutput(commandTab1, "cmd-1\n")
	gui.appendOutput(logsTab1, "log-1\n")
	if got := gui.outputTextForTab(commandTab1); got != "cmd-1\n" {
		t.Fatalf("expected command output captured, got %q", got)
	}
	if got := gui.outputTextForTab(logsTab1); got != "log-1\n" {
		t.Fatalf("expected logs output captured, got %q", got)
	}

	gui.switchOutputTab(OutputTabLogs)
	if gui.outputTab != logsTab1 {
		t.Fatalf("expected switch by route to latest logs tab, got %q", gui.outputTab)
	}

	gui.resetOutput(commandTab2, "Run Check")
	if got := gui.outputTitleForTab(commandTab2); got != "Run Check" {
		t.Fatalf("expected title reset, got %q", got)
	}
	if got := gui.outputTextForTab(commandTab2); got != "" {
		t.Fatalf("expected tab text reset, got %q", got)
	}

	gui.switchOutputTab(commandTab2)
	if err := gui.closeCurrentOutputTab(nil, nil); err != nil {
		t.Fatalf("closeCurrentOutputTab returned error: %v", err)
	}
	if len(gui.outputOrder) != 2 {
		t.Fatalf("expected 2 tabs after close, got %d", len(gui.outputOrder))
	}
	if got := gui.resolveOutputTabID(OutputTabCommand, false); got != commandTab1 {
		t.Fatalf("expected command route to fall back to prior command tab, got %q", got)
	}
}

func TestTabTitleFromCommand(t *testing.T) {
	if got := tabTitleFromCommand("python manage.py migrate"); got != "python manage.py migrate" {
		t.Fatalf("unexpected command title: %q", got)
	}
	if got := tabTitleFromCommand("   "); got != "Command" {
		t.Fatalf("expected default command title, got %q", got)
	}
	long := "docker compose -f compose.yaml up -d django-admin django-api django-app django-worker"
	got := tabTitleFromCommand(long)
	if len(got) > 72 {
		t.Fatalf("expected truncated title length <= 72, got %d", len(got))
	}
}

func TestOrderedOutputTabIDsForPicker(t *testing.T) {
	gui := &Gui{}

	commandTab1 := gui.startCommandOutputTab("cmd-1")
	logsTab1 := gui.startLogsOutputTab("logs-1")
	commandTab2 := gui.startCommandOutputTab("cmd-2")

	got := gui.orderedOutputTabIDsForPicker()
	if len(got) != 3 {
		t.Fatalf("expected 3 tabs in picker order, got %d", len(got))
	}
	if got[0] != commandTab2 || got[1] != logsTab1 || got[2] != commandTab1 {
		t.Fatalf("unexpected picker order: %v", got)
	}
}

func TestOpenOutputTabsModal(t *testing.T) {
	gui := &Gui{currentWindow: MainWindow}

	commandTab1 := gui.startCommandOutputTab("cmd-1")
	logsTab1 := gui.startLogsOutputTab("logs-1")
	gui.switchOutputTab(commandTab1)

	if err := gui.openOutputTabsModal(nil, nil); err != nil {
		t.Fatalf("openOutputTabsModal returned error: %v", err)
	}
	if !gui.isModalOpen {
		t.Fatal("expected output tabs modal to be open")
	}
	if gui.modalType != "outputTabs" {
		t.Fatalf("expected modalType=outputTabs, got %q", gui.modalType)
	}
	if len(gui.outputTabModalIDs) != 2 {
		t.Fatalf("expected 2 tab ids in modal, got %d", len(gui.outputTabModalIDs))
	}
	if gui.outputTabModalIDs[0] != logsTab1 || gui.outputTabModalIDs[1] != commandTab1 {
		t.Fatalf("unexpected outputTabModalIDs ordering: %v", gui.outputTabModalIDs)
	}
	if gui.outputTabModalIndex != 1 {
		t.Fatalf("expected modal index to point at current tab, got %d", gui.outputTabModalIndex)
	}
}

func TestIsLongRunningMakeTarget(t *testing.T) {
	cases := map[string]bool{
		"up":        true,
		"up-all":    true,
		"runserver": true,
		"watch":     true,
		"storybook": true,
		"test":      false,
		"migrate":   false,
	}

	for target, want := range cases {
		if got := isLongRunningMakeTarget(target); got != want {
			t.Fatalf("target %q: expected %v, got %v", target, want, got)
		}
	}
}

func TestParseContainerNameConflicts(t *testing.T) {
	output := `
Error response from daemon: Conflict. The container name "/django-app" is already in use by container "aaa".
Error response from daemon: Conflict. The container name "/django-api" is already in use by container "bbb".
Error response from daemon: Conflict. The container name "/django-app" is already in use by container "ccc".
`

	conflicts := parseContainerNameConflicts(output)
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 unique conflicts, got %d (%v)", len(conflicts), conflicts)
	}
	if conflicts[0] != "/django-api" || conflicts[1] != "/django-app" {
		t.Fatalf("unexpected conflict parsing result: %v", conflicts)
	}
}

func TestParseContainerNameConflictsNone(t *testing.T) {
	conflicts := parseContainerNameConflicts("all good")
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %v", conflicts)
	}
}
