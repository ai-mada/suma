package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) taskConversationReplyProjectionTx(ctx context.Context, _ *dag.Engine, auth *authorizedCommand, r taskConversationReplyRecord) (out TaskConversationReply, err error) {
	a := r.Reply
	// Current source authorization precedes freshness: an obsolete requirement
	// version is not an invitation to reveal a different project's fixed text.
	if _, err = s.getRecordTx(ctx, auth, a.ProjectID, "tasks", a.TaskID); err != nil {
		return out, err
	}
	if a.Source.Requirement != nil {
		if _, err = s.getRecordTx(ctx, auth, a.ProjectID, "requirements", a.Source.Requirement.ID); err != nil {
			return out, err
		}
	}
	out = a
	out.AllowedActions, out.BasisCurrent = []string{}, true
	if err = s.checkTaskConversationReplySourceTx(ctx, auth, a); err != nil {
		var api *APIError
		if !errors.As(err, &api) {
			return TaskConversationReply{}, err
		}
		out.BasisCurrent = false
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, a.ProjectID, a.RunID).Scan(&out.RunVersion, &out.RunState); err != nil {
		return TaskConversationReply{}, err
	}
	if a.State == "running" || a.State == "reconciling" || a.State == "cancelling" {
		_, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, a.ProjectID, true, false)
		if err == nil && out.RunState != "cancelled" && out.RunState != "succeeded" && out.RunState != "failed" {
			out.AllowedActions = append(out.AllowedActions, "cancel")
		} else if err != nil {
			var api *APIError
			if !errors.As(err, &api) || api.Status != 403 {
				return TaskConversationReply{}, err
			}
		}
	}
	return out, nil
}

func (s *Service) GetTaskConversationReply(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out TaskConversationReply, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(conversationReplyInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	record, err := s.taskConversationReplyTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	return s.taskConversationReplyProjectionTx(ctx, engine, auth, record)
}

func (s *Service) ListTaskConversationReplies(ctx context.Context, engine *dag.Engine, actor Actor, project, task string, limit int, cursor string) (out TaskConversationReplyPage, err error) {
	out.Items, out.Limit = []TaskConversationReply{}, limit
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit, out.Limit = 20, 20
	}
	if !handoffID(project, false) || !handoffID(task, false) || limit < 1 || limit > 50 || !handoffID(cursor, true) {
		return out, invalid(conversationReplyInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = s.getRecordTx(ctx, auth, project, "tasks", task); err != nil {
		return out, err
	}
	tenant := auth.target.tenantID
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_task_conversation_replies WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, task).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT id FROM suma_task_conversation_replies WHERE tenant_id=? AND project_id=? AND task_id=?`
	args := []any{tenant, project, task}
	if cursor != "" {
		var at string
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT created_at FROM suma_task_conversation_replies WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), tenant, project, task, cursor).Scan(&at)
		if errors.Is(err, sql.ErrNoRows) {
			return out, invalid(conversationReplyInvalid)
		}
		if err != nil {
			return out, err
		}
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, at, at, cursor)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(ids) > limit {
		ids = ids[:limit]
		out.NextCursor = ids[len(ids)-1]
	}
	for _, id := range ids {
		record, e := s.taskConversationReplyTx(ctx, auth.tx, tenant, project, id)
		if e != nil {
			return out, e
		}
		item, e := s.taskConversationReplyProjectionTx(ctx, engine, auth, record)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}
