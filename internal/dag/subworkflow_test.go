package dag

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func nestedFixture(t *testing.T) (*Engine, Run) {
	t.Helper()
	e := openEngine(t, filepath.Join(t.TempDir(), "nested.db"))
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "child", Name: "child", Nodes: []Node{{ID: "work", Type: "task", Command: []string{"version-one"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "parent", Name: "parent", Nodes: []Node{{ID: "sub", Type: "subworkflow", Idempotent: true, Config: map[string]any{"workflow_id": "child", "workflow_version": 1}}, {ID: "after", Type: "task", DependsOn: []string{"sub"}}}}); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", "parent", "parent-create")
	if err != nil {
		t.Fatal(err)
	}
	return e, r
}
func TestSubworkflowPinsVersionAndPropagatesCompletion(t *testing.T) {
	e, r := nestedFixture(t)
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "child", Name: "child", Nodes: []Node{{ID: "work", Type: "task", Command: []string{"version-two"}}}}); err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if a == nil || a.RunID == r.ID || a.Command[0] != "version-one" {
		t.Fatalf("child not pinned %+v", a)
	}
	parent, _ := e.GetRun(ctx, "t", "p", r.ID)
	sub := parent.Nodes[1]
	if sub.ID != "sub" || sub.State != "waiting" || sub.ChildRunID != a.RunID {
		t.Fatalf("parent slot %+v", parent)
	}
	if claim(t, e) != nil {
		t.Fatal("parent waiting occupied worker or successor ran")
	}
	done(t, e, a, 0)
	next := claim(t, e)
	if next == nil || next.RunID != r.ID || next.NodeID != "after" {
		t.Fatalf("child completion did not unlock parent %+v", next)
	}
	done(t, e, next, 0)
	parent, _ = e.GetRun(ctx, "t", "p", r.ID)
	if parent.State != "succeeded" {
		t.Fatal(parent)
	}
}
func TestSubworkflowRequiresPinnedExistingScopedVersion(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "validation.db"))
	for _, config := range []map[string]any{{"workflow_id": "missing"}, {"workflow_id": "missing", "workflow_version": 1}} {
		_, err := e.Publish(ctx, "t", "p", Workflow{ID: "parent", Name: "parent", Nodes: []Node{{ID: "sub", Type: "subworkflow", Config: config}}})
		if err == nil {
			t.Fatal("invalid child reference accepted")
		}
	}
	if _, err := e.Publish(ctx, "other", "p", Workflow{ID: "child", Name: "child", Nodes: []Node{{ID: "a", Type: "noop"}}}); err != nil {
		t.Fatal(err)
	}
	_, err := e.Publish(ctx, "t", "p", Workflow{ID: "parent", Name: "parent", Nodes: []Node{{ID: "sub", Type: "subworkflow", Config: map[string]any{"workflow_id": "child", "workflow_version": 1}}}})
	if err == nil {
		t.Fatal("cross tenant child reference accepted")
	}
}
func TestSubworkflowPauseResumeAndCancelWaitForActualStop(t *testing.T) {
	e, r := nestedFixture(t)
	a := claim(t, e)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "pause", "pause", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, _ := e.GetRun(ctx, "t", "p", a.RunID)
	if child.State != "paused" {
		t.Fatalf("pause not propagated %+v", child)
	}
	if _, err = e.Command(ctx, "t", "p", child.ID, "resume", "forbidden-resume", child.Version, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("child bypassed paused parent %v", err)
	}
	r, err = e.Command(ctx, "t", "p", r.ID, "resume", "resume", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, _ = e.GetRun(ctx, "t", "p", a.RunID)
	if child.State != "running" {
		t.Fatal(child.State)
	}
	r, err = e.Command(ctx, "t", "p", r.ID, "cancel", "cancel", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "cancelling" {
		t.Fatal("parent claimed stop early")
	}
	if err = e.Heartbeat(ctx, "t", "p", *a); !errors.Is(err, ErrCancelled) {
		t.Fatal(err)
	}
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, Stopped: true, OperationID: "stopped"}); err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "cancelled" {
		t.Fatalf("parent did not resolve stop %+v", r)
	}
}
func TestSubworkflowRestartAndRetryRetainFailedChild(t *testing.T) {
	e, r := nestedFixture(t)
	a := claim(t, e)
	done(t, e, a, 1)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "failed" {
		t.Fatal(r.State)
	}
	oldChild := a.RunID
	r, err := e.Command(ctx, "t", "p", r.ID, "retry", "retry-sub", r.Version, map[string]any{"node_id": "sub"})
	if err != nil {
		t.Fatal(err)
	}
	a = claim(t, e)
	if a == nil || a.RunID == oldChild {
		t.Fatal("retry reused child run")
	}
	old, err := e.GetRun(ctx, "t", "p", oldChild)
	if err != nil || old.State != "failed" {
		t.Fatalf("failed child lost %+v %v", old, err)
	}
	newEngine, err := New(e.db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	done(t, newEngine, a, 0)
	next := claim(t, newEngine)
	if next == nil || next.RunID != r.ID {
		t.Fatal(next)
	}
	var links int
	if err = e.db.QueryRow("SELECT COUNT(*) FROM dag_subruns WHERE parent_run_id=?", r.ID).Scan(&links); err != nil || links != 2 {
		t.Fatalf("attempt links %d %v", links, err)
	}
}

func TestNestedSubworkflowCompletesWithoutWorkerAndCancelWaitingApproval(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "nested-noop.db"))
	for _, w := range []Workflow{{ID: "leaf", Name: "leaf", Nodes: []Node{{ID: "noop", Type: "noop"}}}, {ID: "middle", Name: "middle", Nodes: []Node{{ID: "sub", Type: "subworkflow", Config: map[string]any{"workflow_id": "leaf", "workflow_version": 1}}}}, {ID: "root", Name: "root", Nodes: []Node{{ID: "sub", Type: "subworkflow", Config: map[string]any{"workflow_id": "middle", "workflow_version": 1}}}}} {
		if _, err := e.Publish(ctx, "t", "p", w); err != nil {
			t.Fatal(err)
		}
	}
	r, err := e.CreateRun(ctx, "t", "p", "root", "noop-root")
	if err != nil || r.State != "succeeded" {
		t.Fatalf("nested immediate child %+v %v", r, err)
	}
	if claim(t, e) != nil {
		t.Fatal("internal workflow claimed worker")
	}
	if _, err = e.Publish(ctx, "t", "p", Workflow{ID: "approval", Name: "approval", Nodes: []Node{{ID: "wait", Type: "approval"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = e.Publish(ctx, "t", "p", Workflow{ID: "cancel-parent", Name: "cancel-parent", Nodes: []Node{{ID: "sub", Type: "subworkflow", Config: map[string]any{"workflow_id": "approval", "workflow_version": 1}}}}); err != nil {
		t.Fatal(err)
	}
	r, err = e.CreateRun(ctx, "t", "p", "cancel-parent", "cancel-wait")
	if err != nil {
		t.Fatal(err)
	}
	r, err = e.Command(ctx, "t", "p", r.ID, "cancel", "cancel-waiting", r.Version, nil)
	if err != nil || r.State != "cancelled" {
		t.Fatalf("waiting child not cancelled atomically %+v %v", r, err)
	}
}
func TestParentResumePreservesIndependentlyPausedChild(t *testing.T) {
	e, r := nestedFixture(t)
	a := claim(t, e)
	child, _ := e.GetRun(ctx, "t", "p", a.RunID)
	child, err := e.Command(ctx, "t", "p", child.ID, "pause", "child-pause", child.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err = e.Command(ctx, "t", "p", r.ID, "pause", "parent-pause", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	r, err = e.Command(ctx, "t", "p", r.ID, "resume", "parent-resume", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	child, _ = e.GetRun(ctx, "t", "p", child.ID)
	if child.State != "paused" {
		t.Fatal("parent resumed independently paused child")
	}
}
func TestExternalWorkflowRequiresExactPluginCapability(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "external", Type: "external_workflow", Config: map[string]any{"plugin": "github", "workflow": "build", "input": map[string]any{"ref": "fixed"}}}}})
	if a := claim(t, e); a != nil {
		t.Fatal("unconfigured plugin claimed")
	}
	if a, err := e.Claim(ctx, "t", "p", "worker", []string{"external_workflow", "external_workflow:other"}); err != nil || a != nil {
		t.Fatalf("generic capability claimed %+v %v", a, err)
	}
	a, err := e.Claim(ctx, "t", "p", "worker", []string{"external_workflow:github"})
	if err != nil || a == nil || a.Type != "external_workflow" || a.Config["workflow"] != "build" {
		t.Fatalf("plugin envelope %+v %v", a, err)
	}
	done(t, e, a, 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "succeeded" {
		t.Fatal(r.State)
	}
}
func TestExternalWorkflowRejectsShellAndMissingPlugin(t *testing.T) {
	for _, node := range []Node{{ID: "e", Type: "external_workflow"}, {ID: "e", Type: "external_workflow", Command: []string{"sh", "-c", "fake lifecycle"}, Config: map[string]any{"plugin": "github", "workflow": "build"}}} {
		if Compile(Workflow{Nodes: []Node{node}}) == nil {
			t.Fatalf("invalid external protocol accepted %+v", node)
		}
	}
}

func TestPublicationBoundsMaterializedSubworkflowFanout(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "bounded.db"))
	nodes := make([]Node, 20)
	for i := range nodes {
		nodes[i] = Node{ID: fmt.Sprintf("n%d", i), Type: "task"}
	}
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "child", Name: "child", Nodes: nodes}); err != nil {
		t.Fatal(err)
	}
	items := make([]any, 1000)
	for i := range items {
		items[i] = i
	}
	_, err := e.Publish(ctx, "t", "p", Workflow{ID: "parent", Name: "parent", Nodes: []Node{{ID: "fanout", Type: "map", Config: map[string]any{"items": items, "template": map[string]any{"type": "subworkflow", "config": map[string]any{"workflow_id": "child", "workflow_version": 1}}}}}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded fanout accepted %v", err)
	}
}

