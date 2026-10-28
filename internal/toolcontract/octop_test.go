package toolcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func octopContractProfile() Profile {
	return Profile{CommandID: "octop-fixed", Adapter: "octop", Protocol: OctopProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: OctopHarnessVersion, WorkspaceToolsVersion: "attempt-workspace-v1", OctopExecution: &OctopExecutionCapability{HarnessVersion: OctopHarnessVersion, ACPSDKVersion: OctopACPSDKVersion, ProviderID: "owned", Model: "model-a", EndpointOrigin: "https://models.example", ConfigurationSHA256: strings.Repeat("c", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only"}}
}

func TestOctopFixedCapabilityAndStrictEvidence(t *testing.T) {
	p := octopContractProfile()
	if err := ValidateProfile(p, true); err != nil {
		t.Fatal("fixed official Octop ACP profile rejected", err)
	}
	clone := CloneProfile(p)
	if !EqualProfile(p, clone) {
		t.Fatal("equivalent immutable profile did not match")
	}
	clone.OctopExecution.Model = "other"
	if EqualProfile(p, clone) || p.OctopExecution.Model != "model-a" {
		t.Fatal("Octop configuration was aliased or omitted from equality")
	}
	for name, mutate := range map[string]func(*Profile){
		"foreign":       func(p *Profile) { p.Adapter = "command"; p.Protocol = "text-v1" },
		"version":       func(p *Profile) { p.OctopExecution.HarnessVersion = "other" },
		"sdk":           func(p *Profile) { p.OctopExecution.ACPSDKVersion = "other" },
		"hard-budget":   func(p *Profile) { p.OctopExecution.BudgetMode = "enforce" },
		"readonly":      func(p *Profile) { p.ReadOnly = true },
		"no-host":       func(p *Profile) { p.WorkspaceToolsVersion = "" },
		"native":        func(p *Profile) { p.ModelExecutionProtocol = "suma-model-execution-v1" },
		"extra-purpose": func(p *Profile) { p.IntelligentOperations = &IntelligentOperationsCapability{} },
		"test":          func(p *Profile) { p.TestReportProtocol = "suma-junit-report-v1" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := CloneProfile(p)
			mutate(&bad)
			if ValidateProfile(bad, true) == nil {
				t.Fatal("foreign or unsupported capability accepted")
			}
		})
	}
	proof := &OctopExecutionEvidence{ConfigurationSHA256: p.OctopExecution.ConfigurationSHA256, SessionID: "session_original", RequestedProvider: "owned", RequestedModel: "model-a", AcknowledgedProvider: "owned", AcknowledgedModel: "model-a", StopReason: "end_turn", CompletedAt: "2026-10-28T00:00:00Z", ProcessStopped: true}
	e := Evidence{Profile: p, SessionID: proof.SessionID, Terminal: "completed", EventCount: 3, UsageState: UsageUnreported, OctopExecution: proof}
	if err := ValidateEvidence(e); err != nil {
		t.Fatal("correlated stopped Octop completion rejected", err)
	}
	for name, mutate := range map[string]func(*Evidence){
		"end-turn-without-stop": func(e *Evidence) { e.OctopExecution.ProcessStopped = false },
		"cancel":                func(e *Evidence) { e.OctopExecution.CancelRequested = true },
		"refusal":               func(e *Evidence) { e.OctopExecution.StopReason = "refusal" },
		"no-proof":              func(e *Evidence) { e.OctopExecution = nil },
		"session":               func(e *Evidence) { e.OctopExecution.SessionID = "foreign" },
		"provider":              func(e *Evidence) { e.OctopExecution.AcknowledgedProvider = "foreign" },
		"timestamp":             func(e *Evidence) { e.OctopExecution.CompletedAt = "2026-10-28T08:00:00+08:00" },
		"complete-usage":        func(e *Evidence) { e.UsageState = UsageComplete },
	} {
		t.Run(name, func(t *testing.T) {
			bad := e
			value := *proof
			bad.OctopExecution = &value
			mutate(&bad)
			if ValidateEvidence(bad) == nil {
				t.Fatal("invalid completion/identity accepted")
			}
		})
	}
	for _, raw := range []string{
		`{"harness_version":"1.0.5","harness_version":"1.0.5"}`,
		`{"HarnessVersion":"1.0.5"}`, `{"harness_version":null}`, `null`,
	} {
		var value OctopExecutionCapability
		if json.Unmarshal([]byte(raw), &value) == nil {
			t.Fatal("non-exact capability JSON accepted", raw)
		}
	}
	raw, _ := json.Marshal(proof)
	for _, bad := range []string{strings.Replace(string(raw), `"process_stopped":true`, `"process_stopped":null`, 1), strings.Replace(string(raw), `"process_stopped":true`, `"ProcessStopped":true`, 1), strings.TrimSuffix(string(raw), "}") + `,"abort_acknowledged":true}`, strings.TrimSuffix(string(raw), "}") + `,"process_stopped":true}`} {
		var value OctopExecutionEvidence
		if json.Unmarshal([]byte(bad), &value) == nil {
			t.Fatal("non-exact evidence JSON accepted", bad)
		}
	}
}
