package main

// These are execution facts, not browser-editable process options. Keeping
// the restrictions in the generated contract prevents clients from presenting
// a fixed CLI login/model as a gateway deployment or an enforced allowance.
func claudeExecutionSchemas(schemas map[string]any) {
	for _, model := range []string{"ClaudeExecutionCapability", "ClaudeExecutionEvidence"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	capability := toolContractMap(schemas, "ClaudeExecutionCapability", "properties")
	for name, value := range map[string]string{"model_source": "administrator_cli_argument", "budget_mode": "observe_only", "identity_evidence": "configuration_ack_only", "auth_strategy": "existing_cli_login"} {
		toolContractMap(capability, name)["const"] = value
	}
	toolContractMap(capability, "model")["pattern"] = `^claude-[a-z][a-z0-9-]*[0-9][a-z0-9.-]*$`
	toolContractMap(capability, "model")["maxLength"] = 256
	toolContractMap(capability, "configuration_sha256")["pattern"] = `^[a-f0-9]{64}$`
	proof := toolContractMap(schemas, "ClaudeExecutionEvidence", "properties")
	toolContractMap(proof, "configuration_sha256")["pattern"] = `^[a-f0-9]{64}$`
	for _, name := range []string{"permission_denial_count", "queued_turn_count"} {
		toolContractMap(proof, name)["minimum"] = 0
		toolContractMap(proof, name)["maximum"] = 1000000
	}
	toolContractMap(schemas, "ToolExecutionProfile", "properties", "claude_execution")["description"] = "Administrator-fixed Claude Code foreground CLI configuration using its existing local login. No browser-supplied model, process arguments or credentials."
	toolContractMap(schemas, "ModelExecutionOptions", "properties", "fixed_claude")["description"] = "Fixed model acknowledged by the reviewed CLI configuration; observed usage only. Neither vendor identity proof, selectable gateway deployment nor a hard budget."
	toolContractMap(schemas, "TaskDispatchPlan", "properties", "claude_execution")["description"] = "Original fixed CLI configuration, rechecked against the producer and current authorized profile before any worktree or process effect."
	toolContractMap(schemas, "ToolEvidence", "properties", "claude_execution")["description"] = "Exact foreground session/model/result correlation. A terminal success requires the requested model, no denied permissions and no deferred work; independent candidate tests and human acceptance remain separate."
}
