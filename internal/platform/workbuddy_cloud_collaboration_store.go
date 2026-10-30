package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/store"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

var cloudHistoryTables = []struct {
	name, columns, primary, ddl string
	unique                      []string
}{
	{"suma_workbuddy_cloud_history_heads", "tenant_id,project_id,owner_id,submission_id,run_id,node_id,version,latest_load_id,last_loaded_id", "tenant_id,project_id,owner_id,submission_id", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_history_heads(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,submission_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,version INTEGER NOT NULL,latest_load_id VARCHAR(64) NOT NULL,last_loaded_id VARCHAR(64) NOT NULL,PRIMARY KEY(tenant_id,project_id,owner_id,submission_id),UNIQUE(tenant_id,project_id,owner_id,run_id,node_id))`, []string{"tenant_id,project_id,owner_id,run_id,node_id"}},
	{"suma_workbuddy_cloud_history_loads", "id,tenant_id,project_id,owner_id,submission_id,operation_id,request_sha256,state,deadline_at,payload", "id", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_history_loads(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,owner_id VARCHAR(64) NOT NULL,submission_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,state VARCHAR(32) NOT NULL,deadline_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,UNIQUE(tenant_id,project_id,owner_id,operation_id))`, []string{"tenant_id,project_id,owner_id,operation_id"}},
	{"suma_workbuddy_cloud_history_entries", "tenant_id,project_id,load_id,sequence,kind,text", "tenant_id,project_id,load_id,sequence", `CREATE TABLE IF NOT EXISTS suma_workbuddy_cloud_history_entries(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,load_id VARCHAR(64) NOT NULL,sequence INTEGER NOT NULL,kind VARCHAR(32) NOT NULL,text TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,load_id,sequence))`, nil},
}

// New domain tables share the supported platform baseline; no old domain table
// is changed, adopted, reset, or backfilled.
func (s *Service) validateWorkBuddyCloudCollaborationInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: cloud history dialect", store.ErrInitialSchema)
	}
	present := 0
	for _, table := range cloudHistoryTables {
		var count int
		if err := s.DB.QueryRowContext(ctx, s.q(query), table.name).Scan(&count); err != nil {
			return err
		}
		present += count
	}
	if present == 0 {
		return nil
	}
	if present != len(cloudHistoryTables) {
		return fmt.Errorf("%w: complete cloud history domain required", store.ErrInitialSchema)
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered cloud history schema required", store.ErrInitialSchema)
	}
	for _, table := range cloudHistoryTables {
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: cloud history columns", store.ErrInitialSchema)
		}
		columns, err := rows.Columns()
		rows.Close()
		if err != nil {
			return err
		}
		wanted := strings.Split(table.columns, ",")
		seen := map[string]bool{}
		for _, c := range columns {
			seen[c] = true
		}
		if len(columns) != len(wanted) {
			return fmt.Errorf("%w: exact cloud history columns required", store.ErrInitialSchema)
		}
		for _, c := range wanted {
			if !seen[c] {
				return fmt.Errorf("%w: cloud history column required", store.ErrInitialSchema)
			}
		}
		primary, unique, err := s.notificationInboxSchemaKeys(ctx, table.name)
		if err != nil {
			return err
		}
		if primary != table.primary {
			return fmt.Errorf("%w: cloud history primary key required", store.ErrInitialSchema)
		}
		for _, key := range table.unique {
			if !unique[key] {
				return fmt.Errorf("%w: cloud history uniqueness required", store.ErrInitialSchema)
			}
		}
	}
	return nil
}
func (s *Service) InitializeWorkBuddyCloudCollaboration(ctx context.Context) error {
	if err := s.validateWorkBuddyCloudCollaborationInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	for _, table := range cloudHistoryTables {
		if _, err := s.DB.ExecContext(ctx, s.ddl(table.ddl)); err != nil {
			return err
		}
	}
	return nil
}

type cloudHistoryHead struct {
	version      int
	latest, last string
}
type cloudHistoryRow struct {
	load                                  wc.CloudHistoryLoad
	owner, tenant, operation, requestHash string
}

