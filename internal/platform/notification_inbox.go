package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

type notificationInboxCursor struct {
	Version int    `json:"version"`
	Tenant  string `json:"tenant"`
	Project string `json:"project"`
	User    string `json:"user"`
	Filter  string `json:"filter"`
	Limit   int    `json:"limit"`
	Before  string `json:"before"`
}

func (s *Service) NotificationInbox(ctx context.Context, actor Actor, project, filter string, limit int, cursor string) (out NotificationInboxPage, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if filter == "" {
		filter = "inbox"
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || (filter != "inbox" && filter != "unread" && filter != "archived") || limit < 1 || limit > 50 || len(cursor) > 4096 {
		return out, invalid(notificationInboxInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.notificationInboxTx(bounded, auth, project, filter, limit, cursor)
		return err
	})
	if err != nil {
		return NotificationInboxPage{}, err
	}
	return out, nil
}

func (s *Service) notificationInboxTx(ctx context.Context, auth *authorizedCommand, project, filter string, limit int, cursor string) (out NotificationInboxPage, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return out, err
	}
	if !myWorkHuman(auth.actor) {
		return out, forbidden()
	}
	out = NotificationInboxPage{ProjectID: project, UserID: auth.currentPrincipal.ID, Limit: limit, Items: []NotificationInboxItem{}, AsOf: auth.checkedAt.UTC().Format(time.RFC3339Nano)}
	scope := notificationInboxCursor{Version: 1, Tenant: auth.target.tenantID, Project: project, User: auth.currentPrincipal.ID, Filter: filter, Limit: limit}
	var after notificationInboxCursor
	var before int64
	if cursor != "" {
		raw, decodeErr := base64.RawURLEncoding.Strict().DecodeString(cursor)
		if decodeErr != nil || json.Unmarshal(raw, &after) != nil || base64.RawURLEncoding.EncodeToString([]byte(stringify(after))) != cursor {
			return out, invalid(notificationInboxInvalid)
		}
		var valid bool
		before, valid = notificationInboxSequence(after.Before)
		identity := after
		identity.Before = ""
		if !valid || identity != scope {
			return out, invalid(notificationInboxInvalid)
		}
	}
	where := ` FROM suma_notification_inbox n WHERE n.tenant_id=? AND n.project_id=? AND n.recipient_user_id=?`
	args := []any{auth.target.tenantID, project, auth.currentPrincipal.ID}
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(SUM(CASE WHEN archived_at='' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN archived_at='' AND read_at='' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN archived_at<>'' THEN 1 ELSE 0 END),0)`+where), args...).Scan(&out.Counts.Inbox, &out.Counts.Unread, &out.Counts.Archived)
	if err != nil {
		return out, err
	}
	switch filter {
	case "inbox":
		where += ` AND n.archived_at=''`
		out.Total = out.Counts.Inbox
	case "unread":
		where += ` AND n.archived_at='' AND n.read_at=''`
		out.Total = out.Counts.Unread
	case "archived":
		where += ` AND n.archived_at<>''`
		out.Total = out.Counts.Archived
	}
	if cursor != "" {
		where += ` AND n.sequence<?`
		args = append(args, before)
	}
	// Hydrate only current bounded source fields. No title/body snapshot exists.
	joins := ` FROM suma_notification_inbox n LEFT JOIN suma_tasks t ON t.tenant_id=n.tenant_id AND t.project_id=n.project_id AND t.id=n.task_id`
	query := `SELECT n.id,n.version,n.sequence,n.kind,n.task_id,n.source_task_version,n.source_resource_id,n.created_at,n.read_at,n.archived_at,COALESCE(t.id,''),COALESCE(t.version,0),COALESCE(t.title,''),COALESCE(t.status,''),COALESCE(t.owner_id,'')` + joins + where[len(` FROM suma_notification_inbox n`):] + ` ORDER BY n.sequence DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item NotificationInboxItem
		var task NotificationInboxTask
		var owner string
		var sequence int64
		if err = rows.Scan(&item.ID, &item.Version, &sequence, &item.Kind, &item.TaskID, &item.SourceTaskVersion, &item.SourceResourceID, &item.CreatedAt, &item.ReadAt, &item.ArchivedAt, &task.ID, &task.Version, &task.Title, &task.Status, &owner); err != nil {
			return out, err
		}
		if sequence < 1 || item.Version < 1 || item.SourceTaskVersion < 1 || item.Kind != "task_assigned" {
			return out, conflict("The notification source is invalid")
		}
		item.Sequence = strconv.FormatInt(sequence, 10)
		item.Read = item.ReadAt != ""
		item.Archived = item.ArchivedAt != ""
		item.SourceAvailable = task.ID != ""
		if item.SourceAvailable {
			item.CurrentTask = &task
			item.AssignmentCurrent = owner == item.SourceResourceID
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		scope.Before = out.Items[limit-1].Sequence
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	return out, nil
}

func validNotificationInboxCommand(project, id string, req NotificationInboxCommand) bool {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || req.ExpectedVersion > 2147483646 || len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 128 || strings.TrimSpace(req.IdempotencyKey) == "" || !taskWorkspaceText(req.IdempotencyKey) {
		return false
	}
	return req.Action == "read" || req.Action == "unread" || req.Action == "archive" || req.Action == "restore"
}

// Private lifecycle is authorized by current project READ membership. It never
// calls project-write helpers or publishes team activity, including for viewers.
func (s *Service) CommandNotificationInbox(ctx context.Context, actor Actor, project, id string, req NotificationInboxCommand) (out NotificationInboxCommandReceipt, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !validNotificationInboxCommand(project, id, req) {
		return out, invalid(notificationInboxInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		stored, readErr := s.ownNotificationInboxTx(bounded, auth, project, id)
		if readErr != nil {
			return readErr
		}
		hash, replay, readErr := s.commandReceipt(bounded, auth, req.IdempotencyKey, map[string]any{"notification_inbox_command": id, "request": req})
		if readErr != nil {
			return readErr
		}
		if replay {
			return json.Unmarshal([]byte(hash), &out)
		}
		if stored.version != req.ExpectedVersion {
			return conflict(notificationInboxChanged)
		}
		readAt, archivedAt := stored.readAt, stored.archivedAt
		at := auth.checkedAt.UTC().Format(time.RFC3339Nano)
		switch req.Action {
		case "read":
			if readAt == "" {
				readAt = at
			}
		case "unread":
			readAt = ""
		case "archive":
			if archivedAt == "" {
				archivedAt = at
			}
		case "restore":
			archivedAt = ""
		}
		version, updatedAt := stored.version, stored.updatedAt
		if readAt != stored.readAt || archivedAt != stored.archivedAt {
			version++
			updatedAt = at
			result, writeErr := auth.tx.ExecContext(bounded, s.q(`UPDATE suma_notification_inbox SET version=?,read_at=?,archived_at=?,updated_at=? WHERE tenant_id=? AND project_id=? AND recipient_user_id=? AND id=? AND version=?`), version, readAt, archivedAt, updatedAt, auth.target.tenantID, project, auth.currentPrincipal.ID, id, stored.version)
			if writeErr != nil {
				return writeErr
			}
			count, writeErr := result.RowsAffected()
			if writeErr != nil {
				return writeErr
			}
			if count != 1 {
				return conflict(notificationInboxChanged)
			}
		}
		out = NotificationInboxCommandReceipt{IdempotencyKey: req.IdempotencyKey, NotificationID: id, Version: version, Action: req.Action, Read: readAt != "", Archived: archivedAt != "", ReadAt: readAt, ArchivedAt: archivedAt, UpdatedAt: updatedAt}
		return s.saveCommandReceipt(bounded, auth, req.IdempotencyKey, hash, out)
	})
	if err != nil {
		return NotificationInboxCommandReceipt{}, err
	}
	return out, nil
}
