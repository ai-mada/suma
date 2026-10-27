package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func newExpertModelFixture(t *testing.T) (*expertFixture, ToolProfile, []mc.Deployment) {
	t.Helper()
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	legacy, err := f.s.GetToolProfile(ctx, f.actor, f.project, h.role.ToolProfileID)
	if err != nil {
		t.Fatal(err)
	}
	profile := legacy.Profile
	profile.ModelExecutionProtocol, profile.WorkspaceToolsVersion, profile.ProfileSHA256 = mc.Protocol, "attempt-workspace-v1", strings.Repeat("c", 64)
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: legacy.WorkerID, Name: "Explicit expert native model", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	deployments := []mc.Deployment{}
	for _, id := range []string{"expert-model-a", "expert-model-b"} {
		d := mc.Deployment{ID: id, Revision: 1, Protocol: mc.ProviderProtocol, Model: id, EndpointOrigin: "https://models.example", EndpointConfigSHA256: strings.Repeat("d", 64), ContextWindow: 32000}
		d.SHA256 = mc.DeploymentDigest(d)
		deployments = append(deployments, d)
	}
	reportExpertModelInventory(t, h, p, deployments)
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.role.ToolProfileID, h.role.ExpectedToolVersion = p.ID, p.Version
	h.configuration, err = f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Action: "publish", Reason: "Approve explicit native tool; model remains per task", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	f.e.AttemptAccounting = f.s
	return h, p, deployments
}

func reportExpertModelInventory(t *testing.T, h *expertFixture, p ToolProfile, deployments []mc.Deployment) {
	t.Helper()
	f := h.f
	_, err := f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, h.runnerToken), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true, ModelDeployments: deployments}}})
	if err != nil {
		t.Fatal(err)
	}
}
func expertModelSelections(deployments []mc.Deployment) []*mc.Selection {
	return []*mc.Selection{
		{DeploymentID: deployments[0].ID, DeploymentRevision: deployments[0].Revision, DeploymentSHA256: deployments[0].SHA256, Parameters: mc.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, Budget: &mc.Budget{TokenLimit: usageInt(65000), CostLimitMicros: usageInt(70000), Currency: "CNY"}},
		{DeploymentID: deployments[1].ID, DeploymentRevision: deployments[1].Revision, DeploymentSHA256: deployments[1].SHA256, Parameters: mc.Parameters{MaxSteps: 3, MaxOutputTokens: 200}, Budget: &mc.Budget{TokenLimit: usageInt(100000)}},
	}
}
func expertModelPlanRequest(h *expertFixture, selections []*mc.Selection) ExpertTeamPlanCreateRequest {
	req := h.request()
	for i := range req.Items {
		req.Items[i].ModelExecutionSelection = selections[i]
	}
	return req
}

func TestExpertTeamModelSelectionsFreezePerItemAndReuseOriginalRate(t *testing.T) {
	h, p, deployments := newExpertModelFixture(t)
	f, ctx := h.f, context.Background()
	capacity := schedulingPreview(f, 2, nil)
	f.call("POST", "/scheduling/policy-previews/"+capacity["id"].(string)+"/commit", f.body(map[string]any{"expected_version": 1}), 200)
	rate := modelRateForTest(t, f, p, deployments[0], 0)
	request := expertModelPlanRequest(h, expertModelSelections(deployments))
	preview, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	if expertCount(t, f, "dag_runs") != 0 || preview.ConfigurationVersion != h.configuration.Version || preview.Configuration.Version != h.configuration.Version {
		t.Fatal("model choice changed configuration or started runs")
	}
	for i, item := range preview.Items {
		binding := item.Plan.ModelExecution
		if binding == nil || binding.Deployment != deployments[i] || binding.Parameters != request.Items[i].ModelExecutionSelection.Parameters || stringify(binding.Budget) != stringify(request.Items[i].ModelExecutionSelection.Budget) {
			t.Fatal("per-item choice was lost", i, binding)
		}
		for _, node := range item.Plan.Workflow.Nodes {
			if node.ID == item.Plan.AgentNodeID && node.MaxRetries != 0 {
				t.Fatal("expert template retry was inherited")
			}
		}
	}
	if preview.Items[0].Plan.ModelExecution.Rate == nil || preview.Items[0].Plan.ModelExecution.Rate.ID != rate.ID || preview.Items[1].Plan.ModelExecution.Rate != nil {
		t.Fatal("exact pricing state was not frozen")
	}
	// A later current quote must not replace a reviewed item's historical rate.
	modelRateForTest(t, f, p, deployments[1], rate.Version)
	read, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID)
	if err != nil || !read.Valid {
		t.Fatal(read.Blockers, err)
	}
	startRequest := h.start(preview)
	started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, startRequest)
	if err != nil {
		t.Fatal(err)
	}
	again, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, startRequest)
	if err != nil || expertCount(t, f, "dag_runs") != 2 || again.Items[0].RunID != started.Items[0].RunID {
		t.Fatal("start recovery changed runs", err)
	}
	byRun := map[string]ExpertTeamPlanItem{}
	for _, item := range started.Items {
		byRun[item.RunID] = item
	}
	claimedRuns := map[string]bool{}
	for poll := 0; poll < 6 && len(claimedRuns) < 2; poll++ {
		claimed := f.bearer("POST", "/runner/claim", h.runnerToken, f.body(map[string]any{"worker_id": p.WorkerID, "capabilities": []string{"command:" + p.Profile.CommandID}}), 200)
		if claimed["assignment"] == nil {
			continue // The bounded ready scanner may finish a turn before wrapping.
		}
		var assignment dag.Assignment
		raw, _ := json.Marshal(claimed["assignment"])
		if json.Unmarshal(raw, &assignment) != nil || assignment.RunID == "" {
			t.Fatal(claimed)
		}
		fixed := byRun[assignment.RunID].Plan.ModelExecution
		node := dag.Node{ID: assignment.NodeID, Type: "agent", Command: assignment.Command, Config: assignment.Config}
		binding, e := dag.ModelBinding(node)
		if e != nil || binding == nil || stringify(binding) != stringify(fixed) {
			t.Fatal("claim changed model binding", binding, e)
		}
		claimedRuns[assignment.RunID] = true
	}
	ledger, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 100, "")
	if err != nil || len(ledger.Items) != 2 {
		t.Fatal(ledger, err)
	}
	for _, attempt := range ledger.Items {
		fixed := byRun[attempt.RunID].Plan.ModelExecution
		if attempt.ReservedTokens != fixed.InputTokenReservation+fixed.OutputTokenReservation {
			t.Fatal("expert attempt used another budget path", attempt)
		}
		if fixed.Rate != nil && (attempt.Rate == nil || attempt.Rate.ID != rate.ID || attempt.Rate.Version != rate.Version) {
			t.Fatal("later quote replaced original ledger rate", attempt)
		}
	}
}

