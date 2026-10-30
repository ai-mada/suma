package main

import "github.com/ai-mada/suma/internal/platform"

func taskHandoffModels() []any {
	return []any{platform.TaskHandoffCreateRequest{}, platform.TaskHandoffCommandRequest{}, platform.TaskHandoffBasis{}, platform.TaskHandoffTeam{}, platform.TaskHandoffRecipient{}, platform.TaskHandoff{}, platform.TaskHandoffPage{}}
}

func taskHandoffContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/task-handoffs", "get", "", "TaskHandoffPage", "200"},
		{"/task-handoffs/{id}", "get", "", "TaskHandoff", "200"},
		{"/task-handoffs", "post", "TaskHandoffCreateRequest", "TaskHandoff", "201"},
		{"/task-handoffs/{id}/commands", "post", "TaskHandoffCommandRequest", "TaskHandoff", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current-authorized task responsibility handoff, distinct from execution and acceptance. One pending proposal per task. Accept requires the designated real human account, unchanged task/requirement/delivery and recipient/team versions, no active or unknown source execution, and explicit invalidation acknowledgement. The same transaction changes owner, invalidates old verification/dependent eligibility, records the decision and operation receipt. No new run or acceptance is created. Return/withdraw can close a stale proposal; current authority precedes every replay."
		if route.input != "" {
			request(op, route.input)
		} else {
			op["x-read-only"] = true
			task := queryParameter("task_id", "Required current-project task.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
			task["required"] = true
			appendQuery(op, task)
			if route.path == "/task-handoffs" {
				appendQuery(op, queryParameter("limit", "Bounded page size, exact decimal integer.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
				appendQuery(op, queryParameter("before", "Existing handoff ID within the same tenant/project/task; stable created_at/id descending keyset.", map[string]any{"type": "string", "maxLength": 64}))
			} else {
				op["description"] = "Read the exact handoff under current project/task authority and recompute its current basis and allowed actions. No historical page scan, recipient directory enumeration, notification read, responsibility acceptance or formal Review. Unknown/duplicate query fields and cross-task bindings are rejected."
			}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for key, item := range attrs {
			value[key] = item
		}
	}
	for _, model := range []string{"TaskHandoffCreateRequest", "TaskHandoffCommandRequest"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
		field(model, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
	}
	for _, name := range []string{"task_id", "to_resource_id"} {
		field("TaskHandoffCreateRequest", name, map[string]any{"minLength": 1, "maxLength": 64})
	}
	field("TaskHandoffCreateRequest", "to_team_id", map[string]any{"maxLength": 64})
	field("TaskHandoffCreateRequest", "expected_task_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("TaskHandoffCommandRequest", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("TaskHandoffCreateRequest", "note", map[string]any{"minLength": 1, "maxLength": 4000, "x-max-utf8-bytes": 4000})
	field("TaskHandoffCommandRequest", "reason", map[string]any{"minLength": 1, "maxLength": 4000, "x-max-utf8-bytes": 4000})
	field("TaskHandoffCommandRequest", "action", map[string]any{"enum": []string{"accept", "return", "withdraw"}})
	field("TaskHandoffCommandRequest", "acknowledge_invalidation", map[string]any{"description": "Must be true for accept; explicitly acknowledges owner change and invalidation of previous execution/acceptance eligibility."})
	field("TaskHandoff", "state", map[string]any{"enum": []string{"pending", "accepted", "returned", "withdrawn"}})
	for _, model := range []string{"TaskHandoff", "TaskHandoffBasis", "TaskHandoffPage", "TaskHandoffRecipient", "TaskHandoffTeam"} {
		for _, value := range toolContractMap(schemas, model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
}
