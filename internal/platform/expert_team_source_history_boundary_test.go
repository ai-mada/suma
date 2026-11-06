package platform

import (
	"context"
	"testing"
)

func TestExpertTeamCurrentModelExcludesOriginalOnlyHistoryAndUsesRevisionTime(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	current := h.preview(t)
	started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, current.ID, h.start(current))
	if err != nil {
		t.Fatal(err)
	}
	oldID := newID()
	// This is the exact prior persisted shape: real actual run references live
	// only inside Plan, without a 029 source revision or activation.
	original := stringify(map[string]any{"id": oldID, "project_id": f.project, "team_id": h.team, "team_name": started.TeamName, "version": 2, "state": "started", "configuration_version": started.ConfigurationVersion, "configuration": started.Configuration, "created_at": started.CreatedAt, "expires_at": "", "valid": true, "blockers": []string{}, "can_start": false, "items": []any{map[string]any{"role_key": started.Items[0].RoleKey, "role_title": started.Items[0].RoleTitle, "resource_id": started.Items[0].ResourceID, "resource_name": started.Items[0].ResourceName, "plan": started.Items[0].Plan, "run_state": started.Items[0].RunState}}})
	if _, err = f.s.DB.Exec(`INSERT INTO suma_expert_team_plans(id,tenant_id,project_id,team_id,version,state,expires_at,payload)VALUES(?,?,?,?,2,'started','',?)`, oldID, f.user.TenantID, f.project, h.team, original); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListExpertTeamPlans(ctx, f.e, f.actor, f.project, "", "", 1, "")
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != current.ID || page.NextCursor != "" {
		t.Fatal("original-only history entered current orchestration", page, err)
	}
	_, err = f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, oldID)
	expertStatus(t, err, 409)
	_, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, oldID, ExpertTeamPlanStartRequest{ProjectID: f.project, ExpectedVersion: 2, AcknowledgeDispatch: true, IdempotencyKey: newID()})
	expertStatus(t, err, 409)
	_, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, oldID, ExpertTeamPlanCommandRequest{ProjectID: f.project, ExpectedVersion: 2, Action: "stop", Reason: "Original-only records cannot authorize current controls", Acknowledge: true, IdempotencyKey: newID()})
	expertStatus(t, err, 409)
	var after string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plans WHERE id=?`, oldID).Scan(&after); err != nil || after != original {
		t.Fatal("original references or payload were rewritten", err)
	}
	if expertCount(t, f, "dag_runs") != 2 || expertCount(t, f, "suma_expert_team_plan_activations") != 1 {
		t.Fatal("original-only history acquired activation or execution")
	}

	// A preexisting 029 revision already has a truthful independent SQL time,
	// even when its original payload copied the plan creation timestamp.
	older := "2000-01-01T00:00:00Z"
	var immutableSource string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plan_revisions WHERE plan_id=? AND revision=1`, current.ID).Scan(&immutableSource); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_expert_team_plan_revisions SET created_at=? WHERE plan_id=? AND revision=1`, older, current.ID); err != nil {
		t.Fatal(err)
	}
	first, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, current.ID, 1)
	if err != nil || first.CreatedAt != older {
		t.Fatal("revision detail reused plan creation time", first.CreatedAt, err)
	}
	history, err := f.s.ExpertTeamPlanRevisions(ctx, f.e, f.actor, f.project, current.ID, 20, "")
	if err != nil || len(history.Items) != 1 || history.Items[0].CreatedAt != older {
		t.Fatal("revision list reused plan creation time", history, err)
	}
	var unchangedSource string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plan_revisions WHERE plan_id=? AND revision=1`, current.ID).Scan(&unchangedSource); err != nil || immutableSource != unchangedSource {
		t.Fatal("revision time display rewrote immutable source", err)
	}
	third, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Explicit new revision timestamp source"}))
	if err != nil {
		t.Fatal(err)
	}
	request := h.request()
	request.Items = []ExpertTeamPlanItemRequest{{TaskID: third.ID, ExpectedTaskVersion: third.Version, RoleKey: h.role.Key}}
	fresh, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	third, err = f.s.Update(ctx, f.actor, f.project, "tasks", third.ID, f.body(map[string]any{"expected_version": third.Version, "description": "Explicit revision of still unstarted task"}))
	if err != nil {
		t.Fatal(err)
	}
	command := expertPlanCommand(h, fresh, "revise")
	command.ExpectedConfigurationVersion = h.configuration.Version
	command.Items = []ExpertTeamPlanItemRequest{{TaskID: third.ID, ExpectedTaskVersion: third.Version, RoleKey: h.role.Key}}
	if _, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, fresh.ID, command); err != nil {
		t.Fatal(err)
	}
	var revisionTime string
	if err = f.s.DB.QueryRow(`SELECT created_at FROM suma_expert_team_plan_revisions WHERE plan_id=? AND revision=2`, fresh.ID).Scan(&revisionTime); err != nil {
		t.Fatal(err)
	}
	second, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, fresh.ID, 2)
	if err != nil || second.CreatedAt != revisionTime {
		t.Fatal("new revision detail does not use its recorded creation time", second.CreatedAt, revisionTime, err)
	}
}