func TestExpertTeamModelPendingRevisionAndCurrentDeploymentGate(t *testing.T) {
	h, p, deployments := newExpertModelFixture(t)
	f, ctx := h.f, context.Background()
	modelRateForTest(t, f, p, deployments[0], 0)
	var err error
	h.tasks[1], err = f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": h.tasks[1].Version, "depends_on": []any{h.tasks[0].ID}}))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, expertModelPlanRequest(h, expertModelSelections(deployments)))
	if err != nil {
		t.Fatal(err)
	}
	started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, h.start(preview))
	if err != nil || started.Items[1].RunID != "" {
		t.Fatal("dependent task was dispatched before acceptance", err)
	}
	rootBinding := stringify(started.Items[0].Plan.ModelExecution)
	paused, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, expertPlanCommand(h, started, "pause"))
	if err != nil {
		t.Fatal(err)
	}
	revision := expertPlanCommand(h, paused, "revise")
	revision.ExpectedConfigurationVersion = h.configuration.Version
	choice := expertModelSelections(deployments)[1]
	choice.Parameters.MaxOutputTokens = 150
	revision.Items = []ExpertTeamPlanItemRequest{{TaskID: h.tasks[1].ID, ExpectedTaskVersion: h.tasks[1].Version, RoleKey: h.role.Key, ModelExecutionSelection: choice}}
	revised, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, revision)
	if err != nil || revised.Revision != 2 || stringify(revised.Items[0].Plan.ModelExecution) != rootBinding || revised.Items[1].Plan.ModelExecution.Parameters.MaxOutputTokens != 150 || revised.ConfigurationVersion != preview.ConfigurationVersion {
		t.Fatal(revised, err)
	}
	history, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, preview.ID, 1)
	if err != nil || history.Items[1].Plan.ModelExecution.Parameters.MaxOutputTokens != 200 {
		t.Fatal("revision rewrote original model source", err)
	}
	reportExpertModelInventory(t, h, p, deployments[:1])
	view, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID)
	if err != nil || view.Valid {
		t.Fatal("pending unavailable model remained eligible", view, err)
	}
	_, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, expertPlanCommand(h, revised, "resume"))
	expertStatus(t, err, 409)
	if expertCount(t, f, "dag_runs") != 1 {
		t.Fatal("unavailable pending model fell back or started")
	}
}

func TestExpertTeamModelSelectionIsExplicitAndBudgetBounded(t *testing.T) {
	h, p, deployments := newExpertModelFixture(t)
	f, ctx := h.f, context.Background()
	modelRateForTest(t, f, p, deployments[0], 0)
	for _, scenario := range []string{"missing", "ordinary-limit", "budget", "unknown-deployment"} {
		t.Run(scenario, func(t *testing.T) {
			choices := expertModelSelections(deployments)
			want := 400
			switch scenario {
			case "missing":
				choices[0] = nil
			case "ordinary-limit":
				choices[0].Parameters.MaxSteps = 17
			case "budget":
				choices[0].Budget.TokenLimit = usageInt(1)
			case "unknown-deployment":
				choices[0].DeploymentSHA256 = strings.Repeat("0", 64)
				want = 409
			}
			_, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, expertModelPlanRequest(h, choices))
			expertStatus(t, err, want)
		})
	}
	if expertCount(t, f, "suma_expert_team_plans") != 0 || expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("invalid selection left a plan or run")
	}
}
