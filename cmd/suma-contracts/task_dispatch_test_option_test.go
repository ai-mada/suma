package main

import "testing"

func TestTaskDispatchTestOptionContract(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/tasks/{id}/dispatch-test-option", "get")
	security := op["security"].([]any)
	if len(security) != 1 || mapAt(security[0])["cookieAuth"] == nil ||
		op["x-read-only"] != true || op["x-response-cache-control"] != "no-store" || op["requestBody"] != nil ||
		responseSchema(op, "200")["$ref"] != "#/components/schemas/TaskDispatchTestOption" {
		t.Fatal("independent test preflight must be an uncached browser-only read")
	}
	expected := map[string]bool{
		"project_id": true, "expected_task_version": true, "workflow_id": true, "workflow_version": true,
		"agent_node_id": true, "test_node_id": true, "development_tool_profile_id": true,
		"development_tool_profile_version": true, "test_tool_profile_id": true, "test_tool_profile_version": true,
	}
	for _, raw := range op["parameters"].([]any) {
		p := mapAt(raw)
		if p["in"] != "query" {
			continue
		}
		name := p["name"].(string)
		if !expected[name] || p["required"] != true {
			t.Fatalf("invalid preflight parameter %#v", p)
		}
		delete(expected, name)
	}
	if len(expected) > 0 {
		t.Fatalf("missing preflight parameters %#v", expected)
	}
	fields := mapAt(api, "components", "schemas", "TaskDispatchTestOption", "properties")
	if mapAt(fields, "binding")["$ref"] != "#/components/schemas/TaskDispatchTestBinding" ||
		mapAt(fields, "checked_at")["format"] != "date-time" {
		t.Fatal("independent test preflight must reuse the exact test binding schema and expose observation time")
	}
}
