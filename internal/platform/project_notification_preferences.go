package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func validProjectNotificationPreferences(out ProjectNotificationPreferences) bool {
	if !taskSubscriptionID(out.ProjectID) || !taskSubscriptionID(out.UserID) || out.Version < 0 || out.Version > taskSubscriptionMaxVersion {
		return false
	}
	if out.Version == 0 {
		return !out.FollowedDiscussionsMuted && out.UpdatedAt == ""
	}
	_, err := time.Parse(time.RFC3339Nano, out.UpdatedAt)
	return err == nil
}

func (s *Service) projectNotificationPreferencesTx(ctx context.Context, auth *authorizedCommand) (string, ProjectNotificationPreferences, error) {
	out := ProjectNotificationPreferences{ProjectID: auth.target.projectID, UserID: auth.currentPrincipal.ID}
	var id string
	var muted int
	err := auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT id,version,followed_discussions_muted,updated_at FROM suma_project_notification_preferences WHERE tenant_id=? AND project_id=? AND user_id=?`)), auth.target.tenantID, out.ProjectID, out.UserID).Scan(&id, &out.Version, &muted, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", out, nil
	}
	if err != nil {
		return "", ProjectNotificationPreferences{}, err
	}
	out.FollowedDiscussionsMuted = muted == 1
	if !taskSubscriptionID(id) || out.Version == 0 || muted < 0 || muted > 1 || !validProjectNotificationPreferences(out) {
		return "", ProjectNotificationPreferences{}, conflict(projectNotificationPreferencesChanged)
	}
	return id, out, nil
}

func (s *Service) GetProjectNotificationPreferences(ctx context.Context, actor Actor, project string) (ProjectNotificationPreferences, error) {
	if !myWorkHuman(actor) {
		return ProjectNotificationPreferences{}, forbidden()
	}
	if !taskSubscriptionID(project) {
		return ProjectNotificationPreferences{}, invalid(projectNotificationPreferencesInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return ProjectNotificationPreferences{}, err
	}
	defer close()
	_, out, err := s.projectNotificationPreferencesTx(ctx, auth)
	if err == nil {
		err = auth.assertTx(auth.tx, auth.target, purposeProjectRead)
	}
	return out, err
}

func (s *Service) CommandProjectNotificationPreferences(ctx context.Context, actor Actor, project string, req ProjectNotificationPreferencesCommand) (ProjectNotificationPreferencesReceipt, error) {
	var out ProjectNotificationPreferencesReceipt
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !validProjectNotificationPreferencesCommand(project, req) {
		return out, invalid(projectNotificationPreferencesInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"project_notification_preferences": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return ProjectNotificationPreferencesReceipt{}, err
		}
		if out.IdempotencyKey != req.IdempotencyKey || out.Preferences.ProjectID != project || out.Preferences.UserID != auth.currentPrincipal.ID || !validProjectNotificationPreferences(out.Preferences) {
			return ProjectNotificationPreferencesReceipt{}, conflict(projectNotificationPreferencesChanged)
		}
		return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
	}
	id, current, err := s.projectNotificationPreferencesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	if current.Version != req.ExpectedVersion {
		return out, conflict(projectNotificationPreferencesChanged)
	}
	if current.FollowedDiscussionsMuted != req.FollowedDiscussionsMuted {
		if current.Version >= taskSubscriptionMaxVersion {
			return out, conflict(projectNotificationPreferencesChanged)
		}
		before := current.Version
		current.Version++
		current.FollowedDiscussionsMuted = req.FollowedDiscussionsMuted
		current.UpdatedAt = auth.checkedAt.UTC().Format(time.RFC3339Nano)
		if id == "" {
			_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_notification_preferences(id,tenant_id,project_id,user_id,version,followed_discussions_muted,updated_at)VALUES(?,?,?,?,?,?,?)`), newID(), auth.target.tenantID, project, current.UserID, current.Version, taskSubscriptionFlag(current.FollowedDiscussionsMuted), current.UpdatedAt)
		} else {
			var result sql.Result
			result, err = auth.tx.ExecContext(ctx, s.q(`UPDATE suma_project_notification_preferences SET version=?,followed_discussions_muted=?,updated_at=? WHERE tenant_id=? AND project_id=? AND user_id=? AND id=? AND version=?`), current.Version, taskSubscriptionFlag(current.FollowedDiscussionsMuted), current.UpdatedAt, auth.target.tenantID, project, current.UserID, id, before)
			if err == nil {
				var count int64
				count, err = result.RowsAffected()
				if err == nil && count != 1 {
					err = conflict(projectNotificationPreferencesChanged)
				}
			}
		}
		if err != nil {
			return out, err
		}
	}
	out = ProjectNotificationPreferencesReceipt{IdempotencyKey: req.IdempotencyKey, Preferences: current}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return ProjectNotificationPreferencesReceipt{}, err
	}
	if err = auth.assertTx(auth.tx, auth.target, purposeProjectRead); err != nil {
		return ProjectNotificationPreferencesReceipt{}, err
	}
	if err = auth.tx.Commit(); err != nil {
		return ProjectNotificationPreferencesReceipt{}, err
	}
	return out, nil
}
