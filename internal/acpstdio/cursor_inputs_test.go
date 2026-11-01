package acpstdio

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
)

func cursorInputParams(method string) map[string]any {
	if method == cu.AskQuestionMethod {
		return map[string]any{"toolCallId": "ask190", "title": "Choose", "questions": []any{map[string]any{"id": "q", "prompt": "Select both", "allowMultiple": true, "options": []any{map[string]any{"id": "a", "label": "A"}, map[string]any{"id": "b", "label": "B"}}}}}
	}
	return map[string]any{"toolCallId": "plan190", "name": "Reviewed plan", "overview": "Review only", "plan": "# Exact plan\nChange the owned file", "todos": []any{}, "isProject": false, "phases": []any{}}
}
func cursorInputFrame(d *Driver, id any, method string, params map[string]any) {
	raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	_, _ = d.Write(append(raw, '\n'))
}

func cursorInputFixtureHandler(t *testing.T) CursorInputHandler {
	t.Helper()
	return CursorInputHandler{
		Open: func(_ context.Context, r cu.CursorInputRequest) (string, error) {
			if cu.ValidateRequest(r) != nil || r.SessionID != "fixture-session" || r.PromptID.Value == "" {
				t.Error("input source was not exact active SDK session")
			}
			return "input-" + r.RequestID.Kind + "-" + r.RequestID.Value, nil
		},
		Answer: func(_ context.Context, r cu.CursorInputRequest, _ string) (*CursorInputAnswer, error) {
			decision := cu.CursorInputDecision{Outcome: "rejected", Reason: "Revise this exact plan"}
			if r.Questions != nil {
				decision = cu.CursorInputDecision{Outcome: "answered", Answers: []cu.CursorInputSelection{{QuestionID: "q", OptionIDs: []string{"a", "b"}}}}
			}
			sha, err := cu.AnswerDigest(r, decision)
			return &CursorInputAnswer{Decision: decision, AnswerSHA256: sha}, err
		},
		BeforeWrite: func(_ context.Context, _ cu.CursorInputRequest, _ string, _ CursorInputAnswer, o delivery.Observation) error {
			if !o.ReplyStarted || o.ReplyWritten || delivery.Validate(o) != nil {
				t.Error("write intent was not durable first")
			}
			return nil
		},
		Observe: func(_ context.Context, _ cu.CursorInputRequest, _ string, o delivery.Observation) error {
			if delivery.Validate(o) != nil || o.RequestResolved {
				t.Error("invalid or fabricated Cursor observation")
			}
			return nil
		},
	}
}

func TestCursorInputsAnswerAndRejectThroughOriginalWriter(t *testing.T) {
	h := cursorInputFixtureHandler(t)
	replies := 0
	r, _ := runFixture(t, Options{Runtime: "cursor", Inputs: &h}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
		if methodOf(p) == "session/prompt" {
			s.promptID = append(json.RawMessage(nil), p["id"]...)
			cursorInputFrame(d, 42, cu.AskQuestionMethod, cursorInputParams(cu.AskQuestionMethod))
			cursorInputFrame(d, "42", cu.CreatePlanMethod, cursorInputParams(cu.CreatePlanMethod))
			return
		}
		if methodOf(p) == "" {
			replies++
			if p["error"] != nil {
				t.Error("human decision became RPC error")
			}
			result := object(p["result"])
			out := object(result["outcome"])
			if stringField(out, "outcome") != "answered" && stringField(out, "outcome") != "rejected" {
				t.Error("typed native outcome changed")
			}
			if replies == 2 {
				sendResult(d, s.promptID, map[string]any{"stopReason": "end_turn"})
			}
			return
		}
		if methodOf(p) == "session/set_config_option" {
			s.promptID = nil
		}
		basicReply(d, p, s)
	})
	if replies != 2 || r.Terminal != "completed" || len(r.Evidence.UserInputs) != 2 {
		t.Fatal("two original typed replies were not delivered", r.Error)
	}
	for _, item := range r.Evidence.UserInputs {
		if item.Observation == nil || !item.Observation.ReplyWritten || !item.Observation.ExecutionClosed || item.Observation.RequestResolved {
			t.Fatal("writer/closure facts were not persisted before result")
		}
	}
}

