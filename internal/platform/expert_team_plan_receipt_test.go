package platform

import (
	"context"
	"testing"
)

func expertAssertOriginalPlanReceipt(t *testing.T, got, original ExpertTeamPlan) {
	t.Helper()
	if got.ID != original.ID || got.ProjectID != original.ProjectID || got.TeamID != original.TeamID || got.Version != original.Version || got.Revision != original.Revision || got.GraphSHA256 != original.GraphSHA256 || got.ConfigurationVersion != original.ConfigurationVersion || stringify(got.Configuration) != stringify(original.Configuration) || len(got.Items) != len(original.Items) {
		t.Fatal("replay replaced the original plan/configuration snapshot")
	}
	for i, item := range got.Items {
		fixed := original.Items[i]
		if item.TaskID != fixed.TaskID || item.TaskVersion != fixed.TaskVersion || item.TaskBasisSHA256 != fixed.TaskBasisSHA256 || item.SourceRevision != fixed.SourceRevision || item.RoleKey != fixed.RoleKey || item.RunID != fixed.RunID || item.DispatchPlanID != fixed.DispatchPlanID || stringify(item.Plan.ModelExecution) != stringify(fixed.Plan.ModelExecution) || stringify(item.Plan.Workflow) != stringify(fixed.Plan.Workflow) {
			t.Fatal("replay replaced original task/model/run bindings", i)
		}
	}
	if got.CanStart || len(got.AllowedActions) != 0 || got.Valid {
		t.Fatal("superseded receipt regained current operation eligibility")
	}
	found := false
	for _, reason := range got.Blockers {
		found = found || reason == expertPlanChanged
	}
	if !found {
		t.Fatal("superseded receipt omitted the current-head blocker", got.Blockers)
	}
}

func TestExpertTeamPlanReceiptPreservesReviewedModelsAndCurrentAuthority(t *testing.T) {
	h, profile, deployments := newExpertModelFixture(t)
	f, ctx := h.f, context.Background()
	modelRateForTest(t, f, profile, deployments[0], 0)
	create := expertModelPlanRequest(h, expertModelSelections(deployments))
	original, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
	if err != nil {
		t.Fatal(err)
	}
	// The original create/revise responses may be lost while another authorized
	// view continues reviewing the same plan. Recovery must identify each effect.
	revise := expertPlanCommand(h, original, "revise")
	revise.ExpectedConfigurationVersion = h.configuration.Version
	choice := expertModelSelections(deployments)[1]
	choice.Parameters.MaxOutputTokens = 150
	revise.Items = []ExpertTeamPlanItemRequest{{TaskID: h.tasks[1].ID, ExpectedTaskVersion: h.tasks[1].Version, RoleKey: h.role.Key, ModelExecutionSelection: choice}}
	firstRevision, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, original.ID, revise)
	if err != nil {
		t.Fatal(err)
	}
	h.configuration, err = f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Roles: []ExpertTeamRoleRequest{h.role}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Action: "publish", Reason: "Explicit later configuration review", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.tasks[1], err = f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": h.tasks[1].Version, "description": "Later explicitly reviewed task basis"}))
	if err != nil {
		t.Fatal(err)
	}
	later := expertPlanCommand(h, firstRevision, "revise")
	later.ExpectedConfigurationVersion = h.configuration.Version
	choices := expertModelSelections(deployments)
	choices[1].Parameters.MaxOutputTokens = 125
	later.Items = expertModelPlanRequest(h, choices).Items
	head, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, original.ID, later)
	if err != nil {
		t.Fatal(err)
	}
	replayedCreate, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
	if err != nil {
		t.Fatal(err)
	}
	expertAssertOriginalPlanReceipt(t, replayedCreate, original)
	replayedRevision, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, original.ID, revise)
	if err != nil {
		t.Fatal(err)
	}
	expertAssertOriginalPlanReceipt(t, replayedRevision, firstRevision)
	current, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, original.ID)
	if err != nil || current.Version != head.Version || current.ConfigurationVersion != h.configuration.Version || current.Items[1].Plan.ModelExecution.Parameters.MaxOutputTokens != 125 || expertCount(t, f, "dag_runs") != 0 || expertCount(t, f, "dag_nodes") != 0 {
		t.Fatal("receipt recovery modified the current head or dispatched work", err)
	}
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	if _, err = f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create); err == nil {
		t.Fatal("create receipt bypassed revoked current credential")
	}
	if _, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, original.ID, revise); err == nil {
		t.Fatal("revision receipt bypassed revoked current credential")
	}
}

func TestExpertTeamPlanStartReceiptKeepsOriginalRunsAfterHeadChange(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	preview := h.preview(t)
	request := h.start(preview)
	started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, expertPlanCommand(h, started, "pause"))
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	expertAssertOriginalPlanReceipt(t, replayed, started)
	if replayed.State != "active" || replayed.ActivationGeneration != started.ActivationGeneration || expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("start recovery changed the original activation or created runs")
	}
	current, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID)
	if err != nil || current.Version != paused.Version || current.State != "paused" {
		t.Fatal("old start receipt modified current plan controls", err)
	}
}
