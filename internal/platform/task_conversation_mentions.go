package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const taskConversationMentionInvalid = "Invalid task conversation mention request or cursor"
const taskConversationMentionUnavailable = "One or more mention recipients are no longer available; reload the current people directory"
const taskConversationMentionSourceInvalid = "The task conversation mention source is invalid"

// The shared closed decoder permits null slices generally. This optional
// command field has a closed array shape: omission or an array, never null.
func (req *TaskConversationAppendRequest) UnmarshalJSON(raw []byte) error {
	type request TaskConversationAppendRequest
	var decoded request
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if value, ok := fields["mention_resource_ids"]; ok && strings.TrimSpace(string(value)) == "null" {
		return errors.New(taskConversationMentionInvalid)
	}
	if value, ok := fields["knowledge"]; ok && strings.TrimSpace(string(value)) == "null" {
		return errors.New(taskConversationInvalid)
	}
	*req = TaskConversationAppendRequest(decoded)
	return nil
}

func taskConversationMentionEventKey(message, resource, user string) string {
	return digest(stringify(struct {
		Kind, Message, Resource, User string
	}{"task_mentioned", message, resource, user}))
}

func validTaskConversationMentionIDs(ids []string) bool {
	if len(ids) > 20 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !handoffID(id, false) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func normalizeTaskConversationMentionTargetQuery(q TaskConversationMentionTargetQuery) (TaskConversationMentionTargetQuery, bool) {
	if q.Limit == 0 {
		q.Limit = 20
	}
	valid := q.Limit >= 1 && q.Limit <= 50 && taskWorkspaceText(q.Q) && utf8.RuneCountInString(q.Q) <= 200 && len(q.Cursor) <= 4096 && validTaskConversationMentionIDs(q.ResourceIDs)
	return q, valid && (len(q.ResourceIDs) == 0 || q.Q == "" && q.Cursor == "")
}

type taskConversationMentionTargetCursor struct {
	Version int    `json:"version"`
	Tenant  string `json:"tenant"`
	User    string `json:"user"`
	Project string `json:"project"`
	Task    string `json:"task"`
	Query   string `json:"query"`
	Limit   int    `json:"limit"`
	After   string `json:"after"`
}

func scanTaskConversationMentionTarget(row scanner) (out TaskConversationMentionTarget, err error) {
	err = row.Scan(&out.ResourceID, &out.UserID, &out.Name, &out.ResourceVersion)
	if err == nil && (!handoffID(out.ResourceID, false) || !handoffID(out.UserID, false) || out.ResourceVersion < 1 || !utf8.ValidString(out.Name)) {
		err = conflict(taskConversationMentionSourceInvalid)
	}
	return out, err
}

func (s *Service) taskConversationMentionTargetsTx(ctx context.Context, auth *authorizedCommand, project, task string, q TaskConversationMentionTargetQuery) (out TaskConversationMentionTargetPage, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return out, err
	}
	out = TaskConversationMentionTargetPage{ProjectID: project, TaskID: task, Limit: q.Limit, Items: []TaskConversationMentionTarget{}}
	where, args := s.canonicalHumanTargetsWhere(auth.target.tenantID, project, false)
	if len(q.ResourceIDs) != 0 {
		markers := make([]string, len(q.ResourceIDs))
		for i, id := range q.ResourceIDs {
			markers[i] = "?"
			args = append(args, id)
		}
		where += ` AND m.id IN (` + strings.Join(markers, ",") + `)`
		rows, queryErr := auth.tx.QueryContext(ctx, s.q(`SELECT m.id,h.user_id,m.title,m.version`+where+` ORDER BY m.id LIMIT 20`), args...)
		if queryErr != nil {
			return out, queryErr
		}
		byID := map[string]TaskConversationMentionTarget{}
		for rows.Next() {
			item, scanErr := scanTaskConversationMentionTarget(rows)
			if scanErr != nil {
				rows.Close()
				return out, scanErr
			}
			byID[item.ResourceID] = item
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return out, err
		}
		for _, id := range q.ResourceIDs {
			if item, ok := byID[id]; ok {
				out.Items = append(out.Items, item)
			}
		}
		out.Total = len(out.Items)
		return out, nil
	}
	if q.Q != "" {
		literal := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q.Q) + "%"
		where += ` AND (LOWER(m.title) LIKE LOWER(?) ESCAPE '!' OR LOWER(m.id) LIKE LOWER(?) ESCAPE '!')`
		args = append(args, literal, literal)
	}
	scope := taskConversationMentionTargetCursor{Version: 1, Tenant: auth.target.tenantID, User: auth.currentPrincipal.ID, Project: project, Task: task, Query: digest(q.Q), Limit: q.Limit}
	var position taskConversationMentionTargetCursor
	if q.Cursor != "" {
		raw, decodeErr := base64.RawURLEncoding.Strict().DecodeString(q.Cursor)
		if decodeErr != nil || json.Unmarshal(raw, &position) != nil || base64.RawURLEncoding.EncodeToString([]byte(stringify(position))) != q.Cursor {
			return out, invalid(taskConversationMentionInvalid)
		}
		identity := position
		identity.After = ""
		if identity != scope || !handoffID(position.After, false) {
			return out, invalid(taskConversationMentionInvalid)
		}
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if q.Cursor != "" {
		where += ` AND m.id>?`
		args = append(args, position.After)
	}
	args = append(args, q.Limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT m.id,h.user_id,m.title,m.version`+where+` ORDER BY m.id ASC LIMIT ?`), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanTaskConversationMentionTarget(rows)
		if scanErr != nil {
			return out, scanErr
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		scope.After = out.Items[q.Limit-1].ResourceID
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	return out, nil
}

// Each directory request reauthorizes the exact task. Pagination is a live
// current directory, not a frozen promise that a later post will be eligible.
func (s *Service) ListTaskConversationMentionTargets(ctx context.Context, actor Actor, project, taskID string, q TaskConversationMentionTargetQuery) (out TaskConversationMentionTargetPage, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	var valid bool
	q, valid = normalizeTaskConversationMentionTargetQuery(q)
	if !valid || !handoffID(project, false) || !handoffID(taskID, false) {
		return out, invalid(taskConversationMentionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = s.getRecordTx(ctx, auth, project, "tasks", taskID); err != nil {
		return out, err
	}
	out, err = s.taskConversationMentionTargetsTx(ctx, auth, project, taskID, q)
	if err != nil {
		return TaskConversationMentionTargetPage{}, err
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}

func (s *Service) TaskConversationMessageDetail(ctx context.Context, actor Actor, project, taskID, messageID string) (out TaskConversationMessageDetail, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(taskID, false) || !handoffID(messageID, false) {
		return out, invalid(taskConversationMentionInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return out, err
	}
	message, err := scanTaskConversation(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), auth.target.tenantID, project, taskID, messageID))
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	items := []TaskConversationMessage{message}
	if err = s.taskConversationMessageMetadataTx(ctx, auth.tx, auth.target.tenantID, project, taskID, items); err != nil {
		return out, err
	}
	out = TaskConversationMessageDetail{ProjectID: project, TaskID: taskID, TaskVersion: task.Version, Message: items[0]}
	_, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, false)
	if err == nil {
		out.CanPost = true
	} else {
		var api *APIError
		if !errors.As(err, &api) || api.Status != 403 {
			return TaskConversationMessageDetail{}, err
		}
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}

// This is reached only after original-operation recovery and the saved human
// message INSERT. Every recipient is resolved under the original organization
// authority serialization; no recipient locks invert that lock order.
func (s *Service) projectTaskConversationMentionsTx(ctx context.Context, auth *authorizedCommand, project, taskID string, message *TaskConversationMessage, resourceIDs []string) error {
	if !validTaskConversationMentionIDs(resourceIDs) {
		return invalid(taskConversationMentionInvalid)
	}
	if len(resourceIDs) == 0 {
		return nil
	}
	if auth == nil || !taskConversationActor(auth.actor) || message == nil {
		return forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, project); err != nil {
		return err
	}
	if message.ActorKind != "human" && message.ActorKind != "api" || message.AuthorID != auth.currentPrincipal.ID || len(message.Mentions) != 0 {
		return conflict(taskConversationMentionSourceInvalid)
	}
	saved, err := scanTaskConversation(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), auth.target.tenantID, project, taskID, message.ID))
	if errors.Is(err, sql.ErrNoRows) || err == nil && stringify(saved) != stringify(*message) {
		return conflict(taskConversationMentionSourceInvalid)
	}
	if err != nil {
		return err
	}
	var taskVersion int
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, taskID).Scan(&taskVersion); errors.Is(err, sql.ErrNoRows) || err == nil && taskVersion != message.TaskVersion {
		return conflict(taskConversationMentionSourceInvalid)
	} else if err != nil {
		return err
	}
	targets, err := s.taskConversationMentionTargetsTx(ctx, auth, project, taskID, TaskConversationMentionTargetQuery{Limit: 20, ResourceIDs: resourceIDs})
	if err != nil {
		return err
	}
	if len(targets.Items) != len(resourceIDs) {
		return invalid(taskConversationMentionUnavailable)
	}
	mentions := make([]TaskConversationMention, 0, len(targets.Items))
	users := map[string]bool{}
	for ordinal, target := range targets.Items {
		if users[target.UserID] {
			return conflict(taskConversationMentionSourceInvalid)
		}
		users[target.UserID] = true
		eventKey := taskConversationMentionEventKey(message.ID, target.ResourceID, target.UserID)
		notificationID, insertErr := s.insertNotificationInboxTx(ctx, auth, project, notificationInboxSource{Kind: "task_mentioned", TaskID: taskID, TaskVersion: message.TaskVersion, ResourceID: target.ResourceID, EventKey: eventKey, RecipientUserID: target.UserID})
		if insertErr != nil {
			return insertErr
		}
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_conversation_mentions(tenant_id,project_id,task_id,message_id,ordinal,resource_id,recipient_user_id,name_at_post,notification_id) VALUES(?,?,?,?,?,?,?,?,?)`), auth.target.tenantID, project, taskID, message.ID, ordinal, target.ResourceID, target.UserID, target.Name, notificationID); err != nil {
			return err
		}
		mentions = append(mentions, TaskConversationMention{ResourceID: target.ResourceID, UserID: target.UserID, NameAtPost: target.Name})
	}
	message.Mentions = mentions
	return nil
}

