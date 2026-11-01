package main

import pc "github.com/ai-mada/suma/internal/runnerpairingcontract"

func runnerPairingModels() []any {
	return []any{pc.RunnerPairing{}, pc.RunnerPairingCreateRequest{}, pc.RunnerPairingCommandRequest{}, pc.RunnerPairingProposalRequest{}, pc.RunnerPairingStatusRequest{}}
}

func runnerPairingContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, status string }{
		{"/runner-pairings", "post", "RunnerPairingCreateRequest", "201"},
		{"/runner-pairings/{id}", "get", "", "200"},
		{"/runner-pairings/{id}/commands", "post", "RunnerPairingCommandRequest", "200"},
		{"/runner-pairings/{id}/proposal", "post", "RunnerPairingProposalRequest", "200"},
		{"/runner-pairings/{id}/status", "post", "RunnerPairingStatusRequest", "200"},
	} {
		op := add(route.path, route.method, "RunnerPairing", false, route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
			// The project is part of the original JSON operation, not a second
			// optional query scope that a machine client could accidentally mix.
			parameters := op["parameters"].([]any)
			filtered := make([]any, 0, len(parameters))
			for _, value := range parameters {
				p := value.(map[string]any)
				if p["in"] != "query" || p["name"] != "project_id" {
					filtered = append(filtered, value)
				}
			}
			op["parameters"] = filtered
		}
		if route.input == "RunnerPairingProposalRequest" || route.input == "RunnerPairingStatusRequest" {
			op["security"] = []any{map[string]any{"runnerPairingProof": []string{}}}
			op["x-credential-purpose"] = "Exact original pairing token proof; not general browser, API or task-execution authority."
		} else {
			op["security"] = []any{map[string]any{"cookieAuth": []string{}}}
			op["x-disallowed-token-scopes"] = []string{"api", "runner", "agent"}
		}
		if route.method == "get" || route.input == "RunnerPairingStatusRequest" {
			op["x-read-only"] = true
		}
	}
	toolContractMap(api, "components", "securitySchemes")["runnerPairingProof"] = map[string]any{"type": "http", "scheme": "bearer", "description": "Machine-held random token, checked only against the exact original pairing/project/proposal operation digest. No secret appears in JSON or a browser response. Approved status also rechecks the current original Runner grant."}
	schemas := toolContractMap(api, "components", "schemas")
	for _, value := range []string{"RunnerPairing", "RunnerPairingCreateRequest", "RunnerPairingCommandRequest", "RunnerPairingProposalRequest", "RunnerPairingStatusRequest"} {
		toolContractMap(schemas, value)["additionalProperties"] = false
		toolContractMap(schemas, value, "properties", "project_id")["minLength"] = 1
		toolContractMap(schemas, value, "properties", "project_id")["maxLength"] = 64
		if value != "RunnerPairing" {
			field := toolContractMap(schemas, value, "properties", "idempotency_key")
			field["minLength"], field["maxLength"] = 8, 128
		}
	}
	properties := toolContractMap(schemas, "RunnerPairing", "properties")
	for _, field := range properties {
		field.(map[string]any)["readOnly"] = true
	}
	toolContractMap(properties, "id")["pattern"] = "^[a-f0-9]{32}$"
	toolContractMap(properties, "state")["enum"] = []string{"waiting", "proposed", "approved", "rejected", "expired"}
	toolContractMap(properties, "control_url")["format"] = "uri"
	toolContractMap(properties, "worker_id")["pattern"] = "^remote-[a-f0-9]{32}$"
	toolContractMap(properties, "can_approve")["description"] = "Current browser-read qualification only. Machine responses always return false. Approval records registration, not online execution, fresh inventory or enabled profiles."
	for _, key := range []string{"created_at", "expires_at", "credential_expires_at"} {
		toolContractMap(properties, key)["format"] = "date-time"
	}
	for _, model := range []string{"RunnerPairing", "RunnerPairingProposalRequest"} {
		toolContractMap(schemas, model, "properties", "credential_sha256")["pattern"] = "^[a-f0-9]{64}$"
		toolContractMap(schemas, model, "properties", "device_name")["x-max-utf8-bytes"] = 100
	}
	toolContractMap(schemas, "RunnerPairingCommandRequest", "properties", "action")["enum"] = []string{"approve", "reject"}
	toolContractMap(schemas, "RunnerPairingCommandRequest", "properties", "expected_version")["minimum"] = 1
}
