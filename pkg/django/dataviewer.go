package django

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type commandRunner interface {
	RunCommand(args ...string) (string, error)
}

// DataViewer provides methods for viewing and editing model data
type DataViewer struct {
	project commandRunner
}

// NewDataViewer creates a new data viewer
func NewDataViewer(project commandRunner) *DataViewer {
	return &DataViewer{project: project}
}

// runPythonScript executes Python code and returns parsed JSON result
func (dv *DataViewer) runPythonScript(code string) (map[string]interface{}, error) {
	output, err := dv.project.RunCommand("shell", "-c", code)
	if err != nil {
		out := strings.TrimSpace(output)
		if out == "" {
			return nil, err
		}
		return nil, fmt.Errorf("%w: %s", err, out)
	}

	jsonPayload, err := extractJSONPayload(output)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonPayload), &result); err != nil {
		return nil, fmt.Errorf("failed to parse result: %w", err)
	}

	if errMsg, ok := result["error"]; ok && errMsg != nil {
		msg := strings.TrimSpace(fmt.Sprintf("%v", errMsg))
		if msg != "" {
			return nil, fmt.Errorf("%s", msg)
		}
	}

	return result, nil
}

func extractJSONPayload(output string) (string, error) {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return "", fmt.Errorf("empty response from django shell")
	}

	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}

	lines := strings.Split(trimmed, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if (strings.HasPrefix(line, "{") || strings.HasPrefix(line, "[")) && json.Valid([]byte(line)) {
			return line, nil
		}
	}

	return "", fmt.Errorf("failed to locate JSON in command output")
}

func mapToStruct(input map[string]interface{}, output interface{}) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, output)
}

func pythonLiteral(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return strconv.Quote(fmt.Sprintf("%v", value))
	}
	return string(data)
}

const pythonJSONSafeHelper = `
def _json_safe(value):
    if value is None:
        return None
    if isinstance(value, (str, int, float, bool)):
        return value
    if hasattr(value, 'isoformat'):
        try:
            return value.isoformat()
        except Exception:
            pass
    if hasattr(value, 'pk'):
        try:
            return value.pk
        except Exception:
            pass
    try:
        json.dumps(value)
        return value
    except Exception:
        return str(value)
`

// serializeFieldsCode is a raw Python block; call serializeFieldsCodeWithIndent for context-safe insertion.
const serializeFieldsCode = `fields = {}
for field in model._meta.fields:
    value = getattr(obj, field.name)
    fields[field.name] = _json_safe(value)`

func serializeFieldsCodeWithIndent(spaces int) string {
	return indentPythonBlock(serializeFieldsCode, spaces)
}

