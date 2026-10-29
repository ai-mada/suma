package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) handoffAuthority(ctx context.Context, actor Actor, project, resource string, write bool) (context.Context, *authorizedCommand, func(), error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return ctx, nil, func() {}, err
	}
	cleanup := func() { tx.Rollback(); cancel() }
	purpose := purposeProjectRead
	if write {
		purpose = purposeProjectWrite
	}
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: resource}, purpose)
	if err == nil {
		err = dag.GuardProjectTx(ctx, tx, s.Dialect, auth.target.tenantID, project)
	}
	if err != nil {
		cleanup()
		return ctx, nil, func() {}, err
	}
	return ctx, auth, cleanup, nil
}

func (s *Service) CreateTaskHandoff(ctx context.Context, actor Actor, project string, req TaskHandoffCreateRequest) (out TaskHandoff, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(req.TaskID, false) || !handoffID(req.ToResourceID, false) || !handoffID(req.ToTeamID, true) || req.ExpectedTaskVersion < 1 || !handoffText(req.Note) || len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 128 || !utf8.ValidString(req.IdempotencyKey) {
		return out, invalid("Invalid task handoff request")
	}
	ctx, auth, cleanup, err := s.handoffAuthority(ctx, actor, project, req.TaskID, true)
	if err != nil {
		return out, err
	}
	defer cleanup()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_handoff_create": req.TaskID, "body": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", req.TaskID)
	if err != nil {
		return out, err
	}
	if task.Version != req.ExpectedTaskVersion {
		return out, conflict(handoffBasisChanged)
	}
	basis, blockers, err := s.handoffBasisTx(ctx, auth, task)
	if err != nil {
		return out, err
	}
	if len(blockers) > 0 {
		return out, conflict(blockers[0])
	}
	pending, err := s.handoffPendingTx(ctx, auth, project, task.ID)
	if err != nil {
		return out, err
	}
	if pending {
		return out, conflict(handoffPending)
	}
	recipient, err := s.handoffRecipientTx(ctx, auth, project, req.ToResourceID)
	if err != nil {
		return out, err
	}
	if task.OwnerID == recipient.ResourceID {
		return out, invalid(handoffSameOwner)
	}
	team, ok := handoffSelectedTeam(recipient, req.ToTeamID)
	if !ok {
		return out, invalid(handoffTeamInvalid)
	}
	out = TaskHandoff{ID: newID(), ProjectID: project, TaskID: task.ID, Version: 1, State: "pending", CreatedBy: auth.currentPrincipal.ID, CreatedAt: now(), ToResourceID: recipient.ResourceID, ToResourceVersion: recipient.ResourceVersion, ToUserID: recipient.UserID, ToName: recipient.Name, ToTeamID: team.TeamID, ToTeamName: team.Name, ToTeamVersion: team.TeamVersion, ToMembershipVersion: team.MembershipVersion, Note: req.Note, Basis: basis, Blockers: []string{}, AllowedActions: []string{}}
	out.UpdatedAt = out.CreatedAt
	out, err = s.handoffProjectionTx(ctx, auth, out, basis, blockers)
	if err != nil {
		return out, err
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_handoffs(id,tenant_id,project_id,task_id,version,state,created_at,payload) VALUES(?,?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, project, task.ID, out.Version, out.State, out.CreatedAt, stringify(out)); err != nil {
		return out, err
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_handoff_pending(tenant_id,project_id,task_id,handoff_id) VALUES(?,?,?,?)`), auth.target.tenantID, project, task.ID, out.ID); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "task.handoff_created", out.ID, task.Title); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

// Decision authority is independent of basis freshness: an obsolete proposal can
// still be returned or withdrawn, without making obsolete work current again.
func (s *Service) handoffDecisionActorTx(ctx context.Context, auth *authorizedCommand, h TaskHandoff, action string) error {
	actor := auth.currentPrincipal.ID
	if action == "withdraw" {
		if actor != h.CreatedBy {
			return forbidden()
		}
		return nil
	}
	if actor != h.ToUserID {
		return forbidden()
	}
	if h.ToTeamID != "" {
		var count int
		err := auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_teams t JOIN suma_team_members m ON m.tenant_id=t.tenant_id AND m.team_id=t.id WHERE t.tenant_id=? AND t.id=? AND t.status='active' AND m.resource_kind='human' AND m.resource_id=? AND m.project_id='' AND m.status='active' AND (m.role='member' OR m.role='lead')`), auth.target.tenantID, h.ToTeamID, actor).Scan(&count)
		if err != nil {
			return err
		}
		if count != 1 {
			return forbidden()
		}
	}
	return nil
}

