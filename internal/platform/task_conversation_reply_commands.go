package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
)

// Generic commands cannot bypass the reply version/state transaction or turn
// one paid request into another attempt. Use the owning reply command endpoint.
func (s *Service) authorizeTaskConversationReplyRunCommandTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, run, action string) (bool, error) {
	_, found, err := s.taskConversationReplyForRunTx(ctx, tx, auth.target.tenantID, project, run)
	if err != nil || !found {
		return false, err
	}
	return true, invalid(conversationReplyControls)
}

func (s *Service) CommandTaskConversationReply(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req TaskConversationReplyCommandRequest) (out TaskConversationReply, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || req.ExpectedVersion > 2147483646 || req.ExpectedRunVersion < 1 || req.ExpectedRunVersion > 2147483646 || req.Action != "cancel" || !planningKey(req.IdempotencyKey) {
		return out, invalid(conversationReplyInvalid)
	}
	first, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	body := map[string]any{"task_conversation_reply_command": id, "request": req}
	raw, replay, err := s.commandReceipt(first, auth, req.IdempotencyKey, body)
	if err != nil {
		close()
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(raw), &out); err == nil {
			record, e := s.taskConversationReplyTx(first, auth.tx, auth.target.tenantID, project, id)
			if e == nil {
				out, err = s.taskConversationReplyProjectionTx(first, engine, auth, record)
			} else {
				err = e
			}
		}
		close()
		return out, err
	}
	record, err := s.taskConversationReplyTx(first, auth.tx, auth.target.tenantID, project, id)
	if err == nil && (record.Reply.Version != req.ExpectedVersion || record.Reply.State != "running" && record.Reply.State != "reconciling" && record.Reply.State != "cancelling") {
		err = conflict(conversationReplyVersion)
	}
	close()
	if err != nil {
		return out, err
	}
	authority := s.engineAuthority(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectWrite)
	defer authority.close()
	_, err = engine.CommandWithAuthority(authority.ctx, actor.credential.tenantID, project, record.Reply.RunID, "cancel", digest(id+":"+req.IdempotencyKey), req.ExpectedRunVersion, map[string]any{}, func(tx *sql.Tx) error {
		if e := authority.before(tx); e != nil {
			return e
		}
		_, replayed, e := s.commandReceipt(authority.ctx, authority.proof, req.IdempotencyKey, body)
		if e != nil || replayed {
			return e
		}
		current, e := s.taskConversationReplyTx(authority.ctx, tx, authority.proof.target.tenantID, project, id)
		if e != nil {
			return e
		}
		if current.Reply.Version != req.ExpectedVersion || current.Reply.RunID != record.Reply.RunID {
			return conflict(conversationReplyVersion)
		}
		return nil
	}, func(tx *sql.Tx, run dag.Run) error {
		a := authority.proof
		hash, _, e := s.commandReceipt(authority.ctx, a, req.IdempotencyKey, body)
		if e != nil {
			return e
		}
		current, e := s.taskConversationReplyTx(authority.ctx, tx, a.target.tenantID, project, id)
		if e != nil {
			return e
		}
		previous := current.Reply.Version
		current.Reply.Version++
		current.Reply.State, current.Reply.UpdatedAt = "cancelling", now()
		if run.State == "cancelled" {
			// The Core has already proved every node stopped or never started.
			current.Reply.State = "cancelled"
			if _, e = tx.ExecContext(authority.ctx, s.q(`DELETE FROM suma_task_conversation_reply_active WHERE tenant_id=? AND project_id=? AND task_id=? AND reply_id=?`), a.target.tenantID, project, current.Reply.TaskID, id); e != nil {
				return e
			}
		}
		if e = s.saveTaskConversationReplyTx(authority.ctx, tx, a.target.tenantID, current, previous); e != nil {
			return e
		}
		return s.saveCommandReceipt(authority.ctx, a, req.IdempotencyKey, hash, current.Reply)
	})
	if err != nil {
		return out, err
	}
	return s.GetTaskConversationReply(ctx, engine, actor, project, id)
}
