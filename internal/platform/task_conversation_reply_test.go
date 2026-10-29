package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type taskReplyFixture struct {
	f       *fixture
	profile ToolProfile
	token   string
	request TaskConversationReplyCreateRequest
}

func newTaskReplyFixture(t *testing.T) *taskReplyFixture {
	t.Helper()
	ctx := context.Background()
	f, _, preview, _, deployments := modelExecutionFixture(t)
	worker := "conversation-worker-128"
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	definition := toolTestProfile()
	definition.CommandID, definition.ProfileSHA256 = "reply128", strings.Repeat("f", 64)
	definition.ReadOnly, definition.ModelExecutionProtocol = true, mc.Protocol
	definition.TaskConversationReply = &tc.TaskConversationReplyCapability{Protocol: tc.TaskConversationReplyProtocol}
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Read-only conversation protocol fixture", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: definition, Available: true, ModelDeployments: deployments[:1]}}}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var original TaskDispatchPreviewRequest
	if err = json.Unmarshal([]byte(stringify(preview)), &original); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, original.TaskID, original.ExpectedTaskVersion, "请说明当前任务的实现建议，不要执行命令。")); err != nil {
		t.Fatal(err)
	}
	d := deployments[0]
	selection := &mc.Selection{DeploymentID: d.ID, DeploymentRevision: d.Revision, DeploymentSHA256: d.SHA256, Parameters: mc.Parameters{MaxSteps: 1, MaxOutputTokens: 128}}
	request := TaskConversationReplyCreateRequest{ProjectID: f.project, TaskID: original.TaskID, ExpectedTaskVersion: original.ExpectedTaskVersion, ExpectedLastSequence: 1, ReplyToSequence: 1, ContextMessageLimit: 20, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, ModelExecutionSelection: selection, Locale: "zh-CN", AcknowledgeModelUse: true, IdempotencyKey: newID()}
	return &taskReplyFixture{f: f, profile: profile, token: token, request: request}
}

func (h *taskReplyFixture) create(t *testing.T) TaskConversationReply {
	t.Helper()
	a, err := h.f.s.CreateTaskConversationReply(context.Background(), h.f.e, h.f.actor, h.f.project, h.request)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// Protocol-only receipt helper. Actual SDK/model-gateway execution is tested
// separately; these values do not claim a provider request or supplier charge.
func (h *taskReplyFixture) start(t *testing.T) (TaskConversationReply, dag.Result) {
	t.Helper()
	a := h.create(t)
	claimed := h.f.bearer("POST", "/runner/claim", h.token, h.f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if err := json.Unmarshal([]byte(stringify(claimed["assignment"])), &assignment); err != nil || assignment.RunID != a.RunID || assignment.NodeID != "reply" {
		guardCtx, guard, closeGuard, guardErr := h.f.s.planningAuthority(context.Background(), h.f.actor, h.f.project, h.request.TaskID, purposeProjectWrite)
		if guardErr == nil {
			guardErr = h.f.s.checkTaskConversationReplyExecutionTx(guardCtx, guard.tx, h.f.user.TenantID, h.f.project, a.RunID)
			closeGuard()
		}
		t.Fatal("missing original managed reply assignment", claimed, err, "domain admission", guardErr)
	}
	b := a.ModelExecution
	result := dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID(), Output: `{"schema_version":1,"reply":"先审阅当前需求与验收条件。这是建议，尚未执行开发或验收。"}`, ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 3, UsageState: tc.UsageComplete, Usage: &tc.Usage{InputTokens: usageInt(2), OutputTokens: usageInt(3)}, ModelExecution: &mc.Evidence{GatewayProtocol: mc.GatewayProtocol, GatewayClosed: true, BindingSHA256: b.BindingSHA256, DeploymentSHA256: b.Deployment.SHA256, ParametersSHA256: b.ParametersSHA256, Model: b.Deployment.Model, Requests: []mc.RequestEvidence{{State: "completed", Step: 0, RequestSHA256: strings.Repeat("a", 64), ResponseSHA256: strings.Repeat("b", 64), ProviderModel: b.Deployment.Model, OutputAllowance: b.Parameters.MaxOutputTokens, UsageState: tc.UsageComplete, Usage: &mc.RequestUsage{InputTokens: usageInt(2), OutputTokens: usageInt(3)}}}}}}
	return a, result
}

