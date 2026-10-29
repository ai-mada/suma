package main

import "github.com/ai-mada/suma/internal/platform"

func projectFocusModels() []any { return []any{platform.ProjectFocus{}} }

func projectFocusContracts(add operationAdder, api map[string]any) {
	op := add("/project-focus", "get", "ProjectFocus", false, false)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["description"] = "Current project-authorized transactional read of disjoint actionable task sections. Exact totals and at most six original task/run facts per section. A completed run awaiting review does not prove test coverage, eligibility or formal acceptance. This endpoint never probes tools, dispatches or mutates tasks."
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ProjectFocus", "ProjectFocusSection", "ProjectFocusItem"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for _, property := range toolContractMap(model, "properties") {
			property.(map[string]any)["readOnly"] = true
		}
	}
	toolContractMap(schemas, "ProjectFocus", "properties", "sections")["minItems"] = 4
	toolContractMap(schemas, "ProjectFocus", "properties", "sections")["maxItems"] = 4
	toolContractMap(schemas, "ProjectFocusSection", "properties", "key")["enum"] = []string{"planning", "execution", "review", "attention"}
	toolContractMap(schemas, "ProjectFocusSection", "properties", "total")["minimum"] = 0
	toolContractMap(schemas, "ProjectFocusSection", "properties", "items")["maxItems"] = 6
	toolContractMap(schemas, "ProjectFocusItem", "properties", "task_version")["minimum"] = 1
	toolContractMap(schemas, "ProjectFocusItem", "properties", "reason")["enum"] = []string{"task_unstarted", "run_queued", "run_active", "run_waiting", "run_paused", "run_succeeded_review_pending", "task_blocked", "run_failed", "run_reconciling", "run_cancelled", "run_missing", "run_basis_changed", "run_basis_unavailable"}
}
