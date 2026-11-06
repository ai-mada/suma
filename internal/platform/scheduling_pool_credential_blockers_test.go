package platform

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestSchedulingPoolRunnerCredentialBlockers(t *testing.T) {
	f := newFixture(t)
	const work = "work"
	workflow := f.call("POST", "/workflows", f.body(map[string]any{
		"name": "Pool credential scheduling",
		"nodes": []dag.Node{
			{ID: work, Type: "command", Command: []string{"inspect"}},
			{ID: "dependent", Type: "command", Command: []string{"verify"}, DependsOn: []string{work}},
			{ID: "review", Type: "approval", DependsOn: []string{work}},
		},
	}), 201)
	start := func() string {
		t.Helper()
		return f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"]}), 201)["id"].(string)
	}
	privateRun := start()
	pool, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	firstRun, secondRun := start(), start()
	queuePath := "/scheduling/queue?project_id=" + f.project
	readNode := func(run, node string) dag.NodeAdmissionSnapshot {
		t.Helper()
		return issueRepairRuntimeDecode[dag.NodeAdmissionSnapshot](t, f.call("GET", "/runs/"+run+"/nodes/"+node+"/scheduling?project_id="+f.project, nil, 200))
	}
	var privateIdentity []string
	assertFacts := func(want, state string) {
		t.Helper()
		before := historyDatabaseHash(t, f.s.DB)
		payload := f.call("GET", queuePath, nil, 200)
		queue := issueRepairRuntimeDecode[dag.ReadyPage](t, payload)
		if queue.Total != 3 || len(queue.Items) != 3 {
			t.Fatalf("credential check changed ready membership: %+v", queue)
		}
		assertSnapshot := func(snapshot dag.NodeAdmissionSnapshot, run, node, expected, nodeState string) {
			t.Helper()
			if snapshot.RunID != run || snapshot.NodeID != node || snapshot.NodeState != nodeState || snapshot.Attempt != 0 {
				t.Fatalf("unexpected scheduling identity or attempt: %+v", snapshot)
			}
			var found bool
			for _, blocker := range snapshot.Blockers {
				switch blocker.Code {
				case "pool_runner_registration_missing", "pool_runner_credential_unavailable":
					if blocker.Code != expected || found {
						t.Fatalf("unexpected credential blocker for %s/%s: %+v", run, node, blocker)
					}
				}
				if expected != "" && blocker.Code == expected {
					if blocker.Source != "pool" || blocker.RunID != run || blocker.NodeID != node || blocker.Message == "" {
						t.Fatalf("unexpected scope or reason: %+v", blocker)
					}
					found = true
				}
			}
			if expected != "" && (!found || snapshot.Status != "blocked") {
				t.Fatalf("missing %s for %s/%s: %+v", expected, run, node, snapshot)
			}
		}
		for _, item := range queue.Items {
			exact := readNode(item.RunID, item.NodeID)
			if item.Admission == nil || !reflect.DeepEqual(item.Admission.Blockers, exact.Blockers) || item.Admission.Status != exact.Status {
				t.Fatalf("queue and exact node differ for %s/%s", item.RunID, item.NodeID)
			}
			expected, status, poolID := want, state, pool.ID
			switch item.RunID {
			case privateRun:
				expected, status, poolID = "", "unknown", ""
			case firstRun, secondRun:
			default:
				t.Fatalf("unexpected ready run %s", item.RunID)
			}
			assertSnapshot(exact, item.RunID, work, expected, "ready")
			if exact.Status != status || exact.ExecutionPoolID != poolID {
				t.Fatalf("unexpected readiness or frozen pool: %+v", exact)
			}
		}
		assertSnapshot(readNode(firstRun, "dependent"), firstRun, "dependent", want, "pending")
		assertSnapshot(readNode(firstRun, "review"), firstRun, "review", "", "pending")
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range privateIdentity {
			if strings.Contains(string(raw), value) {
				t.Fatal("queue exposed owner-only runner identity or credential")
			}
		}
		if before != historyDatabaseHash(t, f.s.DB) {
			t.Fatal("scheduling diagnosis mutated persisted business state")
		}
	}
	assertFacts("pool_runner_registration_missing", "blocked")

	const runnerName = "Owner-only scheduling fixture Runner"
	poolPath := "/execution-pools/" + pool.ID
	registered := issueRepairRuntimeDecode[ExecutionPoolCredentialResponse](t, f.call("POST", poolPath+"/runners", map[string]any{
		"name": runnerName, "max_slots": 1, "ttl_seconds": 3600, "idempotency_key": newID(),
	}, 201))
	privateIdentity = []string{registered.Runner.ID, registered.Runner.WorkerID, runnerName, registered.Token}
	assertFacts("", "unknown")

	if _, err := f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET expires_at=? WHERE runner_id=?`, "2000-01-01T00:00:00Z", registered.Runner.ID); err != nil {
		t.Fatal(err)
	}
	assertFacts("pool_runner_credential_unavailable", "blocked")

	f.call("POST", poolPath+"/commands", map[string]any{"action": "pause", "expected_version": pool.Version, "idempotency_key": newID()}, 200)
	assertFacts("pool_paused", "blocked")
	f.call("POST", poolPath+"/commands", map[string]any{"action": "resume", "expected_version": pool.Version + 1, "idempotency_key": newID()}, 200)
	f.call("POST", poolPath+"/runners/"+registered.Runner.ID+"/commands", map[string]any{
		"action": "revoke", "expected_version": registered.Runner.Version, "idempotency_key": newID(),
	}, 200)
	assertFacts("pool_runner_registration_missing", "blocked")

	if _, err := f.s.DB.Exec(`UPDATE dag_execution_pool_scopes SET grant_version=grant_version+1 WHERE pool_id=? AND project_id=?`, pool.ID, f.project); err != nil {
		t.Fatal(err)
	}
	assertFacts("pool_scope_version_mismatch", "blocked")
	if _, err := f.s.DB.Exec(`UPDATE dag_execution_pool_scopes SET grant_version=? WHERE pool_id=? AND project_id=?`, grant.Version, pool.ID, f.project); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/project-execution/grants/"+grant.ID+"/commands", f.body(map[string]any{
		"action": "withdraw", "expected_version": grant.Version,
	}), 200)
	assertFacts("pool_grant_not_current", "blocked")
}
