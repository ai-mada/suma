package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
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
