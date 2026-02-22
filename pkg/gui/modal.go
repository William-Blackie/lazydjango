package gui

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/awesome-gocui/gocui"
	"github.com/williamblackie/lazydjango/pkg/django"
)

func (gui *Gui) moveProjectModalSelection(delta int) {
	count := len(gui.projectModalActions)
	if count == 0 {
		gui.projectModalIndex = 0
		gui.projectModalOffset = 0
		gui.projectModalNumber = ""
		return
	}
	next := gui.projectModalIndex + delta
	if next < 0 {
		next = 0
	}
	if next >= count {
		next = count - 1
	}
	gui.projectModalIndex = next
	gui.projectModalNumber = ""
}

func (gui *Gui) clearProjectModalNumberInput() {
	gui.projectModalNumber = ""
}

func (gui *Gui) appendProjectModalNumberInput(digit rune) {
	if len(gui.projectModalActions) == 0 {
		gui.projectModalNumber = ""
		return
	}

	next := gui.projectModalNumber + string(digit)
	for len(next) > 1 && strings.HasPrefix(next, "0") {
		next = strings.TrimPrefix(next, "0")
	}
	if next == "" {
		next = "0"
	}

	value, err := strconv.Atoi(next)
	if err != nil {
		return
	}

	if value < 1 || value > len(gui.projectModalActions) {
		// Retry using only the latest digit for quick jumps.
		next = string(digit)
		value, err = strconv.Atoi(next)
		if err != nil || value < 1 || value > len(gui.projectModalActions) {
			return
		}
	}

	gui.projectModalNumber = next
	gui.projectModalIndex = value - 1
}

func (gui *Gui) projectActionsViewport(v *gocui.View) (int, int) {
	total := len(gui.projectModalActions)
	if total == 0 {
		gui.projectModalOffset = 0
		return 0, 0
	}

	_, h := v.Size()
	visible := h - 4
	if visible < 1 {
		visible = 1
	}

	idx := clampSelection(gui.projectModalIndex, total)
	start := gui.projectModalOffset
	if start < 0 {
		start = 0
	}

	maxStart := total - visible
	if maxStart < 0 {
		maxStart = 0
	}
	if start > maxStart {
		start = maxStart
	}
	if idx < start {
		start = idx
	}
	if idx >= start+visible {
		start = idx - visible + 1
	}
	if start < 0 {
		start = 0
	}
	if start > maxStart {
		start = maxStart
	}

	end := start + visible
	if end > total {
		end = total
	}

	gui.projectModalOffset = start
	return start, end
}

// openFormModal opens a modal for adding or editing a record
func (gui *Gui) openFormModal(modalType string, fields []map[string]interface{}, currentValues map[string]string) {
	returnWindow := gui.currentWindow
	if returnWindow == "" {
		returnWindow = MainWindow
	}
	gui.isModalOpen = true
	gui.modalType = modalType
	gui.modalReturnWindow = returnWindow
	gui.modalFields = fields
	gui.modalFieldIdx = 0
	gui.modalValues = make(map[string]string)

	if currentValues != nil {
		gui.modalValues = currentValues
	}
	gui.modalMessage = ""

	if modalType == "add" {
		gui.modalTitle = fmt.Sprintf("Add %s.%s", gui.currentApp, gui.currentModel)
	} else {
		gui.modalTitle = fmt.Sprintf("Edit %s.%s", gui.currentApp, gui.currentModel)
	}
}

// openConfirmModal opens a confirmation dialog for deletion
func (gui *Gui) openConfirmModal(modalType string, record django.ModelRecord) {
	returnWindow := gui.currentWindow
	if returnWindow == "" {
		returnWindow = MainWindow
	}
	gui.isModalOpen = true
	gui.modalType = modalType
	gui.modalReturnWindow = returnWindow
	gui.modalTitle = "Confirm Delete"

	fieldInfo := ""
	for key, value := range record.Fields {
		fieldInfo += fmt.Sprintf("  %s: %v\n", key, value)
	}
	gui.modalMessage = fmt.Sprintf("Delete %s.%s #%v?\n\n%s\nPress Enter to confirm, Esc to cancel",
		gui.currentApp, gui.currentModel, record.PK, fieldInfo)
}

