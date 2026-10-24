package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureIndexRestartsPreserveDefinitionAndUsesScopedEventSeek(t *testing.T) {
	db, err := Open("sqlite", filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE events(sequence INTEGER PRIMARY KEY,tenant TEXT,project TEXT,run TEXT,data TEXT)`); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		if err = EnsureIndex(ctx, db, "sqlite", "event_seek", "events", "tenant", "project", "run", "sequence"); err != nil {
			t.Fatal(err)
		}
	}
	var id, parent, unused int
	var plan string
	err = db.QueryRow(`EXPLAIN QUERY PLAN SELECT data FROM events WHERE tenant=? AND project=? AND run=? AND sequence>? ORDER BY sequence LIMIT 128`, "t", "p", "r", 4).Scan(&id, &parent, &unused, &plan)
	if err != nil || !strings.Contains(plan, "event_seek") || !strings.Contains(plan, "sequence>?") {
		t.Fatalf("event replay lacks indexed seek: %s %v", plan, err)
	}
	if err = EnsureIndex(ctx, db, "sqlite", "event_seek", "events", "tenant", "sequence"); err == nil {
		t.Fatal("silently accepted changed index definition")
	}
	for _, ddl := range []string{
		`CREATE INDEX partial_seek ON events(tenant,project,run,sequence) WHERE tenant='only-one-tenant'`,
		`CREATE INDEX reverse_seek ON events(tenant,project,run,sequence DESC)`,
	} {
		if _, err = db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"partial_seek", "reverse_seek"} {
		if err = EnsureIndex(ctx, db, "sqlite", name, "events", "tenant", "project", "run", "sequence"); err == nil {
			t.Fatalf("accepted partial/descending index as ordinary ascending: %s", name)
		}
	}
	for _, name := range []string{"events; DROP TABLE events", "UPPER", "bad-name", "1prefix"} {
		if err = EnsureIndex(ctx, db, "sqlite", name, "events", "sequence"); err == nil {
			t.Fatalf("accepted identifier %q", name)
		}
	}
}
