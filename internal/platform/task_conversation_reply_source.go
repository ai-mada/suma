package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) taskConversationReplySourceTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, project string, req TaskConversationReplyCreateRequest) (out TaskConversationReplySource, err error) {
	task, err := s.getRecordTx(ctx, auth, project, "tasks", req.TaskID)
	if err != nil {
		return out, err
	}
	if task.Version != req.ExpectedTaskVersion {
		return out, conflict(conversationReplyChanged)
	}
	out = TaskConversationReplySource{Task: taskContextRecord(task), TaskRunID: task.RunID, ReplyToSequence: req.ReplyToSequence, Messages: []TaskConversationMessage{}}
	if task.RequirementID != "" {
		requirement, e := s.getRecordTx(ctx, auth, project, "requirements", task.RequirementID)
		if e != nil {
			return out, e
		}
		compact := taskContextRecord(requirement)
		out.Requirement = &compact
	}
	tenant := auth.target.tenantID
	var head int64
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT last_sequence FROM suma_task_conversation_heads WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, task.ID).Scan(&head)
	if errors.Is(err, sql.ErrNoRows) {
		return out, conflict(conversationReplyChanged)
	}
	if err != nil {
		return out, err
	}
	if head != req.ExpectedLastSequence {
		return out, conflict(conversationReplyChanged)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? ORDER BY sequence DESC LIMIT ?`), tenant, project, task.ID, req.ContextMessageLimit)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		message, e := scanTaskConversation(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Messages = append(out.Messages, message)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Messages) == 0 || out.Messages[0].Sequence != head {
		return out, conflict(conversationReplyChanged)
	}
	var latestHuman int64
	for _, message := range out.Messages {
		if message.ActorKind == "human" || message.ActorKind == "api" {
			latestHuman = message.Sequence
			break
		}
	}
	if latestHuman == 0 || latestHuman != req.ReplyToSequence {
		return out, conflict(conversationReplyChanged)
	}
	var answered int
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_task_conversation_reply_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND reply_to_sequence=?`), tenant, project, task.ID, req.ReplyToSequence).Scan(&answered); err != nil {
		return out, err
	}
	if answered > 0 {
		return out, conflict(conversationReplyAnswered)
	}
	for i, j := 0, len(out.Messages)-1; i < j; i, j = i+1, j-1 {
		out.Messages[i], out.Messages[j] = out.Messages[j], out.Messages[i]
	}
	if err = s.taskConversationMessageMetadataTx(ctx, auth.tx, tenant, project, task.ID, out.Messages); err != nil {
		return out, err
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, task.ID).Scan(&out.Coverage.Total); err != nil {
		return out, err
	}
	out.Coverage.Included, out.Coverage.FirstSequence, out.Coverage.LastSequence = len(out.Messages), out.Messages[0].Sequence, head
	out.Coverage.Complete = int64(len(out.Messages)) == out.Coverage.Total
	if len(stringify(map[string]any{"task": out.Task, "requirement": out.Requirement, "messages": out.Messages})) > 32000 {
		return out, invalid(conversationReplyContext)
	}
	if engine == nil {
		out.ExecutionRoute, err = dag.ReadProjectExecutionRouteTx(ctx, auth.tx, s.Dialect, tenant, project)
	} else {
		out.ExecutionRoute, err = engine.ReadProjectExecutionRouteTx(ctx, auth.tx, tenant, project)
	}
	if err != nil {
		return out, err
	}
	out.SHA256 = digest(stringify(out))
	return out, nil
}

// Check immutable inputs, not the live head: a collaborator can post a later
// note while a fixed turn runs. The reply-to identity makes that ordering clear.
func (s *Service) checkTaskConversationReplySourceTx(ctx context.Context, auth *authorizedCommand, a TaskConversationReply) error {
	task, err := s.getRecordTx(ctx, auth, a.ProjectID, "tasks", a.TaskID)
	if err != nil {
		return err
	}
	if stringify(taskContextRecord(task)) != stringify(a.Source.Task) || task.RunID != a.Source.TaskRunID {
		return conflict(conversationReplyChanged)
	}
	if a.Source.Requirement == nil {
		if task.RequirementID != "" {
			return conflict(conversationReplyChanged)
		}
	} else {
		requirement, e := s.getRecordTx(ctx, auth, a.ProjectID, "requirements", a.Source.Requirement.ID)
		if e != nil {
			return e
		}
		if task.RequirementID != requirement.ID || stringify(taskContextRecord(requirement)) != stringify(a.Source.Requirement) {
			return conflict(conversationReplyChanged)
		}
	}
	for _, original := range a.Source.Messages {
		message, e := scanTaskConversation(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), auth.target.tenantID, a.ProjectID, a.TaskID, original.ID))
		if errors.Is(e, sql.ErrNoRows) {
			return conflict(conversationReplyChanged)
		}
		if e != nil {
			return e
		}
		items := []TaskConversationMessage{message}
		if e = s.taskConversationMessageMetadataTx(ctx, auth.tx, auth.target.tenantID, a.ProjectID, a.TaskID, items); e != nil {
			return e
		}
		if stringify(items[0]) != stringify(original) {
			return conflict(conversationReplyChanged)
		}
	}
	return nil
}