// renderModal renders the modal content
func (gui *Gui) renderModal(v *gocui.View) {
	v.Clear()

	if gui.modalType == "delete" {
		fmt.Fprintln(v, gui.modalMessage)
		return
	}
	if gui.modalType == "restore" {
		fmt.Fprintln(v, "Select a snapshot to restore:")
		fmt.Fprintln(v, "Snapshots are shown newest first.")
		fmt.Fprintln(v, "")
		for i, snapshot := range gui.restoreSnapshots {
			prefix := "  "
			if i == gui.restoreIndex {
				prefix = "> "
			}
			fmt.Fprintf(v, "%s%s\n", prefix, snapshot.Name)
			fmt.Fprintf(v, "   %s\n", snapshot.Timestamp.Local().Format("2006-01-02 15:04:05"))
			if snapshot.GitBranch != "" {
				branch := snapshot.GitBranch
				if snapshot.GitCommit != "" {
					branch = fmt.Sprintf("%s (%s)", branch, shortCommit(snapshot.GitCommit))
				}
				fmt.Fprintf(v, "   Branch: %s\n", branch)
			}
			fmt.Fprintln(v)
		}
		fmt.Fprintln(v, "Enter: Restore selected snapshot  |  Esc: Cancel")
		return
	}
	if gui.modalType == "containers" {
		actionLabel := "start"
		if gui.containerAction == "stop" {
			actionLabel = "stop"
		}

		selected := gui.selectedContainerServices()
		fmt.Fprintf(v, "Select services to %s:\n", actionLabel)
		fmt.Fprintf(v, "Selected: %d of %d\n\n", len(selected), len(gui.containerList))

		for i, service := range gui.containerList {
			cursor := "  "
			if i == gui.containerIndex {
				cursor = "> "
			}

			mark := "[ ]"
			if gui.containerSelect[service] {
				mark = "[x]"
			}

			status := gui.containerStatus[service]
			if status == "" {
				status = "unknown"
			}
			fmt.Fprintf(v, "%s%s %-18s (%s)\n", cursor, mark, service, status)
		}

		actionVerb := "Start"
		if actionLabel == "stop" {
			actionVerb = "Stop"
		}
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "Space: toggle  |  a: all  |  n: none")
		fmt.Fprintf(v, "Enter: %s selected  |  Esc: cancel\n", actionVerb)
		return
	}
	if gui.modalType == "projectActions" {
		fmt.Fprintln(v, "Select an action:")
		fmt.Fprintln(v, "")

		start, end := gui.projectActionsViewport(v)
		for i := start; i < end; i++ {
			action := gui.projectModalActions[i]
			cursor := "  "
			if i == gui.projectModalIndex {
				cursor = "> "
			}
			fmt.Fprintf(v, "%s%3d. %s\n", cursor, i+1, action.label)
		}

		fmt.Fprintln(v, "")
		total := len(gui.projectModalActions)
		if total > 0 {
			selected := clampSelection(gui.projectModalIndex, total) + 1
			remaining := total - selected
			if remaining < 0 {
				remaining = 0
			}
			fmt.Fprintf(v, "Selected: %d/%d  Remaining: %d\n", selected, total, remaining)
			fmt.Fprintf(v, "Showing:  %d-%d of %d\n", start+1, end, total)
		}
		if gui.projectModalNumber != "" {
			fmt.Fprintf(v, "Jump #: %s\n", gui.projectModalNumber)
		}
		editHint := ""
		if isProjectTasksModalTitle(gui.modalTitle) {
			editHint = "e:edit tasks file"
		} else if total > 0 {
			idx := clampSelection(gui.projectModalIndex, total)
			if isEditableProjectAction(gui.projectModalActions[idx]) {
				editHint = "e:edit selected"
			}
		}
		if editHint != "" {
			fmt.Fprintf(v, "Enter: run action  |  %s  |  0-9:jump  g/G:top/bottom  Ctrl+d/u:half-page  |  Esc: cancel\n", editHint)
		} else {
			fmt.Fprintln(v, "Enter: run action  |  0-9:jump  g/G:top/bottom  Ctrl+d/u:half-page  |  Esc: cancel")
		}
		return
	}
	if gui.modalType == "outputTabs" {
		fmt.Fprintln(v, "Select output tab:")
		fmt.Fprintln(v, "")

		for i, id := range gui.outputTabModalIDs {
			tab, ok := gui.outputTabs[id]
			if !ok {
				continue
			}
			cursor := "  "
			if i == gui.outputTabModalIndex {
				cursor = "> "
			}
			current := " "
			if id == gui.outputTab {
				current = "*"
			}
			fmt.Fprintf(v, "%s%s [%s] %s\n", cursor, current, outputRouteLabel(tab.route), tab.title)
		}

		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "Enter: switch tab  |  g/G:top/bottom  |  Esc: cancel")
		return
	}
	if gui.modalType == "help" {
		fmt.Fprintln(v, gui.modalMessage)
		fmt.Fprintln(v, "")
		fmt.Fprintln(v, "j/k or Up/Down: scroll  |  Ctrl+d/u: page  |  g/G: top/bottom  |  Enter/Esc/q: close")
		return
	}

	fmt.Fprintln(v, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(v, "  j/k or ↑↓: Navigate  │  e or Enter: Edit field  │  Ctrl+S: Save  │  Esc: Cancel")
	fmt.Fprintln(v, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(v)

	for i, field := range gui.modalFields {
		name := field["name"].(string)
		fieldType := field["type"].(string)

		// Determine if required
		required := false
		if null, ok := field["null"].(bool); ok && !null {
			if blank, ok := field["blank"].(bool); ok && !blank {
				required = true
			}
		}

		// Get constraints
		constraints := gui.getFieldConstraints(field)

		// Selection indicator
		prefix := "   "
		if i == gui.modalFieldIdx {
			prefix = " > "
		}

		value := gui.modalValues[name]
		if value == "" {
			if required {
				value = "<required>"
			} else {
				value = "<empty>"
			}
		}

		// Color/style based on state
		requiredMark := ""
		if required {
			requiredMark = " *"
		}

		// Format field display
		fmt.Fprintf(v, "%s%-20s %-15s%s\n", prefix, name, "["+fieldType+"]", requiredMark)

		// Show current value with better formatting
		if i == gui.modalFieldIdx {
			fmt.Fprintf(v, "     ╰─> %s\n", value)
			if constraints != "" {
				fmt.Fprintf(v, "         %s\n", constraints)
			}
		} else {
			fmt.Fprintf(v, "     %s\n", value)
		}
		fmt.Fprintln(v)
	}

	fmt.Fprintln(v, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(v, "  * = required field")
	if gui.modalMessage != "" {
		fmt.Fprintln(v)
		fmt.Fprintf(v, "Error: %s\n", gui.modalMessage)
		fmt.Fprintln(v, "Fix fields and press Ctrl+S to save.")
	}
}

// setModalKeybindings sets up keybindings when modal is open
func (gui *Gui) setModalKeybindings() {
	if os.Getenv("DEBUG") == "1" {
		log.Printf("setModalKeybindings: modalType=%s, numFields=%d", gui.modalType, len(gui.modalFields))
	}
	gui.g.DeleteKeybindings(ModalWindow)
	// Keep ModalInputWindow bindings while an input/picker is open.
	if _, err := gui.g.View(ModalInputWindow); err != nil {
		gui.g.DeleteKeybindings(ModalInputWindow)
	}

	gui.g.SetKeybinding(ModalWindow, gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.closeModal()
	})

	gui.g.SetKeybinding(ModalWindow, 'q', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.closeModal()
	})

	if gui.modalType == "delete" {
		gui.g.SetKeybinding(ModalWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return gui.submitModal()
		})
		return
	}

	if gui.modalType == "restore" {
		gui.g.SetKeybinding(ModalWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return gui.submitModal()
		})
		gui.g.SetKeybinding(ModalWindow, 'j', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.restoreSnapshots) == 0 {
				return nil
			}
			gui.restoreIndex = (gui.restoreIndex + 1) % len(gui.restoreSnapshots)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'k', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.restoreSnapshots) == 0 {
				return nil
			}
			gui.restoreIndex--
			if gui.restoreIndex < 0 {
				gui.restoreIndex = len(gui.restoreSnapshots) - 1
			}
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowDown, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.restoreSnapshots) == 0 {
				return nil
			}
			gui.restoreIndex = (gui.restoreIndex + 1) % len(gui.restoreSnapshots)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowUp, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.restoreSnapshots) == 0 {
				return nil
			}
			gui.restoreIndex--
			if gui.restoreIndex < 0 {
				gui.restoreIndex = len(gui.restoreSnapshots) - 1
			}
			return nil
		})
		return
	}
	if gui.modalType == "containers" {
		gui.g.SetKeybinding(ModalWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return gui.submitModal()
		})
		gui.g.SetKeybinding(ModalWindow, 'j', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.containerList) == 0 {
				return nil
			}
			gui.containerIndex = (gui.containerIndex + 1) % len(gui.containerList)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'k', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.containerList) == 0 {
				return nil
			}
			gui.containerIndex--
			if gui.containerIndex < 0 {
				gui.containerIndex = len(gui.containerList) - 1
			}
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowDown, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.containerList) == 0 {
				return nil
			}
			gui.containerIndex = (gui.containerIndex + 1) % len(gui.containerList)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowUp, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.containerList) == 0 {
				return nil
			}
			gui.containerIndex--
			if gui.containerIndex < 0 {
				gui.containerIndex = len(gui.containerList) - 1
			}
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeySpace, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.toggleContainerSelectionAtCurrent()
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'a', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.setAllContainerSelection(true)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'n', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.setAllContainerSelection(false)
			return nil
		})
		return
	}
	if gui.modalType == "projectActions" {
		gui.g.SetKeybinding(ModalWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.clearProjectModalNumberInput()
			return gui.submitModal()
		})
		gui.g.SetKeybinding(ModalWindow, 'e', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.clearProjectModalNumberInput()
			return gui.editSelectedProjectModalAction()
		})
		for i := '0'; i <= '9'; i++ {
			digit := i
			gui.g.SetKeybinding(ModalWindow, digit, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
				gui.appendProjectModalNumberInput(digit)
				return nil
			})
		}
		gui.g.SetKeybinding(ModalWindow, gocui.KeyBackspace, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.projectModalNumber) == 0 {
				return nil
			}
			gui.projectModalNumber = gui.projectModalNumber[:len(gui.projectModalNumber)-1]
			if gui.projectModalNumber == "" {
				return nil
			}
			value, err := strconv.Atoi(gui.projectModalNumber)
			if err != nil || value < 1 || value > len(gui.projectModalActions) {
				gui.projectModalNumber = ""
				return nil
			}
			gui.projectModalIndex = value - 1
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyBackspace2, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.projectModalNumber) == 0 {
				return nil
			}
			gui.projectModalNumber = gui.projectModalNumber[:len(gui.projectModalNumber)-1]
			if gui.projectModalNumber == "" {
				return nil
			}
			value, err := strconv.Atoi(gui.projectModalNumber)
			if err != nil || value < 1 || value > len(gui.projectModalActions) {
				gui.projectModalNumber = ""
				return nil
			}
			gui.projectModalIndex = value - 1
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'j', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.moveProjectModalSelection(1)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'k', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.moveProjectModalSelection(-1)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowDown, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.moveProjectModalSelection(1)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowUp, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.moveProjectModalSelection(-1)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyPgdn, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if v == nil {
				return nil
			}
			_, h := v.Size()
			step := h - 4
			if step < 1 {
				step = 1
			}
			gui.moveProjectModalSelection(step)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyPgup, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if v == nil {
				return nil
			}
			_, h := v.Size()
			step := h - 4
			if step < 1 {
				step = 1
			}
			gui.moveProjectModalSelection(-step)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyCtrlD, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if v == nil {
				return nil
			}
			_, h := v.Size()
			step := h / 2
			if step < 1 {
				step = 1
			}
			gui.moveProjectModalSelection(step)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyCtrlU, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if v == nil {
				return nil
			}
			_, h := v.Size()
			step := h / 2
			if step < 1 {
				step = 1
			}
			gui.moveProjectModalSelection(-step)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'g', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.clearProjectModalNumberInput()
			gui.moveProjectModalSelection(-len(gui.projectModalActions))
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'G', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.clearProjectModalNumberInput()
			gui.moveProjectModalSelection(len(gui.projectModalActions))
			return nil
		})
		return
	}
	if gui.modalType == "outputTabs" {
		gui.g.SetKeybinding(ModalWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return gui.submitModal()
		})
		gui.g.SetKeybinding(ModalWindow, 'j', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.outputTabModalIDs) == 0 {
				return nil
			}
			gui.outputTabModalIndex = (gui.outputTabModalIndex + 1) % len(gui.outputTabModalIDs)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'k', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.outputTabModalIDs) == 0 {
				return nil
			}
			gui.outputTabModalIndex--
			if gui.outputTabModalIndex < 0 {
				gui.outputTabModalIndex = len(gui.outputTabModalIDs) - 1
			}
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowDown, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.outputTabModalIDs) == 0 {
				return nil
			}
			gui.outputTabModalIndex = (gui.outputTabModalIndex + 1) % len(gui.outputTabModalIDs)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowUp, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.outputTabModalIDs) == 0 {
				return nil
			}
			gui.outputTabModalIndex--
			if gui.outputTabModalIndex < 0 {
				gui.outputTabModalIndex = len(gui.outputTabModalIDs) - 1
			}
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'g', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.outputTabModalIDs) == 0 {
				return nil
			}
			gui.outputTabModalIndex = 0
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'G', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if len(gui.outputTabModalIDs) == 0 {
				return nil
			}
			gui.outputTabModalIndex = len(gui.outputTabModalIDs) - 1
			return nil
		})
		return
	}
	if gui.modalType == "help" {
		gui.g.SetKeybinding(ModalWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return gui.closeModal()
		})
		scroll := func(delta int) func(*gocui.Gui, *gocui.View) error {
			return func(g *gocui.Gui, v *gocui.View) error {
				if v == nil {
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
		}
		gui.g.SetKeybinding(ModalWindow, 'j', gocui.ModNone, scroll(1))
		gui.g.SetKeybinding(ModalWindow, 'k', gocui.ModNone, scroll(-1))
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowDown, gocui.ModNone, scroll(1))
		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowUp, gocui.ModNone, scroll(-1))
		gui.g.SetKeybinding(ModalWindow, gocui.KeyCtrlD, gocui.ModNone, scroll(8))
		gui.g.SetKeybinding(ModalWindow, gocui.KeyCtrlU, gocui.ModNone, scroll(-8))
		gui.g.SetKeybinding(ModalWindow, 'g', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if v == nil {
				return nil
			}
			ox, _ := v.Origin()
			_ = v.SetOrigin(ox, 0)
			return nil
		})
		gui.g.SetKeybinding(ModalWindow, 'G', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			if v == nil {
				return nil
			}
			ox, oy := v.Origin()
			_, h := v.Size()
			step := h
			if step < 1 {
				step = 1
			}
			_ = v.SetOrigin(ox, oy+step)
			return nil
		})
		return
	}

	// Add/Edit form modal bindings
	gui.g.SetKeybinding(ModalWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.editModalField()
	})
	gui.g.SetKeybinding(ModalWindow, gocui.KeyCtrlS, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		return gui.submitModal()
	})

	if gui.modalType == "add" || gui.modalType == "edit" {
		gui.g.SetKeybinding(ModalWindow, 'j', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.modalFieldIdx = (gui.modalFieldIdx + 1) % len(gui.modalFields)
			return nil
		})

		gui.g.SetKeybinding(ModalWindow, 'k', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.modalFieldIdx--
			if gui.modalFieldIdx < 0 {
				gui.modalFieldIdx = len(gui.modalFields) - 1
			}
			return nil
		})

		gui.g.SetKeybinding(ModalWindow, gocui.KeyTab, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.modalFieldIdx = (gui.modalFieldIdx + 1) % len(gui.modalFields)
			return nil
		})

		gui.g.SetKeybinding(ModalWindow, gocui.KeyBacktab, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.modalFieldIdx--
			if gui.modalFieldIdx < 0 {
				gui.modalFieldIdx = len(gui.modalFields) - 1
			}
			return nil
		})

		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowDown, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.modalFieldIdx = (gui.modalFieldIdx + 1) % len(gui.modalFields)
			return nil
		})

		gui.g.SetKeybinding(ModalWindow, gocui.KeyArrowUp, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			gui.modalFieldIdx--
			if gui.modalFieldIdx < 0 {
				gui.modalFieldIdx = len(gui.modalFields) - 1
			}
			return nil
		})

		gui.g.SetKeybinding(ModalWindow, 'e', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return gui.editModalField()
		})

		// Space to toggle boolean fields
		gui.g.SetKeybinding(ModalWindow, gocui.KeySpace, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			return gui.toggleBooleanField()
		})
	}
}

