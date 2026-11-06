package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestAutomationOccurrenceDetailHTTP(t *testing.T) {
	f, rule := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	originalRevision := rule.Revision
	spec := rule.AutomationSpec
	spec.Name = "Revised automation"
	revised, err := f.s.CommandAutomation(ctx, f.actor, f.project, rule.ID, AutomationCommandRequest{
		ProjectID: f.project, Action: "revise", ExpectedVersion: rule.Version, IdempotencyKey: newID(), Spec: &spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule = revised.Rule
	if rule.Revision != originalRevision+1 {
		t.Fatal("fixture did not retain an older revision")
	}
	spec.Name = "Different rule in the same project"
	otherRule, err := f.s.CreateAutomation(ctx, f.actor, f.project, AutomationCreateRequest{
		ProjectID: f.project, IdempotencyKey: newID(), Spec: spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherProject := f.call(http.MethodPost, "/projects", map[string]any{
		"title": "Other readable project", "idempotency_key": newID(),
	}, http.StatusCreated)["id"].(string)

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	const first = "2026-11-06T10:00:00Z"
	const second = "2026-11-06T10:01:00Z"
	const latest = "2026-11-06T10:02:00Z"
	seedRun := func(project, at string) string {
		t.Helper()
		id := newID()
		exec(`INSERT INTO dag_runs(id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at) VALUES(?,?,?,?,1,'Gate',1,'running',?,?)`,
			id, f.user.TenantID, project, rule.WorkflowID, at, at)
		exec(`INSERT INTO dag_nodes(run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec) VALUES(?,'gate','reconciling','',1,1,'','','','','',0,0,'','{}')`, id)
		return id
	}
	seedOccurrence := func(revision int, at, state, reason, run string) string {
		t.Helper()
		id := newID()
		exec(`INSERT INTO suma_automation_occurrences(id,tenant_id,project_id,rule_id,revision,source,scheduled_at,state,reason,run_id,dedupe_key,created_at,updated_at) VALUES(?,?,?,?,?,'manual',?,?,?,?,?,?,?)`,
			id, f.user.TenantID, f.project, rule.ID, revision, at, state, reason, run, id, at, at)
		return id
	}
	// Durable records give the read test deterministic history order and let
	// it model an invalid cross-project run link without going through a writer.
	blocked := seedOccurrence(originalRevision, first, "blocked", automationFailurePauseReason, "")
	foreign := seedOccurrence(rule.Revision, second, "started", "", seedRun(otherProject, second))
	run := seedRun(f.project, latest)
	started := seedOccurrence(rule.Revision, latest, "started", "", run)

	path := func(ruleID, occurrenceID, projectID string) string {
		return "/automations/" + ruleID + "/occurrences/" + occurrenceID + "?project_id=" + url.QueryEscape(projectID)
	}
	detail := func(id string) AutomationOccurrence {
		t.Helper()
		return issueRepairRuntimeDecode[AutomationOccurrence](t, f.call(http.MethodGet, path(rule.ID, id, f.project), nil, http.StatusOK))
	}
	history := issueRepairRuntimeDecode[AutomationOccurrences](t, f.call(http.MethodGet,
		"/automations/"+rule.ID+"/occurrences?project_id="+url.QueryEscape(f.project)+"&limit=1", nil, http.StatusOK))
	if history.Total != 3 || len(history.Items) != 1 || history.Items[0].ID != started || history.NextCursor == "" {
		t.Fatalf("history did not place the old trigger outside the first page: %+v", history)
	}
	if got := detail(started); got != history.Items[0] || got.RunID != run || got.RunState != "running" || !got.RunNeedsReconciliation {
		t.Fatalf("detail and history disagree on current run facts: %+v", got)
	}
	if got := detail(foreign); got.ID != foreign || got.State != "started" || got.RunID != "" || got.RunState != "" || got.RunNeedsReconciliation {
		t.Fatalf("foreign run facts escaped current project scope: %+v", got)
	}
	assertBlocked := func(got AutomationOccurrence) {
		t.Helper()
		if got.ID != blocked || got.RuleID != rule.ID || got.Revision != originalRevision || got.Source != "manual" ||
			got.ScheduledAt != first || got.State != "blocked" || got.Reason != automationFailurePauseReason || got.RunID != "" {
			t.Fatalf("wrong original blocked occurrence: %+v", got)
		}
	}
	assertBlocked(detail(blocked))

	for _, missing := range []string{
		path(rule.ID, newID(), f.project),
		path(otherRule.ID, blocked, f.project),
		path(rule.ID, blocked, otherProject),
	} {
		f.call(http.MethodGet, missing, nil, http.StatusNotFound)
	}
	endpoint := path(rule.ID, blocked, f.project)
	for _, extra := range []string{"&limit=1", "&filter=attention", "&project_id=" + f.project} {
		f.call(http.MethodGet, endpoint+extra, nil, http.StatusBadRequest)
	}
	base, _, _ := strings.Cut(endpoint, "?")
	f.call(http.MethodGet, base, nil, http.StatusBadRequest)
	f.call(http.MethodGet, base+"?project_id=", nil, http.StatusBadRequest)

	req := httptest.NewRequest(http.MethodGet, "http://suma.test/api/v1"+endpoint, nil)
	req.AddCookie(f.cookie)
	req.Header.Set("X-Suma-Expected-User", f.user.ID)
	req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, req)
	if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("private detail was not returned with no-store: %d, %q", response.Code, response.Header().Get("Cache-Control"))
	}

	apiToken := f.call(http.MethodPost, "/auth/tokens", f.body(map[string]any{"scope": "api"}), http.StatusCreated)["token"].(string)
	f.bearer(http.MethodGet, endpoint, apiToken, nil, http.StatusForbidden)

	const viewerEmail = "automation-trigger-viewer@example.test"
	invite := governanceInvite(f, viewerEmail, "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	viewer := governanceAccept(f, invite, viewerEmail)["user"].(map[string]any)["id"].(string)
	assertBlocked(detail(blocked))
	exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, viewer)
	f.call(http.MethodGet, endpoint, nil, http.StatusForbidden)
}
