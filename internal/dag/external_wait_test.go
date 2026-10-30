package dag

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func cloudWaitFixture(t *testing.T) (*Engine, Run, *Assignment, Result) {
	t.Helper()
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "cloud", Type: wc.NodeType, Resources: map[string]int64{"external_effect": 1}, Config: map[string]any{"plugin": wc.Plugin, "workflow": wc.Protocol}}, {ID: "after", Type: "command", Command: []string{"test-command"}, DependsOn: []string{"cloud"}}}})
	b := wc.CloudAssignmentBinding{DelegationID: "delegation", DelegationVersion: 1, DelegationSHA256: strings.Repeat("a", 64)}
	h := &RunnableHook{BindAssignmentTx: func(_ context.Context, _ *sql.Tx, a *Assignment) error { a.WorkBuddyCloud = &b; return nil }}
	a, err := e.ClaimWithOperation(ctx, "t", "p", "worker", "cloud-claim", []string{"external_workflow:workbuddy-cloud"}, nil, nil, h)
	if err != nil || a == nil {
		t.Fatal("protected claim", err)
	}
	result := Result{RunID: r.ID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: -1, OutcomeUnknown: true, OperationID: "cloud-final", WorkBuddyCloudWait: &wc.CloudWaitEvidence{Protocol: wc.Protocol, ClaimID: a.DispatchID, Assignment: externalAssignmentRef(*a), Binding: b, CreateOperationID: "original-create", Cause: "delivery_required"}}
	return e, r, a, result
}

func TestCloudWaitReleasesProcessWithoutInventingCompletion(t *testing.T) {
	e, r, _, result := cloudWaitFixture(t)
	before := func(*sql.Tx) error { return nil }
	commit := func(_ *sql.Tx, state string) error {
		if state != "waiting" {
			t.Fatalf("unverified terminal: %s", state)
		}
		return nil
	}
	calls := 0
	resolve := func(context.Context, *sql.Tx, ExternalAssignment, Result) (ExternalResultResolution, error) {
		calls++
		return ExternalResultResolution{Reason: "cloud.delivery_required"}, nil
	}
	if err := e.CompleteBound(ctx, "t", "p", result, before, commit); !errors.Is(err, ErrInvalid) {
		t.Fatal("machine acquired resolver authority", err)
	}
	for _, mutate := range []func(*Result){func(r *Result) { r.ExitCode = 0 }, func(r *Result) { r.Stopped = true }, func(r *Result) { r.WorkBuddyCloudWait.ClaimID = "other-claim" }} {
		bad := result
		proof := *result.WorkBuddyCloudWait
		bad.WorkBuddyCloudWait = &proof
		mutate(&bad)
		if err := e.CompleteBound(ctx, "t", "p", bad, before, commit, resolve); err == nil {
			t.Fatal("invalid success/stop/replacement accepted")
		}
	}
	for range 2 {
		if err := e.CompleteBound(ctx, "t", "p", result, before, commit, resolve); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("ACK replay repeated resolution", calls)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range current.Nodes {
		if n.ID == "cloud" && (n.State != "waiting" || n.CompletedAt != "" || n.Reason != "cloud.delivery_required") {
			t.Fatal(n)
		}
		if n.ID == "after" && n.Attempt != 0 {
			t.Fatal("provider completion unlocked downstream", n)
		}
	}
	for name, want := range map[string]string{"execution_slots": "released", "external_effect": "active"} {
		var state string
		if err = e.db.QueryRow(`SELECT state FROM dag_resource_ledger WHERE run_id=? AND name=?`, r.ID, name).Scan(&state); err != nil || state != want {
			t.Fatalf("%s=%s want %s: %v", name, state, want, err)
		}
	}
	changed := result
	changed.OperationID = "different-final"
	if err = e.CompleteBound(ctx, "t", "p", changed, before, commit, resolve); !errors.Is(err, ErrConflict) {
		t.Fatal("new operation rewrote original wait", err)
	}
	if _, err = e.Command(ctx, "t", "p", r.ID, "reconcile", "manual-green", current.Version, map[string]any{"node_id": "cloud", "outcome": "succeeded", "reason": "unverified"}); err == nil {
		t.Fatal("manual completion bypassed evidence")
	}
	cancelled, err := e.Command(ctx, "t", "p", r.ID, "cancel", "request-stop", current.Version, map[string]any{"reason": "stop requested"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != "cancelling" {
		t.Fatal("remote stop invented", cancelled.State)
	}
	for _, n := range cancelled.Nodes {
		if n.ID == "cloud" && (n.State != "waiting" || n.Reason != "cloud.stop_unconfirmed") {
			t.Fatal(n)
		}
	}
}

func TestCloudExpiredLeaseParksAndOriginalMachineMayRefine(t *testing.T) {
	e, r, a, result := cloudWaitFixture(t)
	if _, err := e.db.Exec(`UPDATE dag_nodes SET lease_until=1 WHERE run_id=? AND id=?`, r.ID, a.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE dag_wakeups SET due_at=1 WHERE run_id=? AND kind='lease'`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range current.Nodes {
		if n.ID == a.NodeID && (n.State != "waiting" || n.Reason != "cloud.runner_disconnected") {
			t.Fatal(n)
		}
	}
	resolve := func(context.Context, *sql.Tx, ExternalAssignment, Result) (ExternalResultResolution, error) {
		return ExternalResultResolution{Reason: "cloud.remote_unknown"}, nil
	}
	if err = e.CompleteBound(ctx, "t", "p", result, func(*sql.Tx) error { return nil }, func(*sql.Tx, string) error { return nil }, resolve); err != nil {
		t.Fatal("original machine could not refine system wait", err)
	}
}

func TestCloudNoEffectVerdictIsTrustedAndReleasesAllReservations(t *testing.T) {
	e, r, _, result := cloudWaitFixture(t)
	resolve := func(context.Context, *sql.Tx, ExternalAssignment, Result) (ExternalResultResolution, error) {
		return ExternalResultResolution{NoExternalEffect: true}, nil
	}
	if err := e.CompleteBound(ctx, "t", "p", result, func(*sql.Tx) error { return nil }, func(_ *sql.Tx, state string) error {
		if state != "failed" {
			t.Fatal(state)
		}
		return nil
	}, resolve); err != nil {
		t.Fatal(err)
	}
	var active int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger WHERE run_id=? AND state='active'`, r.ID).Scan(&active); err != nil || active != 0 {
		t.Fatal("verified no-effect retained resources", active, err)
	}
}
