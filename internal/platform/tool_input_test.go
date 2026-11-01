package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func toolInputRequest187(binding cc.Binding, id string) ic.ToolInputRequest {
	return ic.ToolInputRequest{IdempotencyKey: newID(), Protocol: cc.Protocol, Codex: cc.CodexUserInputRequest{RequestID: cc.CodexRPCRequestID{Kind: "string", Value: id}, Method: cc.UserInputMethod, ThreadID: "thread187", TurnID: "turn187", ItemID: "tool187", BindingSHA256: binding.BindingSHA256, ParamsSHA256: strings.Repeat("e", 64), IsBlocking: true, Questions: []cc.CodexUserInputQuestion{{ID: "design", Header: "Design choice", Question: "Which existing approach should this change use?", Options: []cc.CodexUserInputOption{{Label: "Keep current design", Description: "Preserve the reviewed module boundary"}, {Label: "Refine the module", Description: "Use the requested smaller interface"}}, IsOther: true}}}}
}

func TestToolInputObservationReceiptRecoversAfterLeaseAndTerminalDrift(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "pool"}[pooled], func(t *testing.T) {
			h, binding := newCodexToolInteractionFixture(t, pooled, true)
			f, ctx := h.f, context.Background()
			request := toolInputRequest187(binding, "receipt187")
			var input ic.ToolInput
			var err error
			if pooled {
				input, err = f.s.OpenPoolToolInput(ctx, h.token, f.project, h.assignment, request)
			} else {
				input, err = f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, request)
			}
			if err != nil {
				t.Fatal(err)
			}
			o := cc.CodexUserInputObservation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, RequestResolved: true, TurnClosed: true}
			key := newID()
			observe := func(a dag.Assignment, operation string, observation cc.CodexUserInputObservation) (ic.ToolInput, error) {
				if pooled {
					return f.s.ObservePoolToolInput(ctx, h.token, f.project, a, input.ID, operation, observation)
				}
				return f.s.ObserveToolInput(ctx, h.actor, f.project, a, input.ID, operation, observation)
			}
			original, err := observe(h.assignment, key, o)
			if err != nil {
				t.Fatal(err)
			}
			// The first acknowledgement is lost after the operation commits. The
			// same body/key recovers that receipt without authorizing a new write.
			if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID); err != nil {
				t.Fatal(err)
			}
			recovered, err := observe(h.assignment, key, o)
			if err != nil || stringify(recovered) != stringify(original) {
				t.Fatal("original observation receipt blocked by expired lease", err)
			}
			changed := o
			changed.Sequence++
			if _, err = observe(h.assignment, key, changed); err == nil {
				t.Fatal("same operation key accepted a changed observation")
			}
			if _, err = observe(h.assignment, newID(), changed); err == nil {
				t.Fatal("expired lease authorized a fresh observation")
			}
			if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='succeeded' WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.DB.Exec(`UPDATE dag_runs SET state='succeeded' WHERE id=?`, h.assignment.RunID); err != nil {
				t.Fatal(err)
			}
			recovered, err = observe(h.assignment, key, o)
			if err != nil || stringify(recovered) != stringify(original) {
				t.Fatal("original observation receipt blocked by terminal execution", err)
			}
			if _, err = observe(h.assignment, newID(), changed); err == nil {
				t.Fatal("terminal execution authorized a fresh observation")
			}
			wrong := h.assignment
			wrong.Fence++
			if _, err = observe(wrong, key, o); err == nil {
				t.Fatal("old receipt accepted a changed assignment")
			}
			// Receipt recovery still authenticates the current credential. It
			// does not acquire a historical identity exception after expiry.
			if pooled {
				_, err = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET expires_at='2000-01-01T00:00:00Z' WHERE id=?`, h.assignment.PoolCredentialID)
			} else {
				_, err = f.s.DB.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, digest(h.token))
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = observe(h.assignment, key, o); err == nil {
				t.Fatal("expired current credential recovered a private receipt")
			}
		})
	}
}

func TestToolInputHTTPRegistersExactCurrentCodexRequest(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(fmt.Sprint(pooled), func(t *testing.T) {
			h, binding := newCodexToolInteractionFixture(t, pooled, true)
			f := h.f
			ctx := context.Background()
			request := toolInputRequest187(binding, "input187")
			var body any
			prefix := "/runner"
			if pooled {
				prefix = "/pool-runner"
				body = PoolToolInputOpenRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, Request: request}
			} else {
				body = ToolInputOpenRequest{ProjectID: f.project, Assignment: h.assignment, Request: request}
			}
			got := f.bearer("POST", prefix+"/tool-inputs", h.token, body, 200)
			var input ic.ToolInput
			if json.Unmarshal([]byte(stringify(got)), &input) != nil || input.State != "pending" || input.Version != 1 {
				t.Fatal(got)
			}
			if again := f.bearer("POST", prefix+"/tool-inputs", h.token, body, 200); again["id"] != input.ID {
				t.Fatal("open replay changed native request identity")
			}
			created, _ := time.Parse(time.RFC3339Nano, input.CreatedAt)
			expires, _ := time.Parse(time.RFC3339Nano, input.ExpiresAt)
			if expires.Sub(created) <= 30*time.Second || expires.Sub(created) > 15*time.Minute {
				t.Fatal("reply TTL used short assignment lease", input)
			}
			listPath := "/runs/" + h.assignment.RunID + "/nodes/" + h.assignment.NodeID + "/tool-inputs?project_id=" + f.project + "&attempt=" + strconv.Itoa(h.assignment.Attempt) + "&fence=" + strconv.FormatInt(h.assignment.Fence, 10)
			listed := f.call("GET", listPath, nil, 200)
			if listed["protocol"] != cc.UserInputProtocol || len(listed["items"].([]any)) != 1 {
				t.Fatal(listed)
			}
			answer := ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Answers: []cc.CodexUserInputSelection{{QuestionID: "design", Kind: "option", Option: "Keep current design"}}}
			f.call("POST", "/tool-inputs/"+input.ID+"/answer", candidateJSON(t, answer), 200)
			var polled ic.ToolInput
			var err error
			if pooled {
				polled, err = f.s.PollPoolToolInput(ctx, h.token, f.project, h.assignment, input.ID)
			} else {
				polled, err = f.s.PollToolInput(ctx, h.actor, f.project, h.assignment, input.ID)
			}
			if err != nil || polled.Answer == nil || polled.Answer.ActorID != f.user.ID {
				t.Fatal(polled, err)
			}
			o := cc.CodexUserInputObservation{Sequence: 1, ParamsSHA256: request.Codex.ParamsSHA256, AnswerSHA256: polled.Answer.AnswerSHA256, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, RequestResolved: true, TurnClosed: true}
			if pooled {
				body = PoolToolInputObservationRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, InputID: input.ID, IdempotencyKey: newID(), Observation: o}
			} else {
				body = ToolInputObservationRequest{ProjectID: f.project, Assignment: h.assignment, InputID: input.ID, IdempotencyKey: newID(), Observation: o}
			}
			observed := f.bearer("POST", prefix+"/tool-inputs/observations", h.token, body, 200)
			if observed["state"] != "closed" {
				t.Fatal(observed)
			}
			f.bearer("POST", prefix+"/tool-inputs/observations", h.token, body, 200)
			secret := request
			secret.IdempotencyKey = newID()
			secret.Codex.RequestID.Value = "secret187"
			secret.Codex.Questions = append([]cc.CodexUserInputQuestion{}, request.Codex.Questions...)
			secret.Codex.Questions[0].IsSecret = true
			secret.Codex.Questions[0].Question = "private-question-body-187"
			if pooled {
				_, err = f.s.OpenPoolToolInput(ctx, h.token, f.project, h.assignment, secret)
			} else {
				_, err = f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, secret)
			}
			requireAuthorityStatus(t, err, 400)
			var leaks int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tool_inputs WHERE payload LIKE '%private-question-body-187%'`).Scan(&leaks); err != nil || leaks != 0 {
				t.Fatal(leaks, err)
			}
			wrong := h.assignment
			wrong.Fence++
			if pooled {
				_, err = f.s.PollPoolToolInput(ctx, h.token, f.project, wrong, input.ID)
			} else {
				_, err = f.s.PollToolInput(ctx, h.actor, f.project, wrong, input.ID)
			}
			if err == nil {
				t.Fatal("wrong fence read original input")
			}
			denied := 403
			if pooled {
				denied = 401
			}
			f.bearer("GET", listPath, h.token, nil, denied)
			result := toolInputResult187(h, binding, input, o)
			forged := toolInputResult187(h, binding, input, o)
			forged.ToolEvidence.CodexExecution.UserInputs[0].Observation.Sequence++
			if pooled {
				f.bearer("POST", "/pool-runner/results", h.token, PoolRunnerResultRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, Result: forged}, 400)
				body := PoolRunnerResultRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, Result: result}
				f.bearer("POST", "/pool-runner/results", h.token, body, 200)
				f.bearer("POST", "/pool-runner/results", h.token, body, 200)
			} else {
				f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": forged}), 400)
				body := f.body(map[string]any{"result": result})
				f.bearer("POST", "/runner/results", h.token, body, 200)
				f.bearer("POST", "/runner/results", h.token, body, 200)
			}
		})
	}
}

