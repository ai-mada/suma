package dag

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/store"
)

func migrateEventIndexes(db *sql.DB, dialect string) error {
	return store.EnsureIndex(context.Background(), db, dialect, "dag_events_run_sequence", "dag_events", "tenant_id", "project_id", "run_id", "sequence")
}
