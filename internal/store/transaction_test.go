package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

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
