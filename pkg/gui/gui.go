package gui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/awesome-gocui/gocui"
	"github.com/williamblackie/lazydjango/pkg/config"
	"github.com/williamblackie/lazydjango/pkg/django"
)

// Gui represents the main TUI state.
type Gui struct {
	g       *gocui.Gui
	config  *config.AppConfig
	project *django.Project

	// Global state
	currentWindow      string
	mainTitle          string
	commandHistory     []string
	serverCmd          *exec.Cmd
	outputTab          string
	outputCommandTitle string
	outputCommandText  string
	outputLogsTitle    string
	outputLogsText     string

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
	modalType           string // "add", "edit", "delete", "restore", "containers", "projectActions"
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
)

var panelOrder = []string{MenuWindow, ListWindow, DataWindow, MainWindow}

type projectAction struct {
	label      string
	command    string
	internal   string
	makeTarget string
}

type makeTarget struct {
	name        string
	description string
	section     string
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

func (gui *Gui) projectActions() []projectAction {
	runLabel := "Run dev server"
	if gui.makeTargetExists("runserver") {
		runLabel = "Run dev server (make runserver)"
	}

	actions := []projectAction{
		{label: runLabel, internal: "runserver"},
		{label: "Stop dev server", internal: "stopserver"},
	}

	if gui.project != nil && gui.project.HasDocker && gui.project.DockerComposeFile != "" {
		actions = append(actions, projectAction{label: "Containers...", internal: "opencontainers"})
	}

	if len(gui.projectMakeActions()) > 0 {
		actions = append(actions, projectAction{label: "Make Tasks...", internal: "openmaketasks"})
	}

	actions = append(actions,
		projectAction{label: "Migrations...", internal: "openmigrations"},
		projectAction{label: "Tools...", internal: "opentools"},
	)

	return actions
}

func (gui *Gui) projectContainerActions() []projectAction {
	return []projectAction{
		{label: "Start selected services...", internal: "startcontainers"},
		{label: "Stop selected services...", internal: "stopcontainers"},
		{label: "Refresh container status", internal: "refresh"},
	}
}

func (gui *Gui) projectMigrationActions() []projectAction {
	if gui.makeTargetExists("showmigrations") || gui.makeTargetExists("migrations") || gui.makeTargetExists("migrate") {
		actions := make([]projectAction, 0, 3)
		if t, ok := gui.makeTargetByName("showmigrations"); ok {
			label := "make showmigrations"
			if t.description != "" {
				label = fmt.Sprintf("make showmigrations - %s", t.description)
			}
			actions = append(actions, projectAction{label: label, makeTarget: "showmigrations"})
		}
		if t, ok := gui.makeTargetByName("migrations"); ok {
			label := "make migrations"
			if t.description != "" {
				label = fmt.Sprintf("make migrations - %s", t.description)
			}
			actions = append(actions, projectAction{label: label, makeTarget: "migrations"})
		}
		if t, ok := gui.makeTargetByName("migrate"); ok {
			label := "make migrate"
			if t.description != "" {
				label = fmt.Sprintf("make migrate - %s", t.description)
			}
			actions = append(actions, projectAction{label: label, makeTarget: "migrate"})
		}
		if len(actions) > 0 {
			return actions
		}
	}

	applied, total := gui.migrationSummary()
	return []projectAction{
		{label: fmt.Sprintf("Show migrations (%d/%d applied)", applied, total), command: "showmigrations --list"},
		{label: "Make migrations", command: "makemigrations"},
		{label: "Apply migrations", command: "migrate"},
	}
}

func (gui *Gui) projectToolActions() []projectAction {
	actions := make([]projectAction, 0, 12)

	if t, ok := gui.makeTargetByName("test"); ok {
		label := "make test"
		if t.description != "" {
			label = fmt.Sprintf("make test - %s", t.description)
		}
		actions = append(actions, projectAction{label: label, makeTarget: "test"})
	}
	if t, ok := gui.makeTargetByName("test-parallel"); ok {
		label := "make test-parallel"
		if t.description != "" {
			label = fmt.Sprintf("make test-parallel - %s", t.description)
		}
		actions = append(actions, projectAction{label: label, makeTarget: "test-parallel"})
	}
	if t, ok := gui.makeTargetByName("testmon"); ok {
		label := "make testmon"
		if t.description != "" {
			label = fmt.Sprintf("make testmon - %s", t.description)
		}
		actions = append(actions, projectAction{label: label, makeTarget: "testmon"})
	}
	if t, ok := gui.makeTargetByName("shell"); ok {
		label := "make shell"
		if t.description != "" {
			label = fmt.Sprintf("make shell - %s", t.description)
		}
		actions = append(actions, projectAction{label: label, makeTarget: "shell"})
	}
	if t, ok := gui.makeTargetByName("dbshell"); ok {
		label := "make dbshell"
		if t.description != "" {
			label = fmt.Sprintf("make dbshell - %s", t.description)
		}
		actions = append(actions, projectAction{label: label, makeTarget: "dbshell"})
	}
	if t, ok := gui.makeTargetByName("lint"); ok {
		label := "make lint"
		if t.description != "" {
			label = fmt.Sprintf("make lint - %s", t.description)
		}
		actions = append(actions, projectAction{label: label, makeTarget: "lint"})
	}
	if t, ok := gui.makeTargetByName("format"); ok {
		label := "make format"
		if t.description != "" {
			label = fmt.Sprintf("make format - %s", t.description)
		}
		actions = append(actions, projectAction{label: label, makeTarget: "format"})
	}

	actions = append(actions,
		projectAction{label: "Django check", command: "check"},
		projectAction{label: "Show URL patterns", internal: "showurls"},
		projectAction{label: "Dependency doctor", internal: "doctor"},
		projectAction{label: "Refresh project data", internal: "refresh"},
	)

	return actions
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

	gui.setSelectionFor(window, idx)
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

	targetMap := make(map[string]makeTarget, len(targets))
	for _, t := range targets {
		targetMap[t.name] = t
	}

	priority := []string{
		"up", "up-all", "down", "restart", "restart-all",
		"runserver", "shell", "dbshell",
		"migrations", "migrate", "migrate-site", "showmigrations", "loaddata",
		"test", "test-parallel", "testmon", "testmon-all", "test-cov", "test-reset-db",
		"lint", "format",
		"webpack", "watch", "storybook",
		"pg-dump", "pg-restore",
	}

	actions := make([]projectAction, 0)
	seen := make(map[string]struct{}, len(priority))

	for _, name := range priority {
		t, ok := targetMap[name]
		if !ok {
			continue
		}
		label := fmt.Sprintf("make %s", t.name)
		if t.description != "" {
			label = fmt.Sprintf("make %s - %s", t.name, t.description)
		}
		actions = append(actions, projectAction{
			label:      label,
			makeTarget: t.name,
		})
		seen[t.name] = struct{}{}
	}

	return actions
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
		currentWindow:      MenuWindow,
		mainTitle:          "Output",
		outputTab:          OutputTabCommand,
		outputCommandTitle: "Command",
		outputLogsTitle:    "Logs",
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
	gui.refreshContainerStatus()
	gui.clampSelections()

	return gui, nil
}

// Run starts the GUI.
func (gui *Gui) Run() error {
	defer gui.g.Close()
	return gui.g.MainLoop()
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
	gui.renderProjectList(menuView)

	listView, err := g.SetView(ListWindow, 0, panel1Bottom+1, leftWidth, panel2Bottom, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	listView.Wrap = false
	listView.Highlight = false
	listView.Title = gui.panelTitle(ListWindow, "Database")
	gui.renderDatabaseList(listView)

	dataView, err := g.SetView(DataWindow, 0, panel2Bottom+1, leftWidth, contentBottom, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	dataView.Wrap = false
	dataView.Highlight = false
	dataView.Title = gui.panelTitle(DataWindow, "Data")
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

	optionsView, err := g.SetView(OptionsWindow, 0, maxY-2, maxX-1, maxY-1, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}
	optionsView.Frame = false
	gui.updateOptionsView(optionsView)

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
		gui.renderModal(modalView)
		gui.setModalKeybindings()

		if _, err := g.SetCurrentView(ModalWindow); err != nil {
			return err
		}
		g.SetViewOnTop(ModalWindow)
		return nil
	}

	g.DeleteView(ModalWindow)
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

func (gui *Gui) outputTitleForTab(tab string) string {
	if tab == OutputTabLogs {
		if strings.TrimSpace(gui.outputLogsTitle) == "" {
			return "Logs"
		}
		return gui.outputLogsTitle
	}
	if strings.TrimSpace(gui.outputCommandTitle) == "" {
		return "Command"
	}
	return gui.outputCommandTitle
}

func (gui *Gui) outputTextForTab(tab string) string {
	if tab == OutputTabLogs {
		return gui.outputLogsText
	}
	return gui.outputCommandText
}

func (gui *Gui) setOutputTextForTab(tab, text string) {
	if tab == OutputTabLogs {
		gui.outputLogsText = text
		return
	}
	gui.outputCommandText = text
}

func (gui *Gui) setOutputTitleForTab(tab, title string) {
	if tab == OutputTabLogs {
		gui.outputLogsTitle = title
		return
	}
	gui.outputCommandTitle = title
}

func (gui *Gui) appendOutput(tab, text string) {
	current := gui.outputTextForTab(tab)
	gui.setOutputTextForTab(tab, current+text)
}

func (gui *Gui) resetOutput(tab, title string) {
	if strings.TrimSpace(title) == "" {
		title = "Output"
	}
	gui.setOutputTitleForTab(tab, title)
	gui.setOutputTextForTab(tab, "")
}

func (gui *Gui) switchOutputTab(tab string) {
	if tab != OutputTabCommand && tab != OutputTabLogs {
		return
	}
	gui.outputTab = tab
	if gui.currentModel == "" {
		gui.refreshOutputView()
	}
}

func (gui *Gui) currentOutputTabLabel() string {
	if gui.outputTab == OutputTabLogs {
		return "Logs"
	}
	return "Command"
}

func (gui *Gui) renderOutputView(v *gocui.View) {
	if gui.outputTab != OutputTabLogs {
		gui.outputTab = OutputTabCommand
	}

	v.Clear()
	v.Autoscroll = gui.outputTab == OutputTabLogs
	title := fmt.Sprintf("%s [%s]", gui.outputTitleForTab(gui.outputTab), gui.currentOutputTabLabel())
	gui.mainTitle = title
	v.Title = gui.panelTitle(MainWindow, title)

	text := gui.outputTextForTab(gui.outputTab)
	if strings.TrimSpace(text) == "" {
		if gui.outputTab == OutputTabLogs {
			fmt.Fprintln(v, "No logs yet.")
			fmt.Fprintln(v)
			fmt.Fprintln(v, "Long-running processes (dev server, make watch/up, logs) appear here.")
		} else {
			fmt.Fprintln(v, "LazyDjango")
			fmt.Fprintln(v)
			fmt.Fprintln(v, "No command output yet.")
			fmt.Fprintln(v, "Use Project actions or Make Tasks to run workflows.")
			fmt.Fprintln(v)
			fmt.Fprintln(v, "Keys: o toggle tab, [ previous tab, ] next tab, Ctrl+L clear tab.")
		}
		return
	}

	fmt.Fprint(v, text)
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
			fmt.Fprint(v, "Modal | j/k:move  Enter:run action  Esc/q:cancel")
		default:
			fmt.Fprint(v, "Modal | j/k:field  Enter/e:edit  Space:toggle bool  Ctrl+S:save  Esc/q:cancel")
		}
		return
	}

	global := "1-4/h/l/Tab:focus  j/k:move  Enter:run  o/[ ]:output tabs  Ctrl+L:clear tab  r:refresh  q:quit"
	context := ""

	switch gui.currentWindow {
	case MenuWindow:
		context = "Project | Enter opens/executes action, s:stop server, u/D:container selector, Make Tasks for project workflows"
	case ListWindow:
		context = "Database | Enter opens selected model data"
	case DataWindow:
		context = "Data | Enter action, c:create, L:list, R:restore"
	case MainWindow:
		if gui.currentModel != "" {
			context = "Output(model) | j/k/J/K:record  n/p:page  a/e/d:CRUD  Esc:close model"
		} else {
			context = fmt.Sprintf("Output(%s) | command results and logs are split by tab", gui.currentOutputTabLabel())
		}
	default:
		context = ""
	}

	fmt.Fprintf(v, "%s\n%s", global, context)
}

func (gui *Gui) setKeybindings() error {
	if err := gui.g.SetKeybinding("", gocui.KeyCtrlC, gocui.ModNone, gui.quit); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'q', gocui.ModNone, gui.handleGlobalQuit); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", '1', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.switchPanel(MenuWindow)
	}); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", '2', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.switchPanel(ListWindow)
	}); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", '3', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.switchPanel(DataWindow)
	}); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", '4', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.switchPanel(MainWindow)
	}); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", gocui.KeyTab, gocui.ModNone, gui.focusNextPanel); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyBacktab, gocui.ModNone, gui.focusPrevPanel); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'h', gocui.ModNone, gui.focusPrevPanel); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'l', gocui.ModNone, gui.focusNextPanel); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyArrowLeft, gocui.ModNone, gui.focusPrevPanel); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyArrowRight, gocui.ModNone, gui.focusNextPanel); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", 'j', gocui.ModNone, gui.cursorDown); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyArrowDown, gocui.ModNone, gui.cursorDown); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'k', gocui.ModNone, gui.cursorUp); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyArrowUp, gocui.ModNone, gui.cursorUp); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", gocui.KeyEnter, gocui.ModNone, gui.executeCommand); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyEsc, gocui.ModNone, gui.handleEsc); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", 'r', gocui.ModNone, gui.refresh); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'n', gocui.ModNone, gui.nextPage); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'p', gocui.ModNone, gui.prevPage); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", 'a', gocui.ModNone, gui.handleAddKey); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'e', gocui.ModNone, gui.handleEditKey); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'd', gocui.ModNone, gui.handleDeleteKey); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'J', gocui.ModNone, gui.nextRecord); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'K', gocui.ModNone, gui.prevRecord); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", 'c', gocui.ModNone, gui.createSnapshot); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'L', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.listSnapshots()
	}); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'R', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.showRestoreMenu()
	}); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", 'u', gocui.ModNone, gui.startContainers); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'D', gocui.ModNone, gui.stopContainers); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 's', gocui.ModNone, gui.stopServer); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'o', gocui.ModNone, gui.toggleOutputTab); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", '[', gocui.ModNone, gui.prevOutputTab); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", ']', gocui.ModNone, gui.nextOutputTab); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyCtrlL, gocui.ModNone, gui.clearCurrentOutputTab); err != nil {
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
	gui.currentWindow = name
	gui.clampSelections()
	if gui.isModalOpen {
		return nil
	}
	if _, err := gui.g.SetCurrentView(name); err != nil && err != gocui.ErrUnknownView {
		return err
	}
	return nil
}

