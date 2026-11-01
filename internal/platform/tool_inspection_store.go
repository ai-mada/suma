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
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

const toolInspectionDDL = `CREATE TABLE IF NOT EXISTS suma_tool_inspections(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,profile_id VARCHAR(64) NOT NULL,state VARCHAR(32) NOT NULL,version BIGINT NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`
const toolInspectionColumns = "id,tenant_id,project_id,worker_id,profile_id,state,version,created_at,payload"

// The initial data contract rejects unknown layouts before any platform DDL.
// This is not an upgrade/reset path, and it never repairs user data implicitly.
func (s *Service) validateToolInspectionsInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name='suma_tool_inspections'`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='suma_tool_inspections'`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='suma_tool_inspections'`
	default:
		return fmt.Errorf("%w: tool inspection dialect", store.ErrInitialSchema)
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
		return fmt.Errorf("%w: registered tool inspection schema required", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT * FROM suma_tool_inspections WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: inspection columns", store.ErrInitialSchema)
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	want := strings.Split(toolInspectionColumns, ",")
	seen := map[string]bool{}
	for _, c := range columns {
		seen[c] = true
	}
	if len(columns) != len(want) {
		return fmt.Errorf("%w: exact inspection columns required", store.ErrInitialSchema)
	}
	for _, c := range want {
		if !seen[c] {
			return fmt.Errorf("%w: inspection column required", store.ErrInitialSchema)
		}
	}
	primary, _, err := s.notificationInboxSchemaKeys(ctx, "suma_tool_inspections")
	if err != nil {
		return err
	}
	if primary != "id" {
		return fmt.Errorf("%w: inspection primary key required", store.ErrInitialSchema)
	}
	return nil
}
func (s *Service) InitializeToolInspections(ctx context.Context) error {
	if err := s.validateToolInspectionsInitialSchema(ctx); err != nil {
		return err
	}
	if err := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, s.ddl(toolInspectionDDL)); err != nil {
		return err
	}
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tool_inspection_worker", "suma_tool_inspections", "tenant_id", "project_id", "worker_id", "state"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_tool_inspection_profile", "suma_tool_inspections", "tenant_id", "project_id", "profile_id", "created_at", "id")
}

// Private frozen basis never leaves the service. Public results contain neither
// credential hashes nor local paths/argv/env; the original profile remains the
// one configuration authority.
type toolInspectionRow struct {
	Value        tic.Inspection `json:"value"`
	Tenant       string         `json:"tenant"`
	SourceSHA256 string         `json:"source_sha256"`
	Profile      tc.Profile     `json:"profile"`
}

func (toolInspectionRow) Format(s fmt.State, _ rune) {
	_, _ = io.WriteString(s, "toolInspectionRow{redacted}")
}

func (s *Service) toolInspectionReadTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out toolInspectionRow, err error) {
	var raw, worker, profile, state, created string
	var version int
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT payload,worker_id,profile_id,state,version,created_at FROM suma_tool_inspections WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, id).Scan(&raw, &worker, &profile, &state, &version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if len(raw) > 1<<20 || json.Unmarshal([]byte(raw), &out) != nil || tic.ValidateInspection(out.Value) != nil || tc.ValidateProfile(out.Profile, true) != nil || out.Tenant != tenant || out.Value.ID != id || out.Value.ProjectID != project || out.Value.WorkerID != worker || out.Value.ToolProfileID != profile || out.Value.State != state || out.Value.Version != version || out.Value.CreatedAt != created || out.Value.ProfileSHA256 != out.Profile.ProfileSHA256 {
		return out, toolInspectionChanged()
	}
	return out, nil
}
func (s *Service) toolInspectionSaveTx(ctx context.Context, tx *sql.Tx, row toolInspectionRow, expected int) error {
	if tic.ValidateInspection(row.Value) != nil {
		return toolInspectionChanged()
	}
	if expected == 0 {
		_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_inspections(id,tenant_id,project_id,worker_id,profile_id,state,version,created_at,payload)VALUES(?,?,?,?,?,?,?,?,?)`), row.Value.ID, row.Tenant, row.Value.ProjectID, row.Value.WorkerID, row.Value.ToolProfileID, row.Value.State, row.Value.Version, row.Value.CreatedAt, stringify(row))
		return err
	}
	res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tool_inspections SET state=?,version=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), row.Value.State, row.Value.Version, stringify(row), row.Tenant, row.Value.ProjectID, row.Value.ID, expected)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return toolInspectionChanged()
	}
	return nil
}
