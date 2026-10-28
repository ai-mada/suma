package toolcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

// This inventory is a local SDK/configuration probe, never a provider health
// result. The real decoder must retain its fixed model and host-tool contract.
func openCodeWireProfile(t *testing.T) Profile {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{
		"command_id": "opencode", "adapter": "opencode", "protocol": "opencode-sdk-v1",
		"profile_sha256": strings.Repeat("a", 64), "executable_sha256": strings.Repeat("b", 64),
		"tool_version": "1.19.4", "read_only": false, "workspace_tools_version": "attempt-workspace-v1",
		"opencode_execution": map[string]any{
			"sdk_version": "1.19.4", "server_version": "1.19.4", "provider_id": "owned", "model": "model-a",
			"endpoint_origin": "https://provider.example", "configuration_sha256": strings.Repeat("c", 64),
			"budget_mode": "observe_only", "identity_evidence": "configuration_ack_only",
		},
	})
	var profile Profile
	if err := json.Unmarshal(raw, &profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestOpenCodeSDKInventoryRetainsFixedModelAndRequiresExactSupportedProtocol(t *testing.T) {
	p := openCodeWireProfile(t)
	if err := ValidateProfile(p, true); err != nil {
		t.Fatal("fixed SDK/host-workspace profile rejected", err)
	}
	raw, _ := json.Marshal(p)
	var wire map[string]json.RawMessage
	_ = json.Unmarshal(raw, &wire)
	if len(wire["opencode_execution"]) == 0 {
		t.Fatal("fixed administrator model was discarded by the inventory wire")
	}
	p.Protocol = "opencode-jsonl-v1"
	if ValidateProfile(p, true) == nil || ValidProtocol(p.Protocol) {
		t.Fatal("unsupported whole-run JSONL protocol retained as a current alias")
	}
}

func TestOpenCodeProfileRejectsUnprobedOrForeignCapabilities(t *testing.T) {
	p := openCodeWireProfile(t)
	for name, mutate := range map[string]func(*Profile){
		"missing-sdk":  func(p *Profile) { p.OpenCodeExecution = nil },
		"version":      func(p *Profile) { p.OpenCodeExecution.SDKVersion = "1.19.3" },
		"server":       func(p *Profile) { p.OpenCodeExecution.ServerVersion = "1.19.3" },
		"provider":     func(p *Profile) { p.OpenCodeExecution.ProviderID = "../provider" },
		"model":        func(p *Profile) { p.OpenCodeExecution.Model = "model with space" },
		"private-url":  func(p *Profile) { p.OpenCodeExecution.EndpointOrigin = "https://provider.example/private?token=secret" },
		"digest":       func(p *Profile) { p.OpenCodeExecution.ConfigurationSHA256 = strings.Repeat("A", 64) },
		"hard-budget":  func(p *Profile) { p.OpenCodeExecution.BudgetMode = "enforce" },
		"identity":     func(p *Profile) { p.OpenCodeExecution.IdentityEvidence = "provider_attested" },
		"host-tools":   func(p *Profile) { p.WorkspaceToolsVersion = "" },
		"readonly":     func(p *Profile) { p.ReadOnly = true },
		"native-model": func(p *Profile) { p.ModelExecutionProtocol = "suma-model-execution-v1" },
		"repair":       func(p *Profile) { p.ConflictRepairProtocol = "suma.candidate-conflict-repair.v1" },
		"planning":     func(p *Profile) { p.Planning = &PlanningCapability{} },
		"knowledge":    func(p *Profile) { p.KnowledgeExtraction = &KnowledgeExtractionCapability{} },
		"diagnosis":    func(p *Profile) { p.FailureDiagnosis = &FailureDiagnosisCapability{} },
		"foreign":      func(p *Profile) { p.Adapter, p.Protocol = "suma", "suma-jsonl-v1" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := CloneProfile(p)
			mutate(&bad)
			if ValidateProfile(bad, true) == nil {
				t.Fatal("unsupported SDK capability accepted")
			}
		})
	}
	clone := CloneProfile(p)
	if !EqualProfile(p, clone) {
		t.Fatal("value-equivalent fixed configuration did not match")
	}
	clone.OpenCodeExecution.Model = "model-b"
	if EqualProfile(p, clone) || p.OpenCodeExecution.Model != "model-a" {
		t.Fatal("fixed configuration was aliased or ignored by comparison")
	}
	probe := Observation{Profile: p, Available: true}
	if ValidateObservation(probe) != nil {
		t.Fatal("unadvertised transport rejected")
	}
	probe.WorkspaceObservationProtocol, probe.WorkspaceWriteProtocol = testWorkspaceObservationProtocol, testWorkspaceWriteProtocol
	if ValidateObservation(probe) != nil {
		t.Fatal("actual host read/write transport rejected")
	}
	probe.Available = false
	probe.Reason = "probe unavailable"
	if ValidateObservation(probe) == nil {
		t.Fatal("unavailable command advertised host transport")
	}
}

