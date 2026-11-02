package main

import pc "github.com/ai-mada/suma/internal/poolrunnerpairingcontract"

func poolRunnerPairingModels() []any {
	return []any{pc.PoolRunnerPairing{}, pc.PoolRunnerPairingPage{}, pc.PoolRunnerPairingCreateRequest{}, pc.PoolRunnerPairingCommandRequest{}, pc.PoolRunnerPairingProposalRequest{}, pc.PoolRunnerPairingStatusRequest{}}
}

func poolRunnerPairingContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/execution-pools/{pool_id}/runner-pairings", "get", "", "PoolRunnerPairingPage", "200"},
		{"/execution-pools/{pool_id}/runner-pairings", "post", "PoolRunnerPairingCreateRequest", "PoolRunnerPairing", "201"},
		{"/execution-pools/{pool_id}/runner-pairings/{id}", "get", "", "PoolRunnerPairing", "200"},
		{"/execution-pools/{pool_id}/runner-pairings/{id}/commands", "post", "PoolRunnerPairingCommandRequest", "PoolRunnerPairing", "200"},
		{"/pool-runner-pairings/{id}/proposal", "post", "PoolRunnerPairingProposalRequest", "PoolRunnerPairing", "200"},
		{"/pool-runner-pairings/{id}/status", "post", "PoolRunnerPairingStatusRequest", "PoolRunnerPairing", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
		}
		// Pool enrollment is organization-scoped; a browser's current project
		// must never become an alternate machine or authorization scope.
		parameters := op["parameters"].([]any)
		filtered := make([]any, 0, len(parameters))
		for _, raw := range parameters {
			parameter := raw.(map[string]any)
			if parameter["in"] != "query" || parameter["name"] != "project_id" {
				filtered = append(filtered, parameter)
			}
		}
		op["parameters"] = filtered
		machine := route.input == "PoolRunnerPairingProposalRequest" || route.input == "PoolRunnerPairingStatusRequest"
		if machine {
			op["security"] = []any{map[string]any{"poolRunnerPairingProof": []string{}}}
			op["x-credential-purpose"] = "Exact original Pool pairing, machine-held token and original proposal operation only; no project, human or execution authority."
		} else {
			op["security"] = []any{map[string]any{"cookieAuth": []string{}}}
			op["x-disallowed-token-scopes"] = []string{"api", "runner", "agent", "pool_runner"}
		}
		if route.method == "get" || route.input == "PoolRunnerPairingStatusRequest" {
			op["x-read-only"] = true
		}
		if route.output == "PoolRunnerPairingPage" {
			op["description"] = "Current Pool administration reads stable created_at/id seek pages. Cursor binds owner, Pool, filters and original as_of creation boundary; authorization and logical states remain current. History does not prove online execution."
			appendQuery(op, queryParameter("status", "active means unexpired waiting/proposed; default active.", map[string]any{"type": "string", "enum": []string{"active", "waiting", "proposed", "approved", "rejected", "expired", "all"}, "default": "active"}))
			appendQuery(op, queryParameter("q", "Literal name, device or public identity filter; maximum120 UTF-8 bytes.", map[string]any{"type": "string", "x-max-utf8-bytes": 120}))
			appendQuery(op, queryParameter("limit", "Default20, maximum100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("cursor", "Original owner/Pool/filter/as_of-bound seek cursor.", map[string]any{"type": "string", "maxLength": 4096}))
		}
	}
	toolContractMap(api, "components", "securitySchemes")["poolRunnerPairingProof"] = map[string]any{"type": "http", "scheme": "bearer", "description": "Machine-held random token for its exact Pool pairing/proposal only. Approved status rechecks the original current Pool credential generation, expiry, revocation and Pool state. No secret is returned in JSON."}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"PoolRunnerPairing", "PoolRunnerPairingPage", "PoolRunnerPairingCreateRequest", "PoolRunnerPairingCommandRequest", "PoolRunnerPairingProposalRequest", "PoolRunnerPairingStatusRequest"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for field, raw := range model["properties"].(map[string]any) {
			property := raw.(map[string]any)
			switch field {
			case "id", "owner_tenant_id", "pool_id", "runner_id":
				property["pattern"] = `^[a-f0-9]{32}$`
			case "worker_id":
				property["pattern"] = `^pool-[a-f0-9]{32}$`
			case "purpose":
				property["const"] = "pool_runner"
			case "state":
				property["enum"] = []string{"waiting", "proposed", "approved", "rejected", "expired"}
			case "reason_code":
				property["enum"] = []string{"pool_changed", "pool_inactive", "credential_unavailable", "control_changed", "expired", "rejected"}
			case "name":
				property["minLength"], property["x-max-utf8-bytes"] = 1, 200
			case "device_name":
				property["minLength"], property["x-max-utf8-bytes"] = 1, 100
			case "max_slots":
				property["minimum"], property["maximum"] = 1, 128
			case "ttl_seconds":
				property["minimum"], property["maximum"] = 3600, 30*86400
			case "version", "pool_version", "credential_version":
				property["minimum"] = 1
			case "credential_sha256":
				property["pattern"] = `^[a-f0-9]{64}$`
			case "control_url":
				property["minLength"], property["maxLength"] = 1, 2048
			case "created_at", "expires_at", "credential_expires_at", "as_of":
				property["format"] = "date-time"
			case "idempotency_key":
				property["minLength"], property["x-max-utf8-bytes"] = 8, 128
			case "expected_version":
				property["minimum"] = 1
			case "action":
				property["enum"] = []string{"approve", "reject"}
			case "total":
				property["minimum"] = 0
			case "items":
				property["maxItems"] = 100
			}
		}
	}
}
