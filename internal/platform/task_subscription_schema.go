package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

var taskSubscriptionTables = []struct {
	name, columns, primary, ddl string
	unique                      []string
}{
	{"suma_task_subscriptions", "id,tenant_id,project_id,task_id,user_id,version,following,updated_at", "id", `CREATE TABLE IF NOT EXISTS suma_task_subscriptions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,following INTEGER NOT NULL,updated_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,task_id,user_id))`, []string{"tenant_id,project_id,task_id,user_id"}},
	{"suma_task_subscription_messages", "tenant_id,project_id,task_id,message_id,subscription_id,recipient_user_id,task_version,notification_id", "tenant_id,project_id,message_id,subscription_id", `CREATE TABLE IF NOT EXISTS suma_task_subscription_messages(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,message_id VARCHAR(64) NOT NULL,subscription_id VARCHAR(64) NOT NULL,recipient_user_id VARCHAR(64) NOT NULL,task_version INTEGER NOT NULL,notification_id VARCHAR(64) NOT NULL UNIQUE,PRIMARY KEY(tenant_id,project_id,message_id,subscription_id))`, []string{"notification_id"}},
}

func (s *Service) validateTaskSubscriptionsInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: task subscription dialect", store.ErrInitialSchema)
	}
	present := 0
	for _, table := range taskSubscriptionTables {
		var count int
		if err := s.DB.QueryRowContext(ctx, s.q(query), table.name).Scan(&count); err != nil {
			return err
		}
		present += count
	}
	if present == 0 {
		return nil
	}
	if present != len(taskSubscriptionTables) {
		return fmt.Errorf("%w: complete task subscription domain required", store.ErrInitialSchema)
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered task subscription schema required", store.ErrInitialSchema)
	}
	for _, table := range taskSubscriptionTables {
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: task subscription columns", store.ErrInitialSchema)
		}
		columns, err := rows.Columns()
		rows.Close()
		if err != nil {
			return err
		}
		wanted := strings.Split(table.columns, ",")
		seen := map[string]bool{}
		for _, column := range columns {
			seen[column] = true
		}
		if len(columns) != len(wanted) {
			return fmt.Errorf("%w: exact task subscription columns required", store.ErrInitialSchema)
		}
		for _, column := range wanted {
			if !seen[column] {
				return fmt.Errorf("%w: task subscription column required", store.ErrInitialSchema)
			}
		}
		primary, unique, err := s.notificationInboxSchemaKeys(ctx, table.name)
		if err != nil {
			return err
		}
		if primary != table.primary {
			return fmt.Errorf("%w: task subscription primary key required", store.ErrInitialSchema)
		}
		for _, key := range table.unique {
			if !unique[key] {
				return fmt.Errorf("%w: task subscription uniqueness required", store.ErrInitialSchema)
			}
		}
	}
	return nil
}
func (s *Service) InitializeTaskSubscriptions(ctx context.Context) error {
	if err := s.validateTaskSubscriptionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	for _, table := range taskSubscriptionTables {
		if _, err := s.DB.ExecContext(ctx, s.ddl(table.ddl)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_task_subscription_followers", "suma_task_subscriptions", "tenant_id", "project_id", "task_id", "following", "user_id")
}
