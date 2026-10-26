package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This is a control-plane/Pool HTTP protocol fixture. No model or vendor CLI
// runs; the ordinary typed completion proof is validated by the real API/Core.
func TestIntelligentPlanningPoolHTTPResultAndCurrentCreator(t *testing.T) {
	p := newPoolMachineFixture(t)
	f, ctx := p.f, context.Background()
	runner, token := p.credential.Runner, p.credential.Token
	profile := toolTestProfile()
	profile.ReadOnly = true
	profile.Planning = &tc.PlanningCapability{Protocol: tc.PlanningProtocol, Model: "pool-protocol-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	f.bearer("POST", "/pool-runner/tool-inventory", token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}, 200)
	pr, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "Pool read-only planning fixture", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, f.actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: pr.Version, IdempotencyKey: newID()})
	if err != nil || !pr.Available {
		t.Fatal("pool planner lacks current project approval", err)
	}
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Pool planning scope", "description": "An existing task; no code execution"}))
	if err != nil {
		t.Fatal(err)
	}
	h := &intelligentFixture{f: f, profile: pr, task: task, request: PlanningAnalysisCreateRequest{ProjectID: f.project, Goal: "Review the shared team's schedule risks", Locale: "en-US", TaskIDs: []string{task.ID}, ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}}
	a := h.create(t)
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.CommandID}}}}
	reply := f.bearer("POST", "/pool-runner/claim", token, claim, 200)
	var assignment dag.Assignment
	if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment); err != nil || assignment.RunID != a.RunID || assignment.NodeID != "analyze" || assignment.Attempt != 1 || assignment.PoolGrantID != p.grants[0].ID || assignment.Config["purpose"] != "planning" {
		t.Fatal("fixed planner was not claimed through the approved pool route", assignment, err)
	}
	f.bearer("POST", "/pool-runner/heartbeat", token, poolAssignmentBody(assignment), 200)
	result := h.result(assignment, h.output())
	body := poolAssignmentBody(assignment)
	body["result"] = result
	f.bearer("POST", "/pool-runner/results", token, body, 200)
	f.bearer("POST", "/pool-runner/results", token, body, 200)
	a = h.get(t, a.ID)
	if a.State != "ready" || a.Proposal == nil || a.OriginalOutput != result.Output || a.OriginalOutputSHA256 != digest(result.Output) {
		t.Fatal("verified pool result did not project the immutable proposal", a)
	}
	alerts, err := f.s.ListPlanningAlerts(ctx, f.actor, f.project, "", 20, "")
	if err != nil || alerts.Total != 1 || len(alerts.Items) != 1 || alerts.Items[0].AnalysisID != a.ID || alerts.Items[0].RunID != a.RunID {
		t.Fatal("pool replay duplicated or lost scoped model risk", alerts, err)
	}
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, a.RunID)
	if err != nil || run.State != "succeeded" {
		t.Fatal("pool execution fact did not persist", run, err)
	}

	// A different operation may queue a new analysis, but the machine cannot
	// replace the real creator's now-revoked credential with its own authority.
	h.request.IdempotencyKey = newID()
	queued := h.create(t)
	if _, err = f.s.DB.Exec(`UPDATE suma_credential_slots SET revoked=1 WHERE current_digest=?`, f.actor.credential.digest); err != nil {
		t.Fatal(err)
	}
	claim.OperationID = newID()
	if reply = f.bearer("POST", "/pool-runner/claim", token, claim, 200); reply["assignment"] != nil {
		t.Fatal("pool machine replaced the revoked planning creator", reply)
	}
	var attempt int
	if err = f.s.DB.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id='analyze'`, queued.RunID).Scan(&attempt); err != nil || attempt != 0 {
		t.Fatal("denied source consumed a planning attempt", attempt, err)
	}
}