func indentPythonBlock(block string, spaces int) string {
	if spaces < 0 {
		spaces = 0
	}
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(block, "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

// ModelRecord represents a single database record
type ModelRecord struct {
	PK     interface{}            `json:"pk"`
	Fields map[string]interface{} `json:"fields"`
	Model  string                 `json:"model"`
}

// QueryResult contains paginated query results
type QueryResult struct {
	Records  []ModelRecord `json:"records"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
	HasNext  bool          `json:"has_next"`
	HasPrev  bool          `json:"has_prev"`
}

// QueryModel retrieves records for a model with pagination and filtering
func (dv *DataViewer) QueryModel(appName, modelName string, filters map[string]string, page, pageSize int) (*QueryResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize

	pythonCmd := fmt.Sprintf(`
import json
from django.apps import apps

%s
try:
    model = apps.get_model(%s, %s)
    qs = model.objects.all()
%s
    total = qs.count()
    records = qs[%d:%d]

    data = []
    for obj in records:
%s
        data.append({'pk': obj.pk, 'model': f'{model._meta.app_label}.{model.__name__}', 'fields': fields})

    print(json.dumps({
        'records': data,
        'total': total,
        'page': %d,
        'page_size': %d,
        'has_next': total > %d,
        'has_prev': %d > 1
    }))
except Exception as e:
    print(json.dumps({'error': str(e)}))
`, pythonJSONSafeHelper, pythonLiteral(appName), pythonLiteral(modelName), dv.buildFilterCode(filters), offset, offset+pageSize,
		serializeFieldsCodeWithIndent(8), page, pageSize, offset+pageSize, page)

	resultMap, err := dv.runPythonScript(pythonCmd)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	var result QueryResult
	if err := mapToStruct(resultMap, &result); err != nil {
		return nil, fmt.Errorf("failed to parse query result: %w", err)
	}

	return &result, nil
}

// GetRecord retrieves a single record by primary key
func (dv *DataViewer) GetRecord(appName, modelName string, pk interface{}) (*ModelRecord, error) {
	pythonCmd := fmt.Sprintf(`
import json
from django.apps import apps

%s
try:
    model = apps.get_model(%s, %s)
    obj = model.objects.get(pk=%s)
%s
    print(json.dumps({'pk': obj.pk, 'model': f'{obj._meta.app_label}.{obj.__class__.__name__}', 'fields': fields}))
except Exception as e:
    print(json.dumps({'error': str(e)}))
`, pythonJSONSafeHelper, pythonLiteral(appName), pythonLiteral(modelName), pythonLiteral(pk), serializeFieldsCodeWithIndent(4))

	result, err := dv.runPythonScript(pythonCmd)
	if err != nil {
		return nil, fmt.Errorf("get record failed: %w", err)
	}

	var record ModelRecord
	if err := mapToStruct(result, &record); err != nil {
		return nil, fmt.Errorf("failed to parse record: %w", err)
	}

	return &record, nil
}

// CreateRecord creates a new record
func (dv *DataViewer) CreateRecord(appName, modelName string, fields map[string]interface{}) (interface{}, error) {
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return nil, err
	}

	pythonCmd := fmt.Sprintf(`
import json
from django.apps import apps

try:
    model = apps.get_model(%s, %s)
    obj = model.objects.create(**json.loads(%s))
    print(json.dumps({'pk': obj.pk, 'success': True}))
except Exception as e:
    print(json.dumps({'error': str(e), 'success': False}))
`, pythonLiteral(appName), pythonLiteral(modelName), pythonLiteral(string(fieldsJSON)))

	result, err := dv.runPythonScript(pythonCmd)
	if err != nil {
		return nil, fmt.Errorf("create failed: %w", err)
	}

	if success, ok := result["success"].(bool); !ok || !success {
		return nil, fmt.Errorf("create failed: %v", result["error"])
	}

	return result["pk"], nil
}

// UpdateRecord updates an existing record
func (dv *DataViewer) UpdateRecord(appName, modelName string, pk interface{}, fields map[string]interface{}) error {
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return err
	}

	pythonCmd := fmt.Sprintf(`
import json
from django.apps import apps

try:
    model = apps.get_model(%s, %s)
    obj = model.objects.get(pk=%s)
    for key, value in json.loads(%s).items():
        setattr(obj, key, value)
    obj.save()
    print(json.dumps({'success': True}))
except Exception as e:
    print(json.dumps({'error': str(e), 'success': False}))
`, pythonLiteral(appName), pythonLiteral(modelName), pythonLiteral(pk), pythonLiteral(string(fieldsJSON)))

	result, err := dv.runPythonScript(pythonCmd)
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	if success, ok := result["success"].(bool); !ok || !success {
		return fmt.Errorf("update failed: %v", result["error"])
	}

	return nil
}

// DeleteRecord deletes a record
func (dv *DataViewer) DeleteRecord(appName, modelName string, pk interface{}) error {
	pythonCmd := fmt.Sprintf(`
import json
from django.apps import apps

try:
    model = apps.get_model(%s, %s)
    model.objects.get(pk=%s).delete()
    print(json.dumps({'success': True}))
except Exception as e:
    print(json.dumps({'error': str(e), 'success': False}))
`, pythonLiteral(appName), pythonLiteral(modelName), pythonLiteral(pk))

	result, err := dv.runPythonScript(pythonCmd)
	if err != nil {
		return fmt.Errorf("delete failed: %w", err)
	}

	if success, ok := result["success"].(bool); !ok || !success {
		return fmt.Errorf("delete failed: %v", result["error"])
	}

	return nil
}

// GetModelFields retrieves field information for a model
func (dv *DataViewer) GetModelFields(appName, modelName string) ([]map[string]interface{}, error) {
	pythonCmd := fmt.Sprintf(`
import json
from django.apps import apps

%s
try:
    model = apps.get_model(%s, %s)
    fields = []
    for field in model._meta.fields:
        info = {
            'name': field.name,
            'type': field.get_internal_type(),
            'null': field.null,
            'blank': field.blank,
            'primary_key': field.primary_key,
            'unique': field.unique,
        }
        if hasattr(field, 'max_length') and field.max_length:
            info['max_length'] = field.max_length
        if hasattr(field, 'choices') and field.choices:
            info['choices'] = [{'value': _json_safe(k), 'label': str(v)} for k, v in field.choices]
        
        # Add related model info for ForeignKey fields
        if field.get_internal_type() == 'ForeignKey':
            related_model = field.related_model
            info['related_model'] = related_model.__name__
            info['related_app'] = related_model._meta.app_label
        
        fields.append(info)
    print(json.dumps({'fields': fields}))
except Exception as e:
    print(json.dumps({'error': str(e)}))
`, pythonJSONSafeHelper, pythonLiteral(appName), pythonLiteral(modelName))

	result, err := dv.runPythonScript(pythonCmd)
	if err != nil {
		return nil, fmt.Errorf("get fields failed: %w", err)
	}

	fieldsData, ok := result["fields"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid fields payload")
	}

	fields := make([]map[string]interface{}, len(fieldsData))
	for i, f := range fieldsData {
		fieldMap, ok := f.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid field payload")
		}
		fields[i] = fieldMap
	}

	return fields, nil
}

// SearchRecords performs a text search across model fields
func (dv *DataViewer) SearchRecords(appName, modelName, searchTerm string, page, pageSize int) (*QueryResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize

	pythonCmd := fmt.Sprintf(`
import json
from django.apps import apps
from django.db.models import Q

%s
try:
    model = apps.get_model(%s, %s)
    query = Q()
    for field in model._meta.fields:
        if field.get_internal_type() in ['CharField', 'TextField']:
            query |= Q(**{f'{field.name}__icontains': %s})

    qs = model.objects.filter(query) if str(query) != '(AND: )' else model.objects.all()
    total = qs.count()
    records = qs[%d:%d]

    data = []
    for obj in records:
%s
        data.append({'pk': obj.pk, 'model': f'{model._meta.app_label}.{model.__name__}', 'fields': fields})

    print(json.dumps({
        'records': data,
        'total': total,
        'page': %d,
        'page_size': %d,
        'has_next': total > %d,
        'has_prev': %d > 1
    }))
except Exception as e:
    print(json.dumps({'error': str(e)}))
`, pythonJSONSafeHelper, pythonLiteral(appName), pythonLiteral(modelName), pythonLiteral(searchTerm), offset, offset+pageSize,
		serializeFieldsCodeWithIndent(8), page, pageSize, offset+pageSize, page)

	resultMap, err := dv.runPythonScript(pythonCmd)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	var result QueryResult
	if err := mapToStruct(resultMap, &result); err != nil {
		return nil, fmt.Errorf("failed to parse search result: %w", err)
	}

	return &result, nil
}

// buildFilterCode generates Django ORM filter code
func (dv *DataViewer) buildFilterCode(filters map[string]string) string {
	if len(filters) == 0 {
		return ""
	}

	filtersJSON, err := json.Marshal(filters)
	if err != nil {
		return ""
	}

	return fmt.Sprintf(
		"    for key, value in json.loads(%s).items():\n        qs = qs.filter(**{key: value})",
		pythonLiteral(string(filtersJSON)),
	)
}

// GetRelatedObjects retrieves related objects for a foreign key or many-to-many field
func (dv *DataViewer) GetRelatedObjects(appName, modelName string, pk interface{}, fieldName string) ([]ModelRecord, error) {
	pythonCmd := fmt.Sprintf(`
import json
from django.apps import apps

%s
try:
    model = apps.get_model(%s, %s)
    obj = model.objects.get(pk=%s)
    related = getattr(obj, %s)
    related_objs = related.all() if hasattr(related, 'all') else [related] if related else []

    data = []
    for rel_obj in related_objs:
        if rel_obj:
%s
            data.append({
                'pk': rel_obj.pk,
                'model': f'{rel_obj._meta.app_label}.{rel_obj._meta.model_name}',
                'fields': fields
            })
    print(json.dumps({'records': data}))
except Exception as e:
    print(json.dumps({'error': str(e)}))
`, pythonJSONSafeHelper, pythonLiteral(appName), pythonLiteral(modelName), pythonLiteral(pk), pythonLiteral(fieldName), serializeFieldsCodeWithIndent(12))

	resultMap, err := dv.runPythonScript(pythonCmd)
	if err != nil {
		return nil, fmt.Errorf("get related failed: %w", err)
	}

	var parsed struct {
		Records []ModelRecord `json:"records"`
	}

	if err := mapToStruct(resultMap, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse related objects: %w", err)
	}

	return parsed.Records, nil
}
