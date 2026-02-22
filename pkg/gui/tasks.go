package gui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/awesome-gocui/gocui"
)

const (
	projectTasksFileName = "tasks.json"
	projectTasksVersion  = 1
	maxProjectTasks      = 200
)

type projectTaskEntry struct {
	Label   string `json:"label"`
	Command string `json:"command"`
}

type projectTaskConfig struct {
	Version   int                `json:"version"`
	CreatedAt string             `json:"created_at,omitempty"`
	UpdatedAt string             `json:"updated_at,omitempty"`
	Tasks     []projectTaskEntry `json:"tasks"`
}

func isTerminalEditorCommand(editor string) bool {
	editor = strings.TrimSpace(editor)
	if editor == "" {
		return false
	}
	base := editor
	if strings.Contains(base, " ") {
		base = strings.Fields(base)[0]
	}
	base = strings.ToLower(filepath.Base(base))
	switch base {
	case "vi", "vim", "nvim", "nano", "emacs", "hx", "kak", "micro":
		return true
	default:
		return false
	}
}

func resolveEditorCommand() string {
	if visual := strings.TrimSpace(os.Getenv("VISUAL")); visual != "" {
		return visual
	}
	if editor := strings.TrimSpace(os.Getenv("EDITOR")); editor != "" {
		return editor
	}
	return "vi"
}

func runAppleScript(script string) error {
	cmd := exec.Command("osascript", "-e", script)
	output, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}

	msg := strings.TrimSpace(string(output))
	if msg != "" {
		return fmt.Errorf("%w: %s", err, msg)
	}
	return err
}

func terminalDoScript(command string) string {
	return fmt.Sprintf(`tell application "Terminal"
activate
do script %q
end tell`, command)
}

func iTermWriteScript(command string) string {
	return fmt.Sprintf(`tell application "iTerm"
activate
if (count of windows) = 0 then
	create window with default profile
end if
tell current session of current window
	write text %q
end tell
end tell`, command)
}

func launchTerminalEditorCommand(command string) error {
	terminalErr := runAppleScript(terminalDoScript(command))
	if terminalErr == nil {
		return nil
	}

	iTermErr := runAppleScript(iTermWriteScript(command))
	if iTermErr == nil {
		return nil
	}

	return fmt.Errorf("Terminal launch failed (%v), iTerm launch failed (%v)", terminalErr, iTermErr)
}

func launchEditorInPreferredTerminal(command string) error {
	termProgram := strings.ToLower(strings.TrimSpace(os.Getenv("TERM_PROGRAM")))
	switch termProgram {
	case "apple_terminal":
		if err := runAppleScript(terminalDoScript(command)); err == nil {
			return nil
		}
	case "iterm.app":
		if err := runAppleScript(iTermWriteScript(command)); err == nil {
			return nil
		}
	}

	return launchTerminalEditorCommand(command)
}

func runCommandInCurrentTerminal(command string, dir string) error {
	cmd := exec.Command("sh", "-lc", command)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	gocui.Suspend()
	runErr := cmd.Run()
	resumeErr := gocui.Resume()
	if runErr != nil {
		if resumeErr != nil {
			return fmt.Errorf("%v; resume failed: %v", runErr, resumeErr)
		}
		return runErr
	}
	if resumeErr != nil {
		return fmt.Errorf("resume failed: %w", resumeErr)
	}
	return nil
}

func (gui *Gui) projectTasksFilePath() string {
	if gui.project == nil {
		return ""
	}
	if gui.projectTasksPath != "" {
		return gui.projectTasksPath
	}
	gui.projectTasksPath = filepath.Join(gui.project.RootDir, ".lazy-django", projectTasksFileName)
	return gui.projectTasksPath
}

func normalizeProjectTasks(tasks []projectTaskEntry) []projectTaskEntry {
	seen := make(map[string]struct{}, len(tasks))
	normalized := make([]projectTaskEntry, 0, len(tasks))
	for _, task := range tasks {
		task.Label = strings.TrimSpace(task.Label)
		task.Command = strings.TrimSpace(task.Command)
		if task.Command == "" {
			continue
		}
		if task.Label == "" {
			task.Label = task.Command
		}
		key := strings.ToLower(task.Command)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, task)
		if len(normalized) >= maxProjectTasks {
			break
		}
	}
	return normalized
}

func (gui *Gui) defaultProjectTasks() []projectTaskEntry {
	tasks := make([]projectTaskEntry, 0)

	for _, action := range gui.projectMakeActions() {
		if action.makeTarget == "" {
			continue
		}
		tasks = append(tasks, projectTaskEntry{
			Label:   action.label,
			Command: fmt.Sprintf("make %s", action.makeTarget),
		})
	}

	for _, action := range gui.projectMigrationActions() {
		switch {
		case action.makeTarget != "":
			tasks = append(tasks, projectTaskEntry{
				Label:   action.label,
				Command: fmt.Sprintf("make %s", action.makeTarget),
			})
		case action.command != "":
			tasks = append(tasks, projectTaskEntry{
				Label:   action.label,
				Command: fmt.Sprintf("python manage.py %s", action.command),
			})
		}
	}

	for _, action := range gui.projectToolActions() {
		if action.command == "" {
			continue
		}
		tasks = append(tasks, projectTaskEntry{
			Label:   action.label,
			Command: fmt.Sprintf("python manage.py %s", action.command),
		})
	}

	return normalizeProjectTasks(tasks)
}

