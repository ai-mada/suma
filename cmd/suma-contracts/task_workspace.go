package main

// Shared literal descriptors prevent row pages and group aggregates drifting.
func appendTaskWorkspaceFilters(op map[string]any) {
	for _, p := range []struct {
		name, description string
		schema            map[string]any
	}{
		{"q", "Literal title, description or ID substring; spaces preserved, up to 200 Unicode characters, no controls. SQL wildcards are escaped.", map[string]any{"type": "string", "maxLength": 200}},
		{"status", "Exact business status, including literal all or archived. Empty or omitted is unrestricted; mutually exclusive with status_unset=true.", map[string]any{"type": "string", "maxLength": 200}},
		{"priority", "Exact priority; empty or omitted is unrestricted, mutually exclusive with priority_unset=true.", map[string]any{"type": "string", "maxLength": 200}},
		{"owner_id", "Literal resource identifier, including __unassigned__ if that is a real ID. No sentinel; empty or omitted is unrestricted.", map[string]any{"type": "string", "maxLength": 64, "x-max-utf8-bytes": 64}},
		{"requirement_id", "Literal in-project requirement identifier. Unknown IDs match nothing and never expand authority.", map[string]any{"type": "string", "maxLength": 64, "x-max-utf8-bytes": 64}},
		{"iteration_id", "Exact formal planning iteration identifier, never task payload JSON. Empty or omitted is unrestricted; mutually exclusive with iteration_unset=true. Unknown or other-project IDs match nothing. Completed iterations remain selectable.", map[string]any{"type": "string", "maxLength": 64, "x-max-utf8-bytes": 64}},
		{"iteration_unset", "Match tasks with no planning row or an empty formal iteration. True is mutually exclusive with nonempty iteration_id.", map[string]any{"type": "boolean"}},
		{"unassigned", "Match genuinely unassigned owners; true is mutually exclusive with nonempty owner_id.", map[string]any{"type": "boolean"}},
		{"priority_unset", "Match genuinely empty priority; true is mutually exclusive with nonempty priority.", map[string]any{"type": "boolean"}},
		{"status_unset", "Match genuinely empty status; true is mutually exclusive with nonempty status.", map[string]any{"type": "boolean"}},
	} {
		appendQuery(op, queryParameter(p.name, p.description, p.schema))
	}
}

func taskWorkspaceContracts(add operationAdder, api map[string]any) {
	op := add("/task-workspace", "get", "TaskWorkspacePage", false, false)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
	op["description"] = "Current-authorized project-wide task filtering before bounded keyset pagination, including archived tasks. Total uses the same read transaction. Every page is a current read, not an immutable snapshot. UTC time ordering preserves nanoseconds with an ID tie-breaker. Refresh to reconcile edits."
	appendTaskWorkspaceFilters(op)
	appendQuery(op, queryParameter("sort", "Stable server ordering; unrecognized priorities follow known values.", map[string]any{"type": "string", "enum": []string{"updated_desc", "created_desc", "title_asc", "priority_desc"}, "default": "updated_desc"}))
	appendQuery(op, queryParameter("limit", "Bounded task-row page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 100}))
	appendQuery(op, queryParameter("cursor", "Opaque continuation bound to current identity, project, every filter, sort and page size.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}))
	for _, field := range toolContractMap(api, "components", "schemas", "TaskWorkspacePage", "properties") {
		field.(map[string]any)["readOnly"] = true
	}
	toolContractMap(api, "components", "schemas", "TaskWorkspacePage", "properties", "items")["maxItems"] = 100
	plans := toolContractMap(api, "components", "schemas", "TaskWorkspacePage", "properties", "task_plans")
	plans["maxItems"] = 100
	plans["description"] = "Exactly one formal planning reference per task in this page's items, observed in the same current-authorized read transaction. This is not a task payload field or a frozen cross-page snapshot."
	plan := toolContractMap(api, "components", "schemas", "TaskWorkspacePlanRef")
	plan["additionalProperties"] = false
	for _, field := range toolContractMap(plan, "properties") {
		field.(map[string]any)["readOnly"] = true
	}
	for _, name := range []string{"task_id", "iteration_id"} {
		field := toolContractMap(plan, "properties", name)
		field["maxLength"] = 64
		field["x-max-utf8-bytes"] = 64
	}
	toolContractMap(plan, "properties", "task_id")["minLength"] = 1
	toolContractMap(plan, "properties", "iteration_id")["description"] = "Formal suma_task_plans iteration reference. Empty covers either no planning row or an existing row without an iteration; plan_version distinguishes them."
	version := toolContractMap(plan, "properties", "plan_version")
	version["minimum"] = 0
	version["description"] = "Zero only when no planning row exists; otherwise the observed planning CAS version, including an empty iteration association. Distinct from task.version."
}
