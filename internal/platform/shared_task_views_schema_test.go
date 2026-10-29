package platform

import (
	"context"
	"errors"
	"github.com/ai-mada/suma/internal/store"
	"path/filepath"
	"testing"
)

func TestSharedTaskViewsInitialSchemaRejectsMalformedBeforeWrites(t *testing.T) {
	for _, ddl := range []string{`CREATE TABLE suma_shared_task_views(tenant_id TEXT)`, `CREATE VIEW suma_shared_task_views AS SELECT 'existing' AS tenant_id`, `CREATE TABLE suma_shared_task_views(tenant_id TEXT,project_id TEXT,id TEXT,owner_id TEXT,visibility TEXT,version INTEGER,state TEXT,snapshot TEXT)`} {
		t.Run(ddl[:12], func(t *testing.T) {
			ctx := context.Background()
			db, e := store.Open("sqlite", filepath.Join(t.TempDir(), "views.sqlite"))
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			if e = store.InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", store.InitialDataContract); e != nil {
				t.Fatal(e)
			}
			if _, e = db.Exec(ddl); e != nil {
				t.Fatal(e)
			}
			var before, after int
			if e = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&before); e != nil {
				t.Fatal(e)
			}
			s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
			if e = s.Initialize(ctx); !errors.Is(e, store.ErrInitialSchema) {
				t.Fatal("malformed preference schema accepted", e)
			}
			if e = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&after); e != nil || after != before {
				t.Fatal("rejection modified schema", before, after, e)
			}
		})
	}
}
