package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// This fixture reuses the protocol-only original reply receipt. Actual OS/CLI,
// workspace edits, candidate tests and browser journeys have separate evidence.
func taskDiscussionDevelopmentFixture(t *testing.T) (*taskReplyFixture, TaskDiscussionDevelopmentCreateRequest) {
	t.Helper()
	h := newTaskReplyFixture(t)
	f, ctx := h.f, context.Background()
	reply, result := h.start(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	reply, err := f.s.GetTaskConversationReply(ctx, f.e, f.actor, f.project, reply.ID)
	if err != nil || reply.State != "completed" {
		t.Fatal(reply, err)
	}
	var profileID string
	if err = f.s.DB.QueryRow(`SELECT id FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id<>?`, f.user.TenantID, f.project, h.profile.ID).Scan(&profileID); err != nil {
		t.Fatal(err)
	}
	profile, err := f.s.GetToolProfile(ctx, f.actor, f.project, profileID)
	if err != nil {
		t.Fatal(err)
	}
	options, err := f.s.GetModelExecutionOptions(ctx, f.actor, f.project, ModelExecutionOptionsRequest{ProjectID: f.project, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version})
	if err != nil || len(options.Items) < 1 {
		t.Fatal(options, err)
	}
	selection := nativeWorkspaceSelectionForTest(t)
	// Bind the exact deployment advertised by this fixture's current worker;
	// the reply's separate one-call selection is intentionally not copied.
	selection.DeploymentID = options.Items[0].Deployment.ID
	selection.DeploymentRevision = options.Items[0].Deployment.Revision
	selection.DeploymentSHA256 = options.Items[0].Deployment.SHA256
	selection.Parameters.MaxSteps, selection.Parameters.MaxOutputTokens = 2, 100
	request := TaskDiscussionDevelopmentCreateRequest{ProjectID: f.project, TaskID: h.request.TaskID, ExpectedTaskVersion: h.request.ExpectedTaskVersion, ReplyID: reply.ID, ExpectedReplyVersion: reply.Version, WorkflowID: "engineering", WorkflowVersion: 1, AgentNodeID: "develop", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, ModelExecutionSelection: selection, ReviewedGoal: "Implement the reviewed permission fix in the isolated worktree; preserve existing quality gates.", AcceptanceCriteria: []string{"Keep the independent exact-candidate test", "Return evidence for human acceptance"}, AcknowledgeExecution: true, IdempotencyKey: newID()}
	return h, request
}

func TestTaskDiscussionDevelopmentPreservesSourceAndUsesOriginalDispatcher(t *testing.T) {
	h, request := taskDiscussionDevelopmentFixture(t)
	f, ctx := h.f, context.Background()
	before := domainACount(t, f.s, "dag_runs")
	plan, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != plan.DispatchPlan.ID || plan.Version != 1 || plan.Message.ActorKind != "assistant" || plan.Message.ReplyID != request.ReplyID || plan.ReplyVersion != request.ExpectedReplyVersion || plan.DispatchPlan.TaskVersion != request.ExpectedTaskVersion || plan.DispatchPlan.ModelExecution == nil || plan.DispatchPlan.ModelExecution.Parameters.MaxSteps != 2 || plan.DispatchPlan.ModelExecution.Parameters.MaxOutputTokens != 100 {
		t.Fatal("reviewed source or fresh development selection lost", plan)
	}
	goal, _ := plan.DispatchPlan.Workflow.Nodes[0].Config["goal"].(string)
	if !strings.Contains(goal, request.ReviewedGoal) || !strings.Contains(goal, request.AcceptanceCriteria[0]) || !strings.Contains(goal, plan.SourceSHA256) || plan.DispatchPlan.Workflow.Nodes[1].Type != "test" {
		t.Fatal("editable objective or independent candidate test lost", goal)
	}
	if domainACount(t, f.s, "dag_runs") != before {
		t.Fatal("preview started development")
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", request.TaskID)
	if err != nil || current.Version != request.ExpectedTaskVersion || current.RunID != "" || current.Status == "accepted" {
		t.Fatal("preview rewrote the formal task", current, err)
	}
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}
	run, err := f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil || run.ID != plan.DispatchPlan.RunID {
		t.Fatal("handoff did not use original task dispatcher", run, err)
	}
	if again, e := f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, start); e != nil || again.ID != run.ID || domainACount(t, f.s, "dag_runs") != before+1 {
		t.Fatal("original start receipt was not recovered", again, e)
	}
	got, err := f.s.GetTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID)
	if err != nil || got.Version != 2 || got.DispatchPlan.State != "started" || got.Message.ID != plan.Message.ID {
		t.Fatal("started plan lost its immutable conversation provenance", got, err)
	}
	current, err = f.s.Get(ctx, f.actor, f.project, "tasks", request.TaskID)
	if err != nil || current.RunID != run.ID || current.Version != request.ExpectedTaskVersion+1 || current.Status == "accepted" {
		t.Fatal("original binding or acceptance gate lost", current, err)
	}
	if again, e := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request); e != nil || again.ID != plan.ID || again.Version != 2 {
		t.Fatal("preview recovery fabricated another plan", again, e)
	}
}

