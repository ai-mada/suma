package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

const workBuddyTaskDispatchTable = "suma_workbuddy_task_dispatch_plans"
const workBuddyTaskDispatchColumns = "id,tenant_id,project_id,owner_id,run_id,version,state,created_at,expires_at,payload"
const workBuddyTaskDispatchDDL = `CREATE TABLE IF NOT EXISTS suma_workbuddy_task_dispatch_plans(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,run_id VARCHAR(64) NOT NULL UNIQUE,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`

func (s *Service) validateWorkBuddyTaskDispatchInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: WorkBuddy task dispatch dialect", store.ErrInitialSchema)
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, s.q(query), workBuddyTaskDispatchTable).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered WorkBuddy task dispatch schema required", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT * FROM suma_workbuddy_task_dispatch_plans WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: WorkBuddy task dispatch columns", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	wanted := strings.Split(workBuddyTaskDispatchColumns, ",")
	seen := map[string]bool{}
	for _, column := range columns {
		seen[column] = true
	}
	if len(columns) != len(wanted) {
		return fmt.Errorf("%w: exact WorkBuddy task dispatch columns required", store.ErrInitialSchema)
	}
	for _, column := range wanted {
		if !seen[column] {
			return fmt.Errorf("%w: WorkBuddy task dispatch column required", store.ErrInitialSchema)
		}
	}
	primary, unique, err := s.notificationInboxSchemaKeys(ctx, workBuddyTaskDispatchTable)
	if err != nil {
		return err
	}
	if primary != "id" || !unique["run_id"] {
		return fmt.Errorf("%w: WorkBuddy task dispatch identity required", store.ErrInitialSchema)
	}
	return nil
}

func (s *Service) InitializeWorkBuddyTaskDispatch(ctx context.Context) error {
	if err := s.validateWorkBuddyTaskDispatchInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, s.ddl(workBuddyTaskDispatchDDL))
	return err
}
