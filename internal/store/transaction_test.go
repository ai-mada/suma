package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func TestScopedTxFailedDeferredCommitReturnsCleanConnection(t *testing.T) {
	db, err := Open("sqlite", filepath.Join(t.TempDir(), "failed-commit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, statement := range []string{
		`CREATE TABLE scope_parent(id INTEGER PRIMARY KEY)`,
		`CREATE TABLE scope_child(id INTEGER PRIMARY KEY,parent_id INTEGER NOT NULL REFERENCES scope_parent(id) DEFERRABLE INITIALLY DEFERRED)`,
		`CREATE TEMP TABLE scope_connection_marker(id INTEGER PRIMARY KEY)`,
		`INSERT INTO scope_connection_marker(id) VALUES(7)`,
		`PRAGMA busy_timeout=321`,
	} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	scope, err := BeginScopedTx(context.Background(), db, "sqlite", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer scope.Close()
	if _, err = scope.Tx.Exec(`PRAGMA busy_timeout=17`); err != nil {
		t.Fatal(err)
	}
	if _, err = scope.Tx.Exec(`INSERT INTO scope_child(id,parent_id) VALUES(1,999)`); err != nil {
		t.Fatalf("deferred constraint failed before commit: %v", err)
	}
	if err = scope.Tx.Commit(); err == nil {
		t.Fatal("deferred foreign-key violation unexpectedly committed")
	}
	if err = scope.Tx.Rollback(); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("failed commit did not finish the database/sql transaction: %v", err)
	}
	if err = scope.Close(); err != nil {
		t.Fatal(err)
	}
	if err = scope.Close(); err != nil {
		t.Fatal(err)
	}
	probe, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	next, err := BeginScopedTx(probe, db, "sqlite", nil)
	if err != nil {
		t.Fatalf("failed commit left an active transaction in the pool: %v", err)
	}
	defer next.Close()
	var children, timeout, marker int
	if err = next.Tx.QueryRowContext(probe, `SELECT COUNT(*) FROM scope_child`).Scan(&children); err != nil || children != 0 {
		t.Fatalf("failed commit leaked uncommitted rows: count=%d err=%v", children, err)
	}
	if err = next.Tx.QueryRowContext(probe, `PRAGMA busy_timeout`).Scan(&timeout); err != nil || timeout != 321 {
		t.Fatalf("failed commit leaked connection settings: timeout=%d err=%v", timeout, err)
	}
	if err = next.Tx.QueryRowContext(probe, `SELECT id FROM scope_connection_marker`).Scan(&marker); err != nil || marker != 7 {
		t.Fatalf("cleanup discarded a recoverable retained connection: marker=%d err=%v", marker, err)
	}
	if _, err = next.Tx.ExecContext(probe, `INSERT INTO scope_parent(id) VALUES(1)`); err != nil {
		t.Fatal(err)
	}
	if _, err = next.Tx.ExecContext(probe, `INSERT INTO scope_child(id,parent_id) VALUES(2,1)`); err != nil {
		t.Fatal(err)
	}
	if err = next.Tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = next.Close(); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRowContext(probe, `SELECT COUNT(*) FROM scope_child`).Scan(&children); err != nil || children != 1 {
		t.Fatalf("next transaction did not commit independently: count=%d err=%v", children, err)
	}
}

func TestScopedTxRestoresConnectionAndReturnsItToPool(t *testing.T) {
	for _, outcome := range []string{"commit", "rollback", "cancel"} {
		t.Run(outcome, func(t *testing.T) {
			db, err := Open("sqlite", filepath.Join(t.TempDir(), "scope.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var before int
			if err = db.QueryRow(`PRAGMA busy_timeout`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			scope, err := BeginScopedTx(ctx, db, "sqlite", nil)
			if err != nil {
				t.Fatal(err)
			}
			defer scope.Close()
			if _, err = scope.Tx.Exec(`PRAGMA busy_timeout=17`); err != nil {
				t.Fatal(err)
			}
			switch outcome {
			case "commit":
				err = scope.Tx.Commit()
			case "rollback":
				err = scope.Tx.Rollback()
			case "cancel":
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = scope.Close(); err != nil {
				t.Fatal(err)
			}
			if err = scope.Close(); err != nil {
				t.Fatal(err)
			}
			probe, stop := context.WithTimeout(context.Background(), time.Second)
			defer stop()
			var after int
			if err = db.QueryRowContext(probe, `PRAGMA busy_timeout`).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("connection setting leaked: before=%d after=%d", before, after)
			}
		})
	}
}

func TestScopedTxCancelledBeginDoesNotConsumeConnection(t *testing.T) {
	db, err := Open("sqlite", filepath.Join(t.TempDir(), "begin.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := BeginScopedTx(ctx, db, "sqlite", &sql.TxOptions{}); err == nil {
		t.Fatal("cancelled begin succeeded")
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Reproduce the cancellation window with a real SQLite statement, not a fake
// successful query. An interrupted QueryContext must close any driver.Rows it
// created before returning an error: a leaked statement can keep close_v2's
// zombie connection alive even when database/sql reports zero open handles.
func TestSQLiteCancelledReadReleasesRowsAndDatabase(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	var validator driver.Validator
	functionName := fmt.Sprintf("suma_cancelled_read_%x", time.Now().UnixNano())
	if err := sqlite.RegisterScalarFunction(functionName, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		close(entered)
		<-ctx.Done()
		// The driver exposes this public health predicate and protects it with
		// its connection mutex. Wait for the actual native interrupt rather
		// than sleep and hope the cancellation goroutine has been scheduled.
		deadline := time.Now().Add(time.Second)
		for validator.IsValid() {
			if time.Now().After(deadline) {
				return nil, errors.New("native SQLite interrupt was not observed")
			}
			runtime.Gosched()
		}
		return int64(42), nil
	}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cancelled-read.db")
	db, err := Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE cancellation_probe(id INTEGER PRIMARY KEY); INSERT INTO cancellation_probe(id) VALUES(1)`); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	queryDone := make(chan error, 1)
	go func() {
		queryErr := conn.Raw(func(value any) error {
			var ok bool
			validator, ok = value.(driver.Validator)
			if !ok {
				return errors.New("SQLite driver does not expose connection validation")
			}
			// All access to the physical driver stays within Raw. The SQLite
			// scalar is invoked synchronously inside this QueryContext call.
			defer func() { validator = nil }()
			queryer, ok := value.(driver.QueryerContext)
			if !ok {
				return errors.New("SQLite driver does not expose contextual queries")
			}
			rows, err := queryer.QueryContext(ctx, `SELECT `+functionName+`(),id FROM cancellation_probe`, nil)
			if rows != nil {
				rows.Close()
			}
			return err
		})
		queryDone <- queryErr
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("real SQLite callback was not entered")
	}
	cancel()
	select {
	case err = <-queryDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("interrupted query: %v, want context cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interrupted SQLite query did not return")
	}
	probe, stop := context.WithTimeout(context.Background(), 3*time.Second)
	defer stop()
	var busy, logged, checkpointed int
	if err = conn.QueryRowContext(probe, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logged, &checkpointed); err != nil || busy != 0 {
		t.Errorf("cancelled query retained its native statement: busy=%d err=%v", busy, err)
	}
	if err = conn.Close(); err != nil {
		t.Error(err)
	}
	if err = db.Close(); err != nil {
		t.Error(err)
	}
	// Use another physical connection and a write/checkpoint, not just Ping:
	// an old WAL reader can survive the first pool's apparent clean shutdown.
	next, err := Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if _, err = next.ExecContext(probe, `INSERT INTO cancellation_probe(id) VALUES(2)`); err != nil {
		t.Errorf("next connection could not write after cancellation: %v", err)
	}
	if err = next.QueryRowContext(probe, `PRAGMA wal_checkpoint(TRUNCATE)`).Scan(&busy, &logged, &checkpointed); err != nil || busy != 0 {
		t.Errorf("next connection could not checkpoint after cancellation: busy=%d err=%v", busy, err)
	}
	if err = next.Close(); err != nil {
		t.Error(err)
	}
	if err = os.Rename(path, path+".released"); err != nil {
		t.Errorf("closed SQLite pools retained the database file: %v", err)
	}
}
