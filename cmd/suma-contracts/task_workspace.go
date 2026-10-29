package main

func taskWorkspaceContracts(add operationAdder, api map[string]any) {
	op := add("/task-workspace", "get", "TaskWorkspacePage", false, false)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
	op["description"] = "Current-authorized project-wide task search and filtering before bounded pagination. Total uses the same filters in the same read transaction. Grouping is presentation only. Each page is a current read, not an immutable multi-page snapshot; refresh to reconcile concurrent edits."
	for _, parameter := range []struct {
		name, description string
		schema            map[string]any
	}{
		{"q", "Literal title, description or ID substring; trimmed, at most 200 Unicode characters, no control characters. SQL wildcard characters are not operators.", map[string]any{"type": "string", "maxLength": 200}},
		{"status", "Exact trimmed business task status, at most 64 UTF-8 bytes, no controls; omit for all. Empty is invalid. This is not execution state.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64}},
		{"owner_id", "Exact resource ID (1-64 ASCII identifier characters), or __unassigned__ for no owner. Omit for all; unknown IDs yield no matches.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64}},
		{"priority", "Exact trimmed priority, at most 64 UTF-8 bytes, no controls; omit for all. Empty is invalid.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64}},
		{"sort", "Stable server order with ID tie-breaker. Unrecognized priorities sort after known values.", map[string]any{"type": "string", "enum": []string{"updated_desc", "created_desc", "title_asc", "priority_desc"}, "default": "updated_desc"}},
		{"limit", "Bounded page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 100}},
		{"cursor", "Opaque continuation bound to current identity, project and every filter/order. Changing scope requires the first page.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}},
	} {
		appendQuery(op, queryParameter(parameter.name, parameter.description, parameter.schema))
	}
	for _, field := range toolContractMap(api, "components", "schemas", "TaskWorkspacePage", "properties") {
		field.(map[string]any)["readOnly"] = true
	}
}
