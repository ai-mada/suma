package main

import ic "github.com/ai-mada/suma/internal/toolinspectioncontract"

func toolRecoveryModels() []any {
	return []any{ic.ToolMaintenanceCapability{}, ic.ToolMaintenanceStatus{}, ic.ToolInspectionStopProof{}, ic.ToolInspectionRecovery{}, ic.ToolInspectionRecoveryCreateRequest{}, ic.ToolInspectionRecoveryPollRequest{}, ic.PoolToolInspectionRecoveryPollRequest{}, ic.ToolInspectionRecoveryPoll{}, ic.ToolInspectionRecoveryProofRequest{}, ic.PoolToolInspectionRecoveryProofRequest{}, ic.ToolInspectionRecoveryAcknowledgementRequest{}, ic.PoolToolInspectionRecoveryAcknowledgementRequest{}, ic.ToolInspectionRecoveryList{}}
}

func toolRecoveryRoutes(add operationAdder) {
	for _, route := range []struct{ path, method, input, output string }{
		{"/tool-inspections/{id}/recoveries", "post", "ToolInspectionRecoveryCreateRequest", "ToolInspectionRecovery"},
		{"/tool-inspections/{id}/recoveries", "get", "", "ToolInspectionRecoveryList"},
		{"/tool-inspection-recoveries/{id}", "get", "", "ToolInspectionRecovery"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		browserOnly(op)
		status := "200"
		if route.input != "" {
			request(op, route.input)
			status = "201"
		}
		successStatus(op, status)
		op["x-response-cache-control"] = "no-store"
		op["description"] = "One current project manager authorizes only reconciliation and release of this original unknown maintenance reservation, for300 seconds. The exact claim and immutable source stay fixed. Creation replay recovers its original snapshot; read the original ID for its head. Native stop proof, current exact machine/pool authority and durable node acknowledgement are all required. Closure preserves the unknown result and never probes, kills, enables or accepts a task."
	}
	for _, namespace := range []struct{ path, security, scope, poll, proof, ack string }{
		{"/runner", "runnerAuth", "runner", "ToolInspectionRecoveryPollRequest", "ToolInspectionRecoveryProofRequest", "ToolInspectionRecoveryAcknowledgementRequest"},
		{"/pool-runner", "poolRunnerAuth", "pool", "PoolToolInspectionRecoveryPollRequest", "PoolToolInspectionRecoveryProofRequest", "PoolToolInspectionRecoveryAcknowledgementRequest"},
	} {
		for _, route := range []struct {
			suffix, input, output string
			write                 bool
		}{
			{"", namespace.poll, "ToolInspectionRecoveryPoll", false},
			{"/proofs", namespace.proof, "ToolInspectionRecovery", true},
			{"/acknowledgements", namespace.ack, "ToolInspectionRecovery", true},
		} {
			op := add(namespace.path+"/tool-inspections/recovery"+route.suffix, "post", route.output, false, route.write)
			request(op, route.input)
			successStatus(op, "200")
			op["security"] = []any{map[string]any{namespace.security: []string{}}}
			op["x-required-token-scope"] = namespace.scope
			op["x-read-only"] = !route.write
			op["description"] = "The original authenticated inventory owner reads existing durable stop evidence and preserves its exact reference. Empty poll is recovery:null. No probe or kill is authorized. Missing spawn coverage, unconfirmed process trees, pending inventory or absent receipts cannot become proof. Proof acknowledgement and closed_unknown are atomic; lost replies recover the same operation. A later current periodic snapshot and a separately requested inspection remain distinct."
		}
	}
}

func toolRecoverySchemas(api map[string]any) {
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ToolMaintenanceCapability", "ToolMaintenanceStatus", "ToolInspectionStopProof", "ToolInspectionRecovery", "ToolInspectionRecoveryCreateRequest", "ToolInspectionRecoveryPollRequest", "PoolToolInspectionRecoveryPollRequest", "ToolInspectionRecoveryPoll", "ToolInspectionRecoveryProofRequest", "PoolToolInspectionRecoveryProofRequest", "ToolInspectionRecoveryAcknowledgementRequest", "PoolToolInspectionRecoveryAcknowledgementRequest", "ToolInspectionRecoveryList"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for key, raw := range model["properties"].(map[string]any) {
			value := raw.(map[string]any)
			switch key {
			case "id", "project_id", "worker_id", "runner_id", "tenant_id", "requested_by", "recovery_id":
				value["pattern"] = `^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,149}$`
			case "instance_id", "idempotency_key":
				value["minLength"], value["maxLength"] = 1, 128
				value["x-min-utf8-bytes"], value["x-max-utf8-bytes"] = 8, 128
			case "version", "inspection_version", "expected_version":
				value["minimum"] = 1
			case "source_sha256", "receipt_sha256", "proof_sha256":
				value["pattern"] = `^[a-f0-9]{64}$`
			case "created_at", "expires_at", "observed_at", "checked_at", "proof_stored_at", "closed_at":
				value["format"] = "date-time"
			case "native_process_count", "stopped_process_count":
				value["minimum"], value["maximum"] = 0, ic.MaxNativeProcesses
			}
		}
	}
	toolContractMap(schemas, "ToolMaintenanceCapability", "properties", "protocol")["pattern"] = `^suma-tool-maintenance-v[1-9][0-9]{0,3}$`
	status := toolContractMap(schemas, "ToolMaintenanceStatus")
	toolContractMap(status, "properties", "state")["enum"] = []string{"supported", "expired", "unsupported", "unknown"}
	status["description"] = "Authenticated continuous-consumer protocol declaration, not health or proof. checked_at is server time. Supported freshness uses expires_at minus checked_at and client monotonic elapsed time, never the local wall clock or expires_at minus observed_at. Old ACK replay does not renew this watermark. Recovery may use the exact last-known logical owner even when expired; a new inspection needs fresh support."
	known := []string{"protocol", "instance_id", "observed_at", "expires_at"}
	status["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "unknown"}}, "not": inspectionAnyPresence(known...)},
		map[string]any{"properties": map[string]any{"state": map[string]any{"enum": []string{"supported", "expired"}}, "protocol": map[string]any{"const": ic.MaintenanceProtocol}}, "required": known},
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "unsupported"}, "protocol": map[string]any{"pattern": `^suma-tool-maintenance-v[1-9][0-9]{0,3}$`, "not": map[string]any{"const": ic.MaintenanceProtocol}}}, "required": known},
	}
	proof := toolContractMap(schemas, "ToolInspectionStopProof")
	toolContractMap(proof, "properties", "protocol")["const"] = ic.ProbeStopProtocol
	for _, field := range []string{"probe_returned", "all_native_trees_stopped", "no_pending_inventory"} {
		toolContractMap(proof, "properties", field)["const"] = true
	}
	toolContractMap(proof, "properties", "stopped_process_count")["x-equal-to"] = "native_process_count"
	proof["description"] = "Only existing durable evidence: exact original claim/source, closed spawn coverage, returned probe, all owned native process trees confirmed stopped and no pending inventory. Counts refer to owned native starts/trees, not an inferred historical descendant total; stopped count must equal native count. A digest, absent PID or elapsed timeout alone proves nothing."
	recovery := toolContractMap(schemas, "ToolInspectionRecovery")
	toolContractMap(recovery, "properties", "state")["enum"] = []string{"requested", "proof_stored", "closed_unknown", "expired"}
	proofFields := []string{"proof", "proof_sha256", "proof_stored_at"}
	recovery["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "requested"}}, "not": inspectionAnyPresence(append(proofFields, "closed_at")...)},
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "proof_stored"}}, "required": proofFields, "not": map[string]any{"required": []string{"closed_at"}}},
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "closed_unknown"}}, "required": append(append([]string{}, proofFields...), "closed_at")},
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "expired"}}, "not": map[string]any{"required": []string{"closed_at"}}},
	}
	recovery["allOf"] = []any{map[string]any{"if": inspectionAnyPresence(proofFields...), "then": map[string]any{"required": proofFields}}}
	toolContractMap(schemas, "ToolInspectionRecoveryList", "properties", "items")["maxItems"] = 10
	inspection := toolContractMap(schemas, "ToolInspection")
	toolContractMap(inspection, "properties", "state")["enum"] = []string{"pending", "claimed", "completed", "failed", "unknown", "expired", "superseded", "closed_unknown"}
	for _, field := range []string{"recovery_id", "closed_at"} {
		if field == "closed_at" {
			toolContractMap(inspection, "properties", field)["format"] = "date-time"
		} else {
			toolContractMap(inspection, "properties", field)["pattern"] = `^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,149}$`
		}
	}
	branches := inspection["oneOf"].([]any)
	for _, raw := range branches {
		branch := raw.(map[string]any)
		absent := inspectionAnyPresence("recovery_id", "closed_at")
		if previous := branch["not"]; previous != nil {
			branch["not"] = map[string]any{"anyOf": []any{previous, absent}}
		} else {
			branch["not"] = absent
		}
	}
	inspection["oneOf"] = append(branches, map[string]any{"properties": map[string]any{"state": map[string]any{"const": "closed_unknown"}, "fence": map[string]any{"minimum": 1}, "result_code": map[string]any{"enum": []string{"probe_interrupted", "probe_timeout", "probe_failed"}}}, "required": []string{"claim_id", "claimed_at", "lease_until", "result_code", "recovery_id", "closed_at"}, "not": inspectionAnyPresence("observed_at", "inventory_operation_id")})
}