func TestTaskDiscussionDevelopmentRejectsStaleUnverifiedAndUnconsentedSources(t *testing.T) {
	h, request := taskDiscussionDevelopmentFixture(t)
	f, ctx := h.f, context.Background()
	for _, mutate := range []func(*TaskDiscussionDevelopmentCreateRequest){
		func(r *TaskDiscussionDevelopmentCreateRequest) { r.AcknowledgeExecution = false },
		func(r *TaskDiscussionDevelopmentCreateRequest) { r.ReplyID = newID() },
		func(r *TaskDiscussionDevelopmentCreateRequest) { r.ExpectedReplyVersion++ },
		func(r *TaskDiscussionDevelopmentCreateRequest) { r.ExpectedTaskVersion++ },
		func(r *TaskDiscussionDevelopmentCreateRequest) { r.AcceptanceCriteria = nil },
		func(r *TaskDiscussionDevelopmentCreateRequest) { r.ModelExecutionSelection = nil },
		func(r *TaskDiscussionDevelopmentCreateRequest) {
			r.ToolProfileID, r.ExpectedToolVersion = h.profile.ID, h.profile.Version
		},
	} {
		r := request
		r.IdempotencyKey = newID()
		mutate(&r)
		if _, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, r); err == nil {
			t.Fatal("invalid source or scope created a plan")
		}
	}
	if domainACount(t, f.s, "suma_task_discussion_development_plans") != 0 || domainACount(t, f.s, "suma_task_dispatch_plans") != 0 {
		t.Fatal("rejected preview left partial authority")
	}
	_, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, toolActorForTest(t, f, h.token), f.project, request)
	requireAuthorityStatus(t, err, 403)
	f.call("PATCH", "/tasks/"+request.TaskID, f.body(map[string]any{"expected_version": request.ExpectedTaskVersion, "title": "Changed after reply"}), 200)
	_, err = f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	requireAuthorityStatus(t, err, 409)
}

func TestTaskDiscussionDevelopmentStrictHTTPAndProjectScope(t *testing.T) {
	h, request := taskDiscussionDevelopmentFixture(t)
	f, ctx := h.f, context.Background()
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	wrong := map[string]any{}
	for k, v := range body {
		wrong[k] = v
	}
	wrong["source_sha256"] = strings.Repeat("f", 64)
	f.call("POST", "/task-discussion-development-plans", wrong, 400)
	got := f.call("POST", "/task-discussion-development-plans", body, 201)
	id := got["id"].(string)
	f.call("GET", "/task-discussion-development-plans/"+id+"?project_id="+f.project+"&unknown=1", nil, 400)
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	f.bearer("GET", "/task-discussion-development-plans/"+id+"?project_id="+f.project, issued.Token, nil, 200)
	f.bearer("GET", "/task-discussion-development-plans/"+id+"?project_id="+newID(), issued.Token, nil, 403)
	// Reusing a saved preview operation with different reviewed text conflicts.
	body["reviewed_goal"] = "A different reviewed objective"
	f.call("POST", "/task-discussion-development-plans", body, 409)
}