func TestToolInputTwoCurrentHumansCompeteAndRecoverOriginalAnswer(t *testing.T) {
	h, b := newCodexToolInteractionFixture(t, false, true)
	f := h.f
	ctx := context.Background()
	input, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, toolInputRequest187(b, "humans187"))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.s.CreateInvitation(ctx, f.actor, map[string]any{"email": "input-reviewer187@example.test", "role": "member", "grants": []any{map[string]any{"project_id": f.project, "role": "developer"}}, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := f.s.AcceptInvitation(ctx, map[string]any{"token": inv["token"], "email": "input-reviewer187@example.test", "name": "Input reviewer", "password": "input-reviewer-strong-187", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	actors := []Actor{f.actor, second}
	requests := []ic.ToolInputAnswerRequest{}
	for _, option := range []string{"Keep current design", "Refine the module"} {
		requests = append(requests, ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Answers: []cc.CodexUserInputSelection{{QuestionID: "design", Kind: "option", Option: option}}})
	}
	start := make(chan struct{})
	var group sync.WaitGroup
	results := make([]ic.ToolInput, 2)
	failures := make([]error, 2)
	for i := range actors {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			<-start
			results[i], failures[i] = f.s.AnswerToolInput(ctx, actors[i], input.ID, requests[i])
		}(i)
	}
	close(start)
	group.Wait()
	winner := -1
	for i, e := range failures {
		if e == nil {
			if winner != -1 {
				t.Fatal("both human answers won CAS")
			}
			winner = i
		} else {
			requireAuthorityStatus(t, e, 409)
		}
	}
	if winner < 0 {
		t.Fatal(failures)
	}
	for range 2 {
		replay, e := f.s.AnswerToolInput(ctx, actors[winner], input.ID, requests[winner])
		if e != nil || stringify(replay) != stringify(results[winner]) {
			t.Fatal("answer receipt changed", replay, e)
		}
	}
	var version int64
	if err = f.s.DB.QueryRow(`SELECT version FROM suma_tool_inputs WHERE id=?`, input.ID).Scan(&version); err != nil || version != 2 {
		t.Fatal(version, err)
	}
	_, err = f.s.AnswerToolInput(ctx, h.actor, input.ID, requests[0])
	requireAuthorityStatus(t, err, 403)
}

func toolInputResult187(h toolPermissionFixture, b cc.Binding, input ic.ToolInput, o cc.CodexUserInputObservation) dag.Result {
	s := input.Request.Codex
	e := cc.CodexUserInputEvidence{InputID: input.ID, RequestID: s.RequestID, Method: s.Method, ThreadID: s.ThreadID, TurnID: s.TurnID, ItemID: s.ItemID, BindingSHA256: s.BindingSHA256, ParamsSHA256: s.ParamsSHA256, QuestionCount: len(s.Questions), IsBlocking: s.IsBlocking, Observation: &o}
	return dag.Result{RunID: h.assignment.RunID, NodeID: h.assignment.NodeID, Attempt: h.assignment.Attempt, Fence: h.assignment.Fence, WorkerID: h.assignment.WorkerID, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 3, SessionID: s.ThreadID, UsageState: tc.UsageUnreported, CodexExecution: &cc.Evidence{BindingSHA256: b.BindingSHA256, ThreadID: s.ThreadID, TurnID: s.TurnID, RequestedModel: b.Model, RequestedReasoningEffort: b.ReasoningEffort, AcknowledgedModel: b.Model, AcknowledgedReasoningEffort: b.ReasoningEffort, AcknowledgedProvider: "owned-provider", ConfigurationAcknowledged: true, UserInputs: []cc.CodexUserInputEvidence{e}}}}
}

