package toolinputcontract

import (
	"strings"
	"testing"

	cl "github.com/ai-mada/suma/internal/claudecontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
)

func TestClaudeInputUnionBindsOriginalQuestionAndDecision(t *testing.T) {
	raw := `{"questions":[{"question":"Which exact approach?","header":"Design","options":[{"label":"Keep","description":"Use existing modules"},{"label":"Refine","description":"Improve the interface"}],"multiSelect":false}]}`
	native, err := cl.NewInputRequest(cl.Basis{SessionID: "session192", ToolUseID: "tool192", RequestID: "request192", CallbackID: "host192", ConfigurationSHA256: strings.Repeat("c", 64), ParamsSHA256: cl.PayloadDigest(raw)}, raw)
	if err != nil {
		t.Fatal(err)
	}
	r := ToolInputRequest{IdempotencyKey: "original-question192", Protocol: cl.Protocol, Claude: &native}
	d := &cl.InputDecision{Outcome: "answered", Answers: []cl.InputSelection{{QuestionID: "q1", OptionIDs: []string{"o2"}}}}
	digest, err := AnswerDigest(r, nil, nil, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = AnswerDigest(r, nil, &cu.CursorInputDecision{Outcome: "cancelled"}, d); err == nil {
		t.Fatal("another runtime decision borrowed Claude input")
	}
	r.Cursor = &cu.CursorInputRequest{}
	if ValidateRequest(r) == nil {
		t.Fatal("mixed native question accepted")
	}
	r.Cursor = nil
	v := ToolInput{ID: "input192", ProjectID: "project192", RunID: "run192", NodeID: "node192", WorkerID: "worker192", Attempt: 1, Fence: 1, Version: 2, DispatchID: "dispatch192", ProfileSHA256: strings.Repeat("a", 64), State: "answered", Request: r, Answer: &ToolInputAnswer{Claude: d, AnswerSHA256: digest, ActorID: "reviewer192", AnsweredAt: "2026-11-01T10:00:00Z"}, CreatedAt: "2026-11-01T09:59:00Z", ExpiresAt: "2026-11-01T10:01:00Z", AllowedActions: []string{}, Observation: &delivery.Observation{Sequence: 1, ParamsSHA256: native.ParamsSHA256, AnswerSHA256: digest, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, ExecutionClosed: true}}
	if ValidateInput(v) != nil {
		t.Fatal("original question and its exact answer rejected")
	}
	v.Observation.RequestResolved = true
	if ValidateInput(v) == nil {
		t.Fatal("SDK callback invented request resolution")
	}
	v.Observation.RequestResolved = false
	d.Answers[0].OptionIDs = []string{"missing"}
	if ValidateInput(v) == nil {
		t.Fatal("changed answer retained the original source digest")
	}
}