func TestOpenCodeWholeTurnRequiresCorrelatedFinalAndStoppedPrivateServer(t *testing.T) {
	p := openCodeWireProfile(t)
	proof := &OpenCodeExecutionEvidence{ConfigurationSHA256: strings.Repeat("c", 64), SessionID: "ses_original", UserMessageID: "msg_user", AssistantMessageID: "msg_assistant", RequestedProvider: "owned", RequestedModel: "model-a", AcknowledgedProvider: "owned", AcknowledgedModel: "model-a", Finish: "stop", CompletedAt: "2026-10-28T00:00:00Z", ServerStopped: true}
	e := Evidence{Profile: p, SessionID: "ses_original", Terminal: "completed", EventCount: 4, UsageState: UsageUnreported, OpenCodeExecution: proof}
	if err := ValidateEvidence(e); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Evidence){
		"no-proof":        func(e *Evidence) { e.OpenCodeExecution = nil },
		"step-finish":     func(e *Evidence) { e.OpenCodeExecution.Finish = "step_finish" },
		"length":          func(e *Evidence) { e.OpenCodeExecution.Finish = "length" },
		"no-time":         func(e *Evidence) { e.OpenCodeExecution.CompletedAt = "" },
		"foreign-time":    func(e *Evidence) { e.OpenCodeExecution.CompletedAt = "2026-10-28T08:00:00+08:00" },
		"no-ack":          func(e *Evidence) { e.OpenCodeExecution.AcknowledgedModel = "" },
		"wrong-model":     func(e *Evidence) { e.OpenCodeExecution.AcknowledgedModel = "other" },
		"wrong-provider":  func(e *Evidence) { e.OpenCodeExecution.AcknowledgedProvider = "other" },
		"wrong-config":    func(e *Evidence) { e.OpenCodeExecution.ConfigurationSHA256 = strings.Repeat("d", 64) },
		"foreign-session": func(e *Evidence) { e.SessionID = "ses_other" },
		"same-message":    func(e *Evidence) { e.OpenCodeExecution.AssistantMessageID = "msg_user" },
		"no-user":         func(e *Evidence) { e.OpenCodeExecution.UserMessageID = "" },
		"server-live":     func(e *Evidence) { e.OpenCodeExecution.ServerStopped = false },
		"abort":           func(e *Evidence) { e.OpenCodeExecution.AbortAcknowledged = true },
		"complete-usage":  func(e *Evidence) { e.UsageState = UsageComplete },
	} {
		t.Run(name, func(t *testing.T) {
			copy := e
			fact := *proof
			copy.OpenCodeExecution = &fact
			mutate(&copy)
			if ValidateEvidence(copy) == nil {
				t.Fatal("incomplete/mismatched whole-turn success accepted")
			}
		})
	}
	startup := Evidence{Profile: p, Terminal: "unknown", UsageState: UsageUnreported, OpenCodeExecution: &OpenCodeExecutionEvidence{ConfigurationSHA256: strings.Repeat("c", 64), RequestedProvider: "owned", RequestedModel: "model-a", ServerStopped: true}}
	if err := ValidateEvidence(startup); err != nil {
		t.Fatal("pre-session stop required invented session/abort facts", err)
	}
	startup.OpenCodeExecution.AbortAcknowledged = true
	if ValidateEvidence(startup) == nil {
		t.Fatal("abort acknowledged without an original session")
	}
	failed := e
	failed.Terminal = "failed"
	fact := *proof
	failed.OpenCodeExecution = &fact
	fact.AcknowledgedModel = "other"
	fact.Finish = "length"
	if err := ValidateEvidence(failed); err != nil {
		t.Fatal("original mismatch/failure facts lost", err)
	}
}

