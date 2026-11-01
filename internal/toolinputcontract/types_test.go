package toolinputcontract

import (
	"strings"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
)

func TestToolInputUnionSeparatesNativeAnswersAndClosureFacts(t *testing.T) {
	native := cu.CursorInputRequest{RequestID: cu.CursorRPCRequestID{Kind: "string", Value: "request"}, PromptID: cu.CursorRPCRequestID{Kind: "number", Value: "1"}, SessionID: "session", Method: cu.CreatePlanMethod, ConfigurationSHA256: strings.Repeat("a", 64), Plan: &cu.CursorCreatePlan{ToolCallID: "call", Plan: "Exact bounded plan", Todos: []cu.CursorTodo{}}}
	native.ParamsSHA256, _ = cu.ParamsDigest(native)
	r := ToolInputRequest{IdempotencyKey: "operation-190", Protocol: ac.Protocol, Cursor: &native}
	decision := &cu.CursorInputDecision{Outcome: "accepted"}
	if _, err := AnswerDigest(r, nil, decision, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := AnswerDigest(r, []cc.CodexUserInputSelection{{QuestionID: "other", Kind: "skip"}}, decision, nil); err == nil {
		t.Fatal("Cursor accepted a Codex answer")
	}
	r.Codex = &cc.CodexUserInputRequest{}
	if ValidateRequest(r) == nil {
		t.Fatal("mixed source accepted")
	}
	r.Codex = nil
	r.Protocol = cc.Protocol
	if ValidateRequest(r) == nil {
		t.Fatal("Cursor payload borrowed Codex protocol")
	}
	o := cc.CodexUserInputObservation{Sequence: 1, ParamsSHA256: strings.Repeat("b", 64), AnswerSHA256: strings.Repeat("c", 64), AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, RequestResolved: true, TurnClosed: true, FailureCode: "observed_failure"}
	if ToCodexObservation(FromCodexObservation(o)) != o {
		t.Fatal("real native closure facts were lost in the domain projection")
	}
}
