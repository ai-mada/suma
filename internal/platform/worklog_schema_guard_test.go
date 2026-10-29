package platform

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestWorklogInitialDomainRejectsMalformedShapeBeforeWrites(t *testing.T) {
	for _, shape := range []string{"partial", "columns", "primary_key", "view"} {
		t.Run(shape, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "worklogs.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = store.InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", store.InitialDataContract); err != nil {
				t.Fatal(err)
			}
			s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
			if shape == "view" {
				_, err = db.Exec(`CREATE VIEW suma_worklogs AS SELECT 'existing-data' AS tenant_id`)
			} else if shape == "partial" {
				_, err = db.Exec(`CREATE TABLE suma_worklogs(tenant_id TEXT)`)
			} else {
				err = s.InitializeWorklogs(ctx)
				if err == nil {
					_, err = db.Exec(`DROP TABLE suma_worklog_daily_totals`)
				}
				if err == nil && shape == "columns" {
					_, err = db.Exec(`CREATE TABLE suma_worklog_daily_totals(tenant_id TEXT,user_id TEXT,work_date TEXT,PRIMARY KEY(tenant_id,user_id,work_date))`)
				}
				if err == nil && shape == "primary_key" {
					_, err = db.Exec(`CREATE TABLE suma_worklog_daily_totals(tenant_id TEXT,user_id TEXT,work_date TEXT,active_minutes INTEGER)`)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			var before, after int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if err = s.Initialize(ctx); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("incompatible human register accepted", err)
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&after); err != nil || before != after {
				t.Fatal("rejection wrote unrelated schema", before, after, err)
			}
		})
	}
}

func TestWorklogInitialDomainInstallsWithoutInferredHistory(t *testing.T) {
	f := newFixture(t)
	if err := f.s.validateWorklogInitialSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"suma_worklogs", "suma_worklog_revisions", "suma_worklog_daily_totals", "suma_worklog_scopes"} {
		var count int
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("new register must start empty", table, count, err)
		}
	}
}
