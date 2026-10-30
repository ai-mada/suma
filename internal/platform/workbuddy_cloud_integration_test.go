package platform

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestWorkBuddyCloudSharedInitializationRejectsPartialBeforeWrites(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "partial-cloud.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE suma_workbuddy_cloud_delegations(id TEXT PRIMARY KEY,user_note TEXT); INSERT INTO suma_workbuddy_cloud_delegations VALUES('original','preserve')`); err != nil {
		t.Fatal(err)
	}
	var before string
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='suma_workbuddy_cloud_delegations'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: db, Dialect: "sqlite"}
	if err = s.Initialize(context.Background()); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("shared initialization accepted an unregistered partial domain", err)
	}
	var after, note string
	var newTables int
	if err = db.QueryRow(`SELECT sql FROM sqlite_master WHERE name='suma_workbuddy_cloud_delegations'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT user_note FROM suma_workbuddy_cloud_delegations WHERE id='original'`).Scan(&note); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name<>'suma_workbuddy_cloud_delegations'`).Scan(&newTables); err != nil {
		t.Fatal(err)
	}
	if before != after || note != "preserve" || newTables != 0 {
		t.Fatal("rejected initialization changed original data or created a schema")
	}
}
