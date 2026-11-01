package acpstdio

import (
	"encoding/json"
	"errors"
	"strings"
)

func cursorNotification(method string) bool {
	return method == "cursor/update_todos" || method == "cursor/generate_image" || method == "cursor/task"
}
func (d *Driver) cursorObservation(method, session string, raw json.RawMessage) error {
	var value any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return errors.New("acp_update_invalid")
	}
	safe, _ := safeValue(value, d.options.Secrets)
	encoded, err := json.Marshal(safe)
	if err != nil {
		return errors.New("acp_update_invalid")
	}
	// These notifications describe the trusted runtime's activity. They never
	// authorize a task, create a child, acknowledge permission or finish a tool.
	return d.emit(Event{Kind: "notice", SessionID: session, Text: method + ": " + trimText(string(encoded), 4096)})
}

// models is a secondary Cursor observation. configOptions and its exact
// set_config_option response remain the only model-selection authority.
func cursorModelsValid(raw []byte) bool {
	if string(raw) == "null" {
		return true
	}
	m := object(raw)
	if !closed(m, "_meta", "currentModelId", "availableModels") {
		return false
	}
	var current string
	if json.Unmarshal(m["currentModelId"], &current) != nil || current != "" && !token(current, 256) {
		return false
	}
	var values []json.RawMessage
	if json.Unmarshal(m["availableModels"], &values) != nil || values == nil || len(values) > 256 {
		return false
	}
	seen := map[string]bool{}
	for _, rawValue := range values {
		value := object(rawValue)
		id := stringField(value, "modelId")
		if !closed(value, "_meta", "modelId", "name", "description") || !token(id, 256) || seen[id] || !text(stringField(value, "name"), 4096, false) {
			return false
		}
		seen[id] = true
	}
	return true
}
