package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInitialSchemaRejectsOtherUnregisteredNamespaceBeforeAnyWrite(t *testing.T) {
	for _, scenario := range []struct {
		name, component, prefix, otherTable string
		currentDeclaration                  bool
	}{
		{"platform-before-unregistered-dag", "platform", "suma_", "dag_workflows", false},
		{"current-platform-before-unregistered-dag", "platform", "suma_", "dag_workflows", true},
		{"dag-before-unregistered-platform", "dag", "dag_", "suma_contract_sentinel", false},
		{"current-dag-before-unregistered-platform", "dag", "dag_", "suma_contract_sentinel", true},
		{"platform-before-uppercase-dag", "platform", "suma_", "DAG_LOCK", false},
		{"dag-before-uppercase-platform", "dag", "dag_", "SUMA_WORKSPACE_BINDINGS", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "unregistered-other.db")
			db, err := Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if scenario.currentDeclaration {
				if _, err = db.Exec(`CREATE TABLE suma_schema_baselines(component VARCHAR(80) PRIMARY KEY,version VARCHAR(30) NOT NULL)`); err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(`INSERT INTO suma_schema_baselines(component,version) VALUES(?,?)`, scenario.component, InitialDataContract); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Exec("CREATE TABLE " + scenario.otherTable + "(id TEXT PRIMARY KEY,payload TEXT NOT NULL)"); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec("INSERT INTO " + scenario.otherTable + "(id,payload) VALUES('original','preserve unique user data')"); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			db, err = Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			err = InitializeBaseline(context.Background(), db, "sqlite", scenario.component, scenario.prefix, InitialDataContract)
			closeErr := db.Close()
			after, readErr := os.ReadFile(path)
			if !errors.Is(err, ErrInitialSchema) || closeErr != nil || readErr != nil || sha256.Sum256(before) != sha256.Sum256(after) {
				t.Fatalf("other namespace must reject before any write: initialization=%v close=%v read=%v database_changed=%v", err, closeErr, readErr, sha256.Sum256(before) != sha256.Sum256(after))
			}
		})
	}
	t.Run("uppercase-baseline-invalid-declaration", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "uppercase-baseline.db")
		db, err := Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`CREATE TABLE SUMA_SCHEMA_BASELINES(component VARCHAR(80) PRIMARY KEY,version VARCHAR(30) NOT NULL)`); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO SUMA_SCHEMA_BASELINES(component,version) VALUES('unknown-component','unsupported-contract')`); err != nil {
			t.Fatal(err)
		}
		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		db, err = Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		err = InitializeBaseline(context.Background(), db, "sqlite", "platform", "suma_", InitialDataContract)
		closeErr := db.Close()
		after, readErr := os.ReadFile(path)
		if !errors.Is(err, ErrInitialSchema) || closeErr != nil || readErr != nil || sha256.Sum256(before) != sha256.Sum256(after) {
			t.Fatalf("invalid uppercase baseline must reject before any write: initialization=%v close=%v read=%v database_changed=%v", err, closeErr, readErr, sha256.Sum256(before) != sha256.Sum256(after))
		}
	})
}

func TestInitialSchemaDeclarationAndRestart(t *testing.T) {
	db, err := Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err = InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", InitialDataContract); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE suma_organizations(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO suma_organizations(id) VALUES('retained')`); err != nil {
		t.Fatal(err)
	}
	if err = InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", InitialDataContract); err != nil {
		t.Fatal(err)
	}
	if err = InitializeBaseline(ctx, db, "sqlite", "dag", "dag_", InitialDataContract); err != nil {
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
	if err = InitializeBaseline(context.Background(), db, "sqlite", "platform", "suma_", InitialDataContract); !errors.Is(err, ErrInitialSchema) {
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
			if err = InitializeBaseline(context.Background(), db, "sqlite", tc.component, tc.prefix, InitialDataContract); !errors.Is(err, ErrInitialSchema) {
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
			err := InitializeBaseline(context.Background(), db, "sqlite", "platform", "suma_", InitialDataContract)
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
	if err = db.QueryRow(`SELECT COUNT(*) FROM suma_schema_baselines WHERE component='platform' AND version=?`, InitialDataContract).Scan(&count); err != nil || count != 1 {
		t.Fatalf("competing declarations: %d %v", count, err)
	}
}

func TestInitialDataContractFailsBeforeWritesAndCurrentDatabaseReopens(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []struct {
		name         string
		declarations [][2]string
	}{
		{"prior-declaration", [][2]string{{"platform", "1.0.0"}}},
		{"unprotected-issue-semantics", [][2]string{{"platform", "suma-initial-gateway-v1"}, {"dag", "suma-initial-gateway-v1"}}},
		{"mixed-components", [][2]string{{"platform", InitialDataContract}, {"dag", "1.0.0"}}},
		{"unknown-component", [][2]string{{"platform", InitialDataContract}, {"unknown", InitialDataContract}}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			db, err := Open("sqlite", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err = db.Exec(`CREATE TABLE suma_schema_baselines(component VARCHAR(80) PRIMARY KEY,version VARCHAR(30) NOT NULL)`); err != nil {
				t.Fatal(err)
			}
			for _, declaration := range scenario.declarations {
				if _, err = db.Exec(`INSERT INTO suma_schema_baselines(component,version) VALUES(?,?)`, declaration[0], declaration[1]); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = db.Exec(`CREATE TABLE suma_contract_sentinel(id TEXT PRIMARY KEY,payload TEXT NOT NULL)`); err != nil {
				t.Fatal(err)
			}
			const unique = `{"unique":"keep-original","octop_execution":{"provider_id":"prior-provider"}}`
			if _, err = db.Exec(`INSERT INTO suma_contract_sentinel(id,payload) VALUES('unique',?)`, unique); err != nil {
				t.Fatal(err)
			}
			var ddlBefore, changesBefore int
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&ddlBefore); err != nil {
				t.Fatal(err)
			}
			if err = db.QueryRow(`SELECT total_changes()`).Scan(&changesBefore); err != nil {
				t.Fatal(err)
			}
			for _, component := range []struct{ name, prefix string }{{"platform", "suma_"}, {"dag", "dag_"}} {
				if err = InitializeBaseline(ctx, db, "sqlite", component.name, component.prefix, InitialDataContract); !errors.Is(err, ErrInitialSchema) {
					t.Fatal("noncurrent persistent contract passed initialization", err)
				}
			}
			var ddlAfter, changesAfter int
			var retained string
			if err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&ddlAfter); err != nil {
				t.Fatal(err)
			}
			if err = db.QueryRow(`SELECT total_changes()`).Scan(&changesAfter); err != nil {
				t.Fatal(err)
			}
			if err = db.QueryRow(`SELECT payload FROM suma_contract_sentinel WHERE id='unique'`).Scan(&retained); err != nil {
				t.Fatal(err)
			}
			if ddlBefore != ddlAfter || changesBefore != changesAfter || retained != unique {
				t.Fatal("rejected initialization changed declarations, DDL or unique data")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "current.db")
	db, err := Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if err = InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", InitialDataContract); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = InitializeBaseline(ctx, db, "sqlite", "dag", "dag_", InitialDataContract); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = InitializeBaseline(ctx, db, "sqlite", "platform", "suma_", InitialDataContract); err != nil {
		t.Fatal("current initial contract could not reopen", err)
	}
	if err = InitializeBaseline(ctx, db, "sqlite", "dag", "dag_", InitialDataContract); err != nil {
		t.Fatal("current DAG contract could not reopen", err)
	}
}
