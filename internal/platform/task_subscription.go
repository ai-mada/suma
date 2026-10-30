package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func taskSubscriptionFlag(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Service) taskSubscriptionTx(ctx context.Context, auth *authorizedCommand, task string) (string, TaskSubscription, error) {
	out := TaskSubscription{ProjectID: auth.target.projectID, TaskID: task, UserID: auth.currentPrincipal.ID}
	var id string
	var following int
	err := auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT id,version,following,updated_at FROM suma_task_subscriptions WHERE tenant_id=? AND project_id=? AND task_id=? AND user_id=?`)), auth.target.tenantID, out.ProjectID, task, out.UserID).Scan(&id, &out.Version, &following, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", out, nil
	}
	if err != nil {
		return "", TaskSubscription{}, err
	}
	if !taskSubscriptionID(id) || out.Version < 1 || out.Version > taskSubscriptionMaxVersion || following < 0 || following > 1 {
		return "", TaskSubscription{}, conflict(taskSubscriptionChanged)
	}
	if _, err = time.Parse(time.RFC3339Nano, out.UpdatedAt); err != nil {
		return "", TaskSubscription{}, conflict(taskSubscriptionChanged)
	}
	out.Following = following == 1
	return id, out, nil
}
func (s *Service) GetTaskSubscription(ctx context.Context, actor Actor, project, task string) (TaskSubscription, error) {
	if !myWorkHuman(actor) {
		return TaskSubscription{}, forbidden()
	}
	if !taskSubscriptionID(project) || !taskSubscriptionID(task) {
		return TaskSubscription{}, invalid(taskSubscriptionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return TaskSubscription{}, err
	}
	defer close()
	if _, err = s.getRecordTx(ctx, auth, project, "tasks", task); err != nil {
		return TaskSubscription{}, err
	}
	_, out, err := s.taskSubscriptionTx(ctx, auth, task)
	if err == nil {
		err = auth.assertTx(auth.tx, auth.target, purposeProjectRead)
	}
	return out, err
}
func (s *Service) CommandTaskSubscription(ctx context.Context, actor Actor, project, task string, req TaskSubscriptionCommand) (TaskSubscriptionReceipt, error) {
	var out TaskSubscriptionReceipt
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !validTaskSubscriptionCommand(project, task, req) {
		return out, invalid(taskSubscriptionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = s.getRecordTx(ctx, auth, project, "tasks", task); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_subscription": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		if err == nil {
			err = auth.assertTx(auth.tx, auth.target, purposeProjectRead)
		}
		return out, err
	}
	id, current, err := s.taskSubscriptionTx(ctx, auth, task)
	if err != nil {
		return out, err
	}
	if current.Version != req.ExpectedVersion {
		return out, conflict(taskSubscriptionChanged)
	}
	next := req.Action == "follow"
	if current.Following != next {
		if current.Version >= taskSubscriptionMaxVersion {
			return out, conflict(taskSubscriptionChanged)
		}
		before := current.Version
		current.Version++
		current.Following = next
		current.UpdatedAt = auth.checkedAt.UTC().Format(time.RFC3339Nano)
		if id == "" {
			id = newID()
			_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_subscriptions(id,tenant_id,project_id,task_id,user_id,version,following,updated_at)VALUES(?,?,?,?,?,?,?,?)`), id, auth.target.tenantID, project, task, current.UserID, current.Version, taskSubscriptionFlag(next), current.UpdatedAt)
		} else {
			var result sql.Result
			result, err = auth.tx.ExecContext(ctx, s.q(`UPDATE suma_task_subscriptions SET version=?,following=?,updated_at=? WHERE tenant_id=? AND project_id=? AND task_id=? AND user_id=? AND id=? AND version=?`), current.Version, taskSubscriptionFlag(next), current.UpdatedAt, auth.target.tenantID, project, task, current.UserID, id, before)
			if err == nil {
				var count int64
				count, err = result.RowsAffected()
				if err == nil && count != 1 {
					err = conflict(taskSubscriptionChanged)
				}
			}
		}
		if err != nil {
			return out, err
		}
	}
	out = TaskSubscriptionReceipt{IdempotencyKey: req.IdempotencyKey, Subscription: current}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return TaskSubscriptionReceipt{}, err
	}
	if err = auth.assertTx(auth.tx, auth.target, purposeProjectRead); err != nil {
		return TaskSubscriptionReceipt{}, err
	}
	if err = auth.tx.Commit(); err != nil {
		return TaskSubscriptionReceipt{}, err
	}
	return out, nil
}
