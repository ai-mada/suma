package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

const toolRecoveryDDL = `CREATE TABLE IF NOT EXISTS suma_tool_inspection_recoveries(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,inspection_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,state VARCHAR(32) NOT NULL,version BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`
const toolMaintenanceDDL = `CREATE TABLE IF NOT EXISTS suma_tool_maintenance(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,identity_sha256 VARCHAR(64) NOT NULL,observed_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,worker_id))`

func (s *Service) validateToolRecoveriesInitialSchema(ctx context.Context) error {
	tables := []struct{ name, columns, primary string }{
		{"suma_tool_inspection_recoveries", "id,tenant_id,project_id,inspection_id,worker_id,state,version,created_at,payload", "id"},
		{"suma_tool_maintenance", "tenant_id,project_id,worker_id,identity_sha256,observed_at,payload", "tenant_id,project_id,worker_id"},
	}
	for _, table := range tables {
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
		switch s.Dialect {
		case "sqlite":
		case "postgres":
			query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
		case "oceanbase":
			query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
		default:
			return fmt.Errorf("%w: maintenance dialect", store.ErrInitialSchema)
		}
		var count int
		if err := s.DB.QueryRowContext(ctx, s.q(query), table.name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			continue
		}
		var baseline string
		if err := s.DB.QueryRowContext(ctx, `SELECT version FROM suma_schema_baselines WHERE component='platform'`).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
			return fmt.Errorf("%w: registered maintenance schema required", store.ErrInitialSchema)
		}
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: maintenance columns", store.ErrInitialSchema)
		}
		columns, err := rows.Columns()
		rows.Close()
		if err != nil {
			return err
		}
		want := strings.Split(table.columns, ",")
		seen := map[string]bool{}
		for _, c := range columns {
			seen[c] = true
		}
		if len(columns) != len(want) {
			return fmt.Errorf("%w: exact maintenance columns required", store.ErrInitialSchema)
		}
		for _, c := range want {
			if !seen[c] {
				return fmt.Errorf("%w: maintenance column required", store.ErrInitialSchema)
			}
		}
		primary, _, err := s.notificationInboxSchemaKeys(ctx, table.name)
		if err != nil {
			return err
		}
		if primary != table.primary {
			return fmt.Errorf("%w: maintenance primary key required", store.ErrInitialSchema)
		}
	}
	return nil
}
func (s *Service) InitializeToolRecoveries(ctx context.Context) error {
	if err := s.validateToolRecoveriesInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	for _, ddl := range []string{toolRecoveryDDL, toolMaintenanceDDL} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tool_recovery_scope", "suma_tool_inspection_recoveries", "tenant_id", "project_id", "inspection_id", "created_at", "id")
}

type toolRecoveryRow struct {
	Value          tic.ToolInspectionRecovery `json:"value"`
	Tenant         string                     `json:"tenant"`
	IdentitySHA256 string                     `json:"identity_sha256"`
	OwnerID        string                     `json:"owner_id"`
}

func (s *Service) toolRecoveryReadTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out toolRecoveryRow, err error) {
	var raw, inspection, worker, state, created string
	var version int
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT payload,inspection_id,worker_id,state,version,created_at FROM suma_tool_inspection_recoveries WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, id).Scan(&raw, &inspection, &worker, &state, &version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if len(raw) > 64<<10 || json.Unmarshal([]byte(raw), &out) != nil || tic.ValidateRecovery(out.Value) != nil || out.Tenant != tenant || out.Value.ProjectID != project || out.Value.ID != id || out.Value.Inspection.InspectionID != inspection || out.Value.WorkerID != worker || out.Value.State != state || out.Value.Version != version || out.Value.CreatedAt != created || !contentDigest.MatchString(out.IdentitySHA256) || !tic.Operation(out.OwnerID) {
		return out, toolRecoveryChanged()
	}
	return out, nil
}
func (s *Service) toolRecoverySaveTx(ctx context.Context, tx *sql.Tx, row toolRecoveryRow, expected int) error {
	if tic.ValidateRecovery(row.Value) != nil {
		return toolRecoveryChanged()
	}
	if expected == 0 {
		_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_inspection_recoveries(id,tenant_id,project_id,inspection_id,worker_id,state,version,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?)`), row.Value.ID, row.Tenant, row.Value.ProjectID, row.Value.Inspection.InspectionID, row.Value.WorkerID, row.Value.State, row.Value.Version, row.Value.CreatedAt, stringify(row))
		return err
	}
	res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tool_inspection_recoveries SET state=?,version=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), row.Value.State, row.Value.Version, stringify(row), row.Tenant, row.Value.ProjectID, row.Value.ID, expected)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return toolRecoveryChanged()
	}
	return nil
}
