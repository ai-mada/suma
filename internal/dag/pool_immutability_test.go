package dag

import (
	"database/sql"
	"errors"
	"testing"
)

func TestPoolPublicProjectRouteReaderIsScopedAndReadOnly(t *testing.T) {
	e, scopes := poolFixture(t)
	s := scopes[0]
	var before int
	if err := e.db.QueryRow(`SELECT SUM(version) FROM dag_execution_pool_guards`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	route, err := ReadProjectExecutionRouteTx(ctx, tx, "sqlite", s.TenantID, s.ProjectID)
	if err != nil || route.PoolID != "pool" || route.GrantID != s.GrantID || route.Version != 1 {
		t.Fatal(route, err)
	}
	other, err := ReadProjectExecutionRouteTx(ctx, tx, "sqlite", "other", s.ProjectID)
	if err != nil || other.PoolID != "" || other.Version != 0 {
		t.Fatal("route leaked outside project scope", other, err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var after int
	if err = e.db.QueryRow(`SELECT SUM(version) FROM dag_execution_pool_guards`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("display reader changed scheduler locks", before, after)
	}
}

func TestRunWithoutFrozenRouteCannotBecomePrivateWork(t *testing.T) {
	e, run := fixture(t, Workflow{Nodes: []Node{{ID: "work", Type: "task"}}})
	if _, err := e.db.Exec(`DELETE FROM dag_run_execution_routes WHERE run_id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.GetRun(ctx, "t", "p", run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("run without a frozen route was inferred as private", err)
	}
	if assignment, err := e.Claim(ctx, "t", "p", "worker", []string{"task"}); err == nil || assignment != nil {
		t.Fatal("incomplete run became executable", assignment, err)
	}
}
