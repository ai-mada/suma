package platform

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestAutomationOccurrenceAttentionHTTP(t *testing.T) {
	f, rule := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	otherSpec := rule.AutomationSpec
	otherSpec.Name = "Other automation"
	other, err := f.s.CreateAutomation(ctx, f.actor, f.project, AutomationCreateRequest{
		ProjectID: f.project, IdempotencyKey: newID(), Spec: otherSpec,
	})
	if err != nil {
		t.Fatal(err)
	}
	foreignProject := f.call(http.MethodPost, "/projects", map[string]any{"title": "Other project", "idempotency_key": newID()}, http.StatusCreated)["id"].(string)

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	seed := func(index int, triggerState, reason, runProject, runState, nodeState string) (id, runID string) {
		t.Helper()
		at := automationStamp(base.Add(time.Duration(index) * time.Minute))
		if runState != "" {
			runID = newID()
			exec(`INSERT INTO dag_runs(id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at) VALUES(?,?,?,?,1,'Gate',1,?,?,?)`,
				runID, f.user.TenantID, runProject, rule.WorkflowID, runState, at, at)
			if nodeState != "" {
				exec(`INSERT INTO dag_nodes(run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec) VALUES(?,'gate',?,'',1,1,'','','','','',0,0,'','{}')`,
					runID, nodeState)
			}
		}
		id = newID()
		exec(`INSERT INTO suma_automation_occurrences(id,tenant_id,project_id,rule_id,revision,source,scheduled_at,state,reason,run_id,dedupe_key,created_at,updated_at) VALUES(?,?,?,?,?,'manual',?,?,?,?,?,?,?)`,
			id, f.user.TenantID, f.project, rule.ID, rule.Revision, at, triggerState, reason, runID, id, at, at)
		return id, runID
	}
	// Seed durable rows directly so the HTTP read is tested independently of the scheduler.
	blocked, _ := seed(1, "blocked", "manual_trigger_authority_revoked", "", "", "")
	failed, failedRun := seed(2, "started", "", f.project, "failed", "failed")
	reconciling, reconcileRun := seed(3, "started", "", f.project, "running", "reconciling")
	seed(4, "skipped", "misfire_skip", "", "", "")
	seed(5, "started", "", f.project, "cancelled", "cancelled")
	seed(6, "queued", "", "", "", "")
	seed(7, "started", "", f.project, "succeeded", "succeeded")
	foreign, _ := seed(8, "started", "", foreignProject, "failed", "reconciling")

	path := func(id, filter, cursor string) string {
		query := url.Values{"project_id": {f.project}, "limit": {"2"}}
		if filter != "" {
			query.Set("filter", filter)
		}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		return "/automations/" + id + "/occurrences?" + query.Encode()
	}
	read := func(filter, cursor string) AutomationOccurrences {
		t.Helper()
		return issueRepairRuntimeDecode[AutomationOccurrences](t, f.call(http.MethodGet, path(rule.ID, filter, cursor), nil, http.StatusOK))
	}
	all := read("all", "")
	implicitAll := read("", "")
	if all.Total != 8 || len(all.Items) != 2 || all.Items[0].ID != foreign || all.Items[0].RunID != "" || all.Items[0].RunState != "" || all.Items[0].RunNeedsReconciliation || all.NextCursor == "" ||
		implicitAll.Total != all.Total || implicitAll.NextCursor != all.NextCursor {
		t.Fatalf("all did not preserve scope, totals and default behavior: explicit=%+v default=%+v", all, implicitAll)
	}

	first := read("attention", "")
	if first.Total != 3 || len(first.Items) != 2 || first.NextCursor == "" {
		t.Fatalf("attention filter must apply before the page limit and total: %+v", first)
	}
	if got := first.Items[0]; got.ID != reconciling || got.State != "started" || got.RunID != reconcileRun || got.RunState != "running" || !got.RunNeedsReconciliation || got.Reason != "" {
		t.Fatalf("reconciliation must preserve trigger and run facts: %+v", got)
	}
	if got := first.Items[1]; got.ID != failed || got.State != "started" || got.RunID != failedRun || got.RunState != "failed" || got.RunNeedsReconciliation {
		t.Fatalf("a failed run must not become a blocked trigger or imply reconciliation: %+v", got)
	}
	second := read("attention", first.NextCursor)
	if second.Total != 3 || len(second.Items) != 1 || second.NextCursor != "" {
		t.Fatalf("unexpected continuation: %+v", second)
	}
	if got := second.Items[0]; got.ID != blocked || got.State != "blocked" || got.Reason != "manual_trigger_authority_revoked" || got.RunID != "" || got.RunState != "" || got.RunNeedsReconciliation {
		t.Fatalf("blocked trigger should retain its reason without a run: %+v", got)
	}

	for _, invalid := range []string{
		path(rule.ID, "all", first.NextCursor),
		path(rule.ID, "attention", all.NextCursor),
		path(other.ID, "attention", first.NextCursor),
		path(rule.ID, "unsupported", ""),
		path(rule.ID, "attention", "") + "&filter=all",
		path(rule.ID, "", "") + "&filter=",
	} {
		f.call(http.MethodGet, invalid, nil, http.StatusBadRequest)
	}

	exec(`UPDATE dag_nodes SET state='succeeded' WHERE run_id=?`, reconcileRun)
	cleared := read("attention", "")
	if cleared.Total != 2 || len(cleared.Items) != 2 || cleared.Items[0].ID != failed || cleared.Items[1].ID != blocked {
		t.Fatalf("attention must reflect current node state: %+v", cleared)
	}

	endpoint := path(rule.ID, "attention", "")
	apiKey := f.call(http.MethodPost, "/auth/tokens", f.body(map[string]any{"scope": "api"}), http.StatusCreated)["token"].(string)
	f.bearer(http.MethodGet, endpoint, apiKey, nil, http.StatusForbidden)
	f.call(http.MethodPost, "/auth/logout", map[string]any{}, http.StatusOK)
	f.call(http.MethodGet, endpoint, nil, http.StatusUnauthorized)
}
