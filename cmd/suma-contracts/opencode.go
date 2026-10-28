package main

import tc "github.com/ai-mada/suma/internal/toolcontract"

// Public SDK facts come from the same Go models as the Runner and control
// plane. No browser field accepts local executable paths or credentials.
func openCodeExecutionSchemas(schemas map[string]any) {
	for _, model := range []string{"OpenCodeExecutionCapability", "OpenCodeExecutionEvidence"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	capability := toolContractMap(schemas, "OpenCodeExecutionCapability", "properties")
	for _, field := range []string{"sdk_version", "server_version"} {
		capability[field].(map[string]any)["const"] = tc.OpenCodeSDKVersion
	}
	capability["budget_mode"].(map[string]any)["const"] = "observe_only"
	capability["identity_evidence"].(map[string]any)["const"] = "configuration_ack_only"
	capability["configuration_sha256"].(map[string]any)["pattern"] = `^[a-f0-9]{64}$`
	capability["provider_id"].(map[string]any)["pattern"] = `^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`
	capability["model"].(map[string]any)["pattern"] = `^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,255}$`
	toolContractMap(schemas, "ModelExecutionOptions", "properties", "fixed_opencode")["description"] = "Fixed administrator-approved SDK configuration. Not an independently selectable deployment, vendor identity attestation, price or hard budget."
	toolContractMap(schemas, "TaskDispatchPlan", "properties", "opencode_execution")["description"] = "Original reviewed fixed SDK configuration; must equal the producer node and approved current tool profile at admission."
	toolContractMap(schemas, "ToolEvidence", "properties", "opencode_execution")["description"] = "Correlated original session/user/assistant and private service shutdown facts. Whole-turn completion, independent tests and human acceptance are separate facts."
}
