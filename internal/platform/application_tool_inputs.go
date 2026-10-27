package platform

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
)

const applicationToolInputLimit = 512 * 1024
const applicationToolOutputLimit = 64 * 1024

type ApplicationToolPageInput struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type ApplicationToolIDInput struct {
	ID string `json:"id"`
}
type ApplicationToolRequirementInput struct {
	RequirementID string `json:"requirement_id"`
}
type ApplicationToolTaskInput struct {
	TaskID string `json:"task_id"`
}
type ApplicationToolSearchInput struct {
	Query string `json:"query"`
}
type ApplicationToolKnowledgeInput struct {
	References []ProjectKnowledgeReference `json:"references"`
}
type ApplicationToolKnowledgeListInput struct {
	Kind   string `json:"kind,omitempty"`
	State  string `json:"state,omitempty"`
	Query  string `json:"query,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type ApplicationRequirementDraftInput struct {
	Title              string   `json:"title"`
	Description        string   `json:"description,omitempty"`
	Priority           string   `json:"priority,omitempty"`
	OwnerID            string   `json:"owner_id,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	IdempotencyKey     string   `json:"idempotency_key"`
}
type ApplicationTaskDraftInput struct {
	Title              string   `json:"title"`
	Description        string   `json:"description,omitempty"`
	Priority           string   `json:"priority,omitempty"`
	OwnerID            string   `json:"owner_id,omitempty"`
	RequirementID      string   `json:"requirement_id,omitempty"`
	DependsOn          []string `json:"depends_on,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	IdempotencyKey     string   `json:"idempotency_key"`
}
type ApplicationRequirementDesignSaveInput struct {
	RequirementID              string                   `json:"requirement_id"`
	ExpectedVersion            int                      `json:"expected_version"`
	ExpectedRequirementVersion int                      `json:"expected_requirement_version"`
	Content                    RequirementDesignContent `json:"content"`
	Tasks                      []RequirementDesignTask  `json:"tasks"`
	IdempotencyKey             string                   `json:"idempotency_key"`
}
type ApplicationToolDraftResult struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Version   int    `json:"version"`
	Status    string `json:"status"`
}
type ApplicationToolDesignDraftResult struct {
	ProjectID         string `json:"project_id"`
	RequirementID     string `json:"requirement_id"`
	Version           int    `json:"version"`
	LatestRevision    int    `json:"latest_revision"`
	PublishedRevision int    `json:"published_revision"`
	HasDraft          bool   `json:"has_draft"`
}

func applicationInputError() error {
	return &APIError{422, "invalid_tool_arguments", "应用工具参数无效，请核对字段、类型和数量限制", nil}
}

// JSON decoding alone accepts duplicate keys, case-insensitive field aliases and
// null zero values. Validate the original bytes before decoding exact DTOs.
// No argument or parser detail is put in a diagnostic or a log.
func strictApplicationInput(raw json.RawMessage, out any, required ...string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || len(raw) > applicationToolInputLimit || trimmed[0] != '{' {
		return applicationInputError()
	}
	if RejectDuplicateReleaseJSON(raw) != nil {
		return applicationInputError()
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return applicationInputError()
	}
	for _, name := range required {
		if _, exists := fields[name]; !exists {
			return applicationInputError()
		}
	}
	// JSON tags supply exact names, including nested objects; encoding/json's
	// case-insensitive aliases must not evade the declared schema's whitelist.
	if !applicationExactFields(raw, out) {
		return applicationInputError()
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return applicationInputError()
	}
	return nil
}

func applicationExactFields(raw json.RawMessage, out any) bool {
	return applicationFieldsForType(raw, reflect.TypeOf(out))
}
func applicationFieldsForType(raw json.RawMessage, kind reflect.Type) bool {
	if kind == nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	for kind.Kind() == reflect.Pointer {
		kind = kind.Elem()
	}
	switch kind.Kind() {
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return false
		}
		known := map[string]reflect.Type{}
		for i := 0; i < kind.NumField(); i++ {
			field := kind.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name != "" && name != "-" {
				known[name] = field.Type
			}
		}
		for name, value := range fields {
			field, exists := known[name]
			if !exists || !applicationFieldsForType(value, field) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil {
			return false
		}
		for _, value := range values {
			if !applicationFieldsForType(value, kind.Elem()) {
				return false
			}
		}
	}
	return true
}

func applicationHasField(raw json.RawMessage, name string) bool {
	var fields map[string]json.RawMessage
	return json.Unmarshal(raw, &fields) == nil && fields[name] != nil
}

func applicationID(value string) bool {
	return len(value) > 0 && len(value) <= 64 && strings.TrimSpace(value) == value
}
func applicationPage(input ApplicationToolPageInput) error {
	if input.Limit < 0 || input.Limit > 200 || len(input.Cursor) > 4096 {
		return applicationInputError()
	}
	return nil
}
func applicationDraftFields(title, description, priority, owner, key string, criteria []string) error {
	if requireTitle(strings.TrimSpace(title)) != nil || len(title) > 200 || len(description) > 200000 || len(owner) > 64 || len(key) < 8 || len(key) > 128 || len(criteria) > 100 {
		return applicationInputError()
	}
	if priority != "" && priority != "urgent" && priority != "high" && priority != "medium" && priority != "low" {
		return applicationInputError()
	}
	for _, criterion := range criteria {
		if len(criterion) > 1000 {
			return applicationInputError()
		}
	}
	return nil
}

func applicationObject(properties map[string]any, required ...string) map[string]any {
	out := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}
func applicationString(max int) map[string]any {
	return map[string]any{"type": "string", "maxLength": max}
}
func applicationInteger(minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "minimum": minimum, "maximum": maximum}
}
func applicationArray(item map[string]any, maximum int) map[string]any {
	return map[string]any{"type": "array", "items": item, "maxItems": maximum}
}
func applicationPageSchema() map[string]any {
	return applicationObject(map[string]any{"limit": applicationInteger(1, 200), "cursor": applicationString(4096)})
}

// The original task query treats explicit zero as the default page size.
// Keep that contract in both native and MCP adapters; new queries remain strict.
func applicationTaskPageSchema() map[string]any {
	return applicationObject(map[string]any{"limit": applicationInteger(0, 200), "cursor": applicationString(4096)})
}
func applicationDraftSchema(task bool) map[string]any {
	p := map[string]any{"title": applicationString(200), "description": applicationString(200000), "priority": map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}}, "owner_id": applicationString(64), "acceptance_criteria": applicationArray(applicationString(1000), 100), "idempotency_key": map[string]any{"type": "string", "minLength": 8, "maxLength": 128}}
	if task {
		p["requirement_id"] = applicationString(64)
		p["depends_on"] = applicationArray(applicationString(64), 200)
	}
	return applicationObject(p, "title", "idempotency_key")
}
func applicationDesignSchema() map[string]any {
	task := applicationObject(map[string]any{"key": applicationString(64), "title": applicationString(200), "description": applicationString(4000), "priority": map[string]any{"type": "string", "enum": []string{"urgent", "high", "medium", "low"}}, "owner_id": applicationString(64), "depends_on": applicationArray(applicationString(64), 100), "acceptance_criteria": applicationArray(applicationString(1000), 20), "covers": applicationArray(applicationInteger(1, 100), 100)}, "key", "title", "description", "priority", "owner_id", "depends_on", "acceptance_criteria", "covers")
	return applicationObject(map[string]any{"requirement_id": applicationString(64), "expected_version": applicationInteger(0, 2147483646), "expected_requirement_version": applicationInteger(1, 2147483646), "content": applicationObject(map[string]any{"product": applicationString(16000), "experience": applicationString(16000), "technical": applicationString(16000)}, "product", "experience", "technical"), "tasks": applicationArray(task, 100), "idempotency_key": map[string]any{"type": "string", "minLength": 8, "maxLength": 128}}, "requirement_id", "expected_version", "expected_requirement_version", "content", "tasks", "idempotency_key")
}
