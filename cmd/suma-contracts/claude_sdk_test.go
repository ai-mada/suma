package main

import (
	cl "github.com/ai-mada/suma/internal/claudecontract"
	"testing"
)

func TestClaudeSDKContractsSeparateNativeInputsAndBudgetAuthority(t *testing.T) {
	api, _ := generate()
	s := mapAt(api, "components", "schemas")
	for key, value := range map[string]string{"sdk_version": cl.SDKVersion, "cli_version": cl.CLIVersion, "model_source": "administrator_sdk_option", "budget_mode": "observe_only", "auth_strategy": "api_key_env_ref"} {
		if mapAt(s, "ClaudeSDKExecutionCapability", "properties", key)["const"] != value {
			t.Fatal("SDK fixed capability changed", key)
		}
	}
	for _, name := range []string{"ClaudePermissionSource", "ClaudeInputRequest"} {
		if mapAt(s, name)["additionalProperties"] != false || mapAt(s, name, "properties")["request_id"] == nil || mapAt(s, name, "properties")["callback_id"] == nil {
			t.Fatal("native ID and local callback identity collapsed", name)
		}
	}
	for _, name := range []string{"ToolPermissionRequest", "ToolPermissionChoice", "ToolInputRequest", "ToolInputAnswerRequest"} {
		if len(mapAt(s, name)["oneOf"].([]any)) != 3 {
			t.Fatal("runtime native branch missing", name)
		}
	}
	for _, branch := range mapAt(s, "ToolInputRequest")["oneOf"].([]any) {
		b := branch.(map[string]any)
		for _, field := range b["required"].([]string) {
			if mapAt(b, "properties", field)["$ref"] == nil {
				t.Fatal("required native source still admits null", field)
			}
		}
	}
	for _, name := range []string{"ToolInputAnswer", "ToolInputAnswerRequest"} {
		for _, branch := range mapAt(s, name)["oneOf"].([]any) {
			b := branch.(map[string]any)
			field := b["required"].([]string)[0]
			if field != "answers" && mapAt(b, "properties", field)["$ref"] == nil {
				t.Fatal("native decision still admits null", name, field)
			}
		}
	}
	for _, name := range []string{"ToolPermissionObservationRequest", "PoolToolPermissionObservationRequest"} {
		if len(mapAt(s, name)["oneOf"].([]any)) != 2 {
			t.Fatal("observation not exclusive", name)
		}
		restriction := mapAt(s, name, "properties", "claude_observation")["allOf"].([]any)[0].(map[string]any)
		if mapAt(restriction, "properties", "request_resolved")["const"] != false {
			t.Fatal("SDK callback return became native ACK")
		}
	}
	for _, name := range []string{"ClaudeSDKExecutionCapability", "ClaudeInputDecision", "ClaudePermissionChoice"} {
		for _, key := range []string{"api_key", "executable", "updatedPermissions", "input_json", "model_parameters"} {
			if mapAt(s, name, "properties")[key] != nil {
				t.Fatal("human decision acquired execution configuration", name, key)
			}
		}
	}
	if mapAt(s, "ModelExecutionOptions", "properties", "fixed_claude_sdk")["description"] == nil {
		t.Fatal("SDK fixed model presented as gateway")
	}
	fields := mapAt(s, "ClaudeSDKExecutionEvidence", "properties")
	if fields["denied_permissions"] != nil || fields["denied_callbacks"] == nil || fields["native_permission_denials"] == nil {
		t.Fatal("local and native denial facts were merged")
	}
	for _, required := range mapAt(s, "ClaudeSDKExecutionEvidence")["required"].([]string) {
		if required == "native_permission_denials" {
			t.Fatal("missing native result synthesized a denial count")
		}
	}
}