func TestCursorInputClosureAndUnknownWriteDoNotReanswer(t *testing.T) {
	for _, mode := range []string{"closed_before_answer", "partial_write", "secret_before_open", "closed_during_intent"} {
		t.Run(mode, func(t *testing.T) {
			h := cursorInputFixtureHandler(t)
			var mu sync.Mutex
			opened := make(chan struct{})
			opens := 0
			h.Open = func(_ context.Context, r cu.CursorInputRequest) (string, error) {
				mu.Lock()
				opens++
				mu.Unlock()
				close(opened)
				return "input190", nil
			}
			if mode == "closed_before_answer" {
				h.Answer = func(ctx context.Context, _ cu.CursorInputRequest, _ string) (*CursorInputAnswer, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
			}
			options := Options{Runtime: "cursor", Inputs: &h, Secrets: []string{"owned-secret190"}}
			var active *Driver
			if mode == "closed_during_intent" {
				h.BeforeWrite = func(_ context.Context, _ cu.CursorInputRequest, _ string, _ CursorInputAnswer, _ delivery.Observation) error {
					_ = active.Close()
					return nil
				}
			}
			nativeWrites := 0
			var prompt json.RawMessage
			r, _ := runFixture(t, options, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				if methodOf(p) == "session/prompt" {
					active = d
					prompt = append(json.RawMessage(nil), p["id"]...)
					params := cursorInputParams(cu.AskQuestionMethod)
					if mode == "secret_before_open" {
						params["title"] = "owned-secret190"
					}
					cursorInputFrame(d, "input190", cu.AskQuestionMethod, params)
					if mode == "closed_before_answer" {
						<-opened
						sendResult(d, prompt, map[string]any{"stopReason": "end_turn"})
					}
					return
				}
				basicReply(d, p, s)
			}, func(raw []byte) (int, error) {
				p := object(raw)
				if p["method"] == nil && p["result"] != nil {
					nativeWrites++
				}
				if mode == "partial_write" && p["method"] == nil && p["result"] != nil {
					return 3, io.ErrUnexpectedEOF
				}
				return len(raw), nil
			})
			mu.Lock()
			count := opens
			mu.Unlock()
			if mode == "secret_before_open" {
				if count != 0 || r.Terminal != "unknown" {
					t.Fatal("secret entered durable input domain")
				}
				return
			}
			if len(r.Evidence.UserInputs) != 1 || r.Evidence.UserInputs[0].Observation == nil {
				t.Fatal("exact input evidence missing", r.Error)
			}
			o := r.Evidence.UserInputs[0].Observation
			if mode == "closed_before_answer" {
				if o.ReplyStarted || !o.ExecutionClosed || r.Terminal != "unknown" {
					t.Fatal("native closure wrote a late answer", r.Error)
				}
			} else if !o.ReplyStarted || o.ReplyWritten || r.Terminal != "unknown" || !strings.Contains(o.FailureCode, "unknown") {
				t.Fatal("partial native write was retried or accepted", r.Error)
			}
			if mode == "closed_during_intent" && nativeWrites != 0 {
				t.Fatal("native answer was written after local close during durable intent")
			}
		})
	}
}

func TestCursorInputWireBoundsAndCrossKindIdentity(t *testing.T) {
	for _, mode := range []string{"large_plan", "malformed_title", "cross_kind_id", "changed_duplicate"} {
		t.Run(mode, func(t *testing.T) {
			h := cursorInputFixtureHandler(t)
			opened := 0
			baseOpen := h.Open
			h.Open = func(ctx context.Context, r cu.CursorInputRequest) (string, error) {
				opened++
				if mode == "large_plan" && len(r.Plan.Plan) != cu.MaxPlanBytes {
					t.Error("native plan was truncated")
				}
				return baseOpen(ctx, r)
			}
			permissions := 0
			var prompt json.RawMessage
			r, _ := runFixture(t, Options{Runtime: "cursor", Inputs: &h, Permission: func(context.Context, ac.NativeSource) (ac.Choice, error) { permissions++; return ac.Choice{}, nil }}, func(d *Driver, p map[string]json.RawMessage, s *fixtureState) {
				if methodOf(p) == "session/prompt" {
					prompt = append(json.RawMessage(nil), p["id"]...)
					method := cu.AskQuestionMethod
					params := cursorInputParams(method)
					if mode == "large_plan" {
						method = cu.CreatePlanMethod
						params = cursorInputParams(method)
						params["plan"] = strings.Repeat("p", cu.MaxPlanBytes)
					}
					if mode == "malformed_title" {
						params["title"] = true
					}
					cursorInputFrame(d, 99, method, params)
					if mode == "cross_kind_id" {
						cursorInputFrame(d, 99, "session/request_permission", map[string]any{"sessionId": "fixture-session", "toolCall": map[string]any{"toolCallId": "other", "title": "Must not grant", "kind": "edit", "status": "pending", "rawInput": map[string]any{}}, "options": []any{map[string]any{"optionId": "once", "name": "Allow", "kind": "allow_once"}}})
					}
					if mode == "changed_duplicate" {
						params["title"] = "changed same ID"
						cursorInputFrame(d, 99, method, params)
					}
					return
				}
				if methodOf(p) == "" {
					if mode == "large_plan" {
						sendResult(d, prompt, map[string]any{"stopReason": "end_turn"})
					}
					return
				}
				basicReply(d, p, s)
			})
			if mode == "large_plan" {
				if r.Terminal != "completed" || opened != 1 {
					t.Fatal("bounded complete native plan failed", r.Error)
				}
			} else if r.Terminal != "unknown" || permissions != 0 || mode == "malformed_title" && opened != 0 {
				t.Fatal("malformed source or cross-kind ID reached a fresh operation", r.Error)
			}
		})
	}
}
