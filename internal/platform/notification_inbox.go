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
	if !handoffID(project, false) || !validNotificationInboxRead(filter, limit, cursor) {
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
	where := ` WHERE n.tenant_id=? AND n.project_id=? AND n.recipient_user_id=?`
	args := []any{auth.target.tenantID, project, auth.currentPrincipal.ID}
	out.Counts, err = s.notificationInboxCountsTx(ctx, auth, where, args)
	if err != nil {
		return out, err
	}
	condition, total := notificationInboxFilter(filter, out.Counts)
	where += condition
	out.Total = total
	if cursor != "" {
		where += ` AND n.sequence<?`
		args = append(args, before)
	}
	items, err := s.notificationInboxItemsTx(ctx, auth, where, args, "n.sequence DESC", limit)
	if err != nil {
		return out, err
	}
	if len(items) > limit {
		items = items[:limit]
		scope.Before = items[limit-1].Notification.Sequence
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	for _, item := range items {
		out.Items = append(out.Items, item.Notification)
	}
	return out, nil
}

const notificationInboxFrom = ` FROM suma_notification_inbox n JOIN suma_projects p ON p.id=n.project_id AND p.tenant_id=n.tenant_id`

const notificationInboxUnreadWhere = ` AND n.archived_at='' AND n.read_at=''`

func validNotificationInboxRead(filter string, limit int, cursor string) bool {
	return (filter == "inbox" || filter == "unread" || filter == "archived") && limit >= 1 && limit <= 50 && len(cursor) <= 4096
}

func (s *Service) notificationInboxCountsTx(ctx context.Context, auth *authorizedCommand, where string, args []any) (counts NotificationInboxCounts, err error) {
	query := `SELECT COALESCE(SUM(CASE WHEN n.archived_at='' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN n.archived_at='' AND n.read_at='' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN n.archived_at<>'' THEN 1 ELSE 0 END),0)`
	err = auth.tx.QueryRowContext(ctx, s.q(query+notificationInboxFrom+where), args...).Scan(&counts.Inbox, &counts.Unread, &counts.Archived)
	return counts, err
}

func notificationInboxFilter(filter string, counts NotificationInboxCounts) (string, int) {
	switch filter {
	case "unread":
		return notificationInboxUnreadWhere, counts.Unread
	case "archived":
		return ` AND n.archived_at<>''`, counts.Archived
	default:
		return ` AND n.archived_at=''`, counts.Inbox
	}
}

// Callers authorize the complete where scope before loading any private rows.
// The same hydration path is used for both project and organization inboxes.
func (s *Service) notificationInboxItemsTx(ctx context.Context, auth *authorizedCommand, where string, args []any, order string, limit int) ([]NotificationOrganizationInboxItem, error) {
	// Select the bounded authorized page before joining source tables. This
	// keeps hydration proportional to the page even when many projects match.
	page := "SELECT n.*,p.title AS project_title" + notificationInboxFrom + where + " ORDER BY " + order + " LIMIT ?"
	joins := ` LEFT JOIN suma_tasks t ON t.tenant_id=n.tenant_id AND t.project_id=n.project_id AND t.id=n.task_id LEFT JOIN suma_task_conversation_mentions cm ON n.kind='task_mentioned' AND cm.tenant_id=n.tenant_id AND cm.project_id=n.project_id AND cm.task_id=n.task_id AND cm.notification_id=n.id AND cm.resource_id=n.source_resource_id AND cm.recipient_user_id=n.recipient_user_id AND cm.ordinal>=0 AND cm.ordinal<20 LEFT JOIN suma_task_conversation_messages msg ON msg.tenant_id=n.tenant_id AND msg.project_id=n.project_id AND msg.task_id=n.task_id AND msg.id=cm.message_id AND msg.task_version=n.source_task_version AND msg.actor_kind IN ('human','api') LEFT JOIN suma_task_review_requests rq ON n.kind='task_review_requested' AND rq.tenant_id=n.tenant_id AND rq.project_id=n.project_id AND rq.task_id=n.task_id AND rq.notification_id=n.id AND rq.reviewer_resource_id=n.source_resource_id AND rq.reviewer_user_id=n.recipient_user_id AND rq.task_version=n.source_task_version AND rq.state IN ('pending','cancelled','closed') LEFT JOIN suma_task_handoffs h ON n.kind='task_handoff_requested' AND h.tenant_id=n.tenant_id AND h.project_id=n.project_id AND h.task_id=n.task_id AND h.id=n.source_event_key` + taskRunFailureInboxJoins + taskToolInputInboxJoins + automationPauseInboxJoins
	query := `SELECT n.project_id,n.project_title,n.id,n.version,n.sequence,n.kind,n.task_id,n.source_task_version,n.source_resource_id,n.created_at,n.read_at,n.archived_at,COALESCE(t.id,''),COALESCE(t.version,0),COALESCE(t.title,''),COALESCE(t.status,''),COALESCE(t.owner_id,''),COALESCE(msg.id,''),n.source_event_key,n.recipient_user_id,COALESCE(rq.id,''),COALESCE(h.version,0),COALESCE(h.state,''),COALESCE(h.payload,''),COALESCE(fr.id,''),COALESCE(fp.payload,''),COALESCE(fe.sequence,0),COALESCE(fe.at_time,'')` + taskToolInputInboxColumns + automationPauseInboxColumns + " FROM (" + page + ") n" + joins + " ORDER BY " + order
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]NotificationOrganizationInboxItem, 0, limit+1)
	followed := map[string][]taskSubscriptionInboxSource{}
	for rows.Next() {
		var row NotificationOrganizationInboxItem
		item := &row.Notification
		var task NotificationInboxTask
		var owner, messageID, eventKey, recipient, reviewRequestID, handoffState, handoffPayload string
		var failedRun, binding, failedAt string
		var inputSource taskToolInputInboxSource
		var pauseSource automationPauseInboxSource
		occurrence := &pauseSource.occurrence
		var inputBinding, inputRun, inputNode, inputWorker string
		var handoffVersion int
		var sequence, failureSequence int64
		if err = rows.Scan(&row.ProjectID, &row.ProjectTitle, &item.ID, &item.Version, &sequence, &item.Kind, &item.TaskID, &item.SourceTaskVersion, &item.SourceResourceID, &item.CreatedAt, &item.ReadAt, &item.ArchivedAt, &task.ID, &task.Version, &task.Title, &task.Status, &owner, &messageID, &eventKey, &recipient, &reviewRequestID, &handoffVersion, &handoffState, &handoffPayload, &failedRun, &binding, &failureSequence, &failedAt,
			&inputSource.tenant, &inputSource.project, &inputSource.id, &inputSource.run, &inputSource.node,
			&inputSource.attempt, &inputSource.fence, &inputSource.worker, &inputSource.requestSHA256,
			&inputBinding, &inputRun, &inputNode, &inputWorker,
			&pauseSource.tenant, &pauseSource.project, &occurrence.ID, &occurrence.RuleID, &occurrence.Revision,
			&occurrence.State, &occurrence.Reason, &occurrence.RunID, &occurrence.UpdatedAt,
			&pauseSource.ruleTenant, &pauseSource.ruleProject, &pauseSource.ruleID, &pauseSource.revisionRuleID, &pauseSource.spec); err != nil {
			return nil, err
		}
		if sequence < 1 || item.Version < 1 || !validNotificationInboxKind(item.Kind) || !validNotificationInboxTaskBasis(item.Kind, item.TaskID, item.SourceTaskVersion) {
			return nil, conflict("The notification source is invalid")
		}
		item.Sequence = strconv.FormatInt(sequence, 10)
		item.Read = item.ReadAt != ""
		item.Archived = item.ArchivedAt != ""
		item.SourceAvailable = task.ID != ""
		if item.Kind == "task_mentioned" {
			item.SourceAvailable = item.SourceAvailable && handoffID(messageID, false) && eventKey == taskConversationMentionEventKey(messageID, item.SourceResourceID, recipient)
			if item.SourceAvailable {
				item.MessageID = messageID
			}
		}
		if item.Kind == "task_review_requested" {
			item.SourceAvailable = item.SourceAvailable && handoffID(reviewRequestID, false) && eventKey == taskReviewRequestEventKey(reviewRequestID, item.SourceResourceID, recipient)
			if item.SourceAvailable {
				item.ReviewRequestID = reviewRequestID
			}
		}
		if item.Kind == "task_handoff_requested" {
			stored, valid := decodeTaskHandoffPayload(handoffPayload, eventKey, row.ProjectID, item.TaskID, handoffState, handoffVersion)
			item.SourceAvailable = item.SourceAvailable && valid && handoffNotificationSourceAvailable(stored, *item, recipient)
			if item.SourceAvailable {
				item.HandoffID = stored.ID
			}
		}
		if item.Kind == taskRunFailedInboxKind {
			source := taskRunFailureInboxSource{tenant: auth.target.tenantID, project: row.ProjectID, task: item.TaskID, run: failedRun, at: failedAt, sequence: failureSequence, taskVersion: item.SourceTaskVersion}
			item.SourceAvailable = item.SourceAvailable && task.Version >= item.SourceTaskVersion && source.available(*item, binding, eventKey, recipient)
			if item.SourceAvailable {
				item.RunID = failedRun
			}
		}
		if item.Kind == taskToolInputInboxKind {
			inputSource.task, inputSource.at, inputSource.taskVersion = item.TaskID, item.CreatedAt, item.SourceTaskVersion
			item.SourceAvailable = item.SourceAvailable && task.ID == item.TaskID && task.Version >= item.SourceTaskVersion &&
				inputSource.tenant == auth.target.tenantID && inputSource.project == row.ProjectID &&
				inputSource.available(*item, inputBinding, eventKey, recipient, inputRun, inputNode, inputWorker)
			if item.SourceAvailable {
				item.ToolInputTarget = &NotificationInboxToolInputTarget{
					InputID: inputSource.id, RunID: inputSource.run, NodeID: inputSource.node, Attempt: inputSource.attempt,
				}
			}
		}
		if item.Kind == "task_followed" {
			if item.SourceAvailable {
				followed[row.ProjectID] = append(followed[row.ProjectID], taskSubscriptionInboxSource{*item, task, eventKey, recipient})
			}
			item.SourceAvailable = false
		}
		if item.Kind == automationFailurePausedInboxKind {
			item.AutomationTarget = pauseSource.target(*item, auth.target.tenantID, row.ProjectID, eventKey)
			item.SourceAvailable = item.AutomationTarget != nil
		} else if item.SourceAvailable {
			item.CurrentTask = &task
			item.AssignmentCurrent = item.Kind == "task_assigned" && owner == item.SourceResourceID
		}
		items = append(items, row)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if err = rows.Close(); err != nil {
		return nil, err
	}
	// Close the page cursor before the bounded reverse-source read per project.
	// The limit applies to the entire page, including its single cursor sentinel.
	type followedSource struct {
		message string
		task    NotificationInboxTask
	}
	verified := map[string]followedSource{}
	for project, sources := range followed {
		available, sourceErr := s.taskSubscriptionInboxSourcesTx(ctx, auth.tx, auth.target.tenantID, project, auth.currentPrincipal.ID, sources)
		if sourceErr != nil {
			return nil, sourceErr
		}
		for _, source := range sources {
			if message, ok := available[source.item.ID]; ok {
				verified[source.item.ID] = followedSource{message, source.task}
			}
		}
	}
	for i := range items {
		item := &items[i].Notification
		if source, ok := verified[item.ID]; ok {
			item.SourceAvailable = true
			item.MessageID = source.message
			item.CurrentTask = &source.task
		}
	}
	return items, nil
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
		out, readErr = s.applyNotificationInboxCommandTx(bounded, auth, project, id, req, stored)
		if readErr != nil {
			return readErr
		}
		return s.saveCommandReceipt(bounded, auth, req.IdempotencyKey, hash, out)
	})
	if err != nil {
		return NotificationInboxCommandReceipt{}, err
	}
	return out, nil
}

