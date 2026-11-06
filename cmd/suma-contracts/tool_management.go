package main

// These filters and impact facts extend the existing authorized tool read.
// They never add a second availability evaluator or a process-stop authority.
func toolManagementContracts(api map[string]any) {
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ToolProfileListQuery", "ToolProfileImpact", "ToolProfileImpactItem"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	state := toolContractMap(schemas, "ToolProfileListQuery", "properties", "state")
	state["enum"] = []string{"active", "enabled", "paused", "revoked", "all"}
	state["default"] = "active"
	query := toolContractMap(schemas, "ToolProfileListQuery", "properties", "query")
	query["maxLength"] = 120
	query["x-max-utf8-bytes"] = 120
	query["description"] = "Optional bounded public-field query. Server normalizes Unicode lower-case and surrounding whitespace and binds that filter to the original authorized cursor; never searches credentials or private paths."
	worker := toolContractMap(schemas, "ToolProfileListQuery", "properties", "worker_id")
	worker["minLength"], worker["maxLength"], worker["pattern"] = 1, 100, `^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`
	worker["description"] = "Optional exact, case-sensitive execution node identifier, combined with state and public text search."
	limit := toolContractMap(schemas, "ToolProfileListQuery", "properties", "limit")
	limit["minimum"], limit["maximum"], limit["default"] = 1, 200, 50
	toolContractMap(schemas, "ToolProfileListQuery", "properties", "cursor")["maxLength"] = 1024
	get := toolContractMap(api, "paths", "/tool-profiles", "get")
	// Refine the existing operation parameter as well as its reusable query DTO;
	// clients generate request validation from the operation, not the DTO alone.
	for _, parameter := range get["parameters"].([]any) {
		value := parameter.(map[string]any)
		if value["name"] == "cursor" && value["in"] == "query" {
			value["schema"] = toolContractMap(schemas, "ToolProfileListQuery", "properties", "cursor")
			value["description"] = "Opaque cursor, at most 1024 characters, bound to the current actor, tenant, project, normalized state/query and exact worker_id."
		}
	}
	appendQuery(get, queryParameter("state", "Current enabled/paused connections by default. Revoked history is explicit; changing a filter invalidates the old cursor.", state))
	appendQuery(get, queryParameter("query", "At most120 UTF-8 bytes; literal public name/command/worker/adapter/protocol search, authorized before reading.", query))
	appendQuery(get, queryParameter("worker_id", "Restrict results to the exact, case-sensitive execution node identifier.", worker))
	get["description"] = "Current actor/organization/project tool connections with keyset pagination bound to normalized state/query and exact worker_id. Default active includes enabled and paused only. Reading history or diagnostics never enables, repairs or stops tools."
	impact := toolContractMap(schemas, "ToolProfileImpact")
	impact["description"] = "One authorized read snapshot of this project's active runs only, bounded to200 runs/2000 nodes and five observed references. Complete does not cover unstarted plans or historical/unknown external processes. Observed counts are not inferred totals or stop acknowledgements."
	toolContractMap(impact, "properties", "scope")["const"] = "active_project_runs"
	toolContractMap(impact, "properties", "observed_at")["format"] = "date-time"
	toolContractMap(impact, "properties", "coverage")["enum"] = []string{"complete", "partial", "unknown"}
	toolContractMap(impact, "properties", "reason")["enum"] = []string{"", "run_limit", "node_limit", "source_unavailable"}
	for key, max := range map[string]int{"runs_examined": 200, "nodes_examined": 2000, "observed_reference_count": 2000} {
		v := toolContractMap(impact, "properties", key)
		v["minimum"], v["maximum"] = 0, max
	}
	toolContractMap(impact, "properties", "items")["maxItems"] = 5
	relations := []any{}
	for _, pair := range []struct {
		coverage string
		reasons  []string
	}{{"complete", []string{""}}, {"partial", []string{"run_limit", "node_limit"}}, {"unknown", []string{"source_unavailable"}}} {
		relations = append(relations, map[string]any{"if": map[string]any{"properties": map[string]any{"coverage": map[string]any{"const": pair.coverage}}, "required": []string{"coverage"}}, "then": map[string]any{"properties": map[string]any{"reason": map[string]any{"enum": pair.reasons}}}})
	}
	impact["allOf"] = relations
	item := toolContractMap(schemas, "ToolProfileImpactItem", "properties")
	toolContractMap(item, "run_state")["enum"] = []string{"running", "paused", "cancelling"}
	toolContractMap(item, "node_state")["enum"] = []string{"pending", "ready", "running", "waiting", "reconciling"}
	for _, key := range []string{"attempt", "fence"} {
		v := toolContractMap(item, key)
		v["minimum"], v["maximum"] = 0, 9007199254740991
	}
	toolContractMap(item, "worker_id")["description"] = "Empty until a real assignment exists; pending/waiting nodes do not imply an executor."
	toolContractMap(schemas, "ToolProfileDiagnostics", "properties", "impact")["description"] = "Observed references, separate from the four original availability prerequisites. Partial/unknown coverage cannot become zero total or prove external processes stopped."
}
