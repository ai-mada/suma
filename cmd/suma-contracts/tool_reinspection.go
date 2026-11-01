package main

import ic "github.com/ai-mada/suma/internal/toolinspectioncontract"

func toolReinspectionModels() []any {
	return []any{ic.ToolInspection{}, ic.ToolInspectionReference{}, ic.ToolInspectionCreateRequest{}, ic.ToolInspectionClaimRequest{}, ic.PoolToolInspectionClaimRequest{}, ic.ToolInspectionClaim{}, ic.ToolInspectionFailureRequest{}, ic.PoolToolInspectionFailureRequest{}, ic.ToolInspectionList{}}
}

func toolReinspectionRoutes(add operationAdder) {
	for _, route := range []struct{ path, method, input, output string }{
		{"/tool-profiles/{id}/inspections", "post", "ToolInspectionCreateRequest", "ToolInspection"},
		{"/tool-profiles/{id}/inspections", "get", "", "ToolInspectionList"},
		{"/tool-inspections/{id}", "get", "", "ToolInspection"},
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
		op["description"] = "Current project tool-management authority binds the exact immutable profile version, full digest, worker and applicable pool grant. A repeated creation key returns its original pending snapshot; GET the original ID for its current head. The bounded list returns at most10 current/recent records and never replaces an unresolved local intent. Inspection completion is not enablement, model health, task execution or acceptance."
	}
	for _, namespace := range []struct{ path, scope, security, claim, failure string }{
		{"/runner", "runner", "runnerAuth", "ToolInspectionClaimRequest", "ToolInspectionFailureRequest"},
		{"/pool-runner", "pool", "poolRunnerAuth", "PoolToolInspectionClaimRequest", "PoolToolInspectionFailureRequest"},
	} {
		for _, route := range []struct{ path, input, output string }{
			{"/tool-inspections/claim", namespace.claim, "ToolInspectionClaim"},
			{"/tool-inspections/results", namespace.failure, "ToolInspection"},
		} {
			op := add(namespace.path+route.path, "post", route.output, false, true)
			request(op, route.input)
			successStatus(op, "200")
			op["security"] = []any{map[string]any{namespace.security: []string{}}}
			op["x-required-token-scope"] = namespace.scope
			op["description"] = "The original authenticated outbound inventory owner uses a persisted claim nonce and fence. A repeated nonce recovers its original claim; no pending item is represented by inspection:null. Expired claims become unknown without reassignment. Failure reports preserve the original reference and never trigger another probe. Pool identity and project grant remain exact. Successful inspection is committed through its referenced original tool-inventory operation."
		}
	}
}

