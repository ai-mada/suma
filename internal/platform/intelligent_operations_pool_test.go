package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Exercise the real scoped Pool transport. The model result is an owned protocol
// fixture; this test does not make a model call or claim provider acceptance.
func TestIntelligentOperationsPoolManagedClaimCurrentProfileAndResult(t *testing.T) {
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
	runner, token := p.credential.Runner, p.credential.Token
	profile := toolTestProfile()
	profile.ReadOnly = true
	profile.IntelligentOperations = &tc.IntelligentOperationsCapability{Protocol: tc.IntelligentOperationsProtocol, Model: "owned-pool-operation-protocol-fixture", EndpointOrigin: "http://127.0.0.1:45678"}
	observe := func(available bool) {
		reason := ""
		if !available {
			reason = "Owned fixture temporarily reports the fixed model purpose unavailable"
		}
		f.bearer("POST", "/pool-runner/tool-inventory", token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: scope.TenantID, ProjectID: scope.ProjectID, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: available, Reason: reason}}}, 200)
	}
	observe(true)
	pr, err := f.s.CreateToolProfile(ctx, actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: runner.WorkerID, Name: "Scoped Pool local-draft planner", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: pr.Version, IdempotencyKey: newID()})
	if err != nil || !pr.Available {
		t.Fatal(pr, err)
	}
	analysis, err := f.s.CreateIntelligentOperation(ctx, f.e, actor, f.project, IntelligentOperationCreateRequest{ProjectID: f.project, Goal: "Prepare reviewable requirement and design drafts", Locale: "en-US", Sources: []ProjectKnowledgeSourceInput{}, ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, AcknowledgeModelUse: true, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	claim := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: runner.ID, WorkerID: runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: scope.TenantID, ProjectID: scope.ProjectID, Capabilities: []string{"command:" + profile.CommandID}}}}
	// Recognizing this managed family must still recheck the current observed
	// profile; it must not bypass the remaining Pool/tool admission chain.
	observe(false)
	reply := f.bearer("POST", "/pool-runner/claim", token, claim, 200)
	if reply["assignment"] != nil {
		t.Fatal("stale profile availability admitted a Pool model operation", reply)
	}
	observe(true)
	claim.OperationID = newID()
	reply = f.bearer("POST", "/pool-runner/claim", token, claim, 200)
	var assignment dag.Assignment
	if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment); err != nil || assignment.RunID != analysis.RunID || assignment.NodeID != "analyze" || assignment.TenantID != scope.TenantID || assignment.ProjectID != scope.ProjectID || assignment.PoolGrantID != p.grants[1].ID || assignment.Config["purpose"] != "intelligent_operations" || assignment.Attempt != 1 {
		t.Fatal("Pool managed claim lost its fixed plan, profile or scope", reply, err)
	}
	result := dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, WorkerID: assignment.WorkerID, Attempt: assignment.Attempt, Fence: assignment.Fence, OperationID: newID(), Output: stringify(operationTestOutput()), ToolEvidence: &tc.Evidence{Profile: profile, Terminal: "completed", EventCount: 1, UsageState: tc.UsageUnreported}}
	body := poolAssignmentBody(assignment)
	body["result"] = result
	f.bearer("POST", "/pool-runner/results", token, body, 200)
	analysis, err = f.s.GetIntelligentOperation(ctx, actor, f.project, analysis.ID)
	if err != nil || analysis.State != "ready" || analysis.Proposal == nil || !analysis.BasisCurrent || len(analysis.Results) != 0 || analysis.OriginalOutputSHA256 != digest(result.Output) {
		t.Fatal("Pool result was lost or adopted without user review", analysis, err)
	}
}
