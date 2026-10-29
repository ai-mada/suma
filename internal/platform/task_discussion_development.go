package platform

import (
	"context"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
)

const (
	discussionDevelopmentInvalid = "Invalid discussion development handoff request"
	discussionDevelopmentChanged = "The discussion development basis changed; review the current task and reply"
	discussionDevelopmentSource  = "Select a verified completed assistant reply for the current task"
	discussionDevelopmentConsent = "Confirm the reviewed plan, selected model budget and real workspace execution"
	discussionDevelopmentProfile = "Select an enabled writable native workspace development profile"
	discussionDevelopmentLimit   = "The reviewed development context exceeds 32000 UTF-8 bytes; reduce its scope"
)

func validTaskDiscussionDevelopmentCreate(r TaskDiscussionDevelopmentCreateRequest) bool {
	if !handoffID(r.ProjectID, false) || !handoffID(r.TaskID, false) || !handoffID(r.ReplyID, false) ||
		!handoffID(r.WorkflowID, false) || !handoffID(r.AgentNodeID, false) || !handoffID(r.ToolProfileID, false) ||
		r.ExpectedTaskVersion < 1 || r.ExpectedTaskVersion > 2147483646 || r.ExpectedReplyVersion < 1 || r.ExpectedReplyVersion > 2147483646 ||
		r.WorkflowVersion < 1 || r.WorkflowVersion > 2147483646 || r.ExpectedToolVersion < 1 || r.ExpectedToolVersion > 2147483646 ||
		!planningKey(r.IdempotencyKey) || !testPlanText(r.ReviewedGoal, 8000, true) ||
		len(r.AcceptanceCriteria) < 1 || len(r.AcceptanceCriteria) > 32 || r.ModelExecutionSelection == nil || mc.ValidateSelection(*r.ModelExecutionSelection) != nil {
		return false
	}
	for _, criterion := range r.AcceptanceCriteria {
		if !testPlanText(criterion, 1000, true) {
			return false
		}
	}
	return true
}

func (s *Service) CreateTaskDiscussionDevelopmentPlan(ctx context.Context, engine *dag.Engine, actor Actor, project string, request TaskDiscussionDevelopmentCreateRequest) (out TaskDiscussionDevelopmentPlan, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if engine == nil || project != request.ProjectID || !validTaskDiscussionDevelopmentCreate(request) {
		return out, invalid(discussionDevelopmentInvalid)
	}
	if !request.AcknowledgeExecution {
		return out, invalid(discussionDevelopmentConsent)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, request.TaskID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"task_discussion_development_preview": request})
	if err != nil {
		return out, err
	}
	if replay {
		var receipt TaskDiscussionDevelopmentPlan
		if err = json.Unmarshal([]byte(hash), &receipt); err != nil {
			return out, err
		}
		// Recover the original preview identity even if it was started later.
		// Current actor/project read authority is rechecked before exposing it.
		return s.taskDiscussionDevelopmentProjectionTx(ctx, engine, auth, project, receipt.ID)
	}
	reply, message, err := s.taskDiscussionDevelopmentSourceTx(ctx, auth, project, request)
	if err != nil {
		return out, err
	}
	profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, project, request.ToolProfileID)
	if err != nil {
		return out, err
	}
	if profile.State != "enabled" || profile.Profile.ReadOnly || profile.Profile.Adapter != "suma" || profile.Profile.Protocol != "suma-jsonl-v1" || profile.Profile.WorkspaceToolsVersion != "attempt-workspace-v1" || profile.Profile.ModelExecutionProtocol != mc.Protocol {
		return out, invalid(discussionDevelopmentProfile)
	}
	preview := TaskDispatchPreviewRequest{ProjectID: project, TaskID: request.TaskID, ExpectedTaskVersion: request.ExpectedTaskVersion, WorkflowID: request.WorkflowID, WorkflowVersion: request.WorkflowVersion, AgentNodeID: request.AgentNodeID, ToolProfileID: request.ToolProfileID, ExpectedToolVersion: request.ExpectedToolVersion, ModelExecutionSelection: request.ModelExecutionSelection, IdempotencyKey: request.IdempotencyKey}
	plan, err := s.buildTaskDispatchSnapshotTx(ctx, engine, auth, project, preview)
	if err != nil {
		return out, err
	}
	plan.ID, plan.RunID = newID(), newID()
	sourceDigest := digest(stringify(map[string]any{"reply_id": reply.ID, "reply_version": reply.Version, "reply_source_sha256": reply.Source.SHA256, "message": message}))
	// JSON escapes markup in human-edited text. It is an explicit engineering
	// objective, never an extra tool, shell, workspace or approval capability.
	reviewed := stringify(map[string]any{"source_sha256": sourceDigest, "reply_id": reply.ID, "message_id": message.ID, "reviewed_goal": request.ReviewedGoal, "acceptance_criteria": request.AcceptanceCriteria})
	found := false
	for i := range plan.Workflow.Nodes {
		node := &plan.Workflow.Nodes[i]
		if node.ID != plan.AgentNodeID {
			continue
		}
		goal, _ := node.Config["goal"].(string)
		goal += "\nThe following is an explicitly reviewed engineering plan. It supplements the fixed task scope. It does not grant additional tools or permissions, replace published tests, or grant human acceptance.\n<reviewed_discussion_development_json>\n" + reviewed + "\n</reviewed_discussion_development_json>"
		if len(goal) > 32000 {
			return out, invalid(discussionDevelopmentLimit)
		}
		node.Config["goal"] = goal
		node.Config["discussion_development_plan_id"] = plan.ID
		node.Config["discussion_source_sha256"] = sourceDigest
		found = true
	}
	if !found || plan.ModelExecution == nil || plan.ModelExecution.Protocol != mc.Protocol {
		return out, invalid(discussionDevelopmentProfile)
	}
	if err = dag.Compile(plan.Workflow); err != nil {
		return out, err
	}
	if err = s.persistTaskDispatchSnapshotTx(ctx, auth, &plan); err != nil {
		return out, err
	}
	out = TaskDiscussionDevelopmentPlan{ID: plan.ID, ProjectID: project, TaskID: request.TaskID, Version: plan.Version, ReplyID: reply.ID, ReplyVersion: reply.Version, SourceSHA256: sourceDigest, Message: message, ReviewedGoal: request.ReviewedGoal, AcceptanceCriteria: append([]string{}, request.AcceptanceCriteria...), CreatedBy: auth.currentPrincipal.ID, CreatedAt: plan.CreatedAt, DispatchPlan: plan}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_discussion_development_plans(id,tenant_id,project_id,task_id,reply_id,message_id,payload)VALUES(?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, project, out.TaskID, out.ReplyID, out.Message.ID, stringify(out)); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "task.discussion_development_prepared", out.TaskID, "Reviewed discussion plan prepared for native development"); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