func (s *Service) CommandTaskHandoff(ctx context.Context, actor Actor, project, id string, req TaskHandoffCommandRequest) (out TaskHandoff, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || !handoffText(req.Reason) || len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 128 || !utf8.ValidString(req.IdempotencyKey) || req.Action != "accept" && req.Action != "return" && req.Action != "withdraw" {
		return out, invalid("Invalid task handoff command")
	}
	ctx, auth, cleanup, err := s.handoffAuthority(ctx, actor, project, id, true)
	if err != nil {
		return out, err
	}
	defer cleanup()
	out, err = s.taskHandoffTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	if err = s.handoffDecisionActorTx(ctx, auth, out, req.Action); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_handoff_command": id, "body": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if out.Version != req.ExpectedVersion || out.State != "pending" {
		return out, conflict(handoffVersion)
	}
	if req.Action == "accept" {
		if !req.AcknowledgeInvalidation {
			return out, invalid(handoffAcknowledge)
		}
		task, e := s.getRecordTx(ctx, auth, project, "tasks", out.TaskID)
		if e != nil {
			return out, e
		}
		basis, blockers, e := s.handoffBasisTx(ctx, auth, task)
		if e != nil {
			return out, e
		}
		if len(blockers) > 0 {
			return out, conflict(blockers[0])
		}
		if basis != out.Basis {
			return out, conflict(handoffBasisChanged)
		}
		recipient, e := s.handoffRecipientTx(ctx, auth, project, out.ToResourceID)
		if e != nil {
			return out, e
		}
		team, ok := handoffSelectedTeam(recipient, out.ToTeamID)
		if !ok {
			return out, conflict(handoffTeamInvalid)
		}
		if recipient.UserID != out.ToUserID || recipient.ResourceVersion != out.ToResourceVersion || team.TeamVersion != out.ToTeamVersion || team.MembershipVersion != out.ToMembershipVersion {
			return out, conflict(handoffBasisChanged)
		}
		delete(task.Data, "verified_run_id")
		result, e := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET owner_id=?,version=version+1,status='in_progress',updated_at=?,data=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND owner_id=?`), out.ToResourceID, now(), stringify(task.Data), auth.target.tenantID, project, task.ID, out.Basis.TaskVersion, out.Basis.FromOwnerID)
		if e != nil {
			return out, e
		}
		count, e := result.RowsAffected()
		if e != nil {
			return out, e
		}
		if count != 1 {
			return out, conflict(handoffBasisChanged)
		}
		// Acceptance is the actual assignment transition, not its earlier
		// proposal. Hydrate the saved version before projecting the same event.
		assigned, e := s.getRecordTx(ctx, auth, project, "tasks", task.ID)
		if e != nil {
			return out, e
		}
		if e = s.projectTaskAssignmentNotificationTx(ctx, auth, task.OwnerID, assigned); e != nil {
			return out, e
		}
		if e = s.InvalidateDependentsTx(ctx, auth.tx, auth, project, []string{task.ID}, false); e != nil {
			return out, e
		}
		out.ResultTaskVersion = task.Version + 1
		out.State = "accepted"
	} else if req.Action == "return" {
		out.State = "returned"
	} else {
		out.State = "withdrawn"
	}
	out.Version++
	out.DecisionBy, out.DecisionAt, out.DecisionReason = auth.currentPrincipal.ID, now(), req.Reason
	out.UpdatedAt = out.DecisionAt
	out.AllowedActions, out.Blockers = []string{}, []string{}
	out.BasisCurrent = false
	if req.Action != "accept" {
		task, readErr := s.getRecordTx(ctx, auth, project, "tasks", out.TaskID)
		var domain *APIError
		if readErr != nil && !errors.As(readErr, &domain) {
			return out, readErr
		}
		if readErr == nil {
			basis, blockers, readErr := s.handoffBasisTx(ctx, auth, task)
			if readErr != nil && !errors.As(readErr, &domain) {
				return out, readErr
			}
			if readErr == nil {
				out, readErr = s.handoffProjectionTx(ctx, auth, out, basis, blockers)
				if readErr != nil {
					return out, readErr
				}
			}
		}
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_task_handoffs SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND state='pending'`), out.Version, out.State, stringify(out), auth.target.tenantID, project, out.ID, req.ExpectedVersion)
	if err != nil {
		return out, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return out, err
	}
	if count != 1 {
		return out, conflict(handoffVersion)
	}
	result, err = auth.tx.ExecContext(ctx, s.q(`DELETE FROM suma_task_handoff_pending WHERE tenant_id=? AND project_id=? AND task_id=? AND handoff_id=?`), auth.target.tenantID, project, out.TaskID, out.ID)
	if err != nil {
		return out, err
	}
	count, err = result.RowsAffected()
	if err != nil {
		return out, err
	}
	if count != 1 {
		return out, conflict(handoffVersion)
	}
	if err = s.commandActivity(ctx, auth, "task.handoff_"+out.State, out.ID, out.Note); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) handoffProjectionTx(ctx context.Context, auth *authorizedCommand, out TaskHandoff, current TaskHandoffBasis, blockers []string) (TaskHandoff, error) {
	out.AllowedActions, out.Blockers = []string{}, []string{}
	out.BasisCurrent = current == out.Basis && len(blockers) == 0
	recipient, err := s.handoffRecipientTx(ctx, auth, out.ProjectID, out.ToResourceID)
	var domain *APIError
	if err != nil && !errors.As(err, &domain) {
		return out, err
	}
	identityCurrent := err == nil && recipient.UserID == out.ToUserID && recipient.ResourceVersion == out.ToResourceVersion
	team, teamOK := handoffSelectedTeam(recipient, out.ToTeamID)
	identityCurrent = identityCurrent && teamOK && team.TeamVersion == out.ToTeamVersion && team.MembershipVersion == out.ToMembershipVersion
	out.BasisCurrent = out.BasisCurrent && identityCurrent
	if out.State != "pending" {
		return out, nil
	}
	out.Blockers = append(out.Blockers, blockers...)
	if current != out.Basis {
		out.Blockers = append(out.Blockers, handoffBasisChanged)
	}
	if !identityCurrent {
		out.BasisCurrent = false
		out.Blockers = append(out.Blockers, handoffRecipientInvalid)
	}
	write, err := s.handoffWriteUserTx(ctx, auth, out.ProjectID, auth.currentPrincipal.ID)
	if err != nil {
		return out, err
	}
	if !write {
		return out, nil
	}
	if auth.currentPrincipal.ID == out.CreatedBy {
		out.AllowedActions = append(out.AllowedActions, "withdraw")
	}
	if auth.currentPrincipal.ID == out.ToUserID {
		if err = s.handoffDecisionActorTx(ctx, auth, out, "return"); err == nil {
			out.AllowedActions = append(out.AllowedActions, "return")
			if out.BasisCurrent {
				out.AllowedActions = append(out.AllowedActions, "accept")
			}
		} else if !errors.As(err, &domain) {
			return out, err
		}
	}
	return out, nil
}

