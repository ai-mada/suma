package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	ec "github.com/ai-mada/suma/internal/executioncontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func repairModelFixture(t *testing.T) (*compositionFixture, CandidateCompositionPlan, CandidateCompositionRepairCreateRequest, ToolProfile, string, []mc.Deployment) {
	t.Helper()
	h, original := compositionConflictFixture(t, true)
	f := h.f
	req, profile, token := compositionRepairRequestFixture(t, h, original)
	profile.Profile.ModelExecutionProtocol = mc.Protocol
	profile.Profile.ProfileSHA256 = strings.Repeat("c", 64)
	if _, err := f.s.DB.Exec(`UPDATE suma_tool_profiles SET profile=? WHERE id=?`, stringify(profile.Profile), profile.ID); err != nil {
		t.Fatal(err)
	}
	deployments := []mc.Deployment{}
	for _, id := range []string{"repair-model-one", "repair-model-two"} {
		d := mc.Deployment{ID: id, Revision: 1, Protocol: mc.ProviderProtocol, Model: id, EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("d", 64), ContextWindow: 32000}
		d.SHA256 = mc.DeploymentDigest(d)
		deployments = append(deployments, d)
	}
	reportRepairModelInventory(t, h, profile, token, deployments)
	f.e.AttemptAccounting = f.s
	return h, original, req, profile, token, deployments
}

func reportRepairModelInventory(t *testing.T, h *compositionFixture, profile ToolProfile, token string, deployments []mc.Deployment) {
	t.Helper()
	f := h.f
	_, err := f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true, ModelDeployments: deployments}}})
	if err != nil {
		t.Fatal(err)
	}
}

func repairModelSelection(d mc.Deployment) mc.Selection {
	return mc.Selection{DeploymentID: d.ID, DeploymentRevision: d.Revision, DeploymentSHA256: d.SHA256, Parameters: mc.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, Budget: &mc.Budget{TokenLimit: usageInt(65000)}}
}

// Decode through the public request so this also proves the new field is not
// silently dropped at the service boundary.
func repairModelRequest(t *testing.T, req CandidateCompositionRepairCreateRequest, selection mc.Selection) CandidateCompositionRepairCreateRequest {
	t.Helper()
	var fields map[string]any
	if json.Unmarshal([]byte(stringify(req)), &fields) != nil {
		t.Fatal("request encoding")
	}
	fields["model_execution_selection"] = selection
	if err := json.Unmarshal([]byte(stringify(fields)), &req); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestCandidateRepairModelRequiresExplicitSelectionWithoutCreatingTask(t *testing.T) {
	h, original, req, _, _, _ := repairModelFixture(t)
	f := h.f
	tasks := expertCount(t, f, "suma_tasks")
	_, err := f.s.CreateCandidateCompositionRepair(context.Background(), f.e, f.actor, f.project, original.ID, req)
	if err == nil {
		t.Fatal("native repair accepted a missing model selection")
	}
	expertStatus(t, err, 400)
	if expertCount(t, f, "suma_tasks") != tasks || expertCount(t, f, "suma_candidate_composition_repair_sources") != 0 {
		t.Fatal("invalid selection reserved the conflict or created a task")
	}
}

func TestCandidateRepairModelFrozenBindingQuoteAndClaimReservation(t *testing.T) {
	h, original, req, profile, token, deployments := repairModelFixture(t)
	f, ctx := h.f, context.Background()
	rate := modelRateForTest(t, f, profile, deployments[0], 0)
	req = repairModelRequest(t, req, repairModelSelection(deployments[0]))
	plan, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	_ = json.Unmarshal([]byte(stringify(plan)), &payload)
	b, err := mc.DecodeBinding(payload["model_execution"])
	if err != nil || b.Deployment != deployments[0] || b.Rate == nil || b.Rate.ID != rate.ID || b.Parameters.MaxSteps != 2 || b.InputTokenReservation != 64000 {
		t.Fatal("repair did not freeze the explicit model and quote", payload["model_execution"], err)
	}
	modelRateForTest(t, f, profile, deployments[1], rate.Version)
	read, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, plan.ID)
	if err != nil || !read.Valid {
		t.Fatal(read.Blockers, err)
	}
	started := h.start(t, plan)
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &assignment) != nil || assignment.RunID != started.RunID {
		t.Fatal("repair was not admitted", claim)
	}
	fixed, err := dag.ModelBinding(dag.Node{ID: assignment.NodeID, Type: "agent", Command: assignment.Command, Config: assignment.Config})
	if err != nil || stringify(fixed) != stringify(b) {
		t.Fatal("claimed repair model differs from preview", fixed, err)
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 100, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ReservedTokens != 64100 || page.Items[0].Rate == nil || page.Items[0].Rate.ID != rate.ID {
		t.Fatal("repair bypassed native accounting or repriced", page, err)
	}
}

