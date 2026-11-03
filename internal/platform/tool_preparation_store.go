package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/ai-mada/suma/internal/store"
	ti "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

const toolPreparationDDL = `CREATE TABLE IF NOT EXISTS suma_tool_preparations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,state VARCHAR(32) NOT NULL,version BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`
const toolPreparationColumns = "id,tenant_id,project_id,worker_id,state,version,created_at,payload"

// Validate the entire current layout before any DDL. The initial contract has
// no reset, migration backfill or interpretation of an unknown existing table.
func (s *Service) validateToolPreparationsInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name='suma_tool_preparations'`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='suma_tool_preparations'`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='suma_tool_preparations'`
	default:
		return fmt.Errorf("%w: tool preparation dialect", store.ErrInitialSchema)
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	var baseline string
	if err := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered tool preparation schema required", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT * FROM suma_tool_preparations WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: tool preparation columns", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	want, seen := strings.Split(toolPreparationColumns, ","), map[string]bool{}
	for _, name := range columns {
		seen[name] = true
	}
	if len(columns) != len(want) {
		return fmt.Errorf("%w: exact tool preparation columns required", store.ErrInitialSchema)
	}
	for _, name := range want {
		if !seen[name] {
			return fmt.Errorf("%w: tool preparation column required", store.ErrInitialSchema)
		}
	}
	primary, _, err := s.notificationInboxSchemaKeys(ctx, "suma_tool_preparations")
	if err != nil {
		return err
	}
	if primary != "id" {
		return fmt.Errorf("%w: tool preparation primary key required", store.ErrInitialSchema)
	}
	return nil
}

func (s *Service) InitializeToolPreparations(ctx context.Context) error {
	if err := s.validateToolPreparationsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, s.ddl(toolPreparationDDL)); err != nil {
		return err
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tool_preparation_worker", "suma_tool_preparations", "tenant_id", "project_id", "worker_id", "state"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tool_preparation_history", "suma_tool_preparations", "tenant_id", "project_id", "worker_id", "created_at", "id")
}

// The machine credential identity and immutable source scan stay private.
// They are rechecked at every new effect, not accepted from a browser digest.
type toolPreparationRow struct {
	Value          tp.Operation `json:"value"`
	Tenant         string       `json:"tenant"`
	IdentitySHA256 string       `json:"identity_sha256"`
	Scan           ti.Scan      `json:"scan"`
}

func (toolPreparationRow) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "toolPreparationRow{redacted}")
}

func (s *Service) toolPreparationReadTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out toolPreparationRow, err error) {
	var raw, worker, state, created string
	var version int
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT payload,worker_id,state,version,created_at FROM suma_tool_preparations WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, id).Scan(&raw, &worker, &state, &version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if len(raw) > 512<<10 || json.Unmarshal([]byte(raw), &out) != nil || out.Tenant != tenant || out.Value.TenantID != tenant || out.Value.ProjectID != project || out.Value.ID != id || out.Value.WorkerID != worker || out.Value.State != state || out.Value.Version != version || out.Value.CreatedAt != created || tp.ValidateOperation(out.Value) != nil || !tp.ValidDigest(out.IdentitySHA256) || ti.ValidateScan(out.Scan) != nil || out.Scan.ID != out.Value.InstallationScanID || out.Scan.Version != out.Value.InstallationScanVersion || out.Scan.WorkerID != worker || out.Scan.ProjectID != project || out.Scan.SourceSHA256 != out.Value.InstallationSourceSHA256 || out.Scan.Result == nil || out.Scan.Result.StopProof == nil || stringify(out.Scan.Result.StopProof) != stringify(out.Value.InstallationStopProof) {
		return toolPreparationRow{}, toolPreparationChanged()
	}
	return out, nil
}

func (s *Service) toolPreparationSaveTx(ctx context.Context, tx *sql.Tx, row toolPreparationRow, expected int) error {
	value := row.Value
	if tp.ValidateOperation(value) != nil {
		return toolPreparationChanged()
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tool_preparations SET state=?,version=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), value.State, value.Version, stringify(row), row.Tenant, value.ProjectID, value.ID, expected)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return toolPreparationChanged()
	}
	return nil
}