func TestToolInputNativeClosureRacesAnswerAndPreservesWriteUnknownFacts(t *testing.T) {
	h, b := newCodexToolInteractionFixture(t, false, true)
	f := h.f
	ctx := context.Background()
	input, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, toolInputRequest187(b, "close187"))
	if err != nil {
		t.Fatal(err)
	}
	o := cc.CodexUserInputObservation{Sequence: 1, ParamsSHA256: input.Request.Codex.ParamsSHA256, RequestResolved: true, TurnClosed: true}
	closed, err := f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, newID(), o)
	if err != nil || closed.State != "closed" {
		t.Fatal(closed, err)
	}
	answer := ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: closed.Version, IdempotencyKey: newID(), Answers: []cc.CodexUserInputSelection{{QuestionID: "design", Kind: "skip"}}}
	_, err = f.s.AnswerToolInput(ctx, f.actor, input.ID, answer)
	requireAuthorityStatus(t, err, 409)
	// Unanswered native closure is a valid fact, and cannot manufacture an answer.
	result := toolInputResult187(h, b, input, o)
	bounded, tx, cancel, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.validateCodexUserInputResultTx(bounded, tx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal(err)
	}
	tx.Rollback()
	cancel()
	other, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, toolInputRequest187(b, "write187"))
	if err != nil {
		t.Fatal(err)
	}
	answer.ExpectedVersion = 1
	answer.IdempotencyKey = newID()
	answered, err := f.s.AnswerToolInput(ctx, f.actor, other.ID, answer)
	if err != nil {
		t.Fatal(err)
	}
	started := cc.CodexUserInputObservation{Sequence: 1, ParamsSHA256: other.Request.Codex.ParamsSHA256, AnswerSHA256: answered.Answer.AnswerSHA256, AnswerObserved: true, ReplyStarted: true}
	if _, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, other.ID, newID(), started); err != nil {
		t.Fatal(err)
	}
	unknown := started
	unknown.Sequence++
	unknown.RequestResolved = true
	unknown.TurnClosed = true
	unknown.FailureCode = "reply_write_unknown"
	if _, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, other.ID, newID(), unknown); err != nil {
		t.Fatal("native closure erased an already-started write fact", err)
	}
	result.ToolEvidence.CodexExecution.UserInputs = append(result.ToolEvidence.CodexExecution.UserInputs, toolInputResult187(h, b, other, unknown).ToolEvidence.CodexExecution.UserInputs[0])
	bounded, tx, cancel, err = f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	defer tx.Rollback()
	if err = f.s.validateCodexUserInputResultTx(bounded, tx, f.user.TenantID, f.project, result); err == nil {
		t.Fatal("unknown native write accepted as success")
	}
	result.ExitCode = 1
	result.Error = "Controlled unknown response"
	if err = f.s.validateCodexUserInputResultTx(bounded, tx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal("failed execution lost exact original unknown evidence", err)
	}
	result.ToolEvidence.CodexExecution.UserInputs[1].Observation.Sequence++
	if err = f.s.validateCodexUserInputResultTx(bounded, tx, f.user.TenantID, f.project, result); err == nil {
		t.Fatal("forged non-persisted observation accepted")
	}
}

