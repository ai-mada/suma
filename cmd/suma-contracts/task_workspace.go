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
}
