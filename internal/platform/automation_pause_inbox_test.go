package platform

import (
	"context"
	"net/http"
	"testing"
)

func TestAutomationPauseInboxHTTP(t *testing.T) {
	f, rule := automationFixture(t, "allow", "manual", "skip")
	ctx := context.Background()
	const inbox = "/notification-inbox"
	const orgInbox = inbox + "/organization"
	rulePath := "/automations/" + rule.ID
	projectInbox := inbox + "?project_id=" + f.project
	command := func(action string, version int, spec *AutomationSpec) AutomationCommandResult {
		t.Helper()
		body := map[string]any{"action": action, "expected_version": version}
		if spec != nil {
			body["spec"] = spec
		}
		return issueRepairRuntimeDecode[AutomationCommandResult](t, f.call(http.MethodPost, rulePath+"/commands", f.body(body), http.StatusOK))
	}
	readRule := func() AutomationRule {
		t.Helper()
		return issueRepairRuntimeDecode[AutomationRule](t, f.call(http.MethodGet, rulePath+"?project_id="+f.project, nil, http.StatusOK))
	}
	occurrence := func(id string) AutomationOccurrence {
		t.Helper()
		for _, item := range autoRead(t, f, rule).Items {
			if item.ID == id {
				return item
			}
		}
		t.Fatal("trigger missing from occurrence history", id)
		return AutomationOccurrence{}
	}

	spec := rule.AutomationSpec
	spec.FailurePauseThreshold = 2
	rule = command("revise", rule.Version, &spec).Rule
	ownerCookie := f.cookie
	otherProject := f.call(http.MethodPost, "/projects", map[string]any{"name": "Other automation project", "idempotency_key": newID()}, http.StatusCreated)["id"].(string)
	const activatorEmail = "automation-activator@example.test"
	invite := governanceInvite(f, activatorEmail, "admin", []any{map[string]any{"project_id": f.project, "role": "admin"}})
	activator := issueRepairRuntimeDecode[Principal](t, governanceAccept(f, invite, activatorEmail)["user"])
	activatorCookie := f.cookie
	rule = command("enable", rule.Version, nil).Rule
	if rule.ActivatedBy != activator.ID || activator.ID == f.user.ID {
		t.Fatalf("fixture did not set a distinct activator: %+v", rule)
	}
	f.cookie = ownerCookie
	f.call(http.MethodGet, "/auth/session", nil, http.StatusOK)

	// The original owner triggers the failures and the pause, but the separate
	// account that enabled this revision owns the resulting notification.
	for i := 0; i < 2; i++ {
		queued := command("trigger", rule.Version, nil)
		rule = queued.Rule
		if queued.Occurrence == nil {
			t.Fatal("trigger missing from command receipt")
		}
		autoTick(t, f)
		started := occurrence(queued.Occurrence.ID)
		if started.State != "started" || started.RunID == "" {
			t.Fatalf("failed to start trigger: %+v", started)
		}
		run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, started.RunID)
		if err != nil {
			t.Fatal(err)
		}
		resolveAutomationTestApproval(t, f, run, "reject", "failed")
	}
	queued := command("trigger", rule.Version, nil)
	if queued.Occurrence == nil {
		t.Fatal("threshold trigger missing from command receipt")
	}
	triggerID := queued.Occurrence.ID
	runs := autoCount(t, f, "dag_runs")
	receipts := autoCount(t, f, "dag_operations")
	noticeCount := autoCount(t, f, "suma_notification_inbox")
	sequenceCount := autoCount(t, f, "suma_notification_inbox_sequences")

	// Reject the inbox insert itself to exercise rollback after pause and queue
	// writes have been attempted on the same database transaction.
	if _, err := f.s.DB.ExecContext(ctx, `CREATE TRIGGER automation_pause_notice_abort BEFORE INSERT ON suma_notification_inbox WHEN NEW.kind='automation_failure_paused' BEGIN SELECT RAISE(ABORT,'fixture inbox rollback'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickAutomations(ctx, f.e); err == nil {
		t.Fatal("injected inbox write failure was ignored")
	}
	if after := readRule(); after.State != "enabled" || after.Version != queued.Rule.Version || after.LastError != "" {
		t.Fatalf("failed notice write changed the rule: %+v", after)
	}
	if after := occurrence(triggerID); after.State != "queued" || after.Reason != "" || after.RunID != "" {
		t.Fatalf("failed notice write changed the queued trigger: %+v", after)
	}
	if autoCount(t, f, "suma_notification_inbox") != noticeCount || autoCount(t, f, "suma_notification_inbox_sequences") != sequenceCount {
		t.Fatal("failed notice write left inbox state behind")
	}
	if _, err := f.s.DB.ExecContext(ctx, `DROP TRIGGER automation_pause_notice_abort`); err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	autoTick(t, f)
	paused := readRule()
	if paused.State != "paused" || paused.Version != queued.Rule.Version+1 || paused.LastError != automationFailurePauseReason {
		t.Fatalf("expected one durable pause: %+v", paused)
	}
	if after := occurrence(triggerID); after.State != "blocked" || after.Reason != automationFailurePauseReason || after.RunID != "" {
		t.Fatalf("expected blocked trigger without a run: %+v", after)
	}
	if autoCount(t, f, "dag_runs") != runs || autoCount(t, f, "dag_operations") != receipts {
		t.Fatal("paused admission created a DAG run or receipt")
	}
	if autoCount(t, f, "suma_notification_inbox") != noticeCount+1 {
		t.Fatal("retried pause did not create exactly one notice")
	}

	get := func(path string) map[string]any {
		t.Helper()
		return f.call(http.MethodGet, path, nil, http.StatusOK)
	}
	checkUnread := func(want int) {
		t.Helper()
		summary := issueRepairRuntimeDecode[NotificationOrganizationUnreadSummary](t, get(orgInbox+"/unread-summary"))
		if summary != (NotificationOrganizationUnreadSummary{OrganizationID: f.view.TenantID, UserID: f.view.ID, Unread: want}) {
			t.Fatalf("wrong unread summary: %+v", summary)
		}
	}
	empty := func(path string) {
		t.Helper()
		page := get(path)
		counts := issueRepairRuntimeDecode[NotificationInboxCounts](t, page["counts"])
		if page["total"] != float64(0) || counts != (NotificationInboxCounts{}) {
			t.Fatalf("unexpected private notice at %s: %+v", path, page)
		}
	}
	empty(projectInbox)
	empty(orgInbox)
	checkUnread(0)

	spec.Name = "Reviewed threshold policy"
	revised := command("revise", paused.Version, &spec).Rule
	if revised.Revision != paused.Revision+1 || revised.Name == paused.Name {
		t.Fatalf("expected a new distinct revision: %+v", revised)
	}
	f.cookie = activatorCookie
	get("/auth/session")
	checkUnread(1)
	empty(inbox + "?project_id=" + otherProject)
	readNotice := func(path string, wantUnread int) map[string]any {
		t.Helper()
		page := get(path)
		items := page["items"].([]any)
		counts := issueRepairRuntimeDecode[NotificationInboxCounts](t, page["counts"])
		if len(items) != 1 || page["total"] != float64(1) || page["user_id"] != activator.ID || counts != (NotificationInboxCounts{Inbox: 1, Unread: wantUnread}) {
			t.Fatalf("wrong private inbox page at %s: %+v", path, page)
		}
		item := items[0].(map[string]any)
		if path == orgInbox {
			if page["organization_id"] != f.user.TenantID || item["project_id"] != f.project {
				t.Fatalf("wrong organization or project for notice: %+v", page)
			}
			item = item["notification"].(map[string]any)
		}
		if item["kind"] != "automation_failure_paused" || item["task_id"] != "" || item["source_task_version"] != float64(0) || item["current_task"] != nil || item["read"] != (wantUnread == 0) {
			t.Fatalf("wrong automation notice at %s: %+v", path, item)
		}
		return item
	}
	projectNotice := readNotice(projectInbox, 1)
	for _, item := range []map[string]any{projectNotice, readNotice(orgInbox, 1)} {
		target, ok := item["automation_target"].(map[string]any)
		if item["id"] != projectNotice["id"] || item["source_available"] != true || !ok || len(target) != 4 ||
			target["rule_id"] != rule.ID || target["revision"] != float64(paused.Revision) || target["trigger_id"] != triggerID || target["title"] != paused.Name {
			t.Fatalf("notice lost the original source after revision change: %+v", item)
		}
	}

	// A mismatched occurrence retains its inbox history but exposes no locator.
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_automation_occurrences SET reason='workflow_unavailable' WHERE id=?`, triggerID); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{projectInbox, orgInbox} {
		item := readNotice(path, 1)
		if _, hasTarget := item["automation_target"]; item["source_available"] != false || hasTarget {
			t.Fatalf("mismatched occurrence remained available at %s: %+v", path, item)
		}
	}
	f.call(http.MethodPost, inbox+"/"+projectNotice["id"].(string)+"/commands", f.body(map[string]any{
		"action": "read", "expected_version": projectNotice["version"],
	}), http.StatusOK)
	checkUnread(0)
	readNotice(projectInbox, 0)
	readNotice(orgInbox, 0)

	org := f.call(http.MethodPost, "/organizations", map[string]any{"name": "Other automation organization", "idempotency_key": newID()}, http.StatusCreated)
	f.call(http.MethodPost, "/auth/switch", map[string]any{"organization_id": org["id"], "idempotency_key": newID()}, http.StatusOK)
	get("/auth/session")
	if f.view.ID != activator.ID || f.view.TenantID != org["id"] {
		t.Fatalf("fixture did not switch the same account to the other organization: %+v", f.view)
	}
	empty(orgInbox)
	checkUnread(0)
}
