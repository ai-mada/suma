package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

const workBuddyConnectionsDDL = `CREATE TABLE IF NOT EXISTS suma_workbuddy_connections(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,client_id VARCHAR(512) NOT NULL,open_id VARCHAR(512) NOT NULL,version INTEGER NOT NULL,scopes_json TEXT NOT NULL,state VARCHAR(32) NOT NULL,grant_generation INTEGER NOT NULL,token_generation INTEGER NOT NULL,expires_at VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,last_error VARCHAR(100) NOT NULL,app_digest VARCHAR(64) NOT NULL,ciphertext TEXT NOT NULL,active_operation_id VARCHAR(64) NOT NULL)`
const workBuddyOAuthOperationsDDL = `CREATE TABLE IF NOT EXISTS suma_workbuddy_oauth_operations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,connection_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,kind VARCHAR(20) NOT NULL,state VARCHAR(20) NOT NULL,request_hash VARCHAR(64) NOT NULL,credential_digest VARCHAR(64) NOT NULL,state_hash VARCHAR(64) NOT NULL,code_hash VARCHAR(64) NOT NULL,app_digest VARCHAR(64) NOT NULL,scopes_json TEXT NOT NULL,version INTEGER NOT NULL,connection_version INTEGER NOT NULL,grant_generation INTEGER NOT NULL,token_generation INTEGER NOT NULL,expires_at VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,last_error VARCHAR(100) NOT NULL)`

// Validate before any DDL. A declared current platform may install this new,
// empty domain; an existing partial domain is never repaired or interpreted.
func (s *Service) validateWorkBuddyConnectionsInitialSchema(ctx context.Context) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("%w: WorkBuddy database unavailable", store.ErrInitialSchema)
	}
	tables := []struct{ name, columns string }{{"suma_workbuddy_connections", workBuddyConnectionColumns}, {"suma_workbuddy_oauth_operations", workBuddyOAuthOperationColumns}}
	var existence string
	switch s.Dialect {
	case "sqlite":
		existence = `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	case "postgres":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: unsupported WorkBuddy schema dialect", store.ErrInitialSchema)
	}
	present := 0
	for _, table := range tables {
		var count int
		if err := s.DB.QueryRowContext(ctx, s.q(existence), table.name).Scan(&count); err != nil {
			return err
		}
		present += count
	}
	if present == 0 {
		return nil
	}
	if present != len(tables) {
		return fmt.Errorf("%w: complete WorkBuddy connection domain required", store.ErrInitialSchema)
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component=?`), "platform").Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: WorkBuddy domain requires current platform declaration", store.ErrInitialSchema)
	}
	for _, table := range tables {
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: WorkBuddy table unavailable", store.ErrInitialSchema)
		}
		columns, err := rows.Columns()
		closeErr := rows.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		wanted := strings.Split(table.columns, ",")
		seen := map[string]bool{}
		for _, column := range columns {
			seen[column] = true
		}
		if len(columns) != len(wanted) {
			return fmt.Errorf("%w: exact WorkBuddy columns required", store.ErrInitialSchema)
		}
		for _, column := range wanted {
			if !seen[column] {
				return fmt.Errorf("%w: current WorkBuddy column required", store.ErrInitialSchema)
			}
		}
		primary, _, err := s.notificationInboxSchemaKeys(ctx, table.name)
		if err != nil {
			return err
		}
		if primary != "id" {
			return fmt.Errorf("%w: WorkBuddy primary key required", store.ErrInitialSchema)
		}
	}
	return nil
}
func (s *Service) InitializeWorkBuddyConnections(ctx context.Context) error {
	if err := s.validateWorkBuddyConnectionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	for _, ddl := range []string{workBuddyConnectionsDDL, workBuddyOAuthOperationsDDL} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_workbuddy_connections_owner", "suma_workbuddy_connections", "tenant_id", "project_id", "owner_id", "id"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_workbuddy_oauth_connection", "suma_workbuddy_oauth_operations", "tenant_id", "project_id", "connection_id", "id")
}