// Single and batch commands share the exact private lifecycle transition.
func (s *Service) applyNotificationInboxCommandTx(ctx context.Context, auth *authorizedCommand, project, id string, req NotificationInboxCommand, stored notificationInboxStored) (out NotificationInboxCommandReceipt, err error) {
	if stored.version != req.ExpectedVersion {
		return out, conflict(notificationInboxChanged)
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
		result, writeErr := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_notification_inbox SET version=?,read_at=?,archived_at=?,updated_at=? WHERE tenant_id=? AND project_id=? AND recipient_user_id=? AND id=? AND version=?`), version, readAt, archivedAt, updatedAt, auth.target.tenantID, project, auth.currentPrincipal.ID, id, stored.version)
		if writeErr != nil {
			return out, writeErr
		}
		count, writeErr := result.RowsAffected()
		if writeErr != nil {
			return out, writeErr
		}
		if count != 1 {
			return out, conflict(notificationInboxChanged)
		}
	}
	out = NotificationInboxCommandReceipt{IdempotencyKey: req.IdempotencyKey, NotificationID: id, Version: version, Action: req.Action, Read: readAt != "", Archived: archivedAt != "", ReadAt: readAt, ArchivedAt: archivedAt, UpdatedAt: updatedAt}
	return out, nil
}
