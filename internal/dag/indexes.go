package dag

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/store"
)

func initializeEventIndexes(db *sql.DB, dialect string) error {
	if err := store.EnsureIndex(context.Background(), db, dialect, "dag_events_run_sequence", "dag_events", "tenant_id", "project_id", "run_id", "sequence"); err != nil {
		return err
	}
	if err := store.EnsureIndex(context.Background(), db, dialect, "dag_events_node_type_sequence", "dag_events", "tenant_id", "project_id", "run_id", "node_id", "event_type", "sequence"); err != nil {
		return err
	}
	return store.EnsureIndex(context.Background(), db, dialect, "dag_events_type_sequence", "dag_events", "event_type", "sequence")
}
