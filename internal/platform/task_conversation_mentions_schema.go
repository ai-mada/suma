package platform

import (
	"context"
	"fmt"

	"github.com/ai-mada/suma/internal/store"
)

const taskConversationMentionsDDL = `CREATE TABLE IF NOT EXISTS suma_task_conversation_mentions(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,message_id VARCHAR(64) NOT NULL,ordinal INTEGER NOT NULL,resource_id VARCHAR(64) NOT NULL,recipient_user_id VARCHAR(64) NOT NULL,name_at_post VARCHAR(500) NOT NULL,notification_id VARCHAR(64) NOT NULL,PRIMARY KEY(tenant_id,project_id,message_id,resource_id),UNIQUE(notification_id),UNIQUE(tenant_id,project_id,message_id,recipient_user_id),UNIQUE(tenant_id,project_id,message_id,ordinal))`

// Mentions are a separate current initial domain. An absent relation can be
// installed beside valid current conversation/inbox history, without changing
// either existing table or interpreting historical message text.
func (s *Service) validateTaskConversationMentionsInitialSchema(ctx context.Context) error {
	var existence string
	switch s.Dialect {
	case "sqlite":
		existence = `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	case "postgres":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: unsupported task conversation mentions schema dialect", store.ErrInitialSchema)
	}
	const table = "suma_task_conversation_mentions"
	var count int
	if err := s.DB.QueryRowContext(ctx, s.q(existence), table).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT tenant_id,project_id,task_id,message_id,ordinal,resource_id,recipient_user_id,name_at_post,notification_id FROM suma_task_conversation_mentions WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: current task conversation mentions columns are required", store.ErrInitialSchema)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	primary, unique, err := s.notificationInboxSchemaKeys(ctx, table)
	if err != nil {
		return err
	}
	if primary != "tenant_id,project_id,message_id,resource_id" {
		return fmt.Errorf("%w: current task conversation mentions primary key is required", store.ErrInitialSchema)
	}
	for _, key := range []string{"notification_id", "tenant_id,project_id,message_id,recipient_user_id", "tenant_id,project_id,message_id,ordinal"} {
		if !unique[key] {
			return fmt.Errorf("%w: current task conversation mentions uniqueness is required", store.ErrInitialSchema)
		}
	}
	return nil
}

func (s *Service) InitializeTaskConversationMentions(ctx context.Context) error {
	if err := s.validateTaskConversationMentionsInitialSchema(ctx); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, s.ddl(taskConversationMentionsDDL)); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_task_conversation_mentions_message", "suma_task_conversation_mentions", "tenant_id", "project_id", "task_id", "message_id", "ordinal")
}
