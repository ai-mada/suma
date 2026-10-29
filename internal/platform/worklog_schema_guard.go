package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

// The worklog domain is independent initial-version data. A declared current
// database may install it empty. An existing partial/incompatible domain must
// fail before any initializer writes: reconstructing totals from inferred
// estimates or silently deleting history would invalidate the human register.
func (s *Service) validateWorklogInitialSchema(ctx context.Context) error {
	tables := []struct{ name, columns, primary string }{
		{"suma_worklogs", "tenant_id,project_id,id,version,status,task_id,member_id,user_id,work_date,minutes,created_at,updated_at", "tenant_id,project_id,id"},
		{"suma_worklog_revisions", "tenant_id,project_id,worklog_id,revision,snapshot", "tenant_id,project_id,worklog_id,revision"},
		{"suma_worklog_daily_totals", "tenant_id,user_id,work_date,active_minutes", "tenant_id,user_id,work_date"},
		{"suma_worklog_scopes", "tenant_id,project_id,version", "tenant_id,project_id"},
	}
	var existence string
	switch s.Dialect {
	case "sqlite":
		existence = `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	case "postgres":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("unsupported initial schema dialect %q", s.Dialect)
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
		return fmt.Errorf("%w: complete current worklog domain is required", store.ErrInitialSchema)
	}
	for _, table := range tables {
		// Names below are compile-time owned identifiers, never user input.
		rows, err := s.DB.QueryContext(ctx, `SELECT `+table.columns+` FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: current %s columns are required", store.ErrInitialSchema, table.name)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		var keys []string
		if s.Dialect == "sqlite" {
			rows, err = s.DB.QueryContext(ctx, `PRAGMA table_info(`+table.name+`)`)
			if err != nil {
				return err
			}
			// PRAGMA yields table declaration order; PK positions encode the
			// actual composite key, independently of column declaration order.
			positions := map[int]string{}
			for rows.Next() {
				var cid, required, pk int
				var name, kind string
				var defaultValue any
				if err = rows.Scan(&cid, &name, &kind, &required, &defaultValue, &pk); err != nil {
					rows.Close()
					return err
				}
				if pk > 0 {
					positions[pk] = name
				}
			}
			for position := 1; position <= len(positions); position++ {
				keys = append(keys, positions[position])
			}
		} else {
			query := `SELECT k.column_name FROM information_schema.table_constraints c JOIN information_schema.key_column_usage k ON k.constraint_catalog=c.constraint_catalog AND k.constraint_schema=c.constraint_schema AND k.constraint_name=c.constraint_name AND k.table_name=c.table_name WHERE c.table_schema=current_schema() AND c.table_name=? AND c.constraint_type='PRIMARY KEY' ORDER BY k.ordinal_position`
			if s.Dialect == "oceanbase" {
				query = `SELECT column_name FROM information_schema.key_column_usage WHERE table_schema=DATABASE() AND table_name=? AND constraint_name='PRIMARY' ORDER BY ordinal_position`
			}
			rows, err = s.DB.QueryContext(ctx, s.q(query), table.name)
			if err != nil {
				return err
			}
			for rows.Next() {
				var name string
				if err = rows.Scan(&name); err != nil {
					rows.Close()
					return err
				}
				keys = append(keys, name)
			}
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return err
		}
		if err = rows.Close(); err != nil {
			return err
		}
		if strings.Join(keys, ",") != table.primary {
			return fmt.Errorf("%w: current %s primary key is required", store.ErrInitialSchema, table.name)
		}
	}
	return nil
}