type pickerOption struct {
	Value string
	Label string
}

func (gui *Gui) extractChoiceOptions(field map[string]interface{}) []pickerOption {
	choices, ok := field["choices"].([]interface{})
	if !ok || len(choices) == 0 {
		return nil
	}

	options := make([]pickerOption, 0, len(choices))
	for _, rawChoice := range choices {
		choiceMap, ok := rawChoice.(map[string]interface{})
		if !ok {
			continue
		}

		rawValue, ok := choiceMap["value"]
		if !ok {
			continue
		}
		value := strings.TrimSpace(fmt.Sprintf("%v", rawValue))
		if rawValue == nil {
			value = ""
		}

		label := value
		if rawLabel, ok := choiceMap["label"]; ok {
			label = strings.TrimSpace(fmt.Sprintf("%v", rawLabel))
		}
		if label == "" && value == "" {
			label = "<empty>"
		}

		options = append(options, pickerOption{
			Value: value,
			Label: label,
		})
	}

	return options
}

func pickerOptionIndexByValue(options []pickerOption, value string) int {
	needle := strings.TrimSpace(value)
	for i, option := range options {
		if strings.TrimSpace(option.Value) == needle {
			return i
		}
	}
	return -1
}

func pickerOptionsContainValue(options []pickerOption, value string) bool {
	return pickerOptionIndexByValue(options, value) >= 0
}

