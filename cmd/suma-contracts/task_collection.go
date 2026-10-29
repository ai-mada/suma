package main

import "github.com/ai-mada/suma/internal/platform"

func taskCollectionModels() []any {
	return []any{platform.TaskCollectionSummary{}, platform.TaskCollectionGroup{}}
}

// Group headers have their own bounded continuation; rows always use the one
// canonical /task-workspace reader and its shared literal filter compiler.
func taskCollectionContracts(add operationAdder, api map[string]any) {
	op := add("/task-collection/summary", "get", "TaskCollectionSummary", false, false)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
	op["description"] = "Current-authorized aggregate of the same task scope as /task-workspace. Total covers all matched tasks; groups contains only a bounded current group page. Separate requests are live reads, not a frozen snapshot. Does not mutate, dispatch, test or approve."
	appendTaskWorkspaceFilters(op)
	appendQuery(op, queryParameter("group_by", "Current task grouping field. Empty keys are genuine unset data.", map[string]any{"type": "string", "enum": []string{"status", "owner", "priority"}, "default": "status"}))
	appendQuery(op, queryParameter("group_limit", "Maximum group headers; loading more is explicit and independent of task-row pagination.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
	appendQuery(op, queryParameter("group_cursor", "Opaque continuation bound to current identity, project, base filters, grouping and group limit.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}))
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"TaskCollectionSummary", "TaskCollectionGroup"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for _, field := range toolContractMap(model, "properties") {
			field.(map[string]any)["readOnly"] = true
		}
	}
	toolContractMap(schemas, "TaskCollectionSummary", "properties", "groups")["maxItems"] = 100
	for _, name := range []string{"total", "groups_total"} {
		toolContractMap(schemas, "TaskCollectionSummary", "properties", name)["minimum"] = 0
	}
	toolContractMap(schemas, "TaskCollectionGroup", "properties", "total")["minimum"] = 0
}
