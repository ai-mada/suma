package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

const toolInstallationDDL = `CREATE TABLE IF NOT EXISTS suma_tool_installation_scans(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,state VARCHAR(32) NOT NULL,version BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`
const toolInstallationColumns = "id,tenant_id,project_id,worker_id,state,version,created_at,payload"

func (s *Service) validateToolInstallationsInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name='suma_tool_installation_scans'`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='suma_tool_installation_scans'`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='suma_tool_installation_scans'`
	default:
		return fmt.Errorf("%w: installation scan dialect", store.ErrInitialSchema)
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, query).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered installation scan schema required", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT * FROM suma_tool_installation_scans WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: installation scan columns", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	want := strings.Split(toolInstallationColumns, ",")
	seen := map[string]bool{}
	for _, name := range columns {
		seen[name] = true
	}
	if len(columns) != len(want) {
		return fmt.Errorf("%w: exact installation scan columns required", store.ErrInitialSchema)
	}
	for _, name := range want {
		if !seen[name] {
			return fmt.Errorf("%w: installation scan column required", store.ErrInitialSchema)
		}
	}
	primary, _, err := s.notificationInboxSchemaKeys(ctx, "suma_tool_installation_scans")
	if err != nil {
		return err
	}
	if primary != "id" {
		return fmt.Errorf("%w: installation scan primary key required", store.ErrInitialSchema)
	}
	return nil
}
func (s *Service) InitializeToolInstallations(ctx context.Context) error {
	if err := s.validateToolInstallationsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, s.ddl(toolInstallationDDL)); err != nil {
		return err
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tool_installation_worker", "suma_tool_installation_scans", "tenant_id", "project_id", "worker_id", "state"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tool_installation_history", "suma_tool_installation_scans", "tenant_id", "project_id", "worker_id", "created_at", "id")
}

// The identity digest is private and is always rechecked against original
// worker/Pool authority; installation metadata never becomes an auth source.
type toolInstallationRow struct {
	Value          tii.Scan `json:"value"`
	Tenant         string   `json:"tenant"`
	IdentitySHA256 string   `json:"identity_sha256"`
}

func (s *Service) toolInstallationReadTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out toolInstallationRow, err error) {
	var raw, worker, state, created string
	var version int
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT payload,worker_id,state,version,created_at FROM suma_tool_installation_scans WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, id).Scan(&raw, &worker, &state, &version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if len(raw) > 128<<10 || json.Unmarshal([]byte(raw), &out) != nil || tii.ValidateScan(out.Value) != nil || out.Tenant != tenant || out.Value.ID != id || out.Value.ProjectID != project || out.Value.WorkerID != worker || out.Value.State != state || out.Value.Version != version || out.Value.CreatedAt != created || len(out.IdentitySHA256) != 64 {
		return out, toolInstallationChanged()
	}
	return out, nil
}
func (s *Service) toolInstallationSaveTx(ctx context.Context, tx *sql.Tx, row toolInstallationRow, expected int) error {
	if tii.ValidateScan(row.Value) != nil || len(stringify(row)) > 128<<10 {
		return toolInstallationChanged()
	}
	if expected == 0 {
		_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_installation_scans(id,tenant_id,project_id,worker_id,state,version,created_at,payload)VALUES(?,?,?,?,?,?,?,?)`), row.Value.ID, row.Tenant, row.Value.ProjectID, row.Value.WorkerID, row.Value.State, row.Value.Version, row.Value.CreatedAt, stringify(row))
		return err
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tool_installation_scans SET state=?,version=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), row.Value.State, row.Value.Version, stringify(row), row.Tenant, row.Value.ProjectID, row.Value.ID, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return toolInstallationChanged()
	}
	return nil
}
