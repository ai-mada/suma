package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The reply and tool receipts are controlled protocol fixtures. No installed
// CLI, provider, candidate execution or customer acceptance is asserted here.
func discussionCodexFixture(t *testing.T) (*taskReplyFixture, map[string]any, ToolProfile, cc.Catalog, ProjectKnowledgeDetail) {
	t.Helper()
	h, ctx := newTaskReplyFixture(t), context.Background()
	f := h.f
	memory := codexSkillPublished(t, f, "memory", "Keep the approved discussion memory 182.")
	skill := codexSkillPublished(t, f, "skill", "Original approved native skill 182.\nPreserve the existing approval authority.")
	post := taskConversationRequest(f, h.request.TaskID, h.request.ExpectedTaskVersion, "Review these exact publications before implementation.")
	post.Knowledge = []ProjectKnowledgeReference{codexSkillReference(memory), codexSkillReference(skill)}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, post)
	if err != nil {
		t.Fatal(err)
	}
	h.request.ExpectedLastSequence, h.request.ReplyToSequence = message.Sequence, message.Sequence
	reply, result := h.start(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	reply, err = f.s.GetTaskConversationReply(ctx, f.e, f.actor, f.project, reply.ID)
	if err != nil || reply.State != "completed" {
		t.Fatal(reply, err)
	}
	request := taskDiscussionDevelopmentRequestForReply(t, h, reply)
	profile := tc.Profile{CommandID: "discussion-codex182", Adapter: "codex", Protocol: cc.Protocol, ProfileSHA256: strings.Repeat("c", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "controlled discussion Codex fixture"}
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: h.profile.WorkerID, Name: "Discussion Codex", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := cc.NewCatalog(strings.Repeat("d", 64), []cc.ModelOption{{Model: "reviewed-codex182", ReasoningEfforts: []string{"low", "high"}, DefaultReasoningEffort: "low"}})
	if err != nil {
		t.Fatal(err)
	}
	reportCodexCatalog(t, f, h.token, p, catalog)
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err = json.Unmarshal([]byte(stringify(request)), &body); err != nil {
		t.Fatal(err)
	}
	delete(body, "model_execution_selection")
	body["tool_profile_id"], body["expected_tool_version"] = p.ID, p.Version
	body["codex_execution_selection"] = &cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "reviewed-codex182", ReasoningEffort: "high", TokenNoticeLimit: usageInt(12)}
	return h, body, p, catalog, skill
}

func TestDiscussionDevelopmentCodexFreezesSourceSkillsAndRecoversOriginalRun(t *testing.T) {
	h, body, profile, catalog, skill := discussionCodexFixture(t)
	f, ctx := h.f, context.Background()
	before := domainACount(t, f.s, "dag_runs")
	response := f.call("POST", "/task-discussion-development-plans", body, 201)
	var plan TaskDiscussionDevelopmentPlan
	if err := json.Unmarshal([]byte(stringify(response)), &plan); err != nil {
		t.Fatal(err)
	}
	bound := plan.DispatchPlan.CodexExecution
	if bound == nil || bound.Model != "reviewed-codex182" || bound.ReasoningEffort != "high" || bound.ConfigurationSHA256 != catalog.ConfigurationSHA256 || bound.BudgetMode != "observe_only" || plan.DispatchPlan.ModelExecution != nil || domainACount(t, f.s, "dag_runs") != before {
		t.Fatal("Codex preview lost its original model authority or started execution", plan)
	}
	if plan.Message.ReplyID != body["reply_id"] || plan.Message.ID == "" || plan.Message.Sequence < 1 || plan.Message.ActorKind != "assistant" || plan.ReplyVersion != int(body["expected_reply_version"].(float64)) || len(plan.SourceSHA256) != 64 || len(plan.Message.Knowledge) != 2 || len(plan.DispatchPlan.Context.Knowledge) != 2 {
		t.Fatal("exact assistant source or ordered pins were lost", plan)
	}
	if err := checkTaskConversationKnowledgeSnapshot([]TaskConversationMessage{plan.Message}, plan.DispatchPlan.Context.Knowledge); err != nil {
		t.Fatal("the plan consumed different publications from its exact source", err)
	}
	skills := plan.DispatchPlan.ExecutionSkills
	if skills == nil || len(skills.Documents) != 1 || skills.Documents[0].ID != skill.ID || skills.Documents[0].SHA256 != skill.PublishedSHA256 || skills.Documents[0].Revision != skill.PublishedRevision {
		t.Fatal("source skills were appended after the immutable execution bundle", skills)
	}
	node := codexPlanNode(t, plan.DispatchPlan)
	goal, _ := node.Config["goal"].(string)
	if strings.Contains(goal, skill.Published.Content) || !strings.Contains(goal, "$suma-") || !strings.Contains(goal, "Keep the approved discussion memory 182.") || !strings.Contains(goal, body["reviewed_goal"].(string)) || !strings.Contains(goal, plan.SourceSHA256) || node.MaxRetries != 0 {
		t.Fatal("reviewed source bypassed native skill projection or original scope", goal)
	}
	if plan.DispatchPlan.Workflow.Nodes[1].Type != "test" || plan.DispatchPlan.Workflow.Nodes[1].Config["candidate_from"] != "develop" {
		t.Fatal("the independent exact-candidate quality handoff was changed")
	}
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}
	run, err := f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil || run.ID != plan.DispatchPlan.RunID {
		t.Fatal("the original dispatcher did not start its fixed run", run, err)
	}
	claimed := f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if err = json.Unmarshal([]byte(stringify(claimed["assignment"])), &assignment); err != nil || assignment.RunID != run.ID {
		t.Fatal("the original Codex attempt was not claimed", claimed, err)
	}
	actual, err := cc.DecodeExecutionSkillBundle(assignment.Config["execution_skills"])
	model, modelErr := cc.DecodeBinding(assignment.Config["codex_execution"])
	if err != nil || modelErr != nil || stringify(actual) != stringify(skills) || stringify(model) != stringify(bound) || assignment.Config["discussion_source_sha256"] != plan.SourceSHA256 {
		t.Fatal("claim changed the reviewed model, source or native skill bundle", err, modelErr)
	}
	// Later input retirement does not invalidate the immutable original receipt
	// or authorize creating a second plan, run or Codex turn.
	if _, err = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, skill.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, ExpectedVersion: skill.Version, Action: "retire", IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	response = f.call("POST", "/task-discussion-development-plans", body, 201)
	if response["id"] != plan.ID || response["source_sha256"] != plan.SourceSHA256 || response["version"] != float64(2) {
		t.Fatal("lost preview response did not recover the exact original plan", response)
	}
	again, err := f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil || again.ID != run.ID || domainACount(t, f.s, "dag_runs") != before+1 || domainACount(t, f.s, "suma_task_discussion_development_plans") != 1 {
		t.Fatal("lost start response repeated an external execution", again, err)
	}
}

