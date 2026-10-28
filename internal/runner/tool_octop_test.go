package runner

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	mg "github.com/ai-mada/suma/internal/modelgateway"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The marker is replaced with a real canonical ModelBinding digest before the
// stream parser sees the frame. No SDK configuration or usage is authoritative.
const octopReadyFixture = `{"v":1,"type":"ready","request_id":"req117","harness_version":"1.0.5","acp_sdk_version":"0.12.1","binding_sha256":"BOUND_MODEL_SHA"}`
const octopStartedFixture = `{"v":1,"type":"started","request_id":"req117","session_id":"session117"}`
const octopCompletedFixture = `{"v":1,"type":"completed","request_id":"req117","session_id":"session117","binding_sha256":"BOUND_MODEL_SHA","stop_reason":"end_turn","completed_at":"2026-10-28T00:00:00Z","agent_parent_stopped":true}`

func octopModelBinding(t *testing.T, endpoint string) mc.Binding {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := mc.EndpointDigest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	deployment := mc.Deployment{ID: "octop-model", Revision: 1, Protocol: mc.ProviderProtocol, Model: "coding117", EndpointOrigin: parsed.Scheme + "://" + parsed.Host, EndpointConfigSHA256: sha, ContextWindow: 8192}
	deployment.SHA256 = mc.DeploymentDigest(deployment)
	binding, err := mc.NewBinding(deployment, mc.Parameters{MaxSteps: 8, MaxOutputTokens: 512}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func octopFixtureProfile(t *testing.T) tc.Profile {
	t.Helper()
	return tc.Profile{CommandID: "octop117", Adapter: "octop", Protocol: tc.OctopProtocol, ModelExecutionProtocol: mc.Protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "suma-octop-runtime 1.0.0", WorkspaceToolsVersion: NativeWorkspaceVersion, OctopExecution: &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion}}
}

func octopFixture(t *testing.T, verified bool, frames ...string) (*tc.Evidence, error) {
	t.Helper()
	binding := octopModelBinding(t, "https://models.example/v1")
	s := newToolStream(octopFixtureProfile(t), nil, nil, "req117")
	s.modelBinding = &binding
	for _, frame := range frames {
		_, _ = s.Write([]byte(strings.ReplaceAll(frame, "BOUND_MODEL_SHA", binding.BindingSHA256) + "\n"))
	}
	e, err := s.finish()
	applyOctopProcessContainment(e, s.octopState != nil && s.octopState.parentStopped, verified)
	// This is a host-receipt unit fixture, not an SDK usage frame. Real HTTP,
	// persistence and the process controller are covered by the opt-in journey.
	input, output := int64(7), int64(3)
	request := mc.RequestEvidence{State: "completed", Step: 0, RequestSHA256: strings.Repeat("d", 64), ResponseSHA256: strings.Repeat("e", 64), ProviderModel: binding.Deployment.Model, ProviderRequestID: "provider117", OutputAllowance: 512, UsageState: tc.UsageComplete, Usage: &mc.RequestUsage{InputTokens: &input, OutputTokens: &output}}
	if e.OctopExecution != nil && e.OctopExecution.CancelRequested {
		request = mc.RequestEvidence{State: "unknown", Step: 0, RequestSHA256: strings.Repeat("d", 64), OutputAllowance: 512, UsageState: tc.UsageUnreported}
	}
	applyModelGatewayEvidence(e, mg.Snapshot{Version: 1, RequestID: "req117", Binding: binding, Closed: true, Records: []mg.Record{{RequestEvidence: request, ClientSHA256: strings.Repeat("f", 64)}}})
	return e, err
}

// ACP completion, authoritative host usage and independent process containment
// are separate facts. Cancelled upstream requests retain unknown usage.
func TestOctopStreamSeparatesACPAndHostStop(t *testing.T) {
	e, err := octopFixture(t, true, octopReadyFixture, octopStartedFixture, octopCompletedFixture)
	if err != nil || e.Terminal != "completed" || e.Usage == nil || e.UsageState != tc.UsageComplete || e.ModelExecution == nil || !e.ModelExecution.GatewayClosed || tc.ValidateEvidence(*e) != nil {
		t.Fatalf("valid host-bound execution rejected: %+v %v", e, err)
	}
	e, err = octopFixture(t, false, octopReadyFixture, octopStartedFixture, octopCompletedFixture)
	if err != nil || e.Terminal != "unknown" || e.OctopExecution.ProcessStopped {
		t.Fatalf("ACP parent-close became host containment: %+v %v", e, err)
	}
	cancel := `{"v":1,"type":"cancelled","request_id":"req117","session_id":"session117","cancel_requested":true,"agent_parent_stopped":true}`
	e, err = octopFixture(t, true, octopReadyFixture, octopStartedFixture, cancel)
	if err != nil || e.Terminal != "failed" || !e.OctopExecution.CancelRequested || e.UsageState == tc.UsageComplete || tc.ValidateOctopResultStop(*e, true, false) != nil {
		t.Fatalf("cancel intent/host stop correlation lost: %+v %v", e, err)
	}
}

func TestOctopStreamRejectsFalseSuccess(t *testing.T) {
	for name, end := range map[string]string{
		"session":   strings.Replace(octopCompletedFixture, "session117", "other117", 1),
		"binding":   strings.Replace(octopCompletedFixture, "BOUND_MODEL_SHA", strings.Repeat("b", 64), 1),
		"reason":    strings.Replace(octopCompletedFixture, "end_turn", "cancelled", 1),
		"parent":    strings.Replace(octopCompletedFixture, "true", "false", 1),
		"usage":     strings.Replace(octopCompletedFixture, `"stop_reason":"end_turn"`, `"stop_reason":"end_turn","usage_state":"complete","usage":{"input_tokens":0,"output_tokens":0}`, 1),
		"provider":  strings.Replace(octopCompletedFixture, `"stop_reason":"end_turn"`, `"stop_reason":"end_turn","provider_id":"supplier","model":"coding117"`, 1),
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

func TestOctopJournalRetainsOriginalBindingAndACP(t *testing.T) {
	e, _ := octopFixture(t, true, octopReadyFixture, octopStartedFixture, octopCompletedFixture)
	raw, err := json.Marshal(e)
	var restored tc.Evidence
	if err != nil || json.Unmarshal(raw, &restored) != nil || restored.OctopExecution == nil || *restored.OctopExecution != *e.OctopExecution || restored.ModelExecution == nil || restored.ModelExecution.BindingSHA256 != e.ModelExecution.BindingSHA256 {
		t.Fatal("durable evidence lost original binding/ACP facts")
	}
	copy := cloneOctopEvidence(e.OctopExecution)
	copy.SessionID = "other"
	if e.OctopExecution.SessionID != "session117" {
		t.Fatal("caller-owned evidence mutated durable identity")
	}
}
