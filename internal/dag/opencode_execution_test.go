package dag

import (
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func openCodeNodeResultFixture() (Node, Result) {
	fixed := &tc.OpenCodeExecutionCapability{SDKVersion: "1.19.4", ServerVersion: "1.19.4", ProviderID: "owned", Model: "model-a", EndpointOrigin: "https://models.example", ConfigurationSHA256: strings.Repeat("c", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only"}
	profile := tc.Profile{CommandID: "sdk-agent", Adapter: "opencode", Protocol: tc.OpenCodeSDKProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "1.19.4", WorkspaceToolsVersion: "attempt-workspace-v1", OpenCodeExecution: fixed}
	n := Node{ID: "develop", Type: "agent", Command: []string{"sdk-agent"}, Config: map[string]any{"tool_profile_id": "profile-one", "tool_profile_sha256": profile.ProfileSHA256, "opencode_execution": fixed}}
	r := Result{ToolEvidence: &tc.Evidence{Profile: profile, SessionID: "ses_original", Terminal: "completed", EventCount: 2, UsageState: tc.UsageUnreported, OpenCodeExecution: &tc.OpenCodeExecutionEvidence{ConfigurationSHA256: fixed.ConfigurationSHA256, SessionID: "ses_original", UserMessageID: "msg_user", AssistantMessageID: "msg_assistant", RequestedProvider: "owned", RequestedModel: "model-a", AcknowledgedProvider: "owned", AcknowledgedModel: "model-a", Finish: "stop", CompletedAt: "2026-10-28T00:00:00Z", ServerStopped: true}}}
	return n, r
}

func TestOpenCodeResultBindsExactSDKNodeAndStopFacts(t *testing.T) {
	n, r := openCodeNodeResultFixture()
	if err := validateToolResult(n, r); err != nil {
		t.Fatal("exact completed SDK result rejected", err)
	}
	for _, kind := range []string{"sdk-evidence-without-node", "foreign-profile", "missing-sdk-proof", "stopped-without-evidence", "unclosed-server"} {
		t.Run(kind, func(t *testing.T) {
			n, r := openCodeNodeResultFixture()
			switch kind {
			case "sdk-evidence-without-node":
				delete(n.Config, "opencode_execution")
			case "foreign-profile":
				fixed := *r.ToolEvidence.Profile.OpenCodeExecution
				fixed.Model = "other"
				r.ToolEvidence.Profile.OpenCodeExecution = &fixed
				r.ToolEvidence.OpenCodeExecution.RequestedModel = "other"
				r.ToolEvidence.OpenCodeExecution.AcknowledgedModel = "other"
			case "missing-sdk-proof":
				r.ToolEvidence.OpenCodeExecution = nil
			case "stopped-without-evidence":
				r.Stopped = true
				r.ToolEvidence = nil
			case "unclosed-server":
				r.ExitCode = 1
				r.ToolEvidence.Terminal = "failed"
				r.ToolEvidence.OpenCodeExecution.ServerStopped = false
			}
			if validateToolResult(n, r) == nil {
				t.Fatal("unbound/forged SDK result accepted")
			}
		})
	}
	n, r = openCodeNodeResultFixture()
	r.ExitCode = 1
	r.ToolEvidence = nil
	if err := validateToolResult(n, r); err != nil {
		t.Fatal("known prelaunch failure required fabricated protocol facts", err)
	}
}
