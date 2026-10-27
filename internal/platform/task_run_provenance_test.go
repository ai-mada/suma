package platform

import (
	"context"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func provenanceRun(t *testing.T, f *fixture) string {
	t.Helper()
	w, err := f.e.Publish(context.Background(), f.user.TenantID, f.project, dag.Workflow{Name: "Task provenance fixture", Nodes: []dag.Node{{ID: "work", Type: "command", Command: []string{"fixture"}}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.e.CreateRun(context.Background(), f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	return r.ID
}

func TestTaskRunProvenanceLegacyObservationAndBoundHistory(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Legacy current execution"}))
	if err != nil {
		t.Fatal(err)
	}
	oldRun, nextRun := provenanceRun(t, f), provenanceRun(t, f)
	// The original bind was not recorded by the new registry. A read must not
	// fabricate its timestamp or the version at which it originally happened.
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET run_id=?,version=7,status='in_progress' WHERE id=?`, oldRun, task.ID); err != nil {
		t.Fatal(err)
	}
	empty, err := f.s.ListTaskRunProvenance(ctx, f.actor, f.project, task.ID, 1, "")
	if err != nil || empty.Total != 0 || len(empty.Items) != 0 {
		t.Fatal("GET populated legacy provenance", empty, err)
	}
	if err = f.s.AttachRun(ctx, f.actor, f.project, task.ID, nextRun); err != nil {
		t.Fatal(err)
	}
	all, err := f.s.ListTaskRunProvenance(ctx, f.actor, f.project, task.ID, 20, "")
	if err != nil || all.Total != 2 || len(all.Items) != 2 {
		t.Fatal(all, err)
	}
	for _, item := range all.Items {
		if item.ObservedAt == "" {
			t.Fatal("missing actual observation time", item)
		}
		if item.RunID == oldRun {
			if item.BindingKind != "observed" || item.BoundAt != nil || item.TaskVersion != nil || item.Current {
				t.Fatal("fabricated legacy binding fact", item)
			}
		} else if item.RunID == nextRun {
			if item.BindingKind != "bound" || item.BoundAt == nil || item.TaskVersion == nil || *item.TaskVersion != 8 || !item.Current {
				t.Fatal("actual bound fact missing", item)
			}
		} else {
			t.Fatal(item)
		}
	}
	t.Log("public task execution history:", stringify(all))
	first, err := f.s.ListTaskRunProvenance(ctx, f.actor, f.project, task.ID, 1, "")
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" || first.Total != 2 {
		t.Fatal(first, err)
	}
	second, err := f.s.ListTaskRunProvenance(ctx, f.actor, f.project, task.ID, 1, first.NextCursor)
	if err != nil || len(second.Items) != 1 || second.Items[0].RunID == first.Items[0].RunID || second.NextCursor != "" || second.Total != 2 {
		t.Fatal(second, err)
	}
	other, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Other cursor scope"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ListTaskRunProvenance(ctx, f.actor, f.project, other.ID, 1, first.NextCursor); err == nil {
		t.Fatal("history cursor crossed task scope")
	}
	// Exact no-op binding must still consult the registry ownership.
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET run_id=? WHERE id=?`, nextRun, other.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.AttachRun(ctx, f.actor, f.project, other.ID, nextRun); err == nil {
		t.Fatal("no-op bypassed run ownership")
	}
}

func TestTaskRunProvenanceConcurrentBindingHasOneOwner(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	ids := []string{}
	for _, title := range []string{"First racing owner", "Second racing owner"} {
		task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": title}))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, task.ID)
	}
	run := provenanceRun(t, f)
	var group sync.WaitGroup
	results := make(chan error, 2)
	for _, id := range ids {
		group.Add(1)
		go func(task string) { defer group.Done(); results <- f.s.AttachRun(ctx, f.actor, f.project, task, run) }(id)
	}
	group.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	var owners, facts int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tasks WHERE run_id=?`, run).Scan(&owners); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_task_run_provenance WHERE run_id=?`, run).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if success != 1 || owners != 1 || facts != 1 {
		t.Fatal("concurrent run binding did not commit exactly one owner", success, owners, facts)
	}
}

func TestTaskRunProvenanceRejectsReusingAnotherTasksRun(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	a, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Original business task"}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Unrelated business task"}))
	if err != nil {
		t.Fatal(err)
	}
	first, second := provenanceRun(t, f), provenanceRun(t, f)
	if err = f.s.AttachRun(ctx, f.actor, f.project, a.ID, first); err != nil {
		t.Fatal(err)
	}
	if err = f.s.AttachRun(ctx, f.actor, f.project, b.ID, first); err == nil {
		t.Fatal("the same current run was attached to a different task")
	}
	if err = f.s.AttachRun(ctx, f.actor, f.project, a.ID, second); err != nil {
		t.Fatal(err)
	}
	if err = f.s.AttachRun(ctx, f.actor, f.project, b.ID, first); err == nil {
		t.Fatal("the historical run was stolen after rebind")
	}
	if err = f.s.AttachRun(ctx, f.actor, f.project, a.ID, second); err != nil {
		t.Fatal("exact binding replay failed", err)
	}
}