func cloudHistorySameSource(a, b wc.CloudCollaborationSource) bool {
	a.SubmissionVersion = 0
	b.SubmissionVersion = 0
	return a == b
}
func (w *WorkBuddyConnections) cloudHistoryHeadTx(ctx context.Context, a *authorizedCommand, source wc.CloudCollaborationSource) (cloudHistoryHead, error) {
	var out cloudHistoryHead
	err := a.tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT version,latest_load_id,last_loaded_id FROM suma_workbuddy_cloud_history_heads WHERE tenant_id=? AND project_id=? AND owner_id=? AND submission_id=?`)), a.target.tenantID, a.target.projectID, a.currentPrincipal.ID, source.SubmissionID).Scan(&out.version, &out.latest, &out.last)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err == nil && (out.version < 1 || out.version > wc.MaxVersion || !handoffID(out.latest, false) || out.last != "" && !handoffID(out.last, false)) {
		return out, cloudConflict()
	}
	return out, err
}
func (w *WorkBuddyConnections) cloudHistorySaveHeadTx(ctx context.Context, a *authorizedCommand, source wc.CloudCollaborationSource, head cloudHistoryHead, before int) error {
	if head.version != before+1 || head.version > wc.MaxVersion {
		return cloudConflict()
	}
	if before == 0 {
		_, err := a.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_history_heads(tenant_id,project_id,owner_id,submission_id,run_id,node_id,version,latest_load_id,last_loaded_id)VALUES(?,?,?,?,?,?,?,?,?)`), a.target.tenantID, a.target.projectID, a.currentPrincipal.ID, source.SubmissionID, source.RunID, source.NodeID, head.version, head.latest, head.last)
		return err
	}
	result, err := a.tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_history_heads SET version=?,latest_load_id=?,last_loaded_id=? WHERE tenant_id=? AND project_id=? AND owner_id=? AND submission_id=? AND version=?`), head.version, head.latest, head.last, a.target.tenantID, a.target.projectID, a.currentPrincipal.ID, source.SubmissionID, before)
	return cloudCAS(result, err)
}
func (w *WorkBuddyConnections) cloudHistoryLoadTx(ctx context.Context, a *authorizedCommand, id string) (cloudHistoryRow, error) {
	var out cloudHistoryRow
	var raw, state, deadline, submission string
	err := a.tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT tenant_id,owner_id,submission_id,operation_id,request_sha256,state,deadline_at,payload FROM suma_workbuddy_cloud_history_loads WHERE tenant_id=? AND project_id=? AND id=?`)), a.target.tenantID, a.target.projectID, id).Scan(&out.tenant, &out.owner, &submission, &out.operation, &out.requestHash, &state, &deadline, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if out.owner != a.currentPrincipal.ID {
		return out, missing()
	}
	if json.Unmarshal([]byte(raw), &out.load) != nil || out.load.Validate() != nil || out.load.ID != id || out.load.Source.ProjectID != a.target.projectID || out.load.Source.SubmissionID != submission || out.load.State != state || out.load.DeadlineAt != deadline || out.load.OperationID != out.operation {
		return out, cloudConflict()
	}
	return out, nil
}
func (w *WorkBuddyConnections) cloudHistorySaveLoadTx(ctx context.Context, a *authorizedCommand, row cloudHistoryRow, before int) error {
	row.load.Overdue = false
	if row.load.Validate() != nil || row.load.Version != before+1 || row.owner != a.currentPrincipal.ID || row.tenant != a.target.tenantID || row.load.OperationID != row.operation {
		return cloudInvalid()
	}
	if before == 0 {
		_, err := a.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_cloud_history_loads(id,tenant_id,project_id,owner_id,submission_id,operation_id,request_sha256,state,deadline_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?)`), row.load.ID, row.tenant, a.target.projectID, row.owner, row.load.Source.SubmissionID, row.operation, row.requestHash, row.load.State, row.load.DeadlineAt, stringify(row.load))
		return err
	}
	// payload.version is protected by the project authority lock and the exact old
	// payload CAS, rather than a database-specific JSON operator.
	var old string
	if err := a.tx.QueryRowContext(ctx, w.s.q(`SELECT payload FROM suma_workbuddy_cloud_history_loads WHERE tenant_id=? AND project_id=? AND id=? AND owner_id=?`), row.tenant, a.target.projectID, row.load.ID, row.owner).Scan(&old); err != nil {
		return err
	}
	var original wc.CloudHistoryLoad
	if json.Unmarshal([]byte(old), &original) != nil || original.Version != before || original.Source != row.load.Source || original.OperationID != row.load.OperationID || original.HeadVersionAtStart != row.load.HeadVersionAtStart || original.StartedAt != row.load.StartedAt || original.DeadlineAt != row.load.DeadlineAt {
		return cloudConflict()
	}
	result, err := a.tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_cloud_history_loads SET state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND owner_id=? AND payload=?`), row.load.State, stringify(row.load), row.tenant, a.target.projectID, row.load.ID, row.owner, old)
	return cloudCAS(result, err)
}
func cloudHistoryOverdue(load wc.CloudHistoryLoad, clock time.Time) wc.CloudHistoryLoad {
	deadline, err := time.Parse(time.RFC3339Nano, load.DeadlineAt)
	load.Overdue = load.State == "loading" && (err != nil || !clock.Before(deadline))
	return load
}
