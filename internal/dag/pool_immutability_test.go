package dag

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
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

// Zero is the established default. Adding scheduling priority must not mutate
// a fixed workflow's serialized basis or break its already-committed receipt.
func TestPoolPriorityZeroPreservesPinnedWorkflowAndReceipt(t *testing.T) {
	const oldNode = `{"id":"work","name":"","type":"task","command":null,"depends_on":null,"join":"","quorum":0,"timeout_seconds":0,"max_retries":0,"idempotent":false}`
	var node Node
	if err := json.Unmarshal([]byte(oldNode), &node); err != nil {
		t.Fatal(err)
	}
	if encoded(node) != oldNode || fingerprint(node) != hashText(oldNode) {
		t.Fatal("new default priority changed fixed old node bytes/hash", encoded(node), fingerprint(node), hashText(oldNode))
	}
	node.Priority = 7
	if !strings.Contains(encoded(node), `"priority":7`) {
		t.Fatal("explicit priority was lost")
	}
	e := openEngine(t, filepath.Join(t.TempDir(), "fixed-receipt.db"))
	w, err := normalize(Workflow{ID: "legacy", Name: "Pinned workflow", Nodes: []Node{{ID: "work", Type: "task"}}})
	if err != nil {
		t.Fatal(err)
	}
	oldRequest := strings.ReplaceAll(encoded(w), `,"priority":0`, ``)
	w.Version = 1
	oldPublished := strings.ReplaceAll(encoded(w), `,"priority":0`, ``)
	if _, err = e.db.Exec(`INSERT INTO dag_workflows(tenant_id,project_id,id,version,name,spec)VALUES('tenant','project','legacy',1,?,?)`, w.Name, oldPublished); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Exec(`INSERT INTO dag_operations(tenant_id,project_id,scope,operation_id,fingerprint,result_id)VALUES('tenant','project','publish_workflow','prior-publish-operation',?,'legacy:1')`, hashText(oldRequest)); err != nil {
		t.Fatal(err)
	}
	w.Version = 0
	replayed, err := e.PublishWithOperation(ctx, "tenant", "project", w, "prior-publish-operation")
	if err != nil || replayed.Version != 1 || encoded(replayed) != oldPublished {
		t.Fatal("old fixed receipt failed or changed basis", replayed, err)
	}
	var versions int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_workflows WHERE id='legacy'`).Scan(&versions)
	if versions != 1 {
		t.Fatal("receipt replay created another workflow", versions)
	}
}
