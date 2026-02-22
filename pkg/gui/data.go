package gui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/awesome-gocui/gocui"
	"github.com/williamblackie/lazydjango/pkg/django"
)

// loadAndDisplayRecords queries and displays records in a table format
func (gui *Gui) loadAndDisplayRecords() error {
	mainView, err := gui.g.View(MainWindow)
	if err != nil {
		return err
	}

	mainView.Clear()
	gui.setMainTitle(fmt.Sprintf("%s.%s (Page %d)", gui.currentApp, gui.currentModel, gui.currentPage))

	viewer := django.NewDataViewer(gui.project)
	result, err := viewer.QueryModel(gui.currentApp, gui.currentModel, nil, gui.currentPage, gui.pageSize)

	if err != nil {
		mainView.Clear()
		fmt.Fprintf(mainView, "Error loading data: %v\n", err)
		gui.rememberError("model-query", err.Error())
		return nil
	}

	gui.currentRecords = result.Records
	gui.totalRecords = result.Total
	gui.selectedRecordIdx = clampSelection(gui.selectedRecordIdx, len(result.Records))
	var selectedPK interface{}
	if len(gui.currentRecords) > 0 && gui.selectedRecordIdx >= 0 && gui.selectedRecordIdx < len(gui.currentRecords) {
		selectedPK = gui.currentRecords[gui.selectedRecordIdx].PK
	}
	gui.rememberModelAccess(gui.currentApp, gui.currentModel, gui.currentPage, gui.selectedRecordIdx, selectedPK)
	mainView.Clear()

	if len(result.Records) == 0 {
		fmt.Fprintln(mainView, "No records found.")
		fmt.Fprintln(mainView)
		fmt.Fprintln(mainView, "Press 'a' to create a new record.")
		return nil
	}

	// Header
	totalPages := (result.Total + gui.pageSize - 1) / gui.pageSize

	fmt.Fprintf(mainView, "%s.%s - %d total records (Page %d/%d)\n\n",
		gui.currentApp, gui.currentModel, result.Total, gui.currentPage, totalPages)

	// Build table
	fieldNames, colWidths := gui.calculateTableLayout(result.Records)
	gui.printTableHeader(mainView, fieldNames, colWidths)
	gui.printTableRows(mainView, result.Records, fieldNames, colWidths)
	gui.printTableFooter(mainView, result.HasNext)

	return nil
}

// calculateTableLayout determines field names and column widths
func (gui *Gui) calculateTableLayout(records []django.ModelRecord) ([]string, []int) {
	var fieldNames []string
	fieldMap := make(map[string]int)

	if len(records) > 0 {
		fieldNames = append(fieldNames, "ID")
		fieldMap["ID"] = 0
		idx := 1
		keys := make([]string, 0, len(records[0].Fields))
		for key := range records[0].Fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			fieldNames = append(fieldNames, key)
			fieldMap[key] = idx
			idx++
		}
	}

	colWidths := make([]int, len(fieldNames))
	for i, name := range fieldNames {
		colWidths[i] = len(name)
	}

	for _, record := range records {
		idStr := fmt.Sprintf("%v", record.PK)
		if len(idStr) > colWidths[0] {
			colWidths[0] = len(idStr)
		}

		for key, value := range record.Fields {
			valueStr := fmt.Sprintf("%v", value)
			if len(valueStr) > 50 {
				valueStr = valueStr[:47] + "..."
			}
			if len(valueStr) > colWidths[fieldMap[key]] {
				colWidths[fieldMap[key]] = len(valueStr)
			}
		}
	}

	// Cap widths
	for i := range colWidths {
		if colWidths[i] > 50 {
			colWidths[i] = 50
		}
		if colWidths[i] < 3 {
			colWidths[i] = 3
		}
	}

	return fieldNames, colWidths
}

// printTableHeader prints the table header row
func (gui *Gui) printTableHeader(v *gocui.View, fieldNames []string, colWidths []int) {
	fmt.Fprint(v, "  ")
	for i, name := range fieldNames {
		fmt.Fprintf(v, "%-*s", colWidths[i]+2, name)
	}
	fmt.Fprintln(v)

	fmt.Fprint(v, "  ")
	for i := range fieldNames {
		fmt.Fprint(v, strings.Repeat("─", colWidths[i]+2))
	}
	fmt.Fprintln(v)
}

// printTableRows prints data rows with selection indicator
func (gui *Gui) printTableRows(v *gocui.View, records []django.ModelRecord, fieldNames []string, colWidths []int) {
	for i, record := range records {
		if i == gui.selectedRecordIdx {
			fmt.Fprintf(v, "> ")
		} else {
			fmt.Fprintf(v, "  ")
		}

		idStr := fmt.Sprintf("%v", record.PK)
		fmt.Fprintf(v, "%-*s", colWidths[0]+2, idStr)

		for j := 1; j < len(fieldNames); j++ {
			fieldName := fieldNames[j]
			value := record.Fields[fieldName]
			valueStr := fmt.Sprintf("%v", value)

			if len(valueStr) > 50 {
				valueStr = valueStr[:47] + "..."
			}

			fmt.Fprintf(v, "%-*s", colWidths[j]+2, valueStr)
		}
		fmt.Fprintln(v)
	}
}

// printTableFooter prints the footer with controls
func (gui *Gui) printTableFooter(v *gocui.View, hasNext bool) {
	fmt.Fprintln(v, "\n------------------------------------------------------------")
	fmt.Fprint(v, "j/k or J/K: records  |  g/G:first/last  |  a:add  e:edit  d:delete")
	if gui.currentPage > 1 {
		fmt.Fprint(v, "  |  p or Ctrl+u:prev page")
	}
	if hasNext {
		fmt.Fprint(v, "  |  n or Ctrl+d:next page")
	}
	fmt.Fprintln(v)
}