func (gui *Gui) writeProjectTasks(tasks []projectTaskEntry) error {
	path := gui.projectTasksFilePath()
	if path == "" {
		return fmt.Errorf("project tasks path is not configured")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	tasks = normalizeProjectTasks(tasks)
	cfg := projectTaskConfig{
		Version:   projectTasksVersion,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Tasks:     tasks,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (gui *Gui) loadProjectTasks(force bool) ([]projectTaskEntry, error) {
	if gui.projectTasksReady && !force {
		return gui.projectTasks, gui.projectTasksErr
	}
	path := gui.projectTasksFilePath()
	if path == "" {
		gui.projectTasksReady = true
		gui.projectTasks = nil
		gui.projectTasksErr = fmt.Errorf("project tasks path is not configured")
		return nil, gui.projectTasksErr
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			gui.projectTasksReady = true
			gui.projectTasks = nil
			gui.projectTasksErr = err
			return nil, err
		}

		// First project init: freeze current auto-discovered tasks into a project file.
		initial := gui.defaultProjectTasks()
		if writeErr := gui.writeProjectTasks(initial); writeErr != nil {
			gui.projectTasksReady = true
			gui.projectTasks = initial
			gui.projectTasksErr = writeErr
			return initial, writeErr
		}
		gui.projectTasksReady = true
		gui.projectTasks = initial
		gui.projectTasksErr = nil
		return initial, nil
	}

	var cfg projectTaskConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		gui.projectTasksReady = true
		gui.projectTasks = nil
		gui.projectTasksErr = fmt.Errorf("invalid project tasks file: %w", err)
		return nil, gui.projectTasksErr
	}

	tasks := normalizeProjectTasks(cfg.Tasks)
	gui.projectTasksReady = true
	gui.projectTasks = tasks
	gui.projectTasksErr = nil
	return tasks, nil
}

func (gui *Gui) projectTaskActions() []projectAction {
	if !gui.projectTasksReady {
		return nil
	}
	tasks := gui.projectTasks
	if len(tasks) == 0 {
		return nil
	}
	actions := make([]projectAction, 0, len(tasks))
	for _, task := range tasks {
		actions = append(actions, projectAction{
			label:        task.Label,
			shellCommand: task.Command,
		})
	}
	return actions
}

func (gui *Gui) editProjectTasksFile() error {
	if _, err := gui.loadProjectTasks(false); err != nil {
		return gui.showMessage("Project Tasks", fmt.Sprintf("Failed to load project tasks: %v", err))
	}

	path := gui.projectTasksFilePath()
	if path == "" {
		return gui.showMessage("Project Tasks", "Task file path is not available.")
	}

	editor := resolveEditorCommand()
	command := fmt.Sprintf("%s %q", editor, path)
	terminalCommand := fmt.Sprintf("cd %q && %s", gui.project.RootDir, command)
	var interactiveErr error

	// Preferred path: run terminal editors in the same terminal and block until exit.
	if isTerminalEditorCommand(editor) {
		interactiveErr = runCommandInCurrentTerminal(command, gui.project.RootDir)
		if interactiveErr == nil {
			_, reloadErr := gui.loadProjectTasks(true)
			if menuView, viewErr := gui.g.View(MenuWindow); viewErr == nil {
				gui.renderProjectList(menuView)
			}
			if reloadErr != nil {
				return gui.showMessage("Project Tasks", fmt.Sprintf("Edited, but failed to reload tasks: %v", reloadErr))
			}
			return gui.showMessage("Project Tasks", fmt.Sprintf("Tasks reloaded from: %s", path))
		}
	}

	// Terminal editors are launched in a new Terminal window on macOS.
	if runtime.GOOS == "darwin" && isTerminalEditorCommand(editor) {
		if err := launchEditorInPreferredTerminal(terminalCommand); err != nil {
			// Fallback to TextEdit so the user can still edit the file.
			if openErr := exec.Command("open", "-t", path).Start(); openErr == nil {
				return gui.showMessage("Project Tasks", fmt.Sprintf("Terminal launch failed.\nOpened in TextEdit instead: %s\nError: %v", path, err))
			}
			return gui.showMessage("Project Tasks", fmt.Sprintf("Failed to launch terminal editor: %v\nEdit manually: %s", err, path))
		}
		return gui.showMessage("Project Tasks", fmt.Sprintf("Opened in external terminal editor: %s\nRun 'r' to reload tasks after save.", path))
	}
	if interactiveErr != nil && isTerminalEditorCommand(editor) {
		return gui.showMessage("Project Tasks", fmt.Sprintf("Failed to open editor in current terminal: %v", interactiveErr))
	}

	cmd := exec.Command("sh", "-lc", command)
	cmd.Dir = gui.project.RootDir
	if err := cmd.Start(); err != nil {
		return gui.showMessage("Project Tasks", fmt.Sprintf("Failed to launch editor: %v\nEdit manually: %s", err, path))
	}

	// Editor may still be open; user can refresh with `r` after saving.
	msg := fmt.Sprintf("Opened in editor: %s\nRun 'r' to reload tasks after edits.", path)
	return gui.showMessage("Project Tasks", msg)
}
