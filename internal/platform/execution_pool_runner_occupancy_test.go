package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func TestExecutionPoolRunnerOccupancyTracksClaimAndRelease(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	runner := p.credential.Runner
	if idle := readPoolRunnerOccupancy(t, f, p.pool.ID, runner.ID); idle.Held != 0 || idle.Running != 0 || idle.Reconciling != 0 || idle.Unknown != 0 {
		t.Fatalf("unclaimed runner did not report known zero counts: %+v", idle)
	}
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: p.scopes}
	claimNext := func() dag.Assignment {
		t.Helper()
		out := f.bearer(http.MethodPost, "/pool-runner/claim", p.credential.Token, claim, http.StatusOK)
		var assignment dag.Assignment
		if err := json.Unmarshal([]byte(stringify(out["assignment"])), &assignment); err != nil || assignment.RunID == "" {
			t.Fatalf("missing actual pool assignment: %+v %v", assignment, err)
		}
		return assignment
	}
	first := claimNext()
	claim.SlotID = "1"
	claim.OperationID = newID()
	second := claimNext()
	if first.TenantID == second.TenantID {
		t.Fatal("expected dispatches across the two approved organizations")
	}
	if used := readPoolRunnerOccupancy(t, f, p.pool.ID, runner.ID, first.RunID, second.RunID, first.ProjectID, second.ProjectID, p.credential.Token); used.Held != 2 || used.Running != 2 || used.Reconciling != 0 || used.Unknown != 0 {
		t.Fatalf("claimed slots were not reported as two recorded running attempts: %+v", used)
	}

	result := func(a dag.Assignment, unknown bool) {
		t.Helper()
		body := poolAssignmentBody(a)
		body["result"] = dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), OutcomeUnknown: unknown}
		f.bearer(http.MethodPost, "/pool-runner/results", p.credential.Token, body, http.StatusOK)
	}
	result(first, true)
	if used := readPoolRunnerOccupancy(t, f, p.pool.ID, runner.ID); used.Held != 2 || used.Running != 1 || used.Reconciling != 1 || used.Unknown != 0 {
		t.Fatalf("uncertain result must stay occupied and count as reconciliation: %+v", used)
	}
	result(second, false)
	if used := readPoolRunnerOccupancy(t, f, p.pool.ID, runner.ID); used.Held != 1 || used.Running != 0 || used.Reconciling != 1 || used.Unknown != 0 {
		t.Fatalf("successful attempt released the wrong hold: %+v", used)
	}
	ctx := context.Background()
	run, err := f.e.GetRun(ctx, first.TenantID, first.ProjectID, first.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Command(ctx, first.TenantID, first.ProjectID, run.ID, "reconcile", newID(), run.Version, map[string]any{"node_id": first.NodeID, "outcome": "failed", "reason": "isolated fixture confirms the operation failed"}); err != nil {
		t.Fatal(err)
	}
	if done := readPoolRunnerOccupancy(t, f, p.pool.ID, runner.ID); done.Held != 0 || done.Running != 0 || done.Reconciling != 0 || done.Unknown != 0 {
		t.Fatalf("reconciled slot was not released: %+v", done)
	}
}

