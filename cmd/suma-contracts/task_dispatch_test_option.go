package main

func taskDispatchTestOptionContract(add operationAdder, api map[string]any) {
	op := add("/tasks/{id}/dispatch-test-option", "get", "TaskDispatchTestOption", false, false)
	browserOnly(op)
	successStatus(op, "200")
	op["description"] = "Read compatibility of one explicit independent test binding against the current task, published workflow, development profile and execution route. Requires current browser project write authority and does not create a preview or execution."
	op["x-read-only"], op["x-response-cache-control"] = true, "no-store"
	for _, name := range []string{"expected_task_version", "workflow_version", "development_tool_profile_version", "test_tool_profile_version"} {
		p := queryParameter(name, "Exact current version; canonical positive decimal.", map[string]any{"type": "integer", "minimum": 1, "maximum": 2147483646})
		p["required"] = true
		appendQuery(op, p)
	}
	for _, f := range []struct {
		name string
		max  int
	}{
		{"workflow_id", 100}, {"agent_node_id", 150}, {"test_node_id", 150},
		{"development_tool_profile_id", 64}, {"test_tool_profile_id", 64},
	} {
		p := queryParameter(f.name, "Exact source identity.", map[string]any{"type": "string", "minLength": 1, "maxLength": f.max})
		p["required"] = true
		appendQuery(op, p)
	}
	schema := toolContractMap(api, "components", "schemas", "TaskDispatchTestOption")
	schema["additionalProperties"] = false
	for _, property := range toolContractMap(schema, "properties") {
		property.(map[string]any)["readOnly"] = true
	}
	toolContractMap(schema, "properties", "checked_at")["format"] = "date-time"
}
