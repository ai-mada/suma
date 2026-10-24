package dag

import (
	"errors"
	"strings"
	"testing"
)

func TestProgressReceiptOwnershipAndFinalResult(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "command", Command: []string{"local"}}}})
	a := claim(t, e)
	message := "building token=secret\x1b[31m\x00\n"
	if err := e.Progress(ctx, "t", "p", *a, "chunk-1", message); err != nil {
		t.Fatal(err)
	}
	stale := *a
	stale.Fence++
	if err := e.Progress(ctx, "t", "p", stale, "stale", "text"); !errors.Is(err, ErrStale) {
		t.Fatalf("stale: %v", err)
	}
	if err := e.Progress(ctx, "t", "other", *a, "foreign", "text"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("scope: %v", err)
	}
	if err := e.Progress(ctx, "t", "p", *a, "chunk-1", "different"); !errors.Is(err, ErrConflict) {
		t.Fatalf("receipt: %v", err)
	}
	done(t, e, a, 0)
	if err := e.Progress(ctx, "t", "p", *a, "chunk-1", message); err != nil {
		t.Fatalf("lost ACK replay: %v", err)
	}
	if err := e.Progress(ctx, "t", "p", *a, "late", "text"); !errors.Is(err, ErrStale) {
		t.Fatalf("terminal progress: %v", err)
	}
	events, err := e.Events(ctx, "t", "p", r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range events {
		if event.Type == "node_progress" {
			count++
			s, _ := event.Data["message"].(string)
			if strings.Contains(s, "secret") || strings.Contains(s, "\x1b") || strings.Contains(s, "\x00") {
				t.Fatalf("unsafe progress: %q", s)
			}
		}
	}
	if count != 1 {
		t.Fatalf("duplicate events: %d", count)
	}
	current, _ := e.GetRun(ctx, "t", "p", r.ID)
	if current.State != "succeeded" || current.Nodes[0].Output != "" {
		t.Fatalf("progress altered completion: %+v", current)
	}
}

func TestProgressExpiredLeaseAndSizeAreRejected(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	a := claim(t, e)
	if err := e.Progress(ctx, "t", "p", *a, "large", strings.Repeat("x", 8193)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("size: %v", err)
	}
	if _, err := e.db.Exec("UPDATE dag_nodes SET lease_until=0 WHERE run_id=?", a.RunID); err != nil {
		t.Fatal(err)
	}
	if err := e.Progress(ctx, "t", "p", *a, "expired", "text"); !errors.Is(err, ErrStale) {
		t.Fatalf("lease: %v", err)
	}
}
