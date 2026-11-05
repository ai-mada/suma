package main

import (
	"reflect"
	"testing"
)

func TestTaskPriorityChangePublicContract(t *testing.T) {
	api, _ := generate()
	paths := toolContractMap(api, "paths")
	preview := toolContractMap(paths, "/task-priority-changes/preview", "post")
	op := toolContractMap(paths, "/task-priority-changes", "post")
	if preview["x-read-only"] != true || preview["x-no-external-effects"] != true || op["x-read-only"] == true {
		t.Fatal("preview must stay read-only and separate from the batch command")
	}
	for _, operation := range []map[string]any{preview, op} {
		if !reflect.DeepEqual(operation["security"], []any{map[string]any{"cookieAuth": []string{}}, map[string]any{"bearerAuth": []string{}}}) ||
			!reflect.DeepEqual(operation["x-disallowed-token-scopes"], []string{"agent", "runner"}) {
			t.Fatal("priority changes require browser or scoped user API authority")
		}
		for _, raw := range operation["parameters"].([]any) {
			if raw.(map[string]any)["in"] == "query" {
				t.Fatal("project scope belongs in the exact JSON command")
			}
		}
	}
	previewBody := toolContractMap(preview, "requestBody", "content", "application/json", "schema")
	if previewBody["$ref"] != "#/components/schemas/TaskPriorityChangePreviewRequest" {
		t.Fatalf("unexpected preview schema: %v", previewBody)
	}
	body := toolContractMap(op, "requestBody", "content", "application/json", "schema")
	if body["$ref"] != "#/components/schemas/TaskPriorityChangeRequest" {
		t.Fatalf("unexpected request schema: %v", body)
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"TaskPriorityChangeSelection", "TaskPriorityChangeRequest", "TaskPriorityChangeReceipt", "TaskPriorityChangeResult", "TaskPriorityChangePreviewRequest", "TaskPriorityChangePreviewItem", "TaskPriorityChangePreview"} {
		if toolContractMap(schemas, model)["additionalProperties"] != false {
			t.Fatalf("%s must reject undeclared fields", model)
		}
	}
	previewFields := toolContractMap(schemas, "TaskPriorityChangePreviewRequest", "properties")
	if _, hasKey := previewFields["idempotency_key"]; hasKey {
		t.Fatal("read-only preview cannot reserve a write operation")
	}
	input := toolContractMap(schemas, "TaskPriorityChangeRequest", "properties")
	tasks := toolContractMap(input, "tasks")
	if tasks["minItems"] != 1 || tasks["maxItems"] != 16 ||
		!reflect.DeepEqual(toolContractMap(input, "priority")["enum"], []string{"urgent", "high", "medium", "low"}) {
		t.Fatal("expected bounded explicit selection and canonical priority values")
	}
}
