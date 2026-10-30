package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

type taskSubscriptionInboxSource struct {
	item        NotificationInboxItem
	task        NotificationInboxTask
	event, user string
}

// The sole source query is bounded by the inbox page's 50 rows plus one cursor
// sentinel. Preference cancellation does not revoke an immutable past source.
// Current project access is still provided by the enclosing inbox authority.
func (s *Service) taskSubscriptionInboxSourcesTx(ctx context.Context, tx *sql.Tx, tenant, project, user string, sources []taskSubscriptionInboxSource) (map[string]string, error) {
	available := map[string]string{}
	if len(sources) == 0 {
		return available, nil
	}
	if len(sources) > 51 {
		return nil, invalid(taskSubscriptionInvalid)
	}
	wanted := map[string]taskSubscriptionInboxSource{}
	args := []any{tenant, project, user}
	markers := make([]string, 0, len(sources))
	for _, source := range sources {
		item := source.item
		if source.user != user || item.Kind != "task_followed" || !taskSubscriptionID(item.ID) || !taskSubscriptionID(item.TaskID) || !taskSubscriptionID(item.SourceResourceID) || item.SourceTaskVersion < 1 {
			continue
		}
		if _, exists := wanted[item.ID]; exists {
			return nil, conflict(taskSubscriptionSourceInvalid)
		}
		wanted[item.ID] = source
		markers = append(markers, "?")
		args = append(args, item.ID)
	}
	if len(markers) == 0 {
		return available, nil
	}
	columns := "msg." + strings.ReplaceAll(taskConversationColumns, ",", ",msg.")
	query := `SELECT m.notification_id,m.task_id,m.subscription_id,m.recipient_user_id,m.task_version,` + columns + ` FROM suma_task_subscription_messages m JOIN suma_task_subscriptions f ON f.id=m.subscription_id AND f.tenant_id=m.tenant_id AND f.project_id=m.project_id AND f.task_id=m.task_id AND f.user_id=m.recipient_user_id JOIN suma_task_conversation_messages msg ON msg.tenant_id=m.tenant_id AND msg.project_id=m.project_id AND msg.task_id=m.task_id AND msg.id=m.message_id AND msg.task_version=m.task_version WHERE m.tenant_id=? AND m.project_id=? AND m.recipient_user_id=? AND m.notification_id IN (` + strings.Join(markers, ",") + `)`
	rows, err := tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, err
	}
	assistants := map[string]map[string]TaskConversationMessage{}
	backrefs := map[string]map[string][]string{}
	for rows.Next() {
		var notification, task, subscription, recipient string
		var version int
		var message TaskConversationMessage
		if err = rows.Scan(&notification, &task, &subscription, &recipient, &version, &message.ID, &message.Sequence, &message.TaskVersion, &message.Body, &message.AuthorID, &message.AuthorName, &message.ActorKind, &message.CreatedAt, &message.RequirementID, &message.RequirementVersion, &message.RunID); err != nil {
			rows.Close()
			return nil, err
		}
		source, ok := wanted[notification]
		if !ok || source.item.TaskID != task || source.item.SourceResourceID != subscription || source.user != recipient || recipient != user || source.item.SourceTaskVersion != version || message.TaskVersion != version || !taskSubscriptionID(message.ID) || message.Sequence < 1 || source.event != taskSubscriptionEventKey(tenant, project, task, message.ID, subscription, recipient, version) {
			continue
		}
		switch message.ActorKind {
		case "human", "api":
			if taskSubscriptionID(message.AuthorID) {
				available[notification] = message.ID
			}
		case "assistant":
			if assistants[task] == nil {
				assistants[task] = map[string]TaskConversationMessage{}
				backrefs[task] = map[string][]string{}
			}
			assistants[task][message.ID] = message
			backrefs[task][message.ID] = append(backrefs[task][message.ID], notification)
		}
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	// Human/API sources need no more SQL. Assistant messages are deduplicated
	// per task and passed together to the existing protected metadata validator.
	for task, byID := range assistants {
		messages := make([]TaskConversationMessage, 0, len(byID))
		for _, message := range byID {
			messages = append(messages, message)
		}
		valid, validateErr := s.taskSubscriptionAssistantSourcesTx(ctx, tx, tenant, project, task, messages)
		if validateErr != nil {
			return nil, validateErr
		}
		for id := range valid {
			for _, notification := range backrefs[task][id] {
				available[notification] = id
			}
		}
	}
	return available, nil
}

func taskSubscriptionUnavailableSource(err error) bool {
	var api *APIError
	var syntax *json.SyntaxError
	var shape *json.UnmarshalTypeError
	return errors.Is(err, sql.ErrNoRows) || errors.As(err, &api) && api.Status >= 400 && api.Status < 500 || errors.As(err, &syntax) || errors.As(err, &shape)
}

func (s *Service) taskSubscriptionAssistantSourcesTx(ctx context.Context, tx *sql.Tx, tenant, project, task string, messages []TaskConversationMessage) (map[string]bool, error) {
	valid := map[string]bool{}
	if len(messages) == 0 {
		return valid, nil
	}
	if len(messages) > 51 {
		return nil, invalid(taskSubscriptionInvalid)
	}
	items := append([]TaskConversationMessage(nil), messages...)
	err := s.taskConversationMessageMetadataTx(ctx, tx, tenant, project, task, items)
	if err == nil {
		for _, item := range items {
			valid[item.ID] = true
		}
		return valid, nil
	}
	if !taskSubscriptionUnavailableSource(err) {
		return nil, err
	}
	if len(messages) == 1 {
		return valid, nil
	}
	// A malformed source must not hide another message in the same task. Split
	// only on a source-validation failure; the fixed 51-row page bounds this work.
	mid := len(messages) / 2
	for _, group := range [][]TaskConversationMessage{messages[:mid], messages[mid:]} {
		part, e := s.taskSubscriptionAssistantSourcesTx(ctx, tx, tenant, project, task, group)
		if e != nil {
			return nil, e
		}
		for id := range part {
			valid[id] = true
		}
	}
	return valid, nil
}
