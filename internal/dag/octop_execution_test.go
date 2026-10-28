package dag

import (
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestOctopDAGRequiresPinnedConfigurationAndStoppedProcess(t *testing.T) {
	fixed := &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion, ProviderID: "owned", Model: "model-a", EndpointOrigin: "https://models.example", ConfigurationSHA256: strings.Repeat("c", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only"}
	p := tc.Profile{CommandID: "octop-agent", Adapter: "octop", Protocol: tc.OctopProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: tc.OctopHarnessVersion, WorkspaceToolsVersion: "attempt-workspace-v1", OctopExecution: fixed}
	n := Node{ID: "develop", Type: "agent", Command: []string{p.CommandID}, Config: map[string]any{"tool_profile_id": "profile-one", "tool_profile_sha256": p.ProfileSHA256, "octop_execution": fixed}}
	proof := &tc.OctopExecutionEvidence{ConfigurationSHA256: fixed.ConfigurationSHA256, SessionID: "session_original", RequestedProvider: "owned", RequestedModel: "model-a", AcknowledgedProvider: "owned", AcknowledgedModel: "model-a", StopReason: "end_turn", CompletedAt: "2026-10-28T00:00:00Z", ProcessStopped: true}
	r := Result{ToolEvidence: &tc.Evidence{Profile: p, SessionID: proof.SessionID, Terminal: "completed", EventCount: 3, UsageState: tc.UsageUnreported, OctopExecution: proof}}
	if err := validateToolResult(n, r); err != nil {
		t.Fatal("current stopped original ACP result rejected", err)
	}
	for name, mutate := range map[string]func(*Node, *Result){
		"no-fixed-node":        func(n *Node, r *Result) { delete(n.Config, "octop_execution") },
		"retry":                func(n *Node, r *Result) { n.MaxRetries = 1 },
		"max-step-override":    func(n *Node, r *Result) { n.Config["max_steps"] = 8 },
		"resume":               func(n *Node, r *Result) { n.Config["resume"] = "other" },
		"end-turn-only":        func(n *Node, r *Result) { r.ToolEvidence.OctopExecution.ProcessStopped = false },
		"cancelled":            func(n *Node, r *Result) { r.ToolEvidence.OctopExecution.CancelRequested = true },
		"session":              func(n *Node, r *Result) { r.ToolEvidence.OctopExecution.SessionID = "foreign" },
		"failure-as-completed": func(n *Node, r *Result) { r.Error = "actual execution failure" },
	} {
		t.Run(name, func(t *testing.T) {
			badN := n
			badN.Config = map[string]any{}
			for key, value := range n.Config {
				badN.Config[key] = value
			}
			badR := r
			v := *r.ToolEvidence
			cp := *proof
			v.OctopExecution = &cp
			badR.ToolEvidence = &v
			mutate(&badN, &badR)
			if validateToolResult(badN, badR) == nil {
				t.Fatal("unbound or unconfirmed ACP result accepted")
			}
		})
	}
	unknown := r
	value := *r.ToolEvidence
	cp := *proof
	cp.ProcessStopped = false
	cp.CancelRequested = true
	value.Terminal = "unknown"
	value.OctopExecution = &cp
	unknown.ToolEvidence = &value
	unknown.Stopped = false
	unknown.OutcomeUnknown = true
	if err := validateToolResult(n, unknown); err != nil {
		t.Fatal("honest uncertain stop could not retain original facts", err)
	}
}