func TestTaskConversationReplyManagedTurnAndOriginalReceipt(t *testing.T) {
	h, ctx := newTaskReplyFixture(t), context.Background()
	f := h.f
	a, result := h.start(t)
	if a.ModelExecution.Parameters.MaxSteps != 1 || a.Source.Coverage.LastSequence != 1 || len(a.Source.Messages) != 1 || !a.Source.Coverage.Complete {
		t.Fatal("model or conversation input was not fixed", a)
	}
	if again := h.create(t); again.ID != a.ID || again.RunID != a.RunID {
		t.Fatal("lost create acknowledgement repeated the model run")
	}
	req := h.request
	req.IdempotencyKey = newID()
	_, err := f.s.CreateTaskConversationReply(ctx, f.e, f.actor, f.project, req)
	requireAuthorityStatus(t, err, 409)
	// A later human turn is legal, but the first assistant still answers turn1.
	if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, req.TaskID, req.ExpectedTaskVersion, "这是稍后补充的问题；不能改变正在执行的第一轮上下文。")); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	got, err := f.s.GetTaskConversationReply(ctx, f.e, f.actor, f.project, a.ID)
	if err != nil || got.State != "completed" || got.MessageID == "" || got.MessageSequence != 3 {
		t.Fatal("exact original completion was not posted once", got, err)
	}
	page, err := f.s.ListTaskConversation(ctx, f.actor, f.project, req.TaskID, 20, 0)
	if err != nil || page.Total != 3 || page.Items[2].ActorKind != "assistant" || page.Items[2].ReplyID != a.ID || page.Items[2].ReplyToSequence != 1 || page.Items[2].ExecutionRunID != a.RunID || page.Items[2].RunID != "" {
		t.Fatal("assistant attribution or separate execution provenance was lost", page, err)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", req.TaskID)
	if err != nil || task.RunID != "" || task.Version != req.ExpectedTaskVersion || task.Status == "accepted" {
		t.Fatal("conversation reply changed engineering task or acceptance", task, err)
	}
	// Continue from the new human turn; prior human/assistant text is fixed.
	req.IdempotencyKey, req.ExpectedLastSequence, req.ReplyToSequence = newID(), 3, 2
	second, err := f.s.CreateTaskConversationReply(ctx, f.e, f.actor, f.project, req)
	if err != nil || len(second.Source.Messages) != 3 || second.Source.Messages[2].ReplyID != a.ID || second.Source.ReplyToSequence != 2 {
		t.Fatal("continuation lost actual conversation history", second, err)
	}
}

func TestTaskConversationReplyStaleCompletionAndCancellation(t *testing.T) {
	t.Run("changed source preserves terminal facts without posting", func(t *testing.T) {
		h := newTaskReplyFixture(t)
		f, ctx := h.f, context.Background()
		a, result := h.start(t)
		f.call("PATCH", "/tasks/"+h.request.TaskID, f.body(map[string]any{"expected_version": h.request.ExpectedTaskVersion, "title": "Changed task after model admission"}), 200)
		f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
		got, err := f.s.GetTaskConversationReply(ctx, f.e, f.actor, f.project, a.ID)
		if err != nil || got.State != "failed" || got.BasisCurrent || got.MessageID != "" || domainACount(t, f.s, "suma_task_conversation_reply_outcomes") != 1 || domainACount(t, f.s, "suma_task_conversation_reply_active") != 0 {
			t.Fatal("stale terminal result became an assistant message or lost facts", got, err)
		}
	})
	t.Run("queued cancellation is atomic and replayable", func(t *testing.T) {
		h := newTaskReplyFixture(t)
		f, ctx := h.f, context.Background()
		a := h.create(t)
		req := TaskConversationReplyCommandRequest{ProjectID: f.project, ExpectedVersion: a.Version, ExpectedRunVersion: a.RunVersion, Action: "cancel", IdempotencyKey: newID()}
		got, err := f.s.CommandTaskConversationReply(ctx, f.e, f.actor, f.project, a.ID, req)
		if err != nil || got.State != "cancelled" || domainACount(t, f.s, "suma_task_conversation_reply_active") != 0 {
			t.Fatal("never-started cancelled reply retained activity", got, err)
		}
		again, err := f.s.CommandTaskConversationReply(ctx, f.e, f.actor, f.project, a.ID, req)
		if err != nil || again.ID != got.ID || again.State != got.State || again.Version != got.Version {
			t.Fatal("cancel replay lost its original operation", again, err)
		}
	})
}

