package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Real Service/Core/Pool HTTP transactions with authenticated typed receipts;
// no model process, provider request or vendor billing is simulated as inference.
func TestFailureDiagnosisPoolSourceDriftAndScopedUsageReplay(t *testing.T) {
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
		t.Fatal("fixture must bill a distinct project tenant, not the pool owner")
	}
	runner, token := p.credential.Runner, p.credential.Token

	// Establish the source through real browser start, Pool claim and known
	// failed completion. Do not manufacture a failed node or issue with SQL.
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Pool failed engineering task", "description": "Retain the exact failed assertion and require independent regression", "acceptance_criteria": []string{"Exercise the regression"}}), 201)
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Pool known failure source", "nodes": []dag.Node{{ID: "test", Type: "test", Command: []string{"fixture-failure"}, MaxRetries: 0}}}), 201)
	sourceRun := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": task["id"], "expected_version": task["version"]}), 201)
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:fixture-failure"}}}}
	reply := f.bearer("POST", "/pool-runner/claim", token, claim, 200)
	var failed dag.Assignment
	if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &failed); err != nil || failed.RunID != sourceRun["id"] || failed.NodeID != "test" || failed.Attempt != 1 || failed.PoolGrantID != p.grants[1].ID {
		t.Fatal("source failure lost its exact cross-tenant Pool assignment", failed, err)
	}
	sourceBody := poolAssignmentBody(failed)
	sourceBody["result"] = dag.Result{RunID: failed.RunID, NodeID: failed.NodeID, WorkerID: failed.WorkerID, Attempt: failed.Attempt, Fence: failed.Fence, ExitCode: 1, Error: "Expected 42, observed 0", Output: "Actual failed assertion: expected nonzero boundary result", OperationID: newID()}
	f.bearer("POST", "/pool-runner/results", token, sourceBody, 200)
	if err = f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	view, err := f.s.FailureDiagnosisContext(ctx, f.e, actor, f.project, failed.RunID, failed.NodeID, failed.Attempt)
	if err != nil || !view.Eligible || !view.CanCreate || view.Source == nil || view.Issue == nil || view.Source.NodeState != "failed" || view.Source.Fence != failed.Fence || view.Source.TaskID != task["id"] {
		t.Fatal("known failed node did not produce the authoritative issue/source", view, err)
	}

	profile := toolTestProfile()
	profile.ReadOnly = true
	profile.FailureDiagnosis = &tc.FailureDiagnosisCapability{Protocol: tc.FailureDiagnosisProtocol, Model: "pool-diagnosis-protocol-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	f.bearer("POST", "/pool-runner/tool-inventory", token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}, 200)
	pr, err := f.s.CreateToolProfile(ctx, actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "Independent tenant failure diagnosis", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: pr.Version, IdempotencyKey: newID()})
	if err != nil || !pr.Available {
		t.Fatal("dedicated Pool inventory and project consent are required", pr, err)
	}
	f.e.AttemptAccounting = f.s
	usage := &intelligentFixture{f: f, profile: pr}
	rate := usageRateForTest(t, usage)
	usagePolicyForTest(t, usage, "organization", usageInt(4196), usageInt(8292))
	usagePolicyForTest(t, usage, "project", usageInt(4196), usageInt(8292))
	h := &diagnosisFixture{f: f, profile: pr, context: view}
	h.request = FailureDiagnosisCreateRequest{ProjectID: f.project, IssueID: view.Issue.ID, ExpectedIssueVersion: view.Issue.Version, RunID: failed.RunID, NodeID: failed.NodeID, Attempt: failed.Attempt, ExpectedSourceSHA256: view.Source.SHA256, Goal: "Explain this fixed failure with a reviewable repair and independent regression", Locale: "en-US", ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()}
	h.output = stringify(FailureDiagnosisOutput{SchemaVersion: 1, Summary: "The fixed assertion failed; its cause still requires confirmation", Observations: []FailureDiagnosisObservation{{Statement: "Expected and observed values differ", EvidenceIndices: []int{len(view.Source.Evidence) - 2}}}, Hypotheses: []FailureDiagnosisHypothesis{{Statement: "A boundary may return zero", EvidenceIndices: []int{len(view.Source.Evidence) - 1}, Confidence: "medium"}}, RecommendedActions: []string{"Review the fixed evidence before changing the implementation"}, TestPlan: []string{"Add an independent boundary regression"}, Repair: FailureRepairDraft{Title: "Investigate the calculation boundary", Description: "Confirm the proposed cause and implement the reviewed correction.", Priority: "high", AcceptanceCriteria: []string{"The regression passes independently"}}})
	var analysis FailureDiagnosisAnalysis
	created := f.call("POST", "/failure-diagnosis/analyses", h.request, 201)
	if err = json.Unmarshal([]byte(stringify(created)), &analysis); err != nil || analysis.ID == "" || analysis.Source.SHA256 != view.Source.SHA256 {
		t.Fatal(created, err)
	}
	claim.OperationID = newID()
	claim.Scopes[0].Capabilities = []string{"command:" + profile.CommandID}
	reply = f.bearer("POST", "/pool-runner/claim", token, claim, 200)
	var assignment dag.Assignment
	if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment); err != nil || assignment.RunID != analysis.RunID || assignment.NodeID != "diagnose" || assignment.TenantID != f.user.TenantID || assignment.ProjectID != f.project || assignment.PoolGrantID != p.grants[1].ID || assignment.Config["purpose"] != "failure_diagnosis" || assignment.Attempt != 1 {
		t.Fatal("Pool claim lost diagnosis purpose or original billed scope", assignment, err)
	}
	reserved := usageAttemptForTest(t, usage, analysis.RunID)
	if reserved.State != "reserved" || reserved.Rate == nil || reserved.Rate.ID != rate.ID || reserved.Model != profile.FailureDiagnosis.Model || reserved.ReservedTokens != 4196 || reserved.HeldTokens != 4196 || reserved.ReservedCostMicros == nil || *reserved.ReservedCostMicros != 8292 {
		t.Fatal("diagnosis bypassed fixed pricing or atomic reservations", reserved)
	}
	dashboard, err := f.s.GetUsageDashboard(ctx, actor, f.project, "")
	if err != nil || len(dashboard.Balances) != 2 {
		t.Fatal(dashboard, err)
	}
	for _, balance := range dashboard.Balances {
		if balance.ReservedTokens == nil || *balance.ReservedTokens != 4196 || balance.ReportedTokens == nil || *balance.ReportedTokens != 0 || balance.UnresolvedAttempts != 1 {
			t.Fatal("both accounts must hold the original diagnosis attempt", balance)
		}
	}

	// Source freshness gates adoption/new work, not the already-authorized
	// model request or genuine final facts. Heartbeat must retain that request.
	fixedGoal := stringify(assignment.Config["goal"])
	f.call("PATCH", "/issues/"+view.Issue.ID, f.body(map[string]any{"expected_version": view.Issue.Version, "description": "A human corrected this issue after the diagnostic model request was granted."}), 200)
	f.bearer("POST", "/pool-runner/heartbeat", token, poolAssignmentBody(assignment), 200)
	var storedSpec string
	var spec dag.Node
	err = f.s.DB.QueryRow(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.run_id=? AND n.id=?`, f.user.TenantID, f.project, assignment.RunID, assignment.NodeID).Scan(&storedSpec)
	if err != nil || json.Unmarshal([]byte(storedSpec), &spec) != nil || stringify(spec.Config["goal"]) != fixedGoal {
		t.Fatal("heartbeat rewrote the authorized fixed request", err)
	}
	result := h.result(assignment, h.output)
	result.ToolEvidence.Usage = &tc.Usage{InputTokens: usageInt(23), OutputTokens: usageInt(17)}
	result.ToolEvidence.UsageState = tc.UsageComplete
	body := poolAssignmentBody(assignment)
	body["result"] = result
	f.bearer("POST", "/pool-runner/results", token, body, 200)
	f.bearer("POST", "/pool-runner/results", token, body, 200)
	analysis = h.get(t, analysis.ID)
	if analysis.State != "ready" || analysis.Proposal == nil || analysis.Repair == nil || analysis.BasisCurrent || analysis.OriginalOutput != h.output || analysis.OriginalOutputSHA256 != digest(h.output) || analysis.Source.SHA256 != view.Source.SHA256 || skillHasAction(analysis.AllowedActions, "adopt") {
		t.Fatal("source drift erased occurred diagnosis facts or authorized stale adoption", analysis)
	}
	_, err = f.s.CommandFailureDiagnosis(ctx, f.e, actor, f.project, analysis.ID, h.command(analysis, "adopt"))
	expertStatus(t, err, 409)
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, analysis.RunID)
	if err != nil || run.State != "succeeded" {
		t.Fatal("known completion was discarded after source drift", run, err)
	}
	settled := usageAttemptForTest(t, usage, analysis.RunID)
	if settled.State != "settled" || settled.ReportedTokens == nil || *settled.ReportedTokens != 40 || settled.EstimatedCostMicros == nil || *settled.EstimatedCostMicros != 57 || settled.HeldTokens != 0 || settled.HeldCostMicros == nil || *settled.HeldCostMicros != 0 || settled.Rate == nil || settled.Rate.ID != rate.ID {
		t.Fatal("source drift/replayed ACK corrupted the original single settlement", settled)
	}
	dashboard, err = f.s.GetUsageDashboard(ctx, actor, f.project, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, balance := range dashboard.Balances {
		if balance.ReportedTokens == nil || *balance.ReportedTokens != 40 || balance.ReservedTokens == nil || *balance.ReservedTokens != 0 || balance.UnresolvedAttempts != 0 || len(balance.Costs) != 1 || balance.Costs[0].Currency != "CNY" || balance.Costs[0].SpentMicros == nil || *balance.Costs[0].SpentMicros != 57 || balance.Costs[0].ReservedMicros == nil || *balance.Costs[0].ReservedMicros != 0 {
			t.Fatal("both scoped accounts must settle once and release unused holds", balance)
		}
	}
	for _, check := range []struct {
		query string
		args  []any
		want  int
	}{
		{`SELECT COUNT(*) FROM suma_usage_facts WHERE tenant_id=? AND project_id=? AND attempt_id=? AND kind='result'`, []any{f.user.TenantID, f.project, settled.ID}, 1},
		{`SELECT COUNT(*) FROM suma_failure_diagnosis_executions WHERE tenant_id=? AND project_id=?`, []any{f.user.TenantID, f.project}, 1},
		{`SELECT COUNT(*) FROM suma_usage_attempts WHERE tenant_id=?`, []any{owner.user.TenantID}, 0},
		{`SELECT COUNT(*) FROM suma_usage_accounts WHERE tenant_id=?`, []any{owner.user.TenantID}, 0},
		{`SELECT COUNT(*) FROM suma_failure_diagnosis_adoptions WHERE tenant_id=? AND project_id=?`, []any{f.user.TenantID, f.project}, 0},
		{`SELECT COUNT(*) FROM suma_requirements WHERE tenant_id=? AND project_id=?`, []any{f.user.TenantID, f.project}, 0},
	} {
		var count int
		if err = f.s.DB.QueryRow(check.query, check.args...).Scan(&count); err != nil || count != check.want {
			t.Fatal("single immutable receipt, tenant scope or stale-adoption assertion failed", count, check.want, err)
		}
	}
}
