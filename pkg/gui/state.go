package gui

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	projectStateVersion           = 2
	projectStateDirName           = ".lazy-django"
	projectStateFileName          = "state.json"
	projectHistoryFileName        = "history.ndjson"
	maxPersistedOutputTabs        = 16
	maxPersistedOutputTextBytes   = 64 * 1024
	maxPersistedCommandHistoryLen = 200
	maxPersistedRecentModels      = 30
	maxPersistedFavoriteCommands  = 20
	maxPersistedRecentErrors      = 40
	maxPersistedHistoryEvents     = 500
	historyRetentionDays          = 30
)

type persistedOutputTab struct {
	Route      string `json:"route"`
	Title      string `json:"title,omitempty"`
	Text       string `json:"text,omitempty"`
	Autoscroll bool   `json:"autoscroll,omitempty"`
	OriginX    int    `json:"origin_x,omitempty"`
	OriginY    int    `json:"origin_y,omitempty"`
}

type persistedRecentModel struct {
	App            string `json:"app"`
	Model          string `json:"model"`
	LastPage       int    `json:"last_page,omitempty"`
	LastRecordIdx  int    `json:"last_record_idx,omitempty"`
	LastRecordPK   string `json:"last_record_pk,omitempty"`
	LastAccessedAt string `json:"last_accessed_at,omitempty"`
}

type persistedRecentError struct {
	Key       string `json:"key"`
	Message   string `json:"message"`
	Source    string `json:"source,omitempty"`
	Count     int    `json:"count"`
	FirstSeen string `json:"first_seen,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
}

type persistedGUIState struct {
	Version          int                    `json:"version"`
	SavedAt          string                 `json:"saved_at,omitempty"`
	CurrentWindow    string                 `json:"current_window,omitempty"`
	MenuSelection    int                    `json:"menu_selection,omitempty"`
	ListSelection    int                    `json:"list_selection,omitempty"`
	DataSelection    int                    `json:"data_selection,omitempty"`
	ActiveOutputTab  int                    `json:"active_output_tab,omitempty"`
	OutputTabs       []persistedOutputTab   `json:"output_tabs,omitempty"`
	CommandHistory   []string               `json:"command_history,omitempty"`
	RecentModels     []persistedRecentModel `json:"recent_models,omitempty"`
	FavoriteCommands []string               `json:"favorite_commands,omitempty"`
	RecentErrors     []persistedRecentError `json:"recent_errors,omitempty"`
}

type historyEvent struct {
	Time        string   `json:"time"`
	Type        string   `json:"type"`
	Source      string   `json:"source,omitempty"`
	Status      string   `json:"status,omitempty"`
	Command     string   `json:"command,omitempty"`
	ExitCode    int      `json:"exit_code"`
	DurationMS  int64    `json:"duration_ms,omitempty"`
	OutputTab   string   `json:"output_tab,omitempty"`
	OutputRoute string   `json:"output_route,omitempty"`
	Action      string   `json:"action,omitempty"`
	Services    []string `json:"services,omitempty"`
	App         string   `json:"app,omitempty"`
	Model       string   `json:"model,omitempty"`
	Page        int      `json:"page,omitempty"`
	RecordPK    string   `json:"record_pk,omitempty"`
	SnapshotID  string   `json:"snapshot_id,omitempty"`
	Snapshot    string   `json:"snapshot,omitempty"`
	Error       string   `json:"error,omitempty"`
}

type projectStateStore struct {
	path string
}

type projectHistoryStore struct {
	path string
}

var (
	secretAssignPattern = regexp.MustCompile(`(?i)([A-Za-z0-9_-]*(?:password|passwd|pwd|secret|token|api[_-]?key|access[_-]?token|auth[_-]?token)[A-Za-z0-9_-]*)(=)("[^"]*"|'[^']*'|[^\s]+)`)
	secretFlagPattern   = regexp.MustCompile(`(?i)(--?(?:password|passwd|pwd|secret|token|api[-_]?key|access[-_]?token|auth[-_]?token))(=|\s+)("[^"]*"|'[^']*'|[^\s]+)`)
	uriSecretPattern    = regexp.MustCompile(`://([^:/\s]+):([^@/\s]+)@`)
)

func newProjectStateStore(projectRoot string) *projectStateStore {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil
	}
	return &projectStateStore{
		path: filepath.Join(projectRoot, projectStateDirName, projectStateFileName),
	}
}

func newProjectHistoryStore(projectRoot string) *projectHistoryStore {
	projectRoot = strings.TrimSpace(projectRoot)
	if projectRoot == "" {
		return nil
	}
	return &projectHistoryStore{
		path: filepath.Join(projectRoot, projectStateDirName, projectHistoryFileName),
	}
}

