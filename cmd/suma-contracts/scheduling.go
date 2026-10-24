package main

import (
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
)

func schedulingModels() []any {
	return []any{platform.RunnerClaimRequest{}, platform.RunnerClaimResponse{}, platform.SchedulingPolicyView{}, platform.SchedulingPolicyPreviewRequest{}, platform.SchedulingPolicyCommitRequest{}, platform.SchedulingPolicyPreview{}, dag.ReadyPage{}, dag.NodeAdmissionSnapshot{}, dag.AttemptPage{}, dag.ExecutionAttempt{}, dag.TickReport{}}
}

func schedulingContracts(add operationAdder, paths map[string]any) {
	for _, route := range []struct {
		path, model string
		paged       bool
	}{
		{"/scheduling/policy", "SchedulingPolicyView", false},
		{"/scheduling/policy-previews/{id}", "SchedulingPolicyPreview", false},
		{"/scheduling/queue", "ReadyPage", true},
		{"/runs/{id}/nodes/{node}/scheduling", "NodeAdmissionSnapshot", false},
		{"/runs/{id}/nodes/{node}/attempts", "AttemptPage", true},
		{"/runs/{id}/nodes/{node}/attempts/{attempt}", "ExecutionAttempt", false},
	} {
		op := add(route.path, "get", route.model, false, false)
		successStatus(op, "200")
		op["x-disallowed-token-scopes"] = []string{"agent", "runner"}
		if route.paged {
			appendQuery(op, queryParameter("limit", "Default50; maximum100. No invented queue rank or ETA.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 50}))
			appendQuery(op, queryParameter("cursor", "Opaque keyset cursor bound to tenant, project and collection (including run/node for attempts).", map[string]any{"type": "string"}))
		}
	}
	op := add("/scheduling/policy-previews", "post", "SchedulingPolicyPreview", false, true)
	browserOnly(op)
	request(op, "SchedulingPolicyPreviewRequest")
	successStatus(op, "201")
	op["description"] = "Immutable30-minute review of complete project admission policy, current policy version, digest, changes and observed impact. Does not dispatch, reserve, charge or apply a policy. Current write+approval authority is checked before operation receipt replay."
	op = add("/scheduling/policy-previews/{id}/commit", "post", "SchedulingPolicyView", false, true)
	browserOnly(op)
	request(op, "SchedulingPolicyCommitRequest")
	successStatus(op, "200")
	op["description"] = "Current human write+approval, original policy version and preview version are checked in the transaction that applies the policy, audit and receipt. Reduced capacity blocks new attempts without terminating active work or refunding cumulative admission units. Same operation recovers the original applied policy; a new operation cannot recommit the preview."
	op = toolContractMap(paths, "/runner/claim", "post")
	request(op, "RunnerClaimRequest")
	successStatus(op, "200")
	op["responses"].(map[string]any)["200"].(map[string]any)["content"] = map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/RunnerClaimResponse"}}}
	op["description"] = "Durable stable operation required. Current exact worker authority is checked before restoring the same scoped capability fingerprint and dispatch. Retransmission never creates another attempt. Null assignment is a persisted empty receipt; closed/reconciling must never start. Matching tool/candidate/resource policy is enforced before consuming a new attempt. This is project run rotation, not cross-tenant pool fairness."
}

func enhanceSchedulingSchemas(schemas map[string]any) {
	const max = int64(9007199254740991)
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for key, val := range attrs {
			value[key] = val
		}
	}
	for _, model := range []string{"SchedulingPolicyInput", "SchedulingPolicyPreviewRequest", "SchedulingPolicyCommitRequest", "RunnerClaimRequest"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"SchedulingPolicyPreviewRequest", "SchedulingPolicyCommitRequest", "RunnerClaimRequest"} {
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, model := range []string{"SchedulingPolicy", "SchedulingPolicyView", "SchedulingPolicyInput"} {
		if schemas[model] == nil {
			continue
		}
		field(model, "concurrency_limit", map[string]any{"minimum": 0, "maximum": max, "description": "Configured project admission slots, not measured hardware or an OS hard limit. Zero prevents new worker attempts."})
		field(model, "resource_limits", map[string]any{"maxProperties": 64, "additionalProperties": map[string]any{"anyOf": []any{map[string]any{"type": "integer", "minimum": 0, "maximum": max}, map[string]any{"type": "null"}}}})
		field(model, "budget_limit", map[string]any{"anyOf": []any{map[string]any{"type": "integer", "minimum": 0, "maximum": max}, map[string]any{"type": "null"}}, "description": "Cumulative admission units; null explicitly unlimited, zero prevents new worker attempts. Not tokens, monetary settlement or refundable capacity."})
	}
	field("SchedulingPolicyPreviewRequest", "expected_version", map[string]any{"minimum": 0})
	field("SchedulingPolicyCommitRequest", "expected_version", map[string]any{"minimum": 1})
	field("SchedulingPolicyPreview", "state", map[string]any{"enum": []string{"ready", "committed"}})
	field("NodeAdmissionSnapshot", "status", map[string]any{"enum": []string{"eligible", "blocked", "unknown", "not_pending"}})
	field("Assignment", "dispatch_state", map[string]any{"enum": []string{"active", "closed", "reconciling"}})
	field("Node", "max_retries", map[string]any{"minimum": 0, "maximum": 2, "description": "Only explicitly transient failures of idempotent nodes may automatically retry; each retry creates a new attempt with durable backoff and a fixed total deadline."})
	field("Node", "timeout_seconds", map[string]any{"minimum": 0, "maximum": int64(9223372036), "description": "Worker automatic-retry sequence total deadline; zero/omitted uses300 seconds for new kernel generation. Manual retry explicitly opens a new persisted window. Non-worker timeout semantics depend on node type. Bound prevents Go duration overflow."})
	field("Assignment", "timeout_seconds", map[string]any{"minimum": 0, "maximum": int64(9223372036)})
	field("ExecutionAttempt", "result_source", map[string]any{"enum": []string{"not_loaded", "unreported", "runner_result", "manual_reconciliation", "child_workflow", "managed_observation"}, "description": "List is metadata-only and returns not_loaded. Exact detail distinguishes a real runner receipt, manual reconciliation, child workflow and independently observed managed provider result; these are not interchangeable process evidence."})
	field("Result", "failure_class", map[string]any{"enum": []string{"", "transient", "permanent", "protocol"}, "description": "Only an explicitly known transient failure of an idempotent operation is eligible for bounded automatic retry; unknown outcomes require reconciliation."})
	toolContractMap(schemas, "ToolUsage")["minProperties"] = 1
	for name := range toolContractMap(schemas, "ToolUsage", "properties") {
		toolContractMap(schemas, "ToolUsage", "properties")[name] = map[string]any{"type": "integer", "minimum": 0, "maximum": max, "description": "Present zero is a reported zero; omitted is unreported. Null, fractions and overflow are invalid."}
	}
	for _, model := range []string{"ToolEvent", "ToolEvidence"} {
		field(model, "usage_state", map[string]any{"enum": []string{"complete", "partial", "unreported"}, "description": "complete requires known input/output; partial preserves reported subtotals and unknown steps; unreported has no usage object. Outer null is absence; ordinary non-usage events may omit state as well. Event state is required when kind=usage or a usage object is present."})
		conditions := []any{}
		for _, state := range []string{"complete", "partial", "unreported"} {
			usage := map[string]any{"type": "object"}
			then := map[string]any{"required": []string{"usage"}, "properties": map[string]any{"usage": usage}}
			if state == "complete" {
				usage["required"] = []string{"input_tokens", "output_tokens"}
			}
			if state == "unreported" {
				then = map[string]any{"properties": map[string]any{"usage": map[string]any{"type": "null"}}}
			}
			conditions = append(conditions, map[string]any{"if": map[string]any{"required": []string{"usage_state"}, "properties": map[string]any{"usage_state": map[string]any{"const": state}}}, "then": then})
		}
		if model == "ToolEvent" {
			field(model, "usage_state", map[string]any{"enum": []string{"", "complete", "partial", "unreported"}})
			conditions = append(conditions, map[string]any{"if": map[string]any{"anyOf": []any{
				map[string]any{"required": []string{"kind"}, "properties": map[string]any{"kind": map[string]any{"const": "usage"}}},
				map[string]any{"required": []string{"usage"}, "properties": map[string]any{"usage": map[string]any{"type": "object"}}},
			}}, "then": map[string]any{"required": []string{"usage_state"}, "properties": map[string]any{"usage_state": map[string]any{"enum": []string{"complete", "partial", "unreported"}}}}})
		}
		toolContractMap(schemas, model)["allOf"] = conditions
	}
}
