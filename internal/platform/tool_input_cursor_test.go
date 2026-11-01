package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	"github.com/ai-mada/suma/internal/dag"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

func cursorInputFixture190(t *testing.T, pooled bool) (toolPermissionFixture, TaskDispatchPreviewRequest) {
	t.Helper()
	h, source := cursorACPFixture189(t, pooled)
	f, ctx := h.f, context.Background()
	p := cursorACPProfile189()
	p.CommandID, p.ProfileSHA256, p.UserInputProtocol = "cursor-input190", strings.Repeat("d", 64), cu.InputProtocol
	var err error
	h.profile, err = f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: h.profile.WorkerID, Name: "Explicit Cursor original-execution inputs", Profile: p, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	cursorACPInventory189(t, h, p, true)
	h.profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, h.profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: h.profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Original Cursor input scope", Nodes: []dag.Node{{ID: "work", Type: "agent", Command: []string{p.CommandID}, Config: map[string]any{"goal": "Develop under the original fixed model and workspace", "tool_profile_id": h.profile.ID, "tool_profile_sha256": p.ProfileSHA256, "acp_execution": issueRepairRuntimeDecode[map[string]any](t, p.ACPExecution)}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	claimed := cursorACPClaim189(t, h)
	if json.Unmarshal([]byte(stringify(claimed["assignment"])), &h.assignment) != nil || h.assignment.RunID != run.ID {
		t.Fatal(claimed)
	}
	return h, source
}

func cursorInputRequest190(h toolPermissionFixture, id, method string) ic.ToolInputRequest {
	native := cu.CursorInputRequest{SessionID: "cursor-session190", PromptID: cu.CursorRPCRequestID{Kind: "number", Value: "2"}, RequestID: cu.CursorRPCRequestID{Kind: "string", Value: id}, Method: method, ConfigurationSHA256: h.profile.Profile.ACPExecution.ConfigurationSHA256}
	if method == cu.AskQuestionMethod {
		native.Questions = &cu.CursorAskQuestion{ToolCallID: "call-" + id, Title: "Exact original choices", Questions: []cu.CursorQuestion{{ID: "single", Prompt: "Choose the reviewed approach", Options: []cu.CursorQuestionOption{{ID: "keep", Label: "Keep current"}, {ID: "refine", Label: "Refine"}}}, {ID: "multiple", Prompt: "Choose the required checks", Options: []cu.CursorQuestionOption{{ID: "first", Label: "First"}, {ID: "second", Label: "Second"}}, AllowMultiple: true}}}
	} else {
		native.Plan = &cu.CursorCreatePlan{ToolCallID: "call-" + id, Name: "Original plan", Overview: "Confirm this exact proposal only", Plan: "# Exact plan\nPreserve the existing model, workspace and quality gates.\n", Todos: []cu.CursorTodo{{ID: "review", Content: "Implement under original permissions", Status: "pending"}}}
	}
	native.ParamsSHA256, _ = cu.ParamsDigest(native)
	return ic.ToolInputRequest{Protocol: ac.Protocol, IdempotencyKey: newID(), Cursor: &native}
}

func cursorOpenInput190(t *testing.T, h toolPermissionFixture, r ic.ToolInputRequest) ic.ToolInput {
	t.Helper()
	var out ic.ToolInput
	var err error
	if h.pool {
		out, err = h.f.s.OpenPoolToolInput(context.Background(), h.token, h.f.project, h.assignment, r)
	} else {
		out, err = h.f.s.OpenToolInput(context.Background(), h.actor, h.f.project, h.assignment, r)
	}
	if err != nil {
		t.Fatal("original current Cursor input was not admitted", err)
	}
	return out
}

func cursorInputProof190(v ic.ToolInput) cu.CursorInputEvidence {
	s := v.Request.Cursor
	kind, count := "plan", 0
	if s.Questions != nil {
		kind, count = "questions", len(s.Questions.Questions)
	}
	return cu.CursorInputEvidence{InputID: v.ID, RequestID: s.RequestID, PromptID: s.PromptID, SessionID: s.SessionID, Method: s.Method, ConfigurationSHA256: s.ConfigurationSHA256, ParamsSHA256: s.ParamsSHA256, Kind: kind, QuestionCount: count, Observation: v.Observation}
}

