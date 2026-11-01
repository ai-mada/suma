package toolcontract

import (
	cl "github.com/ai-mada/suma/internal/claudecontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	"strings"
	"testing"
)

func sdkTestProfile() Profile {
	h := strings.Repeat("a", 64)
	return Profile{CommandID: "sdk-claude", Adapter: "claude", Protocol: cl.Protocol, ProfileSHA256: h, ExecutableSHA256: h, ToolVersion: cl.CLIVersion, UserInputProtocol: cl.InputProtocol, ClaudeSDKExecution: &ClaudeSDKExecutionCapability{SDKVersion: cl.SDKVersion, CLIVersion: cl.CLIVersion, Model: "claude-sonnet-5", ConfigurationSHA256: h, ModelSource: "administrator_sdk_option", BudgetMode: "observe_only", IdentityEvidence: "sdk_init_model_ack_only", AuthStrategy: "api_key_env_ref"}}
}
func TestClaudeSDKProfileCannotBorrowCLIOrGatewayAuthority(t *testing.T) {
	directory := catalogEntry(t, "claude-code-sdk")
	if directory.Direction != "suma_to_tool" || directory.Implementation != "implemented" || directory.Protocol != ClaudeSDKProtocol || catalogCapability(t, directory, "human_input") != "supported" {
		t.Fatal("native SDK catalog substituted the CLI or reverse client")
	}
	for _, unsupported := range []string{"model_selection", "gateway_budget", "provider_billing", "session_load", "interrupted_resume", "independent_test"} {
		if catalogCapability(t, directory, unsupported) != "unsupported" {
			t.Fatal("SDK catalog invented authority", unsupported)
		}
	}
	p := sdkTestProfile()
	if err := ValidateProfile(p, true); err != nil {
		t.Fatal(err)
	}
	cloned := CloneProfile(p)
	if !EqualProfile(p, cloned) || p.ClaudeSDKExecution == cloned.ClaudeSDKExecution {
		t.Fatal("fixed SDK profile cloning lost value identity")
	}
	cloned.ClaudeSDKExecution.Model = "claude-opus-5"
	if EqualProfile(p, cloned) {
		t.Fatal("model change reused fixed profile")
	}
	for _, mutate := range []func(*Profile){func(v *Profile) { v.Protocol = ClaudeProtocol }, func(v *Profile) { v.UserInputProtocol = "" }, func(v *Profile) { v.ClaudeExecution = &ClaudeExecutionCapability{} }, func(v *Profile) { v.ClaudeSDKExecution.BudgetMode = "enforce" }, func(v *Profile) { v.ClaudeSDKExecution.SDKVersion = "0.3.316" }} {
		v := CloneProfile(p)
		mutate(&v)
		if ValidateProfile(v, true) == nil {
			t.Fatal("SDK profile admitted a different capability")
		}
	}
	for _, key := range []string{"model", "permissions", "env", "resume", "max_cost_micros", "claude_execution"} {
		if _, err := DecodeClaudeSDKNodeConfiguration(map[string]any{"claude_sdk_execution": *p.ClaudeSDKExecution, key: nil}); err == nil {
			t.Fatalf("node override admitted %s", key)
		}
	}
}
func TestClaudeSDKSuccessNeedsExactDeliveryClosureAndStoppedTree(t *testing.T) {
	p := sdkTestProfile()
	h := strings.Repeat("a", 64)
	zero := 0
	proof := ClaudeSDKExecutionEvidence{SDKVersion: cl.SDKVersion, CLIVersion: cl.CLIVersion, ConfigurationSHA256: h, SessionID: "sdk-session", RequestedModel: p.ClaudeSDKExecution.Model, AcknowledgedModel: p.ClaudeSDKExecution.Model, ResultSubtype: "success", TurnEnded: true, ToolsCompleted: true, HostProcessStopped: true, HostProcessTreeStopped: true, PermissionRequests: 1, Permissions: []cl.PermissionEvidence{{PermissionID: "permission-id", Basis: cl.Basis{SessionID: "sdk-session", ToolUseID: "tool-1", RequestID: "native-1", CallbackID: "callback-1", ConfigurationSHA256: h, ParamsSHA256: h}, ToolName: "Bash", Observation: &delivery.Observation{Sequence: 1, ParamsSHA256: h, AnswerSHA256: h, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, ExecutionClosed: true}}}}
	proof.NativePermissionDenials = &zero
	e := Evidence{Profile: p, SessionID: proof.SessionID, Terminal: "completed", UsageState: UsageUnreported, ClaudeSDKExecution: &proof}
	if err := validateClaudeSDKEvidence(e); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*ClaudeSDKExecutionEvidence){func(v *ClaudeSDKExecutionEvidence) { v.Permissions[0].Observation.RequestResolved = true }, func(v *ClaudeSDKExecutionEvidence) { v.Permissions[0].Observation.ExecutionClosed = false }, func(v *ClaudeSDKExecutionEvidence) { v.Permissions[0].Basis.RequestID = "" }, func(v *ClaudeSDKExecutionEvidence) { v.HostProcessTreeStopped = false }, func(v *ClaudeSDKExecutionEvidence) { v.Permissions = nil }, func(v *ClaudeSDKExecutionEvidence) { v.DeniedCallbacks = 1 }, func(v *ClaudeSDKExecutionEvidence) { v.NativePermissionDenials = nil }, func(v *ClaudeSDKExecutionEvidence) { *v.NativePermissionDenials = 1 }} {
		copy := CloneClaudeSDKExecutionEvidence(proof)
		mutate(&copy)
		e.ClaudeSDKExecution = &copy
		if validateClaudeSDKEvidence(e) == nil {
			t.Fatal("incomplete SDK operation became successful")
		}
	}
	if proof.Permissions[0].Observation.RequestResolved {
		t.Fatal("evidence clone mutated stored source")
	}
}