func (s *Service) TaskHandoffs(ctx context.Context, actor Actor, project, taskID string, limit int, before string) (out TaskHandoffPage, err error) {
	out = TaskHandoffPage{Items: []TaskHandoff{}, Recipients: []TaskHandoffRecipient{}, CreateBlockers: []string{}}
	if !handoffID(project, false) || !handoffID(taskID, false) || !handoffID(before, true) || limit < 0 || limit > 100 {
		return out, invalid("Invalid task handoff page")
	}
	if limit == 0 {
		limit = 20
	}
	ctx, auth, cleanup, err := s.handoffAuthority(ctx, actor, project, taskID, false)
	if err != nil {
		return out, err
	}
	defer cleanup()
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return out, err
	}
	out.TaskVersion = task.Version
	basis, blockers, err := s.handoffBasisTx(ctx, auth, task)
	if err != nil {
		return out, err
	}
	out.CreateBlockers = append(out.CreateBlockers, blockers...)
	pending, err := s.handoffPendingTx(ctx, auth, project, taskID)
	if err != nil {
		return out, err
	}
	if pending {
		out.CreateBlockers = append(out.CreateBlockers, handoffPending)
	}
	write, err := s.handoffWriteUserTx(ctx, auth, project, auth.currentPrincipal.ID)
	if err != nil {
		return out, err
	}
	out.CanCreate = write && len(out.CreateBlockers) == 0
	if !write {
		out.CreateBlockers = append(out.CreateBlockers, handoffPermission)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id FROM suma_members WHERE tenant_id=? AND project_id=? AND kind='human' AND status='active' AND id<>? ORDER BY title,id LIMIT 1000`), auth.target.tenantID, project, task.OwnerID)
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
	for _, id := range ids {
		recipient, e := s.handoffRecipientTx(ctx, auth, project, id)
		if e != nil {
			var domain *APIError
			if errors.As(e, &domain) {
				continue
			}
			return out, e
		}
		out.Recipients = append(out.Recipients, recipient)
	}
	if len(out.Recipients) == 0 {
		out.CanCreate = false
		out.CreateBlockers = append(out.CreateBlockers, handoffRecipientInvalid)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_task_handoffs WHERE tenant_id=? AND project_id=? AND task_id=?`), auth.target.tenantID, project, taskID).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT id FROM suma_task_handoffs WHERE tenant_id=? AND project_id=? AND task_id=?`
	args := []any{auth.target.tenantID, project, taskID}
	if before != "" {
		var at string
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT created_at FROM suma_task_handoffs WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), auth.target.tenantID, project, taskID, before).Scan(&at)
		if errors.Is(err, sql.ErrNoRows) {
			return out, invalid("Invalid task handoff cursor")
		}
		if err != nil {
			return out, err
		}
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, at, at, before)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err = auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return out, err
	}
	ids = nil
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
		item, e := s.taskHandoffTx(ctx, auth, project, id)
		if e != nil {
			return out, e
		}
		item, e = s.handoffProjectionTx(ctx, auth, item, basis, blockers)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}
