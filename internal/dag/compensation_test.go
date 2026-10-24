package dag

import (
	"database/sql"
	"errors"
	"testing"
)

func compensable(id string, deps ...string) Node {
	return Node{ID: id, Type: "command", Command: []string{"apply", id}, DependsOn: deps, Idempotent: true, Config: map[string]any{"compensation": map[string]any{"type": "command", "command": []any{"undo", id, "{{source.attempt}}"}, "idempotent": true}}}
}
func TestExplicitCompensationRunsReverseDependenciesWithPinnedEvidence(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{compensable("a"), compensable("b", "a"), compensable("c", "b")}})
	for i := 0; i < 3; i++ {
		done(t, e, claim(t, e), 0)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "compensate", "compensate", r.Version, map[string]any{"reason": "approved inverse operation"})
	if err != nil {
		t.Fatal(err)
	}
	if r.CompensationRunID == "" || r.State != "succeeded" {
		t.Fatalf("original history overwritten %+v", r)
	}
	for _, want := range []string{"c", "b", "a"} {
		a := claim(t, e)
		if a == nil || a.RunID != r.CompensationRunID || a.Command[1] != want || a.Command[2] != "1" {
			t.Fatalf("inverse order want=%s got=%+v", want, a)
		}
		source, ok := a.Config["source_execution"].(map[string]any)
		if !ok || source["run_id"] != r.ID || source["node_id"] != want {
			t.Fatalf("source evidence missing %+v", a)
		}
		done(t, e, a, 0)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.CompensationState != "succeeded" {
		t.Fatal(r)
	}
	before := r.CompensationRunID
	r, err = e.Command(ctx, "t", "p", r.ID, "compensate", "compensate", 1, map[string]any{"reason": "approved inverse operation"})
	if err != nil || r.CompensationRunID != before {
		t.Fatalf("receipt repeated inverse %+v %v", r, err)
	}
}
func TestCompensationRejectsMissingDefinitionsAndRetryAfterCompensation(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "plain", Type: "task"}}})
	done(t, e, claim(t, e), 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if _, err := e.Command(ctx, "t", "p", r.ID, "compensate", "no-inverse", r.Version, map[string]any{"reason": "request"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	e, r = fixture(t, Workflow{Nodes: []Node{compensable("succeeded"), {ID: "failed", Type: "task", Idempotent: true, DependsOn: []string{"succeeded"}}}})
	done(t, e, claim(t, e), 0)
	done(t, e, claim(t, e), 1)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "compensate", "inverse", r.Version, map[string]any{"reason": "request"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Command(ctx, "t", "p", r.ID, "retry", "unsafe-retry", r.Version, map[string]any{"node_id": "failed"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestCompensationFailurePreservesSourceAndBlocksEarlierInverse(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{compensable("a"), compensable("b", "a")}})
	done(t, e, claim(t, e), 0)
	done(t, e, claim(t, e), 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "compensate", "inverse", r.Version, map[string]any{"reason": "request"})
	if err != nil {
		t.Fatal(err)
	}
	done(t, e, claim(t, e), 1)
	if claim(t, e) != nil {
		t.Fatal("earlier inverse ran after later inverse failed")
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "succeeded" || r.CompensationState != "failed" {
		t.Fatal(r)
	}
	var refs int
	if err = e.db.QueryRow("SELECT COUNT(*) FROM dag_compensation_effects WHERE original_run_id=?", r.ID).Scan(&refs); err != nil || refs != 2 {
		t.Fatalf("effect refs %d %v", refs, err)
	}
}

func TestCompensationPreservesParallelMapInverseOrderAfterRestart(t *testing.T) {
	template := map[string]any{"type": "command", "command": []any{"apply", "{{item}}"}, "config": map[string]any{"compensation": map[string]any{"type": "command", "command": []any{"undo", "{{source.node_id}}"}, "idempotent": true}}}
	e, r := fixture(t, Workflow{Nodes: []Node{compensable("before"), {ID: "map", Type: "map", DependsOn: []string{"before"}, Config: map[string]any{"items": []any{"x", "y"}, "template": template}}, compensable("after", "map")}})
	done(t, e, claim(t, e), 0)
	a, b := claim(t, e), claim(t, e)
	done(t, e, a, 0)
	done(t, e, b, 0)
	done(t, e, claim(t, e), 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "compensate", "inverse", r.Version, map[string]any{"reason": "request"})
	if err != nil {
		t.Fatal(err)
	}
	restarted, err := New(e.db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	a = claim(t, restarted)
	if a == nil || a.Command[1] != "after" {
		t.Fatal(a)
	}
	done(t, restarted, a, 0)
	a, b = claim(t, restarted), claim(t, restarted)
	if a == nil || b == nil || a.Command[1] == b.Command[1] || a.Command[1] == "before" || b.Command[1] == "before" {
		t.Fatalf("parallel inverse %+v %+v", a, b)
	}
	done(t, restarted, a, 0)
	if claim(t, restarted) != nil {
		t.Fatal("inverse predecessor ran before all children")
	}
	done(t, restarted, b, 0)
	a = claim(t, restarted)
	if a == nil || a.Command[1] != "before" {
		t.Fatal(a)
	}
	done(t, restarted, a, 0)
}
func TestCompensationRejectsActiveOrUnknownWorkAndRecursiveInverse(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{compensable("a")}})
	a := claim(t, e)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if _, err := e.Command(ctx, "t", "p", r.ID, "compensate", "premature", r.Version, map[string]any{"reason": "request"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	done(t, e, a, 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "compensate", "inverse", r.Version, map[string]any{"reason": "request"})
	if err != nil {
		t.Fatal(err)
	}
	done(t, e, claim(t, e), 0)
	inverse, _ := e.GetRun(ctx, "t", "p", r.CompensationRunID)
	if _, err = e.Command(ctx, "t", "p", inverse.ID, "compensate", "inverse-inverse", inverse.Version, map[string]any{"reason": "request"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestGeneratedCompensationWorkflowIsReadableButCannotBeReused(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{compensable("a")}})
	done(t, e, claim(t, e), 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "compensate", "inverse", r.Version, map[string]any{"reason": "request"})
	if err != nil {
		t.Fatal(err)
	}
	inverse, err := e.GetRun(ctx, "t", "p", r.CompensationRunID)
	if err != nil {
		t.Fatal(err)
	}
	w, err := e.GetWorkflow(ctx, "t", "p", inverse.WorkflowID, inverse.WorkflowVersion)
	if err != nil || w.ID != inverse.WorkflowID {
		t.Fatalf("internal graph unreadable %+v %v", w, err)
	}
	if _, err = e.CreateRunVersion(ctx, "t", "p", w.ID, 1, "replay-generated"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("generated inverse replayed %v", err)
	}
	if _, err = e.Publish(ctx, "t", "p", Workflow{ID: w.ID, Name: "bypass", Nodes: []Node{{ID: "a", Type: "noop"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("internal ID republished %v", err)
	}
	if _, err = e.Publish(ctx, "t", "p", Workflow{ID: "nested-bypass", Name: "nested", Nodes: []Node{{ID: "sub", Type: "subworkflow", Config: map[string]any{"workflow_id": w.ID, "workflow_version": 1}}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nested inverse replayed %v", err)
	}
	visible, err := e.Workflows(ctx, "t", "p")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range visible {
		if item.ID == w.ID {
			t.Fatal("generated inverse listed as public workflow")
		}
	}
}

func TestCompensationHookIsAtomicAndDoesNotReplay(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{compensable("a")}})
	done(t, e, claim(t, e), 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	originalVersion := r.Version
	if _, err := e.db.Exec("CREATE TABLE compensation_hook_test (run_id VARCHAR(100) PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	reject := errors.New("review invalidation failed")
	hook := func(tx *sql.Tx, run Run) error {
		calls++
		if run.CompensationRunID == "" {
			t.Fatal("hook ran before command transition")
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO compensation_hook_test (run_id) VALUES (?)", run.ID); err != nil {
			return err
		}
		return reject
	}
	payload := map[string]any{"reason": "request"}
	if _, err := e.CommandWithHook(ctx, "t", "p", r.ID, "compensate", "inverse-hook", r.Version, payload, hook); !errors.Is(err, reject) {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.CompensationRunID != "" || r.Version != originalVersion {
		t.Fatalf("failed hook leaked compensation %+v", r)
	}
	var count int
	if err := e.db.QueryRow("SELECT COUNT(*) FROM compensation_hook_test").Scan(&count); err != nil || count != 0 {
		t.Fatalf("hook row leaked %d %v", count, err)
	}
	reject = nil
	r, err := e.CommandWithHook(ctx, "t", "p", r.ID, "compensate", "inverse-hook", originalVersion, payload, hook)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := e.CommandWithHook(ctx, "t", "p", r.ID, "compensate", "inverse-hook", originalVersion, payload, hook)
	if err != nil || replayed.CompensationRunID != r.CompensationRunID || calls != 2 {
		t.Fatalf("hook repeated %d %+v %v", calls, replayed, err)
	}
}
