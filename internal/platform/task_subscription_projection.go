package platform

import (
	"context"
	"database/sql"
	"errors"
)

func taskSubscriptionEventKey(tenant, project, task, message, subscription, user string, taskVersion int) string {
	return digest(stringify([]any{"task-followed-v1", tenant, project, task, message, subscription, user, taskVersion}))
}

// Read exactly the saved source. In particular an assistant requires the
// existing protected reply relation and completed, sealed backing reply.
func (s *Service) taskSubscriptionMessageTx(ctx context.Context, tx *sql.Tx, tenant, project, task, id string) (TaskConversationMessage, error) {
	message, err := scanTaskConversation(tx.QueryRowContext(ctx, s.q(`SELECT `+taskConversationColumns+` FROM suma_task_conversation_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND id=?`), tenant, project, task, id))
	if err != nil {
		return message, err
	}
	if !taskSubscriptionID(message.ID) || message.TaskVersion < 1 || message.Sequence < 1 {
		return TaskConversationMessage{}, conflict(taskSubscriptionSourceInvalid)
	}
	switch message.ActorKind {
	case "human", "api":
		if !taskSubscriptionID(message.AuthorID) {
			return TaskConversationMessage{}, conflict(taskSubscriptionSourceInvalid)
		}
	case "assistant":
		items := []TaskConversationMessage{message}
		if err = s.taskConversationMessageMetadataTx(ctx, tx, tenant, project, task, items); err != nil {
			return TaskConversationMessage{}, err
		}
		message = items[0]
	default:
		return TaskConversationMessage{}, conflict(taskSubscriptionSourceInvalid)
	}
	return message, nil
}

func (s *Service) projectTaskSubscriptionMessageTx(ctx context.Context, auth *authorizedCommand, project, task, messageID string) error {
	if auth == nil || !taskConversationActor(auth.actor) {
		return forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, project); err != nil {
		return err
	}
	message, err := s.taskSubscriptionMessageTx(ctx, auth.tx, auth.target.tenantID, project, task, messageID)
	if err != nil {
		return err
	}
	if message.ActorKind != "human" && message.ActorKind != "api" || message.AuthorID != auth.currentPrincipal.ID {
		return forbidden()
	}
	return s.projectTaskSubscriptionFollowersTx(ctx, auth.tx, auth.target.tenantID, project, task, message)
}

// Called only after the existing result hook has saved completed reply and
// outcome facts in this original authorized machine transaction. No human
// Actor or new recipient authority locks are constructed for delivery.
func (s *Service) projectTaskSubscriptionReplyTx(ctx context.Context, tx *sql.Tx, tenant, project, task, messageID, replyID string) error {
	message, err := s.taskSubscriptionMessageTx(ctx, tx, tenant, project, task, messageID)
	if err != nil {
		return err
	}
	if message.ActorKind != "assistant" || message.ReplyID != replyID {
		return forbidden()
	}
	return s.projectTaskSubscriptionFollowersTx(ctx, tx, tenant, project, task, message)
}

func (s *Service) projectTaskSubscriptionFollowersTx(ctx context.Context, tx *sql.Tx, tenant, project, task string, message TaskConversationMessage) error {
	budget, ok := s.authorityBudgets.Load(tx)
	if !ok || !budget.(*authorityTxBudget).lockedOrganizations[tenant] {
		return forbidden()
	}
	if err := budget.(*authorityTxBudget).ctx.Err(); err != nil {
		return err
	}
	var currentVersion int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, task).Scan(&currentVersion); err != nil {
		return err
	} else if currentVersion != message.TaskVersion {
		return conflict(taskSubscriptionSourceInvalid)
	}
	// One set query covers every current account with read access. Neither a
	// human-resource row nor a per-recipient identity lock is required.
	query := `SELECT f.id,f.user_id FROM suma_task_subscriptions f JOIN suma_users u ON u.id=f.user_id JOIN suma_org_members om ON om.tenant_id=f.tenant_id AND om.user_id=f.user_id LEFT JOIN suma_project_members pm ON pm.project_id=f.project_id AND pm.user_id=f.user_id WHERE f.tenant_id=? AND f.project_id=? AND f.task_id=? AND f.following=1 AND (om.role IN ('owner','admin') OR (om.role='member' AND pm.role IN ('admin','reviewer','developer','viewer'))) AND NOT EXISTS(SELECT 1 FROM suma_task_conversation_mentions m WHERE m.tenant_id=f.tenant_id AND m.project_id=f.project_id AND m.task_id=f.task_id AND m.message_id=? AND m.recipient_user_id=f.user_id) ORDER BY f.user_id`
	rows, err := tx.QueryContext(ctx, s.q(query), tenant, project, task, message.ID)
	if err != nil {
		return err
	}
	type target struct{ id, user string }
	targets := []target{}
	for rows.Next() {
		var t target
		if err = rows.Scan(&t.id, &t.user); err != nil {
			rows.Close()
			return err
		}
		if !taskSubscriptionID(t.id) || !taskSubscriptionID(t.user) {
			rows.Close()
			return conflict(taskSubscriptionSourceInvalid)
		}
		if message.ActorKind != "assistant" && t.user == message.AuthorID {
			continue
		}
		targets = append(targets, t)
	}
	err = rows.Err()
	closeErr := rows.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	for _, t := range targets {
		key := taskSubscriptionEventKey(tenant, project, task, message.ID, t.id, t.user, message.TaskVersion)
		notification, insertErr := s.insertNotificationInboxSourceTx(ctx, tx, tenant, project, message.CreatedAt, notificationInboxSource{Kind: "task_followed", TaskID: task, TaskVersion: message.TaskVersion, ResourceID: t.id, EventKey: key, RecipientUserID: t.user})
		if insertErr != nil {
			return insertErr
		}
		var existing string
		readErr := tx.QueryRowContext(ctx, s.q(`SELECT notification_id FROM suma_task_subscription_messages WHERE tenant_id=? AND project_id=? AND task_id=? AND message_id=? AND subscription_id=? AND recipient_user_id=? AND task_version=?`), tenant, project, task, message.ID, t.id, t.user, message.TaskVersion).Scan(&existing)
		if readErr == nil {
			if existing != notification {
				return conflict(taskSubscriptionSourceInvalid)
			}
			continue
		}
		if !errors.Is(readErr, sql.ErrNoRows) {
			return readErr
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_subscription_messages(tenant_id,project_id,task_id,message_id,subscription_id,recipient_user_id,task_version,notification_id)VALUES(?,?,?,?,?,?,?,?)`), tenant, project, task, message.ID, t.id, t.user, message.TaskVersion, notification); err != nil {
			return err
		}
	}
	return nil
}
