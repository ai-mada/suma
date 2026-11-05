package main

func projectPortfolioCapacityContract(add operationAdder, api map[string]any) {
	op := add("/project-portfolio/capacity", "get", "ProjectPortfolioCapacityPage", false, false)
	browserOnly(op)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["description"] = "Compare estimated weekly allocations from currently readable projects against organization work calendars. Unknown availability and estimates are retained. This projection does not expose full calendars or actual work."
	week := queryParameter("week", "Required Monday in YYYY-MM-DD.", map[string]any{"type": "string", "format": "date"})
	week["required"] = true
	appendQuery(op, week)
	op["responses"].(map[string]any)["409"] = map[string]any{"description": "project_capacity_scope_too_large: more than 2000 qualifying assignments; no partial capacity data."}

	for _, name := range []string{"ProjectPortfolioCapacityPage", "ProjectPortfolioCapacityPerson", "ProjectPortfolioCapacityDay", "ProjectPortfolioCapacityTask"} {
		model := toolContractMap(api, "components", "schemas", name)
		model["additionalProperties"] = false
		for _, value := range toolContractMap(model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
	page := toolContractMap(api, "components", "schemas", "ProjectPortfolioCapacityPage", "properties")
	page["week_start"].(map[string]any)["format"] = "date"
	page["as_of"].(map[string]any)["format"] = "date-time"
	page["items"].(map[string]any)["maxItems"] = 2000
	person := toolContractMap(api, "components", "schemas", "ProjectPortfolioCapacityPerson", "properties")
	person["state"].(map[string]any)["enum"] = []string{"over_capacity", "basis_incomplete", "within_visible_capacity"}
	person["state"].(map[string]any)["description"] = "Over capacity can be proven by the known allocation alone. Within visible capacity applies only to the returned assignments."
	days := person["days"].(map[string]any)
	days["minItems"], days["maxItems"] = 7, 7
	tasks := person["tasks"].(map[string]any)
	tasks["minItems"], tasks["maxItems"] = 1, 2000
	day := toolContractMap(api, "components", "schemas", "ProjectPortfolioCapacityDay", "properties")
	day["date"].(map[string]any)["format"] = "date"
	task := toolContractMap(api, "components", "schemas", "ProjectPortfolioCapacityTask", "properties")
	task["start_date"].(map[string]any)["format"] = "date"
	task["due_date"].(map[string]any)["format"] = "date"
}
