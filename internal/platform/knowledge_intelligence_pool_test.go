package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This uses the real Service/Core/Pool HTTP path with typed final usage. It
// runs no model process and makes no provider or vendor-billing request.
func TestKnowledgeIntelligencePoolSourceDriftAndScopedUsageReplay(t *testing.T) {
	p := newPoolMachineFixture(t)
	owner, ctx := p.f, context.Background()
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
	selected.actor, selected.user, selected.view, selected.project = actor, principal, principal, scope.ProjectID
	selected.cookie = &http.Cookie{Name: "suma_session", Value: browserToken}
	f := &selected
	if actor.credential.tenantID == owner.actor.credential.tenantID || f.project == owner.project {
		t.Fatal("pool fixture does not have a distinct billed project tenant")
	}
	f.e.AttemptAccounting = f.s
	runner, token := p.credential.Runner, p.credential.Token
	profile := toolTestProfile()
	profile.ReadOnly = true
	profile.KnowledgeExtraction = &tc.KnowledgeExtractionCapability{Protocol: tc.KnowledgeExtractionProtocol, Model: "pool-knowledge-protocol-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	f.bearer("POST", "/pool-runner/tool-inventory", token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}, 200)
	pr, err := f.s.CreateToolProfile(ctx, actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "Independent tenant knowledge extraction", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: pr.Version, IdempotencyKey: newID()})
	if err != nil || !pr.Available {
		t.Fatal("project consent or observed dedicated capability is missing", pr, err)
	}
	doc, err := f.s.Create(ctx, actor, f.project, "documents", f.body(map[string]any{"title": "Pool extraction source", "description": "Review the rollback plan before authorizing migration."}))
	if err != nil {
		t.Fatal(err)
	}
	usage := &intelligentFixture{f: f, profile: pr}
	rate := usageRateForTest(t, usage)
	usagePolicyForTest(t, usage, "organization", usageInt(4196), usageInt(8292))
	usagePolicyForTest(t, usage, "project", usageInt(4196), usageInt(8292))
	h := &intelligenceFixture{f: f, profile: pr, doc: doc, request: KnowledgeIntelligenceCreateRequest{ProjectID: f.project, Kind: "skill", Goal: "Extract a reviewable migration skill from the fixed source", Locale: "en-US", Sources: []ProjectKnowledgeSourceInput{{Kind: "documents", ID: doc.ID, Version: doc.Version}}, ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}}
	analysis := h.create(t)
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.CommandID}}}}
	reply := f.bearer("POST", "/pool-runner/claim", token, claim, 200)
	var assignment dag.Assignment
	if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment); err != nil || assignment.RunID != analysis.RunID || assignment.NodeID != "extract" || assignment.TenantID != f.user.TenantID || assignment.ProjectID != f.project || assignment.PoolGrantID != p.grants[1].ID || assignment.Config["purpose"] != "knowledge_extraction" || assignment.Attempt != 1 {
		t.Fatal("pool claim lost extraction purpose or billed tenant identity", assignment, err)
	}
	reserved := usageAttemptForTest(t, usage, analysis.RunID)
	if reserved.State != "reserved" || reserved.Rate == nil || reserved.Rate.ID != rate.ID || reserved.Model != profile.KnowledgeExtraction.Model || reserved.ReservedTokens != 4196 || reserved.HeldTokens != 4196 || reserved.ReservedCostMicros == nil || *reserved.ReservedCostMicros != 8292 {
		t.Fatal("knowledge extraction bypassed fixed pricing or allowance reservations", reserved)
	}
	dashboard, err := f.s.GetUsageDashboard(ctx, actor, f.project, "")
	if err != nil || len(dashboard.Balances) != 2 {
		t.Fatal(dashboard, err)
	}
	for _, balance := range dashboard.Balances {
		if balance.ReservedTokens == nil || *balance.ReservedTokens != 4196 || balance.ReportedTokens == nil || *balance.ReportedTokens != 0 || balance.UnresolvedAttempts != 1 {
			t.Fatal("both project and organization must hold this attempt", balance)
		}
	}

	// A new formal source version cannot rewrite the already-started request.
	f.call("PATCH", "/documents/"+doc.ID, f.body(map[string]any{"expected_version": doc.Version, "description": "A newer migration policy now requires additional independent review."}), 200)
	f.bearer("POST", "/pool-runner/heartbeat", token, poolAssignmentBody(assignment), 200)
	result := h.result(assignment, h.output())
	result.ToolEvidence.Usage = &tc.Usage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}
	result.ToolEvidence.UsageState = tc.UsageComplete
	body := poolAssignmentBody(assignment)
	body["result"] = result
	f.bearer("POST", "/pool-runner/results", token, body, 200)
	f.bearer("POST", "/pool-runner/results", token, body, 200)
	analysis = h.get(t, analysis.ID)
	if analysis.State != "ready" || analysis.Proposal == nil || analysis.BasisCurrent || analysis.OriginalOutput != result.Output || analysis.OriginalOutputSHA256 != digest(result.Output) || skillHasAction(analysis.AllowedActions, "adopt") {
		t.Fatal("source drift erased actual model output or authorized stale adoption", analysis)
	}
	if _, err = f.s.CommandKnowledgeIntelligence(ctx, actor, f.project, analysis.ID, h.command(analysis, "adopt")); err == nil {
		t.Fatal("old fixed source was adopted after its formal version changed")
	}
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, analysis.RunID)
	if err != nil || run.State != "succeeded" {
		t.Fatal("source drift lost the actual completed execution fact", run, err)
	}
	settled := usageAttemptForTest(t, usage, analysis.RunID)
	if settled.State != "settled" || settled.ReportedTokens == nil || *settled.ReportedTokens != 40 || settled.EstimatedCostMicros == nil || *settled.EstimatedCostMicros != 57 || settled.HeldTokens != 0 || settled.HeldCostMicros == nil || *settled.HeldCostMicros != 0 || settled.Rate == nil || settled.Rate.ID != rate.ID {
		t.Fatal("source drift/replay corrupted the frozen actual usage settlement", settled)
	}
	dashboard, err = f.s.GetUsageDashboard(ctx, actor, f.project, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, balance := range dashboard.Balances {
		if balance.ReportedTokens == nil || *balance.ReportedTokens != 40 || balance.ReservedTokens == nil || *balance.ReservedTokens != 0 || balance.UnresolvedAttempts != 0 || len(balance.Costs) != 1 || balance.Costs[0].Currency != "CNY" || balance.Costs[0].SpentMicros == nil || *balance.Costs[0].SpentMicros != 57 || balance.Costs[0].ReservedMicros == nil || *balance.Costs[0].ReservedMicros != 0 {
			t.Fatal("project/organization accounting was not settled once in its original currency", balance)
		}
	}
	for _, check := range []struct {
		query string
		args  []any
		want  int
	}{
		{`SELECT COUNT(*) FROM suma_usage_facts WHERE tenant_id=? AND project_id=? AND kind='result'`, []any{f.user.TenantID, f.project}, 1},
		{`SELECT COUNT(*) FROM suma_knowledge_intelligence_outcomes WHERE tenant_id=? AND project_id=?`, []any{f.user.TenantID, f.project}, 1},
		{`SELECT COUNT(*) FROM suma_usage_attempts WHERE tenant_id=?`, []any{owner.user.TenantID}, 0},
		{`SELECT COUNT(*) FROM suma_usage_accounts WHERE tenant_id=?`, []any{owner.user.TenantID}, 0},
		{`SELECT COUNT(*) FROM suma_project_knowledge WHERE tenant_id=? AND project_id=?`, []any{f.user.TenantID, f.project}, 0},
	} {
		var count int
		if err = f.s.DB.QueryRow(check.query, check.args...).Scan(&count); err != nil || count != check.want {
			t.Fatal("immutable ledger/tenant scope/stale adoption assertion failed", count, check.want, err)
		}
	}
}
