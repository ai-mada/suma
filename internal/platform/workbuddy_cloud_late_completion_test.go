package platform

import (
	"context"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestWorkBuddyCloudLateCompletionHTTPPreservesOriginalAttempt(t *testing.T) {
	for _, tc := range []struct {
		name, initialStatus, initialReason string
	}{
		{"waiting_input", "waiting_input", "cloud.waiting_input"},
		{"running", "running", "cloud.observation_unconfirmed"},
		{"stop_unconfirmed", "running", "cloud.observation_unconfirmed"},
		{"stale_fence", "waiting_input", "cloud.waiting_input"},
		{"event_rollback", "waiting_input", "cloud.waiting_input"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := newWorkBuddyTaskDispatchFixtureWithCreateStatus(t, tc.initialStatus)
			f := h.f
			f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: h.w}).Handler()
			a, sub := workBuddyTaskParkAtProviderWait(t, h)
			loadRun := func() dag.Run {
				t.Helper()
				run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, a.RunID)
				if err != nil || len(run.Nodes) != 1 {
					t.Fatalf("load original run: %+v, %v", run, err)
				}
				return run
			}
			attemptResult := func() string {
				t.Helper()
				var result string
				err := f.s.DB.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=? AND fence=?`, a.RunID, a.NodeID, a.Attempt, a.Fence).Scan(&result)
				if err != nil || result == "" {
					t.Fatalf("missing original Runner result: %v", err)
				}
				return result
			}
			readDelivery := func() map[string]any {
				t.Helper()
				out := f.call("GET", "/tasks/"+h.task.ID+"/delivery?project_id="+f.project, nil, 200)
				if out["formal_acceptance"] != false || out["run_basis_state"] != "current" {
					t.Fatalf("observation changed formal task delivery facts: %#v", out)
				}
				return out["run"].(map[string]any)
			}
			before := loadRun()
			if before.State != "running" || before.Nodes[0].State != "waiting" || before.Nodes[0].Reason != tc.initialReason || readDelivery()["workbuddy_delivery"] != nil {
				t.Fatalf("fixture did not park in the original Runner wait: %+v", before)
			}
			switch tc.name {
			case "stop_unconfirmed":
				var err error
				before, err = f.e.Command(ctx, f.user.TenantID, f.project, a.RunID, "cancel", newID(), before.Version, map[string]any{"reason": "Original stop request"})
				if err != nil || before.State != "cancelling" || before.Nodes[0].Reason != "cloud.stop_unconfirmed" {
					t.Fatalf("fixture did not establish the stop request: %+v, %v", before, err)
				}
			case "stale_fence":
				if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, a.RunID, a.NodeID); err != nil {
					t.Fatal(err)
				}
				before = loadRun()
			}
			originalResult, beforeEvents := attemptResult(), domainACount(t, f.s, "dag_events")
			path := "/workbuddy-cloud-submissions/" + sub.ID + "/observe"
			req := wc.CloudObserveRequest{ProjectID: f.project, ExpectedVersion: sub.Version, IdempotencyKey: newID()}
			if tc.name == "event_rollback" {
				if _, err := f.s.DB.Exec(`CREATE TRIGGER reject_cloud_delivery_event BEFORE INSERT ON dag_events
					WHEN NEW.event_type='cloud_delivery_observed' BEGIN SELECT RAISE(ABORT, 'test delivery event failure'); END`); err != nil {
					t.Fatal(err)
				}
				f.call("POST", path, req, 500)
				rolledBack := loadRun()
				if rolledBack.Version != before.Version || rolledBack.Nodes[0].Reason != tc.initialReason || domainACount(t, f.s, "dag_events") != beforeEvents || readDelivery()["workbuddy_delivery"] != nil {
					t.Fatalf("failed event left a visible DAG transition: %+v", rolledBack)
				}
				if _, err := f.s.DB.Exec(`DROP TRIGGER reject_cloud_delivery_event`); err != nil {
					t.Fatal(err)
				}
			}
			queries := h.queries.Load()
			observed := issueRepairRuntimeDecode[wc.CloudSubmission](t, f.call("POST", path, req, 200))
			if observed.ID != sub.ID || observed.Version != sub.Version+1 || observed.Task == nil || observed.Task.ID != sub.Task.ID || observed.Task.Status != "completed" || h.queries.Load() != queries+1 {
				t.Fatalf("owner did not observe the exact original provider task: %+v", observed)
			}
			after := loadRun()
			node := after.Nodes[0]
			if after.State != before.State || node.State != "waiting" || node.ID != a.NodeID || node.Attempt != a.Attempt || node.Fence != before.Nodes[0].Fence || node.CompletedAt != "" || attemptResult() != originalResult || domainACount(t, f.s, "dag_attempts") != 1 {
				t.Fatalf("observation changed original execution facts: %+v", after)
			}
			target, _ := readDelivery()["workbuddy_delivery"].(map[string]any)
			afterEvents := domainACount(t, f.s, "dag_events")
			if tc.name == "stop_unconfirmed" || tc.name == "stale_fence" {
				if node.Reason != before.Nodes[0].Reason || after.Version != before.Version || afterEvents != beforeEvents || target != nil {
					t.Fatalf("completed observation overrode a protected wait: %+v, %#v", after, target)
				}
			} else if node.Reason != "cloud.delivery_required" || after.Version != before.Version+1 || afterEvents <= beforeEvents || target["run_id"] != a.RunID || target["node_id"] != a.NodeID || target["attempt"] != float64(a.Attempt) {
				t.Fatalf("observation did not expose exact waiting delivery: %+v, %#v", after, target)
			}
			if target != nil {
				var count int
				if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE run_id=? AND node_id=? AND event_type='cloud_delivery_observed'`, a.RunID, a.NodeID).Scan(&count); err != nil || count != 1 {
					t.Fatalf("expected one event for the exact original node: %d, %v", count, err)
				}
			}
			replayed := issueRepairRuntimeDecode[wc.CloudSubmission](t, f.call("POST", path, req, 200))
			if replayed.Version != observed.Version || h.queries.Load() != queries+1 {
				t.Fatalf("observation replay repeated provider request or version change: %+v", replayed)
			}
			req.ExpectedVersion, req.IdempotencyKey = observed.Version, newID()
			f.call("POST", path, req, 200)
			if current := loadRun(); current.Version != after.Version || current.Nodes[0].Reason != node.Reason || domainACount(t, f.s, "dag_events") != afterEvents || h.creates.Load() != 1 || h.queries.Load() != queries+2 {
				t.Fatalf("repeat observation rewrote the DAG or redispatched: %+v", current)
			}
		})
	}
}
