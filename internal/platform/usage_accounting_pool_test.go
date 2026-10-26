package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The real Pool HTTP/Service/Core contract consumes typed fixture evidence.
// No Runner process, model request, provider account or vendor billing runs.
func TestUsageAccountingPoolHTTPReservationQuotaAndResultReplay(t *testing.T) {
	p := newPoolMachineFixture(t)
	owner, ctx := p.f, context.Background()
	// This existing fixture has a second authorized organization/project. Use
	// its actual switched browser credential; the pool owner's identity never
	// becomes the billed project or the analysis creator.
	scope := p.scopes[1]
	principal, browserToken, err := owner.s.SwitchOrganization(ctx, owner.actor, map[string]any{"organization_id": scope.TenantID, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := owner.s.AuthenticateActor(ctx, browserToken)
	if err != nil {
		t.Fatal(err)
	}
	selected := *owner
	selected.user, selected.view, selected.actor, selected.project = principal, principal, actor, scope.ProjectID
	selected.cookie = &http.Cookie{Name: "suma_session", Value: browserToken}
	f := &selected
	if f.user.TenantID == owner.user.TenantID || f.project == owner.project {
		t.Fatal("fixture lacks an independent project tenant")
	}
	if err = f.s.MigrateUsageAccounting(ctx); err != nil {
		t.Fatal(err)
	}
	f.e.AttemptAccounting = f.s
	// Two physical admission slots in both layers leave room for another
	// attempt while the first is active; quota must be the denial cause.
	if err = f.e.SetResourceLimits(ctx, f.user.TenantID, f.project, map[string]int64{"execution_slots": 2}); err != nil {
		t.Fatal(err)
	}
	runner, token := p.credential.Runner, p.credential.Token
	profile := toolTestProfile()
	profile.ReadOnly = true
	profile.Planning = &tc.PlanningCapability{Protocol: tc.PlanningProtocol, Model: "pool-usage-protocol-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	f.bearer("POST", "/pool-runner/tool-inventory", token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}, 200)
	pr, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "Pool fixed-model usage fixture", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, f.actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: pr.Version, IdempotencyKey: newID()})
	if err != nil || !pr.Available {
		t.Fatal("pool profile is not currently eligible", pr, err)
	}
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Pool usage and allowance fixture"}))
	if err != nil {
		t.Fatal(err)
	}
	h := &intelligentFixture{f: f, profile: pr, task: task, request: PlanningAnalysisCreateRequest{ProjectID: f.project, Goal: "Return a bounded scheduling proposal", Locale: "en-US", TaskIDs: []string{task.ID}, ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}}
	rate := usageRateForTest(t, h)
	usagePolicyForTest(t, h, "organization", usageInt(4196), usageInt(8292))
	usagePolicyForTest(t, h, "project", usageInt(4196), usageInt(8292))
	first := h.create(t)
	h.request.IdempotencyKey = newID()
	second := h.create(t)
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.CommandID}}}}
	reply := f.bearer("POST", "/pool-runner/claim", token, claim, 200)
	var assignment dag.Assignment
	if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment); err != nil || assignment.TenantID != f.user.TenantID || assignment.ProjectID != f.project || assignment.PoolID != p.pool.ID || assignment.Attempt != 1 || assignment.PoolGrantID != p.grants[1].ID || assignment.RunID != first.RunID && assignment.RunID != second.RunID {
		t.Fatal("pool admission lost its actual project identity", assignment, err)
	}
	blockedID := second.RunID
	if assignment.RunID == second.RunID {
		blockedID = first.RunID
	}
	reserved := usageAttemptForTest(t, h, assignment.RunID)
	if reserved.State != "reserved" || reserved.Rate == nil || reserved.Rate.ID != rate.ID || reserved.ReservedTokens != 4196 || reserved.ReservedCostMicros == nil || *reserved.ReservedCostMicros != 8292 || reserved.HeldTokens != 4196 || reserved.HeldCostMicros == nil || *reserved.HeldCostMicros != 8292 {
		t.Fatal("real pool claim did not freeze the complete reservation", reserved)
	}
	var projectUsed, projectCapacity, poolUsed, poolCapacity, slots int
	if err = f.s.DB.QueryRow(`SELECT used,capacity FROM dag_resource_accounts WHERE tenant_id=? AND project_id=? AND name='execution_slots'`, f.user.TenantID, f.project).Scan(&projectUsed, &projectCapacity); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT used,capacity FROM dag_execution_pool_accounts WHERE pool_id=? AND name='execution_slots'`, p.pool.ID).Scan(&poolUsed, &poolCapacity); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_slots WHERE pool_id=? AND runner_id=?`, p.pool.ID, runner.ID).Scan(&slots); err != nil {
		t.Fatal(err)
	}
	if projectUsed != 1 || projectCapacity != 2 || poolUsed != 1 || poolCapacity != 2 || slots != 1 || runner.MaxSlots != 2 {
		t.Fatal("second slot would be denied by physical admission capacity", projectUsed, projectCapacity, poolUsed, poolCapacity, slots, runner.MaxSlots)
	}
	claim.SlotID, claim.OperationID = "1", newID()
	if reply = f.bearer("POST", "/pool-runner/claim", token, claim, 200); reply["assignment"] != nil {
		t.Fatal("full token/cost reservation admitted another pool attempt", reply)
	}
	blocked, err := f.e.GetRun(ctx, f.user.TenantID, f.project, blockedID)
	if err != nil || blocked.Nodes[0].Attempt != 0 || blocked.Nodes[0].State != "ready" || blocked.Nodes[0].Reason != "Model token or cost allowance is unavailable" {
		t.Fatal("quota denial was not a free attempt with an explicit reason", blocked, err)
	}
	current, err := f.s.GetToolProfile(ctx, f.actor, f.project, pr.ID)
	if err != nil || !current.Available {
		t.Fatal("denial was caused by unavailable inventory", current, err)
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 100, "")
	if err != nil || page.Total != 1 || len(page.Items) != 1 {
		t.Fatal("denied slot created a financial attempt", page, err)
	}
	f.bearer("POST", "/pool-runner/heartbeat", token, poolAssignmentBody(assignment), 200)
	result := usageMeasuredResult(h, assignment, 23, 17)
	completion := poolAssignmentBody(assignment)
	completion["result"] = result
	f.bearer("POST", "/pool-runner/results", token, completion, 200)
	f.bearer("POST", "/pool-runner/results", token, completion, 200)
	settled := usageAttemptForTest(t, h, assignment.RunID)
	if settled.State != "settled" || settled.ReportedTokens == nil || *settled.ReportedTokens != 40 || settled.EstimatedCostMicros == nil || *settled.EstimatedCostMicros != 57 || settled.HeldTokens != 0 || settled.HeldCostMicros == nil || *settled.HeldCostMicros != 0 || settled.Rate == nil || settled.Rate.ID != rate.ID {
		t.Fatal("pool final receipt/replay did not settle exactly once", settled)
	}
	dashboard, err := f.s.GetUsageDashboard(ctx, f.actor, f.project, "")
	if err != nil || len(dashboard.Balances) != 2 {
		t.Fatal(dashboard, err)
	}
	for _, balance := range dashboard.Balances {
		if balance.ReportedTokens == nil || *balance.ReportedTokens != 40 || balance.ReservedTokens == nil || *balance.ReservedTokens != 0 || balance.UnresolvedAttempts != 0 {
			t.Fatal("pool result replay changed scoped token balance", balance)
		}
		var cny *UsageMoneyBalance
		for i := range balance.Costs {
			if balance.Costs[i].Currency == "CNY" {
				cny = &balance.Costs[i]
			}
		}
		if cny == nil || cny.SpentMicros == nil || *cny.SpentMicros != 57 || cny.ReservedMicros == nil || *cny.ReservedMicros != 0 {
			t.Fatal("pool result replay changed scoped monetary balance", balance)
		}
	}
	var facts, foreignAttempts, ownerAccounts int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_facts WHERE tenant_id=? AND project_id=? AND attempt_id=? AND kind='result'`, f.user.TenantID, f.project, settled.ID).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_attempts WHERE tenant_id<>? OR project_id<>?`, f.user.TenantID, f.project).Scan(&foreignAttempts); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_usage_accounts WHERE tenant_id=?`, owner.user.TenantID).Scan(&ownerAccounts); err != nil {
		t.Fatal(err)
	}
	if facts != 1 || foreignAttempts != 0 || ownerAccounts != 0 {
		t.Fatal("pool owner was charged or final facts duplicated", facts, foreignAttempts, ownerAccounts)
	}
}
