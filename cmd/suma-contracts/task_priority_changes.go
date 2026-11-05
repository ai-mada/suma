package main

import "github.com/ai-mada/suma/internal/platform"

func taskPriorityChangeModels() []any {
	return []any{platform.TaskPriorityChangeSelection{}, platform.TaskPriorityChangeRequest{}, platform.TaskPriorityChangeReceipt{}, platform.TaskPriorityChangeResult{}}
}

func taskPriorityChangeContracts(add operationAdder, api map[string]any) {
	op := add("/task-priority-changes", "post", "TaskPriorityChangeResult", false, true)
	browserOnly(op)
	successStatus(op, "200")
	request(op, "TaskPriorityChangeRequest")
	op["parameters"] = []any{}
	op["description"] = "Atomically change the priority of explicit project tasks. Check all original versions under current project write authority, preserve workflow and delivery facts, and return ordered receipts. Replay the original command using the same operation ID and body."
	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"TaskPriorityChangeSelection", "TaskPriorityChangeRequest", "TaskPriorityChangeReceipt", "TaskPriorityChangeResult"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	field := func(model, name string, attributes map[string]any) {
		for key, value := range attributes {
			toolContractMap(schemas, model, "properties", name)[key] = value
		}
	}
	id := map[string]any{"minLength": 1, "maxLength": 64}
	version := map[string]any{"minimum": 1, "maximum": 2147483647}
	priority := map[string]any{"enum": []string{"urgent", "high", "medium", "low"}}
	field("TaskPriorityChangeRequest", "project_id", id)
	field("TaskPriorityChangeRequest", "priority", priority)
	field("TaskPriorityChangeRequest", "tasks", map[string]any{"minItems": 1, "maxItems": 16, "description": "Explicit distinct task IDs in review order."})
	field("TaskPriorityChangeRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	field("TaskPriorityChangeSelection", "task_id", id)
	field("TaskPriorityChangeSelection", "expected_version", version)
	field("TaskPriorityChangeReceipt", "priority", priority)
	field("TaskPriorityChangeReceipt", "previous_version", version)
	field("TaskPriorityChangeReceipt", "version", version)
	field("TaskPriorityChangeResult", "changes", map[string]any{"minItems": 1, "maxItems": 16})
}
