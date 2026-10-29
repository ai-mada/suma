package main

import "github.com/ai-mada/suma/internal/platform"

func workspaceTaskContextModels() []any {
	return []any{platform.WorkspaceTaskContext{}, platform.WorkspaceTask{}, platform.WorkspaceTaskOwner{}, platform.WorkspaceTaskDesign{}, platform.WorkspaceTaskQuality{}, platform.WorkspaceTaskOption{}, platform.WorkspaceTaskPage{}}
}

// This is a read composition of existing authority and delivery domains. Its
// task selection is not a workspace binding or an execution/review grant.
func workspaceTaskContextContracts(add operationAdder, api map[string]any) {
	for _, endpoint := range []struct{ path, model string }{
		{"/workspace/task-context", "WorkspaceTaskContext"},
		{"/workspace/task-context/tasks", "WorkspaceTaskPage"},
	} {
		op := add(endpoint.path, "get", endpoint.model, false, false)
		successStatus(op, "200")
		op["x-read-only"] = true
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Read the exact current task, requirement, adopted design, bounded delivery and published-plan case summary under one current project authority transaction. Selecting a task does not bind the editor directory or grant write, dispatch, terminal, independent-testing or formal-review authority. No arbitrary record Data, run output, command, environment or credentials are disclosed."
		if endpoint.model == "WorkspaceTaskContext" {
			param := queryParameter("task_id", "Exact task in the authorized project, including an accepted task. Identifiers must fit the canonical 64-byte record bound.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
			param["required"] = true
			appendQuery(op, param)
		} else {
			appendQuery(op, queryParameter("search", "Literal title keyword. SQL wildcard characters are escaped; an empty search lists current project tasks.", map[string]any{"type": "string", "maxLength": 200}))
			appendQuery(op, queryParameter("limit", "Maximum returned options; current matching total is separate.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
			appendQuery(op, queryParameter("cursor", "Stable updated_at/id cursor bound to the tenant, project and search.", map[string]any{"type": "string", "maxLength": 4096}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"WorkspaceTaskContext", "WorkspaceTask", "WorkspaceTaskOwner", "WorkspaceTaskDesign", "WorkspaceTaskQuality", "WorkspaceTaskOption", "WorkspaceTaskPage"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for _, field := range toolContractMap(model, "properties") {
			field.(map[string]any)["readOnly"] = true
		}
	}
	toolContractMap(schemas, "WorkspaceTaskDesign", "properties", "state")["enum"] = []string{"none", "current", "stale", "unknown"}
	toolContractMap(schemas, "WorkspaceTaskPage", "properties", "items")["maxItems"] = 50
	toolContractMap(schemas, "WorkspaceTaskPage", "properties", "limit")["maximum"] = 50
	for _, field := range []string{"total", "passed", "failed", "errors", "skipped", "not_run", "revision"} {
		toolContractMap(schemas, "WorkspaceTaskQuality", "properties", field)["minimum"] = 0
	}
	toolContractMap(schemas, "WorkspaceTaskContext", "properties", "quality")["description"] = "Original published-plan case facts. Passed cases do not establish a separate Worker, exact candidate provenance or formal acceptance; inspect the original execution evidence."
}
