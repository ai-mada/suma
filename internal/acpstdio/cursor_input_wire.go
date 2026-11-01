package acpstdio

import (
	"encoding/json"
	"errors"
	"strconv"

	cu "github.com/ai-mada/suma/internal/cursorcontract"
)

func cursorRPCID(raw json.RawMessage) (cu.CursorRPCRequestID, error) {
	var s string
	var id cu.CursorRPCRequestID
	if json.Unmarshal(raw, &s) == nil {
		id = cu.CursorRPCRequestID{Kind: "string", Value: s}
	} else if n, err := strconv.ParseInt(string(raw), 10, 64); err == nil && strconv.FormatInt(n, 10) == string(raw) {
		id = cu.CursorRPCRequestID{Kind: "number", Value: string(raw)}
	}
	return id, cu.ValidateRPCRequestID(id)
}
func cursorShape(m map[string]json.RawMessage, required, optional []string) bool {
	if !closed(m, append(append([]string{}, required...), optional...)...) {
		return false
	}
	for _, key := range required {
		if m[key] == nil || string(m[key]) == "null" {
			return false
		}
	}
	for _, key := range optional {
		if value, ok := m[key]; ok && string(value) == "null" {
			return false
		}
	}
	return true
}
func cursorStrings(m map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		if raw, ok := m[key]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil {
				return false
			}
		}
	}
	return true
}
func cursorTodos(raw json.RawMessage) ([]cu.CursorTodo, error) {
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil || len(values) > cu.MaxTodos {
		return nil, errors.New("cursor_input_invalid")
	}
	out := make([]cu.CursorTodo, 0, len(values))
	for _, value := range values {
		m := object(value)
		if !cursorShape(m, []string{"id", "content", "status"}, nil) {
			return nil, errors.New("cursor_input_invalid")
		}
		var todo cu.CursorTodo
		if json.Unmarshal(value, &todo) != nil {
			return nil, errors.New("cursor_input_invalid")
		}
		out = append(out, todo)
	}
	return out, nil
}

// Native camelCase is validated before projecting to the shared closed source.
// The source digest binds this normalized payload, never raw JSON whitespace.
func cursorPayload(method string, raw json.RawMessage) (*cu.CursorAskQuestion, *cu.CursorCreatePlan, error) {
	invalid := errors.New("cursor_input_invalid")
	m := object(raw)
	if method == cu.AskQuestionMethod {
		if !cursorShape(m, []string{"toolCallId", "questions"}, []string{"title"}) || !cursorStrings(m, "toolCallId", "title") {
			return nil, nil, invalid
		}
		q := &cu.CursorAskQuestion{ToolCallID: stringField(m, "toolCallId"), Title: stringField(m, "title")}
		var questions []json.RawMessage
		if json.Unmarshal(m["questions"], &questions) != nil || len(questions) < 1 || len(questions) > cu.MaxQuestions {
			return nil, nil, invalid
		}
		for _, rawQuestion := range questions {
			v := object(rawQuestion)
			if !cursorShape(v, []string{"id", "prompt", "options", "allowMultiple"}, nil) || !cursorStrings(v, "id", "prompt") {
				return nil, nil, invalid
			}
			var multiple bool
			if json.Unmarshal(v["allowMultiple"], &multiple) != nil {
				return nil, nil, invalid
			}
			item := cu.CursorQuestion{ID: stringField(v, "id"), Prompt: stringField(v, "prompt"), AllowMultiple: multiple}
			var options []json.RawMessage
			if json.Unmarshal(v["options"], &options) != nil || len(options) < 1 || len(options) > cu.MaxOptions {
				return nil, nil, invalid
			}
			for _, rawOption := range options {
				o := object(rawOption)
				if !cursorShape(o, []string{"id", "label"}, nil) || !cursorStrings(o, "id", "label") {
					return nil, nil, invalid
				}
				item.Options = append(item.Options, cu.CursorQuestionOption{ID: stringField(o, "id"), Label: stringField(o, "label")})
			}
			q.Questions = append(q.Questions, item)
		}
		return q, nil, nil
	}
	if method != cu.CreatePlanMethod || !cursorShape(m, []string{"toolCallId", "plan", "todos"}, []string{"name", "overview", "isProject", "phases"}) || !cursorStrings(m, "toolCallId", "plan", "name", "overview") {
		return nil, nil, invalid
	}
	p := &cu.CursorCreatePlan{ToolCallID: stringField(m, "toolCallId"), Name: stringField(m, "name"), Overview: stringField(m, "overview"), Plan: stringField(m, "plan")}
	var err error
	p.Todos, err = cursorTodos(m["todos"])
	if err != nil {
		return nil, nil, err
	}
	if value, ok := m["isProject"]; ok && json.Unmarshal(value, &p.IsProject) != nil {
		return nil, nil, invalid
	}
	if value, ok := m["phases"]; ok {
		var phases []json.RawMessage
		if json.Unmarshal(value, &phases) != nil || phases == nil || len(phases) > cu.MaxPhases {
			return nil, nil, invalid
		}
		for _, rawPhase := range phases {
			v := object(rawPhase)
			if !cursorShape(v, []string{"name", "todos"}, nil) {
				return nil, nil, invalid
			}
			todos, err := cursorTodos(v["todos"])
			if err != nil {
				return nil, nil, err
			}
			p.Phases = append(p.Phases, cu.CursorPlanPhase{Name: stringField(v, "name"), Todos: todos})
		}
	}
	return nil, p, nil
}
