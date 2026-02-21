package gui

import (
	"bufio"
	"context"
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
	"time"

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

	// Global state
	currentWindow       string
	mainTitle           string
	commandHistory      []string
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

type outputTabState struct {
	id         string
	route      string
	title      string
	text       string
	autoscroll bool
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
	actions := []projectAction{
		{label: "Run dev server", internal: "runserver"},
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
	actions := make([]projectAction, 0, 4)
	for _, target := range []string{"showmigrations", "migrations", "migrate", "migrate-site"} {
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
		projectAction{label: "Refresh project data", internal: "refresh"},
	}
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
		"up", "up-all", "down", "restart", "restart-all", "loaddata",
		"shell", "dbshell",
		"test", "test-parallel", "testmon", "testmon-all", "test-cov", "test-reset-db",
		"lint", "format",
		"webpack", "watch", "storybook",
		"pg-dump", "pg-restore",
	}

	actions := make([]projectAction, 0)

	for _, name := range priority {
		t, ok := targetMap[name]
		if !ok {
			continue
		}
		actions = append(actions, projectAction{
			label:      makeActionLabel(t),
			makeTarget: t.name,
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
		g:             g,
		config:        config.GetDefaultConfig(),
		project:       project,
		currentWindow: MenuWindow,
		mainTitle:     "Output",
		appVersion:    "dev",
		outputTabs:    make(map[string]*outputTabState),
		outputOrder:   make([]string, 0),
		outputRoutes:  make(map[string]string),
		pageSize:      20,
		currentPage:   1,
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
	gui.outputTabs[id] = &outputTabState{
		id:         id,
		route:      route,
		title:      title,
		autoscroll: autoscroll,
	}
	gui.outputOrder = append(gui.outputOrder, id)
	gui.outputRoutes[route] = id
	gui.outputTab = id

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
}

func (gui *Gui) switchOutputTab(tab string) {
	id := gui.resolveOutputTabID(tab, false)
	if id == "" {
		return
	}
	gui.outputTab = id
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
	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		v.Autoscroll = false
		gui.mainTitle = "Output"
		v.Title = gui.panelTitle(MainWindow, "Output")
		fmt.Fprintln(v, "LazyDjango")
		fmt.Fprintln(v)
		fmt.Fprintln(v, "No output tabs yet.")
		fmt.Fprintln(v, "Run any project/data command to create a new output tab.")
		fmt.Fprintln(v)
		fmt.Fprintln(v, "Keys: [ previous, ] next, o toggle cmd/log, x close tab, Ctrl+L clear tab.")
		return
	}

	tab := gui.outputTabs[tabID]
	v.Autoscroll = tab.autoscroll
	totalTabs := len(gui.outputOrder)
	tabIndex := gui.outputTabPosition(tabID) + 1
	title := fmt.Sprintf("%s [%s %d/%d]", gui.outputTitleForTab(tabID), outputRouteLabel(tab.route), tabIndex, totalTabs)
	gui.mainTitle = title
	v.Title = gui.panelTitle(MainWindow, title)

	text := gui.outputTextForTab(tabID)
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(v, "No output in this tab yet.")
		fmt.Fprintln(v)
		fmt.Fprintln(v, "Each command creates a new tab.")
		fmt.Fprintln(v, "Use [ and ] to switch tabs, x to close, Ctrl+L to clear.")
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
	fmt.Fprintf(v, "update: %s\n", gui.updateStatusLine())

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

	global := "1-4/h/l/Tab:focus  j/k:move  Enter:run  o/[ ]:output tabs  x:close tab  Ctrl+L:clear tab  r:refresh  U:update  q:quit"
	context := ""

	switch gui.currentWindow {
	case MenuWindow:
		context = "Project | Enter opens/executes action, s:stop server, u/D:container selector, U:update details"
	case ListWindow:
		context = "Database | Enter opens selected model data"
	case DataWindow:
		context = "Data | Enter action, c:create, L:list, R:restore"
	case MainWindow:
		if gui.currentModel != "" {
			context = "Output(model) | j/k/J/K:record  n/p:page  a/e/d:CRUD  Esc:close model"
		} else {
			context = fmt.Sprintf("Output(%s) | each command creates a new tab", gui.currentOutputTabLabel())
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
	if err := gui.g.SetKeybinding("", 'U', gocui.ModNone, gui.showUpdateInfo); err != nil {
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
	if err := gui.g.SetKeybinding("", 'x', gocui.ModNone, gui.closeCurrentOutputTab); err != nil {
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
		gui.refreshOutputView()
		return gui.switchPanel(MainWindow)
	}
	idx := gui.outputTabPosition(currentID)
	if idx < 0 {
		gui.outputTab = gui.outputOrder[0]
		gui.refreshOutputView()
		return gui.switchPanel(MainWindow)
	}
	nextIdx := (idx + 1) % len(gui.outputOrder)
	gui.outputTab = gui.outputOrder[nextIdx]
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
		gui.refreshOutputView()
		return gui.switchPanel(MainWindow)
	}
	idx := gui.outputTabPosition(currentID)
	if idx < 0 {
		gui.outputTab = gui.outputOrder[len(gui.outputOrder)-1]
		gui.refreshOutputView()
		return gui.switchPanel(MainWindow)
	}
	prevIdx := idx - 1
	if prevIdx < 0 {
		prevIdx = len(gui.outputOrder) - 1
	}
	gui.outputTab = gui.outputOrder[prevIdx]
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
		gui.refreshOutputView()
		return nil
	}

	if idx >= len(gui.outputOrder) {
		idx = len(gui.outputOrder) - 1
	}
	gui.outputTab = gui.outputOrder[idx]
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
	tabID := gui.startCommandOutputTab(title)
	gui.appendOutput(tabID, fmt.Sprintf("$ python manage.py %s\n\n", strings.Join(args, " ")))
	_ = gui.switchPanel(MainWindow)

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
	tabID := gui.startLogsOutputTab(title)
	gui.appendOutput(tabID, fmt.Sprintf("$ %s\n\n", strings.Join(cmd.Args, " ")))
	_ = gui.switchPanel(MainWindow)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		gui.appendOutput(tabID, fmt.Sprintf("Failed to open stdout: %v\n", err))
		gui.refreshOutputView()
		return nil
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		gui.appendOutput(tabID, fmt.Sprintf("Failed to open stderr: %v\n", err))
		gui.refreshOutputView()
		return nil
	}

	if err := cmd.Start(); err != nil {
		gui.appendOutput(tabID, fmt.Sprintf("Failed to start command: %v\n", err))
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
				gui.appendOutput(tabID, lineCopy)
				gui.refreshOutputView()
				return nil
			})
		}

		waitErr := cmd.Wait()
		gui.g.Update(func(g *gocui.Gui) error {
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

	tabID := gui.startCommandOutputTab(title)
	gui.appendOutput(tabID, fmt.Sprintf("$ make %s\n\n", target))
	_ = gui.switchPanel(MainWindow)

	go func() {
		output, runErr := cmd.CombinedOutput()

		gui.g.Update(func(g *gocui.Gui) error {
			if runErr != nil {
				gui.appendOutput(tabID, fmt.Sprintf("Error: %v\n\n", runErr))
			}
			gui.appendOutput(tabID, string(output))
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
	tabID := gui.startCommandOutputTab("URL Patterns")
	gui.appendOutput(tabID, "Loading URL patterns...\n")
	_ = gui.switchPanel(MainWindow)

	go func() {
		patterns, runErr := gui.project.GetURLPatterns()
		gui.g.Update(func(g *gocui.Gui) error {
			gui.resetOutput(tabID, "URL Patterns")
			if runErr != nil {
				gui.appendOutput(tabID, fmt.Sprintf("Error: %v\n", runErr))
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
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}

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

	title := "Start Containers"
	command := "up -d"
	if action == "stop" {
		title = "Stop Containers"
		command = "stop"
	}
	tabID := gui.startCommandOutputTab(title)
	gui.appendOutput(tabID, fmt.Sprintf("Running docker compose %s for: %s\n\n", command, strings.Join(selected, ", ")))
	gui.refreshOutputView()
	_ = gui.switchPanel(MainWindow)

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

	if hadError {
		gui.appendOutput(tabID, "Completed with errors. Check output above.\n")
	} else {
		gui.appendOutput(tabID, "Completed successfully.\n")
	}
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
	return gui.runStreamingCommandToLogs("Dev Server", cmd, true)
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
