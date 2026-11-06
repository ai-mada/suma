package platform

import (
	"cmp"
	"context"
	"net/http"
	"slices"
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

	var failed []AutomationOccurrence
	// The original owner triggers the failures and the pause, but the separate
	// account that enabled this revision owns the resulting notification.
	startAndDecide := func(decision, state string) AutomationOccurrence {
		t.Helper()
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
		resolveAutomationTestApproval(t, f, run, decision, state)
		return started
	}
	unselected := startAndDecide("approve", "succeeded")
	// Keep the older dispatch out of the newest two even when the test runs
	// within a single database timestamp tick.
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_automation_occurrences SET created_at='2000-01-01T00:00:00Z' WHERE id=?`, unselected.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		failed = append(failed, startAndDecide("reject", "failed"))
	}
	slices.SortFunc(failed, func(left, right AutomationOccurrence) int {
		return cmp.Or(cmp.Compare(right.CreatedAt, left.CreatedAt), cmp.Compare(right.ID, left.ID))
	})
	wantRuns := []AutomationFailurePauseRun{
		{TriggerID: failed[0].ID, RunID: failed[0].RunID},
		{TriggerID: failed[1].ID, RunID: failed[1].RunID},
	}
	first := command("trigger", rule.Version, nil)
	queued := command("trigger", first.Rule.Version, nil)
	if first.Occurrence == nil || queued.Occurrence == nil {
		t.Fatal("threshold triggers missing from command receipts")
	}
	candidates := []AutomationOccurrence{*first.Occurrence, *queued.Occurrence}
	slices.SortFunc(candidates, func(left, right AutomationOccurrence) int {
		return cmp.Or(cmp.Compare(left.ScheduledAt, right.ScheduledAt), cmp.Compare(left.CreatedAt, right.CreatedAt), cmp.Compare(left.ID, right.ID))
	})
	triggerID, siblingID := candidates[0].ID, candidates[1].ID
	detailPath := func(id, project string) string { return rulePath + "/occurrences/" + id + "?project_id=" + project }
	detail := func(id string) AutomationOccurrence {
		t.Helper()
		return issueRepairRuntimeDecode[AutomationOccurrence](t, f.call(http.MethodGet, detailPath(id, f.project), nil, http.StatusOK))
	}
	evidenceCount := func(want int) {
		t.Helper()
		if count := autoCount(t, f, "suma_automation_pause_evidence"); count != want {
			t.Fatalf("pause evidence row count %d, want %d", count, want)
		}
	}
	evidenceCount(0)
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
	evidenceCount(0)
	if _, err := f.s.DB.ExecContext(ctx, `DROP TRIGGER automation_pause_notice_abort`); err != nil {
		t.Fatal(err)
	}
	autoTick(t, f)
	autoTick(t, f)
	paused := readRule()
	if paused.State != "paused" || paused.Version != queued.Rule.Version+1 || paused.LastError != automationFailurePauseReason {
		t.Fatalf("expected one durable pause: %+v", paused)
	}
	for _, id := range []string{triggerID, siblingID} {
		if after := occurrence(id); after.State != "blocked" || after.Reason != automationFailurePauseReason || after.RunID != "" || after.FailurePauseEvidence != nil {
			t.Fatalf("expected blocked trigger in history without a run or embedded evidence: %+v", after)
		}
	}
	evidenceCount(1)
	assertEvidence := func() {
		t.Helper()
		record := detail(triggerID)
		evidence := record.FailurePauseEvidence
		if record.Revision != paused.Revision || evidence == nil || evidence.Threshold != 2 ||
			evidence.ObservedAt == "" || evidence.ObservedAt != record.UpdatedAt || !slices.Equal(evidence.FailedRuns, wantRuns) {
			t.Fatalf("incorrect durable failure evidence: trigger=%+v evidence=%+v", record, evidence)
		}
	}
	assertEvidence()
	runOriginPath := func(runID, project string) string {
		return "/runs/" + runID + "/automation-origin?project_id=" + project
	}
	assertRunOrigin := func(record AutomationOccurrence, expectPause bool) {
		t.Helper()
		out := f.call(http.MethodGet, runOriginPath(record.RunID, f.project), nil, http.StatusOK)
		origin, ok := out["origin"].(map[string]any)
		if !ok || out["project_id"] != f.project || out["run_id"] != record.RunID ||
			origin["occurrence_id"] != record.ID || origin["rule_id"] != paused.ID ||
			origin["rule_name"] != paused.Name || origin["revision"] != float64(paused.Revision) {
			t.Fatalf("incorrect original run provenance: %+v", out)
		}
		pause, present := origin["failure_pause"]
		if !expectPause {
			if present {
				t.Fatalf("run exposed an unverified pause backlink: %+v", origin)
			}
			return
		}
		link, ok := pause.(map[string]any)
		if !ok || len(link) != 3 || link["occurrence_id"] != triggerID ||
			link["observed_at"] != paused.UpdatedAt || link["threshold"] != float64(2) {
			t.Fatalf("incorrect historical pause backlink: %+v", origin)
		}
	}
	for _, record := range failed {
		assertRunOrigin(record, true)
	}
	assertRunOrigin(unselected, false)
	f.call(http.MethodGet, runOriginPath(failed[0].RunID, otherProject), nil, http.StatusNotFound)
	if record := detail(siblingID); record.FailurePauseEvidence != nil {
		t.Fatalf("blocked sibling was incorrectly credited with the pause decision: %+v", record)
	}
	f.call(http.MethodGet, detailPath(triggerID, otherProject), nil, http.StatusNotFound)
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
	spec.FailurePauseThreshold = 3
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

	// A live run can change independently of the terminal observation used
	// to pause. The evidence preserves that observation after a revision.
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE dag_runs SET state='running' WHERE id=?`, wantRuns[0].RunID); err != nil {
		t.Fatal(err)
	}
	assertEvidence()
	assertRunOrigin(failed[0], true)

	// A second retained decision for the same revision makes a backlink
	// ambiguous, even if both candidates contain the exact original run.
	if _, err := f.s.DB.ExecContext(ctx, `INSERT INTO suma_automation_pause_evidence(
		occurrence_id,tenant_id,project_id,rule_id,revision,workflow_id,workflow_version,threshold,observed_at,failed_runs)
		SELECT ?,tenant_id,project_id,rule_id,revision,workflow_id,workflow_version,threshold,observed_at,failed_runs
		FROM suma_automation_pause_evidence WHERE occurrence_id=?`, siblingID, triggerID); err != nil {
		t.Fatal(err)
	}
	assertRunOrigin(failed[0], false)
	if _, err := f.s.DB.ExecContext(ctx, `DELETE FROM suma_automation_pause_evidence WHERE occurrence_id=?`, siblingID); err != nil {
		t.Fatal(err)
	}
	assertRunOrigin(failed[0], true)

	// A damaged or foreign run binding must not expose old evidence links.
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE dag_runs SET project_id=? WHERE id=?`, otherProject, wantRuns[0].RunID); err != nil {
		t.Fatal(err)
	}
	if record := detail(triggerID); record.FailurePauseEvidence != nil {
		t.Fatalf("foreign run binding leaked saved evidence: %+v", record)
	}
	assertRunOrigin(failed[1], false)
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE dag_runs SET project_id=? WHERE id=?`, f.project, wantRuns[0].RunID); err != nil {
		t.Fatal(err)
	}
	assertEvidence()

	// A mismatched occurrence retains its inbox history but exposes no locator.
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_automation_occurrences SET reason='workflow_unavailable' WHERE id=?`, triggerID); err != nil {
		t.Fatal(err)
	}
	if record := detail(triggerID); record.FailurePauseEvidence != nil {
		t.Fatalf("mismatched trigger leaked saved evidence: %+v", record)
	}
	assertRunOrigin(failed[0], false)
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
