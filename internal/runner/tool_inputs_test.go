package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func cloneToolInputRequest190(request ic.ToolInputRequest) ic.ToolInputRequest {
	raw, _ := json.Marshal(request)
	var copy ic.ToolInputRequest
	_ = json.Unmarshal(raw, &copy)
	return copy
}

func toolInputFixture187(t *testing.T) (Assignment, tc.Profile, ic.ToolInputRequest, ic.ToolInput) {
	t.Helper()
	a, profile, approval, permission := codexPermissionFixture174(t)
	profile.UserInputProtocol = cc.UserInputProtocol
	r := ic.ToolInputRequest{IdempotencyKey: "original187-question", Protocol: cc.Protocol, Codex: &cc.CodexUserInputRequest{
		RequestID: cc.CodexRPCRequestID{Kind: "string", Value: "question187"}, Method: cc.UserInputMethod,
		ThreadID: approval.Codex.ThreadID, TurnID: approval.Codex.TurnID, ItemID: "tool187",
		BindingSHA256: approval.Codex.BindingSHA256, ParamsSHA256: strings.Repeat("f", 64), IsBlocking: true,
		Questions: []cc.CodexUserInputQuestion{{ID: "approach187", Header: "Approach", Question: "Which interface should remain stable?", Options: []cc.CodexUserInputOption{}}},
	}}
	answer := ic.ToolInputAnswer{Answers: []cc.CodexUserInputSelection{{QuestionID: "approach187", Kind: "text", Text: "Keep the existing interface."}}, ActorID: "human187", AnsweredAt: "2026-11-01T00:00:01Z"}
	var err error
	answer.AnswerSHA256, err = cc.UserInputAnswerDigest(*r.Codex, answer.Answers)
	if err != nil {
		t.Fatal(err)
	}
	v := ic.ToolInput{ID: "input187", ProjectID: permission.ProjectID, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: profile.ProfileSHA256, Version: 2, State: "answered", Request: r, Answer: &answer, CreatedAt: "2026-11-01T00:00:00Z", ExpiresAt: "2026-11-01T00:10:00Z", AllowedActions: []string{}}
	if ic.ValidateRequest(r) != nil || ic.ValidateInput(v) != nil {
		t.Fatal("invalid owned tool input fixture")
	}
	return a, profile, r, v
}

// The durable boundary uses owned capability facts, without starting a vendor
// executable or borrowing a real account, home, or model session.
func cursorToolInputFixture190(t *testing.T) (Assignment, tc.Profile, ic.ToolInputRequest, ic.ToolInput) {
	t.Helper()
	a, profile, _ := acpPermissionFixture(t)
	profile = tc.CloneProfile(profile)
	profile.CommandID, profile.Adapter, profile.UserInputProtocol = "cursor190", "cursor", cu.InputProtocol
	profile.ACPExecution.Runtime, profile.ACPExecution.AuthStrategy = "cursor", "explicit_api_key_env"
	a.Command = []string{profile.CommandID}
	a.Config = map[string]any{"acp_execution": profile.ACPExecution, "tool_profile_sha256": profile.ProfileSHA256}
	source := cu.CursorInputRequest{RequestID: cu.CursorRPCRequestID{Kind: "number", Value: "9007199254740993"}, SessionID: "session190", PromptID: cu.CursorRPCRequestID{Kind: "number", Value: "2"}, Method: cu.AskQuestionMethod, ConfigurationSHA256: profile.ACPExecution.ConfigurationSHA256, Questions: &cu.CursorAskQuestion{ToolCallID: "tool190", Questions: []cu.CursorQuestion{{ID: "scope190", Prompt: "Which existing interfaces must remain stable?", AllowMultiple: true, Options: []cu.CursorQuestionOption{{ID: "cli", Label: "Command line"}, {ID: "http", Label: "HTTP"}}}}}}
	var err error
	source.ParamsSHA256, err = cu.ParamsDigest(source)
	if err != nil {
		t.Fatal(err)
	}
	r := ic.ToolInputRequest{IdempotencyKey: "cursor-input-original190", Protocol: ac.Protocol, Cursor: &source}
	decision := &cu.CursorInputDecision{Outcome: "answered", Answers: []cu.CursorInputSelection{{QuestionID: "scope190", OptionIDs: []string{"http", "cli"}}}}
	answer := ic.ToolInputAnswer{Cursor: decision, ActorID: "human190", AnsweredAt: "2026-11-01T00:00:01Z"}
	answer.AnswerSHA256, err = ic.AnswerDigest(r, nil, decision)
	if err != nil {
		t.Fatal(err)
	}
	v := ic.ToolInput{ID: "input190", ProjectID: "project190", RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: profile.ProfileSHA256, Version: 2, State: "answered", Request: r, Answer: &answer, CreatedAt: "2026-11-01T00:00:00Z", ExpiresAt: "2026-11-01T00:10:00Z", AllowedActions: []string{}}
	if tc.ValidateProfile(profile, true) != nil || ic.ValidateInput(v) != nil {
		t.Fatal("invalid owned Cursor input fixture")
	}
	return a, profile, r, v
}

