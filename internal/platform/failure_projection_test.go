package platform

import (
	"context"
	"github.com/ai-mada/suma/internal/dag"
	"testing"
)

func TestFailureProjectionFindsNonRunnerFailuresAndReplaysOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{ID: "approval", Name: "Approval failure", Nodes: []dag.Node{{ID: "gate", Type: "approval"}}}); err != nil {
		t.Fatal(err)
	}
	r, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, "approval", "approval-create")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Command(ctx, f.user.TenantID, f.project, r.ID, "approve", "reject", r.Version, map[string]any{"node_id": "gate", "decision": "rejected"}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = f.s.DB.QueryRow("SELECT COUNT(*) FROM suma_issues").Scan(&count); err != nil || count != 1 {
		t.Fatalf("failure issue %d %v", count, err)
	}
}
func TestFailureProjectionRecoversAfterIssueCommitBeforeLedger(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{ID: "workflow", Name: "Execution failure", Nodes: []dag.Node{{ID: "work", Type: "task", Idempotent: true}}}); err != nil {
		t.Fatal(err)
	}
	r, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, "workflow", "create")
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "worker", nil)
	if err != nil || a == nil {
		t.Fatal(err)
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: r.ID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 1, OperationID: "failed"}); err != nil {
		t.Fatal(err)
	}
	r, _ = f.e.GetRun(ctx, f.user.TenantID, f.project, r.ID)
	if err = f.s.FailureIssue(ctx, f.user.TenantID, f.project, r); err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Command(ctx, f.user.TenantID, f.project, r.ID, "retry", "retry", r.Version, map[string]any{"node_id": "work"}); err != nil {
		t.Fatal(err)
	}
	if err = f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	var issues, receipts int
	if err = f.s.DB.QueryRow("SELECT COUNT(*) FROM suma_issues").Scan(&issues); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow("SELECT COUNT(*) FROM suma_failure_projection").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if issues != 1 || receipts != 1 {
		t.Fatalf("crash recovery duplicates or lost event issues=%d receipts=%d", issues, receipts)
	}
}

func TestFailureProjectionIncludesTickTriggeredSubworkflowDeadline(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for _, w := range []dag.Workflow{{ID: "child", Name: "Waiting child", Nodes: []dag.Node{{ID: "approval", Type: "approval"}}}, {ID: "parent", Name: "Timed workflow", Nodes: []dag.Node{{ID: "sub", Type: "subworkflow", TimeoutSeconds: 10, Config: map[string]any{"workflow_id": "child", "workflow_version": 1}}}}} {
		if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, w); err != nil {
			t.Fatal(err)
		}
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, "parent", "tick-failure")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec("UPDATE dag_nodes SET wake_at=1 WHERE run_id=? AND id='sub'", run.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	run, _ = f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
	if run.State != "failed" {
		t.Fatal(run.State)
	}
	restarted := &Service{DB: f.s.DB, Dialect: "sqlite", DataDir: f.s.DataDir}
	if err = restarted.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = f.s.DB.QueryRow("SELECT COUNT(*) FROM suma_issues").Scan(&count); err != nil || count != 1 {
		t.Fatalf("tick failure lost %d %v", count, err)
	}
}
