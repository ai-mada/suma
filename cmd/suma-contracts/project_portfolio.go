package main

import "github.com/ai-mada/suma/internal/platform"

func projectPortfolioModels() []any {
	return []any{platform.ProjectPortfolioPage{}, platform.ProjectPortfolioItem{}, platform.ProjectPortfolioMilestone{}}
}

func projectPortfolioContracts(add operationAdder, api map[string]any) {
	op := add("/project-portfolio", "get", "ProjectPortfolioPage", false, false)
	browserOnly(op)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["description"] = "Read a bounded page of currently accessible projects in the session organization. Per-project task, risk and milestone values describe the complete authorized project; there is no organization-wide total."
	appendQuery(op, queryParameter("q", "Literal title or description search after trimming.", map[string]any{"type": "string", "maxLength": 120, "x-max-utf8-bytes": 120}))
	appendQuery(op, queryParameter("limit", "Projects per page.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
	appendQuery(op, queryParameter("after", "Opaque continuation for the same actor, organization, search and limit. The anchor must still be visible.", map[string]any{"type": "string", "minLength": 1, "maxLength": 1024}))

	for _, name := range []string{"ProjectPortfolioPage", "ProjectPortfolioItem", "ProjectPortfolioMilestone"} {
		model := toolContractMap(api, "components", "schemas", name)
		model["additionalProperties"] = false
		for _, value := range toolContractMap(model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
	page := toolContractMap(api, "components", "schemas", "ProjectPortfolioPage", "properties")
	page["items"].(map[string]any)["maxItems"] = 50
	page["as_of"].(map[string]any)["format"] = "date-time"
	milestone := toolContractMap(api, "components", "schemas", "ProjectPortfolioMilestone", "properties")
	milestone["target_date"].(map[string]any)["format"] = "date"
}
