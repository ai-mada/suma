package main

import wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"

func workBuddyCloudTurnModels() []any {
	return []any{wc.CloudTurnSource{}, wc.CloudTurnPromptRequest{}, wc.CloudTurn{}, wc.CloudTurnView{}, wc.CloudTurnEntry{}, wc.CloudTurnPermissionOption{}, wc.CloudTurnPermission{}, wc.CloudTurnCommand{}, wc.CloudTurnPermissionReplyRequest{}, wc.CloudTurnControlRequest{}, wc.CloudTurnPage{}}
}

func workBuddyCloudTurnContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/workbuddy-cloud-submissions/{id}/turns", "get", "", "CloudTurnView", "200"},
		{"/workbuddy-cloud-submissions/{id}/turns", "post", "CloudTurnPromptRequest", "CloudTurn", "202"},
		{"/workbuddy-cloud-turns/{id}", "get", "", "CloudTurnPage", "200"},
		{"/workbuddy-cloud-turns/{id}/permissions/{permission_id}/reply", "post", "CloudTurnPermissionReplyRequest", "CloudTurnCommand", "202"},
		{"/workbuddy-cloud-turns/{id}/cancel", "post", "CloudTurnControlRequest", "CloudTurnCommand", "202"},
		{"/workbuddy-cloud-turns/{id}/reconcile", "post", "CloudTurnControlRequest", "CloudTurnCommand", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		browserOnly(op)
		successStatus(op, route.status)
		op["description"] = "Current browser owner of the exact original cloud submission only. A new explicit prompt receives one non-reconstructible runtime permit; original receipts never reconnect or resend. Cross-replica permission/cancel inbox commands are consumed once by the original fenced lease owner. RPC/HTTP/tool facts stay separate from platform run, code delivery, tests and acceptance."
		parameters := []any{}
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			if p["in"] == "query" && (route.method != "get" || p["name"] != "project_id") {
				continue
			}
			p["required"], p["schema"] = true, map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
			parameters = append(parameters, p)
		}
		if route.output == "CloudTurnPage" {
			for _, value := range []struct {
				name              string
				min, max, initial int
			}{{"after", 0, wc.CloudTurnMaxEntries, 0}, {"limit", 1, 200, 50}} {
				parameters = append(parameters, map[string]any{"name": value.name, "in": "query", "schema": map[string]any{"type": "integer", "minimum": value.min, "maximum": value.max, "default": value.initial}})
			}
		}
		op["parameters"] = parameters
		if route.input != "" {
			request(op, route.input)
			op["x-query-parameters-forbidden"], op["x-max-request-body-bytes"] = true, 64<<10
			op["x-idempotency"] = "Current owner authorization recovers the same durable operation before new CAS/admission. It does not recreate the original Prompt or control send permit."
		} else {
			op["x-read-only"], op["x-no-external-effects"] = true, true
		}
		if route.path == "/workbuddy-cloud-turns/{id}/reconcile" {
			op["x-no-external-effects"] = true
			op["description"] = "Current owner explicitly fences an overdue original local runtime into unknown. No provider call, new prompt, restored permission reference or remote-stop claim. The original head remains blocked."
		}
		for _, raw := range op["responses"].(map[string]any) {
			raw.(map[string]any)["headers"] = map[string]any{"Cache-Control": map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}}, "Referrer-Policy": map[string]any{"schema": map[string]any{"type": "string", "const": "no-referrer"}}}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"CloudTurnSource", "CloudTurnPromptRequest", "CloudTurn", "CloudTurnView", "CloudTurnEntry", "CloudTurnPermissionOption", "CloudTurnPermission", "CloudTurnCommand", "CloudTurnPermissionReplyRequest", "CloudTurnControlRequest", "CloudTurnPage"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		input := name == "CloudTurnPromptRequest" || name == "CloudTurnPermissionReplyRequest" || name == "CloudTurnControlRequest"
		for key, raw := range toolContractMap(model, "properties") {
			field := raw.(map[string]any)
			if !input {
				field["readOnly"] = true
			}
			switch key {
			case "project_id", "owner_id", "id", "turn_id", "permission_id":
				field["minLength"], field["maxLength"] = 1, 64
				if name == "CloudTurnPermissionOption" {
					field["maxLength"] = 128
				}
			case "tool_call_id", "option_id":
				field["minLength"], field["maxLength"] = 1, 128
			case "operation_id", "idempotency_key":
				field["minLength"], field["maxLength"], field["x-max-utf8-bytes"] = 8, 128, 128
			case "expected_version", "head_version_at_start", "version":
				minimum := 1
				if name == "CloudTurnPromptRequest" || name == "CloudTurnView" || key == "head_version_at_start" {
					minimum = 0
				}
				maximum := wc.MaxVersion
				if key == "expected_version" {
					maximum = wc.MaxVersion - 1
				}
				if name == "CloudTurnPromptRequest" || key == "head_version_at_start" {
					maximum = wc.MaxVersion - 2
				}
				field["minimum"], field["maximum"] = minimum, maximum
			case "expected_submission_version", "expected_run_version":
				field["minimum"], field["maximum"] = 1, wc.MaxVersion-1
			case "attempt":
				field["minimum"], field["maximum"] = 1, wc.MaxVersion
			case "fence":
				field["minimum"] = 1
			case "prompt":
				field["minLength"], field["maxLength"], field["x-max-utf8-bytes"] = 1, 32000, 32000
			case "prompt_sha256", "assignment_sha256", "details_sha256":
				field["pattern"] = "^[0-9a-f]{64}$"
			case "sequence", "next_after":
				minimum := 0
				if key == "sequence" {
					minimum = 1
				}
				field["minimum"], field["maximum"] = minimum, wc.CloudTurnMaxEntries
			case "text":
				field["maxLength"], field["x-max-utf8-bytes"] = wc.CloudTurnMaxEntryBytes, wc.CloudTurnMaxEntryBytes
			case "items":
				field["maxItems"] = 200
			case "permissions":
				field["maxItems"] = wc.CloudTurnMaxPermissions
			case "options":
				field["minItems"], field["maxItems"] = 1, 32
			case "state":
				if name == "CloudTurn" {
					field["enum"] = []string{"prepared", "running", "waiting_permission", "completed", "unknown", "rejected"}
				}
				if name == "CloudTurnCommand" {
					field["enum"] = []string{"prepared", "sent", "acknowledged", "unknown", "rejected"}
				}
				if name == "CloudTurnPermission" {
					field["enum"] = []string{"pending", "prepared", "sent", "acknowledged", "unknown", "rejected"}
				}
			case "kind":
				if name == "CloudTurnEntry" {
					field["enum"] = []string{"assistant_text", "tool_state", "plan_summary", "notice", "unsupported"}
				}
				if name == "CloudTurnCommand" {
					field["enum"] = []string{"permission", "cancel", "reconcile"}
				}
				if name == "CloudTurnPermissionOption" {
					field["enum"] = []string{"allow_once", "reject_once", "allow_always", "reject_always"}
				}
			case "allowed_actions":
				field["uniqueItems"], field["maxItems"], field["items"] = true, 3, map[string]any{"type": "string", "enum": []string{"prompt", "cancel", "reconcile"}}
			case "usage_state":
				field["enum"] = []string{"partial", "unreported"}
			case "stop_reason":
				field["enum"] = []string{"end_turn", "max_tokens", "max_turn_requests", "refusal", "cancelled"}
			case "error_code":
				field["enum"] = []string{"workbuddy_cloud_turn_unknown", "workbuddy_cloud_turn_interrupted", "workbuddy_cloud_turn_limited", "workbuddy_cloud_turn_authorization_changed", "workbuddy_cloud_turn_rejected", "workbuddy_cloud_turn_unavailable"}
			}
		}
	}
}
