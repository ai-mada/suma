package platform

import (
	"context"
	"encoding/json"
	"testing"
)

func TestExpertTeamOrchestrationOriginalCredentialAndExplicitResume(t *testing.T) {
	h, p := orchestrationFixture(t)
	f, ctx := h.f, context.Background()
	restored, activation, err := f.s.restoreExpertTeamActor(ctx, f.user.TenantID, f.project, p.ID)
	if err != nil || restored.kind != f.actor.kind || restored.credential != f.actor.credential || activation.Generation != p.ActivationGeneration {
		t.Fatal("restored different actual credential reference", err)
	}
	if _, _, err = f.s.restoreExpertTeamActor(ctx, f.user.TenantID, "different-project", p.ID); err == nil {
		t.Fatal("activation crossed project scope")
	}
	issued := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api", "label": "Explicit orchestration resume"}), 201)
	api, err := f.s.AuthenticateActor(ctx, issued["token"].(string))
	if err != nil || api.kind != actorProjectAPI {
		t.Fatal(err)
	}
	orchestrationCompleteRoot(t, h, p)
	orchestrationAcceptRoot(t, h, p)
	if _, err = f.s.DB.Exec(`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, f.actor.credential.digest); err != nil {
		t.Fatal(err)
	}
	orchestrationDue(t, h)
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	if expertCount(t, f, "dag_runs") != 1 {
		t.Fatal("expired activation borrowed another credential")
	}
	current, err := f.s.GetExpertTeamPlan(ctx, f.e, api, f.project, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := f.s.CommandExpertTeamPlan(ctx, f.e, api, f.project, p.ID, ExpertTeamPlanCommandRequest{ProjectID: f.project, ExpectedVersion: current.Version, Action: "resume", Reason: "Explicitly renew with current scoped API credential", Acknowledge: true, IdempotencyKey: newID()})
	if err != nil || resumed.ActivationGeneration <= p.ActivationGeneration {
		t.Fatal("explicit resume did not renew generation", err)
	}
	restored, activation, err = f.s.restoreExpertTeamActor(ctx, f.user.TenantID, f.project, p.ID)
	if err != nil || restored.kind != actorProjectAPI || restored.credential != api.credential {
		t.Fatal("API resume became fabricated browser", err)
	}
	corrupt := activation.Credential
	corrupt.Kind = actorBrowser
	if _, err = f.s.DB.Exec(`UPDATE suma_expert_team_plan_activations SET credential_kind=?,credential_reference=? WHERE plan_id=?`, int(actorBrowser), stringify(corrupt), p.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.s.restoreExpertTeamActor(ctx, f.user.TenantID, f.project, p.ID); err == nil {
		t.Fatal("stored display kind forged actual credential kind")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_expert_team_plan_activations SET credential_kind=?,credential_reference=? WHERE plan_id=?`, int(activation.Credential.Kind), stringify(activation.Credential), p.ID); err != nil {
		t.Fatal(err)
	}
	orchestrationDue(t, h)
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	if expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("renewed actual API credential did not advance")
	}
}

func TestExpertTeamOrchestrationRejectsDifferentAcceptedRun(t *testing.T) {
	h, p := orchestrationFixture(t)
	f, ctx := h.f, context.Background()
	orchestrationCompleteRoot(t, h, p)
	orchestrationAcceptRoot(t, h, p)
	other, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, h.role.WorkflowID, newID())
	if err != nil {
		t.Fatal(err)
	}
	var historical ExpertTeamPlan
	if err = json.Unmarshal([]byte(stringify(p)), &historical); err != nil {
		t.Fatal(err)
	}
	historical.Items[0].RunID = other.ID
	_, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, p.ID, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	ok, err := f.s.expertTeamSelectedDependenciesAcceptedTx(ctx, auth.tx, f.user.TenantID, f.project, historical, historical.Items[1])
	if err != nil || ok {
		t.Fatal("accepted original task substituted a different plan run", ok, err)
	}
	ok, err = f.s.expertTeamSelectedDependenciesAcceptedTx(ctx, auth.tx, f.user.TenantID, f.project, p, p.Items[1])
	if err != nil || !ok {
		t.Fatal("actual plan run formal acceptance not recognized", ok, err)
	}
}
