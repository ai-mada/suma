package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

// This is an independent current initial domain. Inspect before any initializer
// writes; partial tables, views or missing uniqueness are never repaired here.
func (s *Service) validateNotificationInboxInitialSchema(ctx context.Context) error {
	tables := []struct {
		name, columns, primary string
		unique                 []string
	}{
		{"suma_notification_inbox", "id,tenant_id,project_id,recipient_user_id,sequence,kind,task_id,source_task_version,source_resource_id,source_event_key,version,created_at,read_at,archived_at,updated_at", "id", []string{"tenant_id,project_id,source_event_key,recipient_user_id", "tenant_id,project_id,sequence"}},
		{"suma_notification_inbox_sequences", "tenant_id,project_id,last_sequence", "tenant_id,project_id", nil},
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
		return fmt.Errorf("%w: unsupported notification schema dialect", store.ErrInitialSchema)
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
		return fmt.Errorf("%w: complete notification inbox domain is required", store.ErrInitialSchema)
	}
	for _, table := range tables {
		rows, err := s.DB.QueryContext(ctx, `SELECT `+table.columns+` FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: current %s columns are required", store.ErrInitialSchema, table.name)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		primary, unique, err := s.notificationInboxSchemaKeys(ctx, table.name)
		if err != nil {
			return err
		}
		if primary != table.primary {
			return fmt.Errorf("%w: current %s primary key is required", store.ErrInitialSchema, table.name)
		}
		for _, required := range table.unique {
			if !unique[required] {
				return fmt.Errorf("%w: current %s uniqueness is required", store.ErrInitialSchema, table.name)
			}
		}
	}
	return nil
}

func (s *Service) notificationInboxSchemaKeys(ctx context.Context, table string) (string, map[string]bool, error) {
	unique := map[string]bool{}
	primary := ""
	if s.Dialect == "sqlite" {
		type index struct{ name, origin string }
		indexes := []index{}
		rows, err := s.DB.QueryContext(ctx, `PRAGMA index_list(`+table+`)`)
		if err != nil {
			return "", unique, err
		}
		for rows.Next() {
			var sequence, isUnique, partial int
			var name, origin string
			if err = rows.Scan(&sequence, &name, &isUnique, &origin, &partial); err != nil {
				rows.Close()
				return "", unique, err
			}
			if isUnique == 1 && partial == 0 {
				indexes = append(indexes, index{name, origin})
			}
		}
		if err = rows.Err(); err != nil {
			rows.Close()
			return "", unique, err
		}
		if err = rows.Close(); err != nil {
			return "", unique, err
		}
		for _, item := range indexes {
			rows, err = s.DB.QueryContext(ctx, `PRAGMA index_info('`+strings.ReplaceAll(item.name, "'", "''")+`')`)
			if err != nil {
				return "", unique, err
			}
			columns := []string{}
			for rows.Next() {
				var order, columnID int
				var name string
				if err = rows.Scan(&order, &columnID, &name); err != nil {
					rows.Close()
					return "", unique, err
				}
				columns = append(columns, name)
			}
			if err = rows.Err(); err != nil {
				rows.Close()
				return "", unique, err
			}
			if err = rows.Close(); err != nil {
				return "", unique, err
			}
			key := strings.Join(columns, ",")
			unique[key] = true
			if item.origin == "pk" {
				primary = key
			}
		}
		return primary, unique, nil
	}
	query := `SELECT c.constraint_name,c.constraint_type,k.column_name FROM information_schema.table_constraints c JOIN information_schema.key_column_usage k ON k.constraint_catalog=c.constraint_catalog AND k.constraint_schema=c.constraint_schema AND k.constraint_name=c.constraint_name AND k.table_name=c.table_name WHERE c.table_schema=current_schema() AND c.table_name=? AND c.constraint_type IN ('PRIMARY KEY','UNIQUE') ORDER BY c.constraint_name,k.ordinal_position`
	if s.Dialect == "oceanbase" {
		query = `SELECT index_name,CASE WHEN index_name='PRIMARY' THEN 'PRIMARY KEY' ELSE 'UNIQUE' END,column_name FROM information_schema.statistics WHERE table_schema=DATABASE() AND table_name=? AND non_unique=0 ORDER BY index_name,seq_in_index`
	}
	rows, err := s.DB.QueryContext(ctx, s.q(query), table)
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