func appendPickerOptionsUnique(existing []pickerOption, incoming []pickerOption) []pickerOption {
	if len(incoming) == 0 {
		return existing
	}
	seen := make(map[string]struct{}, len(existing))
	for _, option := range existing {
		seen[option.Value] = struct{}{}
	}
	for _, option := range incoming {
		if _, ok := seen[option.Value]; ok {
			continue
		}
		existing = append(existing, option)
		seen[option.Value] = struct{}{}
	}
	return existing
}

func pickerOptionsSummary(options []pickerOption, max int) string {
	if max < 1 {
		max = 1
	}
	labels := make([]string, 0, len(options))
	for _, option := range options {
		label := strings.TrimSpace(option.Label)
		if label == "" {
			label = strings.TrimSpace(option.Value)
		}
		if label == "" {
			label = "<empty>"
		}
		labels = append(labels, label)
		if len(labels) >= max {
			break
		}
	}
	if len(labels) == 0 {
		return ""
	}
	if len(options) > max {
		return strings.Join(labels, ", ") + ", ..."
	}
	return strings.Join(labels, ", ")
}

func (gui *Gui) fieldAllowsEmpty(field map[string]interface{}) bool {
	if null, ok := field["null"].(bool); ok && null {
		return true
	}
	if blank, ok := field["blank"].(bool); ok && blank {
		return true
	}
	return false
}

// editModalField opens an input prompt for the current field
func (gui *Gui) editModalField() error {
	if os.Getenv("DEBUG") == "1" {
		log.Printf("editModalField: modalFieldIdx=%d, len(fields)=%d", gui.modalFieldIdx, len(gui.modalFields))
	}
	if gui.modalFieldIdx >= len(gui.modalFields) {
		return nil
	}
	gui.modalMessage = ""

	field := gui.modalFields[gui.modalFieldIdx]
	fieldName := field["name"].(string)
	fieldType := field["type"].(string)
	currentValue := gui.modalValues[fieldName]

	// Choices should be selected from allowed values only.
	if choiceOptions := gui.extractChoiceOptions(field); len(choiceOptions) > 0 {
		return gui.showChoicePicker(field, fieldName, currentValue, choiceOptions)
	}

	// Check if this is a ForeignKey field - show related records
	if fieldType == "ForeignKey" {
		return gui.showForeignKeyPicker(field, fieldName, currentValue)
	}

	gui.g.DeleteView(ModalInputWindow)

	maxX, maxY := gui.g.Size()
	inputWidth := 60
	inputHeight := 3
	x0 := (maxX - inputWidth) / 2
	y0 := (maxY - inputHeight) / 2

	v, err := gui.g.SetView(ModalInputWindow, x0, y0, x0+inputWidth, y0+inputHeight, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}

	v.Title = fmt.Sprintf(" Enter value for: %s ", fieldName)
	v.Editable = true
	v.Wrap = false
	v.Clear()
	fmt.Fprint(v, currentValue)
	v.SetCursor(len(currentValue), 0)

	gui.g.SetCurrentView(ModalInputWindow)
	gui.g.SetViewOnTop(ModalInputWindow)
	gui.g.DeleteKeybindings(ModalInputWindow)

	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		value := strings.TrimSpace(v.Buffer())
		gui.modalValues[fieldName] = value
		gui.modalMessage = ""
		g.DeleteView(ModalInputWindow)
		g.SetCurrentView(ModalWindow)
		return nil
	})

	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		g.DeleteView(ModalInputWindow)
		g.SetCurrentView(ModalWindow)
		return nil
	})

	return nil
}

