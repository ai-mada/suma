package runner

import (
	"encoding/json"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const octopReadyFixture = `{"v":1,"type":"ready","request_id":"req117","harness_version":"1.0.5","acp_sdk_version":"0.12.1","configuration_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","provider_id":"local117","model":"coding117"}`
const octopStartedFixture = `{"v":1,"type":"started","request_id":"req117","session_id":"session117"}`
const octopCompletedFixture = `{"v":1,"type":"completed","request_id":"req117","session_id":"session117","provider_id":"local117","model":"coding117","stop_reason":"end_turn","completed_at":"2026-10-28T00:00:00Z","agent_parent_stopped":true,"usage_state":"unreported"}`

func octopFixtureProfile(t *testing.T) tc.Profile {
	t.Helper()
	return tc.Profile{CommandID: "octop117", Adapter: "octop", Protocol: tc.OctopProtocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "suma-octop-runtime 1.0.0", WorkspaceToolsVersion: NativeWorkspaceVersion, OctopExecution: &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion, ProviderID: "local117", Model: "coding117", EndpointOrigin: "http://127.0.0.1:49117", ConfigurationSHA256: strings.Repeat("c", 64), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only"}}
}

func octopFixture(t *testing.T, verified bool, frames ...string) (*tc.Evidence, error) {
	t.Helper()
	s := newToolStream(octopFixtureProfile(t), nil, nil, "req117")
	for _, frame := range frames {
		_, _ = s.Write([]byte(frame + "\n"))
	}
	e, err := s.finish()
	applyOctopProcessContainment(e, s.octopState != nil && s.octopState.parentStopped, verified)
	return e, err
}

// The ACP result is insufficient until the independent Go process controller
// has confirmed the entire attempt tree. Missing usage remains missing.
func TestOctopStreamSeparatesACPAndHostStop(t *testing.T) {
	e, err := octopFixture(t, true, octopReadyFixture, octopStartedFixture, octopCompletedFixture)
	if err != nil || e.Terminal != "completed" || e.Usage != nil || e.UsageState != tc.UsageUnreported || tc.ValidateEvidence(*e) != nil {
		t.Fatalf("valid fixed execution rejected: %+v %v", e, err)
	}
	e, err = octopFixture(t, false, octopReadyFixture, octopStartedFixture, octopCompletedFixture)
	if err != nil || e.Terminal != "unknown" || e.OctopExecution.ProcessStopped {
		t.Fatalf("ACP parent-close became host containment: %+v %v", e, err)
	}
	cancel := `{"v":1,"type":"cancelled","request_id":"req117","session_id":"session117","cancel_requested":true,"agent_parent_stopped":true}`
	e, err = octopFixture(t, true, octopReadyFixture, octopStartedFixture, cancel)
	if err != nil || e.Terminal != "failed" || !e.OctopExecution.CancelRequested || tc.ValidateOctopResultStop(*e, true, false) != nil {
		t.Fatalf("cancel intent/host stop correlation lost: %+v %v", e, err)
	}
}

func TestOctopStreamRejectsFalseSuccess(t *testing.T) {
	for name, end := range map[string]string{
		"session":   strings.Replace(octopCompletedFixture, "session117", "other117", 1),
		"reason":    strings.Replace(octopCompletedFixture, "end_turn", "cancelled", 1),
		"parent":    strings.Replace(octopCompletedFixture, "true", "false", 1),
		"usage":     strings.Replace(octopCompletedFixture, `"usage_state":"unreported"`, `"usage_state":"complete","usage":{"input_tokens":0,"output_tokens":0}`, 1),
		"extra":     strings.Replace(octopCompletedFixture, `"stop_reason":"end_turn"`, `"stop_reason":"end_turn","auto_resume":true`, 1),
		"duplicate": strings.Replace(octopCompletedFixture, `"stop_reason":"end_turn"`, `"stop_reason":"end_turn","stop_reason":"end_turn"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			e, err := octopFixture(t, true, octopReadyFixture, octopStartedFixture, end)
			if err == nil || e.Terminal != "unknown" {
				t.Fatalf("unproven completion accepted: %+v %v", e, err)
			}
		})
	}
}

func TestOctopJournalRetainsOriginalConfiguration(t *testing.T) {
	e, _ := octopFixture(t, true, octopReadyFixture, octopStartedFixture, octopCompletedFixture)
	raw, err := json.Marshal(e)
	var restored tc.Evidence
	if err != nil || json.Unmarshal(raw, &restored) != nil || restored.OctopExecution == nil || *restored.OctopExecution != *e.OctopExecution {
		t.Fatal("durable evidence lost original ACP facts")
	}
	copy := cloneOctopEvidence(e.OctopExecution)
	copy.SessionID = "other"
	if e.OctopExecution.SessionID != "session117" {
		t.Fatal("caller-owned evidence mutated durable identity")
	}
}
