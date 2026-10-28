package main

import "github.com/ai-mada/suma/internal/platform"

func taskDependencyModels() []any {
	return []any{platform.TaskDependencyGraph{}, platform.TaskDependencyNode{}, platform.TaskDependencyChangeRequest{}, platform.TaskDependencyChange{}, platform.TaskDependencyChangeResult{}, platform.TaskDependencyChangeReceipt{}}
}

// taskDependencyContracts describes the business task graph, not a workflow's
// execution DAG. Exact graph validity and all task versions are checked together
// in the project's write transaction; JSON Schema cannot replace that check.
func taskDependencyContracts(add operationAdder, api map[string]any) {
	read := add("/task-dependencies", "get", "TaskDependencyGraph", false, false)
	successStatus(read, "200")
	read["description"] = "Read the current authorized project's complete business task dependency graph, bounded at 1000 tasks. When total exceeds limit, complete is false and tasks is empty; a partial graph must never be treated as the full graph. This read does not dispatch work or change acceptance."
	read["x-request-schema-coverage"] = "required project_id; no pagination or partial dependency graph; current project authorization is server-enforced"

	write := add("/task-dependencies", "post", "TaskDependencyChangeResult", false, true)
	successStatus(write, "200")
	request(write, "TaskDependencyChangeRequest")
	// The body is the sole project scope. Shared decorators append only headers.
	write["parameters"] = []any{}
	write["description"] = "Atomically replace predecessor lists for 1..100 changed tasks. Every changed task carries its original version. Under current authority and the project lock, validate the full current project graph, task versions and existing execution/acceptance rules, then persist task changes, downstream invalidations, audit and the operation receipt together. Duplicate task IDs, duplicate predecessor IDs, self references, missing/cross-project references and cycles are rejected. Exact replay returns the immutable original acknowledgement; read the current graph again before rendering saved state. No task, attempt, acceptance or Git merge is started."
	write["x-request-schema-coverage"] = "exact typed body and nested changes; all-or-nothing per-task CAS, full graph checks, authority, original idempotent receipts and execution/acceptance guards remain server-enforced"

	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for key, item := range attrs {
			value[key] = item
		}
	}
	for _, model := range []string{"TaskDependencyGraph", "TaskDependencyNode", "TaskDependencyChangeRequest", "TaskDependencyChange", "TaskDependencyChangeResult", "TaskDependencyChangeReceipt"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	field("TaskDependencyGraph", "total", map[string]any{"minimum": 0})
	field("TaskDependencyGraph", "limit", map[string]any{"const": 1000})
	field("TaskDependencyGraph", "tasks", map[string]any{"maxItems": 1000})
	// Empty-on-overflow is a safety property: clients cannot validate a truncated
	// graph or publish edits that omit predecessors outside the returned page.
	toolContractMap(schemas, "TaskDependencyGraph")["allOf"] = []any{map[string]any{
		"if":   map[string]any{"properties": map[string]any{"complete": map[string]any{"const": false}}},
		"then": map[string]any{"properties": map[string]any{"tasks": map[string]any{"maxItems": 0}}},
	}}
	field("TaskDependencyChangeRequest", "project_id", map[string]any{"minLength": 1})
	field("TaskDependencyChangeRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	field("TaskDependencyChangeRequest", "changes", map[string]any{"minItems": 1, "maxItems": 100, "description": "Dirty tasks only, each task_id exactly once. Every predecessor set must change; reordering is not a change. Preserve this original array for retries."})
	field("TaskDependencyChange", "task_id", map[string]any{"minLength": 1})
	field("TaskDependencyChange", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483647, "description": "Version from this task's original graph snapshot. A task at the version limit cannot be modified."})
	for _, model := range []string{"TaskDependencyNode", "TaskDependencyChange", "TaskDependencyChangeReceipt"} {
		field(model, "depends_on", map[string]any{"uniqueItems": true, "items": map[string]any{"type": "string", "minLength": 1}, "description": "Predecessor task IDs in this project. An explicit empty array removes all predecessors; duplicates are rejected rather than silently normalized."})
	}
	field("TaskDependencyChangeResult", "changes", map[string]any{"minItems": 1, "maxItems": 100})
	for _, model := range []string{"TaskDependencyGraph", "TaskDependencyNode", "TaskDependencyChangeResult", "TaskDependencyChangeReceipt"} {
		for _, value := range toolContractMap(schemas, model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
}
