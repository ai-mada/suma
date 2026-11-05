package main

import (
	"reflect"
	"testing"
)

func TestTaskPriorityChangePublicContract(t *testing.T) {
	api, _ := generate()
	op := toolContractMap(api, "paths", "/task-priority-changes", "post")
	if !reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}}) {
		t.Fatal("task priority changes require browser authority and JSON-only project scope")
	}
	for _, raw := range op["parameters"].([]any) {
		if raw.(map[string]any)["in"] == "query" {
			t.Fatal("project scope belongs in the exact JSON command")
		}
	}
	body := toolContractMap(op, "requestBody", "content", "application/json", "schema")
	if body["$ref"] != "#/components/schemas/TaskPriorityChangeRequest" {
		t.Fatalf("unexpected request schema: %v", body)
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"TaskPriorityChangeSelection", "TaskPriorityChangeRequest", "TaskPriorityChangeReceipt", "TaskPriorityChangeResult"} {
		if toolContractMap(schemas, model)["additionalProperties"] != false {
			t.Fatalf("%s must reject undeclared fields", model)
		}
	}
	input := toolContractMap(schemas, "TaskPriorityChangeRequest", "properties")
	tasks := toolContractMap(input, "tasks")
	if tasks["minItems"] != 1 || tasks["maxItems"] != 16 ||
		!reflect.DeepEqual(toolContractMap(input, "priority")["enum"], []string{"urgent", "high", "medium", "low"}) {
		t.Fatal("expected bounded explicit selection and canonical priority values")
	}
}