// showForeignKeyPicker displays available related records for ForeignKey selection
func (gui *Gui) showForeignKeyPicker(field map[string]interface{}, fieldName, currentValue string) error {
	// Get related model info from field
	relatedModel, relatedApp := gui.getRelatedModelFromField(field, fieldName)
	if relatedModel == "" {
		// Fallback to regular input if we can't determine related model
		return gui.showRegularInput(fieldName, currentValue)
	}

	// Use current app if related_app not specified
	if relatedApp == "" {
		relatedApp = gui.currentApp
	}
	if gui.project == nil {
		gui.modalMessage = fmt.Sprintf("Cannot load related records for %s: project context missing", fieldName)
		return nil
	}

	viewer := django.NewDataViewer(gui.project)
	pageSize := 100
	page := 1
	totalCount := 0
	hasMore := false
	seenValues := make(map[string]struct{})

	buildOptions := func(records []django.ModelRecord) []pickerOption {
		options := make([]pickerOption, 0, len(records))
		for _, record := range records {
			pk := strings.TrimSpace(fmt.Sprintf("%v", record.PK))
			if pk == "" {
				continue
			}
			if _, exists := seenValues[pk]; exists {
				continue
			}
			seenValues[pk] = struct{}{}
			display := gui.getRecordDisplayString(record)
			options = append(options, pickerOption{
				Value: pk,
				Label: fmt.Sprintf("[%s] %s", pk, display),
			})
		}
		return options
	}

	queryPage := func(pageNumber int) ([]pickerOption, error) {
		result, err := viewer.QueryModel(relatedApp, relatedModel, nil, pageNumber, pageSize)
		if err != nil {
			return nil, err
		}
		if totalCount == 0 {
			totalCount = result.Total
		}
		hasMore = result.HasNext
		return buildOptions(result.Records), nil
	}

	options, err := queryPage(page)
	if err != nil {
		gui.modalMessage = fmt.Sprintf("Failed to load related records for %s: %v", fieldName, err)
		return nil
	}
	page++

	if currentValue != "" && !pickerOptionsContainValue(options, currentValue) {
		// Preserve/edit existing FK values even if they are not in the loaded page.
		if currentRecord, err := viewer.GetRecord(relatedApp, relatedModel, currentValue); err == nil {
			seenValues[currentValue] = struct{}{}
			display := gui.getRecordDisplayString(*currentRecord)
			options = append([]pickerOption{{
				Value: currentValue,
				Label: fmt.Sprintf("[%s] %s (current)", currentValue, display),
			}}, options...)
		}
	}

	loadMore := func() ([]pickerOption, bool, error) {
		if !hasMore {
			return nil, true, nil
		}
		more, err := queryPage(page)
		if err != nil {
			return nil, !hasMore, err
		}
		page++
		return more, !hasMore, nil
	}

	if !hasMore {
		loadMore = nil
	}

	return gui.showValuePicker(
		fieldName,
		fmt.Sprintf(" Select %s (FK: %s.%s) ", fieldName, relatedApp, relatedModel),
		options,
		currentValue,
		gui.fieldAllowsEmpty(field),
		totalCount,
		loadMore,
	)
}

func (gui *Gui) showChoicePicker(field map[string]interface{}, fieldName, currentValue string, options []pickerOption) error {
	return gui.showValuePicker(
		fieldName,
		fmt.Sprintf(" Select %s (choices) ", fieldName),
		options,
		currentValue,
		gui.fieldAllowsEmpty(field),
		len(options),
		nil,
	)
}

