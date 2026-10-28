package toolcontract

import (
	"encoding/json"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	"strings"
	"testing"
)

func octopContractProfile() Profile {
	return Profile{CommandID: "octop-acp", Adapter: "octop", Protocol: OctopProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: OctopHarnessVersion, WorkspaceToolsVersion: "attempt-workspace-v1", ModelExecutionProtocol: mc.Protocol, OctopExecution: &OctopExecutionCapability{HarnessVersion: OctopHarnessVersion, ACPSDKVersion: OctopACPSDKVersion}}
}

func TestOctopGatewayCapabilityAndStrictEvidence(t *testing.T) {
	p := octopContractProfile()
	if err := ValidateProfile(p, true); err != nil {
		t.Fatal(err)
	}
	clone := CloneProfile(p)
	clone.OctopExecution.HarnessVersion = "foreign"
	if EqualProfile(p, clone) || p.OctopExecution.HarnessVersion != OctopHarnessVersion {
		t.Fatal("adapter capability was aliased")
	}
	for _, mutate := range []func(*Profile){func(p *Profile) { p.ModelExecutionProtocol = "" }, func(p *Profile) { p.ReadOnly = true }, func(p *Profile) { p.WorkspaceToolsVersion = "" }, func(p *Profile) { p.IntelligentOperations = &IntelligentOperationsCapability{} }, func(p *Profile) { p.OctopExecution.ACPSDKVersion = "other" }} {
		bad := CloneProfile(p)
		mutate(&bad)
		if ValidateProfile(bad, true) == nil {
			t.Fatal("unsupported mixed capability admitted")
		}
	}
	model := &mc.Evidence{GatewayProtocol: mc.GatewayProtocol, GatewayClosed: true, BindingSHA256: strings.Repeat("c", 64), DeploymentSHA256: strings.Repeat("d", 64), ParametersSHA256: strings.Repeat("e", 64), Model: "model-a", Requests: []mc.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("f", 64), ResponseSHA256: strings.Repeat("a", 64), ProviderModel: "model-a", OutputAllowance: 10, UsageState: "unreported"}}}
	proof := &OctopExecutionEvidence{BindingSHA256: model.BindingSHA256, SessionID: "real-acp-session", StopReason: "end_turn", CompletedAt: "2026-10-28T00:00:00Z", ProcessStopped: true}
	e := Evidence{Profile: p, SessionID: proof.SessionID, Terminal: "completed", EventCount: 3, UsageState: UsageUnreported, ModelExecution: model, OctopExecution: proof}
	if err := ValidateEvidence(e); err != nil {
		t.Fatal("stopped gateway-bound ACP reply rejected", err)
	}
	for _, mutate := range []func(*Evidence){func(e *Evidence) { e.OctopExecution.ProcessStopped = false }, func(e *Evidence) { e.OctopExecution.CancelRequested = true }, func(e *Evidence) { e.OctopExecution.BindingSHA256 = strings.Repeat("b", 64) }, func(e *Evidence) { e.ModelExecution.GatewayClosed = false }, func(e *Evidence) { e.ModelExecution = nil }, func(e *Evidence) { e.OctopExecution.StopReason = "refusal" }} {
		bad := e
		cp := *proof
		cm := *model
		bad.OctopExecution = &cp
		bad.ModelExecution = &cm
		mutate(&bad)
		if ValidateEvidence(bad) == nil {
			t.Fatal("uncorrelated ACP completion admitted")
		}
	}
	for _, raw := range []string{`{"harness_version":"1.0.5","acp_sdk_version":"0.12.1","provider_id":"old"}`, `{"harness_version":"1.0.5","harness_version":"1.0.5","acp_sdk_version":"0.12.1"}`, `{"harness_version":null,"acp_sdk_version":"0.12.1"}`, `null`} {
		var value OctopExecutionCapability
		if json.Unmarshal([]byte(raw), &value) == nil {
			t.Fatal("non-closed runtime capability accepted", raw)
		}
	}
	raw, _ := json.Marshal(proof)
	for _, bad := range []string{strings.Replace(string(raw), `"process_stopped":true`, `"process_stopped":null`, 1), strings.TrimSuffix(string(raw), "}") + `,"abort_acknowledged":true}`, strings.TrimSuffix(string(raw), "}") + `,"process_stopped":true}`} {
		var value OctopExecutionEvidence
		if json.Unmarshal([]byte(bad), &value) == nil {
			t.Fatal("non-closed ACP evidence accepted")
		}
	}
}
