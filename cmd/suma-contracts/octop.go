package main

import tc "github.com/ai-mada/suma/internal/toolcontract"

func octopExecutionSchemas(schemas map[string]any) {
	for _, model := range []string{"OctopExecutionCapability", "OctopExecutionEvidence"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	fields := toolContractMap(schemas, "OctopExecutionCapability", "properties")
	for name, value := range map[string]string{"harness_version": tc.OctopHarnessVersion, "acp_sdk_version": tc.OctopACPSDKVersion} {
		fields[name].(map[string]any)["const"] = value
	}
	toolContractMap(schemas, "ToolEvidence", "properties", "octop_execution")["description"] = "Original ACP session, reviewed common model binding and Go-confirmed process stop. Model request/usage authority is the Go gateway; ACP end_turn and cancellation are separate from code, tests and human acceptance."
}
