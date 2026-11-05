package main

import "github.com/ai-mada/suma/internal/platform"

func taskIterationAssignmentModels() []any {
	return []any{platform.TaskIterationAssignmentSelection{}, platform.TaskIterationAssignmentRequest{}, platform.TaskIterationAssignmentReceipt{}, platform.TaskIterationAssignmentResult{}}
}

func taskIterationAssignmentContracts(add operationAdder, api map[string]any) {
	op := add("/planning/iteration-assignments", "post", "TaskIterationAssignmentResult", false, true)
	successStatus(op, "200")
	request(op, "TaskIterationAssignmentRequest")
	op["parameters"] = []any{}
	op["description"] = "Move explicitly selected tasks to one current project iteration, or remove their iteration assignment. Check task, plan and target versions under current project write authority. Preserve other schedule fields and task acceptance. Skip unchanged rows and replay the original immutable result with the same operation ID."
	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"TaskIterationAssignmentSelection", "TaskIterationAssignmentRequest", "TaskIterationAssignmentReceipt", "TaskIterationAssignmentResult"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	field := func(model, name string, attrs map[string]any) {
		property := toolContractMap(schemas, model, "properties", name)
		for key, value := range attrs {
			property[key] = value
		}
	}
	field("TaskIterationAssignmentRequest", "project_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("TaskIterationAssignmentRequest", "iteration_id", map[string]any{"maxLength": 64, "description": "Empty string removes the iteration assignment."})
	field("TaskIterationAssignmentRequest", "expected_iteration_version", map[string]any{"minimum": 0, "maximum": 2147483647, "description": "Current target version. Use zero only for removal."})
	field("TaskIterationAssignmentRequest", "tasks", map[string]any{"minItems": 1, "maxItems": 16, "description": "Unique explicit task IDs in confirmation order."})
	field("TaskIterationAssignmentRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	field("TaskIterationAssignmentSelection", "task_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("TaskIterationAssignmentSelection", "expected_task_version", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("TaskIterationAssignmentSelection", "expected_plan_version", map[string]any{"minimum": 0, "maximum": 2147483647, "description": "Zero only when the task has no saved schedule."})
	field("TaskIterationAssignmentReceipt", "plan_version", map[string]any{"minimum": 0, "maximum": 2147483647})
	field("TaskIterationAssignmentResult", "changes", map[string]any{"minItems": 1, "maxItems": 16})
}