func (gui *Gui) showValuePicker(
	fieldName, title string,
	options []pickerOption,
	currentValue string,
	allowEmpty bool,
	totalCount int,
	loadMore func() ([]pickerOption, bool, error),
) error {
	allOptions := make([]pickerOption, 0, len(options)+1)
	if allowEmpty {
		allOptions = append(allOptions, pickerOption{Value: "", Label: "<empty>"})
	}
	allOptions = append(allOptions, options...)

	if len(allOptions) == 0 {
		gui.modalMessage = fmt.Sprintf("No selectable values found for %s", fieldName)
		return nil
	}

	gui.g.DeleteView(ModalInputWindow)

	maxX, maxY := gui.g.Size()
	pickerWidth := 78
	pickerHeight := 20
	x0 := (maxX - pickerWidth) / 2
	y0 := (maxY - pickerHeight) / 2

	v, err := gui.g.SetView(ModalInputWindow, x0, y0, x0+pickerWidth, y0+pickerHeight, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}

	searchMode := false
	searchQuery := ""
	statusMessage := ""
	hasMore := loadMore != nil
	filtered := make([]pickerOption, 0, len(allOptions))
	selected := 0 // selected index inside filtered list
	offset := 0

	filterOptions := func(query string) []pickerOption {
		query = strings.ToLower(strings.TrimSpace(query))
		if query == "" {
			out := make([]pickerOption, len(allOptions))
			copy(out, allOptions)
			return out
		}
		out := make([]pickerOption, 0, len(allOptions))
		for _, option := range allOptions {
			label := strings.ToLower(strings.TrimSpace(option.Label))
			value := strings.ToLower(strings.TrimSpace(option.Value))
			if strings.Contains(label, query) || strings.Contains(value, query) {
				out = append(out, option)
			}
		}
		return out
	}

	rebuildFiltered := func(preferredValue string) {
		filtered = filterOptions(searchQuery)
		if len(filtered) == 0 {
			selected = 0
			offset = 0
			return
		}
		if preferredValue != "" {
			if idx := pickerOptionIndexByValue(filtered, preferredValue); idx >= 0 {
				selected = idx
			}
		}
		if selected < 0 {
			selected = 0
		}
		if selected >= len(filtered) {
			selected = len(filtered) - 1
		}
	}

	selectedValue := func() string {
		if len(filtered) == 0 {
			return ""
		}
		if selected < 0 || selected >= len(filtered) {
			return ""
		}
		return filtered[selected].Value
	}

	rebuildFiltered(currentValue)

	render := func() {
		v.Clear()
		v.Title = title
		nullable := "no"
		if allowEmpty {
			nullable = "yes"
		}
		mode := "nav"
		if searchMode {
			mode = "search"
		}
		loadedCount := len(allOptions)
		if allowEmpty && loadedCount > 0 {
			loadedCount--
		}
		loadedLabel := fmt.Sprintf("%d", loadedCount)
		if totalCount > 0 {
			loadedLabel = fmt.Sprintf("%d/%d", loadedCount, totalCount)
		} else if hasMore {
			loadedLabel = fmt.Sprintf("%d+", loadedCount)
		}
		selLabel := "0/0"
		if len(filtered) > 0 {
			selLabel = fmt.Sprintf("%d/%d", selected+1, len(filtered))
		}
		queryDisplay := searchQuery
		if searchMode {
			queryDisplay += "_"
		}

		hints := []string{"j/k or ↑↓ move", "Enter select", "/ search", "Esc close/search"}
		if loadMore != nil || hasMore {
			hints = append(hints, "n load more")
		}
		if allowEmpty {
			hints = append(hints, "x clear")
		}
		fmt.Fprintf(v, "Use %s\n", strings.Join(hints, " | "))
		fmt.Fprintf(v, "Mode:%s  Search:/%s  Selected:%s  Loaded:%s  Nullable:%s\n", mode, queryDisplay, selLabel, loadedLabel, nullable)
		if value := selectedValue(); value != "" {
			fmt.Fprintf(v, "Value: %s\n", value)
		} else {
			fmt.Fprintln(v, "Value: <empty>")
		}
		if statusMessage != "" {
			fmt.Fprintln(v, statusMessage)
		}
		fmt.Fprintln(v, "────────────────────────────────────────────────────────────────────────")

		_, h := v.Size()
		headerLines := 4
		if statusMessage != "" {
			headerLines++
		}
		visible := h - headerLines
		if visible < 1 {
			visible = 1
		}
		if len(filtered) == 0 {
			fmt.Fprintln(v, "  No matches. Press / to edit search, Backspace to broaden.")
			return
		}
		if selected < offset {
			offset = selected
		}
		if selected >= offset+visible {
			offset = selected - visible + 1
		}
		if offset < 0 {
			offset = 0
		}
		maxOffset := len(filtered) - visible
		if maxOffset < 0 {
			maxOffset = 0
		}
		if offset > maxOffset {
			offset = maxOffset
		}

		end := offset + visible
		if end > len(filtered) {
			end = len(filtered)
		}
		for i := offset; i < end; i++ {
			cursor := "  "
			if i == selected {
				cursor = "> "
			}
			option := filtered[i]
			label := strings.TrimSpace(option.Label)
			if label == "" {
				label = option.Value
			}
			if label != option.Value && option.Value != "" {
				fmt.Fprintf(v, "%s%s (%s)\n", cursor, label, option.Value)
			} else {
				fmt.Fprintf(v, "%s%s\n", cursor, label)
			}
		}
	}

	moveSelection := func(delta int) {
		if len(filtered) == 0 {
			selected = 0
			offset = 0
			render()
			return
		}
		selected += delta
		if selected < 0 {
			selected = 0
		}
		if selected >= len(filtered) {
			selected = len(filtered) - 1
		}
		render()
	}

	appendSearchChar := func(char rune) {
		if !searchMode {
			return
		}
		statusMessage = ""
		prev := selectedValue()
		searchQuery += string(char)
		rebuildFiltered(prev)
		render()
	}

	removeSearchChar := func() {
		if !searchMode {
			return
		}
		statusMessage = ""
		prev := selectedValue()
		runes := []rune(searchQuery)
		if len(runes) == 0 {
			return
		}
		searchQuery = string(runes[:len(runes)-1])
		rebuildFiltered(prev)
		render()
	}

	loadMoreOptions := func() {
		if loadMore == nil {
			statusMessage = "No more records to load."
			render()
			return
		}
		prev := selectedValue()
		moreOptions, done, err := loadMore()
		if err != nil {
			statusMessage = fmt.Sprintf("Load failed: %v", err)
			render()
			return
		}
		if done {
			hasMore = false
		}
		if len(moreOptions) > 0 {
			allOptions = appendPickerOptionsUnique(allOptions, moreOptions)
			rebuildFiltered(prev)
			statusMessage = fmt.Sprintf("Loaded %d more record(s).", len(moreOptions))
		} else if hasMore {
			statusMessage = "No additional records loaded."
		} else {
			statusMessage = "No more records."
		}
		render()
	}

	bindSearchRune := func(char rune) {
		gui.g.SetKeybinding(ModalInputWindow, char, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
			appendSearchChar(char)
			return nil
		})
	}

	render()

	gui.g.SetCurrentView(ModalInputWindow)
	gui.g.SetViewOnTop(ModalInputWindow)
	gui.g.DeleteKeybindings(ModalInputWindow)

	gui.g.SetKeybinding(ModalInputWindow, 'j', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if searchMode {
			appendSearchChar('j')
			return nil
		}
		moveSelection(1)
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyArrowDown, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		moveSelection(1)
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, 'k', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if searchMode {
			appendSearchChar('k')
			return nil
		}
		moveSelection(-1)
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyArrowUp, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		moveSelection(-1)
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, 'g', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if searchMode {
			appendSearchChar('g')
			return nil
		}
		if len(filtered) > 0 {
			selected = 0
		}
		render()
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, 'G', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if searchMode {
			appendSearchChar('G')
			return nil
		}
		if len(filtered) > 0 {
			selected = len(filtered) - 1
		}
		render()
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, '/', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		searchMode = true
		statusMessage = ""
		render()
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyBackspace, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		removeSearchChar()
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyBackspace2, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		removeSearchChar()
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, gocui.KeySpace, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		appendSearchChar(' ')
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, 'n', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if searchMode {
			appendSearchChar('n')
			return nil
		}
		loadMoreOptions()
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, 'x', gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if searchMode {
			appendSearchChar('x')
			return nil
		}
		if !allowEmpty {
			statusMessage = "Field is not nullable."
			render()
			return nil
		}
		gui.modalValues[fieldName] = ""
		gui.modalMessage = ""
		g.DeleteView(ModalInputWindow)
		g.SetCurrentView(ModalWindow)
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyCtrlU, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if searchMode {
			searchQuery = ""
			statusMessage = ""
			rebuildFiltered(selectedValue())
			render()
		}
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if len(filtered) == 0 {
			statusMessage = "No matching value selected."
			render()
			return nil
		}
		gui.modalValues[fieldName] = filtered[selected].Value
		gui.modalMessage = ""
		g.DeleteView(ModalInputWindow)
		g.SetCurrentView(ModalWindow)
		return nil
	})
	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		if searchMode {
			preferred := selectedValue()
			searchMode = false
			searchQuery = ""
			statusMessage = ""
			rebuildFiltered(preferred)
			render()
			return nil
		}
		g.DeleteView(ModalInputWindow)
		g.SetCurrentView(ModalWindow)
		return nil
	})
	for r := 'a'; r <= 'z'; r++ {
		if strings.ContainsRune("jkgnx", r) {
			continue
		}
		bindSearchRune(r)
	}
	for r := 'A'; r <= 'Z'; r++ {
		if strings.ContainsRune("G", r) {
			continue
		}
		bindSearchRune(r)
	}
	for r := '0'; r <= '9'; r++ {
		bindSearchRune(r)
	}
	for _, r := range []rune{'-', '_', '.', ':', ',', '@'} {
		bindSearchRune(r)
	}

	return nil
}

