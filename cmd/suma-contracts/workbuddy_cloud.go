package main

import (
	"strings"

	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func workBuddyCloudModels() []any {
	return []any{wc.NodeBinding{}, wc.CloudNodeView{}, wc.CloudTaskDeliveryLink{}, wc.CloudNodeProgress{}, wc.CloudNodeProgressPage{}, wc.CloudProgressTaskLink{}, wc.CloudDelegateRequest{}, wc.CloudCreateRequest{}, wc.CloudRefreshRequest{}, wc.CloudRevokeRequest{}, wc.CloudObserveRequest{}, wc.CloudLookupRequest{}, wc.CloudLookupResult{}, wc.CloudAssignmentBinding{}, wc.CloudWaitEvidence{}, wc.CloudDelegation{}, wc.CloudSubmission{}, workbuddy.Task{}, workbuddy.CreateRequest{}}
}

func workBuddyCloudContracts(add operationAdder, api map[string]any) {
	workBuddyCloudProgressContract(add, api)
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/workbuddy-cloud-nodes", "get", "", "CloudNodeView", "200"},
		{"/workbuddy-cloud-delegations", "post", "CloudDelegateRequest", "CloudDelegation", "201"},
		{"/workbuddy-cloud-delegations/{id}", "get", "", "CloudDelegation", "200"},
		{"/workbuddy-cloud-delegations/{id}/revoke", "post", "CloudRevokeRequest", "CloudDelegation", "200"},
		{"/workbuddy-cloud-submissions/{id}", "get", "", "CloudSubmission", "200"},
		{"/workbuddy-cloud-submissions/{id}/observe", "post", "CloudObserveRequest", "CloudSubmission", "200"},
		{"/runner/workbuddy-cloud-submissions", "post", "CloudCreateRequest", "CloudSubmission", "201"},
		{"/runner/workbuddy-cloud-submissions/lookup", "post", "CloudLookupRequest", "CloudLookupResult", "200"},
		{"/runner/workbuddy-cloud-submissions/{id}/refresh", "post", "CloudRefreshRequest", "CloudSubmission", "200"},
		{"/pool-runner/workbuddy-cloud-submissions", "post", "CloudCreateRequest", "CloudSubmission", "201"},
		{"/pool-runner/workbuddy-cloud-submissions/lookup", "post", "CloudLookupRequest", "CloudLookupResult", "200"},
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
		if strings.HasSuffix(route.path, "/lookup") {
			op["x-read-only"] = true
			op["x-no-external-effects"] = true
			op["description"] = "Read only the original control-plane submission under the current exact machine, project, delegation and assignment authority. No supplier request, Create, operation key or CAS mutation. not_found is the absence of a record at this authorized observation, not permission to Create. Prepared and unknown remain unresolved; no OAuth or ACP material is returned."
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
	models := []string{"NodeBinding", "CloudNodeView", "CloudTaskDeliveryLink", "CloudDelegateRequest", "CloudCreateRequest", "CloudRefreshRequest", "CloudRevokeRequest", "CloudObserveRequest", "CloudLookupRequest", "CloudLookupResult", "CloudAssignmentBinding", "CloudWaitEvidence", "CloudDelegation", "CloudSubmission", "Task", "CreateRequest"}
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
			if name == "CloudNodeView" || name == "CloudTaskDeliveryLink" || name == "CloudLookupResult" || name == "CloudDelegation" || name == "CloudSubmission" || name == "Task" {
				p["readOnly"] = true
			}
			switch key {
			case "version", "run_version", "task_version", "connection_version_at_issue", "grant_generation":
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
	field("CloudLookupResult", "state", map[string]any{"enum": []string{"not_found", "prepared", "unknown", "task_known"}, "description": "Original stored submission state. not_found is an authorized observation of absence, never a new Create permit or proof of no supplier effect."})
	field("CloudLookupResult", "submission", map[string]any{"description": "Required for prepared/unknown/task_known, absent for not_found. Must match this result's state, project, delegation and complete original assignment."})
	lookupConditions := []any{}
	for _, state := range []string{"not_found", "prepared", "unknown", "task_known"} {
		then := map[string]any{"not": map[string]any{"required": []string{"submission"}}}
		if state != "not_found" {
			then = map[string]any{"required": []string{"submission"}, "properties": map[string]any{"submission": map[string]any{"type": "object", "properties": map[string]any{"state": map[string]any{"const": state}}}}}
		}
		lookupConditions = append(lookupConditions, map[string]any{"if": map[string]any{"properties": map[string]any{"state": map[string]any{"const": state}}}, "then": then})
	}
	toolContractMap(schemas, "CloudLookupResult")["allOf"] = lookupConditions
	field("CloudSubmission", "usage_state", map[string]any{"const": "unreported", "description": "No trustworthy supplier usage/bill; absent counters and costs are not zero."})
	field("CloudSubmission", "last_error", map[string]any{"pattern": "^$|^[a-z][a-z0-9_]{0,79}$"})
	field("CloudSubmission", "observed_at", map[string]any{"description": "UTC time of the last persisted exact supplier Task observation; empty until task_known."})
	field("CloudNodeView", "task_delivery", map[string]any{"description": "Current locator for the project task that formally dispatched this exact WorkBuddy node. Absent when there is no matching started plan or visible task. This locator does not establish task acceptance or grant command authority."})
	field("CloudTaskDeliveryLink", "task_id", map[string]any{"pattern": "^[A-Za-z0-9_.-]{1,64}$", "not": map[string]any{"enum": []string{".", ".."}}})
	field("CloudTaskDeliveryLink", "run_relation", map[string]any{"enum": []string{"current", "changed", "unbound"}, "description": "Whether the original task currently references this run, a different run, or no run. A different run's identity is not returned."})
	field("CloudNodeView", "allowed_actions", map[string]any{"maxItems": 3, "uniqueItems": true, "items": map[string]any{"type": "string", "enum": []string{"delegate", "revoke", "observe"}}, "description": "Current-authorized subset in this order. Revoke requires own active delegation; observe requires own known task and current original grant. No machine execution action."})
	toolContractMap(schemas, "CloudSubmission")["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"state": map[string]any{"const": "task_known"}}}, "then": map[string]any{"required": []string{"task"}, "properties": map[string]any{"task": map[string]any{"$ref": "#/components/schemas/Task"}, "observed_at": map[string]any{"type": "string", "format": "date-time", "minLength": 1}}}, "else": map[string]any{"properties": map[string]any{"task": map[string]any{"type": "null"}, "observed_at": map[string]any{"const": ""}}}},
	}
	for _, name := range []string{"CloudCreateRequest", "CloudRefreshRequest", "CloudSubmission"} {
		field(name, "assignment", map[string]any{"description": "Exact current candidatecontract.AssignmentRef; complete pool scope when present. project/run/node must match the original delegation and current machine lease; IDs are not authority."})
	}
	for _, name := range []string{"CloudLookupRequest", "CloudLookupResult"} {
		field(name, "assignment", map[string]any{"description": "Original complete candidatecontract.AssignmentRef, preserving LeaseUntil, DispatchState and all pool fields. Current machine and grant authority and the same current attempt/fence/worker epoch must be verified. Reading stored facts does not require an active lease and does not authorize execution, a new supplier request or a new attempt."})
	}
	field("CloudAssignmentBinding", "delegation_version", map[string]any{"minimum": 1, "maximum": 2147483647, "description": "Original delegation version fixed in the claim transaction, not a request to adopt the latest grant."})
	field("CloudAssignmentBinding", "delegation_sha256", map[string]any{"pattern": "^[0-9a-f]{64}$", "description": "Digest of immutable authorization identity. Mutable consumption/display fields are excluded; the digest does not establish current authority."})
	field("CloudWaitEvidence", "protocol", map[string]any{"const": wc.Protocol})
	field("CloudWaitEvidence", "claim_id", map[string]any{"minLength": 1, "maxLength": 100})
	field("CloudWaitEvidence", "create_operation_id", map[string]any{"minLength": 8, "maxLength": 128, "x-max-utf8-bytes": 128, "description": "Original stable control-plane Create operation; a wait does not authorize another supplier POST."})
	field("CloudWaitEvidence", "assignment", map[string]any{"description": "Original complete AssignmentRef for the exact claim/attempt/fence/worker and pool identity. The trusted control-plane resolver checks this proof against the original claim and persisted submission."})
	field("CloudWaitEvidence", "binding", map[string]any{"description": "Original claim-selected binding; a machine cannot select a different delegation through a wait report."})
	field("CloudWaitEvidence", "submission_id", map[string]any{"maxLength": 64})
	field("CloudWaitEvidence", "submission_version", map[string]any{"minimum": 0, "maximum": 2147483647})
	field("CloudWaitEvidence", "cause", map[string]any{"enum": []string{"creation_unconfirmed", "remote_unknown", "waiting_input", "delivery_required", "interrupted", "authorization_changed", "observation_unconfirmed"}, "description": "Request for durable reconciliation, not successful completion, human acceptance or confirmed remote stop."})
	toolContractMap(schemas, "CloudWaitEvidence")["allOf"] = []any{
		map[string]any{"if": map[string]any{"required": []string{"submission_id"}, "properties": map[string]any{"submission_id": map[string]any{"minLength": 1}}}, "then": map[string]any{"required": []string{"submission_version"}, "properties": map[string]any{"submission_version": map[string]any{"minimum": 1}}}, "else": map[string]any{"properties": map[string]any{"submission_version": map[string]any{"const": 0}}}},
	}
	// These fields are reflected from dag.Assignment and dag.Result registered by
	// generate(). Preserve their actual optional typed references and other modes.
	field("Assignment", "workbuddy_cloud", map[string]any{"readOnly": true, "description": "Claim-transaction-selected and persisted WorkBuddy delegation binding. Never inferred from workflow config or replaced by the Runner; current authority is rechecked before external actions."})
	field("Result", "workbuddy_cloud_wait", map[string]any{"description": "Typed request to reconcile the original cloud effect through a trusted control-plane resolver. Not a success/acceptance or remote stop receipt; raw supplier status and this machine proof alone cannot complete the DAG or authorize another Create."})
}

func workBuddyCloudProgressContract(add operationAdder, api map[string]any) {
	pageOp := add("/workbuddy-cloud-progress", "get", "CloudNodeProgressPage", false, false)
	successStatus(pageOp, "200")
	pageOp["x-read-only"] = true
	pageOp["x-no-external-effects"] = true
	pageOp["x-disallowed-token-scopes"] = []string{"agent", "runner"}
	pageOp["description"] = "List saved WorkBuddy submissions in the current project. Each item uses the exact node progress projection. Reads do not contact the provider. Before is the opaque cursor for this reader, project and filter."
	appendQuery(pageOp, queryParameter("limit", "Page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}))
	appendQuery(pageOp, queryParameter("before", "Opaque continuation from next_cursor.", map[string]any{"type": "string", "minLength": 1, "maxLength": 1024}))
	appendQuery(pageOp, queryParameter("filter", "Filter all saved project submissions for attention. A bounded scan may return a short or empty page with a continuation.", map[string]any{"type": "string", "enum": []string{wc.CloudProgressAttentionFilter}}))
	for _, raw := range pageOp["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "project_id" {
			p["required"] = true
			p["schema"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
		}
	}
	pageResponses := toolContractMap(pageOp, "responses")
	pageResponses["405"] = map[string]any{"description": "Only GET is supported."}
	pageResponses["503"] = map[string]any{"description": "The WorkBuddy component is unavailable."}
	for _, raw := range pageResponses {
		raw.(map[string]any)["headers"] = map[string]any{
			"Cache-Control":   map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}},
			"Referrer-Policy": map[string]any{"schema": map[string]any{"type": "string", "const": "no-referrer"}},
		}
	}
	toolContractMap(pageResponses, "405", "headers")["Allow"] = map[string]any{"schema": map[string]any{"type": "string", "const": "GET"}}
	page := toolContractMap(api, "components", "schemas", "CloudNodeProgressPage")
	page["additionalProperties"] = false
	pageFields := toolContractMap(page, "properties")
	for _, field := range pageFields {
		field.(map[string]any)["readOnly"] = true
	}
	toolContractMap(pageFields, "project_id")["minLength"], toolContractMap(pageFields, "project_id")["maxLength"] = 1, 64
	toolContractMap(pageFields, "items")["maxItems"] = 20
	toolContractMap(pageFields, "next_cursor")["minLength"], toolContractMap(pageFields, "next_cursor")["maxLength"] = 1, 1024
	toolContractMap(pageFields, "filter")["enum"] = []string{wc.CloudProgressAttentionFilter}
	toolContractMap(pageFields, "next_cursor")["description"] = "Continue the same project, reader and filter. A filtered cursor advances from the last inspected submission, even when no item matches."
	links := toolContractMap(pageFields, "task_links")
	links["maxItems"] = 20
	links["description"] = "Formal task context keyed by exact run and node among the returned items. Current task binding is independent of the saved execution."
	link := toolContractMap(api, "components", "schemas", "CloudProgressTaskLink")
	link["additionalProperties"] = false
	linkFields := toolContractMap(link, "properties")
	for _, field := range linkFields {
		field.(map[string]any)["readOnly"] = true
	}
	for name, max := range map[string]int{"run_id": 100, "node_id": 150, "task_id": 64} {
		toolContractMap(linkFields, name)["minLength"], toolContractMap(linkFields, name)["maxLength"] = 1, max
	}
	toolContractMap(linkFields, "task_version")["minimum"], toolContractMap(linkFields, "task_version")["maximum"] = 1, wc.MaxVersion
	toolContractMap(linkFields, "run_relation")["enum"] = []string{"current", "changed", "unbound"}
	toolContractMap(linkFields, "task_title")["minLength"] = 1
	toolContractMap(linkFields, "task_title")["maxLength"] = 500
	toolContractMap(linkFields, "task_title")["x-max-utf8-bytes"] = 500

	op := add("/runs/{id}/nodes/{node}/workbuddy-status", "get", "CloudNodeProgress", false, false)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["x-no-external-effects"] = true
	op["x-disallowed-token-scopes"] = []string{"agent", "runner"}
	op["description"] = "Read persisted progress for one exact WorkBuddy node under current browser project membership or a current project API credential. No provider requests, connection access, or mutation. An absent local submission record and an unknown remote effect are distinct. Provider completion does not establish platform delivery."
	limits := map[string]int{"project_id": 64, "id": 100, "node": 150}
	for _, raw := range op["parameters"].([]any) {
		p := raw.(map[string]any)
		if max, ok := limits[p["name"].(string)]; ok {
			p["required"] = true
			p["schema"] = map[string]any{"type": "string", "minLength": 1, "maxLength": max}
		}
	}
	responses := op["responses"].(map[string]any)
	responses["400"] = map[string]any{"description": "Invalid exact scope or the original node is not a valid WorkBuddy node."}
	responses["404"] = map[string]any{"description": "Exact run or node is absent."}
	responses["405"] = map[string]any{"description": "Only GET is supported."}
	responses["503"] = map[string]any{"description": "The WorkBuddy component is unavailable."}
	for _, raw := range responses {
		raw.(map[string]any)["headers"] = map[string]any{
			"Cache-Control":   map[string]any{"schema": map[string]any{"type": "string", "const": "no-store"}},
			"Referrer-Policy": map[string]any{"schema": map[string]any{"type": "string", "const": "no-referrer"}},
		}
	}
	toolContractMap(responses, "405", "headers")["Allow"] = map[string]any{"schema": map[string]any{"type": "string", "const": "GET"}}

	schema := toolContractMap(api, "components", "schemas", "CloudNodeProgress")
	schema["additionalProperties"] = false
	properties := toolContractMap(schema, "properties")
	for _, raw := range properties {
		raw.(map[string]any)["readOnly"] = true
	}
	for name, max := range map[string]int{"project_id": 64, "run_id": 100, "node_id": 150} {
		p := toolContractMap(properties, name)
		p["minLength"], p["maxLength"] = 1, max
	}
	for name, min := range map[string]int{"run_version": 1, "attempt": 0} {
		p := toolContractMap(properties, name)
		p["minimum"], p["maximum"] = min, wc.MaxVersion
	}
	for _, name := range []string{"run_state", "node_state"} {
		toolContractMap(properties, name)["pattern"] = "^[a-z][a-z0-9_]{0,79}$"
	}
	toolContractMap(properties, "node_reason")["description"] = "Allowlisted platform reason for this WorkBuddy node, if available."
	toolContractMap(properties, "submission_state")["enum"] = []string{"not_recorded", "prepared", "unknown", "task_known"}
	toolContractMap(properties, "provider_status")["enum"] = []string{"pending", "queued", "running", "working", "waiting_input", "completed", "failed", "cancelled", "canceled", "unknown"}
	toolContractMap(properties, "observed_at")["format"] = "date-time"
	schema["if"] = map[string]any{"properties": map[string]any{"submission_state": map[string]any{"const": "task_known"}}}
	schema["then"] = map[string]any{"required": []string{"provider_status", "observed_at"}}
	schema["else"] = map[string]any{
		"properties": map[string]any{"observation_failed": map[string]any{"const": false}},
		"not":        map[string]any{"anyOf": []any{map[string]any{"required": []string{"provider_status"}}, map[string]any{"required": []string{"observed_at"}}}},
	}
}
