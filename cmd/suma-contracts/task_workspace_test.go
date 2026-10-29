package main

import "testing"

func TestTaskWorkspaceContractIsScopedReadWithoutBusinessMutation(t *testing.T) {
	api, _ := generate()
	path := mapAt(api, "paths", "/task-workspace")
	if len(path) != 1 || path["get"] == nil {
		t.Fatal("task view must expose only GET")
	}
	op := path["get"].(map[string]any)
	if op["x-read-only"] != true || op["requestBody"] != nil {
		t.Fatal("query is not a business mutation")
	}
	parameters := map[string]map[string]any{}
	for _, item := range op["parameters"].([]any) {
		p := item.(map[string]any)
		if p["in"] != "query" {
			continue
		}
		name := p["name"].(string)
		if parameters[name] != nil {
			t.Fatal("duplicate parameter", name)
		}
		parameters[name] = p
	}
	if parameters["project_id"]["required"] != true {
		t.Fatal("project must be explicit")
	}
	for _, name := range []string{"q", "status", "owner_id", "priority", "sort", "limit", "cursor"} {
		if parameters[name] == nil {
			t.Fatal("missing query field", name)
		}
	}
	limit := parameters["limit"]["schema"].(map[string]any)
	if limit["maximum"] != 100 || limit["minimum"] != 1 {
		t.Fatal("bounded page missing")
	}
	if mapAt(op, "responses", "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/TaskWorkspacePage" {
		t.Fatal("incorrect result contract")
	}
	properties := mapAt(api, "components", "schemas", "TaskWorkspacePage", "properties")
	for _, name := range []string{"items", "total", "as_of", "next_cursor"} {
		if properties[name].(map[string]any)["readOnly"] != true {
			t.Fatal("result must be observed", name)
		}
	}
}
