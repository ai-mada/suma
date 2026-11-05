package main

import (
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
)

func executionPoolModels() []any {
	return []any{
		platform.ExecutionPool{}, platform.ExecutionPoolPage{}, platform.ExecutionPoolCreateRequest{}, platform.ExecutionPoolCommandRequest{},
		platform.ExecutionPoolGrant{}, platform.ExecutionPoolGrantPage{}, platform.ExecutionPoolGrantRequest{}, platform.ExecutionPoolGrantCommand{},
		platform.ExecutionPoolRunner{}, platform.ExecutionPoolRunnerSlotOccupancy{}, platform.ExecutionPoolRunnerPage{}, platform.ExecutionPoolRunnerCreateRequest{}, platform.ExecutionPoolCredentialRequest{}, platform.ExecutionPoolCredentialResponse{}, platform.ExecutionPoolRunnerSession{},
		platform.ExecutionPoolPolicyPreview{}, platform.ExecutionPoolPolicyPreviewRequest{}, platform.ExecutionPreviewCommitRequest{}, platform.ExecutionRouteView{}, platform.ExecutionRoutePreview{}, platform.ExecutionRoutePreviewRequest{},
		dag.PoolClaimRequest{}, platform.PoolRunnerAssignmentRequest{}, platform.PoolRunnerProgressRequest{}, platform.PoolRunnerResultRequest{}, platform.PoolToolInventoryRequest{}, platform.PoolRunnerClaimResponse{},
		platform.ProjectPoolRunnerPage{},
	}
}

func enhanceExecutionPoolSchemas(schemas map[string]any) {
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for key, val := range attrs {
			value[key] = val
		}
	}
	for _, model := range []string{"ExecutionPoolCreateRequest", "ExecutionPoolCommandRequest", "ExecutionPoolGrantRequest", "ExecutionPoolGrantCommand", "ExecutionPoolRunnerCreateRequest", "ExecutionPoolCredentialRequest", "ExecutionPoolPolicyInput", "ExecutionPoolPolicyPreviewRequest", "ExecutionPreviewCommitRequest", "ExecutionRoutePreviewRequest", "ExecutionRouteInput", "PoolClaimRequest", "PoolCapabilityScope", "PoolRunnerAssignmentRequest", "PoolRunnerProgressRequest", "PoolRunnerResultRequest", "PoolToolInventoryRequest"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"ExecutionPoolCreateRequest", "ExecutionPoolCommandRequest", "ExecutionPoolGrantRequest", "ExecutionPoolGrantCommand", "ExecutionPoolRunnerCreateRequest", "ExecutionPoolCredentialRequest"} {
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, model := range []string{"ExecutionPoolPolicy", "ExecutionPoolPolicyInput"} {
		field(model, "concurrency_limit", map[string]any{"minimum": 0, "maximum": int64(9007199254740991)})
		field(model, "tenant_weights", map[string]any{"maxProperties": 256, "propertyNames": map[string]any{"pattern": "^[a-f0-9]{32}$"}, "additionalProperties": map[string]any{"type": "integer", "minimum": 1, "maximum": 100}})
		capacity := map[string]any{"anyOf": []any{map[string]any{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991)}, map[string]any{"type": "null"}}}
		field(model, "resource_limits", map[string]any{"maxProperties": 64, "additionalProperties": capacity})
		field(model, "budget_limit", capacity)
	}
	for _, model := range []string{"ExecutionPoolPolicyPreviewRequest", "ExecutionPreviewCommitRequest", "ExecutionRoutePreviewRequest"} {
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	field("ExecutionRoutePreviewRequest", "expected_version", map[string]any{"minimum": 0})
	for _, model := range []string{"ExecutionPoolPolicyPreviewRequest", "ExecutionPreviewCommitRequest", "ExecutionPoolCommandRequest", "ExecutionPoolCredentialRequest", "ExecutionPoolGrantCommand"} {
		field(model, "expected_version", map[string]any{"minimum": 1})
	}
	field("ExecutionRouteInput", "mode", map[string]any{"enum": []string{"local", "pool"}})
	field("ExecutionPoolCommandRequest", "action", map[string]any{"enum": []string{"pause", "resume", "revoke"}})
	field("ExecutionPoolGrantCommand", "action", map[string]any{"enum": []string{"approve", "reject", "revoke", "withdraw"}})
	field("PoolClaimRequest", "slot_id", map[string]any{"pattern": "^(0|[1-9][0-9]{0,2})$", "description": "Zero-based decimal slot, less than this registered runner's max_slots (server-enforced, maximum128)."})
	field("PoolClaimRequest", "scopes", map[string]any{"minItems": 1, "maxItems": 256})
	field("ExecutionPoolGrantRequest", "project_weight", map[string]any{"minimum": 1, "maximum": 100})
	field("ExecutionPoolRunnerCreateRequest", "max_slots", map[string]any{"minimum": 1, "maximum": 128})
	for _, model := range []string{"ExecutionPoolRunnerCreateRequest", "ExecutionPoolCredentialRequest"} {
		field(model, "ttl_seconds", map[string]any{"minimum": 3600, "maximum": 2592000})
	}
	field("ExecutionPoolRunner", "slot_occupancy", map[string]any{"readOnly": true, "description": "Included only in the owner/admin pool runner list. Counts are scheduler reservations, independent of connectivity and machine capacity."})
	toolContractMap(schemas, "ExecutionPoolRunnerSlotOccupancy")["additionalProperties"] = false
	field("ExecutionPoolRunnerSlotOccupancy", "observed_at", map[string]any{"format": "date-time", "readOnly": true})
	for _, name := range []string{"held", "running", "reconciling", "unknown"} {
		field("ExecutionPoolRunnerSlotOccupancy", name, map[string]any{"minimum": 0, "readOnly": true})
	}
	field("ExecutionPoolCredentialResponse", "token", map[string]any{"readOnly": true, "x-sensitive": true, "description": "Returned once after a committed registration/rotation. Never stored in operation receipts; replay returns metadata only."})
}

