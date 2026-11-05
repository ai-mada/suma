package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

const notificationInboxBatchLimit = 20

func (batch NotificationInboxBatchCommand) itemCommand(item NotificationInboxBatchTarget) NotificationInboxCommand {
	return NotificationInboxCommand{
		ProjectID: item.ProjectID, ExpectedVersion: item.ExpectedVersion,
		Action: batch.Action, IdempotencyKey: batch.IdempotencyKey,
	}
}

func validNotificationInboxBatchCommand(req NotificationInboxBatchCommand) bool {
	if len(req.Items) < 1 || len(req.Items) > notificationInboxBatchLimit {
		return false
	}
	seen := make(map[string]bool, len(req.Items))
	for _, item := range req.Items {
		if item.ProjectID == organizationScope || seen[item.NotificationID] ||
			!validNotificationInboxCommand(item.ProjectID, item.NotificationID, req.itemCommand(item)) {
			return false
		}
		seen[item.NotificationID] = true
	}
	return true
}

func (s *Service) CommandNotificationInboxBatch(ctx context.Context, actor Actor, req NotificationInboxBatchCommand) (out NotificationInboxBatchReceipt, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if !validNotificationInboxBatchCommand(req) {
		return out, invalid(notificationInboxInvalid)
	}
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}
	err = s.withCurrentRead(ctx, actor, target, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		// Current project read and row ownership apply equally to a new
		// command and to returning an earlier private receipt.
		if readErr := s.authorizeNotificationInboxBatchProjectsTx(bounded, auth, req.Items); readErr != nil {
			return readErr
		}
		stored, readErr := s.notificationInboxBatchRowsTx(bounded, auth, req.Items)
		if readErr != nil {
			return readErr
		}
		hash, replay, readErr := s.commandReceipt(bounded, auth, req.IdempotencyKey, map[string]any{"notification_inbox_batch_command": req})
		if readErr != nil {
			return readErr
		}
		if replay {
			return json.Unmarshal([]byte(hash), &out)
		}
		for _, item := range req.Items {
			if stored[item.NotificationID].version != item.ExpectedVersion {
				return conflict(notificationInboxChanged)
			}
		}
		out = NotificationInboxBatchReceipt{
			IdempotencyKey: req.IdempotencyKey, Action: req.Action,
			Results: make([]NotificationInboxBatchResult, 0, len(req.Items)),
		}
		for _, item := range req.Items {
			receipt, applyErr := s.applyNotificationInboxCommandTx(bounded, auth, item.ProjectID, item.NotificationID, req.itemCommand(item), stored[item.NotificationID])
			if applyErr != nil {
				return applyErr
			}
			out.Results = append(out.Results, NotificationInboxBatchResult{ProjectID: item.ProjectID, Receipt: receipt})
		}
		return s.saveCommandReceipt(bounded, auth, req.IdempotencyKey, hash, out)
	})
	if err != nil {
		return NotificationInboxBatchReceipt{}, err
	}
	return out, nil
}

func (s *Service) authorizeNotificationInboxBatchProjectsTx(ctx context.Context, auth *authorizedCommand, items []NotificationInboxBatchTarget) error {
	if err := assertOrganizationRead(auth); err != nil {
		return err
	}
	projects := make(map[string]bool, len(items))
	args := []any{auth.currentPrincipal.ID, auth.target.tenantID}
	for _, item := range items {
		if !projects[item.ProjectID] {
			projects[item.ProjectID] = true
			args = append(args, item.ProjectID)
		}
	}
	// The organization authority lock also serializes project membership
	// changes. Check actual project and user IDs and roles in Go so SQL
	// collations cannot broaden authorization.
	marks := strings.TrimSuffix(strings.Repeat("?,", len(projects)), ",")
	query := `SELECT p.id,p.tenant_id,m.user_id,m.role FROM suma_projects p
LEFT JOIN suma_project_members m ON m.project_id=p.id AND m.user_id=?
WHERE p.tenant_id=? AND p.id IN (` + marks + `)`
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var project, tenant string
		var user, role sql.NullString
		if err = rows.Scan(&project, &tenant, &user, &role); err != nil {
			return err
		}
		if !projects[project] || tenant != auth.target.tenantID ||
			!orgAdmin(auth.currentPrincipal) && (!user.Valid || user.String != auth.currentPrincipal.ID || !role.Valid || !validProjectRole(role.String)) {
			return forbidden()
		}
		delete(projects, project)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if len(projects) != 0 {
		return forbidden()
	}
	return nil
}

func (s *Service) notificationInboxBatchRowsTx(ctx context.Context, auth *authorizedCommand, items []NotificationInboxBatchTarget) (map[string]notificationInboxStored, error) {
	projects := make(map[string]string, len(items))
	args := []any{auth.target.tenantID, auth.currentPrincipal.ID}
	for _, item := range items {
		projects[item.NotificationID] = item.ProjectID
		args = append(args, item.NotificationID)
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(items)), ",")
	query := `SELECT id,tenant_id,project_id,recipient_user_id,version,read_at,archived_at,updated_at
FROM suma_notification_inbox WHERE tenant_id=? AND recipient_user_id=? AND id IN (` + marks + `) ORDER BY id`
	rows, err := auth.tx.QueryContext(ctx, s.q(s.currentSQL(query)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	stored := make(map[string]notificationInboxStored, len(items))
	for rows.Next() {
		var row notificationInboxStored
		var tenant, project, user string
		if err = rows.Scan(&row.id, &tenant, &project, &user, &row.version, &row.readAt, &row.archivedAt, &row.updatedAt); err != nil {
			return nil, err
		}
		if projects[row.id] != project || tenant != auth.target.tenantID || user != auth.currentPrincipal.ID {
			return nil, missing()
		}
		stored[row.id] = row
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if len(stored) != len(items) {
		return nil, missing()
	}
	return stored, nil
}
