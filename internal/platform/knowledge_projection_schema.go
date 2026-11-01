package platform

import (
	"context"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

const indexOutboxDDL = `CREATE TABLE IF NOT EXISTS suma_index_outbox(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,source_kind VARCHAR(40) NOT NULL,source_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,content_revision INTEGER NOT NULL,source_sha256 VARCHAR(64) NOT NULL,action VARCHAR(40) NOT NULL,payload TEXT NOT NULL,state VARCHAR(40) NOT NULL,created_at VARCHAR(40) NOT NULL,receipt TEXT NOT NULL,indexed_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,source_kind,source_id,revision))`
const indexFailuresDDL = `CREATE TABLE IF NOT EXISTS suma_index_failures(event_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,source_kind VARCHAR(40) NOT NULL,source_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,http_status INTEGER NOT NULL,reason VARCHAR(200) NOT NULL,failed_at VARCHAR(40) NOT NULL)`

// This is one initial 1.0.0 data contract, not a Record-payload upgrade path.
// Reject an unknown layout, including case aliases, before any service DDL.
func (s *Service) validateKnowledgeProjectionInitialSchema(ctx context.Context) error {
	tables := []struct {
		name, columns, primary string
		unique                 bool
	}{
		{"suma_index_outbox", "id,tenant_id,project_id,source_kind,source_id,revision,content_revision,source_sha256,action,payload,state,created_at,receipt,indexed_at", "id", true},
		{"suma_index_failures", "event_id,tenant_id,project_id,source_kind,source_id,revision,http_status,reason,failed_at", "event_id", false},
	}
	for _, table := range tables {
		query := `SELECT name FROM sqlite_master WHERE type IN ('table','view') AND lower(name)=?`
		switch s.Dialect {
		case "sqlite":
		case "postgres":
			query = `SELECT table_name FROM information_schema.tables WHERE table_schema=current_schema() AND lower(table_name)=?`
		case "oceanbase":
			query = `SELECT table_name FROM information_schema.tables WHERE table_schema=DATABASE() AND lower(table_name)=?`
		default:
			return fmt.Errorf("%w: index projection dialect", store.ErrInitialSchema)
		}
		names, err := s.DB.QueryContext(ctx, s.q(query), table.name)
		if err != nil {
			return err
		}
		found := []string{}
		for names.Next() {
			var name string
			if err = names.Scan(&name); err != nil {
				names.Close()
				return err
			}
			found = append(found, name)
		}
		err = names.Err()
		names.Close()
		if err != nil {
			return err
		}
		if len(found) == 0 {
			continue
		}
		if len(found) != 1 || found[0] != table.name {
			return fmt.Errorf("%w: exact index table identity required", store.ErrInitialSchema)
		}
		var baseline string
		if err = s.DB.QueryRowContext(ctx, `SELECT version FROM suma_schema_baselines WHERE component='platform'`).Scan(&baseline); err != nil || baseline != store.InitialDataContract {
			return fmt.Errorf("%w: registered index projection required", store.ErrInitialSchema)
		}
		rows, err := s.DB.QueryContext(ctx, `SELECT * FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: index projection columns", store.ErrInitialSchema)
		}
		columns, err := rows.Columns()
		rows.Close()
		if err != nil {
			return err
		}
		want := strings.Split(table.columns, ",")
		seen := map[string]bool{}
		for _, column := range columns {
			seen[column] = true
		}
		if len(columns) != len(want) {
			return fmt.Errorf("%w: exact index projection columns required", store.ErrInitialSchema)
		}
		for _, column := range want {
			if !seen[column] {
				return fmt.Errorf("%w: index projection column required", store.ErrInitialSchema)
			}
		}
		primary, unique, err := s.notificationInboxSchemaKeys(ctx, table.name)
		if err != nil {
			return err
		}
		if primary != table.primary || table.unique && !unique["tenant_id,project_id,source_kind,source_id,revision"] {
			return fmt.Errorf("%w: domain-aware index projection keys required", store.ErrInitialSchema)
		}
	}
	return nil
}
func (s *Service) initializeKnowledgeProjectionIndexes(ctx context.Context) error {
	if err := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_index_outbox_pending", "suma_index_outbox", "state", "created_at", "id"); err != nil {
		return err
	}
	for _, index := range []struct {
		name    string
		columns []string
	}{
		{"suma_index_outbox_domain", []string{"tenant_id", "project_id", "source_kind", "state", "created_at", "id"}},
		{"suma_index_outbox_latest", []string{"tenant_id", "project_id", "source_kind", "created_at", "id"}},
		{"suma_index_outbox_publication", []string{"tenant_id", "project_id", "source_kind", "source_id", "content_revision", "action"}},
	} {
		if err := store.EnsureIndex(ctx, s.DB, s.Dialect, index.name, "suma_index_outbox", index.columns...); err != nil {
			return err
		}
	}
	return nil
}
