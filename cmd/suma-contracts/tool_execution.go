package main

import (
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func toolExecutionModels() []any {
	return []any{platform.ToolProfile{}, platform.ToolProfileCreateRequest{}, platform.ToolProfileCommandRequest{}, platform.ToolProfileList{}, platform.ToolProfileListQuery{}, platform.ToolProfileMatchRequest{}, platform.ToolProfileMatchList{}, platform.ToolProfileDiagnostics{}, platform.ToolProfileDiagnosticCheck{}, platform.ToolInventoryRequest{}, platform.ToolInventoryReceipt{}, platform.TaskContextRecord{}, platform.TaskContext{}, platform.TaskDispatchPlan{}, platform.TaskDispatchPreviewRequest{}, platform.TaskDispatchTestSelection{}, platform.TaskDispatchTestBinding{}, platform.TaskDispatchStartRequest{}, toolcontract.Profile{}, toolcontract.Observation{}, toolcontract.Event{}, toolcontract.Evidence{}, toolcontract.Usage{}}
}

func toolExecutionContracts(add operationAdder, paths map[string]any) {
	for _, route := range []struct{ path, model string }{{"/tool-profiles", "ToolProfileList"}, {"/tool-profiles/{id}", "ToolProfile"}} {
		op := add(route.path, "get", route.model, false, false)
		browserOnly(op)
		successStatus(op, "200")
		if route.path == "/tool-profiles" {
			appendQuery(op, queryParameter("limit", "Default50, maximum200.", map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 50}))
			appendQuery(op, queryParameter("cursor", "Opaque cursor scoped to tenant and project.", map[string]any{"type": "string"}))
		}
		op["description"] = "Immutable administrator configuration plus separately observed local availability. Freshness uses database time and expires after60 seconds. Neither installation nor available=true proves model authentication or successful task execution."
	}
	diagnostics := add("/tool-profiles/{id}/diagnostics", "get", "ToolProfileDiagnostics", false, false)
	browserOnly(diagnostics)
	successStatus(diagnostics, "200")
	diagnostics["x-read-only"] = true
	diagnostics["x-response-cache-control"] = "no-store"
	for _, header := range []string{"X-Suma-Expected-User", "X-Suma-Expected-Organization"} {
		diagnostics["parameters"] = append(diagnostics["parameters"].([]any), map[string]any{"in": "header", "name": header, "required": false, "description": "Optional current-view assertion; if supplied it must be a single valid ID matching the authenticated actor. It never selects an identity or grants authority.", "schema": map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$"}})
	}
	diagnostics["description"] = "Current four-prerequisite diagnosis from the original availability evaluator and one authorized database snapshot. Runner identity is not online/model health. Fixed checks authorization, worker, inventory, profile; stale inventory leaves profile not_checked. A display deadline of at most60 seconds is never an execution/enable/repair grant. Suggestions only navigate to existing explicit workflows."
	match := add("/tool-profiles/matches", "post", "ToolProfileMatchList", false, false)
	browserOnly(match)
	successStatus(match, "200")
	request(match, "ToolProfileMatchRequest")
	match["x-read-only"] = true
	match["x-response-cache-control"] = "no-store"
	match["description"] = "Complete non-paginated exact immutable profile matches for the current tenant/project/worker. Compares every capability field, not names or digests alone. Revoked entries are history; paused/enabled require explicit selection and original enable authority. A match never proves health, creates a receipt or authorizes dispatch."
	op := add("/tool-profiles", "post", "ToolProfile", false, true)
	browserOnly(op)
	successStatus(op, "201")
	request(op, "ToolProfileCreateRequest")
	op["description"] = "Registers paused metadata for an existing bound execution worker. Requires live project write and approval authority before original receipt recovery. In the same authority transaction, an existing paused/enabled exact profile rejects a new create with409 tool_profile_match_exists; only the original operation recovers its own201. Revoked history does not block explicit new registration. No executable, raw args, environment values or secrets are accepted in this public DTO."
	op = add("/tool-profiles/{id}/commands", "post", "ToolProfile", false, true)
	browserOnly(op)
	successStatus(op, "200")
	request(op, "ToolProfileCommandRequest")
	op["description"] = "CAS enable/pause/permanent revoke with current authority checked before receipt replay. Immutable configuration cannot be edited; changed configuration requires a new registration."
	op = add("/runner/tool-inventory", "post", "ToolInventoryReceipt", false, true)
	request(op, "ToolInventoryRequest")
	successStatus(op, "200")
	op["security"] = []any{map[string]any{"runnerAuth": []string{}}}
	op["x-required-token-scope"] = "runner"
	op["description"] = "Bound registered worker identity reports at most128 nonsecret local command observations. Exact operation replay recovers the original database timestamp and does not renew expiry. Reports cannot grant profile approval or deployment authority."
	for _, route := range []struct{ path, method, request, response string }{{"/task-dispatch-plans", "post", "TaskDispatchPreviewRequest", "TaskDispatchPlan"}, {"/task-dispatch-plans/{id}", "get", "", "TaskDispatchPlan"}, {"/task-dispatch-plans/{id}/start", "post", "TaskDispatchStartRequest", "Run"}} {
		op = add(route.path, route.method, route.response, false, route.request != "")
		if route.request != "" {
			request(op, route.request)
			successStatus(op, "201")
		} else {
			successStatus(op, "200")
		}
		op["description"] = "Current project authority required. Preview freezes actual task/requirement versions, full task context, selected tool hash and an exact workflow copy with independent candidate tests. Start atomically creates the reserved managed task run and task binding, revalidating current versions, authority, profile and observations. Preview expires after30 database minutes. Same start operation recovers the original run without another task mutation."
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
	}
	progress := toolContractMap(paths, "/runner/progress", "post")
	progress["description"] = "Durable current-attempt output or bounded typed tool fact. Current worker identity and scope are checked before both new events and receipt replay. Emits node_progress or tool_progress; external completion, tokens and waiting-input observations never imply DAG success, business acceptance or release approval."
	toolContractMap(paths, "/runner/claim", "post")["description"] = "Claims only nodes matching actual command:<id> capabilities and all explicit constraints. Tool-bound nodes additionally require the exact enabled profile, current enabling authority, bound worker and matching fresh inventory before any attempt/resource is consumed."
}

func enhanceToolExecutionSchemas(schemas map[string]any) {
	enhanceTaskDispatchTestBindingSchemas(schemas)
	toolProfileReplacementSchemas(schemas)
	toolDoctorMaintenanceSchemas(schemas)
	enhanceCodexSkillSchemas(schemas)
	nativeDiagnosticContracts(schemas)
	openCodeExecutionSchemas(schemas)
	octopExecutionSchemas(schemas)
	claudeExecutionSchemas(schemas)
	modelGatewaySchemas(schemas)
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for k, v := range attrs {
			value[k] = v
		}
	}
	for _, model := range []string{"ToolProfileCreateRequest", "ToolProfileCommandRequest", "ToolProfileMatchRequest", "ToolProfileMatchList", "ToolProfileDiagnostics", "ToolProfileDiagnosticCheck", "ToolInventoryRequest", "TaskDispatchPreviewRequest", "TaskDispatchStartRequest", "ToolEvent", "ToolUsage"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"ToolProfileCreateRequest", "ToolProfileCommandRequest", "TaskDispatchPreviewRequest", "TaskDispatchStartRequest"} {
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	field("ToolInventoryRequest", "operation_id", map[string]any{"minLength": 8, "maxLength": 128})
	field("ToolInventoryRequest", "items", map[string]any{"maxItems": 128})
	field("ToolProfileCommandRequest", "action", map[string]any{"enum": []string{"enable", "pause", "revoke"}})
	field("ToolProfileMatchList", "items", map[string]any{"maxItems": 1000})
	field("ToolProfileMatchList", "total", map[string]any{"minimum": 0, "maximum": 1000, "description": "Equals items.length; the exact complete match set has no next cursor or silent truncation."})
	field("ToolProfileDiagnostics", "checked_at", map[string]any{"format": "date-time"})
	field("ToolProfileDiagnostics", "expires_at", map[string]any{"format": "date-time", "description": "At most checked_at+60 seconds and no later than a fresh inventory's expiry; a display deadline, never a credential."})
	field("ToolProfileDiagnostics", "checks", map[string]any{"minItems": 4, "maxItems": 4, "x-ordered-codes": []string{"authorization", "worker", "inventory", "profile"}})
	field("ToolProfileDiagnosticCheck", "code", map[string]any{"enum": []string{"authorization", "worker", "inventory", "profile"}})
	field("ToolProfileDiagnosticCheck", "status", map[string]any{"enum": []string{"pass", "blocked", "not_checked"}})
	field("ToolProfileDiagnosticCheck", "reason", map[string]any{"maxLength": 4096, "x-max-utf8-bytes": 4096})
	for _, model := range []string{"ToolProfileDiagnostics", "ToolProfileDiagnosticCheck"} {
		field(model, "next_action", map[string]any{"enum": []string{"none", "review_profile", "open_worker", "register_profile", "refresh"}, "description": "Display/navigation suggestion; never permission or an automatic write."})
	}
	field("ToolProfile", "state", map[string]any{"enum": []string{"paused", "enabled", "revoked"}})
	field("ToolProfileCreateRequest", "name", map[string]any{"minLength": 1, "maxLength": 200, "x-max-utf8-bytes": 200})
	field("ToolExecutionProfile", "command_id", map[string]any{"pattern": `^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`})
	field("ToolExecutionProfile", "profile_sha256", map[string]any{"description": "Canonical SHA256 when available/registered; may be empty only in unavailable observations."})
	field("ToolExecutionProfile", "executable_sha256", map[string]any{"description": "Resolved executable byte digest, not complete transitive dependency attestation; may be empty only in unavailable observations."})
	field("ToolExecutionProfile", "tool_version", map[string]any{"maxLength": 256, "x-max-utf8-bytes": 256})
	field("ToolExecutionProfile", "read_only", map[string]any{"description": "Administrator policy declaration; not a generic operating-system sandbox attestation."})
	field("ToolObservation", "reason", map[string]any{"maxLength": 1000, "x-max-utf8-bytes": 1000})
	field("ToolObservation", "workspace_observation_protocol", map[string]any{"enum": []string{"workspace-observation-v1"}, "description": "Current Runner outbound read broker implementation. Declared only after a successful native workspace probe; not a profile grant or connection-health result."})
	field("ToolObservation", "workspace_write_protocol", map[string]any{"enum": []string{"workspace-write-v1"}, "description": "Current Runner outbound CAS-save broker. Requires the read broker declaration; current file/task/credential authority is checked separately."})
	field("ToolEvent", "schema_version", map[string]any{"const": 1})
	field("ToolEvent", "kind", map[string]any{"enum": []string{"session_started", "turn_started", "activity_started", "activity_updated", "activity_completed", "message", "usage", "error", "turn_completed", "turn_failed", "waiting_input"}})
	field("ToolEvent", "message", map[string]any{"maxLength": 8192, "x-max-utf8-bytes": 8192})
	for _, name := range []string{"session_id", "item_id"} {
		field("ToolEvent", name, map[string]any{"maxLength": 128})
	}
	field("ToolEvent", "status", map[string]any{"maxLength": 64})
	for name := range toolContractMap(schemas, "ToolUsage", "properties") {
		field("ToolUsage", name, map[string]any{"minimum": 0})
	}
	field("ToolEvidence", "terminal", map[string]any{"enum": []string{"completed", "failed", "unknown"}, "description": "Must agree with the process result; bound success requires completed evidence. Not a platform test or human acceptance verdict."})
	field("ToolEvidence", "event_count", map[string]any{"minimum": 0})
	field("TaskDispatchPlan", "state", map[string]any{"enum": []string{"ready", "started"}})
	for _, name := range []string{"created_at", "expires_at"} {
		field("TaskDispatchPlan", name, map[string]any{"format": "date-time"})
	}
	toolContractMap(schemas, "RunnerProgressRequest")["anyOf"] = []any{map[string]any{"required": []string{"message"}}, map[string]any{"required": []string{"tool_event"}, "properties": map[string]any{"tool_event": map[string]any{"type": "object"}}}}
}

func toolContractMap(root map[string]any, path ...string) map[string]any {
	current := root
	for _, key := range path {
		current = current[key].(map[string]any)
	}
	return current
}
