package platform

import (
	"context"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestFailureProjectionDeferredEventsDoNotStarveLaterFailures(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	const blockedCount = 51
	const blockedWorkflow = "failure-retry-blocker"
	const healthyWorkflow = "failure-retry-healthy"
	for _, id := range []string{blockedWorkflow, healthyWorkflow} {
		if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{ID: id, Name: id, Nodes: []dag.Node{{ID: "gate", Type: "approval"}}}); err != nil {
			t.Fatal(err)
		}
	}
	failRun := func(workflow string) string {
		t.Helper()
		run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, workflow, newID())
		if err != nil {
			t.Fatal(err)
		}
		failed, err := f.e.Command(ctx, f.user.TenantID, f.project, run.ID, "approve", newID(), run.Version, map[string]any{"node_id": "gate", "decision": "rejected"})
		if err != nil || failed.State != "failed" {
			t.Fatalf("expected a genuine DAG failure: %s, %v", failed.State, err)
		}
		return run.ID
	}
	first := failRun(blockedWorkflow)
	for range blockedCount - 1 {
		failRun(blockedWorkflow)
	}
	healthy := failRun(healthyWorkflow)
	if _, err := f.s.DB.Exec(`CREATE TRIGGER block_failure_projection BEFORE INSERT ON suma_issues
		WHEN EXISTS(SELECT 1 FROM dag_runs WHERE id=json_extract(NEW.data,'$.run_id') AND workflow_id='failure-retry-blocker')
		BEGIN SELECT RAISE(ABORT,'fixture issue write failed'); END`); err != nil {
		t.Fatal(err)
	}
	var sequence int64
	if err := f.s.DB.QueryRow(`SELECT sequence FROM dag_events WHERE run_id=? AND event_type='run_failed'`, first).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	clock := func() int64 {
		t.Helper()
		var value int64
		if err := f.s.DB.QueryRow(`SELECT unixepoch('now')`).Scan(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	assertCounts := func(issues, acknowledgements, retries int) {
		t.Helper()
		if a, b, c := domainACount(t, f.s, "suma_issues"), domainACount(t, f.s, "suma_failure_projection"), domainACount(t, f.s, "suma_failure_projection_retries"); a != issues || b != acknowledgements || c != retries {
			t.Fatalf("projection state issues=%d acknowledgements=%d retries=%d, want %d/%d/%d", a, b, c, issues, acknowledgements, retries)
		}
	}
	assertRetry := func(attempts int, delay, before int64) {
		t.Helper()
		var actual int
		var due int64
		if err := f.s.DB.QueryRow(`SELECT attempts,next_attempt_at FROM suma_failure_projection_retries WHERE sequence=?`, sequence).Scan(&actual, &due); err != nil {
			t.Fatal(err)
		}
		if actual != attempts || due < before+delay || due > clock()+delay {
			t.Fatalf("unexpected retry state: attempts=%d next=%d", actual, due)
		}
	}
	assertProjection := func(run string) {
		t.Helper()
		var issues, acknowledgements int
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_issues WHERE json_extract(data,'$.run_id')=?`, run).Scan(&issues); err != nil {
			t.Fatal(err)
		}
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_failure_projection WHERE run_id=?`, run).Scan(&acknowledgements); err != nil {
			t.Fatal(err)
		}
		if issues != 1 || acknowledgements != 1 {
			t.Fatalf("run %s has %d issues and %d acknowledgements", run, issues, acknowledgements)
		}
	}
	makeDue := func() {
		t.Helper()
		if _, err := f.s.DB.Exec(`UPDATE suma_failure_projection_retries SET next_attempt_at=0 WHERE sequence=?`, sequence); err != nil {
			t.Fatal(err)
		}
	}
	deferRetries := func() {
		t.Helper()
		if _, err := f.s.DB.Exec(`UPDATE suma_failure_projection_retries SET next_attempt_at=unixepoch('now')+600`); err != nil {
			t.Fatal(err)
		}
	}

	before := clock()
	if err := f.s.ProjectFailures(ctx); err == nil {
		t.Fatal("blocked issue writes were not reported")
	}
	assertCounts(0, 0, 50)
	assertRetry(1, 1, before)
	if err := f.s.deferFailureProjection(ctx, sequence, 0, clock()+600); err != nil {
		t.Fatal(err)
	}
	assertRetry(1, 1, before)
	deferRetries()
	restarted := &Service{DB: f.s.DB, Dialect: "sqlite", DataDir: f.s.DataDir}
	if err := restarted.ProjectFailures(ctx); err == nil {
		t.Fatal("the remaining blocked event did not report its failure")
	}
	assertCounts(1, 1, blockedCount)
	assertProjection(healthy)

	deferRetries()
	makeDue()
	before = clock()
	if err := restarted.ProjectFailures(ctx); err == nil {
		t.Fatal("the still blocked event did not report its retry failure")
	}
	assertRetry(2, 2, before)
	assertCounts(1, 1, blockedCount)
	if _, err := f.s.DB.Exec(`DROP TRIGGER block_failure_projection`); err != nil {
		t.Fatal(err)
	}
	makeDue()
	for range 2 {
		if err := restarted.ProjectFailures(ctx); err != nil {
			t.Fatal(err)
		}
	}
	assertCounts(2, 2, blockedCount-1)
	assertProjection(first)
	assertProjection(healthy)
	if err := restarted.deferFailureProjection(ctx, sequence, 2, clock()); err != nil {
		t.Fatal(err)
	}
	assertCounts(2, 2, blockedCount-1)
}
