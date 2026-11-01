package runner

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func inputAnswer187(t *testing.T, r cc.CodexUserInputRequest, selections []cc.CodexUserInputSelection) *ic.ToolInputAnswer {
	t.Helper()
	sha, err := cc.UserInputAnswerDigest(r, selections)
	if err != nil {
		t.Fatal(err)
	}
	return &ic.ToolInputAnswer{Answers: selections, AnswerSHA256: sha, ActorID: "human187", AnsweredAt: "2026-11-01T00:00:01Z"}
}
func inputDriver187(t *testing.T, h codexUserInputHandler) *codexAppServer {
	t.Helper()
	d := codexApprovalTestDriver(t, codexApprovalHandler{})
	d.spec.CodexAppServer.EnableUserInput = true
	d.stream.profile.UserInputProtocol = cc.UserInputProtocol
	if h.Open == nil {
		h.Open = func(context.Context, cc.CodexUserInputRequest) (string, error) { return "input187", nil }
	}
	if h.Answer == nil {
		h.Answer = func(_ context.Context, r cc.CodexUserInputRequest, _ string) (*ic.ToolInputAnswer, error) {
			return inputAnswer187(t, r, []cc.CodexUserInputSelection{{QuestionID: r.Questions[0].ID, Kind: "text", Text: "reviewed answer"}}), nil
		}
	}
	if h.BeforeWrite == nil {
		h.BeforeWrite = func(context.Context, cc.CodexUserInputRequest, string, ic.ToolInputAnswer, cc.CodexUserInputObservation) error {
			return nil
		}
	}
	if h.Observe == nil {
		h.Observe = func(context.Context, cc.CodexUserInputRequest, string, cc.CodexUserInputObservation) error {
			return nil
		}
	}
	if err := d.configureUserInputs(context.Background(), h); err != nil {
		t.Fatal(err)
	}
	return d
}
func inputParams187(d *codexAppServer) map[string]any {
	return map[string]any{"threadId": d.thread, "turnId": d.turn, "itemId": "question-item187", "isBlocking": true, "questions": []any{map[string]any{"id": "scope", "header": "Scope", "question": "Which reviewed change should continue?"}}}
}
func inputAck187(t *testing.T, d *codexAppServer) {
	codexApprovalFrame(t, d, map[string]any{"id": 4, "result": map[string]any{"turn": map[string]any{"id": d.turn, "status": "inProgress"}}})
}
func inputTerminal187(t *testing.T, d *codexAppServer) {
	codexApprovalFrame(t, d, map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": d.thread, "turn": map[string]any{"id": d.turn, "status": "completed"}}})
}
func inputProof187(d *codexAppServer) []cc.CodexUserInputEvidence {
	d.mu.Lock()
	defer d.mu.Unlock()
	return cc.CloneEvidence(d.evidence).UserInputs
}

func TestCodexUserInputLiveTurnWritesExactAnswerAndKeepsLateFacts(t *testing.T) {
	var opened atomic.Int32
	var intent atomic.Bool
	var mu sync.Mutex
	var observations []cc.CodexUserInputObservation
	h := codexUserInputHandler{
		Open: func(context.Context, cc.CodexUserInputRequest) (string, error) { opened.Add(1); return "input187", nil },
		Answer: func(_ context.Context, r cc.CodexUserInputRequest, _ string) (*ic.ToolInputAnswer, error) {
			return inputAnswer187(t, r, []cc.CodexUserInputSelection{{QuestionID: "color", Kind: "option", Option: "Blue", Text: "keep the API"}, {QuestionID: "scope", Kind: "text", Text: "bounded goal"}, {QuestionID: "skip", Kind: "skip"}}), nil
		},
		BeforeWrite: func(_ context.Context, _ cc.CodexUserInputRequest, _ string, _ ic.ToolInputAnswer, o cc.CodexUserInputObservation) error {
			if !o.ReplyStarted || o.ReplyWritten {
				t.Error("intent was not an actual pre-write fact")
			}
			intent.Store(true)
			return nil
		},
		Observe: func(_ context.Context, _ cc.CodexUserInputRequest, _ string, o cc.CodexUserInputObservation) error {
			mu.Lock()
			observations = append(observations, o)
			mu.Unlock()
			return nil
		},
	}
	d := inputDriver187(t, h)
	inputAck187(t, d)
	p := inputParams187(d)
	p["autoResolutionMs"] = nil
	p["questions"] = []any{map[string]any{"id": "color", "header": "Color", "question": "Choose the reviewed color", "isOther": true, "options": []any{map[string]any{"label": "Blue", "description": "Original option"}}}, map[string]any{"id": "scope", "header": "Scope", "question": "Describe the scope", "options": nil}, map[string]any{"id": "skip", "header": "Optional", "question": "Optional detail", "options": []any{}}}
	const nativeID = "9007199254740993"
	codexApprovalSend(t, d, json.Number(nativeID), cc.UserInputMethod, p)
	codexApprovalSend(t, d, json.Number(nativeID), cc.UserInputMethod, p)
	var writes atomic.Int32
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = d.CopyInput(codexApprovalWriter(func(raw []byte) (int, error) {
			if !intent.Load() {
				t.Error("native write preceded durable source intent")
			}
			m, err := decodeToolObject(raw)
			if err != nil || m["id"] != json.Number(nativeID) {
				t.Error("native RPC ID type/value changed")
			}
			result := objectAt(m, "result")
			answers := objectAt(result, "answers")
			if rawAnswers, _ := json.Marshal(answers); !strings.Contains(string(rawAnswers), `"answers":[]`) || !strings.Contains(string(rawAnswers), "user_note: keep the API") || !strings.Contains(string(rawAnswers), "user_note: bounded goal") {
				t.Errorf("native answer union changed: %s", rawAnswers)
			}
			writes.Add(1)
			// These native facts arrive before the local actual Write returns.
			codexApprovalFrame(t, d, map[string]any{"method": "serverRequest/resolved", "params": map[string]any{"threadId": d.thread, "requestId": json.Number(nativeID)}})
			inputTerminal187(t, d)
			return len(raw), nil
		}))
	}()
	codexApprovalAwait(t, done)
	d.Finish()
	proof := inputProof187(d)
	if opened.Load() != 1 || writes.Load() != 1 || len(proof) != 1 || proof[0].Observation == nil || !proof[0].Observation.ReplyWritten || !proof[0].Observation.RequestResolved || !proof[0].Observation.TurnClosed || proof[0].Observation.FailureCode != "" || d.bad != nil {
		t.Fatalf("native answer did not close once: %+v error=%v", proof, d.bad)
	}
	raw, _ := json.Marshal(proof)
	if strings.Contains(string(raw), "bounded goal") || strings.Contains(string(raw), "Choose the reviewed color") {
		t.Fatal("question/answer leaked into execution evidence")
	}
	mu.Lock()
	defer mu.Unlock()
	for i := 1; i < len(observations); i++ {
		if !inputObservationAdvances(ic.FromCodexObservation(observations[i-1]), ic.FromCodexObservation(observations[i])) {
			t.Fatalf("delivery fact regressed: %+v", observations)
		}
	}
}

