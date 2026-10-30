package main

import wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"

func workBuddyCloudCollaborationModels() []any {
	return []any{wc.CloudCollaborationSource{}, wc.CloudHistoryLoadRequest{}, wc.CloudHistoryLoad{}, wc.CloudHistoryEntry{}, wc.CloudCollaborationView{}, wc.CloudHistoryPage{}}
}

// The load command establishes an external connection. Only the two archive
// GETs are free of external effects; ACP's load acknowledgement is not a task
// result or a guarantee that the provider replayed its complete history.
func workBuddyCloudCollaborationContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output string }{
		{"/workbuddy-cloud-submissions/{id}/collaboration", "get", "", "CloudCollaborationView"},
		{"/workbuddy-cloud-submissions/{id}/history-loads", "post", "CloudHistoryLoadRequest", "CloudHistoryLoad"},
		{"/workbuddy-cloud-history-loads/{id}", "get", "", "CloudHistoryPage"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, "200")
		browserOnly(op)
		op["description"] = "Current original owner and project authority are rechecked for the exact submission, run, node and immutable source. No OAuth, ACP ticket, RPC handle or raw packet is returned. GET reads only retained, owner-private observations; loading is explicit and never implies task completion, full history coverage or a pending interaction."
		parameters := []any{}
		hasProject := false
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			if route.method == "post" && p["in"] == "query" {
				continue
			}
			if p["name"] == "project_id" || p["in"] == "path" {
				p["required"] = true
				p["schema"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
			}
			hasProject = hasProject || p["name"] == "project_id"
			parameters = append(parameters, p)
		}
		if route.method == "get" && !hasProject {
			p := queryParameter("project_id", "Exact current project.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
			p["required"] = true
			parameters = append(parameters, p)
		}
		op["parameters"] = parameters
		if route.input != "" {
			request(op, route.input)
			op["x-max-request-body-bytes"] = 64 << 10
			op["x-query-parameters-forbidden"] = true
			op["x-external-effects"] = "One explicitly prepared connection and exact session/load; never Prompt, permission reply or Cancel."
			op["x-idempotency"] = "Original operation/body/source/head binding recovers the same load without another Attach; current authority still applies."
		} else {
			op["x-read-only"], op["x-no-external-effects"] = true, true
		}
		if route.path == "/workbuddy-cloud-history-loads/{id}" {
			appendQuery(op, queryParameter("after", "Local sequence within this load, not a provider replay cursor.", map[string]any{"type": "integer", "minimum": 0, "maximum": wc.CloudHistoryMaxEntries, "default": 0}))
			appendQuery(op, queryParameter("limit", "Bounded retained observations.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}))
		}
		responses := op["responses"].(map[string]any)
		responses["404"] = map[string]any{"description": "Source/load is absent or not visible to this owner."}
		responses["409"] = map[string]any{"description": "Source/head changed, an active load or quota prevents another load, or the operation body differs. Never automatically reconnect."}
		responses["503"] = map[string]any{"description": "The configured component is unavailable; no provider health is inferred."}
		for _, raw := range responses {
			raw.(map[string]any)["headers"] = map[string]any{"Cache-Control": map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}}, "Referrer-Policy": map[string]any{"schema": map[string]any{"type": "string", "const": "no-referrer"}}}
		}
	}

	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, key string, values map[string]any) {
		for name, value := range values {
			toolContractMap(schemas, model, "properties", key)[name] = value
		}
	}
	for _, name := range []string{"CloudCollaborationSource", "CloudHistoryLoadRequest", "CloudHistoryLoad", "CloudHistoryEntry", "CloudCollaborationView", "CloudHistoryPage"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for key, raw := range toolContractMap(model, "properties") {
			p := raw.(map[string]any)
			if name != "CloudHistoryLoadRequest" {
				p["readOnly"] = true
			}
			switch key {
			case "id", "project_id", "submission_id":
				p["minLength"], p["maxLength"] = 1, 64
			case "run_id":
				p["minLength"], p["maxLength"] = 1, 100
			case "node_id":
				p["minLength"], p["maxLength"] = 1, 150
			case "task_id":
				p["minLength"], p["maxLength"], p["pattern"] = 1, 200, "^[A-Za-z0-9_-]+$"
			case "node_spec_sha256", "request_sha256":
				p["pattern"] = "^[0-9a-f]{64}$"
			case "version", "submission_version":
				p["minimum"], p["maximum"] = 1, wc.MaxVersion
			case "expected_submission_version":
				p["minimum"], p["maximum"] = 1, wc.MaxVersion-1
			case "expected_version", "head_version_at_start":
				p["minimum"], p["maximum"] = 0, wc.MaxVersion-1
			case "operation_id", "idempotency_key":
				p["minLength"], p["maxLength"], p["x-max-utf8-bytes"] = 8, 128, 128
			case "started_at", "deadline_at", "finished_at":
				p["format"], p["pattern"] = "date-time", "Z$"
			case "entry_count", "unsupported_count", "total", "next_after":
				p["minimum"], p["maximum"] = 0, wc.CloudHistoryMaxEntries
			case "sequence":
				p["minimum"], p["maximum"] = 1, wc.CloudHistoryMaxEntries
			case "text_bytes":
				p["minimum"], p["maximum"] = 0, wc.CloudHistoryMaxTextBytes
			}
		}
	}
	field("CloudCollaborationView", "version", map[string]any{"minimum": 0})
	field("CloudHistoryLoad", "coverage", map[string]any{"const": "unverified"})
	field("CloudHistoryLoad", "state", map[string]any{"enum": []string{"loading", "loaded", "interrupted", "limited", "rejected"}})
	field("CloudHistoryLoad", "overdue", map[string]any{"description": "Derived from database time for loading only; does not change the durable state/version. Terminal loads always return false."})
	field("CloudHistoryLoad", "error_code", map[string]any{"enum": []string{"workbuddy_cloud_history_unavailable", "workbuddy_cloud_history_interrupted", "workbuddy_cloud_history_limited", "workbuddy_cloud_history_rejected"}})
	field("CloudHistoryEntry", "kind", map[string]any{"enum": []string{"user_text", "assistant_text", "tool_summary", "plan_summary", "unsupported"}})
	field("CloudHistoryEntry", "text", map[string]any{"minLength": 1, "maxLength": wc.CloudHistoryMaxEntryBytes, "x-max-utf8-bytes": wc.CloudHistoryMaxEntryBytes})
	field("CloudCollaborationView", "allowed_actions", map[string]any{"maxItems": 1, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": []string{"load_history"}}})
	field("CloudHistoryPage", "items", map[string]any{"maxItems": 100})
	// These state-dependent response rules prevent a syntactically successful
	// receipt from claiming both an in-flight load and a terminal outcome.
	toolContractMap(schemas, "CloudHistoryLoad")["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "loading"}}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"finished_at"}}, map[string]any{"required": []string{"error_code"}}}}},
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "loaded"}, "overdue": map[string]any{"const": false}}, "required": []string{"finished_at"}, "not": map[string]any{"required": []string{"error_code"}}},
		map[string]any{"properties": map[string]any{"state": map[string]any{"enum": []string{"interrupted", "limited", "rejected"}}, "overdue": map[string]any{"const": false}}, "required": []string{"finished_at", "error_code"}},
	}
}
