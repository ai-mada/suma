package main

import wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"

func workBuddyCloudHistorySharingModels() []any {
	return []any{wc.CloudHistorySharePreviewRequest{}, wc.CloudHistoryShareRequest{}, wc.CloudHistorySharePreview{}, wc.CloudHistorySharedEntry{}, wc.CloudHistorySharedObservation{}}
}

func workBuddyCloudHistorySharingContracts(add operationAdder, api map[string]any) {
	for _, r := range []struct{ suffix, input, output, status string }{
		{"share-preview", "CloudHistorySharePreviewRequest", "CloudHistorySharePreview", "200"},
		{"shares", "CloudHistoryShareRequest", "TaskConversationMessage", "201"},
	} {
		op := add("/workbuddy-cloud-history-loads/{id}/"+r.suffix, "post", r.output, false, true)
		browserOnly(op)
		request(op, r.input)
		successStatus(op, r.status)
		op["x-no-external-effects"], op["x-query-parameters-forbidden"], op["x-max-request-body-bytes"] = true, true, 64<<10
		op["description"] = "Current human owner explicitly shares selected terminal observations into a same-project human task discussion. Exact source, target version and preview are checked server-side. The resulting copy follows current target-task read authority. No provider call, model, task transition or acceptance is caused."
		parameters := []any{}
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			if p["in"] == "query" {
				continue
			}
			if p["in"] == "path" {
				p["required"], p["schema"] = true, map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
			}
			parameters = append(parameters, p)
		}
		op["parameters"] = parameters
		if r.suffix == "share-preview" {
			op["x-read-only"] = true
			op["description"] = op["description"].(string) + " Preview writes no message, provenance, receipt or notification. Private preview text must not enter browser recovery storage."
		} else {
			op["x-idempotency"] = "Current target-task write authority and original metadata request recover the same immutable message before new source/task qualification; message, provenance, notifications, activity and receipt commit in one transaction."
		}
		responses := op["responses"].(map[string]any)
		responses["404"] = map[string]any{"description": "Exact source/load or target is absent or not visible to this principal."}
		responses["409"] = map[string]any{"description": "Task, terminal load, source or preview changed; a new share requires a new preview. Original operations recover the same message."}
		for _, raw := range responses {
			raw.(map[string]any)["headers"] = map[string]any{"Cache-Control": map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}}, "Referrer-Policy": map[string]any{"schema": map[string]any{"type": "string", "const": "no-referrer"}}}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"CloudHistorySharePreviewRequest", "CloudHistoryShareRequest", "CloudHistorySharePreview", "CloudHistorySharedEntry", "CloudHistorySharedObservation"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
		for key, raw := range toolContractMap(schemas, name, "properties") {
			p := raw.(map[string]any)
			if name != "CloudHistorySharePreviewRequest" && name != "CloudHistoryShareRequest" {
				p["readOnly"] = true
			}
			switch key {
			case "project_id", "task_id", "load_id", "source_submission_id":
				p["minLength"], p["maxLength"] = 1, 64
			case "source_run_id":
				p["minLength"], p["maxLength"] = 1, 100
			case "source_node_id":
				p["minLength"], p["maxLength"] = 1, 150
			case "task_title":
				p["maxLength"], p["x-max-utf8-bytes"] = 500, 500
			case "expected_task_version", "task_version":
				p["minimum"], p["maximum"] = 1, 2147483646
			case "expected_load_version", "load_version":
				p["minimum"], p["maximum"] = 1, wc.MaxVersion
			case "body_sha256", "selection_sha256", "preview_sha256", "text_sha256":
				p["pattern"] = "^[0-9a-f]{64}$"
			case "sequences":
				p["minItems"], p["maxItems"], p["uniqueItems"] = 1, 8, true
				p["items"] = map[string]any{"type": "integer", "minimum": 1, "maximum": wc.CloudHistoryMaxEntries}
				p["description"] = "Strictly increasing local sequences in the exact terminal load."
			case "entries":
				p["minItems"], p["maxItems"] = 1, 8
			case "coverage":
				p["const"] = "unverified"
			case "sharing_scope":
				p["const"] = "project_task_readers"
			case "load_state":
				p["enum"] = []string{"loaded", "interrupted", "limited", "rejected"}
			case "sequence":
				p["minimum"], p["maximum"] = 1, wc.CloudHistoryMaxEntries
			case "kind":
				p["enum"] = []string{"user_text", "assistant_text", "tool_summary", "plan_summary"}
			case "body":
				p["minLength"], p["maxLength"], p["x-max-utf8-bytes"] = 1, 8000, 32000
			case "idempotency_key":
				p["minLength"], p["maxLength"], p["x-max-utf8-bytes"] = 8, 128, 128
			}
		}
	}
	toolContractMap(schemas, "TaskConversationMessage", "properties", "cloud_observation")["description"] = "Explicitly shared immutable copy under target-task read authority. Source IDs do not grant owner-private load access; message run_id remains target task context."
}
