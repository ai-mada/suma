package main

import mc "github.com/ai-mada/suma/internal/modelcontract"

func modelGatewaySchemas(schemas map[string]any) {
	for _, model := range []string{"ModelEvidence", "ModelRequestEvidence", "ModelRequestUsage"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	toolContractMap(schemas, "ModelEvidence", "properties", "gateway_protocol")["const"] = mc.GatewayProtocol
	toolContractMap(schemas, "ModelRequestEvidence", "properties", "state")["enum"] = []string{"started", "completed", "rejected", "unknown"}
	toolContractMap(schemas, "ModelRequestEvidence", "properties", "step")["minimum"] = 0
	toolContractMap(schemas, "ModelRequestEvidence", "properties", "step")["maximum"] = mc.MaxSteps - 1
	toolContractMap(schemas, "ModelEvidence", "properties", "requests")["maxItems"] = mc.MaxSteps
	toolContractMap(schemas, "ModelExecutionOptions")["allOf"] = []any{map[string]any{"if": map[string]any{"properties": map[string]any{"supported": map[string]any{"const": true}}}, "then": map[string]any{"required": []string{"parameter_limits"}}}}
}