func (gui *Gui) toggleOutputTab(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}
	if gui.outputTab == OutputTabLogs {
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
	if gui.outputTab == OutputTabCommand {
		gui.switchOutputTab(OutputTabLogs)
	} else {
		gui.switchOutputTab(OutputTabCommand)
	}
	return gui.switchPanel(MainWindow)
}

func (gui *Gui) prevOutputTab(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}
	if gui.outputTab == OutputTabLogs {
		gui.switchOutputTab(OutputTabCommand)
	} else {
		gui.switchOutputTab(OutputTabLogs)
	}
	return gui.switchPanel(MainWindow)
}

func (gui *Gui) clearCurrentOutputTab(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen || gui.currentModel != "" {
		return nil
	}
	gui.setOutputTextForTab(gui.outputTab, "")
	gui.refreshOutputView()
	return nil
}

func (gui *Gui) cursorDown(g *gocui.Gui, v *gocui.View) error {
	if gui.isModalOpen {
		return nil
	}

	if gui.currentWindow == MainWindow {
		if gui.currentModel != "" && len(gui.currentRecords) > 0 {
			return gui.nextRecord(g, v)
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
	return nil
}

func (gui *Gui) runProjectAction(action projectAction) error {
	if action.makeTarget != "" {
		return gui.runMakeTarget(action.label, action.makeTarget)
	}

	if action.command != "" {
		args := strings.Fields(action.command)
		return gui.runManageCommand(action.label, args...)
	}

	switch action.internal {
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
	case "openmaketasks":
		return gui.openProjectActionsModal("Make Tasks", gui.projectMakeActions())
	case "openmigrations":
		return gui.openProjectActionsModal("Migration Actions", gui.projectMigrationActions())
	case "opentools":
		return gui.openProjectActionsModal("Tool Actions", gui.projectToolActions())
	default:
		return nil
	}
}

func (gui *Gui) runManageCommand(title string, args ...string) error {
	gui.resetOutput(OutputTabCommand, title)
	gui.appendOutput(OutputTabCommand, fmt.Sprintf("$ python manage.py %s\n\n", strings.Join(args, " ")))
	gui.switchOutputTab(OutputTabCommand)
	_ = gui.switchPanel(MainWindow)

	go func() {
		output, runErr := gui.project.RunCommand(args...)
		gui.g.Update(func(g *gocui.Gui) error {
			if runErr != nil {
				gui.appendOutput(OutputTabCommand, fmt.Sprintf("Error: %v\n\n", runErr))
			}
			gui.appendOutput(OutputTabCommand, output)
			gui.refreshOutputView()

			if len(args) > 0 && (args[0] == "makemigrations" || args[0] == "migrate") {
				gui.project.Migrations = nil
				gui.project.DiscoverMigrations()
			}

			gui.commandHistory = append(gui.commandHistory, strings.Join(args, " "))
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

func (gui *Gui) runStreamingCommandToLogs(title string, cmd *exec.Cmd, trackAsServer bool) error {
	gui.resetOutput(OutputTabLogs, title)
	gui.appendOutput(OutputTabLogs, fmt.Sprintf("$ %s\n\n", strings.Join(cmd.Args, " ")))
	gui.switchOutputTab(OutputTabLogs)
	_ = gui.switchPanel(MainWindow)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		gui.appendOutput(OutputTabLogs, fmt.Sprintf("Failed to open stdout: %v\n", err))
		gui.refreshOutputView()
		return nil
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		gui.appendOutput(OutputTabLogs, fmt.Sprintf("Failed to open stderr: %v\n", err))
		gui.refreshOutputView()
		return nil
	}

	if err := cmd.Start(); err != nil {
		gui.appendOutput(OutputTabLogs, fmt.Sprintf("Failed to start command: %v\n", err))
		gui.refreshOutputView()
		return nil
	}

	if trackAsServer {
		gui.serverCmd = cmd
	}

	lineCh := make(chan string, 256)
	var wg sync.WaitGroup

	readStream := func(reader io.ReadCloser) {
		defer wg.Done()
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			lineCh <- scanner.Text() + "\n"
		}
		if scanErr := scanner.Err(); scanErr != nil {
			lineCh <- fmt.Sprintf("[stream error: %v]\n", scanErr)
		}
	}

	wg.Add(2)
	go readStream(stdout)
	go readStream(stderr)
	go func() {
		wg.Wait()
		close(lineCh)
	}()

	go func() {
		var captured strings.Builder
		for line := range lineCh {
			captured.WriteString(line)
			lineCopy := line
			gui.g.Update(func(g *gocui.Gui) error {
				gui.appendOutput(OutputTabLogs, lineCopy)
				gui.refreshOutputView()
				return nil
			})
		}

		waitErr := cmd.Wait()
		gui.g.Update(func(g *gocui.Gui) error {
			if waitErr != nil {
				gui.appendOutput(OutputTabLogs, fmt.Sprintf("\nProcess exited with error: %v\n", waitErr))
			} else {
				gui.appendOutput(OutputTabLogs, "\nProcess exited.\n")
			}
			if waitErr != nil && strings.Contains(strings.ToLower(captured.String()), "is not running") {
				gui.appendOutput(OutputTabLogs, "Hint: start the required services first (Project -> Containers..., or press 'u').\n")
			}

			if trackAsServer {
				gui.serverCmd = nil
				if menuView, err := g.View(MenuWindow); err == nil {
					gui.renderProjectList(menuView)
				}
			}
			gui.refreshOutputView()
			return nil
		})
	}()

	return nil
}

func (gui *Gui) runMakeTarget(title, target string) error {
	if target == "runserver" {
		return gui.startServer(gui.g, nil)
	}

	cmd := exec.Command("make", target)
	cmd.Dir = gui.project.RootDir

	if isLongRunningMakeTarget(target) {
		return gui.runStreamingCommandToLogs(title, cmd, false)
	}

	gui.resetOutput(OutputTabCommand, title)
	gui.appendOutput(OutputTabCommand, fmt.Sprintf("$ make %s\n\n", target))
	gui.switchOutputTab(OutputTabCommand)
	_ = gui.switchPanel(MainWindow)

	go func() {
		output, runErr := cmd.CombinedOutput()

		gui.g.Update(func(g *gocui.Gui) error {
			if runErr != nil {
				gui.appendOutput(OutputTabCommand, fmt.Sprintf("Error: %v\n\n", runErr))
			}
			gui.appendOutput(OutputTabCommand, string(output))
			gui.refreshOutputView()

			switch target {
			case "migrations", "migrate", "migrate-site", "showmigrations":
				gui.project.Migrations = nil
				gui.project.DiscoverMigrations()
			case "up", "up-all", "down", "restart", "restart-all":
				gui.refreshContainerStatus()
			}

			gui.commandHistory = append(gui.commandHistory, fmt.Sprintf("make %s", target))
			return nil
		})
	}()

	return nil
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
	gui.currentWindow = MainWindow

	if _, err := gui.g.SetCurrentView(MainWindow); err != nil && err != gocui.ErrUnknownView {
		return err
	}

	return gui.loadAndDisplayRecords()
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
		return gui.editModalField()
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
	gui.resetOutput(OutputTabCommand, "URL Patterns")
	gui.appendOutput(OutputTabCommand, "Loading URL patterns...\n")
	gui.switchOutputTab(OutputTabCommand)
	_ = gui.switchPanel(MainWindow)

	go func() {
		patterns, runErr := gui.project.GetURLPatterns()
		gui.g.Update(func(g *gocui.Gui) error {
			gui.resetOutput(OutputTabCommand, "URL Patterns")
			if runErr != nil {
				gui.appendOutput(OutputTabCommand, fmt.Sprintf("Error: %v\n", runErr))
				gui.refreshOutputView()
				return nil
			}
			for _, pattern := range patterns {
				pattern = strings.TrimSpace(pattern)
				if pattern == "" {
					continue
				}
				gui.appendOutput(OutputTabCommand, pattern+"\n")
			}
			gui.refreshOutputView()
			return nil
		})
	}()

	return nil
}

func (gui *Gui) showDependencyDoctor() error {
	report := django.BuildDependencyReport(gui.project)
	gui.resetOutput(OutputTabCommand, "Dependency Doctor")
	gui.appendOutput(OutputTabCommand, report.String())
	gui.switchOutputTab(OutputTabCommand)
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)
	return nil
}

func (gui *Gui) createSnapshot(g *gocui.Gui, v *gocui.View) error {
	gui.resetOutput(OutputTabCommand, "Create Snapshot")
	gui.appendOutput(OutputTabCommand, "Creating snapshot...\n")
	gui.switchOutputTab(OutputTabCommand)
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)

	go func() {
		sm := django.NewSnapshotManager(gui.project)
		snapshot, createErr := sm.CreateSnapshot("")
		gui.g.Update(func(g *gocui.Gui) error {
			gui.resetOutput(OutputTabCommand, "Create Snapshot")
			if createErr != nil {
				gui.appendOutput(OutputTabCommand, fmt.Sprintf("Error: %v\n", createErr))
			} else {
				gui.appendOutput(OutputTabCommand, fmt.Sprintf("Snapshot created: %s\n", snapshot.Name))
				gui.appendOutput(OutputTabCommand, fmt.Sprintf("Created: %s\n", snapshot.Timestamp.Local().Format("2006-01-02 15:04:05")))
				if snapshot.GitBranch != "" {
					if snapshot.GitCommit != "" {
						gui.appendOutput(OutputTabCommand, fmt.Sprintf("Git: %s (%s)\n", snapshot.GitBranch, shortCommit(snapshot.GitCommit)))
					} else {
						gui.appendOutput(OutputTabCommand, fmt.Sprintf("Git: %s\n", snapshot.GitBranch))
					}
				}
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
	gui.resetOutput(OutputTabCommand, "Snapshots")
	gui.switchOutputTab(OutputTabCommand)

	if listErr != nil {
		gui.appendOutput(OutputTabCommand, fmt.Sprintf("Error: %v\n", listErr))
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}

	if len(snapshots) == 0 {
		gui.appendOutput(OutputTabCommand, "No snapshots available.\n")
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}

	for i, snapshot := range snapshots {
		gui.appendOutput(OutputTabCommand, fmt.Sprintf("%2d. %s\n", i+1, snapshot.Name))
		gui.appendOutput(OutputTabCommand, fmt.Sprintf("    %s\n", snapshot.Timestamp.Local().Format("2006-01-02 15:04:05")))
		if snapshot.GitBranch != "" {
			if snapshot.GitCommit != "" {
				gui.appendOutput(OutputTabCommand, fmt.Sprintf("    %s (%s)\n", snapshot.GitBranch, shortCommit(snapshot.GitCommit)))
			} else {
				gui.appendOutput(OutputTabCommand, fmt.Sprintf("    %s\n", snapshot.GitBranch))
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

	title := "Start Containers"
	command := "up -d"
	if action == "stop" {
		title = "Stop Containers"
		command = "stop"
	}
	gui.resetOutput(OutputTabCommand, title)
	gui.appendOutput(OutputTabCommand, fmt.Sprintf("Running docker compose %s for: %s\n\n", command, strings.Join(selected, ", ")))
	gui.switchOutputTab(OutputTabCommand)
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)

	baseArgs := []string{"compose", "-f", gui.project.DockerComposeFile}
	var hadError bool

	for _, service := range selected {
		args := append([]string{}, baseArgs...)
		if action == "stop" {
			args = append(args, "stop", service)
		} else {
			args = append(args, "up", "-d", service)
		}

		cmd := exec.Command("docker", args...)
		cmd.Dir = gui.project.RootDir
		output, runErr := cmd.CombinedOutput()
		gui.appendOutput(OutputTabCommand, fmt.Sprintf("$ docker %s\n", strings.Join(args, " ")))
		if len(output) > 0 {
			gui.appendOutput(OutputTabCommand, string(output))
			if !strings.HasSuffix(string(output), "\n") {
				gui.appendOutput(OutputTabCommand, "\n")
			}
		}
		if runErr != nil {
			hadError = true
			gui.appendOutput(OutputTabCommand, fmt.Sprintf("Error for %s: %v\n\n", service, runErr))
		} else {
			gui.appendOutput(OutputTabCommand, fmt.Sprintf("OK: %s\n\n", service))
		}
	}

	gui.refreshContainerStatus()
	if menuView, err := gui.g.View(MenuWindow); err == nil {
		gui.renderProjectList(menuView)
	}

	if hadError {
		gui.appendOutput(OutputTabCommand, "Completed with errors. Check output above.\n")
	} else {
		gui.appendOutput(OutputTabCommand, "Completed successfully.\n")
	}
	gui.refreshOutputView()

	return nil
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
		gui.resetOutput(OutputTabLogs, "Dev Server")
		gui.appendOutput(OutputTabLogs, "Server is already running.\n")
		gui.switchOutputTab(OutputTabLogs)
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
	return gui.runStreamingCommandToLogs("Dev Server", cmd, true)
}

func (gui *Gui) stopServer(g *gocui.Gui, v *gocui.View) error {
	gui.switchOutputTab(OutputTabLogs)
	gui.setOutputTitleForTab(OutputTabLogs, "Dev Server")

	if gui.serverCmd == nil || gui.serverCmd.Process == nil {
		gui.appendOutput(OutputTabLogs, "\nNo running server process.\n")
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}

	if err := gui.serverCmd.Process.Kill(); err != nil {
		gui.appendOutput(OutputTabLogs, fmt.Sprintf("\nFailed to stop server: %v\n", err))
	} else {
		gui.appendOutput(OutputTabLogs, "\nServer stop signal sent.\n")
	}
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)
	return nil
}

func (gui *Gui) refresh(g *gocui.Gui, v *gocui.View) error {
	gui.resetOutput(OutputTabCommand, "Refresh")
	gui.appendOutput(OutputTabCommand, "Refreshing project metadata...\n")
	gui.switchOutputTab(OutputTabCommand)
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

	gui.appendOutput(OutputTabCommand, "Done.\n")
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
	return gocui.ErrQuit
}

// viewAllModels displays all discovered models in the output panel.
func (gui *Gui) viewAllModels(g *gocui.Gui, v *gocui.View) error {
	gui.resetOutput(OutputTabCommand, "All Models")
	gui.switchOutputTab(OutputTabCommand)

	models := gui.sortedModels()
	if len(models) == 0 {
		gui.appendOutput(OutputTabCommand, "No models found.\n")
		gui.refreshOutputView()
		return nil
	}

	for _, model := range models {
		gui.appendOutput(OutputTabCommand, fmt.Sprintf("%s.%s (%d fields)\n", model.App, model.Name, model.Fields))
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

// Run launches the GUI for a discovered project.
func Run(project *django.Project) error {
	gui, err := NewGui(project)
	if err != nil {
		log.Panicln(err)
	}
	return gui.Run()
}