func TestCandidateRepairModelRejectsChangedRunBindingAndCurrentInventory(t *testing.T) {
	h, original, req, profile, token, deployments := repairModelFixture(t)
	f, ctx := h.f, context.Background()
	req = repairModelRequest(t, req, repairModelSelection(deployments[0]))
	plan, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	started := h.start(t, plan)
	var raw string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_nodes WHERE run_id=? AND id=?`, started.RunID, plan.ProducerNodeID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var node dag.Node
	if err = json.Unmarshal([]byte(raw), &node); err != nil {
		t.Fatal(err)
	}
	replacement, err := mc.NewBinding(deployments[1], mc.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	node.Config["model_execution"] = candidateJSON(t, replacement)
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET spec=? WHERE run_id=? AND id=?`, stringify(node), started.RunID, plan.ProducerNodeID); err != nil {
		t.Fatal(err)
	}
	bounded, tx, close, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.checkCandidateCompositionRepairExecutionTx(bounded, tx, f.user.TenantID, f.project, started.RunID, plan.ProducerNodeID, true)
	close()
	if err == nil {
		t.Fatal("fresh repair accepted a different model than its frozen plan")
	}
	expertStatus(t, err, 409)
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET spec=? WHERE run_id=? AND id=?`, raw, started.RunID, plan.ProducerNodeID); err != nil {
		t.Fatal(err)
	}
	reportRepairModelInventory(t, h, profile, token, deployments[1:])
	f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}}), 409)
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 100, "")
	if err != nil || len(page.Items) != 0 {
		t.Fatal("blocked repair created a model reservation", page, err)
	}
}

func TestCandidateRepairModelReadyAndCertifiedStartupRevision(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "certified_startup_failure"}[started], func(t *testing.T) {
			h, original, req, profile, _, deployments := repairModelFixture(t)
			f, ctx := h.f, context.Background()
			req = repairModelRequest(t, req, repairModelSelection(deployments[0]))
			parent, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
			if err != nil {
				t.Fatal(err)
			}
			if started {
				parent = h.start(t, parent)
				a, e := f.e.Claim(ctx, f.user.TenantID, f.project, profile.WorkerID, []string{"command:" + profile.Profile.CommandID})
				if e != nil || a == nil {
					t.Fatal(a, e)
				}
				proof := &ec.StartupFailureProof{Protocol: ec.StartupFailureProtocol, Stage: "before_workspace", Assignment: cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}, Profile: profile.Profile, Purpose: "candidate_conflict_repair"}
				if e = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: -1, Error: "Owned root unavailable before workspace", OperationID: newID(), StartupFailure: proof}); e != nil {
					t.Fatal(e)
				}
			}
			var old string
			if err = f.s.DB.QueryRow(`SELECT payload FROM suma_candidate_compositions WHERE id=?`, parent.ID).Scan(&old); err != nil {
				t.Fatal(err)
			}
			view, err := f.s.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, parent.ID)
			if err != nil || !view.CanRevise || view.ConflictBasis == nil {
				t.Fatal(view, err)
			}
			r := CandidateCompositionRepairRevisionRequest{ProjectID: f.project, ExpectedVersion: parent.Version, ExpectedTaskVersion: view.TaskVersion, ExpectedConflictSHA256: view.ConflictBasis.SHA256, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, ToolProfileID: req.ToolProfileID, ExpectedToolVersion: req.ExpectedToolVersion, Reason: "Explicitly select the reviewed replacement model", Acknowledge: true, IdempotencyKey: newID()}
			if view.RecoveryBasis != nil {
				r.ExpectedRecoverySHA256 = view.RecoveryBasis.SHA256
			}
			tasks, runs := expertCount(t, f, "suma_tasks"), expertCount(t, f, "dag_runs")
			_, err = f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, parent.ID, r)
			expertStatus(t, err, 400)
			choice := repairModelSelection(deployments[1])
			choice.Parameters.MaxOutputTokens = 200
			r.ModelExecutionSelection = &choice
			r.IdempotencyKey = newID()
			child, err := f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, parent.ID, r)
			if err != nil || child.ModelExecution == nil || child.ModelExecution.Deployment != deployments[1] || child.ModelExecution.Parameters.MaxOutputTokens != 200 || child.TaskID != parent.TaskID || child.RunID != "" || child.RepairRevision == nil || child.RepairRevision.DefinitionSHA256 != repairDefinitionDigest(child) {
				t.Fatal("revision lost explicit fixed selection or lineage", child, err)
			}
			var after string
			if err = f.s.DB.QueryRow(`SELECT payload FROM suma_candidate_compositions WHERE id=?`, parent.ID).Scan(&after); err != nil || old != after || expertCount(t, f, "suma_tasks") != tasks || expertCount(t, f, "dag_runs") != runs {
				t.Fatal("revision rewrote history or started work", err)
			}
			startedChild := h.start(t, child)
			if startedChild.RunID == parent.RunID || stringify(startedChild.ModelExecution) != stringify(child.ModelExecution) {
				t.Fatal("replacement Start changed its model", startedChild)
			}
			replay, err := f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, parent.ID, r)
			if err != nil || replay.ID != child.ID || replay.RunID != "" || stringify(replay.ModelExecution) != stringify(child.ModelExecution) {
				t.Fatal("revision receipt no longer restores the exact original preview", replay, err)
			}
		})
	}
}

func TestCandidateRepairModelExistingConflictCannotDropAnotherSelection(t *testing.T) {
	h, original, req, _, _, deployments := repairModelFixture(t)
	f, ctx := h.f, context.Background()
	req = repairModelRequest(t, req, repairModelSelection(deployments[0]))
	first, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	req.IdempotencyKey = newID()
	recovered, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil || recovered.ID != first.ID || stringify(recovered.ModelExecution) != stringify(first.ModelExecution) {
		t.Fatal("same explicit selection lost conflict reservation recovery", recovered, err)
	}
	req = repairModelRequest(t, req, repairModelSelection(deployments[1]))
	req.IdempotencyKey = newID()
	_, err = f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err == nil {
		t.Fatal("existing conflict silently discarded another explicit model selection")
	}
	expertStatus(t, err, 409)
	if expertCount(t, f, "suma_candidate_composition_repair_sources") != 1 {
		t.Fatal("selection mismatch replaced the conflict reservation")
	}
}

func TestCandidateRepairModelCreateReceiptPreservesOriginalPreviewAfterStart(t *testing.T) {
	h, original, req, profile, _, deployments := repairModelFixture(t)
	f, ctx := h.f, context.Background()
	rate := modelRateForTest(t, f, profile, deployments[0], 0)
	req = repairModelRequest(t, req, repairModelSelection(deployments[0]))
	created, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil || created.ModelExecution == nil || created.RunID != "" {
		t.Fatal(created, err)
	}
	// Another view starts the plan before the lost Create response is recovered.
	started := h.start(t, created)
	modelRateForTest(t, f, profile, deployments[1], rate.Version)
	recovered, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil || recovered.ID != created.ID || recovered.Version != created.Version || recovered.RunID != "" || recovered.State != created.State || stringify(recovered.ModelExecution) != stringify(created.ModelExecution) || stringify(recovered.Workflow) != stringify(created.Workflow) {
		t.Fatal("Create receipt adopted the current run or execution inputs", recovered, err)
	}
	if recovered.Valid || len(recovered.AllowedActions) != 0 || len(recovered.Blockers) == 0 {
		t.Fatal("historical Create receipt advertised fresh plan actions", recovered)
	}
	current, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, created.ID)
	if err != nil || current.Version != started.Version || current.RunID != started.RunID || current.RunID == "" {
		t.Fatal("current GET no longer exposes the current run", current, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,'viewer') ON CONFLICT(project_id,user_id) DO UPDATE SET role='viewer'`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	expertStatus(t, err, 403)
}
