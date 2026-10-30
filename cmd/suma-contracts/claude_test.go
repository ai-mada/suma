package main

import "testing"

// A client must see the same fixed-model and budget boundary that admission
// enforces. Local paths and credentials never belong to these public facts.
func TestClaudeContractKeepsFixedModelAndObservedBudget(t *testing.T) {
	api, _ := generate()
	schemas := toolContractMap(api, "components", "schemas")
	capability, ok := schemas["ClaudeExecutionCapability"].(map[string]any)
	if !ok {
		t.Fatal("missing fixed Claude execution capability")
	}
	fields := toolContractMap(capability, "properties")
	if capability["additionalProperties"] != false || len(fields) != 6 {
		t.Fatal("Claude capability is not a closed nonsecret configuration")
	}
	for name, value := range map[string]string{"model_source": "administrator_cli_argument", "budget_mode": "observe_only", "identity_evidence": "configuration_ack_only", "auth_strategy": "existing_cli_login"} {
		if toolContractMap(fields, name)["const"] != value {
			t.Fatalf("Claude %s must remain %s", name, value)
		}
	}
	proof := toolContractMap(schemas, "ClaudeExecutionEvidence")
	if proof["additionalProperties"] != false {
		t.Fatal("Claude result correlation must be closed")
	}
	for _, ref := range []struct{ model, field string }{{"ToolExecutionProfile", "claude_execution"}, {"ToolEvidence", "claude_execution"}, {"TaskDispatchPlan", "claude_execution"}, {"ModelExecutionOptions", "fixed_claude"}} {
		if toolContractMap(schemas, ref.model, "properties", ref.field)["description"] == nil {
			t.Fatalf("missing public boundary for %s.%s", ref.model, ref.field)
		}
	}
	for _, model := range []string{"ClaudeExecutionCapability", "ClaudeExecutionEvidence"} {
		for _, forbidden := range []string{"api_key", "executable", "args", "home", "permission_mode", "max_cost"} {
			if toolContractMap(schemas, model, "properties")[forbidden] != nil {
				t.Fatalf("%s exposes local authority %s", model, forbidden)
			}
		}
	}
}
