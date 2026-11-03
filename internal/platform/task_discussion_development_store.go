package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) InitializeTaskDiscussionDevelopment(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_task_discussion_development_plans(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,reply_id VARCHAR(64) NOT NULL,message_id VARCHAR(64) NOT NULL,payload TEXT NOT NULL)`))
	return err
}

func (s *Service) taskDiscussionDevelopmentSourceTx(ctx context.Context, auth *authorizedCommand, project string, request TaskDiscussionDevelopmentCreateRequest) (TaskConversationReply, TaskConversationMessage, error) {
	r, err := s.taskConversationReplyTx(ctx, auth.tx, auth.target.tenantID, project, request.ReplyID)
	if err != nil {
		return TaskConversationReply{}, TaskConversationMessage{}, err
	}
	reply := r.Reply
	if reply.TaskID != request.TaskID || reply.State != "completed" || reply.Version != request.ExpectedReplyVersion || reply.MessageID == "" || reply.MessageSequence < 1 || reply.Source.Task.Version != request.ExpectedTaskVersion {
		return reply, TaskConversationMessage{}, conflict(discussionDevelopmentSource)
	}
	if err = s.checkTaskConversationReplySourceTx(ctx, auth, reply); err != nil {
		return reply, TaskConversationMessage{}, err
	}
	message, err := scanTaskConversation(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), auth.target.tenantID, project, request.TaskID, reply.MessageID))
	if err != nil {
		return reply, TaskConversationMessage{}, err
	}
	items := []TaskConversationMessage{message}
	if err = s.taskConversationMessageMetadataTx(ctx, auth.tx, auth.target.tenantID, project, request.TaskID, items); err != nil {
		return reply, TaskConversationMessage{}, err
	}
	message = items[0]
	if message.ActorKind != "assistant" || message.ReplyID != reply.ID || message.Sequence != reply.MessageSequence || message.ExecutionRunID != reply.RunID || message.TaskVersion != request.ExpectedTaskVersion {
		return reply, message, conflict(discussionDevelopmentSource)
	}
	return reply, message, nil
}

func (s *Service) taskDiscussionDevelopmentProjectionTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, project, id string) (out TaskDiscussionDevelopmentPlan, err error) {
	var raw, task, reply, message string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT task_id,reply_id,message_id,payload FROM suma_task_discussion_development_plans WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, id).Scan(&task, &reply, &message, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.ID != id || out.ProjectID != project || out.TaskID != task || out.ReplyID != reply || out.Message.ID != message || out.Message.ReplyID != reply || out.Message.ActorKind != "assistant" || !contentDigest.MatchString(out.SourceSHA256) {
		return TaskDiscussionDevelopmentPlan{}, conflict(discussionDevelopmentChanged)
	}
	if _, err = s.getRecordTx(ctx, auth, project, "tasks", task); err != nil {
		return TaskDiscussionDevelopmentPlan{}, err
	}
	if out.Message.RequirementID != "" {
		if _, err = s.getRecordTx(ctx, auth, project, "requirements", out.Message.RequirementID); err != nil {
			return TaskDiscussionDevelopmentPlan{}, err
		}
	}
	plan, err := s.taskDispatchPlanTx(ctx, auth.tx, auth.currentPrincipal, project, id)
	if err != nil {
		return TaskDiscussionDevelopmentPlan{}, err
	}
	if plan.TaskID != task || plan.RunID != out.DispatchPlan.RunID || plan.AgentNodeID != out.DispatchPlan.AgentNodeID || plan.Workflow.Nodes == nil || stringify(plan.DevelopmentContinuation) != stringify(out.DevelopmentContinuation) || !sameTaskDispatchTestBindings(plan.TestBindings, out.DispatchPlan.TestBindings) {
		return TaskDiscussionDevelopmentPlan{}, conflict(discussionDevelopmentChanged)
	}
	plan.Valid, plan.Reason = true, ""
	if check := s.validateTaskDispatchPlanTx(ctx, auth.tx, engine, auth, project, plan); check != nil {
		var api *APIError
		if !errors.As(check, &api) {
			return TaskDiscussionDevelopmentPlan{}, check
		}
		plan.Valid, plan.Reason = false, check.Error()
	}
	out.Version, out.DispatchPlan = plan.Version, plan
	return out, nil
}

func (s *Service) GetTaskDiscussionDevelopmentPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out TaskDiscussionDevelopmentPlan, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(discussionDevelopmentInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	return s.taskDiscussionDevelopmentProjectionTx(ctx, engine, auth, project, id)
}

func (s *Service) StartTaskDiscussionDevelopmentPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, request TaskDispatchStartRequest) (dag.Run, error) {
	// The dedicated relation authorizes this provenance view. The existing
	// dispatcher then rechecks authority, current context, route, tool, budget,
	// exact original operation and transactionally binds the task to its run.
	if _, err := s.GetTaskDiscussionDevelopmentPlan(ctx, engine, actor, project, id); err != nil {
		return dag.Run{}, err
	}
	return s.StartTaskDispatchPlan(ctx, engine, actor, project, id, request)
}
