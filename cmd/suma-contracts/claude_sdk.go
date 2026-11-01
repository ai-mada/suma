package main

import (
	cl "github.com/ai-mada/suma/internal/claudecontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
)

// Applied after original protocol schemas so each source remains an exclusive
// native branch. A new SDK never widens Codex or ACP reply semantics.
func claudeSDKContracts(api map[string]any) {
	schemas := api["components"].(map[string]any)["schemas"].(map[string]any)
	for _, name := range []string{"ClaudeSDKExecutionCapability", "ClaudeSDKExecutionEvidence", "ClaudePermissionSource", "ClaudePermissionChoice", "ClaudePermissionEvidence", "ClaudeQuestionOption", "ClaudeQuestion", "ClaudeInputRequest", "ClaudeInputSelection", "ClaudeInputDecision", "ClaudeInputEvidence"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	for name, value := range map[string]string{"sdk_version": cl.SDKVersion, "cli_version": cl.CLIVersion, "model_source": "administrator_sdk_option", "budget_mode": "observe_only", "identity_evidence": "sdk_init_model_ack_only", "auth_strategy": "api_key_env_ref"} {
		toolContractMap(schemas, "ClaudeSDKExecutionCapability", "properties", name)["const"] = value
	}
	for name, value := range map[string]string{"sdk_version": cl.SDKVersion, "cli_version": cl.CLIVersion} {
		toolContractMap(schemas, "ClaudeSDKExecutionEvidence", "properties", name)["const"] = value
	}
	model := toolContractMap(schemas, "ClaudeSDKExecutionCapability", "properties", "model")
	model["pattern"] = `^claude-[a-z][a-z0-9-]*[0-9][a-z0-9.-]*$`
	model["maxLength"] = 256
	for _, name := range []string{"ClaudeSDKExecutionCapability", "ClaudeSDKExecutionEvidence", "ClaudePermissionSource", "ClaudePermissionEvidence", "ClaudeInputRequest", "ClaudeInputEvidence"} {
		toolContractMap(schemas, name, "properties", "configuration_sha256")["pattern"] = `^[a-f0-9]{64}$`
	}
	for _, name := range []string{"ClaudePermissionSource", "ClaudePermissionEvidence", "ClaudeInputRequest", "ClaudeInputEvidence"} {
		for _, field := range []string{"session_id", "tool_use_id", "request_id", "callback_id"} {
			p := toolContractMap(schemas, name, "properties", field)
			p["minLength"] = 1
			p["maxLength"] = 150
		}
		toolContractMap(schemas, name, "properties", "params_sha256")["pattern"] = `^[a-f0-9]{64}$`
	}
	for _, name := range []string{"ClaudePermissionSource", "ClaudeInputRequest"} {
		toolContractMap(schemas, name, "properties", "input_json")["maxLength"] = cl.MaxPayloadBytes
		toolContractMap(schemas, name, "properties", "input_json")["description"] = "Exact bounded SDK callback input JSON; params_sha256 hashes its UTF-8 bytes. No duplicate keys, invalid Unicode or hidden/truncated authorization details."
	}
	for _, name := range []string{"ClaudePermissionSource", "ClaudePermissionEvidence"} {
		toolContractMap(schemas, name, "properties", "tool_name")["enum"] = []string{"Read", "Edit", "Write", "Glob", "Grep", "Bash"}
	}
	toolContractMap(schemas, "ClaudePermissionChoice", "properties", "outcome")["enum"] = []string{"allow_once", "reject_once"}
	toolContractMap(schemas, "ClaudeInputDecision", "properties", "outcome")["enum"] = []string{"answered", "cancelled"}
	for _, name := range []string{"ClaudePermissionChoice", "ClaudeInputDecision"} {
		toolContractMap(schemas, name, "properties", "reason")["maxLength"] = 2000
	}
	for _, name := range []string{"ClaudeInputRequest", "ClaudeQuestion"} {
		field := "questions"
		minimum, maximum := 1, 4
		if name == "ClaudeQuestion" {
			field = "options"
			minimum = 2
		}
		p := toolContractMap(schemas, name, "properties", field)
		p["minItems"], p["maxItems"] = minimum, maximum
	}
	for _, name := range []string{"ClaudePermissionEvidence", "ClaudeInputEvidence"} {
		toolContractMap(schemas, name)["description"] = "Original platform record and exact native SDK callback identity. Observation is local host delivery/execution closure, never a provider acceptance ACK; request_resolved must remain false."
	}
	for _, field := range []string{"permissions", "user_inputs"} {
		toolContractMap(schemas, "ClaudeSDKExecutionEvidence", "properties", field)["maxItems"] = cl.MaxCallbacks
	}
	for _, field := range []string{"permission_requests", "denied_callbacks"} {
		p := toolContractMap(schemas, "ClaudeSDKExecutionEvidence", "properties", field)
		p["minimum"], p["maximum"] = 0, cl.MaxCallbacks
	}
	nativeDenials := toolContractMap(schemas, "ClaudeSDKExecutionEvidence", "properties", "native_permission_denials")
	nativeDenials["minimum"], nativeDenials["maximum"] = 0, 1000000
	nativeDenials["description"] = "Exact native result permission_denials array length; omitted until that result is observed, never synthesized from local callback denials."
	toolContractMap(schemas, "ClaudeSDKExecutionEvidence")["description"] = "Fixed SDK/CLI source and original foreground result; session/model acknowledgement, callback delivery, process stopping, code candidates, independent tests and human acceptance are distinct facts. Observe-only usage is never provider billing or a hard budget."
	sources := []struct{ field, protocol string }{{"acp", "acp-stdio-v1"}, {"codex", cc.Protocol}, {"claude", cl.Protocol}}
	for _, name := range []string{"ToolPermissionRequest", "ToolPermissionChoice", "ToolPermissionDecision", "ToolPermissionDecisionRequest"} {
		object := toolContractMap(schemas, name)
		toolContractMap(object, "properties", "protocol")["enum"] = []string{"acp-stdio-v1", cc.Protocol, cl.Protocol}
		branches := []any{}
		for _, source := range sources {
			forbidden := []any{}
			for _, other := range sources {
				if other.field != source.field {
					forbidden = append(forbidden, map[string]any{"required": []string{other.field}})
				}
			}
			branches = append(branches, map[string]any{"required": []string{source.field}, "properties": map[string]any{"protocol": map[string]any{"const": source.protocol}}, "not": map[string]any{"anyOf": forbidden}})
		}
		object["oneOf"] = branches
	}
	for _, name := range []string{"ToolPermissionObservationRequest", "PoolToolPermissionObservationRequest"} {
		toolContractMap(schemas, name)["oneOf"] = []any{map[string]any{"required": []string{"observation"}, "not": map[string]any{"required": []string{"claude_observation"}}}, map[string]any{"required": []string{"claude_observation"}, "not": map[string]any{"required": []string{"observation"}}}}
	}
	toolContractMap(schemas, "ToolInputRequest", "properties", "protocol")["enum"] = []string{cc.Protocol, "acp-stdio-v1", cl.Protocol}
	toolContractMap(schemas, "ToolInputList", "properties", "protocol")["enum"] = []string{"", cc.UserInputProtocol, cu.InputProtocol, cl.InputProtocol}
	toolContractMap(schemas, "ToolExecutionProfile", "properties", "user_input_protocol")["enum"] = []string{cc.UserInputProtocol, cu.InputProtocol, cl.InputProtocol}
	toolContractMap(schemas, "ToolExecutionProfile", "properties", "user_input_protocol")["description"] = "Exact writable native runtime input capability: Codex experimental questions, Cursor ACP questions/plan, or fixed Claude SDK questions. It never grants commands or formal acceptance."
	branches := []any{}
	nativeRequests := map[string]string{"codex": "CodexUserInputRequest", "cursor": "CursorInputRequest", "claude": "ClaudeInputRequest"}
	for _, source := range []struct{ field, protocol string }{{"codex", cc.Protocol}, {"cursor", "acp-stdio-v1"}, {"claude", cl.Protocol}} {
		others := []any{}
		for _, field := range []string{"codex", "cursor", "claude"} {
			if field != source.field {
				others = append(others, map[string]any{"required": []string{field}})
			}
		}
		branches = append(branches, map[string]any{"required": []string{source.field}, "properties": map[string]any{"protocol": map[string]any{"const": source.protocol}, source.field: map[string]any{"$ref": "#/components/schemas/" + nativeRequests[source.field]}}, "not": map[string]any{"anyOf": others}})
	}
	toolContractMap(schemas, "ToolInputRequest")["oneOf"] = branches
	for _, name := range []string{"ToolInputAnswer", "ToolInputAnswerRequest"} {
		toolContractMap(schemas, name)["oneOf"] = []any{
			map[string]any{"required": []string{"answers"}, "properties": map[string]any{"answers": map[string]any{"minItems": 1}}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"cursor"}}, map[string]any{"required": []string{"claude"}}}}},
			map[string]any{"required": []string{"cursor"}, "properties": map[string]any{"cursor": map[string]any{"$ref": "#/components/schemas/CursorInputDecision"}, "answers": map[string]any{"maxItems": 0}}, "not": map[string]any{"required": []string{"claude"}}},
			map[string]any{"required": []string{"claude"}, "properties": map[string]any{"claude": map[string]any{"$ref": "#/components/schemas/ClaudeInputDecision"}, "answers": map[string]any{"maxItems": 0}}, "not": map[string]any{"required": []string{"cursor"}}},
		}
	}
	toolContractMap(schemas, "ModelExecutionOptions", "properties", "fixed_claude_sdk")["description"] = "Administrator-fixed Agent SDK model and explicit API-key reference. Not a selectable gateway deployment, authenticated model health, provider billing or enforced allowance."
	for _, model := range []string{"ToolExecutionProfile", "ToolEvidence", "TaskDispatchPlan"} {
		toolContractMap(schemas, model, "properties", "claude_sdk_execution")["description"] = "Exact fixed official Agent SDK execution, separate from the foreground CLI and reverse MCP client. No browser-supplied process, model, credential or permission rules."
	}
	for _, model := range []string{"ToolPermission", "ToolPermissionObservationRequest", "PoolToolPermissionObservationRequest"} {
		toolContractMap(schemas, model, "properties", "claude_observation")["allOf"] = []any{map[string]any{"properties": map[string]any{"request_resolved": map[string]any{"const": false}}}}
	}
}
