package dag

import "testing"

func TestPublicEventsNormalizeEmptyMetadataWithoutRewritingFacts(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "command", Command: []string{"local"}}}})
	done(t, e, claim(t, e), 0)
	var persisted string
	if err := e.db.QueryRow(`SELECT data FROM dag_events WHERE run_id=? AND event_type='run_succeeded'`, r.ID).Scan(&persisted); err != nil || persisted != "null" {
		t.Fatalf("terminal fact fixture: %q %v", persisted, err)
	}
	events, err := e.Events(ctx, "t", "p", r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var terminal bool
	for _, event := range events {
		if event.Data == nil {
			t.Fatalf("public event violates object metadata contract: %s", event.Type)
		}
		if event.Type == "run_succeeded" {
			terminal = true
			if len(event.Data) != 0 {
				t.Fatal("invented terminal metadata")
			}
		}
	}
	if !terminal {
		t.Fatal("terminal event missing")
	}
	if err = e.db.QueryRow(`SELECT data FROM dag_events WHERE run_id=? AND event_type='run_succeeded'`, r.ID).Scan(&persisted); err != nil || persisted != "null" {
		t.Fatalf("read rewrote original fact: %q %v", persisted, err)
	}
}