func TestCodexUserInputNativeClosureBeforeAnswerDoesNotWrite(t *testing.T) {
	for _, kind := range []string{"resolved", "turn_closed"} {
		t.Run(kind, func(t *testing.T) {
			entered := make(chan struct{})
			d := inputDriver187(t, codexUserInputHandler{Answer: func(ctx context.Context, _ cc.CodexUserInputRequest, _ string) (*ic.ToolInputAnswer, error) {
				close(entered)
				<-ctx.Done()
				return nil, ctx.Err()
			}})
			inputAck187(t, d)
			codexApprovalSend(t, d, "native187", cc.UserInputMethod, inputParams187(d))
			codexApprovalAwait(t, entered)
			var writes atomic.Int32
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = d.CopyInput(codexApprovalWriter(func(raw []byte) (int, error) { writes.Add(1); return len(raw), nil }))
			}()
			if kind == "resolved" {
				codexApprovalFrame(t, d, map[string]any{"method": "serverRequest/resolved", "params": map[string]any{"threadId": d.thread, "requestId": "native187"}})
				codexApprovalEventually(t, func() bool { d.mu.Lock(); defer d.mu.Unlock(); return d.userInputsClosed() })
			}
			inputTerminal187(t, d)
			codexApprovalAwait(t, done)
			d.Finish()
			proof := inputProof187(d)
			if writes.Load() != 0 || d.bad != nil || len(proof) != 1 || proof[0].Observation.AnswerObserved || proof[0].Observation.ReplyStarted || !proof[0].Observation.TurnClosed || (kind == "turn_closed" && proof[0].Observation.RequestResolved) {
				t.Fatalf("native closure was fabricated as an answer: %+v %v", proof, d.bad)
			}
		})
	}
}

