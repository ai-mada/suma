package platform

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestResourceCalendarMalformedDeclaredShapeRejectsBeforeInitializationWrites(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "calendar.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", store.InitialDataContract); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE suma_resource_calendars(tenant_id TEXT,member_id TEXT)`); err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
	if err = s.Initialize(ctx); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("an incompatible declared calendar was silently accepted", err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table'`).Scan(&after); err != nil || before != after {
		t.Fatal("schema rejection wrote unrelated tables", before, after, err)
	}
}