func (store *projectStateStore) load() (*persistedGUIState, error) {
	if store == nil {
		return nil, nil
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var state persistedGUIState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("invalid project state: %w", err)
	}
	return &state, nil
}

func (store *projectStateStore) save(state *persistedGUIState) error {
	if store == nil || state == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(store.path, data, 0644)
}

func (store *projectHistoryStore) append(event historyEvent) error {
	if store == nil {
		return nil
	}
	if strings.TrimSpace(event.Time) == "" {
		event.Time = time.Now().UTC().Format(time.RFC3339)
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(store.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if _, err := f.Write([]byte("\n")); err != nil {
		return err
	}

	return store.compact()
}

func (store *projectHistoryStore) compact() error {
	if store == nil {
		return nil
	}
	f, err := os.Open(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	cutoff := time.Now().UTC().AddDate(0, 0, -historyRetentionDays)
	kept := make([]string, 0, maxPersistedHistoryEvents)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event historyEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if ts, err := time.Parse(time.RFC3339, event.Time); err == nil {
			if ts.Before(cutoff) {
				continue
			}
		}
		kept = append(kept, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if len(kept) > maxPersistedHistoryEvents {
		kept = kept[len(kept)-maxPersistedHistoryEvents:]
	}

	var out strings.Builder
	for _, line := range kept {
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return os.WriteFile(store.path, []byte(out.String()), 0644)
}

func (store *projectHistoryStore) tail(limit int) ([]historyEvent, error) {
	if store == nil || limit <= 0 {
		return nil, nil
	}
	f, err := os.Open(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	events := make([]historyEvent, 0, limit)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event historyEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func sanitizeCommand(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	command = secretAssignPattern.ReplaceAllString(command, `$1$2[REDACTED]`)
	command = secretFlagPattern.ReplaceAllString(command, `$1$2[REDACTED]`)
	command = uriSecretPattern.ReplaceAllString(command, `://$1:[REDACTED]@`)
	return command
}

func safeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return sanitizeCommand(strings.TrimSpace(err.Error()))
}

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func modelStateKey(app, model string) string {
	return strings.ToLower(strings.TrimSpace(app) + "." + strings.TrimSpace(model))
}

func stringifyPK(pk interface{}) string {
	if pk == nil {
		return ""
	}
	return fmt.Sprintf("%v", pk)
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (gui *Gui) markStateDirty() {
	gui.stateDirty = true
}

func (gui *Gui) updateFavoriteCommands(command string) {
	command = sanitizeCommand(command)
	if command == "" {
		return
	}
	favorites := make([]string, 0, len(gui.favoriteCommands)+1)
	favorites = append(favorites, command)
	for _, existing := range gui.favoriteCommands {
		if existing == command {
			continue
		}
		favorites = append(favorites, existing)
		if len(favorites) >= maxPersistedFavoriteCommands {
			break
		}
	}
	gui.favoriteCommands = favorites
	gui.markStateDirty()
}

func (gui *Gui) rememberCommand(command string) {
	command = sanitizeCommand(command)
	if command == "" {
		return
	}
	gui.commandHistory = append(gui.commandHistory, command)
	if len(gui.commandHistory) > maxPersistedCommandHistoryLen {
		gui.commandHistory = append([]string(nil), gui.commandHistory[len(gui.commandHistory)-maxPersistedCommandHistoryLen:]...)
	}
	gui.updateFavoriteCommands(command)
	gui.markStateDirty()
}

func (gui *Gui) rememberError(source, message string) {
	source = strings.TrimSpace(source)
	message = sanitizeCommand(strings.TrimSpace(message))
	if message == "" {
		return
	}
	if source == "" {
		source = "unknown"
	}
	key := strings.ToLower(source + "|" + message)
	now := nowRFC3339()

	updated := false
	entries := make([]persistedRecentError, 0, len(gui.recentErrors)+1)
	for _, item := range gui.recentErrors {
		if item.Key == key {
			item.Count++
			item.LastSeen = now
			entries = append([]persistedRecentError{item}, entries...)
			updated = true
			continue
		}
		entries = append(entries, item)
	}
	if !updated {
		entries = append([]persistedRecentError{{
			Key:       key,
			Message:   message,
			Source:    source,
			Count:     1,
			FirstSeen: now,
			LastSeen:  now,
		}}, entries...)
	}
	if len(entries) > maxPersistedRecentErrors {
		entries = entries[:maxPersistedRecentErrors]
	}
	gui.recentErrors = entries
	gui.markStateDirty()

	gui.appendHistoryEvent(historyEvent{
		Time:   now,
		Type:   "error",
		Source: source,
		Status: "error",
		Error:  message,
	})
}

func (gui *Gui) rememberModelAccess(app, model string, page, recordIdx int, recordPK interface{}) {
	app = strings.TrimSpace(app)
	model = strings.TrimSpace(model)
	if app == "" || model == "" {
		return
	}
	if page < 1 {
		page = 1
	}
	if recordIdx < 0 {
		recordIdx = 0
	}

	entry := persistedRecentModel{
		App:            app,
		Model:          model,
		LastPage:       page,
		LastRecordIdx:  recordIdx,
		LastRecordPK:   stringifyPK(recordPK),
		LastAccessedAt: nowRFC3339(),
	}

	key := modelStateKey(app, model)
	items := make([]persistedRecentModel, 0, len(gui.recentModels)+1)
	items = append(items, entry)
	for _, existing := range gui.recentModels {
		if modelStateKey(existing.App, existing.Model) == key {
			continue
		}
		items = append(items, existing)
		if len(items) >= maxPersistedRecentModels {
			break
		}
	}
	gui.recentModels = items
	gui.markStateDirty()
}

func (gui *Gui) recentModelState(app, model string) (persistedRecentModel, bool) {
	key := modelStateKey(app, model)
	for _, item := range gui.recentModels {
		if modelStateKey(item.App, item.Model) == key {
			return item, true
		}
	}
	return persistedRecentModel{}, false
}

func (gui *Gui) appendHistoryEvent(event historyEvent) {
	if gui.historyStore == nil {
		if gui.project == nil {
			return
		}
		gui.historyStore = newProjectHistoryStore(gui.project.RootDir)
	}
	if gui.historyStore == nil {
		return
	}
	if strings.TrimSpace(event.Time) == "" {
		event.Time = nowRFC3339()
	}
	if event.Command != "" {
		event.Command = sanitizeCommand(event.Command)
	}
	if event.Error != "" {
		event.Error = sanitizeCommand(event.Error)
	}
	if err := gui.historyStore.append(event); err != nil {
		gui.historyStoreErr = err.Error()
	} else {
		gui.historyStoreErr = ""
	}
}

func (gui *Gui) recordCommandExecution(command, tabID string, startedAt time.Time, runErr error) {
	command = sanitizeCommand(command)
	gui.rememberCommand(command)

	route := ""
	if tab, ok := gui.outputTabs[tabID]; ok {
		route = tab.route
	}
	status := "success"
	if runErr != nil {
		status = "error"
		gui.rememberError("command", fmt.Sprintf("%s: %v", command, runErr))
	}

	duration := time.Since(startedAt)
	if duration < 0 {
		duration = 0
	}
	gui.appendHistoryEvent(historyEvent{
		Type:        "command",
		Source:      "project",
		Status:      status,
		Command:     command,
		ExitCode:    exitCodeFromError(runErr),
		DurationMS:  duration.Milliseconds(),
		OutputTab:   tabID,
		OutputRoute: route,
		Error:       safeErrorMessage(runErr),
	})
}

func (gui *Gui) recordContainerAction(action string, services []string, startedAt time.Time, runErr error) {
	status := "success"
	if runErr != nil {
		status = "error"
		gui.rememberError("containers", runErr.Error())
	}
	servicesCopy := append([]string(nil), services...)
	duration := time.Since(startedAt)
	if duration < 0 {
		duration = 0
	}
	gui.appendHistoryEvent(historyEvent{
		Type:       "container",
		Source:     "docker",
		Status:     status,
		Action:     action,
		Services:   servicesCopy,
		DurationMS: duration.Milliseconds(),
		ExitCode:   exitCodeFromError(runErr),
		Error:      safeErrorMessage(runErr),
	})
}

func (gui *Gui) recordSnapshotActivity(action, snapshotID, snapshotName string, runErr error) {
	status := "success"
	if runErr != nil {
		status = "error"
		gui.rememberError("snapshot", runErr.Error())
	}
	gui.appendHistoryEvent(historyEvent{
		Type:       "snapshot",
		Source:     "data",
		Status:     status,
		Action:     action,
		SnapshotID: strings.TrimSpace(snapshotID),
		Snapshot:   strings.TrimSpace(snapshotName),
		ExitCode:   exitCodeFromError(runErr),
		Error:      safeErrorMessage(runErr),
	})
}

func (gui *Gui) recordModelOpen(app, model string, page int, recordPK interface{}) {
	gui.appendHistoryEvent(historyEvent{
		Type:     "model",
		Source:   "database",
		Status:   "success",
		Action:   "open",
		App:      strings.TrimSpace(app),
		Model:    strings.TrimSpace(model),
		Page:     page,
		RecordPK: stringifyPK(recordPK),
	})
}

func (gui *Gui) loadProjectState() error {
	if gui.project == nil {
		return nil
	}
	if gui.stateStore == nil {
		gui.stateStore = newProjectStateStore(gui.project.RootDir)
	}
	if gui.historyStore == nil {
		gui.historyStore = newProjectHistoryStore(gui.project.RootDir)
	}
	state, err := gui.stateStore.load()
	if err != nil || state == nil {
		return err
	}

	if isPanelName(state.CurrentWindow) {
		gui.currentWindow = state.CurrentWindow
	}
	gui.menuSelection = state.MenuSelection
	gui.listSelection = state.ListSelection
	gui.dataSelection = state.DataSelection

	if len(state.CommandHistory) > 0 {
		history := append([]string(nil), state.CommandHistory...)
		if len(history) > maxPersistedCommandHistoryLen {
			history = history[len(history)-maxPersistedCommandHistoryLen:]
		}
		for i := range history {
			history[i] = sanitizeCommand(history[i])
		}
		gui.commandHistory = history
	}

	if len(state.RecentModels) > 0 {
		models := append([]persistedRecentModel(nil), state.RecentModels...)
		if len(models) > maxPersistedRecentModels {
			models = models[:maxPersistedRecentModels]
		}
		for i := range models {
			if models[i].LastPage < 1 {
				models[i].LastPage = 1
			}
			if models[i].LastRecordIdx < 0 {
				models[i].LastRecordIdx = 0
			}
		}
		gui.recentModels = models
	}

	if len(state.FavoriteCommands) > 0 {
		favorites := make([]string, 0, len(state.FavoriteCommands))
		seen := make(map[string]struct{}, len(state.FavoriteCommands))
		for _, cmd := range state.FavoriteCommands {
			cmd = sanitizeCommand(cmd)
			if cmd == "" {
				continue
			}
			if _, ok := seen[cmd]; ok {
				continue
			}
			seen[cmd] = struct{}{}
			favorites = append(favorites, cmd)
			if len(favorites) >= maxPersistedFavoriteCommands {
				break
			}
		}
		gui.favoriteCommands = favorites
	}

	if len(state.RecentErrors) > 0 {
		errs := append([]persistedRecentError(nil), state.RecentErrors...)
		if len(errs) > maxPersistedRecentErrors {
			errs = errs[:maxPersistedRecentErrors]
		}
		for i := range errs {
			errs[i].Message = sanitizeCommand(errs[i].Message)
			if errs[i].Count < 1 {
				errs[i].Count = 1
			}
			if errs[i].Key == "" {
				errs[i].Key = strings.ToLower(strings.TrimSpace(errs[i].Source) + "|" + strings.TrimSpace(errs[i].Message))
			}
		}
		gui.recentErrors = errs
	}

	gui.restoreOutputTabsFromState(state.OutputTabs, state.ActiveOutputTab)
	gui.clampSelections()
	gui.stateDirty = false
	return nil
}

func (gui *Gui) restoreOutputTabsFromState(savedTabs []persistedOutputTab, activeIndex int) {
	gui.ensureOutputState()
	gui.outputTabs = make(map[string]*outputTabState, len(savedTabs))
	gui.outputRoutes = make(map[string]string)
	gui.outputOrder = make([]string, 0, len(savedTabs))
	gui.outputCounter = 0
	gui.outputTab = ""

	for _, saved := range savedTabs {
		route := saved.Route
		if !isOutputRoute(route) {
			route = OutputTabCommand
		}

		title := strings.TrimSpace(saved.Title)
		if title == "" {
			title = outputDefaultTitle(route)
		}

		gui.outputCounter++
		id := fmt.Sprintf("%s-%03d", route, gui.outputCounter)

		tab := &outputTabState{
			id:         id,
			route:      route,
			title:      title,
			text:       saved.Text,
			autoscroll: saved.Autoscroll,
			originX:    saved.OriginX,
			originY:    saved.OriginY,
		}
		if tab.autoscroll {
			tab.originX = 0
			tab.originY = 0
		}

		gui.outputTabs[id] = tab
		gui.outputOrder = append(gui.outputOrder, id)
		gui.outputRoutes[route] = id
	}

	if len(gui.outputOrder) == 0 {
		return
	}

	if activeIndex < 0 || activeIndex >= len(gui.outputOrder) {
		activeIndex = len(gui.outputOrder) - 1
	}
	gui.outputTab = gui.outputOrder[activeIndex]
}

func truncateTextTail(text string, maxBytes int) string {
	if maxBytes <= 0 || len(text) <= maxBytes {
		return text
	}
	const marker = "\n...[truncated from previous session]...\n"
	if maxBytes <= len(marker) {
		return text[len(text)-maxBytes:]
	}
	return marker + text[len(text)-(maxBytes-len(marker)):]
}

func isPanelName(name string) bool {
	switch name {
	case MenuWindow, ListWindow, DataWindow, MainWindow:
		return true
	default:
		return false
	}
}

func (gui *Gui) captureOutputOriginForState() {
	if gui.g == nil || gui.currentModel != "" {
		return
	}
	tabID := gui.resolveOutputTabID("", false)
	if tabID == "" {
		return
	}
	tab, ok := gui.outputTabs[tabID]
	if !ok || tab.autoscroll {
		return
	}
	v, err := gui.g.View(MainWindow)
	if err != nil {
		return
	}
	originX, originY := v.Origin()
	if v.Autoscroll {
		tab.originX = 0
		tab.originY = outputOriginTail
		return
	}
	tab.originX = originX
	tab.originY = originY
}

func (gui *Gui) buildPersistedState() *persistedGUIState {
	gui.captureOutputOriginForState()

	state := &persistedGUIState{
		Version:       projectStateVersion,
		SavedAt:       time.Now().UTC().Format(time.RFC3339),
		CurrentWindow: gui.currentWindow,
		MenuSelection: gui.menuSelection,
		ListSelection: gui.listSelection,
		DataSelection: gui.dataSelection,
	}

	if !isPanelName(state.CurrentWindow) {
		state.CurrentWindow = MenuWindow
	}

	if len(gui.commandHistory) > 0 {
		history := gui.commandHistory
		if len(history) > maxPersistedCommandHistoryLen {
			history = history[len(history)-maxPersistedCommandHistoryLen:]
		}
		state.CommandHistory = append([]string(nil), history...)
	}

	if len(gui.recentModels) > 0 {
		models := gui.recentModels
		if len(models) > maxPersistedRecentModels {
			models = models[:maxPersistedRecentModels]
		}
		state.RecentModels = append([]persistedRecentModel(nil), models...)
	}

	if len(gui.favoriteCommands) > 0 {
		favorites := gui.favoriteCommands
		if len(favorites) > maxPersistedFavoriteCommands {
			favorites = favorites[:maxPersistedFavoriteCommands]
		}
		state.FavoriteCommands = append([]string(nil), favorites...)
	}

	if len(gui.recentErrors) > 0 {
		errs := gui.recentErrors
		if len(errs) > maxPersistedRecentErrors {
			errs = errs[:maxPersistedRecentErrors]
		}
		state.RecentErrors = append([]persistedRecentError(nil), errs...)
	}

	tabIDs := gui.outputOrder
	if len(tabIDs) > maxPersistedOutputTabs {
		tabIDs = tabIDs[len(tabIDs)-maxPersistedOutputTabs:]
	}

	active := -1
	state.OutputTabs = make([]persistedOutputTab, 0, len(tabIDs))
	for _, tabID := range tabIDs {
		tab, ok := gui.outputTabs[tabID]
		if !ok {
			continue
		}
		saved := persistedOutputTab{
			Route:      tab.route,
			Title:      tab.title,
			Text:       truncateTextTail(tab.text, maxPersistedOutputTextBytes),
			Autoscroll: tab.autoscroll,
			OriginX:    tab.originX,
			OriginY:    tab.originY,
		}
		if saved.Autoscroll {
			saved.OriginX = 0
			saved.OriginY = 0
		}

		state.OutputTabs = append(state.OutputTabs, saved)
		if tabID == gui.outputTab {
			active = len(state.OutputTabs) - 1
		}
	}

	if len(state.OutputTabs) > 0 {
		if active < 0 || active >= len(state.OutputTabs) {
			active = len(state.OutputTabs) - 1
		}
		state.ActiveOutputTab = active
	}

	return state
}

func (gui *Gui) saveProjectState() error {
	if gui.project == nil {
		return nil
	}
	if gui.stateStore == nil {
		gui.stateStore = newProjectStateStore(gui.project.RootDir)
	}
	if gui.stateStore == nil {
		return nil
	}
	if !gui.stateDirty {
		return nil
	}
	if err := gui.stateStore.save(gui.buildPersistedState()); err != nil {
		return err
	}
	gui.stateDirty = false
	return nil
}