// Pool governance has no implicit selected-project authority. Project routes
// carry an exact project while the machine protocol has a separate identity.
func executionPoolContracts(add operationAdder) {
	for _, route := range []struct {
		path, method, model, input string
		project, paged             bool
	}{
		{"/execution-pools", "get", "ExecutionPoolPage", "", false, true},
		{"/execution-pools", "post", "ExecutionPool", "ExecutionPoolCreateRequest", false, false},
		{"/execution-pools/{pool}", "get", "ExecutionPool", "", false, false},
		{"/execution-pools/{pool}/commands", "post", "ExecutionPool", "ExecutionPoolCommandRequest", false, false},
		{"/execution-pools/{pool}/grants", "get", "ExecutionPoolGrantPage", "", false, true},
		{"/execution-pools/{pool}/grants/{grant}/commands", "post", "ExecutionPoolGrant", "ExecutionPoolGrantCommand", false, false},
		{"/execution-pools/{pool}/runners", "get", "ExecutionPoolRunnerPage", "", false, true},
		{"/execution-pools/{pool}/runners/{runner}", "get", "ExecutionPoolRunner", "", false, false},
		{"/execution-pools/{pool}/runners", "post", "ExecutionPoolCredentialResponse", "ExecutionPoolRunnerCreateRequest", false, false},
		{"/execution-pools/{pool}/runners/{runner}/credentials", "post", "ExecutionPoolCredentialResponse", "ExecutionPoolCredentialRequest", false, false},
		{"/execution-pools/{pool}/runners/{runner}/commands", "post", "ExecutionPoolRunner", "ExecutionPoolCommandRequest", false, false},
		{"/execution-pools/{pool}/policy-previews", "post", "ExecutionPoolPolicyPreview", "ExecutionPoolPolicyPreviewRequest", false, false},
		{"/execution-pools/{pool}/policy-previews/{preview}", "get", "ExecutionPoolPolicyPreview", "", false, false},
		{"/execution-pools/{pool}/policy-previews/{preview}/commit", "post", "ExecutionPool", "ExecutionPreviewCommitRequest", false, false},
		{"/project-execution/grants", "get", "ExecutionPoolGrantPage", "", true, true},
		{"/project-execution/grants", "post", "ExecutionPoolGrant", "ExecutionPoolGrantRequest", true, false},
		{"/project-execution/grants/{grant}/commands", "post", "ExecutionPoolGrant", "ExecutionPoolGrantCommand", true, false},
		{"/project-execution/route", "get", "ExecutionRouteView", "", true, false},
		{"/project-execution/runners", "get", "ProjectPoolRunnerPage", "", true, false},
		{"/project-execution/route-previews", "post", "ExecutionRoutePreview", "ExecutionRoutePreviewRequest", true, false},
		{"/project-execution/route-previews/{preview}", "get", "ExecutionRoutePreview", "", true, false},
		{"/project-execution/route-previews/{preview}/commit", "post", "ExecutionRouteView", "ExecutionPreviewCommitRequest", true, false},
	} {
		op := add(route.path, route.method, route.model, false, route.input != "")
		browserOnly(op)
		op["x-authorization-scope"] = "current organization owner/admin; exact browser credential"
		if route.project {
			op["x-authorization-scope"] = "exact project; read or current project admin for mutations"
		}
		if route.path == "/execution-pools/{pool}/runners" && route.method == "get" {
			op["description"] = "Read the current organization-owned pool's paged Runner registrations with aggregate held slot counts from the control plane. Running, reconciling and unknown refer to recorded attempt states; no project, run or slot identifiers are exposed."
			op["x-read-only"], op["x-response-cache-control"] = true, "no-store"
		}
		if route.path == "/execution-pools/{pool}/runners/{runner}" {
			op["description"] = "Read one exact registered Runner in its current organization-owned pool, independently of list pagination. Current browser owner/admin authority is checked before pool ownership and the closed request. No query parameters or project context are accepted. Registration and historical last contact do not prove tool readiness; credentials and consuming-project content are never returned."
			op["x-read-only"], op["x-response-cache-control"] = true, "no-store"
			op["x-query-parameters-forbidden"] = true
		}
		if route.input != "" {
			request(op, route.input)
		}
		status := "200"
		if route.method == "post" && (route.path == "/execution-pools" || route.path == "/project-execution/grants" || route.model == "ExecutionPoolCredentialResponse" || route.model == "ExecutionPoolPolicyPreview" || route.model == "ExecutionRoutePreview") {
			status = "201"
		}
		successStatus(op, status)
		if route.paged {
			appendQuery(op, queryParameter("limit", "Default50, maximum100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}))
			appendQuery(op, queryParameter("cursor", "Opaque keyset cursor scoped to current tenant and collection.", map[string]any{"type": "string"}))
		}
	}
	for _, route := range []struct{ path, method, model, input string }{
		{"/pool-runner/session", "get", "ExecutionPoolRunnerSession", ""},
		{"/pool-runner/claim", "post", "PoolRunnerClaimResponse", "PoolClaimRequest"},
		{"/pool-runner/heartbeat", "post", "", "PoolRunnerAssignmentRequest"},
		{"/pool-runner/progress", "post", "", "PoolRunnerProgressRequest"},
		{"/pool-runner/results", "post", "", "PoolRunnerResultRequest"},
		{"/pool-runner/tool-inventory", "post", "ToolInventoryReceipt", "PoolToolInventoryRequest"},
	} {
		op := add(route.path, route.method, route.model, false, route.input != "")
		op["security"] = []any{map[string]any{"poolRunnerAuth": []string{}}}
		op["x-authorization-scope"] = "independent pool machine identity; exact current credential and bilateral project grants; never a synthesized human principal"
		if route.input != "" {
			request(op, route.input)
		} else {
			appendQuery(op, queryParameter("runner_id", "Exact registered pool Runner identity.", map[string]any{"type": "string"}))
		}
		successStatus(op, "200")
	}
}