func TestCursorToolInputJournalKeepsOriginalSourceAndIntent(t *testing.T) {
	t.Run("source_answer_and_secret", func(t *testing.T) {
		a, p, r, v := cursorToolInputFixture190(t)
		j, _ := savedToolInput187(t, a, p, r, v)
		for _, field := range []string{"configuration", "session", "prompt", "params"} {
			changed := cloneToolInputRequest190(r)
			changed.IdempotencyKey += "-" + field
			switch field {
			case "configuration":
				changed.Cursor.ConfigurationSHA256 = strings.Repeat("f", 64)
			case "session":
				changed.Cursor.SessionID = "another-session"
				changed.Cursor.RequestID.Value = "9007199254740994"
			case "prompt":
				changed.Cursor.PromptID.Kind = "string"
				changed.Cursor.RequestID.Value = "9007199254740995"
			case "params":
				changed.Cursor.Questions.Questions[0].Prompt = "Changed original prompt"
				changed.Cursor.ParamsSHA256, _ = cu.ParamsDigest(*changed.Cursor)
			}
			if j.recordToolInput(a, changed) == nil {
				t.Fatal("native request accepted another " + field)
			}
		}
		changed := v
		raw, _ := json.Marshal(v.Answer)
		var answer ic.ToolInputAnswer
		_ = json.Unmarshal(raw, &answer)
		answer.Cursor.Answers[0].OptionIDs = []string{"cli"}
		answer.AnswerSHA256, _ = ic.AnswerDigest(r, nil, answer.Cursor)
		changed.Answer, changed.Version = &answer, v.Version+1
		if j.observeToolInput(a, changed, false) == nil {
			t.Fatal("original multiple selection was replaced")
		}
		secret := "owned-credential190"
		private := cloneToolInputRequest190(r)
		private.Cursor.Questions.Questions[0].Prompt = secret
		private.Cursor.ParamsSHA256, _ = cu.ParamsDigest(*private.Cursor)
		if safeToolInput(private, []string{secret}) == nil {
			t.Fatal("credential-bearing native question was accepted")
		}
		privateAnswer := *v.Answer
		privateAnswer.Cursor = &cu.CursorInputDecision{Outcome: "skipped", Reason: secret}
		privateAnswer.AnswerSHA256, _ = ic.AnswerDigest(r, nil, privateAnswer.Cursor)
		if safeToolInputAnswer(r, privateAnswer, []string{secret}) == nil {
			t.Fatal("credential-bearing decision was accepted")
		}
		cp, _ := j.toolInput(a, r)
		if !samePermissionValue(cp.Answer, v.Answer) {
			t.Fatal("rejected replacement mutated original answer")
		}
	})
	t.Run("closed_then_no_write_and_crash_unknown", func(t *testing.T) {
		a, p, r, v := cursorToolInputFixture190(t)
		j, dir := savedToolInput187(t, a, p, r, v)
		o := delivery.Observation{Sequence: 1, ParamsSHA256: ic.ParamsSHA256(r), AnswerObserved: true, AnswerSHA256: v.Answer.AnswerSHA256, ReplyStarted: true}
		if err := j.recordToolInputObservation(a, r, v.ID, o, true); err != nil {
			t.Fatal(err)
		}
		if j.recordToolInputObservation(a, r, v.ID, o, true) == nil {
			t.Fatal("same durable intent granted a second native write")
		}
		o.Sequence++
		o.ExecutionClosed = true
		if err := j.recordToolInputObservation(a, r, v.ID, o, false); err != nil {
			t.Fatal(err)
		}
		o.Sequence++
		o.ReplyWritten = true
		if err := j.recordToolInputObservation(a, r, v.ID, o, false); err != nil {
			t.Fatal("late actual write fact was lost", err)
		}
		if err := j.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenJournal(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		cp, err := reopened.toolInput(a, r)
		if err != nil || cp.Observation == nil || !cp.Observation.ReplyWritten || !cp.Observation.ExecutionClosed || len(reopened.Pending()) != 1 || !reopened.Pending()[0].OutcomeUnknown {
			t.Fatal("crash changed original unknown write facts", err)
		}
		if reopened.beginToolInputExecution(a, p) == nil || reopened.recordToolInputObservation(a, r, v.ID, o, true) == nil {
			t.Fatal("restart reissued a live session write")
		}
		// A closed unanswered request cannot acquire its first native write.
		a2, p2, r2, v2 := cursorToolInputFixture190(t)
		j2, _ := savedToolInput187(t, a2, p2, r2, v2)
		closed := delivery.Observation{Sequence: 1, ParamsSHA256: ic.ParamsSHA256(r2), ExecutionClosed: true}
		if err := j2.recordToolInputObservation(a2, r2, v2.ID, closed, false); err != nil {
			t.Fatal(err)
		}
		closed.Sequence++
		closed.AnswerObserved = true
		closed.AnswerSHA256 = v2.Answer.AnswerSHA256
		closed.ReplyStarted = true
		if j2.recordToolInputObservation(a2, r2, v2.ID, closed, true) == nil {
			t.Fatal("execution closure granted a late first write")
		}
	})
	t.Run("plan_and_shared_attempt_cap", func(t *testing.T) {
		a, p, r, v := cursorToolInputFixture190(t)
		j, _ := savedToolInput187(t, a, p, r, v)
		if err := j.beginToolPermissionExecution(a, p); err != nil {
			t.Fatal(err)
		}
		_, _, permission := acpPermissionFixture(t)
		if err := j.recordToolPermission(a, permission); err != nil {
			t.Fatal(err)
		}
		for i := 1; i <= 31; i++ {
			plan := cloneToolInputRequest190(r)
			plan.IdempotencyKey = fmt.Sprintf("cursor-plan190-%02d", i)
			plan.Cursor.RequestID.Value = fmt.Sprint(i + 100)
			plan.Cursor.Method = cu.CreatePlanMethod
			plan.Cursor.Questions = nil
			plan.Cursor.Plan = &cu.CursorCreatePlan{ToolCallID: fmt.Sprint("plan", i), Plan: "Keep the fixed model and original workspace.", Todos: []cu.CursorTodo{}}
			if i == 1 {
				plan.Cursor.Plan.Plan = strings.Repeat("p", cu.MaxPlanBytes)
			}
			plan.Cursor.ParamsSHA256, _ = cu.ParamsDigest(*plan.Cursor)
			if safeToolInput(plan, nil) != nil {
				t.Fatal("complete bounded plan was truncated or rejected")
			}
			err := j.recordToolInput(a, plan)
			if i <= 30 && err != nil || i == 31 && err == nil {
				t.Fatal("permission and input exceeded combined attempt budget", i, err)
			}
			if i == 1 {
				accepted := ic.ToolInputAnswer{Cursor: &cu.CursorInputDecision{Outcome: "accepted"}, ActorID: v.Answer.ActorID, AnsweredAt: v.Answer.AnsweredAt}
				accepted.AnswerSHA256, err = ic.AnswerDigest(plan, nil, accepted.Cursor)
				if err != nil || safeToolInputAnswer(plan, accepted, nil) != nil {
					t.Fatal("typed plan acceptance rejected", err)
				}
				planInput := v
				planInput.ID, planInput.Request, planInput.Answer = "input190-plan", plan, &accepted
				if err = j.observeToolInput(a, planInput, false); err != nil {
					t.Fatal(err)
				}
				checkpoint, err := j.toolInput(a, plan)
				if err != nil || checkpoint.Request.Cursor.Plan.Plan != plan.Cursor.Plan.Plan || !samePermissionValue(checkpoint.Answer, &accepted) {
					t.Fatal("full original plan/decision was not preserved", err)
				}
			}
		}
	})
	t.Run("current_poll_before_write", func(t *testing.T) {
		a, p, r, v := cursorToolInputFixture190(t)
		j, _ := savedToolInput187(t, a, p, r, v)
		requests, drift := 0, false
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			requests++
			if request.Header.Get("Authorization") != "Bearer private190" {
				t.Error("current identity was replaced")
			}
			var body struct {
				Request ic.ToolInputRequest `json:"request"`
				InputID string              `json:"input_id"`
			}
			if json.NewDecoder(request.Body).Decode(&body) != nil {
				t.Error("invalid current poll body")
			}
			response := v
			switch request.URL.Path {
			case "/api/v1/runner/tool-inputs":
				if !samePermissionValue(body.Request, r) {
					t.Error("original native source changed")
				}
			case "/api/v1/runner/tool-inputs/poll":
				if body.InputID != v.ID {
					t.Error("current poll changed original input")
				}
				if drift {
					response.WorkerID = "another-worker190"
				}
			default:
				t.Error("unexpected live input route", request.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()
		client := &Client{BaseURL: server.URL, ProjectID: v.ProjectID, Token: "private190", HTTP: server.Client()}
		h := newToolInputHandler(a, ExecConfig{Journal: j, ToolInputClient: client, ToolInputProfile: p.ProfileSHA256, Secrets: []string{"private190"}})
		id, err := h.Open(context.Background(), r)
		if err != nil || id != v.ID {
			t.Fatal("original open failed", err)
		}
		answer, err := h.Answer(context.Background(), r, id)
		if err != nil || !samePermissionValue(answer, v.Answer) {
			t.Fatal("original answer changed", err)
		}
		o := delivery.Observation{Sequence: 1, ParamsSHA256: ic.ParamsSHA256(r), AnswerObserved: true, AnswerSHA256: answer.AnswerSHA256, ReplyStarted: true}
		drift = true
		if h.BeforeWrite(context.Background(), r, id, *answer, o) == nil {
			t.Fatal("stale current source gained a write intent")
		}
		cp, _ := j.toolInput(a, r)
		if cp.Observation != nil {
			t.Fatal("invalid current source was saved as native write intent")
		}
		drift = false
		if err = h.BeforeWrite(context.Background(), r, id, *answer, o); err != nil {
			t.Fatal("exact current source did not gain its one intent", err)
		}
		o.Sequence++
		o.ReplyWritten = true
		o.ExecutionClosed = true
		if err = h.Observe(context.Background(), r, id, o); err != nil {
			t.Fatal("actual complete delivery facts rejected", err)
		}
		if h.BeforeWrite(context.Background(), r, id, *answer, o) == nil {
			t.Fatal("closed delivery gained another write intent")
		}
		if requests != 5 {
			t.Fatal("unexpected common handler request count", requests)
		}
	})
}

func savedToolInput187(t *testing.T, a Assignment, p tc.Profile, r ic.ToolInputRequest, v ic.ToolInput) (*Journal, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "journal")
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	if err = j.beginToolInputExecution(a, p); err != nil {
		t.Fatal(err)
	}
	if err = j.recordToolInput(a, r); err != nil {
		t.Fatal(err)
	}
	if err = j.observeToolInput(a, v, false); err != nil {
		t.Fatal(err)
	}
	return j, dir
}

func TestToolInputCheckpointBindsSourceAndSharesNativeRequestBudget(t *testing.T) {
	t.Run("source_binding_and_secret", func(t *testing.T) {
		a, p, r, v := toolInputFixture187(t)
		j, _ := savedToolInput187(t, a, p, r, v)
		changed := cloneToolInputRequest190(r)
		changed.IdempotencyKey = "changed187-binding"
		changed.Codex.RequestID.Value = "changed187"
		changed.Codex.BindingSHA256 = strings.Repeat("0", 64)
		if j.recordToolInput(a, changed) == nil {
			t.Fatal("question was rebound to another Codex selection")
		}
		changed = cloneToolInputRequest190(r)
		changed.IdempotencyKey = "changed187-params"
		changed.Codex.ParamsSHA256 = strings.Repeat("1", 64)
		if j.recordToolInput(a, changed) == nil {
			t.Fatal("same native RPC identity accepted changed parameters")
		}
		changed = cloneToolInputRequest190(r)
		changed.Codex.Questions = append([]cc.CodexUserInputQuestion{}, r.Codex.Questions...)
		changed.Codex.Questions[0].IsSecret = true
		if j.recordToolInput(a, changed) == nil {
			t.Fatal("secret question entered durable storage")
		}
		otherAttempt := a
		otherAttempt.Fence++
		if _, err := j.toolInput(otherAttempt, r); err == nil {
			t.Fatal("another fence read the original private answer")
		}
		cp, err := j.toolInput(a, r)
		if err != nil || cp.InputID != v.ID || !samePermissionValue(cp.Request, r) {
			t.Fatal("rejected source altered the original checkpoint", err)
		}
	})
	for _, permissionFirst := range []bool{true, false} {
		name := map[bool]string{true: "approval_then_question", false: "question_then_approval"}[permissionFirst]
		t.Run(name, func(t *testing.T) {
			a, p, r, _ := toolInputFixture187(t)
			_, _, approval, _ := codexPermissionFixture174(t)
			r.Codex.RequestID = approval.Codex.RequestID
			j, err := OpenJournal(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			if err = j.Accept(a); err != nil {
				t.Fatal(err)
			}
			if err = j.beginToolPermissionExecution(a, p); err != nil {
				t.Fatal(err)
			}
			if err = j.beginToolInputExecution(a, p); err != nil {
				t.Fatal(err)
			}
			if permissionFirst {
				if err = j.recordToolPermission(a, approval); err != nil {
					t.Fatal(err)
				}
				if j.recordToolInput(a, r) == nil {
					t.Fatal("approval native ID was reused for a question")
				}
			} else {
				if err = j.recordToolInput(a, r); err != nil {
					t.Fatal(err)
				}
				if j.recordToolPermission(a, approval) == nil {
					t.Fatal("question native ID was reused for an approval")
				}
			}
		})
	}
	t.Run("combined_32_request_cap", func(t *testing.T) {
		a, p, r, _ := toolInputFixture187(t)
		_, _, approval, permission := codexPermissionFixture174(t)
		j, _ := savedCodexPermission174(t, a, p, approval, permission)
		if err := j.beginToolInputExecution(a, p); err != nil {
			t.Fatal(err)
		}
		for i := 1; i <= 32; i++ {
			question := cloneToolInputRequest190(r)
			question.IdempotencyKey = fmt.Sprintf("question187-%02d", i)
			question.Codex.RequestID.Value = fmt.Sprintf("question187-%02d", i)
			question.Codex.ParamsSHA256 = digest([]byte(question.IdempotencyKey))
			err := j.recordToolInput(a, question)
			if i <= 31 && err != nil || i == 32 && err == nil {
				t.Fatal("independent domains did not share the bounded native budget", i, err)
			}
		}
	})
	t.Run("native_codex_json_preserves_closed_receipt", func(t *testing.T) {
		a, p, r, v := toolInputFixture187(t)
		j, dir := savedToolInput187(t, a, p, r, v)
		o := delivery.Observation{Sequence: 1, ParamsSHA256: ic.ParamsSHA256(r), ExecutionClosed: true}
		if err := j.recordToolInputObservation(a, r, v.ID, o, false); err != nil {
			t.Fatal(err)
		}
		original := j.pendingToolInputEvents()
		raw, err := os.ReadFile(filepath.Join(dir, "journal.json"))
		var stored any
		if err == nil {
			err = json.Unmarshal(raw, &stored)
		}
		compact, _ := json.Marshal(stored)
		if err != nil || !strings.Contains(string(compact), `"turn_closed":true`) || strings.Contains(string(compact), `"execution_closed"`) {
			t.Fatal("native Codex closure was not stored as its exact current native JSON", err)
		}
		cpRaw, err := json.Marshal(j.entries[key(a)].ToolInputs)
		if err != nil {
			t.Fatal(err)
		}
		var malformed toolInputExecutionCheckpoint
		if json.Unmarshal([]byte(strings.ReplaceAll(string(cpRaw), `"turn_closed"`, `"execution_closed"`)), &malformed) == nil {
			t.Fatal("Codex checkpoint accepted another protocol's observation fields")
		}
		if err = j.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := OpenJournal(dir)
		if err != nil {
			t.Fatal("exact stored native Codex closure did not reopen", err)
		}
		defer reopened.Close()
		pending := reopened.pendingToolInputEvents()
		if len(original) != 1 || len(pending) != 1 || !samePermissionValue(original[0].toolInputEvent, pending[0].toolInputEvent) || !pending[0].Observation.ExecutionClosed {
			t.Fatal("reopen lost native true TurnClosed or changed original receipt")
		}
		calls, acceptedBody := 0, ""
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			calls++
			body, _ := io.ReadAll(request.Body)
			if request.URL.Path != "/api/v1/runner/tool-inputs/observations" || request.Header.Get("Authorization") != "Bearer native190" {
				t.Error("native replay changed original route or machine")
			}
			if acceptedBody == "" {
				acceptedBody = string(body)
			} else if acceptedBody != string(body) {
				t.Error("native closure ACK recovery changed original operation/body")
			}
			var sent struct {
				OperationID string               `json:"idempotency_key"`
				Observation delivery.Observation `json:"observation"`
			}
			if json.Unmarshal(body, &sent) != nil || sent.OperationID != original[0].OperationID || !samePermissionValue(sent.Observation, o) {
				t.Error("native projection changed original close fact or operation")
			}
			w.Header().Set("Content-Type", "application/json")
			if calls == 1 {
				_, _ = io.WriteString(w, `{"id":`)
				return
			}
			response := v
			response.Observation = &sent.Observation
			_ = json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()
		client := &Client{BaseURL: server.URL, ProjectID: v.ProjectID, Token: "native190", HTTP: server.Client()}
		if ReplayToolInputObservations(context.Background(), client, reopened) == nil || len(reopened.pendingToolInputEvents()) != 1 {
			t.Fatal("lost ACK discarded original native closure")
		}
		if err = ReplayToolInputObservations(context.Background(), client, reopened); err != nil || calls != 2 || len(reopened.pendingToolInputEvents()) != 0 {
			t.Fatal("native closure receipt did not recover with original body/key", err)
		}
	})
}

func TestToolInputAnswerAndClosureCannotIssueAnotherWrite(t *testing.T) {
	a, p, r, input := toolInputFixture187(t)
	j, _ := savedToolInput187(t, a, p, r, input)
	changed := input
	answer := *input.Answer
	answer.Answers = append([]cc.CodexUserInputSelection{}, input.Answer.Answers...)
	answer.Answers[0].Text = "Replace it instead."
	answer.AnswerSHA256, _ = cc.UserInputAnswerDigest(*r.Codex, answer.Answers)
	changed.Answer, changed.Version = &answer, input.Version+1
	if j.observeToolInput(a, changed, false) == nil {
		t.Fatal("a later snapshot replaced the original human answer")
	}
	o := delivery.Observation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, AnswerObserved: true, AnswerSHA256: input.Answer.AnswerSHA256}
	if err := j.recordToolInputObservation(a, r, input.ID, o, false); err != nil {
		t.Fatal(err)
	}
	o.Sequence, o.RequestResolved, o.ExecutionClosed = 2, true, true
	if err := j.recordToolInputObservation(a, r, input.ID, o, false); err != nil {
		t.Fatal(err)
	}
	if err := j.recordToolInputObservation(a, r, input.ID, o, false); err != nil {
		t.Fatal("exact persisted closure retry rejected", err)
	}
	late := o
	late.Sequence, late.ReplyStarted = 3, true
	if j.recordToolInputObservation(a, r, input.ID, late, true) == nil {
		t.Fatal("closed question issued a fresh native response intent")
	}
	late = o
	late.Sequence, late.RequestResolved, late.ExecutionClosed = 3, false, false
	if j.recordToolInputObservation(a, r, input.ID, late, false) == nil {
		t.Fatal("native cleanup facts regressed")
	}
	cp, err := j.toolInput(a, r)
	if err != nil || !samePermissionValue(cp.Answer, input.Answer) || !samePermissionValue(cp.Observation, &o) || len(cp.Observations) != 2 {
		t.Fatal("rejected answer/write changed durable original facts", err)
	}
}

func TestToolInputDurableIntentCannotReplayAfterCrash(t *testing.T) {
	a, p, r, input := toolInputFixture187(t)
	j, dir := savedToolInput187(t, a, p, r, input)
	o := delivery.Observation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, AnswerObserved: true, AnswerSHA256: input.Answer.AnswerSHA256}
	if err := j.recordToolInputObservation(a, r, input.ID, o, false); err != nil {
		t.Fatal(err)
	}
	o.Sequence, o.ReplyStarted = 2, true
	if err := j.recordToolInputObservation(a, r, input.ID, o, true); err != nil {
		t.Fatal(err)
	}
	if j.recordToolInputObservation(a, r, input.ID, o, true) == nil {
		t.Fatal("a second native writer acquired the original intent")
	}
	before := j.pendingToolInputEvents()
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if len(reopened.Pending()) != 1 || !reopened.Pending()[0].OutcomeUnknown || reopened.beginToolInputExecution(a, p) == nil || reopened.recordToolInputObservation(a, r, input.ID, o, true) == nil {
		t.Fatal("restart manufactured a new process/response permit")
	}
	after := reopened.pendingToolInputEvents()
	if len(before) != 2 || len(after) != 2 {
		t.Fatal("restart changed the original observation operation or body")
	}
	for i := range before {
		if !samePermissionValue(before[i].toolInputEvent, after[i].toolInputEvent) || !samePermissionValue(before[i].Request, after[i].Request) || candidateRef(before[i].Assignment) != candidateRef(after[i].Assignment) || before[i].Assignment.DispatchID != after[i].Assignment.DispatchID {
			t.Fatal("restart changed the original observation operation or body")
		}
	}
	cp, err := reopened.toolInput(a, r)
	if err != nil || cp.Observation == nil || !cp.Observation.ReplyStarted || cp.Observation.ReplyWritten {
		t.Fatal("durable start intent was converted into a successful native write", err)
	}
}

func TestToolInputObservationRecoveryKeepsOriginalBodyBeforeResult(t *testing.T) {
	for _, fixture := range []struct {
		name string
		make func(*testing.T) (Assignment, tc.Profile, ic.ToolInputRequest, ic.ToolInput)
	}{{"codex", toolInputFixture187}, {"cursor", cursorToolInputFixture190}} {
		t.Run(fixture.name, func(t *testing.T) {
			for _, pool := range []bool{false, true} {
				t.Run(map[bool]string{false: "ordinary", true: "pool"}[pool], func(t *testing.T) {
					a, p, r, input := fixture.make(t)
					var scope *PoolContext
					namespace := "runner"
					if pool {
						a.TenantID, a.ProjectID, a.PoolID, a.PoolRunnerID, a.PoolSlotID = "tenant187", input.ProjectID, "pool187", "machine187", "0"
						a.PoolGrantID, a.PoolGrantVersion, a.PoolCredentialID, a.PoolCredentialVersion = "grant187", 1, "credential187", 1
						a.DispatchState, a.LeaseUntil = "active", "2026-11-01T00:10:00Z"
						frozen := a
						scope = &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID, Assignment: &frozen}
						namespace = "pool-runner"
					}
					j, _ := savedToolInput187(t, a, p, r, input)
					o := delivery.Observation{Sequence: 1, ParamsSHA256: ic.ParamsSHA256(r), AnswerObserved: true, AnswerSHA256: input.Answer.AnswerSHA256, ReplyStarted: true}
					if err := j.recordToolInputObservation(a, r, input.ID, o, true); err != nil {
						t.Fatal(err)
					}
					if err := j.Finish(Result{Assignment: a, ExitCode: -1, OutcomeUnknown: true}); err != nil {
						t.Fatal(err)
					}
					paths := []string{}
					originalBody := ""
					accepted := false
					pollDrift := false
					s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
						paths = append(paths, request.URL.Path)
						if request.Header.Get("Authorization") != "Bearer private187" || !strings.HasPrefix(request.URL.Path, "/api/v1/"+namespace+"/") {
							t.Error("tool input changed machine credentials or namespace")
						}
						raw, _ := io.ReadAll(request.Body)
						var body map[string]json.RawMessage
						if json.Unmarshal(raw, &body) != nil {
							t.Error("invalid tool input HTTP body")
						}
						var source Assignment
						_ = json.Unmarshal(body["assignment"], &source)
						if !strings.HasSuffix(request.URL.Path, "/results") && (source.RunID != a.RunID || source.Fence != a.Fence || source.DispatchID != a.DispatchID) {
							t.Error("original native assignment identity changed")
						}
						if pool && (len(body["tenant_id"]) == 0 || len(body["runner_id"]) == 0 || source.Config != nil || source.Command != nil) {
							t.Error("Pool transport lost frozen scope or copied private command inputs")
						}
						w.Header().Set("Content-Type", "application/json")
						response := input
						switch {
						case strings.HasSuffix(request.URL.Path, "/observations"):
							if originalBody == "" {
								originalBody = string(raw)
							} else if originalBody != string(raw) {
								t.Error("ACK recovery changed the accepted operation/body")
							}
							var observed delivery.Observation
							_ = json.Unmarshal(body["observation"], &observed)
							if !samePermissionValue(observed, o) || len(body["idempotency_key"]) == 0 {
								t.Error("observation does not match durable native facts")
							}
							if !accepted {
								accepted = true
								// The service accepted this body/key, but its ACK was lost.
								_, _ = io.WriteString(w, `{"id":`)
								return
							}
							response.Observation = &observed
						case strings.HasSuffix(request.URL.Path, "/results"):
							var result Result
							if json.Unmarshal(body["result"], &result) != nil || result.RunID != a.RunID || result.Fence != a.Fence || result.WorkerID != a.WorkerID || !result.OutcomeUnknown {
								t.Error("final result changed the original unknown execution fact")
							}
							if !accepted || len(j.pendingToolInputEvents()) != 0 {
								t.Error("final result overtook an unconfirmed native observation")
							}
							return
						case strings.HasSuffix(request.URL.Path, "/poll"):
							if pollDrift {
								response.WorkerID = "another-worker187"
							}
						default:
							t.Error("unexpected recovery route", request.URL.Path)
						}
						_ = json.NewEncoder(w).Encode(response)
					}))
					defer s.Close()
					client := &Client{BaseURL: s.URL, ProjectID: input.ProjectID, Token: "private187", Pool: scope, HTTP: s.Client()}
					recoveryClient := client
					if pool {
						// A restarted Pool slot has the parent machine client, not the
						// previous execution's derived project/assignment client.
						parent := *client
						parent.ProjectID = ""
						parent.Pool = &PoolContext{PoolID: scope.PoolID, RunnerID: scope.RunnerID}
						recoveryClient = &parent
					}
					replay := func() error {
						if pool {
							return replayPool(context.Background(), recoveryClient, j)
						}
						return Replay(context.Background(), recoveryClient, j)
					}
					if err := replay(); err == nil || len(paths) != 1 || len(j.pendingToolInputEvents()) != 1 || len(j.Pending()) != 1 {
						t.Fatal("lost ACK discarded original observation or delivered result", err, paths)
					}
					if err := replay(); err != nil || len(paths) != 3 || !strings.HasSuffix(paths[2], "/results") || len(j.pendingToolInputEvents()) != 0 || len(j.Pending()) != 0 {
						t.Fatal("original receipt recovery did not precede final result", err, paths)
					}
					if _, err := client.PollToolInput(context.Background(), a, input.ID); err != nil {
						t.Fatal("original machine could not poll exact question", err)
					}
					beforeDrift := len(paths)
					if pool {
						scope.Assignment.PoolCredentialVersion++
					} else {
						pollDrift = true
					}
					if _, err := client.PollToolInput(context.Background(), a, input.ID); err == nil {
						t.Fatal("changed current source was accepted as the original question")
					}
					if pool && len(paths) != beforeDrift {
						t.Fatal("changed Pool credential scope escaped before local source validation")
					}
				})
			}
		})
	}
}
