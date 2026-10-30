package dag

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

const externalAssignmentDDL = `CREATE TABLE IF NOT EXISTS dag_external_assignments(tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,dispatch_id VARCHAR(100) NOT NULL UNIQUE,protocol VARCHAR(100) NOT NULL,delegation_id VARCHAR(64) NOT NULL,delegation_version INTEGER NOT NULL,delegation_sha256 VARCHAR(64) NOT NULL,assignment TEXT NOT NULL,assignment_sha256 VARCHAR(64) NOT NULL,PRIMARY KEY(tenant_id,project_id,run_id,node_id,attempt))`
const externalAssignmentColumns = "tenant_id,project_id,run_id,node_id,attempt,fence,worker_id,dispatch_id,protocol,delegation_id,delegation_version,delegation_sha256,assignment,assignment_sha256"

// ValidateExternalAssignmentsInitialSchema precedes every initializer write.
// A current installation includes this whole domain; no old database is backfilled.
func ValidateExternalAssignmentsInitialSchema(ctx context.Context, db *sql.DB, dialect string) error {
	e := &Engine{dialect: dialect}
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	switch dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: external assignment dialect", store.ErrInitialSchema)
	}
	var present, runs int
	if err := db.QueryRowContext(ctx, e.q(query), "dag_external_assignments").Scan(&present); err != nil {
		return err
	}
	if err := db.QueryRowContext(ctx, e.q(query), "dag_runs").Scan(&runs); err != nil {
		return err
	}
	if present == 0 && runs == 0 {
		return nil
	}
	if present != 1 || runs != 1 {
		return fmt.Errorf("%w: complete external assignment domain required", store.ErrInitialSchema)
	}
	var baseline string
	if err := db.QueryRowContext(ctx, e.q(`SELECT version FROM suma_schema_baselines WHERE component='dag'`)).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered external assignment domain required", store.ErrInitialSchema)
	}
	rows, err := db.QueryContext(ctx, `SELECT * FROM dag_external_assignments WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: external assignment columns", store.ErrInitialSchema)
	}
	cols, err := rows.Columns()
	rows.Close()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, c := range cols {
		seen[c] = true
	}
	wanted := strings.Split(externalAssignmentColumns, ",")
	if len(cols) != len(wanted) {
		return fmt.Errorf("%w: exact external assignment columns required", store.ErrInitialSchema)
	}
	for _, c := range wanted {
		if !seen[c] {
			return fmt.Errorf("%w: external assignment column required", store.ErrInitialSchema)
		}
	}
	primary, unique, err := externalAssignmentKeys(ctx, db, e)
	if err != nil {
		return err
	}
	if primary != "tenant_id,project_id,run_id,node_id,attempt" || !unique["dispatch_id"] {
		return fmt.Errorf("%w: external assignment keys required", store.ErrInitialSchema)
	}
	return nil
}

func externalAssignmentKeys(ctx context.Context, db *sql.DB, e *Engine) (string, map[string]bool, error) {
	unique := map[string]bool{}
	primary := ""
	if e.dialect == "sqlite" {
		type index struct{ name, origin string }
		indexes := []index{}
		rows, err := db.QueryContext(ctx, `PRAGMA index_list(dag_external_assignments)`)
		if err != nil {
			return "", unique, err
		}
		for rows.Next() {
			var seq, u, partial int
			var name, origin string
			if err = rows.Scan(&seq, &name, &u, &origin, &partial); err != nil {
				rows.Close()
				return "", unique, err
			}
			if u == 1 && partial == 0 {
				indexes = append(indexes, index{name, origin})
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", unique, err
		}
		for _, idx := range indexes {
			rows, err = db.QueryContext(ctx, `PRAGMA index_info('`+strings.ReplaceAll(idx.name, "'", "''")+`')`)
			if err != nil {
				return "", unique, err
			}
			columns := []string{}
			for rows.Next() {
				var seq, cid int
				var name string
				if err = rows.Scan(&seq, &cid, &name); err != nil {
					rows.Close()
					return "", unique, err
				}
				columns = append(columns, name)
			}
			err = rows.Err()
			rows.Close()
			if err != nil {
				return "", unique, err
			}
			key := strings.Join(columns, ",")
			unique[key] = true
			if idx.origin == "pk" {
				primary = key
			}
		}
		return primary, unique, nil
	}
	query := `SELECT c.constraint_name,c.constraint_type,k.column_name FROM information_schema.table_constraints c JOIN information_schema.key_column_usage k ON k.constraint_catalog=c.constraint_catalog AND k.constraint_schema=c.constraint_schema AND k.constraint_name=c.constraint_name AND k.table_name=c.table_name WHERE c.table_schema=current_schema() AND c.table_name=? AND c.constraint_type IN ('PRIMARY KEY','UNIQUE') ORDER BY c.constraint_name,k.ordinal_position`
	if e.dialect == "oceanbase" {
		query = `SELECT index_name,CASE WHEN index_name='PRIMARY' THEN 'PRIMARY KEY' ELSE 'UNIQUE' END,column_name FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND non_unique=0 ORDER BY index_name,seq_in_index`
	}
	rows, err := db.QueryContext(ctx, e.q(query), "dag_external_assignments")
	if err != nil {
		return "", unique, err
	}
	defer rows.Close()
	keys := map[string][]string{}
	kinds := map[string]string{}
	for rows.Next() {
		var name, kind, column string
		if err = rows.Scan(&name, &kind, &column); err != nil {
			return "", unique, err
		}
		keys[name] = append(keys[name], column)
		kinds[name] = kind
	}
	if err = rows.Err(); err != nil {
		return "", unique, err
	}
	for name, columns := range keys {
		key := strings.Join(columns, ",")
		unique[key] = true
		if kinds[name] == "PRIMARY KEY" {
			primary = key
		}
	}
	return primary, unique, nil
}

func (e *Engine) initializeExternalAssignmentsSchema(ctx context.Context) error {
	ddl := externalAssignmentDDL
	if e.dialect == "oceanbase" {
		ddl = strings.ReplaceAll(ddl, " TEXT", " LONGTEXT")
	}
	_, err := e.db.ExecContext(ctx, ddl)
	return err
}