func cursorOpenInputHTTP190(t *testing.T, h toolPermissionFixture, request ic.ToolInputRequest) ic.ToolInput {
	t.Helper()
	var reply map[string]any
	if h.pool {
		reply = h.f.bearer("POST", "/pool-runner/tool-inputs", h.token, PoolToolInputOpenRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: h.f.project, Assignment: h.assignment}, Request: request}, 200)
	} else {
		reply = h.f.bearer("POST", "/runner/tool-inputs", h.token, ToolInputOpenRequest{ProjectID: h.f.project, Assignment: h.assignment, Request: request}, 200)
	}
	return issueRepairRuntimeDecode[ic.ToolInput](t, reply)
}

func cursorInputResult190(h toolPermissionFixture, proofs []cu.CursorInputEvidence) dag.Result {
	return dag.Result{RunID: h.assignment.RunID, NodeID: h.assignment.NodeID, Attempt: h.assignment.Attempt, Fence: h.assignment.Fence, WorkerID: h.assignment.WorkerID, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, SessionID: "cursor-session190", Terminal: "completed", EventCount: 4, UsageState: tc.UsageUnreported, ACPExecution: &ac.Evidence{ConfigurationSHA256: h.profile.Profile.ACPExecution.ConfigurationSHA256, RequestedModel: h.profile.Profile.ACPExecution.Model, SelectedModel: h.profile.Profile.ACPExecution.Model, ModelSelectionAcknowledged: true, SessionID: "cursor-session190", ProtocolVersion: 1, PromptStarted: true, PromptCompleted: true, StopReason: "end_turn", ToolsComplete: true, HostProcessStopped: true, UserInputs: proofs}}}
}

func TestCursorToolInputsKeepQuestionsPlansAndOriginalMachineScope(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(fmt.Sprint(pooled), func(t *testing.T) {
			h, taskSource := cursorInputFixture190(t, pooled)
			f, ctx := h.f, context.Background()
			var proofs []cu.CursorInputEvidence
			for i, outcome := range []string{"answered", "accepted", "rejected", "cancelled"} {
				method := cu.CreatePlanMethod
				if i == 0 {
					method = cu.AskQuestionMethod
				}
				request := cursorInputRequest190(h, fmt.Sprintf("native-%d", i), method)
				input := cursorOpenInputHTTP190(t, h, request)
				list, err := f.s.ListToolInputs(ctx, f.actor, f.project, h.assignment.RunID, h.assignment.NodeID, h.assignment.Attempt, h.assignment.Fence)
				canAnswer := false
				for _, item := range list.Items {
					canAnswer = canAnswer || item.ID == input.ID && len(item.AllowedActions) == 1
				}
				if err != nil || list.Protocol != cu.InputProtocol || len(list.Items) != i+1 || !canAnswer {
					t.Fatal(list, err)
				}
				decision := &cu.CursorInputDecision{Outcome: outcome}
				if i == 0 {
					decision.Answers = []cu.CursorInputSelection{{QuestionID: "multiple", OptionIDs: []string{"second", "first"}}, {QuestionID: "single", OptionIDs: []string{"keep"}}}
				}
				answer := ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: input.Version, IdempotencyKey: newID(), Cursor: decision}
				answered := issueRepairRuntimeDecode[ic.ToolInput](t, f.call("POST", "/tool-inputs/"+input.ID+"/answer", candidateJSON(t, answer), 200))
				if answered.Answer == nil || answered.Answer.Cursor == nil || answered.Answer.Cursor.Outcome != outcome {
					t.Fatal(answered)
				}
				var polled ic.ToolInput
				if pooled {
					polled, err = f.s.PollPoolToolInput(ctx, h.token, f.project, h.assignment, input.ID)
				} else {
					polled, err = f.s.PollToolInput(ctx, h.actor, f.project, h.assignment, input.ID)
				}
				if err != nil || polled.Answer == nil || polled.Answer.AnswerSHA256 != answered.Answer.AnswerSHA256 {
					t.Fatal(polled, err)
				}
				o := delivery.Observation{Sequence: 1, ParamsSHA256: request.Cursor.ParamsSHA256, AnswerSHA256: answered.Answer.AnswerSHA256, AnswerObserved: true, ReplyStarted: true, ReplyWritten: true, ExecutionClosed: true}
				if pooled {
					input, err = f.s.ObservePoolToolInput(ctx, h.token, f.project, h.assignment, input.ID, newID(), o)
				} else {
					input, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, newID(), o)
				}
				if err != nil || input.State != "closed" || input.Observation.RequestResolved {
					t.Fatal(input, err)
				}
				proofs = append(proofs, cursorInputProof190(input))
			}
			result := cursorInputResult190(h, proofs)
			if pooled {
				f.bearer("POST", "/pool-runner/results", h.token, PoolRunnerResultRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, Result: result}, 200)
			} else {
				f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
			}
			currentTask, err := f.s.Get(ctx, f.actor, f.project, "tasks", taskSource.TaskID)
			if err != nil || currentTask.Version != taskSource.ExpectedTaskVersion || currentTask.RunID != "" || currentTask.Status == "accepted" {
				t.Fatal("native plan confirmation changed a formal task", currentTask, err)
			}
		})
	}
}

