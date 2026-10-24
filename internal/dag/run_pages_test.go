package dag

import (
	"fmt"
	"testing"
)

func TestRunMetadataPagesAreScopedStableAndDoNotLoadGraphs(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "test", Type: "test", Command: []string{"test"}}}})
	for i := 0; i < 6; i++ {
		if _, err := e.CreateRun(ctx, "t", "p", "wf", fmt.Sprintf("run-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	var before, after int
	e.db.QueryRow(`SELECT seq FROM dag_lock WHERE id=1`).Scan(&before)
	seen := map[string]bool{}
	cursor := ""
	for {
		items, total, next, err := e.ListRunsPage(ctx, "t", "p", 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		if total != 7 || len(items) > 2 {
			t.Fatalf("page size or total wrong: %d %d", total, len(items))
		}
		for _, run := range items {
			if seen[run.ID] || len(run.Nodes) != 0 {
				t.Fatal("duplicate or graph-loaded summary")
			}
			seen[run.ID] = true
		}
		if next == "" {
			break
		}
		if _, _, _, err = e.ListRunsPage(ctx, "t", "other", 2, next); err == nil {
			t.Fatal("cross-project cursor accepted")
		}
		cursor = next
	}
	if len(seen) != 7 {
		t.Fatalf("omitted runs: %d", len(seen))
	}
	e.db.QueryRow(`SELECT seq FROM dag_lock WHERE id=1`).Scan(&after)
	if before != after {
		t.Fatal("metadata read mutated scheduler")
	}
	for id := range seen {
		full, err := e.GetRun(ctx, "t", "p", id)
		if err != nil || len(full.Nodes) != 1 {
			t.Fatalf("selected graph unavailable: %v", err)
		}
		break
	}
}
