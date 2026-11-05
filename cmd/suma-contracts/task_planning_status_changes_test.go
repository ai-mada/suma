package main

import (
	"reflect"
	"testing"
)

func TestTaskPlanningStatusPublicContract(t *testing.T) {
	api, _ := generate()
	paths := toolContractMap(api, "paths")
	preview := toolContractMap(paths, "/task-planning-status-changes/preview", "post")
	apply := toolContractMap(paths, "/task-planning-status-changes", "post")
	if preview["x-read-only"] != true || preview["x-no-external-effects"] != true || apply["x-read-only"] == true {
		t.Fatal("preview must stay read-only and separate from the batch command")
	}
	for _, op := range []map[string]any{preview, apply} {
		if !reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}}) {
			t.Fatal("planning status requires current browser authority")
		}
		for _, raw := range op["parameters"].([]any) {
			if raw.(map[string]any)["in"] == "query" {
				t.Fatal("project scope belongs only in the exact JSON body")
			}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	input := toolContractMap(schemas, "TaskPlanningStatusChangeRequest")
	statuses := toolContractMap(input, "properties", "status")["enum"]
	if input["additionalProperties"] != false ||
		!reflect.DeepEqual(statuses, []string{"draft", "todo", "in_progress", "blocked", "cancelled"}) {
		t.Fatal("completion and approval cannot be authored by a planning status command")
	}
	items := toolContractMap(schemas, "TaskPlanningStatusPreview", "properties", "items")
	if items["minItems"] != 1 || items["maxItems"] != 16 {
		t.Fatal("preview must remain bounded by the explicit task selection")
	}
}
