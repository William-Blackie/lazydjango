package gui

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/awesome-gocui/gocui"
	"github.com/williamblackie/lazydjango/pkg/config"
	"github.com/williamblackie/lazydjango/pkg/django"
	updatepkg "github.com/williamblackie/lazydjango/pkg/update"
)

// Gui represents the main TUI state.
type Gui struct {
	g       *gocui.Gui
	config  *config.AppConfig
	project *django.Project
	// Project-specific persisted state.
	stateStore      *projectStateStore
	historyStore    *projectHistoryStore
	historyStoreErr string
	stateDirty      bool

	// Global state
	currentWindow       string
	mainTitle           string
	commandHistory      []string
	favoriteCommands    []string
	recentModels        []persistedRecentModel
	recentErrors        []persistedRecentError
	serverCmd           *exec.Cmd
	appVersion          string
	updateChecked       bool
	updateChecking      bool
	updateAvailable     bool
	updateLatestVersion string
	updateReleaseURL    string
	updateCheckError    string
	updateNoticeShown   bool
	updateCheckStarted  bool
	outputTab           string // currently selected output tab ID
	outputTabs          map[string]*outputTabState
	outputOrder         []string
	outputRoutes        map[string]string // logical route (command/logs) -> tab ID
	outputCounter       int
	outputInputWriters  map[string]io.WriteCloser
	outputSelectMode    bool
	outputSelectTabID   string
	outputSelectAnchor  int
	outputSelectCursor  int

	// Selection state for left panels
	menuSelection int
	listSelection int
	dataSelection int

	// Cached left-panel metadata
	snapshotCache     []*django.Snapshot
	snapshotErr       error
	snapshotLoaded    bool
	containerStatus   map[string]string
	makeTargets       []makeTarget
	makeTargetsErr    error
	makeTargetsLoaded bool
	projectTasks      []projectTaskEntry
	projectTasksErr   error
	projectTasksPath  string
	projectTasksReady bool

	// Data viewer state
	currentApp        string
	currentModel      string
	currentRecords    []django.ModelRecord
	currentPage       int
	selectedRecordIdx int
	totalRecords      int
	pageSize          int

	// Modal state
	isModalOpen         bool
	modalType           string // "add", "edit", "delete", "restore", "containers", "projectActions", "outputTabs"
	modalReturnWindow   string
	modalFields         []map[string]interface{}
	modalFieldIdx       int
	modalValues         map[string]string
	modalMessage        string
	modalTitle          string
	restoreSnapshots    []*django.Snapshot
	restoreIndex        int
	containerAction     string // "start" or "stop"
	containerList       []string
	containerIndex      int
	containerSelect     map[string]bool
	projectModalActions []projectAction
	projectModalIndex   int
	projectModalOffset  int
	projectModalNumber  string
	outputTabModalIDs   []string
	outputTabModalIndex int

	// Command/search input bar state
	inputMode         string // "", "command", "search"
	inputReturnWindow string
	inputTargetTabID  string
}

// Window names.
const (
	MenuWindow       = "menu"
	ListWindow       = "list"
	DataWindow       = "data"
	MainWindow       = "main"
	OptionsWindow    = "options"
	ModalWindow      = "modal"
	ModalInputWindow = "modalInput"
)

const (
	OutputTabCommand = "command"
	OutputTabLogs    = "logs"
	outputOriginTail = -1
)

const (
	panelFrameColorInactive = gocui.ColorWhite
	panelFrameColorActive   = gocui.ColorGreen
	panelTitleColorInactive = gocui.ColorWhite
	panelTitleColorActive   = gocui.ColorGreen | gocui.AttrBold
)

var panelOrder = []string{MenuWindow, ListWindow, DataWindow, MainWindow}

type projectAction struct {
	label        string
	command      string
	internal     string
	makeTarget   string
	shellCommand string
}

type makeTarget struct {
	name        string
	description string
	section     string
}

type outputTabState struct {
	id         string
	route      string
	title      string
	text       string
	autoscroll bool
	originX    int
	originY    int
}

func clampSelection(selection, count int) int {
	if count <= 0 {
		return 0
	}
	if selection < 0 {
		return 0
	}
	if selection >= count {
		return count - 1
	}
	return selection
}

func (gui *Gui) panelTitle(windowName, label string) string {
	if gui.currentWindow == windowName && !gui.isModalOpen {
		return fmt.Sprintf(" > %s ", label)
	}
	return fmt.Sprintf("   %s ", label)
}

func (gui *Gui) stylePanelView(v *gocui.View, windowName string) {
	if v == nil {
		return
	}
	isActive := gui.currentWindow == windowName && !gui.isModalOpen && gui.inputMode == ""
	if isActive {
		v.FrameColor = panelFrameColorActive
		v.TitleColor = panelTitleColorActive
		return
	}
	v.FrameColor = panelFrameColorInactive
	v.TitleColor = panelTitleColorInactive
}

func (gui *Gui) projectActions() []projectAction {
	actions := []projectAction{
		{label: "Server...", internal: "openserver"},
	}

	if gui.project != nil && gui.project.HasDocker && gui.project.DockerComposeFile != "" {
		actions = append(actions, projectAction{label: "Containers...", internal: "opencontainers"})
	}

	if len(gui.projectTaskActions()) > 0 {
		actions = append(actions, projectAction{label: "Project Tasks...", internal: "openprojecttasks"})
	}
	if len(gui.projectFavoriteActions()) > 0 {
		actions = append(actions, projectAction{label: "Favorites...", internal: "openfavorites"})
	}
	if len(gui.projectRecentCommandActions()) > 0 {
		actions = append(actions, projectAction{label: "Recent Commands...", internal: "openrecentcommands"})
	}

	actions = append(actions,
		projectAction{label: "Migrations...", internal: "openmigrations"},
		projectAction{label: "Tools...", internal: "opentools"},
	)

	return actions
}

func (gui *Gui) projectServerActions() []projectAction {
	return []projectAction{
		{label: "Start dev server", internal: "runserver"},
		{label: "Stop dev server", internal: "stopserver"},
	}
}

func (gui *Gui) projectContainerActions() []projectAction {
	return []projectAction{
		{label: "Start selected services...", internal: "startcontainers"},
		{label: "Stop selected services...", internal: "stopcontainers"},
		{label: "Refresh container status", internal: "refresh"},
	}
}

func (gui *Gui) projectMigrationActions() []projectAction {
	actions := make([]projectAction, 0, 4)
	for _, target := range []string{"showmigrations", "migrations", "migrate"} {
		t, ok := gui.makeTargetByName(target)
		if !ok {
			continue
		}
		actions = append(actions, projectAction{
			label:      makeActionLabel(t),
			makeTarget: t.name,
		})
	}
	if len(actions) > 0 {
		return actions
	}

	applied, total := gui.migrationSummary()
	return []projectAction{
		{label: fmt.Sprintf("Show migrations (%d/%d applied)", applied, total), command: "showmigrations --list"},
		{label: "Make migrations", command: "makemigrations"},
		{label: "Apply migrations", command: "migrate"},
	}
}

func (gui *Gui) projectToolActions() []projectAction {
	return []projectAction{
		projectAction{label: "Django check", command: "check"},
		projectAction{label: "Show URL patterns", internal: "showurls"},
		projectAction{label: "Dependency doctor", internal: "doctor"},
		projectAction{label: "History report", internal: "historyreport"},
		projectAction{label: "Refresh project data", internal: "refresh"},
	}
}

func (gui *Gui) projectFavoriteActions() []projectAction {
	if len(gui.favoriteCommands) == 0 {
		return nil
	}
	actions := make([]projectAction, 0, len(gui.favoriteCommands))
	for _, command := range gui.favoriteCommands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}
		label := command
		if len(label) > 70 {
			label = label[:67] + "..."
		}
		actions = append(actions, projectAction{
			label:        label,
			shellCommand: command,
		})
	}
	return actions
}

func (gui *Gui) projectRecentCommandActions() []projectAction {
	if len(gui.commandHistory) == 0 {
		return nil
	}

	const limit = 20
	actions := make([]projectAction, 0, limit)
	seen := make(map[string]struct{}, limit)
	for i := len(gui.commandHistory) - 1; i >= 0; i-- {
		command := strings.TrimSpace(gui.commandHistory[i])
		if command == "" {
			continue
		}
		if _, exists := seen[command]; exists {
			continue
		}
		seen[command] = struct{}{}

		label := command
		if len(label) > 66 {
			label = label[:63] + "..."
		}
		actions = append(actions, projectAction{
			label:        label,
			shellCommand: command,
		})
		if len(actions) >= limit {
			break
		}
	}

	return actions
}

func isEditableProjectAction(action projectAction) bool {
	switch action.internal {
	case "openprojecttasks", "editprojecttasks":
		return true
	default:
		return false
	}
}

func (gui *Gui) selectedProjectAction() (projectAction, bool) {
	actions := gui.projectActions()
	if len(actions) == 0 {
		return projectAction{}, false
	}
	idx := clampSelection(gui.menuSelection, len(actions))
	return actions[idx], true
}

func (gui *Gui) editSelectedProjectAction() error {
	action, ok := gui.selectedProjectAction()
	if !ok {
		return nil
	}
	if !isEditableProjectAction(action) {
		return nil
	}
	return gui.editProjectTasksFile()
}

func (gui *Gui) sortedModels() []django.Model {
	if gui.project == nil {
		return nil
	}

	models := append([]django.Model(nil), gui.project.Models...)
	sort.Slice(models, func(i, j int) bool {
		if models[i].App == models[j].App {
			return models[i].Name < models[j].Name
		}
		return models[i].App < models[j].App
	})
	return models
}

func (gui *Gui) dataActions() []string {
	return []string{
		"Create snapshot",
		"List snapshots",
		"Restore snapshot",
	}
}

func (gui *Gui) selectionCount(windowName string) int {
	switch windowName {
	case MenuWindow:
		return len(gui.projectActions())
	case ListWindow:
		return len(gui.sortedModels())
	case DataWindow:
		return len(gui.dataActions())
	default:
		return 0
	}
}

func (gui *Gui) selectionFor(windowName string) int {
	switch windowName {
	case MenuWindow:
		return gui.menuSelection
	case ListWindow:
		return gui.listSelection
	case DataWindow:
		return gui.dataSelection
	default:
		return 0
	}
}

func (gui *Gui) setSelectionFor(windowName string, value int) {
	switch windowName {
	case MenuWindow:
		gui.menuSelection = value
	case ListWindow:
		gui.listSelection = value
	case DataWindow:
		gui.dataSelection = value
	}
}

func (gui *Gui) clampSelections() {
	gui.menuSelection = clampSelection(gui.menuSelection, gui.selectionCount(MenuWindow))
	gui.listSelection = clampSelection(gui.listSelection, gui.selectionCount(ListWindow))
	gui.dataSelection = clampSelection(gui.dataSelection, gui.selectionCount(DataWindow))
}

func (gui *Gui) rowCursor(windowName string, row int) string {
	if gui.currentWindow == windowName && !gui.isModalOpen && gui.selectionFor(windowName) == row {
		return "> "
	}
	return "  "
}

func (gui *Gui) moveSelection(delta int) {
	window := gui.currentWindow
	count := gui.selectionCount(window)
	if count == 0 {
		return
	}

	idx := gui.selectionFor(window)
	idx += delta
	if idx < 0 {
		idx = count - 1
	}
	if idx >= count {
		idx = 0
	}

	prev := gui.selectionFor(window)
	gui.setSelectionFor(window, idx)
	if idx != prev {
		gui.markStateDirty()
	}
}

func parseComposePSOutput(output string) map[string]string {
	status := make(map[string]string)
	type containerStatus struct {
		Service string `json:"Service"`
		State   string `json:"State"`
		Status  string `json:"Status"`
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return status
	}

	var containers []containerStatus
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &containers); err != nil {
			return status
		}
	} else {
		for _, line := range strings.Split(trimmed, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var c containerStatus
			if err := json.Unmarshal([]byte(line), &c); err == nil {
				containers = append(containers, c)
			}
		}
	}

	for _, c := range containers {
		if c.Service == "" {
			continue
		}
		state := c.State
		if state == "" {
			state = c.Status
		}
		if state == "" {
			state = "unknown"
		}
		status[c.Service] = strings.ToLower(state)
	}

	return status
}

var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiEscapePattern.ReplaceAllString(s, "")
}

func hasEnvKey(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func prepareCommandForExecution(cmd *exec.Cmd, attachTTY bool) {
	if cmd == nil {
		return
	}

	baseEnv := os.Environ()
	if len(cmd.Env) > 0 {
		baseEnv = append(baseEnv, cmd.Env...)
	}
	if !hasEnvKey(baseEnv, "COMPOSE_INTERACTIVE_NO_CLI") {
		baseEnv = append(baseEnv, "COMPOSE_INTERACTIVE_NO_CLI=1")
	}
	cmd.Env = baseEnv

	if attachTTY && cmd.Stdin == nil {
		cmd.Stdin = os.Stdin
	}
}

func newTTYCompatShellCommand(command string, dir string) *exec.Cmd {
	if scriptPath, err := exec.LookPath("script"); err == nil {
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.Command(scriptPath, "-q", "/dev/null", "sh", "-lc", command)
		} else {
			cmd = exec.Command(scriptPath, "-q", "-e", "-c", command, "/dev/null")
		}
		cmd.Dir = dir
		return cmd
	}

	cmd := exec.Command("sh", "-lc", command)
	cmd.Dir = dir
	return cmd
}

func isProjectTasksModalTitle(title string) bool {
	return strings.EqualFold(strings.TrimSpace(title), "Project Tasks")
}

func (gui *Gui) hasMakefile() bool {
	if gui.project == nil || gui.project.RootDir == "" {
		return false
	}
	_, err := os.Stat(fmt.Sprintf("%s/Makefile", gui.project.RootDir))
	return err == nil
}

func parseMakeHelpOutput(output string) []makeTarget {
	lines := strings.Split(stripANSI(output), "\n")
	targets := make([]makeTarget, 0)
	section := ""
	targetLine := regexp.MustCompile(`^\s*([A-Za-z0-9_.-]+)\s{2,}(.+?)\s*$`)

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			section = trimmed
			continue
		}

		matches := targetLine.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		targets = append(targets, makeTarget{
			name:        strings.TrimSpace(matches[1]),
			description: strings.TrimSpace(matches[2]),
			section:     section,
		})
	}

	return targets
}

