package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

const projectNotificationPreferencesTable = "suma_project_notification_preferences"
const projectNotificationPreferencesColumns = "id,tenant_id,project_id,user_id,version,followed_discussions_muted,updated_at"
const projectNotificationPreferencesDDL = `CREATE TABLE IF NOT EXISTS suma_project_notification_preferences(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,user_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,followed_discussions_muted INTEGER NOT NULL,updated_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,user_id))`

func (s *Service) validateProjectNotificationPreferencesInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: project notification preferences dialect", store.ErrInitialSchema)
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, s.q(query), projectNotificationPreferencesTable).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered project notification preferences schema required", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT * FROM suma_project_notification_preferences WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: project notification preferences columns", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	wanted := strings.Split(projectNotificationPreferencesColumns, ",")
	seen := map[string]bool{}
	for _, column := range columns {
		seen[column] = true
	}
	if len(columns) != len(wanted) {
		return fmt.Errorf("%w: exact project notification preferences columns required", store.ErrInitialSchema)
	}
	for _, column := range wanted {
		if !seen[column] {
			return fmt.Errorf("%w: project notification preferences column required", store.ErrInitialSchema)
		}
	}
	primary, unique, err := s.notificationInboxSchemaKeys(ctx, projectNotificationPreferencesTable)
	if err != nil {
		return err
	}
	if primary != "id" || !unique["tenant_id,project_id,user_id"] {
		return fmt.Errorf("%w: project notification preferences identity required", store.ErrInitialSchema)
	}
	return nil
}

func (s *Service) InitializeProjectNotificationPreferences(ctx context.Context) error {
	if err := s.validateProjectNotificationPreferencesInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, s.ddl(projectNotificationPreferencesDDL))
	return err
}