func TestOpenCodeNestedWireRejectsAliasesDuplicatesUnknownAndNullFacts(t *testing.T) {
	raw, _ := json.Marshal(openCodeWireProfile(t).OpenCodeExecution)
	for _, value := range []string{
		strings.Replace(string(raw), `"sdk_version"`, `"SDK_VERSION"`, 1),
		strings.Replace(string(raw), `"sdk_version":"1.19.4"`, `"sdk_version":"1.19.4","sdk_version":"1.19.4"`, 1),
		strings.Replace(string(raw), `"sdk_version":"1.19.4"`, `"sdk_version":null`, 1),
		strings.Replace(string(raw), `"sdk_version":"1.19.4"`, `"sdk_version":"1.19.4","secret":"hidden"`, 1),
	} {
		var c OpenCodeExecutionCapability
		if json.Unmarshal([]byte(value), &c) == nil {
			t.Fatal("noncanonical capability decoded")
		}
	}
	var proof OpenCodeExecutionEvidence
	if json.Unmarshal([]byte(`{"server_stopped":null}`), &proof) == nil || json.Unmarshal([]byte(`{"Server_Stopped":true}`), &proof) == nil {
		t.Fatal("null or aliased stop proof accepted")
	}
}

func TestOpenCodeResultStopSeparatesAbortAcknowledgementFromQuiescence(t *testing.T) {
	p := openCodeWireProfile(t)
	e := Evidence{Profile: p, SessionID: "ses_original", Terminal: "unknown", UsageState: UsageUnreported, OpenCodeExecution: &OpenCodeExecutionEvidence{ConfigurationSHA256: strings.Repeat("c", 64), RequestedProvider: "owned", RequestedModel: "model-a", SessionID: "ses_original", UserMessageID: "msg_user", ServerStopped: true, AbortAcknowledged: true}}
	if err := ValidateOpenCodeResultStop(e, true, false); err != nil {
		t.Fatal("confirmed original abort/tree stop rejected", err)
	}
	e.OpenCodeExecution.AbortAcknowledged = false
	if ValidateOpenCodeResultStop(e, true, false) == nil {
		t.Fatal("post-session stop without abort ACK accepted")
	}
	if err := ValidateOpenCodeResultStop(e, true, true); err != nil {
		t.Fatal("unknown original abort outcome lost", err)
	}
	e.OpenCodeExecution.ServerStopped = false
	if ValidateOpenCodeResultStop(e, true, true) == nil || ValidateOpenCodeResultStop(e, false, false) == nil {
		t.Fatal("live private server became a definitive stopped/failed result")
	}
	if err := ValidateOpenCodeResultStop(e, false, true); err != nil {
		t.Fatal("unconfirmed private process lost reconciliation facts", err)
	}
	e.SessionID = ""
	e.OpenCodeExecution.SessionID = ""
	e.OpenCodeExecution.UserMessageID = ""
	e.OpenCodeExecution.ServerStopped = true
	if err := ValidateOpenCodeResultStop(e, true, false); err != nil {
		t.Fatal("pre-session stop required invented abort", err)
	}
}
