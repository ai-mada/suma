package main

import (
	"strings"

	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func workBuddyCloudModels() []any {
	return []any{wc.NodeBinding{}, wc.CloudNodeView{}, wc.CloudDelegateRequest{}, wc.CloudCreateRequest{}, wc.CloudRefreshRequest{}, wc.CloudRevokeRequest{}, wc.CloudObserveRequest{}, wc.CloudDelegation{}, wc.CloudSubmission{}, workbuddy.Task{}, workbuddy.CreateRequest{}}
}

func workBuddyCloudContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/workbuddy-cloud-nodes", "get", "", "CloudNodeView", "200"},
		{"/workbuddy-cloud-delegations", "post", "CloudDelegateRequest", "CloudDelegation", "201"},
		{"/workbuddy-cloud-delegations/{id}", "get", "", "CloudDelegation", "200"},
		{"/workbuddy-cloud-delegations/{id}/revoke", "post", "CloudRevokeRequest", "CloudDelegation", "200"},
		{"/workbuddy-cloud-submissions/{id}", "get", "", "CloudSubmission", "200"},
		{"/workbuddy-cloud-submissions/{id}/observe", "post", "CloudObserveRequest", "CloudSubmission", "200"},
		{"/runner/workbuddy-cloud-submissions", "post", "CloudCreateRequest", "CloudSubmission", "201"},
		{"/runner/workbuddy-cloud-submissions/{id}/refresh", "post", "CloudRefreshRequest", "CloudSubmission", "200"},
		{"/pool-runner/workbuddy-cloud-submissions", "post", "CloudCreateRequest", "CloudSubmission", "201"},
		{"/pool-runner/workbuddy-cloud-submissions/{id}/refresh", "post", "CloudRefreshRequest", "CloudSubmission", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		machine := strings.HasPrefix(route.path, "/runner/") || strings.HasPrefix(route.path, "/pool-runner/")
		if machine {
			scheme := "runnerAuth"
			if strings.HasPrefix(route.path, "/pool-runner/") {
				scheme = "bearerAuth"
			}
			op["security"] = []any{map[string]any{scheme: []string{}}}
			op["x-disallowed-token-scopes"] = []string{"browser", "api", "agent"}
			op["description"] = "Exact current machine assignment, owner grant, run/node/attempt/fence/lease and project authority. Pool credentials retain their full current bilateral grant and credential versions. Current authority precedes receipt recovery. One durable control-plane prepare permits one Create only; prepared/unknown never authorizes another POST, including another attempt. Refresh only reads the original supplier task. No OAuth or ACP material is returned."
		} else {
			browserOnly(op)
			op["description"] = "Current browser project authority. Delegations are private to their original owner; safe submission facts are project-readable. Original node source is fixed. Explicit owner observe reads only the original task using the current original grant; it does not restore a machine lease, delegation, or execution permission. Local GETs never call the supplier."
		}
		parameters := []any{}
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			if route.method == "post" && p["in"] == "query" {
				continue
			}
			if p["name"] == "project_id" || p["in"] == "path" {
				p["schema"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
				p["required"] = true
			}
			parameters = append(parameters, p)
		}
		op["parameters"] = parameters
		if route.input != "" {
			request(op, route.input)
			op["x-max-request-body-bytes"] = 64 << 10
			op["x-query-parameters-forbidden"] = true
		} else {
			op["x-read-only"] = true
			op["x-no-external-effects"] = true
		}
		if route.path == "/workbuddy-cloud-delegations" || strings.HasSuffix(route.path, "/revoke") {
			op["x-no-external-effects"] = true
		}
		if route.path == "/workbuddy-cloud-nodes" {
			for _, item := range []struct {
				name string
				max  int
			}{{"run_id", 100}, {"node_id", 150}} {
				p := queryParameter(item.name, "Exact original node scope; no list or inferred selection.", map[string]any{"type": "string", "minLength": 1, "maxLength": item.max})
				p["required"] = true
				appendQuery(op, p)
			}
		}
		responses := op["responses"].(map[string]any)
		responses["404"] = map[string]any{"description": "Object is absent or not currently visible to this identity."}
		responses["409"] = map[string]any{"description": "Exact version/source changed, consumed submission or uncertain external effect requires reconciliation. Never retry supplier Create."}
		responses["503"] = map[string]any{"description": "Cloud connection component is unavailable; configuration is not execution health."}
		if strings.HasSuffix(route.path, "/observe") || strings.HasSuffix(route.path, "/refresh") {
			responses["502"] = map[string]any{"description": "Exact supplier GET failed. Original task identity and last verified observation remain stored."}
		}
		for _, raw := range responses {
			raw.(map[string]any)["headers"] = map[string]any{"Cache-Control": map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}}, "Referrer-Policy": map[string]any{"schema": map[string]any{"type": "string", "const": "no-referrer"}}}
		}
	}
	models := []string{"NodeBinding", "CloudNodeView", "CloudDelegateRequest", "CloudCreateRequest", "CloudRefreshRequest", "CloudRevokeRequest", "CloudObserveRequest", "CloudDelegation", "CloudSubmission", "Task", "CreateRequest"}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, key string, attrs map[string]any) {
		for k, v := range attrs {
			toolContractMap(schemas, model, "properties", key)[k] = v
		}
	}
	for _, name := range models {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for key, raw := range toolContractMap(model, "properties") {
			p := raw.(map[string]any)
			if name == "CloudNodeView" || name == "CloudDelegation" || name == "CloudSubmission" || name == "Task" {
				p["readOnly"] = true
			}
			switch key {
			case "version", "run_version", "connection_version_at_issue", "grant_generation":
				p["minimum"], p["maximum"] = 1, 2147483647
			case "expected_version", "expected_run_version", "expected_connection_version":
				p["minimum"], p["maximum"] = 1, 2147483646
			case "idempotency_key":
				p["minLength"], p["maxLength"], p["x-max-utf8-bytes"] = 8, 128, 128
			case "node_spec_sha256", "request_sha256", "client_binding_sha256":
				p["pattern"] = "^[0-9a-f]{64}$"
			case "project_id", "connection_id", "delegation_id", "owner_id", "id":
				p["minLength"], p["maxLength"] = 1, 64
			case "run_id":
				p["minLength"], p["maxLength"] = 1, 100
			case "node_id":
				p["minLength"], p["maxLength"] = 1, 150
			case "created_at", "updated_at", "expires_at":
				p["format"] = "date-time"
			}
		}
	}
	field("CreateRequest", "prompt", map[string]any{"minLength": 1, "maxLength": 256 << 10, "x-max-utf8-bytes": 256 << 10, "description": "Original immutable node prompt, not accepted by machine Create or human delegation commands."})
	field("CreateRequest", "name", map[string]any{"maxLength": 512})
	field("Task", "task_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,200}$"})
	field("Task", "status", map[string]any{"minLength": 1, "maxLength": 128, "description": "Raw supplier status; unknown values are retained and do not establish Suma completion."})
	field("Task", "name", map[string]any{"maxLength": 512})
	for _, name := range []string{"CloudDelegateRequest", "CloudDelegation"} {
		field(name, "max_creates", map[string]any{"const": 1})
		field(name, "expires_at", map[string]any{"description": "Explicit UTC expiry. New delegations require expiry after authoritative database time and at most 24 hours later; reads preserve expired records."})
	}
	field("CloudDelegation", "creates_consumed", map[string]any{"minimum": 0, "maximum": 1})
	field("CloudDelegation", "state", map[string]any{"enum": []string{"active", "revoked"}})
	field("CloudSubmission", "state", map[string]any{"enum": []string{"prepared", "unknown", "task_known"}})
	field("CloudSubmission", "usage_state", map[string]any{"const": "unreported", "description": "No trustworthy supplier usage/bill; absent counters and costs are not zero."})
	field("CloudSubmission", "last_error", map[string]any{"pattern": "^$|^[a-z][a-z0-9_]{0,79}$"})
	field("CloudSubmission", "observed_at", map[string]any{"description": "UTC time of the last persisted exact supplier Task observation; empty until task_known."})
	field("CloudNodeView", "allowed_actions", map[string]any{"maxItems": 3, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": []string{"delegate", "revoke", "observe"}}, "description": "Current-authorized subset in this order. Revoke requires own active delegation; observe requires own known task and current original grant. No machine execution action."})
	toolContractMap(schemas, "CloudSubmission")["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"state": map[string]any{"const": "task_known"}}}, "then": map[string]any{"required": []string{"task"}, "properties": map[string]any{"task": map[string]any{"$ref": "#/components/schemas/Task"}, "observed_at": map[string]any{"type": "string", "format": "date-time", "minLength": 1}}}, "else": map[string]any{"properties": map[string]any{"task": map[string]any{"type": "null"}, "observed_at": map[string]any{"const": ""}}}},
	}
	for _, name := range []string{"CloudCreateRequest", "CloudRefreshRequest", "CloudSubmission"} {
		field(name, "assignment", map[string]any{"description": "Exact current candidatecontract.AssignmentRef; complete pool scope when present. project/run/node must match the original delegation and current machine lease; IDs are not authority."})
	}
}
