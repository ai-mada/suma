package platform

import (
	"context"
	"net/http"
	"testing"
)

func TestAutomationRunOriginHTTPReadsOriginalOccurrenceAndCurrentProjectAccess(t *testing.T) {
	f, rule := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	rule = autoCommand(t, f, rule, "enable").Rule
	trigger := autoCommand(t, f, rule, "trigger")
	if trigger.Occurrence == nil || trigger.Occurrence.State != "queued" || trigger.Occurrence.RunID != "" {
		t.Fatalf("trigger did not record a queued occurrence: %+v", trigger.Occurrence)
	}

	autoTick(t, f)
	history := autoRead(t, f, rule)
	if len(history.Items) != 1 || history.Items[0].ID != trigger.Occurrence.ID ||
		history.Items[0].State != "started" || history.Items[0].RunID == "" {
		t.Fatalf("scheduler did not bind the original occurrence: %+v", history.Items)
	}
	original := history.Items[0]

	revisedSpec := rule.AutomationSpec
	revisedSpec.Name = "New automation title"
	revised, err := f.s.CommandAutomation(ctx, f.actor, f.project, rule.ID, AutomationCommandRequest{
		ProjectID: f.project, Action: "revise", ExpectedVersion: trigger.Rule.Version, IdempotencyKey: newID(), Spec: &revisedSpec,
	})
	if err != nil || revised.Rule.Revision != original.Revision+1 || revised.Rule.Name != revisedSpec.Name {
		t.Fatalf("fixture did not publish a different revision: %+v, %v", revised.Rule, err)
	}

	runPath := "/runs/" + original.RunID + "/automation-origin"
	path := runPath + "?project_id=" + f.project
	assertOriginal := func(out map[string]any) {
		t.Helper()
		origin, ok := out["origin"].(map[string]any)
		if out["project_id"] != f.project || out["run_id"] != original.RunID ||
			!ok || origin["rule_id"] != rule.ID || origin["rule_name"] != rule.Name ||
			origin["occurrence_id"] != original.ID || origin["revision"] != float64(original.Revision) ||
			origin["source"] != "manual" || origin["scheduled_at"] != trigger.Occurrence.ScheduledAt {
			t.Fatalf("wrong original automation occurrence: %#v", out)
		}
	}
	assertOriginal(f.call(http.MethodGet, path, nil, http.StatusOK))

	ordinary := f.call(http.MethodPost, "/runs", f.body(map[string]any{
		"workflow_id": rule.WorkflowID, "workflow_version": rule.WorkflowVersion,
	}), http.StatusCreated)["id"].(string)
	out := f.call(http.MethodGet, "/runs/"+ordinary+"/automation-origin?project_id="+f.project, nil, http.StatusOK)
	if origin, present := out["origin"]; out["project_id"] != f.project || out["run_id"] != ordinary || !present || origin != nil {
		t.Fatalf("ordinary run did not return an explicit null origin: %#v", out)
	}

	other := f.call(http.MethodPost, "/projects", map[string]any{
		"title": "Other authorized automation scope", "idempotency_key": newID(),
	}, http.StatusCreated)["id"].(string)
	f.call(http.MethodGet, runPath+"?project_id="+other, nil, http.StatusNotFound)
	f.call(http.MethodGet, "/runs/"+newID()+"/automation-origin?project_id="+f.project, nil, http.StatusNotFound)

	api := f.call(http.MethodPost, "/auth/tokens", f.body(map[string]any{"scope": "api"}), http.StatusCreated)["token"].(string)
	f.bearer(http.MethodGet, path, api, nil, http.StatusForbidden)

	const viewerEmail = "automation-origin-viewer@example.test"
	invitation := governanceInvite(f, viewerEmail, "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	viewer := governanceAccept(f, invitation, viewerEmail)["user"].(map[string]any)["id"].(string)
	assertOriginal(f.call(http.MethodGet, path, nil, http.StatusOK))
	if _, err := f.s.DB.ExecContext(ctx, `DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, viewer); err != nil {
		t.Fatal(err)
	}
	f.call(http.MethodGet, path, nil, http.StatusForbidden)
}