func TestToolInputSharesNativeIDAndAttemptCapWithPermissions(t *testing.T) {
	h, b := newCodexToolInteractionFixture(t, false, true)
	ctx := context.Background()
	f := h.f
	request := toolInputRequest187(b, "shared187")
	if _, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, request); err != nil {
		t.Fatal(err)
	}
	approval := codexToolRequest174(b, "shared187")
	approval.Codex.ThreadID = request.Codex.ThreadID
	if _, err := h.open(ctx, approval); err == nil {
		t.Fatal("approval reused an input native RPC ID")
	}
	approval = codexToolRequest174(b, "approval-first187")
	approval.Codex.ThreadID = request.Codex.ThreadID
	if _, err := h.open(ctx, approval); err != nil {
		t.Fatal(err)
	}
	reverse := toolInputRequest187(b, "approval-first187")
	if _, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, reverse); err == nil {
		t.Fatal("input reused an approval native RPC ID")
	}
	for i := 2; i < 32; i++ {
		r := toolInputRequest187(b, fmt.Sprintf("bounded-%d", i))
		if _, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, r); err != nil {
			t.Fatal(i, err)
		}
	}
	if _, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, toolInputRequest187(b, "overflow187")); err == nil {
		t.Fatal("input exceeded combined cap")
	}
	approval = codexToolRequest174(b, "approval-overflow187")
	if _, err := h.open(ctx, approval); err == nil {
		t.Fatal("approval exceeded combined cap")
	}
	noCap, b2 := newCodexToolPermissionFixture(t, false)
	_, err := noCap.f.s.OpenToolInput(ctx, noCap.actor, noCap.f.project, noCap.assignment, toolInputRequest187(b2, "not-enabled187"))
	requireAuthorityStatus(t, err, 409)
}

func TestToolInputInitialSchemaRejectsMissingNativeUniquenessBeforeWrites(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "malformed-input.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{`CREATE TABLE suma_schema_baselines(component VARCHAR(40) PRIMARY KEY,version VARCHAR(80) NOT NULL)`, `INSERT INTO suma_schema_baselines(component,version)VALUES('platform','` + store.InitialDataContract + `')`, `CREATE TABLE user_kept_fact(value TEXT NOT NULL)`, `INSERT INTO user_kept_fact(value)VALUES('keep this original database')`, strings.Replace(toolInputDDL, `,UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence,source_session_sha256,source_request_sha256)`, "", 1)} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	s := &Service{DB: db, Dialect: "sqlite"}
	if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("malformed registered tool inputs were accepted", err)
	}
	var kept string
	var count int
	if err = db.QueryRow(`SELECT value FROM user_kept_fact`).Scan(&kept); err != nil || kept != "keep this original database" {
		t.Fatal(kept, err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&count); err != nil || count != 3 {
		t.Fatal("schema rejection wrote other domain tables", count, err)
	}
}
