package dag

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

type boundSubworkflowFixture struct {
	before, cleaned, bindings int
	denyScope                 bool
	unknown                   map[string]bool
}

func (h *boundSubworkflowFixture) BeforeBoundSubworkflowScope(context.Context, *sql.Tx, string, string) (func(), error) {
	h.before++
	cleanup := func() { h.cleaned++ }
	if h.denyScope {
		return cleanup, ErrConflict
	}
	return cleanup, nil
}
func (h *boundSubworkflowFixture) ResolveBoundSubworkflowTx(_ context.Context, _ *sql.Tx, r BoundSubworkflowRef) (BoundSubworkflowPlan, error) {
	if r.ParentRunID != "bound-parent" || r.Attempt != 1 || r.Fence != 1 || r.BindingID != "one" && r.BindingID != "two" {
		return BoundSubworkflowPlan{}, ErrInvalid
	}
	return BoundSubworkflowPlan{RunID: "child-" + r.BindingID, Workflow: Workflow{Name: "original child", Nodes: []Node{{ID: "work", Type: "command", Command: []string{"test-command"}}}}}, nil
}
func (h *boundSubworkflowFixture) BindBoundSubworkflowTx(_ context.Context, _ *sql.Tx, r BoundSubworkflowRef, run string) error {
	if run != "child-"+r.BindingID {
		return ErrInvalid
	}
	h.bindings++
	return nil
}
func (h *boundSubworkflowFixture) CheckBoundSubworkflowTx(_ context.Context, _ *sql.Tx, _ BoundSubworkflowRef, child, state string) (BoundSubworkflowCheck, error) {
	if h.unknown[child] {
		return BoundSubworkflowCheck{PauseReason: "fixture_outcome_unknown"}, nil
	}
	return BoundSubworkflowCheck{StopConfirmed: terminal(state)}, nil
}
func (h *boundSubworkflowFixture) AuthorizeBoundRunCommandTx(context.Context, *sql.Tx, string, string, string, string) error {
	return nil
}
func newBoundSubworkflowFixture(t *testing.T) (*Engine, *boundSubworkflowFixture, Run) {
	t.Helper()
	e := openEngine(t, filepath.Join(t.TempDir(), "bound.db"))
	h := &boundSubworkflowFixture{unknown: map[string]bool{}}
	e.BoundSubworkflows = h
	w := Workflow{Name: "protected sequence", Nodes: []Node{{ID: "one", Type: "subworkflow", Config: map[string]any{"service_binding_id": "one"}}, {ID: "two", Type: "subworkflow", DependsOn: []string{"one"}, Config: map[string]any{"service_binding_id": "two"}}}}
	r, err := e.CreateManagedRunBound(ctx, "t", "p", "bound-parent", "original-bound-start", w, func(*sql.Tx) error { return nil }, func(*sql.Tx, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return e, h, r
}
func TestBoundSubworkflowManagedAuthorityAndOriginalIdentity(t *testing.T) {
	e, h, r := newBoundSubworkflowFixture(t)
	if h.bindings != 1 {
		t.Fatal("downstream child started before its original dependency", h.bindings)
	}
	if _, err := e.Publish(ctx, "t", "p", Workflow{Name: "copied service binding", Nodes: []Node{{ID: "forged", Type: "subworkflow", Config: map[string]any{"service_binding_id": "one"}}}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("ordinary publication copied service authority", err)
	}
	a := claim(t, e)
	if a == nil || a.RunID != "child-one" {
		t.Fatal("reserved original child identity changed", a)
	}
	done(t, e, a, 0)
	parent, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || parent.State != "running" || h.bindings != 2 {
		t.Fatal("original child completion did not release exactly one successor", parent, h.bindings, err)
	}
	a = claim(t, e)
	if a == nil || a.RunID != "child-two" {
		t.Fatal(a)
	}
	done(t, e, a, 0)
	parent, err = e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || parent.State != "succeeded" || h.before != h.cleaned {
		t.Fatal("terminal state or scope cleanup lost", parent, h.before, h.cleaned, err)
	}
}
func TestBoundSubworkflowPauseDrainsCurrentChild(t *testing.T) {
	e, h, r := newBoundSubworkflowFixture(t)
	a := claim(t, e)
	paused, err := e.Command(ctx, "t", "p", r.ID, "pause", "pause-once", r.Version, map[string]any{"reason": "drain the current execution"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := e.GetRun(ctx, "t", "p", a.RunID)
	if err != nil || child.State != "running" {
		t.Fatal("bound child was paused instead of draining", child, err)
	}
	done(t, e, a, 0)
	paused, err = e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || paused.State != "paused" || h.bindings != 1 {
		t.Fatal("paused parent launched the next child", paused, h.bindings, err)
	}
	if _, err = e.Command(ctx, "t", "p", r.ID, "resume", "explicit-resume", paused.Version, map[string]any{"reason": "current authority reviewed"}); err != nil {
		t.Fatal(err)
	}
	if h.bindings != 2 {
		t.Fatal("resume did not continue the exact next child", h.bindings)
	}
}
func TestBoundSubworkflowUnknownPausesWithoutRedispatch(t *testing.T) {
	e, h, r := newBoundSubworkflowFixture(t)
	a := claim(t, e)
	h.unknown[a.RunID] = true
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "unknown-original", OutcomeUnknown: true}); err != nil {
		t.Fatal(err)
	}
	parent, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || parent.State != "paused" || h.bindings != 1 {
		t.Fatal("unknown external execution launched or recreated a child", parent, h.bindings, err)
	}
	if next := claim(t, e); next != nil {
		t.Fatal("unknown non-idempotent operation was redispatched", next)
	}
	if _, err = e.Command(ctx, "t", "p", r.ID, "retry", "no-blind-retry", parent.Version, map[string]any{"node_id": "one", "reason": "unknown is not retryable"}); !errors.Is(err, ErrConflict) {
		t.Fatal("bound operation could be blindly retried", err)
	}
}
func TestBoundSubworkflowScopeCleanupOnRejectedWake(t *testing.T) {
	e, h, r := newBoundSubworkflowFixture(t)
	// An authority denial happens before any scheduler mutation. Its lifecycle
	// cleanup must also run after rollback, including the system/nil-before path.
	h.denyScope = true
	before, cleaned := h.before, h.cleaned
	if _, err := e.Command(ctx, "t", "p", r.ID, "pause", "scope-denied", r.Version, map[string]any{}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if h.before != before+1 || h.cleaned != cleaned+1 {
		t.Fatal("denied scope retained resources", h.before, h.cleaned)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.Version != r.Version || current.State != r.State {
		t.Fatal("denied scope changed the original run", current, err)
	}
}
