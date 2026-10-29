package workbuddy

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// Read an exact, case-sensitive object before selecting typed fields. Unknown
// presentation fields may be ignored, but duplicate names cannot override facts.
func object(raw []byte) (map[string]json.RawMessage, error) {
	if len(raw) > maxResponseBytes || !toolcontract.ValidJSONUnicode(raw) {
		return nil, localError("invalid_response")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, localError("invalid_response")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err = decoder.Token()
		key, ok := token.(string)
		if err != nil || !ok || fields[key] != nil {
			return nil, localError("invalid_response")
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return nil, localError("invalid_response")
		}
		fields[key] = value
	}
	if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
		return nil, localError("invalid_response")
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, localError("invalid_response")
	}
	return fields, nil
}
func stringField(fields map[string]json.RawMessage, key string, required bool, max int) (string, error) {
	raw, exists := fields[key]
	if !exists {
		if required {
			return "", localError("invalid_response")
		}
		return "", nil
	}
	var value string
	if len(raw) == 0 || raw[0] != '"' || json.Unmarshal(raw, &value) != nil || !validText(value, max, false) || required && strings.TrimSpace(value) == "" {
		return "", localError("invalid_response")
	}
	return value, nil
}
func decodeTask(raw []byte) (Task, error) {
	fields, err := object(raw)
	if err != nil {
		return Task{}, err
	}
	var task Task
	task.ID, err = stringField(fields, "task_id", true, 200)
	if err != nil || !validTaskID(task.ID) {
		return Task{}, localError("invalid_response")
	}
	task.Status, err = stringField(fields, "status", true, 128)
	if err != nil {
		return Task{}, err
	}
	task.Name, err = stringField(fields, "name", false, 512)
	if err != nil {
		return Task{}, err
	}
	task.CreatedAt, err = stringField(fields, "created_at", false, 64)
	if err != nil {
		return Task{}, err
	}
	task.UpdatedAt, err = stringField(fields, "updated_at", false, 64)
	if err != nil {
		return Task{}, err
	}
	for _, value := range []string{task.CreatedAt, task.UpdatedAt} {
		if value != "" {
			if _, err = time.Parse(time.RFC3339, value); err != nil {
				return Task{}, localError("invalid_response")
			}
		}
	}
	return task, nil
}
func integerField(fields map[string]json.RawMessage, key string) (int, error) {
	raw, exists := fields[key]
	var value int
	if !exists || bytes.Equal(raw, []byte("null")) || json.Unmarshal(raw, &value) != nil || value < 0 {
		return 0, localError("invalid_response")
	}
	return value, nil
}
func decodePage(raw []byte, page, size int) (TaskPage, error) {
	fields, err := object(raw)
	if err != nil {
		return TaskPage{}, err
	}
	out := TaskPage{Tasks: []Task{}}
	out.Total, err = integerField(fields, "total")
	if err != nil {
		return TaskPage{}, err
	}
	paging, err := object(fields["pagination"])
	if err != nil {
		return TaskPage{}, err
	}
	out.Pagination.Page, err = integerField(paging, "page")
	if err != nil {
		return TaskPage{}, err
	}
	out.Pagination.Size, err = integerField(paging, "size")
	if err != nil {
		return TaskPage{}, err
	}
	out.Pagination.Total, err = integerField(paging, "total")
	if err != nil {
		return TaskPage{}, err
	}
	if out.Pagination.Page != page || out.Pagination.Size != size || out.Pagination.Total != out.Total {
		return TaskPage{}, localError("invalid_response")
	}
	var tasks []json.RawMessage
	taskRaw := fields["tasks"]
	if len(taskRaw) == 0 || taskRaw[0] != '[' || json.Unmarshal(taskRaw, &tasks) != nil || len(tasks) > size {
		return TaskPage{}, localError("invalid_response")
	}
	// int64 arithmetic remains bounded by the validated page and size, including
	// callers on 32-bit targets. A page beyond the total can only be empty.
	remaining := int64(out.Total) - (int64(page)-1)*int64(size)
	if remaining < 0 {
		remaining = 0
	}
	if int64(len(tasks)) > remaining {
		return TaskPage{}, localError("invalid_response")
	}
	seen := map[string]bool{}
	for _, rawTask := range tasks {
		task, e := decodeTask(rawTask)
		if e != nil || seen[task.ID] {
			return TaskPage{}, localError("invalid_response")
		}
		seen[task.ID] = true
		out.Tasks = append(out.Tasks, task)
	}
	return out, nil
}
