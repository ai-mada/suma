package runner

import (
	"encoding/json"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const openCodeReadyFixture = `{"v":1,"type":"ready","request_id":"req-104","sdk_version":"1.19.4","server_version":"1.19.4","configuration_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","provider_id":"local104","model":"coding104"}`
const openCodeStartedFixture = `{"v":1,"type":"started","request_id":"req-104","session_id":"ses_104","user_message_id":"msg_user104"}`
const openCodeCompletedFixture = `{"v":1,"type":"completed","request_id":"req-104","session_id":"ses_104","user_message_id":"msg_user104","assistant_message_id":"msg_assistant104","provider_id":"local104","model":"coding104","finish":"stop","completed_at":"2026-10-28T00:00:00Z","server_stopped":true,"usage_state":"unreported"}`

func openCodeFixtureProfile(t *testing.T) tc.Profile {
	t.Helper()
	var profile tc.Profile
	raw := `{"command_id":"opencode104","adapter":"opencode","protocol":"opencode-sdk-v1","profile_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","executable_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","tool_version":"suma-opencode-runtime 1.0.0","read_only":false,"workspace_tools_version":"attempt-workspace-v1","opencode_execution":{"sdk_version":"1.19.4","server_version":"1.19.4","provider_id":"local104","model":"coding104","endpoint_origin":"http://127.0.0.1:49104","configuration_sha256":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","budget_mode":"observe_only","identity_evidence":"configuration_ack_only"}}`
	if err := json.Unmarshal([]byte(raw), &profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func decodeOpenCodeFixture(t *testing.T, frames ...string) (*tc.Evidence, []tc.Event, error) {
	t.Helper()
	var events []tc.Event
	stream := newToolStream(openCodeFixtureProfile(t), []string{"private104"}, func(event tc.Event) error {
		events = append(events, event)
		return nil
	}, "req-104")
	for _, frame := range frames {
		_, _ = stream.Write([]byte(frame + "\n"))
	}
	evidence, err := stream.finish()
	return evidence, events, err
}

// This catches accepting an exit/SSE/step-finish without the exact final
// assistant, or discarding the fixed configuration and stop provenance.
func TestOpenCodeWrapperStreamRequiresCorrelatedFinalAssistant(t *testing.T) {
	evidence, events, err := decodeOpenCodeFixture(t, openCodeReadyFixture, openCodeStartedFixture, openCodeCompletedFixture)
	if err != nil || evidence.Terminal != "completed" || evidence.SessionID != "ses_104" || evidence.UsageState != tc.UsageUnreported || evidence.Usage != nil {
		t.Fatalf("correlated whole-run proof rejected: %+v %v", evidence, err)
	}
	if err = tc.ValidateEvidence(*evidence); err != nil {
		t.Fatalf("shared evidence rejected: %v", err)
	}
	raw, _ := json.Marshal(evidence)
	var encoded map[string]any
	_ = json.Unmarshal(raw, &encoded)
	proof, ok := encoded["opencode_execution"].(map[string]any)
	if !ok || proof["configuration_sha256"] != strings.Repeat("c", 64) || proof["user_message_id"] != "msg_user104" || proof["assistant_message_id"] != "msg_assistant104" || proof["server_stopped"] != true || proof["acknowledged_model"] != "coding104" {
		t.Fatalf("fixed configuration/assistant/stop facts lost: %s", raw)
	}
	if len(events) < 3 || events[0].Kind != "session_started" || events[1].Kind != "turn_started" || events[len(events)-1].Kind != "turn_completed" {
		t.Fatalf("missing typed progress: %+v", events)
	}
}

// Each mutation removes an independent success prerequisite. Unknown extra
// fields are refused so an SDK upgrade cannot silently broaden this protocol.
func TestOpenCodeWrapperStreamRejectsUnprovenCompletion(t *testing.T) {
	for name, frames := range map[string][]string{
		"no-terminal":             {openCodeReadyFixture, openCodeStartedFixture},
		"step-only":               {openCodeReadyFixture, openCodeStartedFixture, `{"v":1,"type":"step_finish","request_id":"req-104"}`},
		"missing-ready":           {openCodeStartedFixture, openCodeCompletedFixture},
		"foreign-request":         {strings.Replace(openCodeReadyFixture, "req-104", "req-other", 1), openCodeStartedFixture, openCodeCompletedFixture},
		"sdk-drift":               {strings.Replace(openCodeReadyFixture, "1.19.4", "1.19.5", 1), openCodeStartedFixture, openCodeCompletedFixture},
		"config-drift":            {strings.Replace(openCodeReadyFixture, strings.Repeat("c", 64), strings.Repeat("d", 64), 1), openCodeStartedFixture, openCodeCompletedFixture},
		"session-drift":           {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, "ses_104", "ses_other", 1)},
		"foreign-parent":          {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, "msg_user104", "msg_other", 1)},
		"provider-drift":          {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, "local104", "other104", 1)},
		"model-drift":             {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, "coding104", "other104", 1)},
		"unfinished-tool-loop":    {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, `"finish":"stop"`, `"finish":"tool-calls"`, 1)},
		"missing-time":            {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, `,"completed_at":"2026-10-28T00:00:00Z"`, "", 1)},
		"unconfirmed-server":      {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, `"server_stopped":true`, `"server_stopped":false`, 1)},
		"extra-field":             {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, `"finish":"stop"`, `"finish":"stop","arbitrary":"payload"`, 1)},
		"duplicate-terminal":      {openCodeReadyFixture, openCodeStartedFixture, openCodeCompletedFixture, openCodeCompletedFixture},
		"usage-null":              {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, `"usage_state":"unreported"`, `"usage_state":"partial","usage":{"input_tokens":null}`, 1)},
		"invented-complete-usage": {openCodeReadyFixture, openCodeStartedFixture, strings.Replace(openCodeCompletedFixture, `"usage_state":"unreported"`, `"usage_state":"complete","usage":{"input_tokens":0,"output_tokens":0}`, 1)},
	} {
		t.Run(name, func(t *testing.T) {
			evidence, _, err := decodeOpenCodeFixture(t, frames...)
			if err == nil || evidence.Terminal != "unknown" {
				t.Fatalf("unproven success accepted: %+v %v", evidence, err)
			}
		})
	}
}

