package store

import (
	"context"
	"errors"
	"testing"
)

func TestInitialSchemaDeclarationAndRestart(t *testing.T) {
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err = InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE suma_organizations(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO suma_organizations(id) VALUES('retained')`); err != nil {
		t.Fatal(err)
	}
	if err = InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err = InitializeBaseline(ctx, db, "sqlite", "dag", "dag_", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM suma_schema_baselines`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("declarations=%d: %v", count, err)
	}
	if err = InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", "2.0.0"); !errors.Is(err, ErrInitialSchema) {
		t.Fatalf("unsupported declaration accepted: %v", err)
	}
	var retained string
	if err = db.QueryRow(`SELECT id FROM suma_organizations`).Scan(&retained); err != nil || retained != "retained" {
		t.Fatalf("existing data changed: %q %v", retained, err)
	}
}

func TestInitialSchemaRejectsUnregisteredDataWithoutWriting(t *testing.T) {
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE suma_organizations(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if err = InitializeBaseline(context.Background(), db, "sqlite", "platform", "suma_", "1.0.0"); !errors.Is(err, ErrInitialSchema) {
		t.Fatalf("unregistered database accepted: %v", err)
	}
	present, err := baselineTableExists(context.Background(), db, "sqlite", "suma_schema_baselines")
	if err != nil || present {
		t.Fatalf("rejected database was modified: %v", err)
	}
}

func TestInitialSchemaRejectsPartialDomainNamespace(t *testing.T) {
	for _, tc := range []struct{ component, prefix, table string }{
		{"platform", "suma_", "suma_human_resources"},
		{"dag", "dag_", "dag_workflows"},
	} {
		t.Run(tc.component, func(t *testing.T) {
			db, err := Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err = db.Exec("CREATE TABLE " + tc.table + "(id TEXT PRIMARY KEY)"); err != nil {
				t.Fatal(err)
			}
			if err = InitializeBaseline(context.Background(), db, "sqlite", tc.component, tc.prefix, "1.0.0"); !errors.Is(err, ErrInitialSchema) {
				t.Fatalf("partial unregistered schema accepted: %v", err)
			}
			present, err := baselineTableExists(context.Background(), db, "sqlite", "suma_schema_baselines")
			if err != nil || present {
				t.Fatalf("rejected partial schema was modified: %v", err)
			}
		})
	}
}

func TestInitialSchemaConcurrentCurrentStartup(t *testing.T) {
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	start, failures := make(chan struct{}), make(chan error, 8)
	for n := 0; n < 8; n++ {
		go func() {
			<-start
			err := InitializeBaseline(context.Background(), db, "sqlite", "platform", "suma_", "1.0.0")
			if err == nil {
				_, err = db.Exec(`CREATE TABLE IF NOT EXISTS suma_human_resources(id TEXT PRIMARY KEY)`)
			}
			failures <- err
		}()
	}
	close(start)
	for n := 0; n < 8; n++ {
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err = db.QueryRow(`SELECT COUNT(*) FROM suma_schema_baselines WHERE component='platform' AND version='1.0.0'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("competing declarations: %d %v", count, err)
	}
}