func TestCursorToolInputHumanCASPauseAndUnknownReceiptRemainSeparate(t *testing.T) {
	h, _ := cursorInputFixture190(t, false)
	f, ctx := h.f, context.Background()
	input := cursorOpenInput190(t, h, cursorInputRequest190(h, "compete190", cu.AskQuestionMethod))
	inv, err := f.s.CreateInvitation(ctx, f.actor, map[string]any{"email": "cursor-input190@example.test", "role": "member", "grants": []any{map[string]any{"project_id": f.project, "role": "developer"}}, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := f.s.AcceptInvitation(ctx, map[string]any{"token": inv["token"], "email": "cursor-input190@example.test", "name": "Cursor input author", "password": "cursor-input-strong-fixture190", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	actors := []Actor{f.actor, second}
	requests := []ic.ToolInputAnswerRequest{}
	for _, outcome := range []string{"skipped", "cancelled"} {
		requests = append(requests, ic.ToolInputAnswerRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID(), Cursor: &cu.CursorInputDecision{Outcome: outcome}})
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
	for i, failure := range failures {
		if failure == nil {
			if winner != -1 {
				t.Fatal("both original human decisions won")
			}
			winner = i
		} else {
			requireAuthorityStatus(t, failure, 409)
		}
	}
	if winner < 0 {
		t.Fatal(failures)
	}
	if _, err = f.s.AnswerToolInput(ctx, h.actor, input.ID, requests[winner]); err == nil {
		t.Fatal("machine submitted a human decision")
	}
	answer := results[winner].Answer
	if _, err = f.s.DB.Exec(`UPDATE dag_runs SET state='paused' WHERE id=?`, h.assignment.RunID); err != nil {
		t.Fatal(err)
	}
	polled, err := f.s.PollToolInput(ctx, h.actor, f.project, h.assignment, input.ID)
	if err != nil || polled.State != "closed" {
		t.Fatal("pause delivered a new native response", polled, err)
	}
	started := delivery.Observation{Sequence: 1, ParamsSHA256: input.Request.Cursor.ParamsSHA256, AnswerSHA256: answer.AnswerSHA256, AnswerObserved: true, ReplyStarted: true}
	if _, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, newID(), started); err == nil {
		t.Fatal("native write crossed paused execution")
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_runs SET state='running' WHERE id=?`, h.assignment.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, newID(), started); err != nil {
		t.Fatal(err)
	}
	resolved := started
	resolved.Sequence++
	resolved.RequestResolved = true
	if _, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, newID(), resolved); err == nil {
		t.Fatal("Cursor manufactured a Codex resolved ACK")
	}
	unknown := started
	unknown.Sequence++
	unknown.ExecutionClosed, unknown.FailureCode = true, "input_native_response_unknown"
	operation := newID()
	observed, err := f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, operation, unknown)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0,state='succeeded' WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, operation, unknown)
	if err != nil || stringify(replay) != stringify(observed) {
		t.Fatal("original Cursor observation receipt did not recover", err)
	}
	if _, err = f.s.AnswerToolInput(ctx, actors[winner], input.ID, requests[winner]); err != nil {
		t.Fatal("original human decision receipt did not recover", err)
	}
	if _, err = f.s.ObserveToolInput(ctx, h.actor, f.project, h.assignment, input.ID, newID(), unknown); err == nil {
		t.Fatal("closed execution authorized a new observation")
	}
	wrong := h.assignment
	wrong.Fence++
	if _, err = f.s.ObserveToolInput(ctx, h.actor, f.project, wrong, input.ID, operation, unknown); err == nil {
		t.Fatal("wrong original fence recovered a receipt")
	}
	result := cursorInputResult190(h, []cu.CursorInputEvidence{cursorInputProof190(observed)})
	bounded, tx, cancel, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	defer tx.Rollback()
	if err = f.s.validateToolInputResultTx(bounded, tx, f.user.TenantID, f.project, result); err == nil {
		t.Fatal("unknown native response became success")
	}
	result.ExitCode, result.Error, result.ToolEvidence.Terminal = 1, "Controlled original native response unknown", "failed"
	if err = f.s.validateToolInputResultTx(bounded, tx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal("failed result lost exact original unknown facts", err)
	}
	result.ToolEvidence.ACPExecution.UserInputs[0].Observation.Sequence++
	if err = f.s.validateToolInputResultTx(bounded, tx, f.user.TenantID, f.project, result); err == nil {
		t.Fatal("non-persisted Cursor observation accepted")
	}
}

func TestCursorToolInputNativeNamespaceAndSharedAttemptLimit(t *testing.T) {
	h, _ := cursorInputFixture190(t, false)
	f, ctx := h.f, context.Background()
	if _, err := h.open(ctx, permissionRequest170()); err != nil {
		t.Fatal(err)
	}
	r := cursorInputRequest190(h, "tool170", cu.AskQuestionMethod)
	r.Cursor.SessionID = "session170" // Same text as permission IDs is a different native namespace.
	first := cursorOpenInput190(t, h, r)
	duplicate := cursorInputRequest190(h, "tool170", cu.CreatePlanMethod)
	duplicate.Cursor.SessionID = r.Cursor.SessionID
	if _, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, duplicate); err == nil {
		t.Fatal("one native server request acquired two kinds")
	}
	changed := r
	source := *r.Cursor
	changed.Cursor = &source
	changed.Cursor.ConfigurationSHA256 = strings.Repeat("f", 64)
	if _, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, changed); err == nil {
		t.Fatal("different configuration recovered an input")
	}
	for i := 1; i < 31; i++ {
		method := cu.AskQuestionMethod
		if i%2 == 0 {
			method = cu.CreatePlanMethod
		}
		next := cursorInputRequest190(h, fmt.Sprintf("bounded-%d", i), method)
		if i == 1 || i == 2 {
			next.Cursor.RequestID.Value = "7"
			if i == 2 {
				next.Cursor.RequestID.Kind = "number"
			}
		}
		cursorOpenInput190(t, h, next)
	}
	if _, err := f.s.OpenToolInput(ctx, h.actor, f.project, h.assignment, cursorInputRequest190(h, "overflow190", cu.CreatePlanMethod)); err == nil {
		t.Fatal("plan reset the shared native attempt limit")
	}
	var inputs, permissions int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tool_inputs WHERE run_id=?`, h.assignment.RunID).Scan(&inputs); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tool_permissions WHERE run_id=?`, h.assignment.RunID).Scan(&permissions); err != nil {
		t.Fatal(err)
	}
	if inputs != 31 || permissions != 1 || first.ID == "" {
		t.Fatal(inputs, permissions)
	}
}
