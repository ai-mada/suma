package platform

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	sc "github.com/ai-mada/suma/internal/steeringcontract"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func steeringFixture188(t *testing.T, pooled bool) (toolPermissionFixture, TaskDispatchPlan, Record, TaskConversationMessage) {
	t.Helper()
	h, binding := newCodexToolPermissionFixture(t, pooled)
	f, ctx := h.f, context.Background()
	// Release the original generic fixture through its real result transport;
	// the new task uses the original managed preview/Start rather than fake rows.
	ended := dag.Result{RunID: h.assignment.RunID, NodeID: h.assignment.NodeID, Attempt: h.assignment.Attempt, Fence: h.assignment.Fence, WorkerID: h.assignment.WorkerID, OperationID: newID(), ExitCode: 1, Error: "Controlled generic fixture ended before task dispatch", ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, Terminal: "failed", UsageState: tc.UsageUnreported, CodexExecution: &cc.Evidence{BindingSHA256: binding.BindingSHA256, RequestedModel: binding.Model, RequestedReasoningEffort: binding.ReasoningEffort, AcknowledgedModel: binding.Model, AcknowledgedReasoningEffort: binding.ReasoningEffort, AcknowledgedProvider: "owned-fixture", ConfigurationAcknowledged: true}}}
	ended.ToolEvidence.SessionID = "generic188"
	ended.ToolEvidence.CodexExecution.ThreadID = "generic188"
	ended.ToolEvidence.CodexExecution.TurnID = "genericturn188"
	if pooled {
		f.bearer("POST", "/pool-runner/results", h.token, PoolRunnerResultRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, Result: ended}, 200)
	} else {
		f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": ended}), 200)
	}
	w, _, _ := taskDispatchBuildFixture()
	w.ID = ""
	w.Version = 0
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, w)
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Original live task steering"}))
	if err != nil {
		t.Fatal(err)
	}
	preview := TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: workflow.ID, WorkflowVersion: workflow.Version, AgentNodeID: "develop", ToolProfileID: h.profile.ID, ExpectedToolVersion: h.profile.Version, CodexExecutionSelection: &cc.Selection{CatalogSHA256: binding.CatalogSHA256, Model: binding.Model, ReasoningEffort: binding.ReasoningEffort}, IdempotencyKey: newID()}
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, preview)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var claim map[string]any
	if pooled {
		claim = f.bearer("POST", "/pool-runner/claim", h.token, dag.PoolClaimRequest{PoolID: h.assignment.PoolID, RunnerID: h.assignment.PoolRunnerID, WorkerID: h.assignment.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + h.profile.Profile.CommandID}}}}, 200)
	} else {
		claim = f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}, "idempotency_key": newID()}), 200)
	}
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &h.assignment) != nil || h.assignment.RunID != run.ID {
		t.Fatal(claim)
	}
	task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, TaskConversationAppendRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, Body: "Keep the reviewed interfaces stable while completing this task.", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return h, plan, task, message
}

func steeringOpen188(t *testing.T, h toolPermissionFixture, p TaskDispatchPlan) (sc.SteeringSession, cc.CodexSteeringSource) {
	t.Helper()
	source := cc.CodexSteeringSource{Protocol: cc.SteeringProtocol, ThreadID: "thread188", TurnID: "turn188", BindingSHA256: p.CodexExecution.BindingSHA256}
	out, err := h.f.s.OpenSteeringSession(context.Background(), h.actor, h.f.project, h.assignment, source, newID())
	if err != nil || out.Session == nil {
		t.Fatal(out, err)
	}
	return *out.Session, source
}
func steeringCreate188(t *testing.T, h toolPermissionFixture, task Record, m TaskConversationMessage, session sc.SteeringSession) (sc.SteeringInstruction, sc.SteeringCreateRequest) {
	t.Helper()
	r := sc.SteeringCreateRequest{ProjectID: h.f.project, TaskID: task.ID, MessageID: m.ID, ExpectedTaskVersion: int64(task.Version), SessionID: session.ID, ExpectedSessionVersion: session.Version, IdempotencyKey: newID()}
	out, err := h.f.s.CreateSteeringInstruction(context.Background(), h.f.actor, r)
	if err != nil {
		t.Fatal(err)
	}
	return out, r
}

