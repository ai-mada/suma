package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func toolInputFixture187(t *testing.T) (Assignment, tc.Profile, ic.ToolInputRequest, ic.ToolInput) {
	t.Helper()
	a, profile, approval, permission := codexPermissionFixture174(t)
	profile.UserInputProtocol = cc.UserInputProtocol
	r := ic.ToolInputRequest{IdempotencyKey: "original187-question", Protocol: cc.Protocol, Codex: cc.CodexUserInputRequest{
		RequestID: cc.CodexRPCRequestID{Kind: "string", Value: "question187"}, Method: cc.UserInputMethod,
		ThreadID: approval.Codex.ThreadID, TurnID: approval.Codex.TurnID, ItemID: "tool187",
		BindingSHA256: approval.Codex.BindingSHA256, ParamsSHA256: strings.Repeat("f", 64), IsBlocking: true,
		Questions: []cc.CodexUserInputQuestion{{ID: "approach187", Header: "Approach", Question: "Which interface should remain stable?", Options: []cc.CodexUserInputOption{}}},
	}}
	answer := ic.ToolInputAnswer{Answers: []cc.CodexUserInputSelection{{QuestionID: "approach187", Kind: "text", Text: "Keep the existing interface."}}, ActorID: "human187", AnsweredAt: "2026-11-01T00:00:01Z"}
	var err error
	answer.AnswerSHA256, err = cc.UserInputAnswerDigest(r.Codex, answer.Answers)
	if err != nil {
		t.Fatal(err)
	}
	v := ic.ToolInput{ID: "input187", ProjectID: permission.ProjectID, RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, DispatchID: a.DispatchID, WorkerID: a.WorkerID, ProfileSHA256: profile.ProfileSHA256, Version: 2, State: "answered", Request: r, Answer: &answer, CreatedAt: "2026-11-01T00:00:00Z", ExpiresAt: "2026-11-01T00:10:00Z", AllowedActions: []string{}}
	if ic.ValidateRequest(r) != nil || ic.ValidateInput(v) != nil {
		t.Fatal("invalid owned tool input fixture")
	}
	return a, profile, r, v
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
		changed := r
		changed.IdempotencyKey = "changed187-binding"
		changed.Codex.RequestID.Value = "changed187"
		changed.Codex.BindingSHA256 = strings.Repeat("0", 64)
		if j.recordToolInput(a, changed) == nil {
			t.Fatal("question was rebound to another Codex selection")
		}
		changed = r
		changed.IdempotencyKey = "changed187-params"
		changed.Codex.ParamsSHA256 = strings.Repeat("1", 64)
		if j.recordToolInput(a, changed) == nil {
			t.Fatal("same native RPC identity accepted changed parameters")
		}
		changed = r
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
			question := r
			question.IdempotencyKey = fmt.Sprintf("question187-%02d", i)
			question.Codex.RequestID.Value = fmt.Sprintf("question187-%02d", i)
			question.Codex.ParamsSHA256 = digest([]byte(question.IdempotencyKey))
			err := j.recordToolInput(a, question)
			if i <= 31 && err != nil || i == 32 && err == nil {
				t.Fatal("independent domains did not share the bounded native budget", i, err)
			}
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
	answer.AnswerSHA256, _ = cc.UserInputAnswerDigest(r.Codex, answer.Answers)
	changed.Answer, changed.Version = &answer, input.Version+1
	if j.observeToolInput(a, changed, false) == nil {
		t.Fatal("a later snapshot replaced the original human answer")
	}
	o := cc.CodexUserInputObservation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, AnswerObserved: true, AnswerSHA256: input.Answer.AnswerSHA256}
	if err := j.recordToolInputObservation(a, r, input.ID, o, false); err != nil {
		t.Fatal(err)
	}
	o.Sequence, o.RequestResolved, o.TurnClosed = 2, true, true
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
	late.Sequence, late.RequestResolved, late.TurnClosed = 3, false, false
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
	o := cc.CodexUserInputObservation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, AnswerObserved: true, AnswerSHA256: input.Answer.AnswerSHA256}
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
	for _, pool := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "pool"}[pool], func(t *testing.T) {
			a, p, r, input := toolInputFixture187(t)
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
			o := cc.CodexUserInputObservation{Sequence: 1, ParamsSHA256: r.Codex.ParamsSHA256, AnswerObserved: true, AnswerSHA256: input.Answer.AnswerSHA256, ReplyStarted: true}
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
					var observed cc.CodexUserInputObservation
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
}