// showRegularInput shows regular text input modal
func (gui *Gui) showRegularInput(fieldName, currentValue string) error {
	gui.g.DeleteView(ModalInputWindow)

	maxX, maxY := gui.g.Size()
	inputWidth := 60
	inputHeight := 3
	x0 := (maxX - inputWidth) / 2
	y0 := (maxY - inputHeight) / 2

	v, err := gui.g.SetView(ModalInputWindow, x0, y0, x0+inputWidth, y0+inputHeight, 0)
	if err != nil && err != gocui.ErrUnknownView {
		return err
	}

	v.Title = fmt.Sprintf(" Enter value for: %s ", fieldName)
	v.Editable = true
	v.Wrap = false
	v.Clear()
	fmt.Fprint(v, currentValue)
	v.SetCursor(len(currentValue), 0)

	gui.g.SetCurrentView(ModalInputWindow)
	gui.g.SetViewOnTop(ModalInputWindow)
	gui.g.DeleteKeybindings(ModalInputWindow)

	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyEnter, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		value := strings.TrimSpace(v.Buffer())
		gui.modalValues[fieldName] = value
		g.DeleteView(ModalInputWindow)
		g.SetCurrentView(ModalWindow)
		return nil
	})

	gui.g.SetKeybinding(ModalInputWindow, gocui.KeyEsc, gocui.ModNone, func(g *gocui.Gui, v *gocui.View) error {
		g.DeleteView(ModalInputWindow)
		g.SetCurrentView(ModalWindow)
		return nil
	})

	return nil
}

// getRelatedModelFromField extracts the related model name from ForeignKey field
func (gui *Gui) getRelatedModelFromField(field map[string]interface{}, fieldName string) (string, string) {
	// Try to get related model from field metadata
	if relatedModel, ok := field["related_model"].(string); ok {
		relatedApp := ""
		if app, ok := field["related_app"].(string); ok {
			relatedApp = app
		}
		return relatedModel, relatedApp
	}

	// Heuristic: if field ends with _id, try removing _id suffix
	if strings.HasSuffix(fieldName, "_id") {
		modelName := strings.TrimSuffix(fieldName, "_id")
		// Capitalize first letter
		if len(modelName) > 0 {
			return strings.ToUpper(modelName[:1]) + modelName[1:], ""
		}
	}

	return "", ""
}

// getRecordDisplayString creates a readable string representation of a record
func (gui *Gui) getRecordDisplayString(record django.ModelRecord) string {
	// Try common display fields first
	displayFields := []string{"name", "title", "username", "email", "slug"}

	for _, fieldName := range displayFields {
		if value, ok := record.Fields[fieldName]; ok {
			return fmt.Sprintf("%v", value)
		}
	}

	// Deterministic fallback: prefer string-valued fields, then any non-id field.
	keys := make([]string, 0, len(record.Fields))
	for key := range record.Fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if key == "id" || strings.HasSuffix(key, "_id") {
			continue
		}
		if _, ok := record.Fields[key].(string); ok {
			return fmt.Sprintf("%s: %v", key, record.Fields[key])
		}
	}

	for _, key := range keys {
		if key == "id" || strings.HasSuffix(key, "_id") {
			continue
		}
		return fmt.Sprintf("%s: %v", key, record.Fields[key])
	}

	return fmt.Sprintf("Record #%v", record.PK)
}

// closeModal closes the modal and returns to the main view
func (gui *Gui) closeModal() error {
	returnWindow := gui.modalReturnWindow
	if returnWindow == "" {
		returnWindow = MainWindow
	}

	gui.isModalOpen = false
	gui.modalType = ""
	gui.modalReturnWindow = ""
	gui.modalFields = nil
	gui.modalFieldIdx = 0
	gui.modalValues = nil
	gui.modalMessage = ""
	gui.modalTitle = ""
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
	gui.outputTabModalIDs = nil
	gui.outputTabModalIndex = 0

	gui.g.DeleteKeybindings(ModalWindow)
	gui.g.DeleteKeybindings(ModalInputWindow)
	gui.g.DeleteView(ModalWindow)
	gui.g.DeleteView(ModalInputWindow)
	if gui.currentWindow != returnWindow {
		gui.currentWindow = returnWindow
		gui.markStateDirty()
	} else {
		gui.currentWindow = returnWindow
	}
	gui.g.SetCurrentView(returnWindow)
	return nil
}

// submitModal processes the modal submission
func (gui *Gui) submitModal() error {
	if os.Getenv("DEBUG") == "1" {
		log.Printf("submitModal: modalType=%s, values=%+v", gui.modalType, gui.modalValues)
	}
	viewer := django.NewDataViewer(gui.project)

	switch gui.modalType {
	case "add":
		if err := gui.validateModalFieldValues(); err != nil {
			gui.modalMessage = err.Error()
			return nil
		}
		gui.modalMessage = ""

		fields := gui.convertModalFields()
		pk, err := viewer.CreateRecord(gui.currentApp, gui.currentModel, fields)
		if err != nil {
			gui.modalMessage = fmt.Sprintf("Create failed: %v", err)
			return nil
		}

		gui.showMessage("Success", fmt.Sprintf("Created %s.%s #%v successfully!", gui.currentApp, gui.currentModel, pk))
		gui.closeModal()
		return gui.loadAndDisplayRecords()

	case "edit":
		if len(gui.currentRecords) == 0 {
			gui.closeModal()
			return nil
		}
		if err := gui.validateModalFieldValues(); err != nil {
			gui.modalMessage = err.Error()
			return nil
		}
		gui.modalMessage = ""

		selectedRecord := gui.currentRecords[gui.selectedRecordIdx]
		fields := gui.convertModalFields()

		err := viewer.UpdateRecord(gui.currentApp, gui.currentModel, selectedRecord.PK, fields)
		if err != nil {
			gui.modalMessage = fmt.Sprintf("Update failed: %v", err)
			return nil
		}

		gui.showMessage("Success", fmt.Sprintf("Updated %s.%s #%v successfully!", gui.currentApp, gui.currentModel, selectedRecord.PK))
		gui.closeModal()
		return gui.loadAndDisplayRecords()

	case "delete":
		if len(gui.currentRecords) == 0 {
			gui.closeModal()
			return nil
		}

		selectedRecord := gui.currentRecords[gui.selectedRecordIdx]
		err := viewer.DeleteRecord(gui.currentApp, gui.currentModel, selectedRecord.PK)
		if err != nil {
			gui.showMessage("Error", fmt.Sprintf("Failed to delete record: %v", err))
			gui.closeModal()
			return nil
		}

		gui.showMessage("Success", fmt.Sprintf("Deleted %s.%s #%v successfully!", gui.currentApp, gui.currentModel, selectedRecord.PK))
		gui.closeModal()

		if gui.selectedRecordIdx >= len(gui.currentRecords)-1 && gui.selectedRecordIdx > 0 {
			gui.selectedRecordIdx--
		}

		return gui.loadAndDisplayRecords()

	case "restore":
		if len(gui.restoreSnapshots) == 0 || gui.restoreIndex < 0 || gui.restoreIndex >= len(gui.restoreSnapshots) {
			return gui.closeModal()
		}

		snapshot := gui.restoreSnapshots[gui.restoreIndex]
		gui.closeModal()
		tabID := gui.startCommandOutputTab("Restore Snapshot")
		gui.appendOutput(tabID, fmt.Sprintf("Restoring snapshot: %s\n", snapshot.Name))
		gui.appendOutput(tabID, "Please wait...\n")
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)

		go func() {
			sm := django.NewSnapshotManager(gui.project)
			err := sm.RestoreSnapshot(snapshot.ID)
			gui.g.Update(func(g *gocui.Gui) error {
				gui.resetOutput(tabID, "Restore Snapshot")
				if err != nil {
					gui.appendOutput(tabID, fmt.Sprintf("Restore failed: %v\n", err))
					gui.recordSnapshotActivity("restore", snapshot.ID, snapshot.Name, err)
				} else {
					gui.appendOutput(tabID, fmt.Sprintf("Snapshot restored successfully: %s\n", snapshot.Name))
					gui.recordSnapshotActivity("restore", snapshot.ID, snapshot.Name, nil)
					gui.project.DiscoverMigrations()
					if dataView, err := gui.g.View(DataWindow); err == nil {
						gui.renderDataList(dataView)
					}
				}
				gui.refreshOutputView()
				return nil
			})
		}()

		return nil

	case "containers":
		return gui.runContainerSelectionAction()

	case "projectActions":
		if len(gui.projectModalActions) == 0 {
			return gui.closeModal()
		}

		idx := clampSelection(gui.projectModalIndex, len(gui.projectModalActions))
		action := gui.projectModalActions[idx]
		if err := gui.closeModal(); err != nil {
			return err
		}
		return gui.runProjectAction(action)

	case "outputTabs":
		if len(gui.outputTabModalIDs) == 0 {
			return gui.closeModal()
		}

		idx := clampSelection(gui.outputTabModalIndex, len(gui.outputTabModalIDs))
		tabID := gui.outputTabModalIDs[idx]
		if err := gui.closeModal(); err != nil {
			return err
		}
		gui.switchOutputTab(tabID)
		return gui.switchPanel(MainWindow)
	}

	gui.closeModal()
	return nil
}

