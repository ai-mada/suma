package main

import "github.com/ai-mada/suma/internal/platform"

func sharedTaskViewModels() []any {
	return []any{platform.SharedTaskViewPreferences{}, platform.SharedTaskView{}, platform.SharedTaskViewPage{}, platform.SharedTaskViewCreateRequest{}, platform.SharedTaskViewUpdateRequest{}, platform.SharedTaskViewArchiveRequest{}}
}

// Saved conditions are presentation data, never a grant or a task snapshot.
func sharedTaskViewContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/task-views", "get", "", "SharedTaskViewPage", "200"},
		{"/task-views", "post", "SharedTaskViewCreateRequest", "SharedTaskView", "201"},
		{"/task-views/{id}", "get", "", "SharedTaskView", "200"},
		{"/task-views/{id}", "patch", "SharedTaskViewUpdateRequest", "SharedTaskView", "200"},
		{"/task-views/{id}/archive", "post", "SharedTaskViewArchiveRequest", "SharedTaskView", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current-authorized personal or project-shared task conditions. Private views are visible only to their owner; project views do not grant project or task access. Mutations use a stable operation ID; updates and archive require the observed version. Current object visibility and management authority are checked before receipt replay. Archive retains a versioned tombstone. This does not execute tasks or provide offline synchronization."
		if route.input != "" {
			request(op, route.input)
		} else {
			op["x-read-only"] = true
		}
		if route.path == "/task-views" && route.method == "get" {
			appendQuery(op, queryParameter("limit", "Bounded active-view page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("cursor", "Opaque ID continuation bound to current identity, project and page size.", map[string]any{"type": "string", "maxLength": 4096}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for key, item := range attrs {
			value[key] = item
		}
	}
	for _, model := range []string{"SharedTaskViewPreferences", "SharedTaskView", "SharedTaskViewPage", "SharedTaskViewCreateRequest", "SharedTaskViewUpdateRequest", "SharedTaskViewArchiveRequest"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"SharedTaskViewCreateRequest", "SharedTaskViewUpdateRequest", "SharedTaskViewArchiveRequest"} {
		field(model, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128, "x-max-utf8-bytes": 128})
	}
	for _, model := range []string{"SharedTaskViewCreateRequest", "SharedTaskViewUpdateRequest", "SharedTaskView"} {
		field(model, "visibility", map[string]any{"enum": []string{"private", "project"}})
		field(model, "name", map[string]any{"minLength": 1, "maxLength": 80})
	}
	field("SharedTaskViewUpdateRequest", "visibility", map[string]any{"description": "Must equal the current visibility; use an explicit new create to save in another scope."})
	toolContractMap(schemas, "SharedTaskViewCreateRequest")["required"] = []string{"project_id", "name", "visibility", "idempotency_key"}
	toolContractMap(schemas, "SharedTaskViewUpdateRequest")["required"] = []string{"project_id", "name", "visibility", "expected_version", "idempotency_key"}
	toolContractMap(schemas, "SharedTaskViewPreferences")["required"] = []string{}
	for _, model := range []string{"SharedTaskViewUpdateRequest", "SharedTaskViewArchiveRequest"} {
		field(model, "expected_version", map[string]any{"minimum": 1})
	}
	field("SharedTaskView", "state", map[string]any{"enum": []string{"active", "archived"}})
	field("SharedTaskViewPage", "items", map[string]any{"maxItems": 100})
	field("SharedTaskViewPage", "total", map[string]any{"minimum": 0})
	for _, name := range []string{"q", "status", "priority"} {
		field("SharedTaskViewPreferences", name, map[string]any{"maxLength": 200})
	}
	for _, name := range []string{"owner_id", "requirement_id", "iteration_id"} {
		field("SharedTaskViewPreferences", name, map[string]any{"maxLength": 64, "x-max-utf8-bytes": 64})
	}
	field("SharedTaskViewPreferences", "status", map[string]any{"description": "Exact literal status. Empty means unrestricted; mutually exclusive with status_unset=true."})
	field("SharedTaskViewPreferences", "priority", map[string]any{"description": "Exact literal priority. Empty means unrestricted; mutually exclusive with priority_unset=true."})
	field("SharedTaskViewPreferences", "owner_id", map[string]any{"description": "Literal resource ID; __unassigned__ has no special meaning. Mutually exclusive with unassigned=true."})
	field("SharedTaskViewPreferences", "iteration_id", map[string]any{"description": "Exact formal planning iteration ID; empty means unrestricted. Mutually exclusive with iteration_unset=true. Unknown or other-project IDs match nothing; completed iterations remain selectable."})
	field("SharedTaskViewPreferences", "iteration_unset", map[string]any{"description": "Select no planning row or an empty formal iteration. True is mutually exclusive with nonempty iteration_id."})
	field("SharedTaskViewPreferences", "sort", map[string]any{"enum": []string{"", "updated_desc", "created_desc", "title_asc", "priority_desc"}, "default": "updated_desc"})
	field("SharedTaskViewPreferences", "layout", map[string]any{"enum": []string{"", "list", "board"}, "default": "list"})
	field("SharedTaskViewPreferences", "group_by", map[string]any{"enum": []string{"", "status", "owner", "priority", "iteration"}, "default": "status"})
	for _, model := range []string{"SharedTaskView", "SharedTaskViewPage"} {
		for _, value := range toolContractMap(schemas, model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
}
