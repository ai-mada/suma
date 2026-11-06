package platform

import (
	"context"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func authorityAutomationFixture(t *testing.T) (*Service, *dag.Engine, Principal, Actor, string, string, AutomationCreateRequest) {
	t.Helper()
	s, p, token, project := authorityFixture(t)
	ctx := context.Background()
	e, err := dag.New(s.DB, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	w, err := e.Publish(ctx, p.TenantID, project, dag.Workflow{Name: "Current source automation", Nodes: []dag.Node{{ID: "gate", Type: "approval", Name: "Actual durable fixture"}}})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	req := AutomationCreateRequest{ProjectID: project, IdempotencyKey: "current-automation-create", Spec: AutomationSpec{Name: "Current automation", WorkflowID: w.ID, WorkflowVersion: w.Version, Trigger: "manual", Timezone: "UTC", Misfire: "skip", CatchUpLimit: 1, Overlap: "allow"}}
	return s, e, p, actor, token, project, req
}

func TestAuthorityAutomationManagementFreshAndReplay(t *testing.T) {
	s, _, _, actor, token, project, req := authorityAutomationFixture(t)
	ctx := context.Background()
	if _, err := s.CreateAutomation(ctx, actor, project, req); err != nil {
		t.Fatal(err)
	}
	api, err := s.IssueProjectCredential(ctx, actor, ProjectCredentialIssueRequest{ProjectID: project, Scope: "api", IdempotencyKey: "automation-scoped-api"})
	if err != nil {
		t.Fatal(err)
	}
	apiActor, err := s.AuthenticateActor(ctx, api.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(ctx, s, token); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{req.IdempotencyKey, "current-automation-fresh"} {
		req.IdempotencyKey = key
		_, err = s.CreateAutomation(ctx, actor, project, req)
		varStatus(t, err, 401)
	}
	req.IdempotencyKey = "api-cannot-manage-automation"
	_, err = s.CreateAutomation(ctx, apiActor, project, req)
	varStatus(t, err, 403)
}

func varStatus(t *testing.T, err error, status int) {
	t.Helper()
	if err == nil {
		t.Errorf("current credential guard missing: wanted %d", status)
		return
	}
	requireAuthorityStatus(t, err, status)
}

func TestAuthorityDurableActivationLogoutAndRevocation(t *testing.T) {
	s, e, p, actor, token, project, req := authorityAutomationFixture(t)
	ctx := context.Background()
	a, err := s.CreateAutomation(ctx, actor, project, req)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := s.CommandAutomation(ctx, actor, project, a.ID, AutomationCommandRequest{ProjectID: project, Action: "enable", ExpectedVersion: a.Version, IdempotencyKey: "durable-enable"})
	if err != nil {
		t.Fatal(err)
	}
	triggered, err := s.CommandAutomation(ctx, actor, project, a.ID, AutomationCommandRequest{ProjectID: project, Action: "trigger", ExpectedVersion: enabled.Rule.Version, IdempotencyKey: "durable-trigger"})
	if err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(ctx, s, token); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = s.TickAutomations(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	var runs int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs); err != nil || runs != 1 {
		t.Fatalf("logout cancelled/duplicated durable activation: %d %v", runs, err)
	}
	_, fresh, err := s.Login(ctx, "authority@example.test", "authority-test-password-42")
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.AuthenticateActor(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CommandAutomation(ctx, current, project, a.ID, AutomationCommandRequest{ProjectID: project, Action: "trigger", ExpectedVersion: triggered.Rule.Version, IdempotencyKey: "durable-trigger-revoked"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, p.TenantID, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,'developer')`, project, p.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.TickAutomations(ctx, e); err != nil {
		t.Fatal(err)
	}
	var blocked int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM suma_automation_occurrences WHERE state='blocked' AND reason='activation_authority_revoked'`).Scan(&blocked); err != nil || blocked != 1 {
		t.Fatalf("revoked activation missing bounded blocked fact: %d %v", blocked, err)
	}
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&runs); err != nil || runs != 1 {
		t.Fatal("evaluation proof started an execution")
	}
	ref := automationRef{tenant: p.TenantID, project: project, id: a.ID}
	candidate, err := s.durableAutomationActor(ctx, s.DB, ref)
	if err != nil {
		t.Fatal(err)
	}
	bounded, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer tx.Rollback()
	proof, err := s.authorizeTx(bounded, tx, candidate, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: a.ID}, purposeAutomationEvaluate)
	if err != nil || !proof.activationDenied {
		t.Fatalf("evaluation did not retain current denial: %v", err)
	}
	_, _, err = s.commandReceipt(bounded, proof, "evaluation-no-receipt", map[string]any{})
	requireAuthorityStatus(t, err, 403)
	_, err = s.authorizeTx(bounded, tx, candidate, proof.target, purposeProjectWrite)
	requireAuthorityStatus(t, err, 403)
}

func TestAuthorityBReadsRequireCurrentBrowser(t *testing.T) {
	s, _, _, browser, token, project, req := authorityAutomationFixture(t)
	ctx := context.Background()
	rule, err := s.CreateAutomation(ctx, browser, project, req)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueProjectCredential(ctx, browser, ProjectCredentialIssueRequest{ProjectID: project, Scope: "api", IdempotencyKey: "read-browser-only-api"})
	if err != nil {
		t.Fatal(err)
	}
	api, err := s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	reads := []func(Actor) error{
		func(a Actor) error { _, e := s.PreviewAutomation(ctx, a, project, req.Spec); return e },
		func(a Actor) error { _, e := s.GetAutomation(ctx, a, project, rule.ID); return e },
		func(a Actor) error { _, e := s.ListAutomations(ctx, a, project, 20, ""); return e },
		func(a Actor) error {
			_, e := s.AutomationOccurrences(ctx, a, project, rule.ID, automationOccurrenceAll, 20, "")
			return e
		},
		func(a Actor) error { _, e := s.AutomationRevisions(ctx, a, project, rule.ID, 20, ""); return e },
		func(a Actor) error { _, e := s.GetAutomationIngress(ctx, a, project, "unreadable-ingress"); return e },
		func(a Actor) error { _, e := s.ListAutomationIngresses(ctx, a, project, rule.ID, 20, ""); return e },
		func(a Actor) error {
			_, e := s.AutomationIngressRevisions(ctx, a, project, "unreadable-ingress", 20, "")
			return e
		},
		func(a Actor) error {
			_, e := s.AutomationIngressDeliveries(ctx, a, project, "unreadable-ingress", 20, "")
			return e
		},
		func(a Actor) error {
			_, e := s.GetAutomationIngressDelivery(ctx, a, project, "unreadable-ingress", "unreadable-delivery")
			return e
		},
		func(a Actor) error { _, e := s.GetToolProfile(ctx, a, project, "unreadable-profile"); return e },
		func(a Actor) error {
			_, e := s.ListToolProfiles(ctx, a, project, ToolProfileListQuery{Limit: 20})
			return e
		},
	}
	var before int
	s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&before)
	for _, read := range reads {
		requireAuthorityStatus(t, read(api), 403)
	}
	if err = logoutCredentialForTest(ctx, s, token); err != nil {
		t.Fatal(err)
	}
	for _, read := range reads {
		requireAuthorityStatus(t, read(browser), 401)
	}
	var after int
	s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&after)
	if before != after {
		t.Fatal("denied protected read wrote a success operation")
	}
}
