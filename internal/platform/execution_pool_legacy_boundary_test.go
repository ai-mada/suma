package platform

import (
	"context"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestProjectRunnerCannotWritePoolAttemptByStrippingScope(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pool, err := f.s.CreateExecutionPool(ctx, f.e, f.user, f.cookie.Value, ExecutionPoolCreateRequest{Name: "隔离池", Policy: ExecutionPoolPolicyInput{ConcurrencyLimit: 2, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{}}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	grant := newID()
	if err = f.e.WritePoolScopeTx(ctx, tx, dag.PoolScope{PoolID: pool.ID, TenantID: f.user.TenantID, ProjectID: f.project, GrantID: grant, GrantVersion: 2, ProjectWeight: 1, Status: "active"}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if _, err = f.e.WriteProjectExecutionRouteTx(ctx, tx, f.user.TenantID, f.project, 0, dag.ProjectExecutionRouteInput{PoolID: pool.ID, GrantID: grant, GrantVersion: 2}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "已固定池路由", "nodes": []dag.Node{{ID: "work", Type: "command", Command: []string{"inspect"}}}}), 201)
	r := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	// The original project credential is valid, but none of these bodies can
	// turn it into the machine authority required by the persisted pool route.
	a := dag.Assignment{RunID: r["id"].(string), NodeID: "work", Attempt: 1, Fence: 1, WorkerID: "pool-" + newID()}
	f.bearer("POST", "/runner/heartbeat", token, f.body(map[string]any{"assignment": a}), 403)
	f.bearer("POST", "/runner/progress", token, f.body(map[string]any{"assignment": a, "operation_id": newID(), "message": "forged"}), 403)
	f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: 1, Fence: 1, WorkerID: a.WorkerID, OperationID: newID()}}), 403)
	var attempts int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=?`, a.RunID).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("unauthorized callback changed execution", err)
	}
}