// Navigation functions
func (gui *Gui) nextRecord(g *gocui.Gui, v *gocui.View) error {
	if gui.currentWindow != MainWindow || gui.currentModel == "" || len(gui.currentRecords) == 0 {
		return nil
	}

	if gui.selectedRecordIdx < len(gui.currentRecords)-1 {
		gui.selectedRecordIdx++
		return gui.loadAndDisplayRecords()
	}
	return nil
}

func (gui *Gui) prevRecord(g *gocui.Gui, v *gocui.View) error {
	if gui.currentWindow != MainWindow || gui.currentModel == "" || len(gui.currentRecords) == 0 {
		return nil
	}

	if gui.selectedRecordIdx > 0 {
		gui.selectedRecordIdx--
		return gui.loadAndDisplayRecords()
	}
	return nil
}

func (gui *Gui) nextPage(g *gocui.Gui, v *gocui.View) error {
	if gui.currentWindow != MainWindow || gui.currentModel == "" {
		return nil
	}

	totalPages := (gui.totalRecords + gui.pageSize - 1) / gui.pageSize
	if gui.currentPage < totalPages {
		gui.currentPage++
		gui.selectedRecordIdx = 0
		return gui.loadAndDisplayRecords()
	}
	return nil
}

func (gui *Gui) prevPage(g *gocui.Gui, v *gocui.View) error {
	if gui.currentWindow != MainWindow || gui.currentModel == "" {
		return nil
	}

	if gui.currentPage > 1 {
		gui.currentPage--
		gui.selectedRecordIdx = 0
		return gui.loadAndDisplayRecords()
	}
	return nil
}

// CRUD operations
func (gui *Gui) addRecord(g *gocui.Gui, v *gocui.View) error {
	if gui.currentWindow != MainWindow || gui.currentModel == "" {
		return nil
	}

	viewer := django.NewDataViewer(gui.project)
	fields, err := viewer.GetModelFields(gui.currentApp, gui.currentModel)
	if err != nil {
		gui.showMessage("Error", fmt.Sprintf("Failed to get model fields: %v", err))
		return nil
	}

	editableFields := gui.filterEditableFields(fields)
	if len(editableFields) == 0 {
		gui.showMessage("Info", "No editable fields found. This model only has auto-generated fields.")
		return nil
	}

	gui.openFormModal("add", editableFields, nil)
	return nil
}

func (gui *Gui) editRecord(g *gocui.Gui, v *gocui.View) error {
	if gui.currentWindow != MainWindow || gui.currentModel == "" || len(gui.currentRecords) == 0 {
		return nil
	}

	selectedRecord := gui.currentRecords[gui.selectedRecordIdx]

	viewer := django.NewDataViewer(gui.project)
	fields, err := viewer.GetModelFields(gui.currentApp, gui.currentModel)
	if err != nil {
		gui.showMessage("Error", fmt.Sprintf("Failed to get model fields: %v", err))
		return nil
	}

	editableFields := gui.filterEditableFields(fields)
	if len(editableFields) == 0 {
		gui.showMessage("Info", "No editable fields found.")
		return nil
	}

	currentValues := make(map[string]string)
	for key, value := range selectedRecord.Fields {
		currentValues[key] = fmt.Sprintf("%v", value)
	}

	gui.openFormModal("edit", editableFields, currentValues)
	return nil
}

func (gui *Gui) deleteRecord(g *gocui.Gui, v *gocui.View) error {
	if gui.currentWindow != MainWindow || gui.currentModel == "" || len(gui.currentRecords) == 0 {
		return nil
	}

	selectedRecord := gui.currentRecords[gui.selectedRecordIdx]
	gui.openConfirmModal("delete", selectedRecord)
	return nil
}

// Utility functions
func (gui *Gui) filterEditableFields(fields []map[string]interface{}) []map[string]interface{} {
	var editableFields []map[string]interface{}
	for _, field := range fields {
		if isPrimaryKey, ok := field["primary_key"].(bool); ok && isPrimaryKey {
			continue
		}
		if name, ok := field["name"].(string); ok && name == "id" {
			continue
		}
		editableFields = append(editableFields, field)
	}
	return editableFields
}

func (gui *Gui) convertFieldValue(value string, fieldType string) interface{} {
	if value == "" || value == "null" || value == "None" {
		return nil
	}

	switch fieldType {
	case "BooleanField":
		value = strings.ToLower(strings.TrimSpace(value))
		return value == "true" || value == "1" || value == "yes" || value == "t"

	case "IntegerField", "BigIntegerField", "SmallIntegerField", "PositiveIntegerField", "PositiveSmallIntegerField", "AutoField", "BigAutoField":
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
		return value

	case "FloatField", "DecimalField":
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
		return value

	default:
		return value
	}
}

func (gui *Gui) showMessage(title, message string) error {
	if strings.EqualFold(strings.TrimSpace(title), "Error") {
		gui.rememberError("ui", message)
	}

	if gui.currentModel == "" {
		tabID := gui.startCommandOutputTab(title)
		gui.appendOutput(tabID, message+"\n")
		gui.refreshOutputView()
		_ = gui.switchPanel(MainWindow)
		return nil
	}

	mainView, err := gui.g.View(MainWindow)
	if err != nil {
		return err
	}

	mainView.Clear()
	gui.setMainTitle(title)
	fmt.Fprintln(mainView, message)

	return nil
}