// validateRequiredFields validates that all required fields have values
func (gui *Gui) validateRequiredFields() error {
	for _, field := range gui.modalFields {
		name := field["name"].(string)
		if null, ok := field["null"].(bool); ok && !null {
			if blank, ok := field["blank"].(bool); ok && !blank {
				if gui.modalValues[name] == "" {
					return fmt.Errorf("field '%s' is required", name)
				}
			}
		}
	}
	return nil
}

func (gui *Gui) validateModalFieldValues() error {
	if err := gui.validateRequiredFields(); err != nil {
		return err
	}
	return gui.validateConstrainedFields()
}

func (gui *Gui) validateConstrainedFields() error {
	var viewer *django.DataViewer

	for _, field := range gui.modalFields {
		name, ok := field["name"].(string)
		if !ok || name == "" {
			continue
		}
		value := strings.TrimSpace(gui.modalValues[name])
		if value == "" {
			continue
		}

		if choiceOptions := gui.extractChoiceOptions(field); len(choiceOptions) > 0 {
			if !pickerOptionsContainValue(choiceOptions, value) {
				return fmt.Errorf("field '%s' must be one of: %s", name, pickerOptionsSummary(choiceOptions, 6))
			}
		}

		fieldType, _ := field["type"].(string)
		if fieldType != "ForeignKey" {
			continue
		}

		relatedModel, relatedApp := gui.getRelatedModelFromField(field, name)
		if strings.TrimSpace(relatedModel) == "" {
			continue
		}
		if strings.TrimSpace(relatedApp) == "" {
			relatedApp = gui.currentApp
		}
		if gui.project == nil {
			return fmt.Errorf("cannot validate foreign key field '%s': project context missing", name)
		}

		if viewer == nil {
			viewer = django.NewDataViewer(gui.project)
		}
		if _, err := viewer.GetRecord(relatedApp, relatedModel, value); err != nil {
			return fmt.Errorf("field '%s' must reference an existing %s.%s record", name, relatedApp, relatedModel)
		}
	}

	return nil
}

// convertModalFields converts modal string values to proper types
func (gui *Gui) convertModalFields() map[string]interface{} {
	fields := make(map[string]interface{})
	for k, v := range gui.modalValues {
		var fieldType string
		for _, field := range gui.modalFields {
			if field["name"].(string) == k {
				fieldType = field["type"].(string)
				break
			}
		}
		fields[k] = gui.convertFieldValue(v, fieldType)
	}
	return fields
}

// getFieldConstraints returns a human-readable constraint description for a field
func (gui *Gui) getFieldConstraints(field map[string]interface{}) string {
	var constraints []string

	if maxLen, ok := field["max_length"].(float64); ok && maxLen > 0 {
		constraints = append(constraints, fmt.Sprintf("max length: %.0f", maxLen))
	}

	if choices, ok := field["choices"].([]interface{}); ok && len(choices) > 0 {
		choiceStrs := []string{}
		for i, choice := range choices {
			if i >= 5 { // Show only first 5 choices
				choiceStrs = append(choiceStrs, "...")
				break
			}
			if choiceMap, ok := choice.(map[string]interface{}); ok {
				if label, ok := choiceMap["label"].(string); ok {
					choiceStrs = append(choiceStrs, label)
				}
			}
		}
		if len(choiceStrs) > 0 {
			constraints = append(constraints, "choices: "+strings.Join(choiceStrs, ", "))
		}
	}

	if unique, ok := field["unique"].(bool); ok && unique {
		constraints = append(constraints, "unique")
	}

	if len(constraints) > 0 {
		return strings.Join(constraints, " | ")
	}
	return ""
}

// toggleBooleanField quickly toggles boolean field values with spacebar
func (gui *Gui) toggleBooleanField() error {
	if gui.modalFieldIdx >= len(gui.modalFields) {
		return nil
	}

	field := gui.modalFields[gui.modalFieldIdx]
	fieldType := field["type"].(string)

	if fieldType == "BooleanField" {
		fieldName := field["name"].(string)
		currentValue := gui.modalValues[fieldName]

		if currentValue == "" || currentValue == "false" || currentValue == "False" || currentValue == "0" {
			gui.modalValues[fieldName] = "true"
		} else {
			gui.modalValues[fieldName] = "false"
		}
		gui.modalMessage = ""
	}

	return nil
}
