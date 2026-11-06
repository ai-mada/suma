package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestFailureProjectionFrontierKeepsUnaccountedGapAcrossRestart(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	const ordinary = "frontier-ordinary"
	const blocked = "frontier-blocked"
	for _, id := range []string{ordinary, blocked} {
		if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{ID: id, Name: id, Nodes: []dag.Node{{ID: "gate", Type: "approval"}}}); err != nil {
			t.Fatal(err)
		}
	}
	failRun := func(workflow string) (string, int64) {
		t.Helper()
		run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, workflow, newID())
		if err != nil {
			t.Fatal(err)
		}
		if run, err = f.e.Command(ctx, f.user.TenantID, f.project, run.ID, "approve", newID(), run.Version, map[string]any{"node_id": "gate", "decision": "rejected"}); err != nil || run.State != "failed" {
			t.Fatalf("expected a durable failed run: %s, %v", run.State, err)
		}
		var sequence int64
		if err = f.s.DB.QueryRow(`SELECT sequence FROM dag_events WHERE run_id=? AND event_type='run_failed'`, run.ID).Scan(&sequence); err != nil {
			t.Fatal(err)
		}
		return run.ID, sequence
	}
	assertFrontier := func(s *Service, want int64) {
		t.Helper()
		if got, err := s.failureProjectionFrontier(ctx); err != nil || got != want {
			t.Fatalf("failure frontier=%d, want %d: %v", got, want, err)
		}
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	assertCounts := func(issues, acknowledgements, retries int) {
		t.Helper()
		if a, b, c := domainACount(t, f.s, "suma_issues"), domainACount(t, f.s, "suma_failure_projection"), domainACount(t, f.s, "suma_failure_projection_retries"); a != issues || b != acknowledgements || c != retries {
			t.Fatalf("issues=%d acknowledgements=%d retries=%d, want %d/%d/%d", a, b, c, issues, acknowledgements, retries)
		}
	}
	readReceipt := func(run string) string {
		t.Helper()
		var raw string
		if err := f.s.DB.QueryRowContext(ctx, `SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, failureIssueKey(run)).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		return raw
	}

	firstRun, firstSequence := failRun(ordinary)
	if err := f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	assertFrontier(f.s, firstSequence)
	firstReceipt := readReceipt(firstRun)
	restarted := &Service{DB: f.s.DB, Dialect: "sqlite", DataDir: f.s.DataDir}
	assertFrontier(restarted, firstSequence)
	if err := restarted.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	assertCounts(1, 1, 0)
	if readReceipt(firstRun) != firstReceipt {
		t.Fatal("restarting changed a previously acknowledged failure receipt")
	}

	blockedRun, blockedSequence := failRun(blocked)
	laterRun, laterSequence := failRun(ordinary)
	var step, parent, auxiliary int
	var access string
	if err := f.s.DB.QueryRow(`EXPLAIN QUERY PLAN SELECT sequence FROM dag_events WHERE event_type='run_failed' AND sequence>? AND sequence<=? ORDER BY sequence LIMIT ?`, firstSequence, laterSequence, failureProjectionBatchSize).Scan(&step, &parent, &auxiliary, &access); err != nil || !strings.Contains(access, "dag_events_type_sequence") || !strings.Contains(access, "sequence>?") {
		t.Fatalf("bounded failure discovery did not use the event type/sequence index: %q, %v", access, err)
	}
	// Keep the actual issue commit, then fail its separate acknowledgement and
	// retry writes. Only the original failure event is left unaccounted for.
	exec(`CREATE TRIGGER frontier_block_ack BEFORE INSERT ON suma_failure_projection
		WHEN EXISTS(SELECT 1 FROM dag_runs WHERE id=NEW.run_id AND workflow_id='frontier-blocked')
		BEGIN SELECT RAISE(ABORT,'fixture acknowledgement failure'); END`)
	exec(`CREATE TRIGGER frontier_block_retry BEFORE INSERT ON suma_failure_projection_retries
		BEGIN SELECT RAISE(ABORT,'fixture retry bookkeeping failure'); END`)
	if err := restarted.ProjectFailures(ctx); err == nil || !strings.Contains(err.Error(), "fixture retry bookkeeping failure") {
		t.Fatalf("expected persisted retry failure, got %v", err)
	}
	assertFrontier(restarted, firstSequence)
	assertCounts(3, 2, 0)
	blockedReceipt, laterReceipt := readReceipt(blockedRun), readReceipt(laterRun)
	var laterAck int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_failure_projection WHERE sequence=? AND run_id=?`, laterSequence, laterRun).Scan(&laterAck); err != nil || laterAck != 1 {
		t.Fatalf("later event could not project across the gap: %d, %v", laterAck, err)
	}

	exec(`DROP TRIGGER frontier_block_retry`)
	if err := restarted.ProjectFailures(ctx); err == nil || !strings.Contains(err.Error(), "fixture acknowledgement failure") {
		t.Fatalf("expected the original acknowledgement failure, got %v", err)
	}
	assertCounts(3, 2, 1)
	assertFrontier(restarted, laterSequence)
	exec(`DROP TRIGGER frontier_block_ack`)
	exec(`UPDATE suma_failure_projection_retries SET next_attempt_at=0 WHERE sequence=?`, blockedSequence)
	if err := restarted.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	assertFrontier(restarted, laterSequence)
	assertCounts(3, 3, 0)
	if readReceipt(firstRun) != firstReceipt || readReceipt(blockedRun) != blockedReceipt || readReceipt(laterRun) != laterReceipt {
		t.Fatal("retry replaced a committed issue instead of recovering its acknowledgement")
	}
	if err := restarted.advanceFailureProjectionFrontier(ctx, firstSequence); err != nil {
		t.Fatal(err)
	}
	assertFrontier(restarted, laterSequence)
}
