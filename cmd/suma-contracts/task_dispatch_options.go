package main

import "github.com/ai-mada/suma/internal/platform"

func taskDispatchOptionsModels() []any {
	return []any{platform.TaskDispatchOptionsRequest{}, platform.TaskDispatchOptions{}, platform.TaskDispatchToolOption{}}
}

func taskDispatchOptionsContracts(add operationAdder, api map[string]any) {
	op := add("/tasks/{id}/dispatch-options", "get", "TaskDispatchOptions", false, false)
	op["description"] = "Read current tool observations and fixed workflow compatibility only. The first eligible tool on this stable ID-ordered page is suggested, without quality/cost/slot claims. Adoption never creates or starts work; explicit preview and Start recheck current authority, context, route and budget. Model stays fixed by tool; cost remains unknown."
	for _, field := range []string{"expected_task_version", "workflow_version"} {
		p := queryParameter(field, "Exact current version; canonical positive decimal.", map[string]any{"type": "integer", "minimum": 1, "maximum": 2147483646})
		p["required"] = true
		appendQuery(op, p)
	}
	for _, field := range []struct {
		name string
		max  int
	}{{"workflow_id", 100}, {"agent_node_id", 150}} {
		p := queryParameter(field.name, "Fixed workflow/node identity.", map[string]any{"type": "string", "minLength": 1, "maxLength": field.max})
		p["required"] = true
		appendQuery(op, p)
	}
	appendQuery(op, queryParameter("limit", "Current page size; recommendation is page-local.", map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 50}))
	appendQuery(op, queryParameter("before", "Original actor/project/task/workflow/node-bound cursor.", map[string]any{"type": "string", "maxLength": 4096}))
	for field, value := range map[string]any{"reason_code": []string{"eligible", "tool_unavailable", "workflow_incompatible", "route_incompatible"}, "model_state": []string{"fixed_by_tool"}, "cost_state": []string{"unknown"}, "eligible_basis": []string{"current_observation_and_compatibility"}} {
		toolContractMap(api, "components", "schemas", "TaskDispatchToolOption", "properties", field)["enum"] = value
	}
}
