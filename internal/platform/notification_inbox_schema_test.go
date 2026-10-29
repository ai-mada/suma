package platform

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestNotificationInboxInitialSchemaRejectsPartialAndMissingConstraintsBeforeWrites(t *testing.T) {
	for _, shape := range []string{"partial", "columns", "primary", "unique", "view"} {
		t.Run(shape, func(t *testing.T) {
			ctx := context.Background()
			db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "inbox.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err = store.InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", store.InitialDataContract); err != nil {
				t.Fatal(err)
			}
			s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
			switch shape {
			case "partial":
				_, err = db.Exec(`CREATE TABLE suma_notification_inbox(id TEXT)`)
			case "view":
				_, err = db.Exec(`CREATE VIEW suma_notification_inbox AS SELECT 'unique source' AS id`)
			default:
				err = s.InitializeNotificationInbox(ctx)
				if err == nil && shape == "unique" {
					_, err = db.Exec(`DROP TABLE suma_notification_inbox`)
					if err == nil {
						ddl := strings.ReplaceAll(notificationInboxRowsDDL, ",UNIQUE(tenant_id,project_id,source_event_key,recipient_user_id)", "")
						ddl = strings.ReplaceAll(ddl, ",UNIQUE(tenant_id,project_id,sequence)", "")
						_, err = db.Exec(ddl)
					}
				}
				if err == nil && shape != "unique" {
					_, err = db.Exec(`DROP TABLE suma_notification_inbox_sequences`)
					if err == nil {
						ddl := `CREATE TABLE suma_notification_inbox_sequences(tenant_id TEXT,project_id TEXT,last_sequence INTEGER)`
						if shape == "columns" {
							ddl = `CREATE TABLE suma_notification_inbox_sequences(tenant_id TEXT,project_id TEXT,PRIMARY KEY(tenant_id,project_id))`
						}
						_, err = db.Exec(ddl)
					}
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
				t.Fatal("incompatible inbox domain accepted", err)
			}
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&after); err != nil || before != after {
				t.Fatal("rejection wrote unrelated schema", before, after, err)
			}
		})
	}
}

func TestNotificationInboxInitialDomainStartsEmptyAndReopensWithoutSourceInference(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.s.validateNotificationInboxInitialSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeNotificationInbox(ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"suma_notification_inbox", "suma_notification_inbox_sequences"} {
		var count int
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("initializer inferred event history", table, count, err)
		}
	}
}
