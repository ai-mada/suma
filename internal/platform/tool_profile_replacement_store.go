package platform

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
	"github.com/ai-mada/suma/internal/strictjson"
)

const toolProfileReplacementTable = "suma_tool_profile_replacements"
const maxToolProfileReplacementBytes = 2 << 20
const toolProfileReplacementDDL = `CREATE TABLE IF NOT EXISTS suma_tool_profile_replacements(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,id VARCHAR(64) NOT NULL,original_id VARCHAR(64) NOT NULL,replacement_id VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,id))`
const replacementStamp = "2006-01-02T15:04:05.000000000Z"

// Only the current initial relation layout is accepted. Missing is a normal
// first initialization; an unknown existing layout is never rewritten/backfilled.
func (s *Service) validateToolProfileReplacementsInitialSchema(ctx context.Context) error {
	query := `SELECT name FROM sqlite_master WHERE type IN ('table','view') AND lower(name)=?`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema=current_schema() AND lower(table_name)=?`
	case "oceanbase":
		query = `SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND lower(table_name)=?`
	default:
		return fmt.Errorf("%w: tool replacement dialect", store.ErrInitialSchema)
	}
	rows, err := s.DB.QueryContext(ctx, s.q(query), toolProfileReplacementTable)
	if err != nil {
		return err
	}
	names := []string{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		names = append(names, name)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	if len(names) != 1 || names[0] != toolProfileReplacementTable {
		return fmt.Errorf("%w: exact tool replacement table identity required", store.ErrInitialSchema)
	}
	var baseline string
	if err = s.DB.QueryRowContext(ctx, `SELECT version FROM suma_schema_baselines WHERE component='platform'`).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered tool replacement schema required", store.ErrInitialSchema)
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT * FROM suma_tool_profile_replacements WHERE 1=0`)
	if err != nil {
		return err
	}
	columns, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	want := strings.Split("tenant_id,project_id,id,original_id,replacement_id,created_at,payload", ",")
	seen := map[string]bool{}
	for _, column := range columns {
		seen[column] = true
	}
	if len(columns) != len(want) {
		return fmt.Errorf("%w: exact tool replacement columns required", store.ErrInitialSchema)
	}
	for _, column := range want {
		if !seen[column] {
			return fmt.Errorf("%w: tool replacement column required", store.ErrInitialSchema)
		}
	}
	primary, _, err := s.notificationInboxSchemaKeys(ctx, toolProfileReplacementTable)
	if err != nil {
		return err
	}
	if primary != "tenant_id,project_id,id" {
		return fmt.Errorf("%w: tool replacement primary key required", store.ErrInitialSchema)
	}
	return nil
}
func (s *Service) InitializeToolProfileReplacements(ctx context.Context) error {
	if err := s.validateToolProfileReplacementsInitialSchema(ctx); err != nil {
		return err
	}
	if _, err := s.DB.ExecContext(ctx, s.ddl(toolProfileReplacementDDL)); err != nil {
		return err
	}
	for _, direction := range []struct{ name, column string }{{"suma_tool_replacement_outgoing", "original_id"}, {"suma_tool_replacement_incoming", "replacement_id"}} {
		if err := store.EnsureIndex(ctx, s.DB, s.Dialect, direction.name, toolProfileReplacementTable, "tenant_id", "project_id", direction.column, "created_at", "id"); err != nil {
			return err
		}
	}
	return nil
}
func decodeToolProfileReplacement(raw string) (out ToolProfileReplacement, err error) {
	if strictjson.Decode([]byte(raw), &out, maxToolProfileReplacementBytes) != nil || !ValidToolProfileReplacement(out) {
		return out, toolReplacementChanged()
	}
	return out, nil
}
func (s *Service) toolProfileReplacementTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out ToolProfileReplacement, err error) {
	var raw, original, replacement, created string
	err = tx.QueryRowContext(ctx, s.q(`SELECT payload,original_id,replacement_id,created_at FROM suma_tool_profile_replacements WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw, &original, &replacement, &created)
	if err == sql.ErrNoRows {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	out, err = decodeToolProfileReplacement(raw)
	if err != nil {
		return out, err
	}
	if out.ID != id || out.ProjectID != project || out.Original.ID != original || out.Replacement.ID != replacement || out.CreatedAt != created {
		return ToolProfileReplacement{}, toolReplacementChanged()
	}
	return out, nil
}