func TestOpenCodeWrapperUsagePreservesPartialMeasuredZero(t *testing.T) {
	completed := strings.Replace(openCodeCompletedFixture, `"usage_state":"unreported"`, `"usage_state":"partial","usage":{"input_tokens":0,"output_tokens":2,"cached_input_tokens":0}`, 1)
	evidence, _, err := decodeOpenCodeFixture(t, openCodeReadyFixture, openCodeStartedFixture, completed)
	if err != nil || evidence.UsageState != tc.UsagePartial || evidence.Usage == nil || evidence.Usage.InputTokens == nil || *evidence.Usage.InputTokens != 0 || evidence.Usage.OutputTokens == nil || *evidence.Usage.OutputTokens != 2 || evidence.Usage.CacheWriteInputTokens != nil {
		t.Fatalf("partial source counters became missing/complete/zero: %+v %v", evidence, err)
	}
}

func TestOpenCodeJournalKeepsOriginalProofAcrossLostACK(t *testing.T) {
	evidence, _, err := decodeOpenCodeFixture(t, openCodeReadyFixture, openCodeStartedFixture, openCodeCompletedFixture)
	if err != nil {
		t.Fatal(err)
	}
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	a := Assignment{RunID: "journal104", NodeID: "develop", WorkerID: "worker104", Attempt: 1, Fence: 1, Config: map[string]any{"opencode_execution": evidence.Profile.OpenCodeExecution}}
	if err = journal.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = journal.Finish(Result{Assignment: a, ExitCode: 0, ToolEvidence: evidence}); err != nil {
		t.Fatal(err)
	}
	evidence.OpenCodeExecution.AssistantMessageID = "mutated"
	evidence.Profile.OpenCodeExecution.Model = "mutated"
	first := journal.Pending()
	if len(first) != 1 || first[0].ToolEvidence.OpenCodeExecution.AssistantMessageID != "msg_assistant104" || first[0].ToolEvidence.Profile.OpenCodeExecution.Model != "coding104" {
		t.Fatal("caller changed durable original receipt")
	}
	first[0].ToolEvidence.OpenCodeExecution.AssistantMessageID = "reader-mutated"
	second := journal.Pending()
	if second[0].ToolEvidence.OpenCodeExecution.AssistantMessageID != "msg_assistant104" || second[0].OperationID != first[0].OperationID {
		t.Fatal("lost ACK replay changed original proof/operation")
	}
}

func TestOpenCodeProcessContainmentErrorRevokesServerStopProof(t *testing.T) {
	for _, verified := range []bool{true, false} {
		t.Run(map[bool]string{true: "verified", false: "unconfirmed"}[verified], func(t *testing.T) {
			evidence, _, err := decodeOpenCodeFixture(t, openCodeReadyFixture, openCodeStartedFixture, openCodeCompletedFixture)
			if err != nil {
				t.Fatal(err)
			}
			applyOpenCodeProcessContainment(evidence, verified)
			if verified {
				if !evidence.OpenCodeExecution.ServerStopped || evidence.Terminal != "completed" {
					t.Fatal("lost a confirmed private stop")
				}
			} else {
				if evidence.OpenCodeExecution.ServerStopped || evidence.Terminal != "unknown" {
					t.Fatal("wrapper-only ACK survived failed host containment")
				}
				if err := tc.ValidateOpenCodeResultStop(*evidence, true, false); err == nil {
					t.Fatal("unconfirmed containment could settle a stopped result")
				}
			}
		})
	}
}