// Immutable refs are shared by ordinary reads and protected source captures.
// Current member data is deliberately absent from this query.
func (s *Service) taskConversationMentionsMetadataTx(ctx context.Context, tx *sql.Tx, tenant, project, task string, items []TaskConversationMessage) error {
	if len(items) == 0 {
		return nil
	}
	args := []any{tenant, project, task}
	markers := make([]string, len(items))
	positions := map[string]int{}
	users := make([]map[string]bool, len(items))
	for i, item := range items {
		markers[i] = "?"
		args = append(args, item.ID)
		positions[item.ID] = i
		items[i].Mentions = nil
		users[i] = map[string]bool{}
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT message_id,ordinal,resource_id,recipient_user_id,name_at_post,notification_id FROM suma_task_conversation_mentions WHERE tenant_id=? AND project_id=? AND task_id=? AND message_id IN (`+strings.Join(markers, ",")+`) ORDER BY message_id,ordinal`), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var message, notification string
		var ordinal int
		var mention TaskConversationMention
		if err = rows.Scan(&message, &ordinal, &mention.ResourceID, &mention.UserID, &mention.NameAtPost, &notification); err != nil {
			return err
		}
		position, ok := positions[message]
		if !ok || items[position].ActorKind != "human" && items[position].ActorKind != "api" || ordinal != len(items[position].Mentions) || ordinal >= 20 || !handoffID(mention.ResourceID, false) || !handoffID(mention.UserID, false) || !handoffID(notification, false) || !utf8.ValidString(mention.NameAtPost) || users[position][mention.UserID] {
			return conflict(taskConversationMentionSourceInvalid)
		}
		users[position][mention.UserID] = true
		items[position].Mentions = append(items[position].Mentions, mention)
	}
	return rows.Err()
}
