package main

import "github.com/ai-mada/suma/internal/platform"

func projectOverviewModels() []any {
	return []any{platform.OverviewDelivery{}, platform.OverviewAttentionItem{}, platform.OverviewAttentionPage{}, platform.OverviewMilestone{}, platform.OverviewMilestones{}}
}

func projectOverviewContracts(add operationAdder, api map[string]any) {
	op := add("/overview/attention", "get", "OverviewAttentionPage", false, false)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["description"] = "Current-authorized project attention queue; exact full matching count with bounded updated_at/id pages. Cursors bind tenant, project, queue and UTC date. No approval or test eligibility is inferred."
	kind := queryParameter("kind", "Project queue, not a personal approval assignment.", map[string]any{"type": "string", "enum": []string{"review", "blocked", "overdue", "reconciling"}})
	kind["required"] = true
	appendQuery(op, kind)
	appendQuery(op, queryParameter("limit", "Bounded page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 6}))
	appendQuery(op, queryParameter("cursor", "Opaque scope-bound current queue cursor.", map[string]any{"type": "string", "maxLength": 1024}))
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"OverviewDelivery", "OverviewAttentionItem", "OverviewAttentionPage", "OverviewMilestone", "OverviewMilestones"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for _, field := range toolContractMap(model, "properties") {
			field.(map[string]any)["readOnly"] = true
		}
	}
	overview := toolContractMap(api, "paths", "/overview", "get")
	overview["description"] = "Single current-authorized project snapshot. delivery uses whole-project task counts and only exact linked runs; scope_tasks excludes cancelled tasks. attention contains bounded review/blocked/overdue/reconciling pages. milestones lists at most six dated unfinished milestones ordered by target date. Neither execution nor in_review proves independent tests or acceptance. as_of and overdue date basis are UTC."
}
