package main

import tc "github.com/ai-mada/suma/internal/toolcontract"

func octopExecutionSchemas(schemas map[string]any) {
	for _, model := range []string{"OctopExecutionCapability", "OctopExecutionEvidence"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	fields := toolContractMap(schemas, "OctopExecutionCapability", "properties")
	for name, value := range map[string]string{"harness_version": tc.OctopHarnessVersion, "acp_sdk_version": tc.OctopACPSDKVersion, "budget_mode": "observe_only", "identity_evidence": "configuration_ack_only"} {
		fields[name].(map[string]any)["const"] = value
	}
	fields["configuration_sha256"].(map[string]any)["pattern"] = `^[a-f0-9]{64}$`
	toolContractMap(schemas, "ToolEvidence", "properties", "octop_execution")["description"] = "Original ACP session and fixed Harness configuration. ACP end_turn, cancellation intent, Go-confirmed process stop, independent tests and human acceptance are separate facts. No vendor identity or complete billing attestation."
}