// Keep generated request validators aligned with the public, fixed-purpose
// contract: callers select an existing tool; they cannot send executable input.
func toolReinspectionSchemas(api map[string]any) {
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ToolInspection", "ToolInspectionReference", "ToolInspectionCreateRequest", "ToolInspectionClaimRequest", "PoolToolInspectionClaimRequest", "ToolInspectionClaim", "ToolInspectionFailureRequest", "PoolToolInspectionFailureRequest", "ToolInspectionList"} {
		target := toolContractMap(schemas, name)
		target["additionalProperties"] = false
		for key, value := range target["properties"].(map[string]any) {
			property := value.(map[string]any)
			switch key {
			case "id", "project_id", "worker_id", "tool_profile_id", "command_id", "requested_by", "tenant_id", "runner_id", "inspection_id":
				property["pattern"] = `^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,149}$`
			case "claim_id", "idempotency_key", "inventory_operation_id":
				// Operation measures UTF-8 bytes; JSON Schema minLength counts
				// characters and must not reject a valid shorter Unicode nonce.
				property["minLength"], property["maxLength"] = 1, 128
				property["x-min-utf8-bytes"], property["x-max-utf8-bytes"] = 8, 128
			case "fence":
				property["minimum"], property["maximum"] = 1, 9007199254740991
				if name == "ToolInspection" {
					property["minimum"] = 0
				}
			case "version", "expected_version", "tool_profile_version":
				property["minimum"] = 1
			case "profile_sha256":
				property["pattern"] = `^[a-f0-9]{64}$`
			case "created_at", "expires_at", "claimed_at", "lease_until", "observed_at":
				property["format"] = "date-time"
			}
		}
	}
	toolContractMap(schemas, "ToolInspection", "properties", "state")["enum"] = []string{"pending", "claimed", "completed", "failed", "unknown", "expired", "superseded"}
	toolContractMap(schemas, "ToolInspection")["oneOf"] = toolInspectionStateRelations()
	toolContractMap(schemas, "ToolInspection", "properties", "result_code")["enum"] = []string{"reported_available", "reported_unavailable", "profile_changed", "command_missing", "probe_interrupted", "probe_timeout", "probe_failed", "not_started", "request_expired", "source_changed"}
	toolContractMap(schemas, "ToolInspectionList", "properties", "items")["maxItems"] = 10
	for _, name := range []string{"ToolInspectionFailureRequest", "PoolToolInspectionFailureRequest"} {
		target := toolContractMap(schemas, name)
		toolContractMap(target, "properties", "outcome")["enum"] = []string{"failed", "unknown"}
		toolContractMap(target, "properties", "failure_code")["enum"] = []string{"not_started", "probe_interrupted", "probe_timeout", "probe_failed"}
		target["oneOf"] = []any{
			map[string]any{"properties": map[string]any{"outcome": map[string]any{"const": "failed"}, "failure_code": map[string]any{"const": "not_started"}}},
			map[string]any{"properties": map[string]any{"outcome": map[string]any{"const": "unknown"}, "failure_code": map[string]any{"enum": []string{"probe_interrupted", "probe_timeout", "probe_failed"}}}},
		}
	}
	toolContractMap(schemas, "ToolInspection")["description"] = "A fixed-purpose current-authority inspection, pending at most300 seconds and claimed for60 seconds. Unknown effects are reconciled against the original claim and report, never automatically re-executed. Source changes supersede authority. Completed means only the associated inventory was persisted; it does not authorize tasks, installation or enablement."
	receipt := toolContractMap(schemas, "ToolInventoryReceipt")
	toolContractMap(receipt, "properties", "inspection_state")["const"] = "completed"
	receipt["oneOf"] = []any{
		map[string]any{"required": []string{"inspection", "inspection_state"}},
		map[string]any{"not": inspectionAnyPresence("inspection", "inspection_state")},
	}
}

func inspectionAnyPresence(names ...string) map[string]any {
	branches := make([]any, 0, len(names))
	for _, name := range names {
		branches = append(branches, map[string]any{"required": []string{name}})
	}
	return map[string]any{"anyOf": branches}
}

// Each state preserves the same pairs enforced by ValidateInspection. In
// particular, a completed object cannot be accepted without its original
// positive fence, claim and inventory operation; a pending one has none.
func toolInspectionStateRelations() []any {
	branches := make([]any, 0, 7)
	for _, rule := range []struct {
		state   string
		claim   int // 0 unclaimed, 1 claimed, 2 either (superseded source)
		results []string
	}{
		{"pending", 0, nil}, {"claimed", 1, nil},
		{"completed", 1, []string{"reported_available", "reported_unavailable", "profile_changed", "command_missing"}},
		{"failed", 1, []string{"not_started"}},
		{"unknown", 1, []string{"probe_interrupted", "probe_timeout", "probe_failed"}},
		{"expired", 0, []string{"request_expired"}},
		{"superseded", 2, []string{"source_changed"}},
	} {
		properties := map[string]any{"state": map[string]any{"const": rule.state}}
		required, absent := []string{}, []string{}
		branch := map[string]any{"properties": properties}
		claimFields := []string{"claim_id", "claimed_at", "lease_until"}
		if rule.claim == 0 {
			properties["fence"] = map[string]any{"const": 0}
			absent = append(absent, claimFields...)
		} else if rule.claim == 1 {
			properties["fence"] = map[string]any{"minimum": 1}
			required = append(required, claimFields...)
		} else {
			branch["oneOf"] = []any{
				map[string]any{"properties": map[string]any{"fence": map[string]any{"const": 0}}, "not": inspectionAnyPresence(claimFields...)},
				map[string]any{"properties": map[string]any{"fence": map[string]any{"minimum": 1}}, "required": claimFields},
			}
		}
		if rule.results == nil {
			absent = append(absent, "result_code")
		} else {
			properties["result_code"] = map[string]any{"enum": rule.results}
			required = append(required, "result_code")
		}
		if rule.state == "completed" {
			required = append(required, "observed_at", "inventory_operation_id")
		} else {
			absent = append(absent, "observed_at", "inventory_operation_id")
		}
		if len(required) > 0 {
			branch["required"] = required
		}
		if len(absent) > 0 {
			branch["not"] = inspectionAnyPresence(absent...)
		}
		branches = append(branches, branch)
	}
	return branches
}
