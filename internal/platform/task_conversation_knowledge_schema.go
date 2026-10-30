package platform

import (
	"context"
	"fmt"

	"github.com/ai-mada/suma/internal/store"
)

const taskConversationKnowledgeDDL = `CREATE TABLE IF NOT EXISTS suma_task_conversation_knowledge(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,message_id VARCHAR(64) NOT NULL,ordinal INTEGER NOT NULL,knowledge_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,sha256 VARCHAR(64) NOT NULL,title_at_post VARCHAR(200) NOT NULL,kind_at_post VARCHAR(20) NOT NULL,PRIMARY KEY(tenant_id,project_id,message_id,knowledge_id),UNIQUE(tenant_id,project_id,message_id,ordinal))`

func (s *Service) validateTaskConversationKnowledgeInitialSchema(ctx context.Context) error {
	var existence string
	switch s.Dialect {
	case "sqlite":
		existence = `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	case "postgres":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: unsupported task conversation knowledge schema dialect", store.ErrInitialSchema)
	}
	const table = "suma_task_conversation_knowledge"
	var count int
	if err := s.DB.QueryRowContext(ctx, s.q(existence), table).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT tenant_id,project_id,task_id,message_id,ordinal,knowledge_id,revision,sha256,title_at_post,kind_at_post FROM suma_task_conversation_knowledge WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: current task conversation knowledge columns are required", store.ErrInitialSchema)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	primary, unique, err := s.notificationInboxSchemaKeys(ctx, table)
	if err != nil {
		return err
	}
	if primary != "tenant_id,project_id,message_id,knowledge_id" || !unique["tenant_id,project_id,message_id,ordinal"] {
		return fmt.Errorf("%w: current task conversation knowledge identity and ordinal uniqueness are required", store.ErrInitialSchema)
	}
	return nil
}

func (s *Service) InitializeTaskConversationKnowledge(ctx context.Context) error {
	if err := s.validateTaskConversationKnowledgeInitialSchema(ctx); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, s.ddl(taskConversationKnowledgeDDL)); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_task_conversation_knowledge_message", "suma_task_conversation_knowledge", "tenant_id", "project_id", "task_id", "message_id", "ordinal")
}