func TestSubworkflowAutomaticRetryIsDurableAndDoesNotSpinInOneTransaction(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "subretry.db"))
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "child", Name: "child", Nodes: []Node{{ID: "map", Type: "map", Config: map[string]any{"items": []any{1}, "template": map[string]any{"type": "task", "condition": "false"}}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "parent", Name: "parent", Nodes: []Node{{ID: "sub", Type: "subworkflow", Idempotent: true, MaxRetries: 2, Config: map[string]any{"workflow_id": "child", "workflow_version": 1}}}}); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", "parent", "create")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err = e.db.QueryRow("SELECT COUNT(*) FROM dag_subruns WHERE parent_run_id=?", r.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("internal retry spun %d %v", count, err)
	}
	for i := 0; i < 2; i++ {
		if _, err = e.db.Exec("UPDATE dag_nodes SET wake_at=1 WHERE run_id=? AND id='sub'", r.ID); err != nil {
			t.Fatal(err)
		}
		if err = e.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "failed" || r.Nodes[0].Attempt != 3 {
		t.Fatalf("retry budget %+v", r)
	}
}

func TestSubworkflowDeadlineWaitsForChildStopBeforeFailure(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "deadline.db"))
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "child", Name: "child", Nodes: []Node{{ID: "a", Type: "task"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "parent", Name: "parent", Nodes: []Node{{ID: "sub", Type: "subworkflow", TimeoutSeconds: 10, Config: map[string]any{"workflow_id": "child", "workflow_version": 1}}}}); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", "parent", "create")
	if err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if _, err = e.db.Exec("UPDATE dag_nodes SET wake_at=1 WHERE run_id=? AND id='sub'", r.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "running" || r.Nodes[0].State != "waiting" {
		t.Fatal("timeout fabricated child termination")
	}
	if err = e.Heartbeat(ctx, "t", "p", *a); !errors.Is(err, ErrCancelled) {
		t.Fatal(err)
	}
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "stopped", Stopped: true}); err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "failed" || !strings.Contains(r.Nodes[0].Error, "deadline") {
		t.Fatalf("deadline not evidenced %+v", r)
	}
}