func (gui *Gui) loadMakeTargets(force bool) ([]makeTarget, error) {
	if gui.makeTargetsLoaded && !force {
		return gui.makeTargets, gui.makeTargetsErr
	}

	if !gui.hasMakefile() {
		gui.makeTargetsLoaded = true
		gui.makeTargets = nil
		gui.makeTargetsErr = nil
		return nil, nil
	}

	cmd := exec.Command("make", "help")
	cmd.Dir = gui.project.RootDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		gui.makeTargetsLoaded = true
		gui.makeTargets = nil
		gui.makeTargetsErr = fmt.Errorf("make help failed: %w", err)
		return nil, gui.makeTargetsErr
	}

	targets := parseMakeHelpOutput(string(output))
	gui.makeTargetsLoaded = true
	gui.makeTargets = targets
	gui.makeTargetsErr = nil
	return targets, nil
}

func (gui *Gui) makeTargetByName(name string) (makeTarget, bool) {
	targets, err := gui.loadMakeTargets(false)
	if err != nil || len(targets) == 0 {
		return makeTarget{}, false
	}
	for _, target := range targets {
		if target.name == name {
			return target, true
		}
	}
	return makeTarget{}, false
}

func (gui *Gui) makeTargetExists(name string) bool {
	_, ok := gui.makeTargetByName(name)
	return ok
}

func (gui *Gui) projectMakeActions() []projectAction {
	targets, err := gui.loadMakeTargets(false)
	if err != nil || len(targets) == 0 {
		return nil
	}

	actions := make([]projectAction, 0, len(targets))
	for _, t := range targets {
		name := strings.TrimSpace(t.name)
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if lower == "help" || strings.HasPrefix(lower, ".") {
			continue
		}
		actions = append(actions, projectAction{
			label:      makeActionLabel(t),
			makeTarget: name,
		})
	}
	return actions
}

func makeActionLabel(t makeTarget) string {
	label := t.name
	desc := strings.TrimSpace(t.description)
	if desc == "" {
		return label
	}
	if strings.EqualFold(desc, t.name) {
		return label
	}
	return fmt.Sprintf("%s - %s", label, desc)
}

// NewGui creates a new GUI.
func NewGui(project *django.Project) (*Gui, error) {
	if project == nil {
		return nil, fmt.Errorf("project is required")
	}

	g, err := gocui.NewGui(gocui.OutputNormal, true)
	if err != nil {
		return nil, err
	}

	gui := &Gui{
		g:                  g,
		config:             config.GetDefaultConfig(),
		project:            project,
		stateStore:         newProjectStateStore(project.RootDir),
		historyStore:       newProjectHistoryStore(project.RootDir),
		currentWindow:      MenuWindow,
		mainTitle:          "Output",
		appVersion:         "dev",
		commandHistory:     make([]string, 0),
		favoriteCommands:   make([]string, 0),
		recentModels:       make([]persistedRecentModel, 0),
		recentErrors:       make([]persistedRecentError, 0),
		outputTabs:         make(map[string]*outputTabState),
		outputOrder:        make([]string, 0),
		outputRoutes:       make(map[string]string),
		outputInputWriters: make(map[string]io.WriteCloser),
		pageSize:           20,
		currentPage:        1,
	}

	g.Highlight = false
	g.Cursor = false
	g.FgColor = gocui.ColorWhite
	g.SetManagerFunc(gui.layout)

	if err := gui.setKeybindings(); err != nil {
		g.Close()
		return nil, err
	}

	gui.project.DiscoverSettings()
	gui.project.DiscoverModels()
	gui.project.DiscoverMigrations()
	gui.loadMakeTargets(false)
	gui.loadProjectTasks(false)
	gui.refreshContainerStatus()
	if err := gui.loadProjectState(); err != nil {
		log.Printf("warning: failed to load project state: %v", err)
	}
	gui.clampSelections()

	return gui, nil
}

// Run starts the GUI.
func (gui *Gui) Run() error {
	defer gui.g.Close()
	if err := gui.g.MainLoop(); err != nil {
		if errors.Is(err, gocui.ErrQuit) {
			return nil
		}
		return err
	}
	return nil
}

