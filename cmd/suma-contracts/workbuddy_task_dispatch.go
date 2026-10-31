package main

import "github.com/ai-mada/suma/internal/platform"

func workBuddyTaskDispatchModels() []any {
	return []any{platform.WorkBuddyTaskDispatchPreviewRequest{}, platform.WorkBuddyTaskDispatchPlan{}, platform.WorkBuddyTaskDispatchStartRequest{}}
}

// A cloud task is a protected external workflow, never a fabricated CLI
// profile. All wire types originate in the same Go domain as HTTP admission.
func workBuddyTaskDispatchContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/workbuddy-task-dispatch-plans", "post", "WorkBuddyTaskDispatchPreviewRequest", "WorkBuddyTaskDispatchPlan", "201"},
		{"/workbuddy-task-dispatch-plans/{id}", "get", "", "WorkBuddyTaskDispatchPlan", "200"},
		{"/workbuddy-task-dispatch-plans/{id}/start", "post", "WorkBuddyTaskDispatchStartRequest", "Run", "201"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		browserOnly(op)
		successStatus(op, route.status)
		op["x-no-provider-effects"] = true
		op["description"] = "Current browser project writer and original account owner only. A fixed task context is explicitly previewed, then atomically bound to one original DAG run and one cloud delegation. Provider completion is not candidate, independent-test, acceptance or deployment evidence. Model and usage remain provider-controlled/unverified."
		parameters := []any{}
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			if p["in"] == "query" && (route.method != "get" || p["name"] != "project_id") {
				continue
			}
			p["required"], p["schema"] = true, map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
			parameters = append(parameters, p)
		}
		op["parameters"] = parameters
		if route.input != "" {
			request(op, route.input)
			op["x-query-parameters-forbidden"], op["x-max-request-body-bytes"] = true, 64<<10
			op["x-idempotency"] = "Recover the exact original body/key under current owner project authority before new CAS, expiry or consumed-delegation checks. A receipt never grants another dispatch."
		} else {
			op["x-read-only"] = true
		}
		for _, raw := range op["responses"].(map[string]any) {
			raw.(map[string]any)["headers"] = map[string]any{"Cache-Control": map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}}, "Referrer-Policy": map[string]any{"schema": map[string]any{"type": "string", "const": "no-referrer"}}}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"WorkBuddyTaskDispatchPreviewRequest", "WorkBuddyTaskDispatchPlan", "WorkBuddyTaskDispatchStartRequest"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for key, raw := range toolContractMap(model, "properties") {
			field := raw.(map[string]any)
			if name == "WorkBuddyTaskDispatchPlan" {
				field["readOnly"] = true
			}
			switch key {
			case "id", "project_id", "owner_id", "task_id", "connection_id", "delegation_id":
				field["minLength"], field["maxLength"] = 1, 64
			case "version":
				field["enum"] = []int{1, 2}
			case "expected_version", "task_version", "expected_task_version", "connection_version", "expected_connection_version":
				field["minimum"], field["maximum"] = 1, 2147483646
			case "grant_generation":
				field["minimum"], field["maximum"] = 1, 2147483647
			case "state":
				field["enum"] = []string{"ready", "started"}
			case "node_spec_sha256", "request_sha256":
				field["pattern"] = "^[0-9a-f]{64}$"
			case "knowledge":
				field["maxItems"] = 16
			case "idempotency_key":
				field["minLength"], field["maxLength"], field["x-max-utf8-bytes"] = 8, 128, 128
			case "context":
				field["description"] = "Server-resolved task, requirement, adopted design, published test-plan and explicitly selected knowledge. Prompt limit is32000 UTF-8bytes; no truncation. A protected code-baseline task is rejected until trusted cloud baseline delivery exists."
			}
		}
	}
}
