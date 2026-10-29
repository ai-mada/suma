package platform

import (
	"bytes"
	"encoding/json"
)

// Check presence before decoding nullable hours: omission must never turn into
// an intentional unknown override or a partial calendar replacement.
func decodeResourceCalendarSave(m map[string]any) (out ResourceCalendarSaveRequest, err error) {
	bad := func() (ResourceCalendarSaveRequest, error) { return out, invalid("Invalid resource calendar request") }
	for key := range m {
		if key != "project_id" && key != "expected_version" && key != "idempotency_key" && key != "schedule" {
			return bad()
		}
	}
	if err = exactVersion(m, "expected_version", 0); err != nil {
		return out, err
	}
	schedule, ok := m["schedule"].(map[string]any)
	if !ok || len(schedule) != 2 {
		return bad()
	}
	weekdays, ok := schedule["weekdays"].([]any)
	if !ok || len(weekdays) != 7 {
		return bad()
	}
	exceptions, ok := schedule["exceptions"].([]any)
	if !ok {
		return bad()
	}
	for _, entry := range exceptions {
		exception, valid := entry.(map[string]any)
		if !valid {
			return bad()
		}
		if _, present := exception["available_hours"]; !present {
			return bad()
		}
		if _, valid = exception["date"].(string); !valid {
			return bad()
		}
		for key, value := range exception {
			if key != "date" && key != "available_hours" && key != "note" {
				return bad()
			}
			if key == "note" {
				if _, valid = value.(string); !valid {
					return bad()
				}
			}
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return bad()
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&out); err != nil {
		return bad()
	}
	return out, nil
}
