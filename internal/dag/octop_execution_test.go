package dag

import (
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strings"
	"testing"
)

func TestOctopDAGRequiresGatewayBindingAndStoppedProcess(t *testing.T) {
	d := mc.Deployment{ID: "model-a", Revision: 1, Protocol: mc.ProviderProtocol, Model: "model-a", EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("d", 64), ContextWindow: 100}
	d.SHA256 = mc.DeploymentDigest(d)
	b, err := mc.NewBinding(d, mc.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := tc.Profile{CommandID: "octop-agent", Adapter: "octop", Protocol: tc.OctopProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: tc.OctopHarnessVersion, WorkspaceToolsVersion: "attempt-workspace-v1", ModelExecutionProtocol: mc.Protocol, OctopExecution: &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion}}
	n := Node{ID: "develop", Type: "agent", Command: []string{p.CommandID}, Config: map[string]any{"tool_profile_id": "profile-one", "tool_profile_sha256": p.ProfileSHA256, "model_execution": b, "max_steps": 2, "max_output_tokens": 100}}
	proof := &tc.OctopExecutionEvidence{BindingSHA256: b.BindingSHA256, SessionID: "real-acp-session", StopReason: "end_turn", CompletedAt: "2026-10-28T00:00:00Z", ProcessStopped: true}
	me := &mc.Evidence{GatewayProtocol: mc.GatewayProtocol, GatewayClosed: true, BindingSHA256: b.BindingSHA256, DeploymentSHA256: d.SHA256, ParametersSHA256: b.ParametersSHA256, Model: d.Model, Requests: []mc.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: d.Model, OutputAllowance: 100, UsageState: "unreported"}}}
	r := Result{ToolEvidence: &tc.Evidence{Profile: p, SessionID: proof.SessionID, Terminal: "completed", EventCount: 3, UsageState: tc.UsageUnreported, OctopExecution: proof, ModelExecution: me}}
	if err := validateToolResult(n, r); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"missing-binding", "old-snapshot", "retry", "limits", "resume", "stop", "cancel", "session", "gateway", "actual-failure"} {
		t.Run(kind, func(t *testing.T) {
			badN := n
			badN.Config = map[string]any{}
			for k, v := range n.Config {
				badN.Config[k] = v
			}
			badR := r
			ev := *r.ToolEvidence
			cp := *proof
			cm := *me
			ev.OctopExecution = &cp
			ev.ModelExecution = &cm
			badR.ToolEvidence = &ev
			switch kind {
			case "missing-binding":
				delete(badN.Config, "model_execution")
			case "old-snapshot":
				badN.Config["octop_execution"] = p.OctopExecution
			case "retry":
				badN.MaxRetries = 1
			case "limits":
				badN.Config["max_steps"] = 3
			case "resume":
				badN.Config["resume"] = "foreign"
			case "stop":
				cp.ProcessStopped = false
			case "cancel":
				cp.CancelRequested = true
			case "session":
				cp.SessionID = "foreign"
			case "gateway":
				cm.GatewayClosed = false
			case "actual-failure":
				badR.Error = "source check failed"
			}
			if validateToolResult(badN, badR) == nil {
				t.Fatal("unreviewed or unconfirmed ACP result accepted")
			}
		})
	}
}