func TestTaskConversationReplyStrictScopeAndContextWindow(t *testing.T) {
	h, ctx := newTaskReplyFixture(t), context.Background()
	f := h.f
	for _, change := range []func(*TaskConversationReplyCreateRequest){
		func(r *TaskConversationReplyCreateRequest) { r.AcknowledgeModelUse = false },
		func(r *TaskConversationReplyCreateRequest) { r.ContextMessageLimit = 0 },
		func(r *TaskConversationReplyCreateRequest) { r.ExpectedLastSequence = 2 },
		func(r *TaskConversationReplyCreateRequest) { r.ReplyToSequence = 2 },
		func(r *TaskConversationReplyCreateRequest) { r.ExpectedTaskVersion++ },
	} {
		req := h.request
		req.IdempotencyKey = newID()
		change(&req)
		if _, err := f.s.CreateTaskConversationReply(ctx, f.e, f.actor, f.project, req); err == nil {
			t.Fatal("ambiguous or unconfirmed source started a model run")
		}
	}
	if domainACount(t, f.s, "dag_runs") != 0 {
		t.Fatal("invalid requests had effects")
	}
	_, err := f.s.CreateTaskConversationReply(ctx, f.e, toolActorForTest(t, f, h.token), f.project, h.request)
	requireAuthorityStatus(t, err, 403)
	for i := 0; i < 21; i++ {
		if _, err = f.s.AppendTaskConversation(ctx, f.actor, f.project, taskConversationRequest(f, h.request.TaskID, h.request.ExpectedTaskVersion, "A bounded newer human turn")); err != nil {
			t.Fatal(err)
		}
	}
	h.request.ExpectedLastSequence, h.request.ReplyToSequence, h.request.ContextMessageLimit = 22, 22, 3
	a := h.create(t)
	if a.Source.Coverage.Complete || a.Source.Coverage.Total != 22 || a.Source.Coverage.Included != 3 || a.Source.Coverage.FirstSequence != 20 || len(a.Source.Messages) != 3 {
		t.Fatal("explicit context window was silently summarized or broadened", a.Source)
	}
	f.call("GET", "/task-conversation-replies?project_id="+f.project+"&task_id="+h.request.TaskID+"&limit=01", nil, 400)
	for _, action := range []string{"retry", "approve", "reconcile", "compensate"} {
		f.call("POST", "/task-conversation-replies/"+a.ID+"/commands", f.body(map[string]any{"expected_version": a.Version, "expected_run_version": a.RunVersion, "action": action}), 400)
	}
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	optionsPath := "/model-execution/options?project_id=" + f.project + "&tool_profile_id=" + h.profile.ID + "&expected_tool_version=" + strings.TrimSpace(stringify(h.profile.Version))
	options := f.bearer("GET", optionsPath, issued.Token, nil, 200)
	limits, ok := options["parameter_limits"].(map[string]any)
	items, itemsOK := options["items"].([]any)
	if options["supported"] != true || !ok || limits["max_steps"] != float64(1) || limits["max_output_tokens"] != float64(8192) || !itemsOK || len(items) != 1 {
		t.Fatal("project API did not discover the same current single-call model limits", options)
	}
	f.bearer("GET", strings.Replace(optionsPath, f.project, newID(), 1), issued.Token, nil, 403)
	// The issued credential keeps its original identity, but both organization
	// and project authority are read again. A project viewer grant alone cannot
	// revoke an organization owner's inherited write authority.
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'viewer') ON CONFLICT(project_id,user_id) DO UPDATE SET role='viewer'`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	// This owner also registered the fixture worker. Revoking its write role
	// correctly invalidates that inventory; it must not be advertised as ready.
	f.bearer("GET", optionsPath, issued.Token, nil, 409)
	readOnly := f.bearer("GET", "/task-conversation-replies/"+a.ID+"?project_id="+f.project, issued.Token, nil, 200)
	if len(readOnly["allowed_actions"].([]any)) != 0 {
		t.Fatal("current viewer retained reply write controls", readOnly)
	}
	h.request.IdempotencyKey = newID()
	f.bearer("POST", "/task-conversation-replies", issued.Token, h.request, 403)
}
