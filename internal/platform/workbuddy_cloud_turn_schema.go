package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

var cloudTurnTables = []struct {
	name, columns, primary, ddl string
	unique                      []string
}{
	{"suma_workbuddy_cloud_turn_heads", "tenant_id,project_id,owner_id,submission_id,version,latest_turn_id", "tenant_id,project_id,owner_id,submission_id", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_turn_heads(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,submission_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,latest_turn_id VARCHAR(64) NOT NULL,PRIMARY KEY(tenant_id,project_id,owner_id,submission_id))`, nil},
	{"suma_workbuddy_cloud_turns", "id,tenant_id,project_id,owner_id,submission_id,operation_id,request_sha256,version,state,lease_owner,lease_fence,lease_until,deadline_at,prompt_state,entry_count,text_bytes,tools_invalid,prompt,payload", "id", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_turns(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,submission_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(32) NOT NULL,lease_owner VARCHAR(64) NOT NULL,lease_fence BIGINT NOT NULL,lease_until VARCHAR(40) NOT NULL,deadline_at VARCHAR(40) NOT NULL,prompt_state VARCHAR(32) NOT NULL,entry_count INTEGER NOT NULL,text_bytes INTEGER NOT NULL,tools_invalid INTEGER NOT NULL,prompt TEXT NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,owner_id,operation_id))`, []string{"tenant_id,project_id,owner_id,operation_id"}},
	{"suma_workbuddy_cloud_turn_entries", "tenant_id,project_id,turn_id,sequence,payload", "tenant_id,project_id,turn_id,sequence", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_turn_entries(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,turn_id VARCHAR(64) NOT NULL,sequence INTEGER NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,turn_id,sequence))`, nil},
	{"suma_workbuddy_cloud_turn_permissions", "id,tenant_id,project_id,turn_id,version,state,payload", "id", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_turn_permissions(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,turn_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(32) NOT NULL,payload TEXT NOT NULL)`, nil},
	{"suma_workbuddy_cloud_turn_commands", "id,tenant_id,project_id,owner_id,turn_id,operation_id,request_sha256,kind,state,send_nonce_sha256,payload", "id", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_turn_commands(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,turn_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,kind VARCHAR(32) NOT NULL,state VARCHAR(32) NOT NULL,send_nonce_sha256 VARCHAR(64) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,owner_id,operation_id))`, []string{"tenant_id,project_id,owner_id,operation_id"}},
	{"suma_workbuddy_cloud_turn_tools", "tenant_id,project_id,turn_id,tool_call_id,state", "tenant_id,project_id,turn_id,tool_call_id", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_turn_tools(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,turn_id VARCHAR(64) NOT NULL,tool_call_id VARCHAR(256) NOT NULL,state VARCHAR(32) NOT NULL,PRIMARY KEY(tenant_id,project_id,turn_id,tool_call_id))`, nil},
}

func (s *Service) validateWorkBuddyCloudTurnsInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: cloud turn dialect", store.ErrInitialSchema)
	}
	present := 0
	for _, table := range cloudTurnTables {
		var count int
		if err := s.DB.QueryRowContext(ctx, s.q(query), table.name).Scan(&count); err != nil {
			return err
		}
		present += count
	}
	if present == 0 {
		return nil
	}
	if present != len(cloudTurnTables) {
		return fmt.Errorf("%w: complete cloud turn domain required", store.ErrInitialSchema)
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered cloud turn schema required", store.ErrInitialSchema)
	}
	for _, table := range cloudTurnTables {
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: cloud turn columns", store.ErrInitialSchema)
		}
		columns, err := rows.Columns()
		rows.Close()
		if err != nil {
			return err
		}
		wanted := strings.Split(table.columns, ",")
		if len(columns) != len(wanted) {
			return fmt.Errorf("%w: exact cloud turn columns required", store.ErrInitialSchema)
		}
		seen := map[string]bool{}
		for _, column := range columns {
			seen[column] = true
		}
		for _, column := range wanted {
			if !seen[column] {
				return fmt.Errorf("%w: cloud turn column required", store.ErrInitialSchema)
			}
		}
		primary, unique, err := s.notificationInboxSchemaKeys(ctx, table.name)
		if err != nil {
			return err
		}
		if primary != table.primary {
			return fmt.Errorf("%w: cloud turn primary key required", store.ErrInitialSchema)
		}
		for _, key := range table.unique {
			if !unique[key] {
				return fmt.Errorf("%w: cloud turn uniqueness required", store.ErrInitialSchema)
			}
		}
	}
	return nil
}
func (s *Service) InitializeWorkBuddyCloudTurns(ctx context.Context) error {
	if err := s.validateWorkBuddyCloudTurnsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	for _, table := range cloudTurnTables {
		if _, err := s.DB.ExecContext(ctx, s.ddl(table.ddl)); err != nil {
			return err
		}
	}
	return nil
}