func TestCodexUserInputIntentStagesClosureAndPartialWriteStaysUnknown(t *testing.T) {
	for _, kind := range []string{"closure_during_intent", "partial_write"} {
		t.Run(kind, func(t *testing.T) {
			entered, release := make(chan struct{}), make(chan struct{})
			var mu sync.Mutex
			var observations []cc.CodexUserInputObservation
			d := inputDriver187(t, codexUserInputHandler{BeforeWrite: func(_ context.Context, _ cc.CodexUserInputRequest, _ string, _ ic.ToolInputAnswer, o cc.CodexUserInputObservation) error {
				mu.Lock()
				observations = append(observations, o)
				mu.Unlock()
				close(entered)
				if kind == "closure_during_intent" {
					<-release
				}
				return nil
			}, Observe: func(_ context.Context, _ cc.CodexUserInputRequest, _ string, o cc.CodexUserInputObservation) error {
				mu.Lock()
				defer mu.Unlock()
				if len(observations) > 0 && !inputObservationAdvances(ic.FromCodexObservation(observations[len(observations)-1]), ic.FromCodexObservation(o)) {
					return errToolInput
				}
				observations = append(observations, o)
				return nil
			}})
			inputAck187(t, d)
			codexApprovalSend(t, d, "native187", cc.UserInputMethod, inputParams187(d))
			var writes atomic.Int32
			done := make(chan struct{})
			go func() {
				defer close(done)
				_, _ = d.CopyInput(codexApprovalWriter(func(raw []byte) (int, error) { writes.Add(1); return len(raw) - 1, nil }))
			}()
			codexApprovalAwait(t, entered)
			if kind == "closure_during_intent" {
				codexApprovalFrame(t, d, map[string]any{"method": "serverRequest/resolved", "params": map[string]any{"threadId": d.thread, "requestId": "native187"}})
				inputTerminal187(t, d)
				close(release)
			}
			codexApprovalAwait(t, done)
			d.Finish()
			proof := inputProof187(d)
			if len(proof) != 1 || proof[0].Observation == nil || !proof[0].Observation.ReplyStarted || proof[0].Observation.ReplyWritten || proof[0].Observation.FailureCode == "" || d.bad == nil || kind == "closure_during_intent" && (writes.Load() != 0 || !proof[0].Observation.RequestResolved || !proof[0].Observation.TurnClosed) || kind == "partial_write" && writes.Load() != 1 {
				t.Fatalf("uncertain native response was replayable: %+v %v writes=%d", proof, d.bad, writes.Load())
			}
			mu.Lock()
			defer mu.Unlock()
			last := observations[len(observations)-1]
			if last.FailureCode == "" || kind == "closure_during_intent" && (!last.RequestResolved || !last.TurnClosed) {
				t.Fatalf("late facts were not durably published: %+v", observations)
			}
		})
	}
}

func TestCodexUserInputRejectsUnsafeAndCrossKindRequests(t *testing.T) {
	for _, kind := range []string{"secret", "null_secret", "bad_secret", "null_other", "unknown_extension", "bad_scope", "cross_kind", "changed_repeat"} {
		t.Run(kind, func(t *testing.T) {
			var opens atomic.Int32
			d := inputDriver187(t, codexUserInputHandler{Open: func(context.Context, cc.CodexUserInputRequest) (string, error) { opens.Add(1); return "input187", nil }, Answer: func(ctx context.Context, _ cc.CodexUserInputRequest, _ string) (*ic.ToolInputAnswer, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}})
			p := inputParams187(d)
			q := p["questions"].([]any)[0].(map[string]any)
			switch kind {
			case "secret":
				q["isSecret"] = true
			case "null_secret":
				q["isSecret"] = nil
			case "bad_secret":
				q["isSecret"] = "false"
			case "null_other":
				q["isOther"] = nil
			case "unknown_extension":
				q["unknown"] = true
			case "bad_scope":
				p["turnId"] = "another-turn"
			}
			codexApprovalSend(t, d, "native187", cc.UserInputMethod, p)
			if kind == "cross_kind" || kind == "changed_repeat" {
				codexApprovalEventually(t, func() bool { return opens.Load() == 1 })
				if kind == "cross_kind" {
					codexApprovalItemEvent(t, d, codexApprovalCommand(d, "command187"), false)
					codexApprovalSend(t, d, "native187", "item/commandExecution/requestApproval", codexApprovalParams(d, "command187"))
				} else {
					codexApprovalSend(t, d, "native187", cc.UserInputMethod, p)
					q["question"] = "Changed native source"
					codexApprovalSend(t, d, "native187", cc.UserInputMethod, p)
				}
			}
			if d.bad == nil || (kind != "cross_kind" && kind != "changed_repeat" && opens.Load() != 0) {
				t.Fatalf("unsafe request reached Journal/HTTP callback: %s count=%d", kind, opens.Load())
			}
		})
	}
}
