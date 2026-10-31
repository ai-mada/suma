package platform

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestWorkBuddyTaskDispatchInitialSchemaRejectsBeforeWrites(t *testing.T) {
	for _, kind := range []string{"unregistered", "columns", "identity"} {
		t.Run(kind, func(t *testing.T) {
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "task-dispatch.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if kind != "unregistered" {
				if err = store.InitializeBaseline(context.Background(), db, "sqlite", "platform", "suma_", store.InitialDataContract); err != nil {
					t.Fatal(err)
				}
			}
			ddl := workBuddyTaskDispatchDDL
			if kind == "columns" {
				ddl = `CREATE TABLE suma_workbuddy_task_dispatch_plans(id TEXT PRIMARY KEY,note TEXT)`
			}
			if kind == "identity" {
				ddl = `CREATE TABLE suma_workbuddy_task_dispatch_plans(id TEXT PRIMARY KEY,tenant_id TEXT,project_id TEXT,owner_id TEXT,run_id TEXT,version INTEGER,state TEXT,created_at TEXT,expires_at TEXT,payload TEXT)`
			}
			if _, err = db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			if kind == "columns" {
				if _, err = db.Exec(`INSERT INTO suma_workbuddy_task_dispatch_plans VALUES('unique','preserve')`); err != nil {
					t.Fatal(err)
				}
			}
			var before, after int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			s := &Service{DB: db, Dialect: "sqlite"}
			if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("partial source accepted", err)
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&after); err != nil || before != after {
				t.Fatal("rejection wrote schema", before, after, err)
			}
			if kind == "columns" {
				var note string
				if err = db.QueryRow(`SELECT note FROM suma_workbuddy_task_dispatch_plans WHERE id='unique'`).Scan(&note); err != nil || note != "preserve" {
					t.Fatal("unique data changed", err)
				}
			}
		})
	}
}