func TestExecutionPoolRunnerOccupancyConcealsScopeAndKeepsUnmatchedHolds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pool := seedCredentialPool(t, f)
	registered, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool, ExecutionPoolRunnerCreateRequest{
		Name: "runner with unmatched occupancy", MaxSlots: 4, TTLSeconds: 3600, IdempotencyKey: newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := registered.Runner
	missing := newID()
	wrongFence := newID()
	wrongWorker := newID()
	otherState := newID()
	for _, attempt := range []struct {
		run, worker, state string
		fence              int
	}{
		{wrongFence, runner.WorkerID, "running", 10},
		{wrongWorker, "pool-other-machine", "reconciling", 9},
		{otherState, runner.WorkerID, "succeeded", 9},
	} {
		if _, err = f.s.DB.Exec(`INSERT INTO dag_attempts (run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result) VALUES (?,'inspect',1,?,?,?,'','','')`, attempt.run, attempt.fence, attempt.worker, attempt.state); err != nil {
			t.Fatal(err)
		}
	}
	for i, run := range []string{missing, wrongFence, wrongWorker, otherState} {
		if _, err = f.s.DB.Exec(`INSERT INTO dag_execution_pool_slots (pool_id,runner_id,slot_id,run_id,node_id,attempt,fence) VALUES (?,?,?,?,'inspect',1,9)`, pool, runner.ID, string(rune('0'+i)), run); err != nil {
			t.Fatal(err)
		}
	}
	if observed := readPoolRunnerOccupancy(t, f, pool, runner.ID, missing, wrongFence, wrongWorker, otherState, registered.Token); observed.Held != 4 || observed.Unknown != 4 || observed.Running != 0 || observed.Reconciling != 0 {
		t.Fatalf("unmatched/mismatched holds were classified as a known attempt or as free: %+v", observed)
	}
	if got := f.call(http.MethodGet, "/execution-pools/"+pool+"/runners/"+runner.ID, nil, http.StatusOK); got["slot_occupancy"] != nil {
		t.Fatal("exact registration read disclosed occupancy")
	}
	apiToken := f.call(http.MethodPost, "/auth/tokens", f.body(map[string]any{"scope": "api"}), http.StatusCreated)["token"].(string)
	f.bearer(http.MethodGet, "/execution-pools/"+pool+"/runners", apiToken, nil, http.StatusForbidden)

	other, err := f.s.CreateOrganization(ctx, f.actor, map[string]any{"name": "unrelated pool organization", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, otherToken, err := f.s.SwitchOrganization(ctx, f.actor, map[string]any{"organization_id": other["id"], "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	otherActor, err := f.s.AuthenticateActor(ctx, otherToken)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ListExecutionPoolRunners(ctx, otherActor, pool, 50, "")
	exactPoolRunnerStatus(t, err, http.StatusNotFound)

	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	f.call(http.MethodGet, "/execution-pools/"+pool+"/runners", nil, http.StatusForbidden)
	var holds int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_slots WHERE pool_id=?`, pool).Scan(&holds); err != nil || holds != 4 {
		t.Fatalf("read changed the slot ledger: %d %v", holds, err)
	}
}

type poolRunnerOccupancyTestSnapshot struct {
	ObservedAt  string `json:"observed_at"`
	Held        int    `json:"held"`
	Running     int    `json:"running"`
	Reconciling int    `json:"reconciling"`
	Unknown     int    `json:"unknown"`
}

func readPoolRunnerOccupancy(t *testing.T, f *fixture, poolID, runnerID string, hidden ...string) poolRunnerOccupancyTestSnapshot {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/execution-pools/"+poolID+"/runners", nil)
	request.AddCookie(f.cookie)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("runner list: %d %s", response.Code, response.Body)
	}
	var page struct {
		Items []struct {
			ID            string                           `json:"id"`
			SlotOccupancy *poolRunnerOccupancyTestSnapshot `json:"slot_occupancy"`
		} `json:"items"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if item.ID != runnerID {
			continue
		}
		if item.SlotOccupancy == nil {
			t.Fatal("authorized runner list is missing the exact node's slot occupancy")
		}
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("occupancy response must be non-cacheable: %q", response.Header().Get("Cache-Control"))
		}
		for _, value := range hidden {
			if strings.Contains(response.Body.String(), value) {
				t.Fatal("occupancy response disclosed underlying project, run, or credential evidence")
			}
		}
		if _, err := time.Parse(time.RFC3339, item.SlotOccupancy.ObservedAt); err != nil {
			t.Fatal("occupancy is missing the authoritative read time", err)
		}
		return *item.SlotOccupancy
	}
	t.Fatal("occupancy list is missing the target runner")
	return poolRunnerOccupancyTestSnapshot{}
}