func TestDiscussionDevelopmentCodexRejectsSelectionAndSourceDriftWithoutPartialPlans(t *testing.T) {
	h, body, _, _, _ := discussionCodexFixture(t)
	f := h.f
	for _, test := range []struct {
		name   string
		status int
		change func(map[string]any)
	}{
		{"missing Codex selection", 400, func(r map[string]any) { delete(r, "codex_execution_selection") }},
		{"both selection protocols", 400, func(r map[string]any) { r["model_execution_selection"] = nativeWorkspaceSelectionForTest(t) }},
		{"stale catalog", 409, func(r map[string]any) {
			r["codex_execution_selection"] = &cc.Selection{CatalogSHA256: strings.Repeat("f", 64), Model: "reviewed-codex182", ReasoningEffort: "high"}
		}},
		{"stale exact reply", 409, func(r map[string]any) { r["expected_reply_version"] = r["expected_reply_version"].(float64) + 1 }},
		{"caller knowledge injection", 400, func(r map[string]any) { r["knowledge"] = []ProjectKnowledgeReference{} }},
		{"read-only reply tool", 400, func(r map[string]any) {
			delete(r, "codex_execution_selection")
			r["tool_profile_id"], r["expected_tool_version"] = h.profile.ID, h.profile.Version
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := map[string]any{}
			for key, value := range body {
				r[key] = value
			}
			r["idempotency_key"] = newID()
			test.change(r)
			f.call("POST", "/task-discussion-development-plans", r, test.status)
		})
	}
	codexPolicy(t, f, "project", "enforce", 0)
	f.call("POST", "/task-discussion-development-plans", body, 400)
	if domainACount(t, f.s, "suma_task_discussion_development_plans") != 0 || domainACount(t, f.s, "suma_task_dispatch_plans") != 0 {
		t.Fatal("rejected handoffs left partial plans")
	}
}

func TestDiscussionDevelopmentFixedProfileUsesOriginalDispatcherWithoutModelSelection(t *testing.T) {
	h, request := taskDiscussionDevelopmentFixture(t)
	f, ctx := h.f, context.Background()
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: h.profile.WorkerID, Name: "Reviewed fixed Claude", Profile: claudeTestProfile(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, h.token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	request.ToolProfileID, request.ExpectedToolVersion, request.ModelExecutionSelection = profile.ID, profile.Version, nil
	plan, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil || plan.DispatchPlan.ModelExecution != nil || plan.DispatchPlan.CodexExecution != nil || !claudeFactMatches(codexPlanNode(t, plan.DispatchPlan).Config["claude_execution"], profile.Profile.ClaudeExecution) {
		t.Fatal("fixed development configuration was forced into another model protocol", plan, err)
	}
	_, err = f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal("the original dispatcher rejected its existing fixed configuration", err)
	}
}
