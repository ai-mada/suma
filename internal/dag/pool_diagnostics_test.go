package dag

import (
	"database/sql"
	"testing"
)

func TestPoolAdmissionAndQueueExplainSharedZeroCapacityAndBudget(t *testing.T) {
	e, scopes := poolFixture(t)
	zero := int64(0)
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.WritePoolPolicyTx(ctx, tx, "pool", 1, PoolPolicyInput{ConcurrencyLimit: 0, BudgetLimit: &zero, TenantWeights: map[string]int{"ta": 1, "tb": 3}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page, err := e.QueuePage(ctx, scopes[0].TenantID, scopes[0].ProjectID, 50, "")
	if err != nil || len(page.Items) == 0 {
		t.Fatal(page, err)
	}
	snapshot := page.Items[0].Admission
	if snapshot == nil {
		t.Fatal("queue omitted admission facts")
	}
	codes := map[string]bool{}
	for _, b := range snapshot.Blockers {
		if b.Source == "pool" {
			codes[b.Code] = true
		}
	}
	if snapshot.Status != "blocked" || !codes["pool_resource_capacity"] || !codes["pool_budget_exhausted"] {
		t.Fatalf("zero shared limits missing from queue/diagnostics: %+v", snapshot)
	}
}

func TestPoolAdmissionExplainsInactiveAndChangedGrant(t *testing.T) {
	e, scopes := poolFixture(t)
	scope := scopes[0]
	scope.Status = "inactive"
	scope.GrantVersion++
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = e.WritePoolScopeTx(ctx, tx, scope); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var run string
	if err = e.db.QueryRow(`SELECT id FROM dag_runs WHERE tenant_id=? AND project_id=?`, scope.TenantID, scope.ProjectID).Scan(&run); err != nil {
		t.Fatal(err)
	}
	read, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer read.Rollback()
	snapshot, err := e.AdmissionSnapshotTx(ctx, read, scope.TenantID, scope.ProjectID, run, "n000")
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, b := range snapshot.Blockers {
		codes[b.Code] = true
	}
	if snapshot.Status != "blocked" || !codes["pool_scope_inactive"] || !codes["pool_scope_version_mismatch"] {
		t.Fatalf("current shared grant not diagnosed: %+v", snapshot)
	}
}