func (gui *Gui) layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if maxX < 40 || maxY < 12 {
		return nil
	}

	leftWidth := maxX * 32 / 100
	if leftWidth < 28 {
		leftWidth = 28
	}
	if leftWidth > maxX-20 {
		leftWidth = maxX - 20
	}

	contentBottom := maxY - 3
	if contentBottom < 6 {
		contentBottom = maxY - 1
	}

	panel1Bottom := contentBottom / 3
	panel2Bottom := (contentBottom * 2) / 3

	menuView, err := g.SetView(MenuWindow, 0, 0, leftWidth, panel1Bottom, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	menuView.Wrap = false
	menuView.Highlight = false
	menuView.Title = gui.panelTitle(MenuWindow, "Project")
	gui.stylePanelView(menuView, MenuWindow)
	gui.renderProjectList(menuView)

	listView, err := g.SetView(ListWindow, 0, panel1Bottom+1, leftWidth, panel2Bottom, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	listView.Wrap = false
	listView.Highlight = false
	listView.Title = gui.panelTitle(ListWindow, "Database")
	gui.stylePanelView(listView, ListWindow)
	gui.renderDatabaseList(listView)

	dataView, err := g.SetView(DataWindow, 0, panel2Bottom+1, leftWidth, contentBottom, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	dataView.Wrap = false
	dataView.Highlight = false
	dataView.Title = gui.panelTitle(DataWindow, "Data")
	gui.stylePanelView(dataView, DataWindow)
	gui.renderDataList(dataView)

	mainView, err := g.SetView(MainWindow, leftWidth+1, 0, maxX-1, contentBottom, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	mainView.Wrap = true
	mainView.Editable = false
	if gui.currentModel == "" {
		gui.renderOutputView(mainView)
	} else {
		mainView.Autoscroll = false
		mainView.Title = gui.panelTitle(MainWindow, gui.mainTitleLabel())
	}
	gui.stylePanelView(mainView, MainWindow)

	optionsView, err := g.SetView(OptionsWindow, 0, maxY-2, maxX-1, maxY-1, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	optionsView.Frame = false
	gui.updateOptionsView(optionsView)

	if !gui.updateCheckStarted {
		gui.startUpdateCheck()
	}

	if gui.isModalOpen {
		modalWidth := 80
		modalHeight := 20
		if modalWidth > maxX-4 {
			modalWidth = maxX - 4
		}
		if modalHeight > maxY-4 {
			modalHeight = maxY - 4
		}
		x0 := (maxX - modalWidth) / 2
		y0 := (maxY - modalHeight) / 2

		modalView, err := g.SetView(ModalWindow, x0, y0, x0+modalWidth, y0+modalHeight, 0)
		if err != nil && err != gocui.ErrUnknownView {
			return err
		}
		modalView.Title = fmt.Sprintf(" %s ", gui.modalTitle)
		modalView.Wrap = true
		modalView.Highlight = false
		modalView.FrameColor = panelFrameColorActive
		modalView.TitleColor = panelTitleColorActive
		gui.renderModal(modalView)
		gui.setModalKeybindings()

		if _, err := g.SetCurrentView(ModalWindow); err != nil {
			return err
		}
		g.SetViewOnTop(ModalWindow)
		return nil
	}

	g.DeleteView(ModalWindow)
	if gui.inputMode != "" {
		return gui.layoutInputPrompt(g, maxX, maxY)
	}
	g.DeleteView(ModalInputWindow)

	if _, err := g.SetCurrentView(gui.currentWindow); err != nil && err != gocui.ErrUnknownView {
		return err
	}

	return nil
}

func (gui *Gui) mainTitleLabel() string {
	if strings.TrimSpace(gui.mainTitle) == "" {
		return "Output"
	}
	return gui.mainTitle
}

func isReleaseVersion(version string) bool {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^\d+\.\d+(\.\d+)?$`, version)
	return matched
}

func (gui *Gui) setAppVersion(version string) {
	version = strings.TrimSpace(version)
	if version == "" {
		version = "dev"
	}
	gui.appVersion = version
}

func (gui *Gui) updateStatusLine() string {
	if !isReleaseVersion(gui.appVersion) {
		return "n/a (dev build)"
	}
	if gui.updateChecking {
		return "checking..."
	}
	if gui.updateAvailable {
		return fmt.Sprintf("%s available (U)", gui.updateLatestVersion)
	}
	if gui.updateChecked {
		return "up to date"
	}
	if gui.updateCheckError != "" {
		return "check failed"
	}
	return "pending"
}

func (gui *Gui) updatePromptBody() string {
	var b strings.Builder
	fmt.Fprintln(&b, "LazyDjango update check")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "Current version: %s\n", gui.appVersion)

	if gui.updateAvailable {
		fmt.Fprintf(&b, "Latest version:  %s\n", gui.updateLatestVersion)
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "Upgrade:")
		fmt.Fprintln(&b, "  brew upgrade William-Blackie/lazydjango/lazy-django")
		if strings.TrimSpace(gui.updateReleaseURL) != "" {
			fmt.Fprintf(&b, "  or download from: %s\n", gui.updateReleaseURL)
		}
		return b.String()
	}

	if gui.updateChecking {
		fmt.Fprintln(&b, "Update check is in progress...")
		return b.String()
	}

	if gui.updateCheckError != "" {
		fmt.Fprintf(&b, "Update check failed: %s\n", gui.updateCheckError)
		if strings.TrimSpace(gui.updateReleaseURL) != "" {
			fmt.Fprintf(&b, "You can still check releases manually: %s\n", gui.updateReleaseURL)
		}
		return b.String()
	}

	if !isReleaseVersion(gui.appVersion) {
		fmt.Fprintln(&b, "Running a development build; automatic release checks are skipped.")
		return b.String()
	}

	fmt.Fprintln(&b, "You are running the latest release.")
	if strings.TrimSpace(gui.updateLatestVersion) != "" {
		fmt.Fprintf(&b, "Latest version: %s\n", gui.updateLatestVersion)
	}
	return b.String()
}

func (gui *Gui) showUpdatePromptInOutput() {
	tabID := gui.startCommandOutputTab("LazyDjango Update")
	gui.appendOutput(tabID, gui.updatePromptBody())
	gui.refreshOutputView()
}

func (gui *Gui) startUpdateCheck() {
	if gui.updateCheckStarted {
		return
	}
	gui.updateCheckStarted = true

	if !isReleaseVersion(gui.appVersion) {
		gui.updateChecked = true
		gui.updateChecking = false
		return
	}

	gui.updateChecking = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		res, err := updatepkg.CheckLatest(ctx, gui.appVersion)
		gui.g.Update(func(g *gocui.Gui) error {
			gui.updateChecking = false
			gui.updateChecked = true

			if err != nil {
				gui.updateCheckError = err.Error()
				gui.updateReleaseURL = "https://github.com/William-Blackie/lazydjango/releases/latest"
			} else {
				gui.updateCheckError = ""
				gui.updateAvailable = res.UpdateAvailable
				gui.updateLatestVersion = res.LatestVersion
				gui.updateReleaseURL = res.LatestReleaseURL
				if gui.updateAvailable && !gui.updateNoticeShown {
					gui.updateNoticeShown = true
					gui.showUpdatePromptInOutput()
				}
			}

			if menuView, viewErr := g.View(MenuWindow); viewErr == nil {
				gui.renderProjectList(menuView)
			}
			if optionsView, viewErr := g.View(OptionsWindow); viewErr == nil {
				gui.updateOptionsView(optionsView)
			}
			return nil
		})
	}()
}

func isOutputRoute(tab string) bool {
	return tab == OutputTabCommand || tab == OutputTabLogs
}

func outputRouteLabel(route string) string {
	if route == OutputTabLogs {
		return "Logs"
	}
	return "Command"
}

func outputDefaultTitle(route string) string {
	if route == OutputTabLogs {
		return "Logs"
	}
	if route == OutputTabCommand {
		return "Command"
	}
	return "Output"
}

func tabTitleFromCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return "Command"
	}
	if len(command) > 72 {
		return command[:69] + "..."
	}
	return command
}

func outputLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.Split(text, "\n")
}

func outputLineAt(lines []string, idx int) string {
	if len(lines) == 0 {
		return ""
	}
	idx = clampSelection(idx, len(lines))
	return lines[idx]
}

func normalizeRange(a, b int) (int, int) {
	if a <= b {
		return a, b
	}
	return b, a
}

func (gui *Gui) ensureOutputState() {
	if gui.outputTabs == nil {
		gui.outputTabs = make(map[string]*outputTabState)
	}
	if gui.outputRoutes == nil {
		gui.outputRoutes = make(map[string]string)
	}
	if gui.outputOrder == nil {
		gui.outputOrder = make([]string, 0)
	}
}

func (gui *Gui) latestOutputTabForRoute(route string) string {
	for i := len(gui.outputOrder) - 1; i >= 0; i-- {
		id := gui.outputOrder[i]
		tab, ok := gui.outputTabs[id]
		if !ok {
			continue
		}
		if tab.route == route {
			return id
		}
	}
	return ""
}

func (gui *Gui) resolveOutputTabID(tab string, createIfMissing bool) string {
	gui.ensureOutputState()

	if tab == "" {
		if gui.outputTab != "" {
			if _, ok := gui.outputTabs[gui.outputTab]; ok {
				return gui.outputTab
			}
			gui.outputTab = ""
		}
		for i := len(gui.outputOrder) - 1; i >= 0; i-- {
			id := gui.outputOrder[i]
			if _, ok := gui.outputTabs[id]; ok {
				return id
			}
		}
		if createIfMissing {
			return gui.startOutputTab(OutputTabCommand, outputDefaultTitle(OutputTabCommand), false)
		}
		return ""
	}

	if _, ok := gui.outputTabs[tab]; ok {
		return tab
	}

	if mapped, ok := gui.outputRoutes[tab]; ok {
		if _, exists := gui.outputTabs[mapped]; exists {
			return mapped
		}
		delete(gui.outputRoutes, tab)
	}

	if !isOutputRoute(tab) {
		return ""
	}

	latest := gui.latestOutputTabForRoute(tab)
	if latest != "" {
		gui.outputRoutes[tab] = latest
		return latest
	}

	if createIfMissing {
		return gui.startOutputTab(tab, outputDefaultTitle(tab), tab == OutputTabLogs)
	}

	return ""
}

func (gui *Gui) outputTabPosition(tabID string) int {
	for idx, id := range gui.outputOrder {
		if id == tabID {
			return idx
		}
	}
	return -1
}

func (gui *Gui) orderedOutputTabIDsForPicker() []string {
	ids := make([]string, 0, len(gui.outputOrder))
	for i := len(gui.outputOrder) - 1; i >= 0; i-- {
		id := gui.outputOrder[i]
		if _, ok := gui.outputTabs[id]; !ok {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

func (gui *Gui) startOutputTab(route, title string, autoscroll bool) string {
	gui.ensureOutputState()
	if !isOutputRoute(route) {
		route = OutputTabCommand
	}
	if strings.TrimSpace(title) == "" {
		title = outputDefaultTitle(route)
	}

	gui.outputCounter++
	id := fmt.Sprintf("%s-%03d", route, gui.outputCounter)
	originY := 0
	if !autoscroll {
		// New command tabs should open at the latest output by default.
		originY = outputOriginTail
	}
	gui.outputTabs[id] = &outputTabState{
		id:         id,
		route:      route,
		title:      title,
		autoscroll: autoscroll,
		originY:    originY,
	}
	gui.outputOrder = append(gui.outputOrder, id)
	gui.outputRoutes[route] = id
	gui.outputTab = id
	gui.markStateDirty()

	if gui.currentModel == "" {
		gui.refreshOutputView()
	}

	return id
}

func (gui *Gui) startCommandOutputTab(title string) string {
	return gui.startOutputTab(OutputTabCommand, title, false)
}

func (gui *Gui) startLogsOutputTab(title string) string {
	return gui.startOutputTab(OutputTabLogs, title, true)
}

func (gui *Gui) outputTitleForTab(tab string) string {
	id := gui.resolveOutputTabID(tab, false)
	if id == "" {
		return outputDefaultTitle(tab)
	}
	title := strings.TrimSpace(gui.outputTabs[id].title)
	if title == "" {
		return outputDefaultTitle(gui.outputTabs[id].route)
	}
	return title
}

func (gui *Gui) outputTextForTab(tab string) string {
	id := gui.resolveOutputTabID(tab, false)
	if id == "" {
		return ""
	}
	return gui.outputTabs[id].text
}

func (gui *Gui) setOutputTextForTab(tab, text string) {
	id := gui.resolveOutputTabID(tab, true)
	if id == "" {
		return
	}
	gui.outputTabs[id].text = text
}

func (gui *Gui) setOutputTitleForTab(tab, title string) {
	id := gui.resolveOutputTabID(tab, true)
	if id == "" {
		return
	}
	if strings.TrimSpace(title) == "" {
		title = outputDefaultTitle(gui.outputTabs[id].route)
	}
	gui.outputTabs[id].title = title
}

func (gui *Gui) appendOutput(tab, text string) {
	id := gui.resolveOutputTabID(tab, true)
	if id == "" {
		return
	}
	gui.outputTabs[id].text += text
}

func (gui *Gui) resetOutput(tab, title string) {
	id := gui.resolveOutputTabID(tab, true)
	if id == "" {
		return
	}
	if strings.TrimSpace(title) == "" {
		title = outputDefaultTitle(gui.outputTabs[id].route)
	}
	gui.outputTabs[id].title = title
	gui.outputTabs[id].text = ""
	if gui.outputTabs[id].autoscroll {
		gui.outputTabs[id].originX = 0
		gui.outputTabs[id].originY = 0
	} else {
		gui.outputTabs[id].originX = 0
		gui.outputTabs[id].originY = outputOriginTail
	}
}

func (gui *Gui) switchOutputTab(tab string) {
	id := gui.resolveOutputTabID(tab, false)
	if id == "" {
		return
	}
	changed := gui.outputTab != id
	gui.outputTab = id
	if changed && gui.outputSelectMode {
		gui.outputSelectMode = false
		gui.outputSelectTabID = ""
	}
	if changed && gui.inputMode == "output-input" {
		gui.inputMode = ""
		gui.inputTargetTabID = ""
		gui.inputReturnWindow = ""
		gui.g.DeleteKeybindings(ModalInputWindow)
		gui.g.DeleteView(ModalInputWindow)
	}
	if changed {
		gui.markStateDirty()
	}
	if gui.currentModel == "" {
		gui.refreshOutputView()
	}
}

func (gui *Gui) currentOutputTabLabel() string {
	id := gui.resolveOutputTabID("", false)
	if id == "" {
		return "Output"
	}
	return outputRouteLabel(gui.outputTabs[id].route)
}

func (gui *Gui) renderOutputView(v *gocui.View) {
	v.Clear()
	v.Highlight = false
	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		v.Autoscroll = false
		gui.mainTitle = "Output"
		v.Title = gui.panelTitle(MainWindow, "Output")
		fmt.Fprintln(v, "LazyDjango")
		fmt.Fprintln(v)
		if gui.project != nil {
			fmt.Fprintf(v, "Project: %s\n", gui.project.RootDir)
			fmt.Fprintf(v, "Apps: %d  Models: %d  Database: %s\n", len(gui.project.InstalledApps), len(gui.project.Models), gui.databaseLabel())
			if gui.project.HasDocker {
				fmt.Fprintln(v, "Docker: configured")
			} else {
				fmt.Fprintln(v, "Docker: not configured")
			}
			if gui.hasMakefile() {
				fmt.Fprintln(v, "Workflow: make + django")
			} else {
				fmt.Fprintln(v, "Workflow: django")
			}
			if tasks, err := gui.loadProjectTasks(false); err == nil {
				fmt.Fprintf(v, "Project tasks: %d\n", len(tasks))
			}
		}
		fmt.Fprintln(v)
		fmt.Fprintln(v, "Quick Start")
		fmt.Fprintln(v, "  1. Project -> Server... -> Start dev server")
		fmt.Fprintln(v, "  2. Project -> Containers... -> Start selected services")
		fmt.Fprintln(v, "  3. Database -> select model -> Enter to browse data")
		fmt.Fprintln(v, "  4. Press : to run ad-hoc commands, / to search current view")
		fmt.Fprintln(v)
		fmt.Fprintln(v, "Output tabs are created automatically when commands run.")
		fmt.Fprintln(v, "Keys: [ previous, ] next, o toggle cmd/log, x close tab, Ctrl+L clear tab, g/G jump, Ctrl+d/u page.")
		return
	}

	tab := gui.outputTabs[tabID]
	if gui.outputSelectMode && gui.outputSelectTabID != tabID {
		gui.outputSelectMode = false
		gui.outputSelectTabID = ""
	}
	useAutoscroll := tab.autoscroll || tab.originY == outputOriginTail
	v.Autoscroll = useAutoscroll
	totalTabs := len(gui.outputOrder)
	tabIndex := gui.outputTabPosition(tabID) + 1
	title := fmt.Sprintf("%s [%s %d/%d]", gui.outputTitleForTab(tabID), outputRouteLabel(tab.route), tabIndex, totalTabs)
	gui.mainTitle = title
	v.Title = gui.panelTitle(MainWindow, title)

	text := gui.outputTextForTab(tabID)
	if strings.TrimSpace(text) == "" {
		gui.outputSelectMode = false
		gui.outputSelectTabID = ""
		fmt.Fprintln(v, "No output in this tab yet.")
		fmt.Fprintln(v)
		fmt.Fprintln(v, "Each command creates a new tab.")
		fmt.Fprintln(v, "Use [ and ] to switch tabs, x to close, Ctrl+L to clear.")
		return
	}
	lines := outputLines(text)
	for _, line := range lines {
		fmt.Fprintln(v, line)
	}
	if !tab.autoscroll {
		v.Autoscroll = false
		if tab.originY != outputOriginTail {
			_ = v.SetOrigin(tab.originX, tab.originY)
		}
	}

	if !gui.outputSelectMode || gui.outputSelectTabID != tabID {
		return
	}
	if len(lines) == 0 {
		return
	}

	start, end := normalizeRange(gui.outputSelectAnchor, gui.outputSelectCursor)
	start = clampSelection(start, len(lines))
	end = clampSelection(end, len(lines))

	v.Highlight = true
	v.SelBgColor = gocui.ColorGreen
	v.SelFgColor = gocui.ColorBlack | gocui.AttrBold
	for i := 0; i < len(lines); i++ {
		_ = v.SetHighlight(i, false)
	}
	for i := start; i <= end; i++ {
		_ = v.SetHighlight(i, true)
	}
}

func (gui *Gui) currentOutputLineIndex(tabID string) int {
	tab, ok := gui.outputTabs[tabID]
	if !ok {
		return 0
	}
	lines := outputLines(gui.outputTextForTab(tabID))
	if len(lines) == 0 {
		return 0
	}
	if tab.originY == outputOriginTail {
		return len(lines) - 1
	}
	return clampSelection(tab.originY, len(lines))
}

func (gui *Gui) ensureOutputSelectionVisible(tabID string) {
	tab, ok := gui.outputTabs[tabID]
	if !ok {
		return
	}

	v, err := gui.g.View(MainWindow)
	if err != nil {
		return
	}

	lines := outputLines(gui.outputTextForTab(tabID))
	if len(lines) == 0 {
		return
	}

	tab.autoscroll = false
	cursor := clampSelection(gui.outputSelectCursor, len(lines))
	originY := tab.originY
	if originY == outputOriginTail || originY < 0 {
		originY = 0
	}

	_, viewHeight := v.Size()
	visible := viewHeight - 2
	if visible < 1 {
		visible = 1
	}

	if cursor < originY {
		originY = cursor
	}
	if cursor >= originY+visible {
		originY = cursor - visible + 1
	}
	if originY < 0 {
		originY = 0
	}

	tab.originX = 0
	tab.originY = originY
}

func copyToClipboard(text string) error {
	writeCommand := func(name string, args ...string) error {
		if _, err := exec.LookPath(name); err != nil {
			return err
		}
		cmd := exec.Command(name, args...)
		cmd.Stdin = bytes.NewBufferString(text)
		return cmd.Run()
	}

	if runtime.GOOS == "darwin" {
		if err := writeCommand("pbcopy"); err == nil {
			return nil
		}
	}
	if err := writeCommand("wl-copy"); err == nil {
		return nil
	}
	if err := writeCommand("xclip", "-selection", "clipboard"); err == nil {
		return nil
	}
	if err := writeCommand("xsel", "--clipboard", "--input"); err == nil {
		return nil
	}

	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	if _, err := fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07", encoded); err != nil {
		return err
	}
	return nil
}

func sanitizeOutputForClipboard(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = stripANSI(text)
	text = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case unicode.IsPrint(r):
			return r
		default:
			return -1
		}
	}, text)
	return strings.TrimRight(text, "\n")
}

func (gui *Gui) toggleOutputSelectionMode(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentWindow != MainWindow || gui.currentModel != "" {
		return nil
	}

	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		return nil
	}
	lines := outputLines(gui.outputTextForTab(tabID))
	if len(lines) == 0 {
		return nil
	}

	if gui.outputSelectMode && gui.outputSelectTabID == tabID {
		gui.outputSelectMode = false
		gui.outputSelectTabID = ""
		gui.refreshOutputView()
		return nil
	}

	gui.outputSelectMode = true
	gui.outputSelectTabID = tabID
	idx := gui.currentOutputLineIndex(tabID)
	gui.outputSelectAnchor = idx
	gui.outputSelectCursor = idx
	gui.ensureOutputSelectionVisible(tabID)
	gui.refreshOutputView()
	return nil
}

func (gui *Gui) moveOutputSelection(delta int) error {
	if !gui.outputSelectMode || gui.currentModel != "" {
		return nil
	}
	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" || gui.outputSelectTabID != tabID {
		return nil
	}

	lines := outputLines(gui.outputTextForTab(tabID))
	if len(lines) == 0 {
		gui.outputSelectMode = false
		gui.outputSelectTabID = ""
		return nil
	}

	next := gui.outputSelectCursor + delta
	next = clampSelection(next, len(lines))
	if next == gui.outputSelectCursor {
		return nil
	}
	gui.outputSelectCursor = next
	gui.ensureOutputSelectionVisible(tabID)
	gui.refreshOutputView()
	return nil
}

func (gui *Gui) copyOutputSelection(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentWindow != MainWindow || gui.currentModel != "" {
		return nil
	}
	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		return nil
	}

	lines := outputLines(gui.outputTextForTab(tabID))
	if len(lines) == 0 {
		return nil
	}

	start := gui.currentOutputLineIndex(tabID)
	end := start
	if gui.outputSelectMode && gui.outputSelectTabID == tabID {
		start, end = normalizeRange(gui.outputSelectAnchor, gui.outputSelectCursor)
	}
	start = clampSelection(start, len(lines))
	end = clampSelection(end, len(lines))
	content := strings.Join(lines[start:end+1], "\n")
	if strings.TrimSpace(content) == "" {
		content = outputLineAt(lines, start)
	}
	content = sanitizeOutputForClipboard(content)
	if strings.TrimSpace(content) == "" {
		return gui.showMessage("Copy", "No copyable text in the selected output lines.")
	}

	if err := copyToClipboard(content); err != nil {
		return gui.showMessage("Copy", fmt.Sprintf("Failed to copy selection: %v", err))
	}
	return nil
}

func (gui *Gui) refreshOutputView() {
	if gui.currentModel != "" || gui.g == nil {
		return
	}
	if v, err := gui.g.View(MainWindow); err == nil {
		gui.renderOutputView(v)
	}
}

func (gui *Gui) setMainTitle(label string) {
	gui.mainTitle = label
	if gui.g == nil {
		return
	}
	if v, err := gui.g.View(MainWindow); err == nil {
		v.Title = gui.panelTitle(MainWindow, gui.mainTitleLabel())
	}
}

func (gui *Gui) renderProjectList(v *gocui.View) {
	v.Clear()
	gui.clampSelections()

	serverStatus := "stopped"
	if gui.serverCmd != nil && gui.serverCmd.Process != nil {
		serverStatus = "running"
	}

	fmt.Fprintf(v, "server: %s\n", serverStatus)
	fmt.Fprintf(v, "database: %s\n", gui.databaseLabel())
	if gui.hasMakefile() {
		fmt.Fprintln(v, "workflow: make")
	} else {
		fmt.Fprintln(v, "workflow: django")
	}
	applied, total := gui.migrationSummary()
	fmt.Fprintf(v, "apps:%d models:%d migrations:%d/%d\n", len(gui.project.InstalledApps), len(gui.project.Models), applied, total)

	if gui.project.HasDocker {
		if len(gui.containerStatus) == 0 {
			fmt.Fprintln(v, "docker: configured")
		} else {
			running := 0
			for _, state := range gui.containerStatus {
				if state == "running" {
					running++
				}
			}
			fmt.Fprintf(v, "docker: %d/%d running\n", running, len(gui.containerStatus))
		}
	} else {
		fmt.Fprintln(v, "docker: not configured")
	}
	fmt.Fprintf(v, "update: %s\n", gui.updateStatusLine())
	fmt.Fprintf(v, "history: cmd:%d model:%d err:%d\n", len(gui.commandHistory), len(gui.recentModels), len(gui.recentErrors))
	if strings.TrimSpace(gui.historyStoreErr) != "" {
		fmt.Fprintln(v, "history-log: write error")
	}
	tasks, tasksErr := gui.loadProjectTasks(false)
	if tasksErr != nil {
		fmt.Fprintln(v, "tasks: error")
	} else {
		fmt.Fprintf(v, "tasks: %d\n", len(tasks))
	}

	fmt.Fprintln(v, "")
	for i, action := range gui.projectActions() {
		fmt.Fprintf(v, "%s%s\n", gui.rowCursor(MenuWindow, i), action.label)
	}
}

func (gui *Gui) renderDatabaseList(v *gocui.View) {
	v.Clear()
	gui.clampSelections()

	models := gui.sortedModels()
	fmt.Fprintf(v, "models: %d\n", len(models))
	fmt.Fprintln(v, "")

	if len(models) == 0 {
		fmt.Fprintln(v, "No models discovered")
		return
	}

	for i, model := range models {
		label := fmt.Sprintf("%s.%s (%d fields)", model.App, model.Name, model.Fields)
		fmt.Fprintf(v, "%s%s\n", gui.rowCursor(ListWindow, i), label)
	}
}

func (gui *Gui) loadSnapshots(force bool) ([]*django.Snapshot, error) {
	if gui.snapshotLoaded && !force {
		return gui.snapshotCache, gui.snapshotErr
	}

	sm := django.NewSnapshotManager(gui.project)
	snapshots, err := sm.ListSnapshots()
	gui.snapshotLoaded = true
	gui.snapshotCache = snapshots
	gui.snapshotErr = err
	return snapshots, err
}

func (gui *Gui) invalidateSnapshotCache() {
	gui.snapshotLoaded = false
	gui.snapshotCache = nil
	gui.snapshotErr = nil
}

func (gui *Gui) renderDataList(v *gocui.View) {
	v.Clear()
	gui.clampSelections()

	snapshots, err := gui.loadSnapshots(false)
	if err != nil {
		fmt.Fprintf(v, "snapshots: error (%v)\n", err)
	} else {
		fmt.Fprintf(v, "snapshots: %d\n", len(snapshots))
		if len(snapshots) > 0 {
			fmt.Fprintf(v, "latest: %s\n", snapshots[0].Name)
		}
	}

	fmt.Fprintln(v, "")
	for i, action := range gui.dataActions() {
		fmt.Fprintf(v, "%s%s\n", gui.rowCursor(DataWindow, i), action)
	}
}

func (gui *Gui) updateOptionsView(v *gocui.View) {
	v.Clear()

	if gui.isModalOpen {
		switch gui.modalType {
		case "delete", "restore":
			fmt.Fprint(v, "Modal | j/k:move  Enter:confirm  Esc/q:cancel")
		case "containers":
			fmt.Fprint(v, "Modal | j/k:move  Space:toggle  a:all  n:none  Enter:run  Esc/q:cancel")
		case "projectActions":
			fmt.Fprint(v, "Modal | j/k:move  g/G:top/bottom  Ctrl+d/u:half-page  PgUp/PgDn:page  0-9:jump  Enter:run  e:edit  Esc/q:cancel")
		case "outputTabs":
			fmt.Fprint(v, "Modal | j/k:move  g/G:top/bottom  Enter:switch tab  Esc/q:cancel")
		default:
			fmt.Fprint(v, "Modal | j/k:field  Enter/e:edit  Space:toggle bool  Ctrl+S:save  Esc/q:cancel")
		}
		return
	}
	if gui.inputMode == "command" {
		fmt.Fprint(v, "Command modal | Type shell/manage/make command  Enter:run  Esc:cancel  :help for key reference")
		return
	}
	if gui.inputMode == "search" {
		fmt.Fprint(v, "Search modal | Type query for current panel  Enter:jump  Esc:cancel")
		return
	}
	if gui.inputMode == "output-input" {
		fmt.Fprint(v, "Process input | Type input for running command  Enter:send  Esc:cancel")
		return
	}

	global := "Nav | 1/2/3/4:panel  Tab/h/l:switch  j/k:move  g/G:top/bottom  Ctrl+d/u:page  Enter:run  :command  /search  v:select  y:copy  t:tabs  r:refresh  q:quit"
	context := ""

	switch gui.currentWindow {
	case MenuWindow:
		context = "Project | Enter opens action groups, e:edit selected task config, u/D:container selector, U:update details"
	case ListWindow:
		context = "Database | Enter opens selected model data"
	case DataWindow:
		context = "Data | Enter action, c:create, L:list, R:restore"
	case MainWindow:
		if gui.currentModel != "" {
			context = "Output(model) | j/k/J/K:record  n/p or Ctrl+d/u:page  g/G:first/last row  a/e/d:CRUD  Esc:close model"
		} else {
			if gui.outputSelectMode {
				context = fmt.Sprintf("Output(%s) select | j/k:extend  g/G:top/bottom  y:copy selected lines  v/Esc:exit select  [ ]:tabs  x:close", gui.currentOutputTabLabel())
			} else {
				context = fmt.Sprintf("Output(%s) | t:picker  [ ]:tabs  o:other type  x:close  Ctrl+L:clear  j/k/Ctrl+d/u:scroll  g/G:top/bottom  v:select  y:copy line  i:send input", gui.currentOutputTabLabel())
			}
		}
	default:
		context = ""
	}

	fmt.Fprintf(v, "%s\n%s", global, context)
}

func (gui *Gui) editableInputViewFocused(v *gocui.View) bool {
	if v == nil || !v.Editable {
		return false
	}
	switch v.Name() {
	case ModalInputWindow:
		return true
	default:
		return false
	}
}

func (gui *Gui) maybeTypeRuneInEditableView(v *gocui.View, ch rune) bool {
	if !gui.editableInputViewFocused(v) {
		return false
	}
	gocui.DefaultEditor.Edit(v, 0, ch, gocui.ModNone)
	return true
}

func (gui *Gui) maybeHandleKeyInEditableView(v *gocui.View, key gocui.Key) bool {
	if !gui.editableInputViewFocused(v) {
		return false
	}
	gocui.DefaultEditor.Edit(v, key, 0, gocui.ModNone)
	return true
}

func (gui *Gui) bindGlobalRuneKey(ch rune, handler func(*gocui.Gui, *gocui.View) error) error {
	return gui.g.SetKeybinding("", ch, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if gui.maybeTypeRuneInEditableView(v, ch) {
			return nil
		}
		return handler(g, v)
	})
}

func (gui *Gui) bindGlobalKey(key gocui.Key, handler func(*gocui.Gui, *gocui.View) error) error {
	return gui.g.SetKeybinding("", key, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if gui.maybeHandleKeyInEditableView(v, key) {
			return nil
		}
		return handler(g, v)
	})
}

func (gui *Gui) setKeybindings() error {
	if err := gui.bindGlobalKey(gocui.KeyCtrlC, gui.quit); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('q', gui.handleGlobalQuit); err != nil {
		return err
	}

	if err := gui.bindGlobalRuneKey('1', func(g *gocui.Gui, v *gocui.View) error {
		return gui.switchPanel(MenuWindow)
	}); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('2', func(g *gocui.Gui, v *gocui.View) error {
		return gui.switchPanel(ListWindow)
	}); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('3', func(g *gocui.Gui, v *gocui.View) error {
		return gui.switchPanel(DataWindow)
	}); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('4', func(g *gocui.Gui, v *gocui.View) error {
		return gui.switchPanel(MainWindow)
	}); err != nil {
		return err
	}

	if err := gui.bindGlobalKey(gocui.KeyTab, gui.focusNextPanel); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyBacktab, gui.focusPrevPanel); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('h', gui.focusPrevPanel); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('l', gui.focusNextPanel); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyArrowLeft, gui.focusPrevPanel); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyArrowRight, gui.focusNextPanel); err != nil {
		return err
	}

	if err := gui.bindGlobalRuneKey('j', gui.cursorDown); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyArrowDown, gui.cursorDown); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('k', gui.cursorUp); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyArrowUp, gui.cursorUp); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('g', gui.jumpToTop); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('G', gui.jumpToBottom); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyCtrlD, gui.pageDownVim); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyCtrlU, gui.pageUpVim); err != nil {
		return err
	}

	if err := gui.bindGlobalKey(gocui.KeyEnter, gui.executeCommand); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyEsc, gui.handleEsc); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey(':', gui.openCommandBar); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('/', gui.openSearchBar); err != nil {
		return err
	}

	if err := gui.bindGlobalRuneKey('r', gui.refresh); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('n', gui.nextPage); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('p', gui.prevPage); err != nil {
		return err
	}

	if err := gui.bindGlobalRuneKey('a', gui.handleAddKey); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('e', gui.handleEditKey); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('d', gui.handleDeleteKey); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('J', gui.nextRecord); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('K', gui.prevRecord); err != nil {
		return err
	}

	if err := gui.bindGlobalRuneKey('c', gui.createSnapshot); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('L', func(g *gocui.Gui, v *gocui.View) error {
		return gui.listSnapshots()
	}); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('R', func(g *gocui.Gui, v *gocui.View) error {
		return gui.showRestoreMenu()
	}); err != nil {
		return err
	}

	if err := gui.bindGlobalRuneKey('u', gui.startContainers); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('D', gui.stopContainers); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('U', gui.showUpdateInfo); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('o', gui.toggleOutputTab); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('i', gui.openOutputInputBar); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('v', gui.toggleOutputSelectionMode); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('y', gui.copyOutputSelection); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('t', gui.openOutputTabsModal); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('[', gui.prevOutputTab); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey(']', gui.nextOutputTab); err != nil {
		return err
	}
	if err := gui.bindGlobalRuneKey('x', gui.closeCurrentOutputTab); err != nil {
		return err
	}
	if err := gui.bindGlobalKey(gocui.KeyCtrlL, gui.clearCurrentOutputTab); err != nil {
		return err
	}

	return nil
}

func (gui *Gui) focusNextPanel(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	idx := 0
	for i, name := range panelOrder {
		if name == gui.currentWindow {
			idx = i
			break
		}
	}
	idx = (idx + 1) % len(panelOrder)
	return gui.switchPanel(panelOrder[idx])
}

func (gui *Gui) focusPrevPanel(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	idx := 0
	for i, name := range panelOrder {
		if name == gui.currentWindow {
			idx = i
			break
		}
	}
	idx--
	if idx < 0 {
		idx = len(panelOrder) - 1
	}
	return gui.switchPanel(panelOrder[idx])
}

func (gui *Gui) switchPanel(name string) error {
	changed := gui.currentWindow != name
	gui.currentWindow = name
	gui.clampSelections()
	if changed {
		gui.markStateDirty()
	}
	if gui.isModalOpen {
		return nil
	}
	if _, err := gui.g.SetCurrentView(name); err != nil && err != gocui.ErrUnknownView {
		return err
	}
	return nil
}

func (gui *Gui) setSelectionClamped(window string, idx int) {
	count := gui.selectionCount(window)
	if count <= 0 {
		gui.setSelectionFor(window, 0)
		return
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= count {
		idx = count - 1
	}
	prev := gui.selectionFor(window)
	gui.setSelectionFor(window, idx)
	if idx != prev {
		gui.markStateDirty()
	}
}

func (gui *Gui) jumpToTop(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	switch gui.currentWindow {
	case MenuWindow, ListWindow, DataWindow:
		gui.setSelectionClamped(gui.currentWindow, 0)
		return nil
	case MainWindow:
		if gui.currentModel != "" {
			if len(gui.currentRecords) == 0 || gui.selectedRecordIdx == 0 {
				return nil
			}
			gui.selectedRecordIdx = 0
			return gui.loadAndDisplayRecords()
		}

		tabID := gui.resolveOutputTabID("", false)
		tab, ok := gui.outputTabs[tabID]
		if !ok {
			return nil
		}
		lines := outputLines(gui.outputTextForTab(tabID))
		if len(lines) == 0 {
			return nil
		}

		if gui.outputSelectMode && gui.outputSelectTabID == tabID {
			gui.outputSelectCursor = 0
			gui.ensureOutputSelectionVisible(tabID)
			gui.refreshOutputView()
			return nil
		}

		tab.autoscroll = false
		tab.originX = 0
		tab.originY = 0
		gui.markStateDirty()
		gui.refreshOutputView()
		return nil
	default:
		return nil
	}
}

func (gui *Gui) jumpToBottom(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	switch gui.currentWindow {
	case MenuWindow, ListWindow, DataWindow:
		gui.setSelectionClamped(gui.currentWindow, gui.selectionCount(gui.currentWindow)-1)
		return nil
	case MainWindow:
		if gui.currentModel != "" {
			if len(gui.currentRecords) == 0 {
				return nil
			}
			last := len(gui.currentRecords) - 1
			if gui.selectedRecordIdx == last {
				return nil
			}
			gui.selectedRecordIdx = last
			return gui.loadAndDisplayRecords()
		}

		tabID := gui.resolveOutputTabID("", false)
		tab, ok := gui.outputTabs[tabID]
		if !ok {
			return nil
		}
		lines := outputLines(gui.outputTextForTab(tabID))
		if len(lines) == 0 {
			return nil
		}
		last := len(lines) - 1

		if gui.outputSelectMode && gui.outputSelectTabID == tabID {
			gui.outputSelectCursor = last
			gui.ensureOutputSelectionVisible(tabID)
			gui.refreshOutputView()
			return nil
		}

		tab.autoscroll = false
		tab.originX = 0
		tab.originY = last
		gui.markStateDirty()
		gui.refreshOutputView()
		return nil
	default:
		return nil
	}
}

func (gui *Gui) pageStepForView(viewName string, fallback int) int {
	view, err := gui.g.View(viewName)
	if err != nil || view == nil {
		if fallback < 1 {
			return 1
		}
		return fallback
	}
	_, h := view.Size()
	step := h / 2
	if step < 1 {
		step = 1
	}
	return step
}

func (gui *Gui) pageDownVim(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	switch gui.currentWindow {
	case MainWindow:
		if gui.currentModel != "" {
			return gui.nextPage(g, v)
		}
		return gui.scrollMain(gui.pageStepForView(MainWindow, 5))
	case MenuWindow, ListWindow, DataWindow:
		step := gui.pageStepForView(gui.currentWindow, 5)
		gui.setSelectionClamped(gui.currentWindow, gui.selectionFor(gui.currentWindow)+step)
		return nil
	default:
		return nil
	}
}

func (gui *Gui) pageUpVim(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	switch gui.currentWindow {
	case MainWindow:
		if gui.currentModel != "" {
			return gui.prevPage(g, v)
		}
		return gui.scrollMain(-gui.pageStepForView(MainWindow, 5))
	case MenuWindow, ListWindow, DataWindow:
		step := gui.pageStepForView(gui.currentWindow, 5)
		gui.setSelectionClamped(gui.currentWindow, gui.selectionFor(gui.currentWindow)-step)
		return nil
	default:
		return nil
	}
}

func (gui *Gui) inputBarTitle() string {
	switch gui.inputMode {
	case "command":
		return " : Command "
	case "search":
		return " / Search "
	case "output-input":
		tabID := gui.inputTargetTabID
		if tabID == "" {
			tabID = gui.resolveOutputTabID("", false)
		}
		if tabID != "" {
			return fmt.Sprintf(" Input -> %s ", gui.outputTitleForTab(tabID))
		}
		return " Input -> Output "
	default:
		return " Input "
	}
}

func (gui *Gui) layoutInputPrompt(g *gocui.Gui, maxX, maxY int) error {
	if gui.inputMode == "" {
		return nil
	}

	width := (maxX * 70) / 100
	if width < 44 {
		width = 44
	}
	if width > maxX-4 {
		width = maxX - 4
	}

	x0 := (maxX - width) / 2
	x1 := x0 + width
	height := 3
	y0 := (maxY - height) / 2
	if y0 < 1 {
		y0 = 1
	}
	y1 := y0 + height
	if y1 > maxY-3 {
		y1 = maxY - 3
		y0 = y1 - height
		if y0 < 1 {
			y0 = 1
		}
	}

	v, err := g.SetView(ModalInputWindow, x0, y0, x1, y1, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	v.Editable = true
	v.Wrap = false
	v.Highlight = false
	v.Title = gui.inputBarTitle()
	v.FrameColor = panelFrameColorActive
	v.TitleColor = panelTitleColorActive
	if err == gocui.ErrUnknownView {
		v.Clear()
	}

	g.DeleteKeybindings(ModalInputWindow)
	if err := g.SetKeybinding(ModalInputWindow, gocui.KeyEnter, gocui.ModNone, gui.submitInputBar); err != nil {
		return err
	}
	if err := g.SetKeybinding(ModalInputWindow, gocui.KeyEsc, gocui.ModNone, gui.cancelInputBar); err != nil {
		return err
	}
	if err := g.SetKeybinding(ModalInputWindow, gocui.KeyCtrlC, gocui.ModNone, gui.cancelInputBar); err != nil {
		return err
	}

	g.SetViewOnTop(ModalInputWindow)
	if _, err := g.SetCurrentView(ModalInputWindow); err != nil {
		return err
	}
	return nil
}

func (gui *Gui) openCommandBar(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.inputMode != "" {
		return nil
	}
	gui.inputMode = "command"
	gui.inputReturnWindow = gui.currentWindow
	maxX, maxY := gui.g.Size()
	return gui.layoutInputPrompt(gui.g, maxX, maxY)
}

func (gui *Gui) openSearchBar(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.inputMode != "" {
		return nil
	}
	gui.inputMode = "search"
	gui.inputReturnWindow = gui.currentWindow
	maxX, maxY := gui.g.Size()
	return gui.layoutInputPrompt(gui.g, maxX, maxY)
}

func (gui *Gui) openOutputInputBar(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.inputMode != "" {
		return nil
	}
	if gui.currentWindow != MainWindow || gui.currentModel != "" {
		return nil
	}

	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		return nil
	}
	if _, ok := gui.outputInputWriters[tabID]; !ok {
		return gui.showMessage("Input", "No running process is accepting input in this output tab.")
	}

	gui.inputMode = "output-input"
	gui.inputReturnWindow = gui.currentWindow
	gui.inputTargetTabID = tabID
	maxX, maxY := gui.g.Size()
	return gui.layoutInputPrompt(gui.g, maxX, maxY)
}

func (gui *Gui) closeInputBar() error {
	if gui.inputMode == "" {
		return nil
	}
	returnWindow := gui.inputReturnWindow
	if returnWindow == "" {
		returnWindow = gui.currentWindow
	}
	if returnWindow == "" {
		returnWindow = MainWindow
	}
	gui.inputMode = ""
	gui.inputReturnWindow = ""
	gui.inputTargetTabID = ""
	gui.g.DeleteKeybindings(ModalInputWindow)
	gui.g.DeleteView(ModalInputWindow)
	if _, err := gui.g.SetCurrentView(returnWindow); err != nil && err != gocui.ErrUnknownView {
		return err
	}
	return nil
}

func (gui *Gui) cancelInputBar(g *gocui.Gui, v *gocui.View) error {
	return gui.closeInputBar()
}

func (gui *Gui) submitInputBar(g *gocui.Gui, v *gocui.View) error {
	if gui.inputMode == "" {
		return nil
	}
	raw := ""
	if v != nil {
		raw = strings.TrimRight(v.Buffer(), "\r\n")
	}
	mode := gui.inputMode
	if err := gui.closeInputBar(); err != nil {
		return err
	}
	if mode == "command" {
		input := strings.TrimSpace(raw)
		if input == "" {
			return nil
		}
		return gui.runCommandBarInput(input)
	}
	if mode == "search" {
		input := strings.TrimSpace(raw)
		if input == "" {
			return nil
		}
		return gui.searchCurrentWindow(input)
	}
	if mode == "output-input" {
		return gui.sendOutputInput(raw)
	}
	return nil
}

func (gui *Gui) sendOutputInput(input string) error {
	tabID := gui.inputTargetTabID
	if tabID == "" {
		tabID = gui.resolveOutputTabID("", false)
	}
	if tabID == "" {
		return nil
	}

	writer, ok := gui.outputInputWriters[tabID]
	if !ok || writer == nil {
		return gui.showMessage("Input", "This process is no longer accepting input.")
	}

	payload := input + "\n"
	if _, err := io.WriteString(writer, payload); err != nil {
		return gui.showMessage("Input", fmt.Sprintf("Failed to send input: %v", err))
	}
	return nil
}

func isHelpCommand(input string) bool {
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), ":"))
	switch strings.ToLower(trimmed) {
	case "help", "h", "?":
		return true
	default:
		return false
	}
}

func (gui *Gui) runCommandBarInput(input string) error {
	if isHelpCommand(input) {
		return gui.openHelpModal()
	}
	return gui.runFavoriteCommand(input)
}

func (gui *Gui) openHelpModal() error {
	if gui.isModalOpen {
		return nil
	}

	returnWindow := gui.currentWindow
	if returnWindow == "" {
		returnWindow = MainWindow
	}

	gui.isModalOpen = true
	gui.modalType = "help"
	gui.modalReturnWindow = returnWindow
	gui.modalTitle = "Help"
	gui.modalMessage = gui.helpContent()
	gui.modalFields = nil
	gui.modalValues = nil
	gui.restoreSnapshots = nil
	gui.containerList = nil
	gui.projectModalActions = nil
	gui.outputTabModalIDs = nil

	return nil
}

func (gui *Gui) helpContent() string {
	return strings.Join([]string{
		"Panels",
		"  1/2/3/4      Focus Project/Database/Data/Output",
		"  Tab/Shift+Tab or h/l",
		"               Move panel focus",
		"  j/k           Move list cursor or scroll output",
		"  g/G           Jump to first/last item in focused panel",
		"  Ctrl+d/u      Page down/up in focused panel",
		"  Enter         Run/open selected item",
		"",
		"Command/Search",
		"  :             Open command bar",
		"  :help         Open this help modal",
		"  /             Search current panel/output and jump to closest match",
		"  Esc           Close command/search bar",
		"",
		"Output Tabs",
		"  t             Open tab picker",
		"  [ / ]         Previous/next tab",
		"  o             Toggle command/logs route",
		"  x             Close current tab",
		"  Ctrl+L        Clear current tab",
		"",
		"Project/Data",
		"  Server...     Start/Stop dev server from Project panel",
		"  u / D         Open start/stop container selector",
		"  c / L / R     Create/List/Restore snapshots",
		"",
		"Model Data View",
		"  j/k or J/K    Select previous/next record",
		"  n / p         Next/previous page",
		"  Ctrl+d / Ctrl+u  Next/previous page (vim-style)",
		"  g / G         Jump to first/last record on page",
		"  a / e / d     Add/Edit/Delete record",
		"  Esc           Close model view",
		"",
		"General",
		"  r             Refresh project metadata",
		"  U             Show update information",
		"  q or Ctrl+C   Quit",
		"",
		"Help Modal",
		"  j/k or Up/Down to scroll",
		"  Enter/Esc/q to close",
	}, "\n")
}

func nextMatchIndex(labels []string, query string, current int) int {
	if len(labels) == 0 {
		return -1
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return clampSelection(current, len(labels))
	}

	current = clampSelection(current, len(labels))
	matches := make([]bool, len(labels))
	hasMatch := false
	for idx, label := range labels {
		if strings.Contains(strings.ToLower(label), q) {
			matches[idx] = true
			hasMatch = true
		}
	}
	if !hasMatch {
		return -1
	}

	for step := 0; step < len(labels); step++ {
		forward := (current + step) % len(labels)
		if matches[forward] {
			return forward
		}
		if step == 0 {
			continue
		}
		backward := (current - step + len(labels)) % len(labels)
		if matches[backward] {
			return backward
		}
	}

	return -1
}

func (gui *Gui) searchCurrentWindow(query string) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}

	switch gui.currentWindow {
	case MenuWindow:
		actions := gui.projectActions()
		labels := make([]string, 0, len(actions))
		for _, action := range actions {
			labels = append(labels, action.label)
		}
		if idx := nextMatchIndex(labels, query, gui.menuSelection); idx >= 0 {
			gui.menuSelection = idx
			gui.markStateDirty()
			return nil
		}
		return gui.showMessage("Search", "No match in Project panel.")
	case ListWindow:
		models := gui.sortedModels()
		labels := make([]string, 0, len(models))
		for _, model := range models {
			labels = append(labels, fmt.Sprintf("%s.%s", model.App, model.Name))
		}
		if idx := nextMatchIndex(labels, query, gui.listSelection); idx >= 0 {
			gui.listSelection = idx
			gui.markStateDirty()
			return nil
		}
		return gui.showMessage("Search", "No match in Database panel.")
	case DataWindow:
		actions := gui.dataActions()
		if idx := nextMatchIndex(actions, query, gui.dataSelection); idx >= 0 {
			gui.dataSelection = idx
			gui.markStateDirty()
			return nil
		}
		return gui.showMessage("Search", "No match in Data panel.")
	case MainWindow:
		if gui.currentModel == "" {
			return gui.searchCurrentOutput(query)
		}
		labels := make([]string, 0, len(gui.currentRecords))
		for _, record := range gui.currentRecords {
			labels = append(labels, fmt.Sprintf("%v %s", record.PK, gui.getRecordDisplayString(record)))
		}
		if idx := nextMatchIndex(labels, query, gui.selectedRecordIdx); idx >= 0 {
			gui.selectedRecordIdx = idx
			return gui.loadAndDisplayRecords()
		}
		return gui.showMessage("Search", "No matching record in current model page.")
	default:
		return nil
	}
}

func (gui *Gui) searchCurrentOutput(query string) error {
	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		return gui.showMessage("Search", "No output tab is currently available.")
	}

	tab, ok := gui.outputTabs[tabID]
	if !ok {
		return gui.showMessage("Search", "Current output tab is not available.")
	}

	lines := strings.Split(gui.outputTextForTab(tabID), "\n")
	if len(lines) == 0 {
		return gui.showMessage("Search", "No output to search in this tab.")
	}

	current := tab.originY
	if current == outputOriginTail || current < 0 {
		current = len(lines) - 1
	}
	if current >= len(lines) {
		current = len(lines) - 1
	}

	idx := nextMatchIndex(lines, query, current)
	if idx < 0 {
		return gui.showMessage("Search", "No match in current output tab.")
	}

	tab.autoscroll = false
	tab.originX = 0
	tab.originY = idx
	gui.markStateDirty()
	gui.refreshOutputView()
	return nil
}

func (gui *Gui) openOutputTabsModal(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}

	tabIDs := gui.orderedOutputTabIDsForPicker()
	if len(tabIDs) == 0 {
		return nil
	}

	returnWindow := gui.currentWindow
	if returnWindow == "" {
		returnWindow = MainWindow
	}

	selectedIdx := 0
	currentID := gui.resolveOutputTabID("", false)
	for i, id := range tabIDs {
		if id == currentID {
			selectedIdx = i
			break
		}
	}

	gui.isModalOpen = true
	gui.modalType = "outputTabs"
	gui.modalReturnWindow = returnWindow
	gui.modalTitle = "Output Tabs"
	gui.modalFields = nil
	gui.modalValues = nil
	gui.modalMessage = ""
	gui.restoreSnapshots = nil
	gui.restoreIndex = 0
	gui.containerAction = ""
	gui.containerList = nil
	gui.containerIndex = 0
	gui.containerSelect = nil
	gui.projectModalActions = nil
	gui.projectModalIndex = 0
	gui.projectModalOffset = 0
	gui.projectModalNumber = ""
	gui.outputTabModalIDs = tabIDs
	gui.outputTabModalIndex = selectedIdx

	return nil
}

func (gui *Gui) toggleOutputTab(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}
	currentID := gui.resolveOutputTabID("", false)
	if currentID == "" {
		return gui.switchPanel(MainWindow)
	}
	if gui.outputTabs[currentID].route == OutputTabLogs {
		gui.switchOutputTab(OutputTabCommand)
	} else {
		gui.switchOutputTab(OutputTabLogs)
	}
	return gui.switchPanel(MainWindow)
}

func (gui *Gui) nextOutputTab(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}
	if len(gui.outputOrder) == 0 {
		return gui.switchPanel(MainWindow)
	}
	currentID := gui.resolveOutputTabID("", false)
	if currentID == "" {
		gui.outputTab = gui.outputOrder[0]
		gui.markStateDirty()
		gui.refreshOutputView()
		return gui.switchPanel(MainWindow)
	}
	idx := gui.outputTabPosition(currentID)
	if idx < 0 {
		gui.outputTab = gui.outputOrder[0]
		gui.markStateDirty()
		gui.refreshOutputView()
		return gui.switchPanel(MainWindow)
	}
	nextIdx := (idx + 1) % len(gui.outputOrder)
	if gui.outputTab != gui.outputOrder[nextIdx] {
		gui.outputTab = gui.outputOrder[nextIdx]
		gui.markStateDirty()
	}
	gui.refreshOutputView()
	return gui.switchPanel(MainWindow)
}

func (gui *Gui) prevOutputTab(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}
	if len(gui.outputOrder) == 0 {
		return gui.switchPanel(MainWindow)
	}
	currentID := gui.resolveOutputTabID("", false)
	if currentID == "" {
		gui.outputTab = gui.outputOrder[len(gui.outputOrder)-1]
		gui.markStateDirty()
		gui.refreshOutputView()
		return gui.switchPanel(MainWindow)
	}
	idx := gui.outputTabPosition(currentID)
	if idx < 0 {
		gui.outputTab = gui.outputOrder[len(gui.outputOrder)-1]
		gui.markStateDirty()
		gui.refreshOutputView()
		return gui.switchPanel(MainWindow)
	}
	prevIdx := idx - 1
	if prevIdx < 0 {
		prevIdx = len(gui.outputOrder) - 1
	}
	if gui.outputTab != gui.outputOrder[prevIdx] {
		gui.outputTab = gui.outputOrder[prevIdx]
		gui.markStateDirty()
	}
	gui.refreshOutputView()
	return gui.switchPanel(MainWindow)
}

func (gui *Gui) clearCurrentOutputTab(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}
	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		return nil
	}
	gui.setOutputTextForTab(tabID, "")
	if gui.outputSelectMode && gui.outputSelectTabID == tabID {
		gui.outputSelectMode = false
		gui.outputSelectTabID = ""
	}
	if gui.inputMode == "output-input" && gui.inputTargetTabID == tabID {
		gui.inputMode = ""
		gui.inputTargetTabID = ""
		gui.inputReturnWindow = ""
		gui.g.DeleteKeybindings(ModalInputWindow)
		gui.g.DeleteView(ModalInputWindow)
	}
	gui.markStateDirty()
	gui.refreshOutputView()
	return nil
}

func (gui *Gui) closeCurrentOutputTab(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}

	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		return nil
	}

	idx := gui.outputTabPosition(tabID)
	if idx < 0 {
		return nil
	}

	delete(gui.outputTabs, tabID)
	gui.outputOrder = append(gui.outputOrder[:idx], gui.outputOrder[idx+1:]...)
	if writer, ok := gui.outputInputWriters[tabID]; ok {
		_ = writer.Close()
		delete(gui.outputInputWriters, tabID)
	}
	if gui.outputSelectMode && gui.outputSelectTabID == tabID {
		gui.outputSelectMode = false
		gui.outputSelectTabID = ""
	}
	if gui.inputMode == "output-input" && gui.inputTargetTabID == tabID {
		gui.inputMode = ""
		gui.inputTargetTabID = ""
		gui.inputReturnWindow = ""
		gui.g.DeleteKeybindings(ModalInputWindow)
		gui.g.DeleteView(ModalInputWindow)
	}

	for route, mapped := range gui.outputRoutes {
		if mapped != tabID {
			continue
		}
		if latest := gui.latestOutputTabForRoute(route); latest != "" {
			gui.outputRoutes[route] = latest
		} else {
			delete(gui.outputRoutes, route)
		}
	}

	if len(gui.outputOrder) == 0 {
		gui.outputTab = ""
		gui.markStateDirty()
		gui.refreshOutputView()
		return nil
	}

	if idx >= len(gui.outputOrder) {
		idx = len(gui.outputOrder) - 1
	}
	gui.outputTab = gui.outputOrder[idx]
	gui.markStateDirty()
	gui.refreshOutputView()
	return nil
}

func (gui *Gui) showUpdateInfo(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}
	gui.showUpdatePromptInOutput()
	return gui.switchPanel(MainWindow)
}

func (gui *Gui) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	if gui.currentWindow == MainWindow {
		if gui.currentModel != "" && len(gui.currentRecords) > 0 {
			return gui.nextRecord(g, v)
		}
		if gui.outputSelectMode {
			return gui.moveOutputSelection(1)
		}
		return gui.scrollMain(1)
	}

	gui.moveSelection(1)
	return nil
}

func (gui *Gui) cursorUp(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	if gui.currentWindow == MainWindow {
		if gui.currentModel != "" && len(gui.currentRecords) > 0 {
			return gui.prevRecord(g, v)
		}
		if gui.outputSelectMode {
			return gui.moveOutputSelection(-1)
		}
		return gui.scrollMain(-1)
	}

	gui.moveSelection(-1)
	return nil
}

func (gui *Gui) scrollMain(delta int) error {
	v, err := gui.g.View(MainWindow)
	if err != nil {
		return nil
	}

	ox, oy := v.Origin()
	ny := oy + delta
	if ny < 0 {
		ny = 0
	}
	if err := v.SetOrigin(ox, ny); err != nil {
		return nil
	}
	if gui.currentModel == "" {
		tabID := gui.resolveOutputTabID("", false)
		if tab, ok := gui.outputTabs[tabID]; ok && !tab.autoscroll {
			prevX, prevY := tab.originX, tab.originY
			tab.originX = ox
			tab.originY = ny
			if tab.originX != prevX || tab.originY != prevY {
				gui.markStateDirty()
			}
		}
	}
	return nil
}

func (gui *Gui) executeCommand(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	switch gui.currentWindow {
	case MenuWindow:
		return gui.executeProjectSelection()
	case ListWindow:
		return gui.openSelectedModel()
	case DataWindow:
		return gui.executeDataSelection()
	case MainWindow:
		return nil
	default:
		return nil
	}
}

func (gui *Gui) executeProjectSelection() error {
	actions := gui.projectActions()
	if len(actions) == 0 {
		return nil
	}
	idx := clampSelection(gui.menuSelection, len(actions))
	return gui.runProjectAction(actions[idx])
}

func (gui *Gui) openProjectActionsModal(title string, actions []projectAction) error {
	if len(actions) == 0 {
		return gui.showMessage(title, "No actions available.")
	}

	returnWindow := gui.currentWindow
	if returnWindow == "" {
		returnWindow = MainWindow
	}

	gui.isModalOpen = true
	gui.modalType = "projectActions"
	gui.modalReturnWindow = returnWindow
	gui.modalTitle = title
	gui.modalFields = nil
	gui.modalValues = nil
	gui.modalMessage = ""
	gui.restoreSnapshots = nil
	gui.restoreIndex = 0
	gui.containerAction = ""
	gui.containerList = nil
	gui.containerIndex = 0
	gui.containerSelect = nil
	gui.projectModalActions = actions
	gui.projectModalIndex = 0
	gui.projectModalOffset = 0
	gui.projectModalNumber = ""
	return nil
}

func (gui *Gui) editSelectedProjectModalAction() error {
	if gui.modalType != "projectActions" {
		return nil
	}
	if isProjectTasksModalTitle(gui.modalTitle) {
		if err := gui.closeModal(); err != nil {
			return err
		}
		return gui.editProjectTasksFile()
	}
	if len(gui.projectModalActions) == 0 {
		return nil
	}

	idx := clampSelection(gui.projectModalIndex, len(gui.projectModalActions))
	action := gui.projectModalActions[idx]
	if !isEditableProjectAction(action) {
		return nil
	}

	if err := gui.closeModal(); err != nil {
		return err
	}
	return gui.editProjectTasksFile()
}

func (gui *Gui) runProjectAction(action projectAction) error {
	if action.shellCommand != "" {
		return gui.runFavoriteCommand(action.shellCommand)
	}

	if action.makeTarget != "" {
		return gui.runMakeTarget(action.label, action.makeTarget)
	}

	if action.command != "" {
		args := strings.Fields(action.command)
		return gui.runManageCommand(action.label, args...)
	}

	switch action.internal {
	case "openserver":
		return gui.openProjectActionsModal("Server Actions", gui.projectServerActions())
	case "runserver":
		return gui.startServer(gui.g, nil)
	case "stopserver":
		return gui.stopServer(gui.g, nil)
	case "showurls":
		return gui.showURLPatterns()
	case "doctor":
		return gui.showDependencyDoctor()
	case "refresh":
		return gui.refresh(gui.g, nil)
	case "startcontainers":
		return gui.startContainers(gui.g, nil)
	case "stopcontainers":
		return gui.stopContainers(gui.g, nil)
	case "opencontainers":
		return gui.openProjectActionsModal("Container Actions", gui.projectContainerActions())
	case "openprojecttasks":
		return gui.openProjectActionsModal("Project Tasks", gui.projectTaskActions())
	case "openfavorites":
		return gui.openProjectActionsModal("Favorite Commands", gui.projectFavoriteActions())
	case "openrecentcommands":
		return gui.openProjectActionsModal("Recent Commands", gui.projectRecentCommandActions())
	case "editprojecttasks":
		return gui.editProjectTasksFile()
	case "openmigrations":
		return gui.openProjectActionsModal("Migration Actions", gui.projectMigrationActions())
	case "opentools":
		return gui.openProjectActionsModal("Tool Actions", gui.projectToolActions())
	case "historyreport":
		return gui.showHistoryReport()
	default:
		return nil
	}
}

func (gui *Gui) runFavoriteCommand(command string) error {
	command = sanitizeCommand(command)
	if command == "" {
		return nil
	}

	if strings.HasPrefix(command, "make ") {
		fields := strings.Fields(command)
		if len(fields) == 2 {
			return gui.runMakeTarget(command, fields[1])
		}
	}

	return gui.runShellCommand(command)
}

func (gui *Gui) runShellCommand(command string) error {
	command = sanitizeCommand(command)
	if command == "" {
		return nil
	}

	cmd := newTTYCompatShellCommand(command, gui.project.RootDir)
	return gui.runStreamingCommandToRoute(OutputTabCommand, tabTitleFromCommand(command), cmd, false, command, nil)
}

func (gui *Gui) showHistoryReport() error {
	tabID := gui.startCommandOutputTab("History Report")
	gui.resetOutput(tabID, "History Report")
	_ = gui.switchPanel(MainWindow)

	gui.appendOutput(tabID, "LazyDjango project history\n\n")
	gui.appendOutput(tabID, fmt.Sprintf("Recent commands: %d\n", len(gui.commandHistory)))
	gui.appendOutput(tabID, fmt.Sprintf("Favorite commands: %d\n", len(gui.favoriteCommands)))
	gui.appendOutput(tabID, fmt.Sprintf("Recent models: %d\n", len(gui.recentModels)))
	gui.appendOutput(tabID, fmt.Sprintf("Recent errors: %d\n\n", len(gui.recentErrors)))

	if len(gui.favoriteCommands) > 0 {
		gui.appendOutput(tabID, "Favorites:\n")
		for i, command := range gui.favoriteCommands {
			gui.appendOutput(tabID, fmt.Sprintf("%2d. %s\n", i+1, command))
		}
		gui.appendOutput(tabID, "\n")
	}

	if len(gui.recentModels) > 0 {
		gui.appendOutput(tabID, "Recent models:\n")
		for i, model := range gui.recentModels {
			lastPage := model.LastPage
			if lastPage < 1 {
				lastPage = 1
			}
			line := fmt.Sprintf("%2d. %s.%s (page %d, row %d)", i+1, model.App, model.Model, lastPage, model.LastRecordIdx+1)
			if model.LastRecordPK != "" {
				line = fmt.Sprintf("%s pk=%s", line, model.LastRecordPK)
			}
			if model.LastAccessedAt != "" {
				line = fmt.Sprintf("%s at %s", line, model.LastAccessedAt)
			}
			gui.appendOutput(tabID, line+"\n")
		}
		gui.appendOutput(tabID, "\n")
	}

	if len(gui.recentErrors) > 0 {
		gui.appendOutput(tabID, "Recent errors:\n")
		for i, item := range gui.recentErrors {
			source := item.Source
			if source == "" {
				source = "unknown"
			}
			gui.appendOutput(tabID, fmt.Sprintf("%2d. [%s] x%d %s\n", i+1, source, item.Count, item.Message))
		}
		gui.appendOutput(tabID, "\n")
	}

	if gui.historyStore != nil {
		events, err := gui.historyStore.tail(20)
		if err != nil {
			gui.appendOutput(tabID, fmt.Sprintf("History log unavailable: %v\n", err))
		} else if len(events) > 0 {
			gui.appendOutput(tabID, "Recent events:\n")
			for _, event := range events {
				line := fmt.Sprintf("- %s [%s]", event.Time, event.Type)
				if event.Action != "" {
					line += " " + event.Action
				}
				if event.Command != "" {
					line += " " + event.Command
				}
				if event.Status != "" {
					line += " (" + event.Status + ")"
				}
				if event.Error != "" {
					line += " err=" + event.Error
				}
				gui.appendOutput(tabID, line+"\n")
			}
		}
	}

	gui.refreshOutputView()
	return nil
}

func (gui *Gui) runManageCommand(title string, args ...string) error {
	command := strings.TrimSpace(fmt.Sprintf("python manage.py %s", strings.Join(args, " ")))
	tabID := gui.startCommandOutputTab(tabTitleFromCommand(command))
	gui.appendOutput(tabID, fmt.Sprintf("$ %s\n\n", command))
	_ = gui.switchPanel(MainWindow)

	startedAt := time.Now()
	go func() {
		output, runErr := gui.project.RunCommand(args...)
		gui.g.Update(func(g *gocui.Gui) error {
			if runErr != nil {
				gui.appendOutput(tabID, fmt.Sprintf("Error: %v\n\n", runErr))
			}
			gui.appendOutput(tabID, output)
			gui.refreshOutputView()

			if len(args) > 0 && (args[0] == "makemigrations" || args[0] == "migrate") {
				gui.project.Migrations = nil
				gui.project.DiscoverMigrations()
			}

			gui.recordCommandExecution(command, tabID, startedAt, runErr)
			return nil
		})
	}()

	return nil
}

func isLongRunningMakeTarget(target string) bool {
	switch target {
	case "up", "up-all", "runserver", "watch", "storybook":
		return true
	default:
		return false
	}
}

func (gui *Gui) runStreamingCommandToRoute(
	route string,
	title string,
	cmd *exec.Cmd,
	trackAsServer bool,
	displayCommand string,
	onExit func(*gocui.Gui, error),
) error {
	if !isOutputRoute(route) {
		route = OutputTabCommand
	}

	command := strings.TrimSpace(displayCommand)
	if command == "" {
		command = strings.TrimSpace(strings.Join(cmd.Args, " "))
	}
	tabTitle := strings.TrimSpace(title)
	if tabTitle == "" {
		tabTitle = tabTitleFromCommand(command)
	}
	if tabTitle == "" {
		tabTitle = outputDefaultTitle(route)
	}
	var tabID string
	if route == OutputTabLogs {
		tabID = gui.startLogsOutputTab(tabTitle)
	} else {
		tabID = gui.startCommandOutputTab(tabTitle)
	}
	if command != "" {
		gui.appendOutput(tabID, fmt.Sprintf("$ %s\n\n", command))
	}
	_ = gui.switchPanel(MainWindow)
	startedAt := time.Now()
	prepareCommandForExecution(cmd, false)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		stdin = nil
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		if stdin != nil {
			_ = stdin.Close()
		}
		gui.appendOutput(tabID, fmt.Sprintf("Failed to open stdout: %v\n", err))
		gui.recordCommandExecution(command, tabID, startedAt, err)
		gui.refreshOutputView()
		return nil
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		if stdin != nil {
			_ = stdin.Close()
		}
		gui.appendOutput(tabID, fmt.Sprintf("Failed to open stderr: %v\n", err))
		gui.recordCommandExecution(command, tabID, startedAt, err)
		gui.refreshOutputView()
		return nil
	}

	if err := cmd.Start(); err != nil {
		if stdin != nil {
			_ = stdin.Close()
		}
		gui.appendOutput(tabID, fmt.Sprintf("Failed to start command: %v\n", err))
		gui.recordCommandExecution(command, tabID, startedAt, err)
		gui.refreshOutputView()
		return nil
	}

	if stdin != nil {
		gui.outputInputWriters[tabID] = stdin
	}

	if trackAsServer {
		gui.serverCmd = cmd
	}

	chunkCh := make(chan string, 256)
	var wg sync.WaitGroup

	readStream := func(reader io.ReadCloser) {
		defer wg.Done()
		defer reader.Close()

		buf := make([]byte, 4096)
		for {
			n, readErr := reader.Read(buf)
			if n > 0 {
				chunkCh <- string(buf[:n])
			}
			if readErr == nil {
				continue
			}
			if !errors.Is(readErr, io.EOF) {
				chunkCh <- fmt.Sprintf("[stream error: %v]\n", readErr)
			}
			return
		}
	}

	wg.Add(2)
	go readStream(stdout)
	go readStream(stderr)
	go func() {
		wg.Wait()
		close(chunkCh)
	}()

	go func() {
		var captured strings.Builder
		for chunk := range chunkCh {
			captured.WriteString(chunk)
			chunkCopy := chunk
			gui.g.Update(func(g *gocui.Gui) error {
				gui.appendOutput(tabID, chunkCopy)
				gui.refreshOutputView()
				return nil
			})
		}

		waitErr := cmd.Wait()
		gui.g.Update(func(g *gocui.Gui) error {
			if writer, ok := gui.outputInputWriters[tabID]; ok {
				_ = writer.Close()
				delete(gui.outputInputWriters, tabID)
			}
			if waitErr != nil {
				gui.appendOutput(tabID, fmt.Sprintf("\nProcess exited with error: %v\n", waitErr))
			} else {
				gui.appendOutput(tabID, "\nProcess exited.\n")
			}
			if waitErr != nil && strings.Contains(strings.ToLower(captured.String()), "is not running") {
				gui.appendOutput(tabID, "Hint: start the required services first (Project -> Containers..., or press 'u').\n")
			}

			if trackAsServer {
				gui.serverCmd = nil
				if menuView, err := g.View(MenuWindow); err == nil {
					gui.renderProjectList(menuView)
				}
			}
			if onExit != nil {
				onExit(g, waitErr)
			}
			gui.recordCommandExecution(command, tabID, startedAt, waitErr)
			gui.refreshOutputView()
			return nil
		})
	}()

	return nil
}

func (gui *Gui) runStreamingCommandToLogs(title string, cmd *exec.Cmd, trackAsServer bool, displayCommand string) error {
	return gui.runStreamingCommandToRoute(OutputTabLogs, title, cmd, trackAsServer, displayCommand, nil)
}

func (gui *Gui) runMakeTarget(_ string, target string) error {
	if target == "runserver" {
		return gui.startServer(gui.g, nil)
	}

	command := fmt.Sprintf("make %s", target)
	cmd := newTTYCompatShellCommand(command, gui.project.RootDir)
	route := OutputTabCommand
	if isLongRunningMakeTarget(target) {
		route = OutputTabLogs
	}

	return gui.runStreamingCommandToRoute(route, tabTitleFromCommand(command), cmd, false, command, func(g *gocui.Gui, _ error) {
		switch target {
		case "migrations", "migrate", "migrate-site", "showmigrations":
			gui.project.Migrations = nil
			gui.project.DiscoverMigrations()
		case "up", "up-all", "down", "restart", "restart-all":
			gui.refreshContainerStatus()
		}
		if menuView, err := g.View(MenuWindow); err == nil {
			gui.renderProjectList(menuView)
		}
	})
}

func (gui *Gui) openSelectedModel() error {
	models := gui.sortedModels()
	if len(models) == 0 {
		return nil
	}

	idx := clampSelection(gui.listSelection, len(models))
	model := models[idx]

	gui.currentApp = model.App
	gui.currentModel = model.Name
	gui.currentPage = 1
	gui.selectedRecordIdx = 0
	if recent, ok := gui.recentModelState(model.App, model.Name); ok {
		if recent.LastPage > 0 {
			gui.currentPage = recent.LastPage
		}
		if recent.LastRecordIdx > 0 {
			gui.selectedRecordIdx = recent.LastRecordIdx
		}
	}
	gui.currentWindow = MainWindow
	gui.markStateDirty()

	if _, err := gui.g.SetCurrentView(MainWindow); err != nil && err != gocui.ErrUnknownView {
		return err
	}

	if err := gui.loadAndDisplayRecords(); err != nil {
		return err
	}
	var selectedPK interface{}
	if len(gui.currentRecords) > 0 && gui.selectedRecordIdx >= 0 && gui.selectedRecordIdx < len(gui.currentRecords) {
		selectedPK = gui.currentRecords[gui.selectedRecordIdx].PK
	}
	gui.recordModelOpen(gui.currentApp, gui.currentModel, gui.currentPage, selectedPK)
	return nil
}

func (gui *Gui) executeDataSelection() error {
	actions := gui.dataActions()
	if len(actions) == 0 {
		return nil
	}

	idx := clampSelection(gui.dataSelection, len(actions))
	switch actions[idx] {
	case "Create snapshot":
		return gui.createSnapshot(gui.g, nil)
	case "List snapshots":
		return gui.listSnapshots()
	case "Restore snapshot":
		return gui.showRestoreMenu()
	default:
		return nil
	}
}

func (gui *Gui) handleEsc(g *gocui.Gui, v *gocui.View) error {
	if gui.inputMode != "" {
		return gui.closeInputBar()
	}
	if gui.outputSelectMode {
		gui.outputSelectMode = false
		gui.outputSelectTabID = ""
		gui.refreshOutputView()
		return nil
	}
	if gui.isModalOpen {
		return gui.closeModal()
	}
	if gui.currentModel == "" {
		return nil
	}
	return gui.clearModelView()
}

func (gui *Gui) clearModelView() error {
	gui.currentApp = ""
	gui.currentModel = ""
	gui.currentRecords = nil
	gui.currentPage = 1
	gui.selectedRecordIdx = 0
	gui.totalRecords = 0

	mainView, err := gui.g.View(MainWindow)
	if err == nil {
		gui.renderOutputView(mainView)
	}

	return gui.switchPanel(ListWindow)
}

// handleAddKey handles add context.
func (gui *Gui) handleAddKey(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}
	return gui.addRecord(g, v)
}

// handleEditKey handles edit context.
func (gui *Gui) handleEditKey(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		switch gui.modalType {
		case "add", "edit":
			return gui.editModalField()
		case "projectActions":
			return gui.editSelectedProjectModalAction()
		default:
			return nil
		}
	}
	if gui.currentWindow == MenuWindow {
		return gui.editSelectedProjectAction()
	}
	return gui.editRecord(g, v)
}

// handleDeleteKey handles delete context.
func (gui *Gui) handleDeleteKey(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}
	return gui.deleteRecord(g, v)
}

func (gui *Gui) showURLPatterns() error {
	tabID := gui.startCommandOutputTab("URL Patterns")
	gui.appendOutput(tabID, "Loading URL patterns...\n")
	_ = gui.switchPanel(MainWindow)

	go func() {
		patterns, runErr := gui.project.GetURLPatterns()
		gui.g.Update(func(g *gocui.Gui) error {
			gui.resetOutput(tabID, "URL Patterns")
			if runErr != nil {
				gui.appendOutput(tabID, fmt.Sprintf("Error: %v\n", runErr))
				gui.rememberError("urls", runErr.Error())
				gui.refreshOutputView()
				return nil
			}
			for _, pattern := range patterns {
				pattern = strings.TrimSpace(pattern)
				if pattern == "" {
					continue
				}
				gui.appendOutput(tabID, pattern+"\n")
			}
			gui.refreshOutputView()
			return nil
		})
	}()

	return nil
}

func (gui *Gui) showDependencyDoctor() error {
	report := django.BuildDependencyReport(gui.project)
	tabID := gui.startCommandOutputTab("Dependency Doctor")
	gui.appendOutput(tabID, report.String())
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)
	return nil
}

func (gui *Gui) createSnapshot(g *gocui.Gui, v *gocui.View) error {
	tabID := gui.startCommandOutputTab("Create Snapshot")
	gui.appendOutput(tabID, "Creating snapshot...\n")
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)

	go func() {
		sm := django.NewSnapshotManager(gui.project)
		snapshot, createErr := sm.CreateSnapshot("")
		gui.g.Update(func(g *gocui.Gui) error {
			gui.resetOutput(tabID, "Create Snapshot")
			if createErr != nil {
				gui.appendOutput(tabID, fmt.Sprintf("Error: %v\n", createErr))
				gui.recordSnapshotActivity("create", "", "", createErr)
			} else {
				gui.appendOutput(tabID, fmt.Sprintf("Snapshot created: %s\n", snapshot.Name))
				gui.appendOutput(tabID, fmt.Sprintf("Created: %s\n", snapshot.Timestamp.Local().Format("2006-01-02 15:04:05")))
				if snapshot.GitBranch != "" {
					if snapshot.GitCommit != "" {
						gui.appendOutput(tabID, fmt.Sprintf("Git: %s (%s)\n", snapshot.GitBranch, shortCommit(snapshot.GitCommit)))
					} else {
						gui.appendOutput(tabID, fmt.Sprintf("Git: %s\n", snapshot.GitBranch))
					}
				}
				gui.recordSnapshotActivity("create", snapshot.ID, snapshot.Name, nil)
			}

			gui.invalidateSnapshotCache()
			if dataView, err := g.View(DataWindow); err == nil {
				gui.renderDataList(dataView)
			}
			gui.refreshOutputView()
			return nil
		})
	}()

	return nil
}

func (gui *Gui) listSnapshots() error {
	snapshots, listErr := gui.loadSnapshots(true)
	tabID := gui.startCommandOutputTab("Snapshots")

	if listErr != nil {
		gui.appendOutput(tabID, fmt.Sprintf("Error: %v\n", listErr))
		gui.recordSnapshotActivity("list", "", "", listErr)
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}
	gui.recordSnapshotActivity("list", "", "", nil)

	if len(snapshots) == 0 {
		gui.appendOutput(tabID, "No snapshots available.\n")
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}

	for i, snapshot := range snapshots {
		gui.appendOutput(tabID, fmt.Sprintf("%2d. %s\n", i+1, snapshot.Name))
		gui.appendOutput(tabID, fmt.Sprintf("    %s\n", snapshot.Timestamp.Local().Format("2006-01-02 15:04:05")))
		if snapshot.GitBranch != "" {
			if snapshot.GitCommit != "" {
				gui.appendOutput(tabID, fmt.Sprintf("    %s (%s)\n", snapshot.GitBranch, shortCommit(snapshot.GitCommit)))
			} else {
				gui.appendOutput(tabID, fmt.Sprintf("    %s\n", snapshot.GitBranch))
			}
		}
	}
	gui.refreshOutputView()

	_ = gui.switchPanel(MainWindow)
	return nil
}

func (gui *Gui) showRestoreMenu() error {
	snapshots, err := gui.loadSnapshots(true)
	if err != nil {
		return gui.showMessage("Error", fmt.Sprintf("Failed to load snapshots: %v", err))
	}
	if len(snapshots) == 0 {
		return gui.showMessage("Restore Snapshot", "No snapshots available.")
	}

	returnWindow := gui.currentWindow
	if returnWindow == "" {
		returnWindow = MainWindow
	}

	gui.isModalOpen = true
	gui.modalType = "restore"
	gui.modalReturnWindow = returnWindow
	gui.modalTitle = "Restore Snapshot"
	gui.projectModalActions = nil
	gui.projectModalIndex = 0
	gui.restoreSnapshots = snapshots
	gui.restoreIndex = 0
	return nil
}

func parseComposeServicesFromYAML(content string) []string {
	inServices := false
	services := make([]string, 0)
	seen := make(map[string]struct{})

	for _, rawLine := range strings.Split(content, "\n") {
		line := rawLine
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !inServices {
			if trimmed == "services:" {
				inServices = true
			}
			continue
		}

		indent := len(line) - len(strings.TrimLeft(line, " "))
		// Leaving the services block.
		if indent == 0 {
			break
		}

		// Top-level service names are typically indented by two spaces.
		if indent == 2 && strings.HasSuffix(trimmed, ":") {
			name := strings.TrimSuffix(trimmed, ":")
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			services = append(services, name)
		}
	}

	sort.Strings(services)
	return services
}

func (gui *Gui) listComposeServices() ([]string, error) {
	if gui.project == nil || gui.project.DockerComposeFile == "" {
		return nil, fmt.Errorf("docker compose file is not configured")
	}

	composeArgs := []string{"compose", "-f", gui.project.DockerComposeFile, "config", "--services"}
	cmd := exec.Command("docker", composeArgs...)
	cmd.Dir = gui.project.RootDir
	output, err := cmd.Output()
	if err == nil {
		services := make([]string, 0)
		seen := make(map[string]struct{})
		for _, line := range strings.Split(string(output), "\n") {
			name := strings.TrimSpace(line)
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			seen[name] = struct{}{}
			services = append(services, name)
		}
		sort.Strings(services)
		if len(services) > 0 {
			return services, nil
		}
	}

	content, readErr := os.ReadFile(gui.project.DockerComposeFile)
	if readErr == nil {
		services := parseComposeServicesFromYAML(string(content))
		if len(services) > 0 {
			return services, nil
		}
	}

	if len(gui.containerStatus) > 0 {
		services := make([]string, 0, len(gui.containerStatus))
		for name := range gui.containerStatus {
			services = append(services, name)
		}
		sort.Strings(services)
		if len(services) > 0 {
			return services, nil
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to discover services: %w", err)
	}
	if readErr != nil {
		return nil, fmt.Errorf("failed to discover services: %w", readErr)
	}
	return nil, fmt.Errorf("no docker services found")
}

func (gui *Gui) openContainerModal(action string) error {
	if gui.project == nil || !gui.project.HasDocker || gui.project.DockerComposeFile == "" {
		return gui.showMessage("Containers", "Docker compose is not configured for this project.")
	}

	returnWindow := gui.currentWindow
	if returnWindow == "" {
		returnWindow = MainWindow
	}

	gui.refreshContainerStatus()
	services, err := gui.listComposeServices()
	if err != nil {
		return gui.showMessage("Containers", fmt.Sprintf("Failed to discover services: %v", err))
	}
	if len(services) == 0 {
		return gui.showMessage("Containers", "No services found in docker compose.")
	}

	gui.isModalOpen = true
	gui.modalType = "containers"
	gui.modalReturnWindow = returnWindow
	gui.modalFields = nil
	gui.modalValues = nil
	gui.modalMessage = ""
	gui.projectModalActions = nil
	gui.projectModalIndex = 0
	gui.restoreSnapshots = nil
	gui.restoreIndex = 0
	gui.containerAction = action
	gui.containerList = services
	gui.containerIndex = 0
	gui.containerSelect = make(map[string]bool, len(services))

	// Smart defaults based on current runtime status.
	for _, service := range services {
		status := strings.ToLower(gui.containerStatus[service])
		if action == "start" {
			gui.containerSelect[service] = status != "running"
		} else {
			gui.containerSelect[service] = status == "running"
		}
	}

	if len(gui.selectedContainerServices()) == 0 && len(services) > 0 {
		gui.containerSelect[services[0]] = true
	}

	if action == "start" {
		gui.modalTitle = "Start Containers"
	} else {
		gui.modalTitle = "Stop Containers"
	}

	return nil
}

func (gui *Gui) selectedContainerServices() []string {
	services := make([]string, 0, len(gui.containerList))
	for _, service := range gui.containerList {
		if gui.containerSelect[service] {
			services = append(services, service)
		}
	}
	return services
}

func (gui *Gui) setAllContainerSelection(selected bool) {
	if gui.containerSelect == nil {
		gui.containerSelect = make(map[string]bool, len(gui.containerList))
	}
	for _, service := range gui.containerList {
		gui.containerSelect[service] = selected
	}
}

func (gui *Gui) toggleContainerSelectionAtCurrent() {
	if len(gui.containerList) == 0 {
		return
	}
	gui.containerIndex = clampSelection(gui.containerIndex, len(gui.containerList))
	service := gui.containerList[gui.containerIndex]
	gui.containerSelect[service] = !gui.containerSelect[service]
}

func (gui *Gui) runContainerSelectionAction() error {
	selected := gui.selectedContainerServices()
	action := gui.containerAction
	if action == "" {
		action = "start"
	}
	if err := gui.closeModal(); err != nil {
		return err
	}

	if len(selected) == 0 {
		return gui.showMessage("Containers", "No services selected.")
	}

	command := "up -d"
	if action == "stop" {
		command = "stop"
	}
	tabID := gui.startCommandOutputTab(tabTitleFromCommand(fmt.Sprintf("docker compose %s", command)))
	gui.appendOutput(tabID, fmt.Sprintf("Running docker compose %s for: %s\n\n", command, strings.Join(selected, ", ")))
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)
	startedAt := time.Now()

	baseArgs := []string{"compose", "-f", gui.project.DockerComposeFile}
	args := append([]string{}, baseArgs...)
	if action == "stop" {
		args = append(args, "stop")
	} else {
		args = append(args, "up", "-d")
	}
	args = append(args, selected...)

	output, runErr := gui.runDockerComposeCommand(tabID, args)
	hadError := runErr != nil
	if runErr != nil && action == "start" {
		conflicts := parseContainerNameConflicts(output)
		if len(conflicts) > 0 {
			gui.appendOutput(tabID, fmt.Sprintf("\nDetected existing containers with conflicting names: %s\n", strings.Join(conflicts, ", ")))
			gui.appendOutput(tabID, "Attempting to start the existing containers directly...\n")

			startFailed := false
			for _, containerName := range conflicts {
				name := strings.TrimPrefix(containerName, "/")
				startArgs := []string{"start", name}
				_, startErr := gui.runDockerCommand(tabID, startArgs)
				if startErr != nil {
					startFailed = true
					gui.appendOutput(tabID, fmt.Sprintf("Failed to start existing container %s: %v\n", name, startErr))
				} else {
					gui.appendOutput(tabID, fmt.Sprintf("Started existing container: %s\n", name))
				}
			}

			if !startFailed {
				gui.appendOutput(tabID, "\nRetrying docker compose up after starting existing containers...\n")
				_, retryErr := gui.runDockerComposeCommand(tabID, args)
				if retryErr != nil {
					hadError = true
					gui.appendOutput(tabID, fmt.Sprintf("Retry failed: %v\n", retryErr))
				} else {
					hadError = false
				}
			}
		}
	}

	gui.refreshContainerStatus()
	if menuView, err := gui.g.View(MenuWindow); err == nil {
		gui.renderProjectList(menuView)
	}

	var actionErr error
	if hadError {
		gui.appendOutput(tabID, "Completed with errors. Check output above.\n")
		actionErr = fmt.Errorf("container %s failed", action)
	} else {
		gui.appendOutput(tabID, "Completed successfully.\n")
	}
	gui.recordContainerAction(action, selected, startedAt, actionErr)
	gui.refreshOutputView()

	return nil
}

func (gui *Gui) runDockerComposeCommand(tabID string, args []string) (string, error) {
	// args should already include: compose -f <file> ...
	gui.appendOutput(tabID, fmt.Sprintf("$ docker %s\n", strings.Join(args, " ")))
	return gui.runDockerCommand(tabID, args)
}

func (gui *Gui) runDockerCommand(tabID string, args []string) (string, error) {
	cmd := exec.Command("docker", args...)
	cmd.Dir = gui.project.RootDir
	prepareCommandForExecution(cmd, false)
	output, err := cmd.CombinedOutput()
	text := string(output)
	if strings.TrimSpace(text) != "" {
		gui.appendOutput(tabID, text)
		if !strings.HasSuffix(text, "\n") {
			gui.appendOutput(tabID, "\n")
		}
	}
	if err != nil {
		gui.appendOutput(tabID, fmt.Sprintf("Error: %v\n", err))
	}
	gui.appendOutput(tabID, "\n")
	return text, err
}

func parseContainerNameConflicts(output string) []string {
	re := regexp.MustCompile(`container name "([^"]+)" is already in use`)
	matches := re.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func resolvePythonBinary() string {
	if pythonPath, err := exec.LookPath("python"); err == nil {
		return pythonPath
	}
	if python3Path, err := exec.LookPath("python3"); err == nil {
		return python3Path
	}
	return "python"
}

func (gui *Gui) startServer(g *gocui.Gui, v *gocui.View) error {
	if gui.serverCmd != nil && gui.serverCmd.Process != nil {
		tabID := gui.startLogsOutputTab("Dev Server")
		gui.appendOutput(tabID, "Server is already running.\n")
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}

	var cmd *exec.Cmd
	if gui.makeTargetExists("runserver") {
		cmd = exec.Command("make", "runserver")
	} else if gui.project.HasDocker && gui.project.DockerService != "" {
		args := []string{"compose"}
		if gui.project.DockerComposeFile != "" {
			args = append(args, "-f", gui.project.DockerComposeFile)
		}
		args = append(args, "exec", "-T", gui.project.DockerService, "python", "manage.py", "runserver", "0.0.0.0:8000")
		cmd = exec.Command("docker", args...)
	} else {
		cmd = exec.Command(resolvePythonBinary(), gui.project.ManagePyPath, "runserver")
	}
	cmd.Dir = gui.project.RootDir
	return gui.runStreamingCommandToLogs("Dev Server", cmd, true, strings.Join(cmd.Args, " "))
}

func (gui *Gui) stopServer(g *gocui.Gui, v *gocui.View) error {
	tabID := gui.startLogsOutputTab("Dev Server")

	if gui.serverCmd == nil || gui.serverCmd.Process == nil {
		gui.appendOutput(tabID, "\nNo running server process.\n")
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}

	if err := gui.serverCmd.Process.Kill(); err != nil {
		gui.appendOutput(tabID, fmt.Sprintf("\nFailed to stop server: %v\n", err))
	} else {
		gui.appendOutput(tabID, "\nServer stop signal sent.\n")
	}
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)
	return nil
}

func (gui *Gui) refresh(g *gocui.Gui, v *gocui.View) error {
	tabID := gui.startCommandOutputTab("Refresh")
	gui.appendOutput(tabID, "Refreshing project metadata...\n")
	gui.refreshOutputView()

	gui.project.Migrations = nil
	gui.project.Models = nil
	for i := range gui.project.Apps {
		gui.project.Apps[i].Models = nil
	}

	gui.project.DiscoverSettings()
	gui.project.DiscoverModels()
	gui.project.DiscoverMigrations()
	gui.loadMakeTargets(true)
	gui.loadProjectTasks(true)
	gui.refreshContainerStatus()
	gui.invalidateSnapshotCache()
	gui.clampSelections()

	if gui.currentModel != "" {
		found := false
		for _, model := range gui.project.Models {
			if model.App == gui.currentApp && model.Name == gui.currentModel {
				found = true
				break
			}
		}
		if found {
			_ = gui.loadAndDisplayRecords()
		} else {
			_ = gui.clearModelView()
		}
	}

	gui.appendOutput(tabID, "Done.\n")
	gui.refreshOutputView()

	return nil
}

func (gui *Gui) refreshContainerStatus() {
	gui.containerStatus = gui.getContainerStatus()
}

func (gui *Gui) getContainerStatus() map[string]string {
	if gui.project == nil || !gui.project.HasDocker || gui.project.DockerComposeFile == "" {
		return map[string]string{}
	}

	cmd := exec.Command("docker", "compose", "-f", gui.project.DockerComposeFile, "ps", "--format", "json")
	cmd.Dir = gui.project.RootDir
	output, err := cmd.Output()
	if err != nil {
		return map[string]string{}
	}

	return parseComposePSOutput(string(output))
}

func (gui *Gui) startContainers(g *gocui.Gui, v *gocui.View) error {
	return gui.openContainerModal("start")
}

func (gui *Gui) stopContainers(g *gocui.Gui, v *gocui.View) error {
	return gui.openContainerModal("stop")
}

func (gui *Gui) databaseLabel() string {
	engine := strings.ToLower(gui.project.Database.Engine)
	switch {
	case strings.Contains(engine, "sqlite"):
		return "SQLite"
	case strings.Contains(engine, "postgres"):
		return "PostgreSQL"
	case strings.Contains(engine, "mysql"):
		return "MySQL"
	case engine == "":
		return "Unknown"
	default:
		return gui.project.Database.Engine
	}
}

func (gui *Gui) handleGlobalQuit(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return gui.closeModal()
	}
	return gui.quit(g, v)
}

func (gui *Gui) migrationSummary() (int, int) {
	total := len(gui.project.Migrations)
	applied := 0
	for _, m := range gui.project.Migrations {
		if m.Applied {
			applied++
		}
	}
	return applied, total
}

// quit exits the application.
func (gui *Gui) quit(g *gocui.Gui, v *gocui.View) error {
	for tabID, writer := range gui.outputInputWriters {
		if writer != nil {
			_ = writer.Close()
		}
		delete(gui.outputInputWriters, tabID)
	}
	if err := gui.saveProjectState(); err != nil {
		log.Printf("warning: failed to save project state: %v", err)
	}
	return gocui.ErrQuit
}

// viewAllModels displays all discovered models in the output panel.
func (gui *Gui) viewAllModels(g *gocui.Gui, v *gocui.View) error {
	tabID := gui.startCommandOutputTab("All Models")

	models := gui.sortedModels()
	if len(models) == 0 {
		gui.appendOutput(tabID, "No models found.\n")
		gui.refreshOutputView()
		return nil
	}

	for _, model := range models {
		gui.appendOutput(tabID, fmt.Sprintf("%s.%s (%d fields)\n", model.App, model.Name, model.Fields))
	}
	gui.refreshOutputView()

	return gui.switchPanel(MainWindow)
}

func shortCommit(commit string) string {
	if len(commit) <= 7 {
		return commit
	}
	return commit[:7]
}

// Run launches the GUI for a discovered project using default build metadata.
func Run(project *django.Project) error {
	return RunWithVersion(project, "dev")
}

// RunWithVersion launches the GUI with explicit app version metadata.
func RunWithVersion(project *django.Project, appVersion string) error {
	gui, err := NewGui(project)
	if err != nil {
		log.Panicln(err)
	}
	gui.setAppVersion(appVersion)
	return gui.Run()
}
