package main

import pc "github.com/ai-mada/suma/internal/poolconnectioncontract"

func poolConnectionModels() []any {
	return []any{pc.PoolRunnerReadiness{}, pc.PoolRunnerReadinessRunner{}, pc.PoolRunnerReadinessScope{}, pc.PoolConnectionCheck{}, pc.PoolConnectionScope{}}
}

func poolConnectionContracts(add operationAdder, api map[string]any) {
	op := add("/execution-pools/{pool_id}/runners/{runner_id}/readiness", "get", "PoolRunnerReadiness", false, false)
	op["security"] = []any{map[string]any{"cookieAuth": []string{}}}
	op["x-disallowed-token-scopes"] = []string{"api", "runner", "agent", "pool_runner"}
	op["x-read-only"] = true
	op["description"] = "Current Pool administration reads one exact public Grant. Private route/inventory/maintenance facts require a separate current project-read proof. SourceCurrent is an as-of matching report, not online, prepared-file identity or task/tool qualification."
	parameters := []any{}
	for _, raw := range op["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["in"] != "query" || p["name"] != "project_id" {
			parameters = append(parameters, p)
		}
	}
	for _, name := range []string{"tenant_id", "project_id"} {
		p := queryParameter(name, "Exact configured Grant scope; does not widen project access.", map[string]any{"type": "string", "pattern": `^[a-f0-9]{32}$`})
		p["required"] = true
		parameters = append(parameters, p)
	}
	op["parameters"] = parameters
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"PoolRunnerReadiness", "PoolRunnerReadinessRunner", "PoolRunnerReadinessScope", "PoolConnectionCheck", "PoolConnectionScope"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		properties := toolContractMap(model, "properties")
		for _, key := range []string{"pool_id", "runner_id", "grant_id"} {
			if properties[key] != nil {
				toolContractMap(properties, key)["pattern"] = `^[a-f0-9]{32}$`
			}
		}
		if properties["worker_id"] != nil {
			toolContractMap(properties, "worker_id")["pattern"] = `^pool-[a-f0-9]{32}$`
		}
	}
	runner := toolContractMap(schemas, "PoolRunnerReadinessRunner", "properties")
	toolContractMap(runner, "credential_state")["enum"] = []string{"valid", "expired", "revoked", "version_mismatch"}
	toolContractMap(runner, "historical_last_seen_at")["description"] = "Historical new inventory contact; retained through rotations, not current connection proof. Empty means unobserved."
	scope := toolContractMap(schemas, "PoolRunnerReadinessScope", "properties")
	toolContractMap(scope, "route_state")["enum"] = []string{"restricted", "unconfigured", "different", "current"}
	toolContractMap(scope, "source_reason")["enum"] = []string{"project_access_required", "route_not_current", "inventory_unreported", "inventory_stale", "maintenance_unknown", "maintenance_stale", "observation_mismatch", "current"}
	toolContractMap(scope, "source_current")["description"] = "At checked_at only: current authenticated maintenance provenance, supported/fresh capability and the same fresh inventory observation. A current empty inventory is possible; this never qualifies a tool or task."
	toolContractMap(scope, "inventory_item_count")["description"] = "Present only with current matching inventory; zero is an actual empty report, absence is unknown/restricted."
	check := toolContractMap(schemas, "PoolConnectionCheck", "properties")
	toolContractMap(check, "protocol")["const"] = pc.PoolConnectionCheckProtocol
	toolContractMap(check, "configuration_file_sha256")["pattern"] = `^[a-f0-9]{64}$`
	toolContractMap(check, "configuration_file_sha256")["description"] = "Original bounded file bytes, not resolved runtime Journal configuration."
	toolContractMap(check, "scopes")["minItems"] = 1
	toolContractMap(check, "scopes")["maxItems"] = 256
	toolContractMap(schemas, "PoolConnectionCheck")["x-local-cli-only"] = true
}