func TestTurnSteeringOwnMessageAndOriginalReceiptAuthority(t *testing.T) {
	h, plan, task, message := steeringFixture188(t, false)
	f, ctx := h.f, context.Background()
	session, _ := steeringOpen188(t, h, plan)
	inv, err := f.s.CreateInvitation(ctx, f.actor, map[string]any{"email": "steering188@example.test", "role": "member", "grants": []any{map[string]any{"project_id": f.project, "role": "developer"}}, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	user, token, err := f.s.AcceptInvitation(ctx, map[string]any{"token": inv["token"], "email": "steering188@example.test", "name": "Steering developer", "password": "steering-strong-password-188", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := f.s.AppendTaskConversation(ctx, second, f.project, TaskConversationAppendRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, Body: "Preserve the selected behavior.", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.s.GetTaskSteeringContext(ctx, f.actor, f.project, task.ID, foreign.ID)
	if err != nil || view.CanSend || view.Reason != "not_own_message" || view.Message.Body != foreign.Body {
		t.Fatal(view, err)
	}
	r := sc.SteeringCreateRequest{ProjectID: f.project, TaskID: task.ID, MessageID: foreign.ID, ExpectedTaskVersion: int64(task.Version), SessionID: session.ID, ExpectedSessionVersion: 1, IdempotencyKey: newID()}
	_, err = f.s.CreateSteeringInstruction(ctx, f.actor, r)
	requireAuthorityStatus(t, err, 403)
	created, err := f.s.CreateSteeringInstruction(ctx, second, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateSteeringInstruction(ctx, second, r)
	requireAuthorityStatus(t, err, 403)
	poll, err := f.s.PollSteering(ctx, h.actor, f.project, h.assignment, session.ID)
	if err != nil || len(poll.Instructions) != 0 {
		t.Fatal("withdrawn original sender still delivered", poll, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, user.ID); err != nil {
		t.Fatal(err)
	}
	_, own := steeringCreate188(t, h, task, message, session)
	if _, err = f.s.Update(ctx, f.actor, f.project, "tasks", task.ID, f.body(map[string]any{"expected_version": task.Version, "description": "Current task changed after explicit send"})); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.CreateSteeringInstruction(ctx, f.actor, own)
	if err != nil || replay.Version != 1 || replay.Observation != nil {
		t.Fatal("receipt was re-admitted as new work", replay, err)
	}
	fresh := own
	fresh.IdempotencyKey = newID()
	_, err = f.s.CreateSteeringInstruction(ctx, f.actor, fresh)
	requireAuthorityStatus(t, err, 409)
	_, err = f.s.CreateSteeringInstruction(ctx, h.actor, own)
	requireAuthorityStatus(t, err, 403)
	view, err = f.s.GetTaskSteeringContext(ctx, f.actor, f.project, task.ID, foreign.ID)
	if err != nil || len(view.Instructions) != 1 || view.Instructions[0].ID != created.ID {
		t.Fatal("history disappeared with fresh source drift", view, err)
	}
}

func TestTurnSteeringFreshGateAndStartedFactsAreSeparate(t *testing.T) {
	h, p, task, m := steeringFixture188(t, false)
	f, ctx := h.f, context.Background()
	session, source := steeringOpen188(t, h, p)
	instruction, _ := steeringCreate188(t, h, task, m, session)
	sha, _ := cc.SteeringParamsDigest(source, instruction.ID, m.Body)
	o := cc.CodexSteeringObservation{Sequence: 1, RequestID: cc.CodexRPCRequestID{Kind: "number", Value: "5"}, ParamsSHA256: sha, Observed: true, WriteStarted: true}
	if _, err := f.s.DB.Exec(`UPDATE dag_runs SET state='paused' WHERE id=?`, h.assignment.RunID); err != nil {
		t.Fatal(err)
	}
	poll, err := f.s.PollSteering(ctx, h.actor, f.project, h.assignment, session.ID)
	requireAuthorityStatus(t, err, 409)
	if len(poll.Instructions) != 0 {
		t.Fatal(poll, err)
	}
	_, err = f.s.ObserveSteering(ctx, h.actor, f.project, h.assignment, session.ID, instruction.ID, newID(), o)
	if err == nil {
		t.Fatal("new write crossed pause")
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_runs SET state='running' WHERE id=?`, h.assignment.RunID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ObserveSteering(ctx, h.actor, f.project, h.assignment, session.ID, instruction.ID, newID(), o); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	late := o
	late.Sequence++
	late.WriteWritten = true
	late.NativeAccepted = true
	late.TurnClosed = true
	key := newID()
	closed, err := f.s.ObserveSteering(ctx, h.actor, f.project, h.assignment, session.ID, instruction.ID, key, late)
	if err != nil {
		t.Fatal("actual started facts were rejected by new-work lease gate", err)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='succeeded' WHERE run_id=? AND id=?`, h.assignment.RunID, h.assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	recovered, err := f.s.ObserveSteering(ctx, h.actor, f.project, h.assignment, session.ID, instruction.ID, key, late)
	if err != nil || stringify(recovered) != stringify(closed) {
		t.Fatal(recovered, err)
	}
	wrong := h.assignment
	wrong.Fence++
	_, err = f.s.ObserveSteering(ctx, h.actor, f.project, wrong, session.ID, instruction.ID, key, late)
	if err == nil {
		t.Fatal("wrong original scope recovered a receipt")
	}
}

func TestTurnSteeringClosedPollIncludesUndeliveredTailAndUnknownBlocksSuccess(t *testing.T) {
	h, p, task, m := steeringFixture188(t, false)
	f, ctx := h.f, context.Background()
	session, source := steeringOpen188(t, h, p)
	first, _ := steeringCreate188(t, h, task, m, session)
	sha, _ := cc.SteeringParamsDigest(source, first.ID, m.Body)
	o := cc.CodexSteeringObservation{Sequence: 1, RequestID: cc.CodexRPCRequestID{Kind: "number", Value: "5"}, ParamsSHA256: sha, Observed: true, WriteStarted: true}
	if _, err := f.s.ObserveSteering(ctx, h.actor, f.project, h.assignment, session.ID, first.ID, newID(), o); err != nil {
		t.Fatal(err)
	}
	tail, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, TaskConversationAppendRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, Body: "Keep this final saved detail too.", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	second, _ := steeringCreate188(t, h, task, tail, session)
	if _, err = f.s.CloseSteeringSession(ctx, h.actor, f.project, h.assignment, session.ID, newID()); err != nil {
		t.Fatal(err)
	}
	poll, err := f.s.PollSteering(ctx, h.actor, f.project, h.assignment, session.ID)
	if err != nil || poll.Session.State != "closed" || len(poll.Instructions) != 2 {
		t.Fatal(poll, err)
	}
	e := &cc.CodexSteeringEvidence{SessionID: session.ID, Source: source, Closed: true, Instructions: []cc.CodexSteeringInstructionEvidence{{InstructionID: first.ID, MessageSHA256: sc.MessageDigest(m.Body), Observation: &o}, {InstructionID: second.ID, MessageSHA256: sc.MessageDigest(tail.Body)}}}
	result := dag.Result{RunID: h.assignment.RunID, NodeID: h.assignment.NodeID, Attempt: h.assignment.Attempt, Fence: h.assignment.Fence, WorkerID: h.assignment.WorkerID, ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, CodexExecution: &cc.Evidence{BindingSHA256: source.BindingSHA256, ThreadID: source.ThreadID, TurnID: source.TurnID, Steering: e}}}
	bounded, tx, cancel, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	defer tx.Rollback()
	if err = f.s.validateCodexSteeringResultTx(bounded, tx, f.user.TenantID, f.project, result); err == nil {
		t.Fatal("unknown native ACK accepted as success")
	}
	result.ExitCode = 1
	result.Error = "Controlled unknown original steer"
	if err = f.s.validateCodexSteeringResultTx(bounded, tx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal("original unknown facts lost", err)
	}
	e.Instructions[0].Observation = nil
	if err = f.s.validateCodexSteeringResultTx(bounded, tx, f.user.TenantID, f.project, result); err == nil {
		t.Fatal("missing original persisted started evidence accepted")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	cancel()
	// A late exact native rejection resolves the started write; the tail remains
	// honestly undelivered and does not have to invent a native observation.
	o.Sequence++
	o.WriteWritten = true
	o.NativeRejected = true
	o.TurnClosed = true
	if _, err = f.s.ObserveSteering(ctx, h.actor, f.project, h.assignment, session.ID, first.ID, newID(), o); err != nil {
		t.Fatal(err)
	}
	e.Instructions[0].Observation = &o
	result.ExitCode = 0
	result.Error = ""
	bounded, tx, cancel2, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel2()
	defer tx.Rollback()
	if err = f.s.validateCodexSteeringResultTx(bounded, tx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal("known rejected write and never-dispatched tail could not close", err)
	}
}

func TestTurnSteeringNormalWorkflowAndMessageLimitRemainExplicit(t *testing.T) {
	ctx := context.Background()
	generic, b := newCodexToolPermissionFixture(t, false)
	normal, err := generic.f.s.OpenSteeringSession(ctx, generic.actor, generic.f.project, generic.assignment, cc.CodexSteeringSource{Protocol: cc.SteeringProtocol, ThreadID: "normal188", TurnID: "normalturn188", BindingSHA256: b.BindingSHA256}, newID())
	if err != nil || normal.Session != nil || normal.Reason != "not_task_execution" {
		t.Fatal(normal, err)
	}
	h, p, task, m := steeringFixture188(t, false)
	f := h.f
	session, _ := steeringOpen188(t, h, p)
	_, r := steeringCreate188(t, h, task, m, session)
	r.IdempotencyKey = newID()
	_, err = f.s.CreateSteeringInstruction(ctx, f.actor, r)
	requireAuthorityStatus(t, err, 409)
	for i := 1; i < 32; i++ {
		body := fmt.Sprintf("Saved steering detail %d", i)
		if i == 1 {
			body = strings.Repeat("𐀀", cc.MaxSteeringTextBytes/4)
		}
		message, e := f.s.AppendTaskConversation(ctx, f.actor, f.project, TaskConversationAppendRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, Body: body, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		instruction, _ := steeringCreate188(t, h, task, message, session)
		if instruction.Message.Body != body {
			t.Fatal("saved original message was truncated")
		}
	}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, TaskConversationAppendRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, Body: "Overflow saved detail", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	r.MessageID = message.ID
	r.IdempotencyKey = newID()
	_, err = f.s.CreateSteeringInstruction(ctx, f.actor, r)
	requireAuthorityStatus(t, err, 409)
}

func TestTurnSteeringHTTPPublishesExactLiveTaskSession(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(fmt.Sprint(pooled), func(t *testing.T) {
			h, plan, task, message := steeringFixture188(t, pooled)
			f, ctx := h.f, context.Background()
			source := cc.CodexSteeringSource{Protocol: cc.SteeringProtocol, ThreadID: "thread188", TurnID: "turn188", BindingSHA256: plan.CodexExecution.BindingSHA256}
			key := newID()
			prefix := "/runner"
			var body any
			if pooled {
				prefix = "/pool-runner"
				body = PoolSteeringSessionOpenRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, Source: source, IdempotencyKey: key}
			} else {
				body = SteeringSessionOpenRequest{ProjectID: f.project, Assignment: h.assignment, Source: source, IdempotencyKey: key}
			}
			got := f.bearer("POST", prefix+"/steering/sessions", h.token, body, 200)
			var opened sc.SteeringOpenResult
			if json.Unmarshal([]byte(stringify(got)), &opened) != nil || opened.Session == nil {
				t.Fatal(got)
			}
			session := *opened.Session
			f.bearer("POST", prefix+"/steering/sessions", h.token, body, 200)
			context, err := f.s.GetTaskSteeringContext(ctx, f.actor, f.project, task.ID, message.ID)
			if err != nil || !context.CanSend || context.Reason != "" || len(context.Targets) != 1 {
				t.Fatal(context, err)
			}
			request := sc.SteeringCreateRequest{ProjectID: f.project, TaskID: task.ID, MessageID: message.ID, ExpectedTaskVersion: int64(task.Version), SessionID: session.ID, ExpectedSessionVersion: session.Version, IdempotencyKey: newID()}
			posted := f.call("POST", "/task-execution-steering", candidateJSON(t, request), 200)
			var instruction sc.SteeringInstruction
			if json.Unmarshal([]byte(stringify(posted)), &instruction) != nil || instruction.Version != 1 || instruction.Observation != nil || instruction.Message.Body != message.Body {
				t.Fatal(posted)
			}
			var poll sc.SteeringPoll
			if pooled {
				poll, err = f.s.PollPoolSteering(ctx, h.token, f.project, h.assignment, session.ID)
			} else {
				poll, err = f.s.PollSteering(ctx, h.actor, f.project, h.assignment, session.ID)
			}
			if err != nil || len(poll.Instructions) != 1 {
				t.Fatal(poll, err)
			}
			sha, err := cc.SteeringParamsDigest(source, instruction.ID, message.Body)
			if err != nil {
				t.Fatal(err)
			}
			o := cc.CodexSteeringObservation{Sequence: 1, RequestID: cc.CodexRPCRequestID{Kind: "number", Value: "5"}, ParamsSHA256: sha, Observed: true, WriteStarted: true, WriteWritten: true, NativeAccepted: true, TurnClosed: true}
			observationKey := newID()
			if pooled {
				body = PoolSteeringObservationRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, SessionID: session.ID, InstructionID: instruction.ID, IdempotencyKey: observationKey, Observation: o}
			} else {
				body = SteeringObservationRequest{ProjectID: f.project, Assignment: h.assignment, SessionID: session.ID, InstructionID: instruction.ID, IdempotencyKey: observationKey, Observation: o}
			}
			f.bearer("POST", prefix+"/steering/observations", h.token, body, 200)
			f.bearer("POST", prefix+"/steering/observations", h.token, body, 200)
			if pooled {
				body = PoolSteeringCloseRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, SessionID: session.ID, IdempotencyKey: newID()}
			} else {
				body = SteeringCloseRequest{ProjectID: f.project, Assignment: h.assignment, SessionID: session.ID, IdempotencyKey: newID()}
			}
			f.bearer("POST", prefix+"/steering/close", h.token, body, 200)
			f.bearer("POST", prefix+"/steering/close", h.token, body, 200)
			if pooled {
				poll, err = f.s.PollPoolSteering(ctx, h.token, f.project, h.assignment, session.ID)
			} else {
				poll, err = f.s.PollSteering(ctx, h.actor, f.project, h.assignment, session.ID)
			}
			if err != nil || poll.Session.State != "closed" || len(poll.Instructions) != 1 {
				t.Fatal(poll, err)
			}
			if replay, e := f.s.CreateSteeringInstruction(ctx, f.actor, request); e != nil || replay.Version != 1 || replay.Observation != nil {
				t.Fatal("historical create receipt replaced with live head", replay, e)
			}
			result := dag.Result{RunID: h.assignment.RunID, NodeID: h.assignment.NodeID, Attempt: h.assignment.Attempt, Fence: h.assignment.Fence, WorkerID: h.assignment.WorkerID, OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 3, UsageState: tc.UsageUnreported, CodexExecution: &cc.Evidence{BindingSHA256: source.BindingSHA256, ThreadID: source.ThreadID, TurnID: source.TurnID, RequestedModel: plan.CodexExecution.Model, RequestedReasoningEffort: plan.CodexExecution.ReasoningEffort, AcknowledgedModel: plan.CodexExecution.Model, AcknowledgedReasoningEffort: plan.CodexExecution.ReasoningEffort, AcknowledgedProvider: "owned", ConfigurationAcknowledged: true, Steering: &cc.CodexSteeringEvidence{SessionID: session.ID, Source: source, Closed: true, Instructions: []cc.CodexSteeringInstructionEvidence{{InstructionID: instruction.ID, MessageSHA256: sc.MessageDigest(message.Body), Observation: &o}}}}}}
			result.ToolEvidence.SessionID = source.ThreadID
			if pooled {
				body = PoolRunnerResultRequest{PoolRunnerAssignmentRequest: PoolRunnerAssignmentRequest{RunnerID: h.assignment.PoolRunnerID, TenantID: h.assignment.TenantID, ProjectID: f.project, Assignment: h.assignment}, Result: result}
			} else {
				body = f.body(map[string]any{"result": result})
			}
			f.bearer("POST", prefix+"/results", h.token, body, 200)
			f.bearer("POST", prefix+"/results", h.token, body, 200)
		})
	}
}

func TestTurnSteeringInitialSchemaRejectsMissingUniquenessBeforeWrites(t *testing.T) {
	for _, def := range []struct{ name, ddl, key string }{
		{"session", steeringSessionDDL, `,UNIQUE(tenant_id,project_id,run_id,node_id,attempt,fence)`},
		{"instruction", steeringInstructionDDL, `,UNIQUE(session_id,message_id)`},
	} {
		t.Run(def.name, func(t *testing.T) {
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "malformed-steering.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			for _, statement := range []string{`CREATE TABLE suma_schema_baselines(component VARCHAR(40) PRIMARY KEY,version VARCHAR(80) NOT NULL)`, `INSERT INTO suma_schema_baselines(component,version)VALUES('platform','` + store.InitialDataContract + `')`, `CREATE TABLE user_kept_fact(value TEXT NOT NULL)`, `INSERT INTO user_kept_fact(value)VALUES('keep original database')`, strings.Replace(def.ddl, def.key, "", 1)} {
				if _, err = db.Exec(statement); err != nil {
					t.Fatal(err)
				}
			}
			s := &Service{DB: db, Dialect: "sqlite"}
			if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("malformed steering schema accepted", err)
			}
			var kept string
			var count int
			if err = db.QueryRow(`SELECT value FROM user_kept_fact`).Scan(&kept); err != nil || kept != "keep original database" {
				t.Fatal(kept, err)
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&count); err != nil || count != 3 {
				t.Fatal("schema rejection wrote domain tables", count, err)
			}
		})
	}
}
