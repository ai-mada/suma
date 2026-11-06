package platform

import (
	"cmp"
	"context"
	"encoding/json"
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

	verifyAutomationPauseIssueProjection(t, f, detailPath(triggerID, f.project), triggerID, failed[1], paused, otherProject)

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

func verifyAutomationPauseIssueProjection(t *testing.T, f *fixture, path, triggerID string, run AutomationOccurrence, rule AutomationRule, otherProject string) {
	t.Helper()
	ctx := context.Background()
	runID := run.RunID
	issueSourcePath := func(project, id string) string {
		return "/failure-diagnosis/issue-source?project_id=" + project + "&issue_id=" + id
	}
	read := func() AutomationFailurePauseEvidence {
		t.Helper()
		result := issueRepairRuntimeDecode[AutomationOccurrence](t, f.call(http.MethodGet, path, nil, http.StatusOK))
		if result.FailurePauseEvidence == nil {
			t.Fatal("optional issue projection removed verified pause evidence")
		}
		return *result.FailurePauseEvidence
	}
	before := read()
	assertLink := func(issueID string) {
		t.Helper()
		counts := func() [3]int {
			return [3]int{autoCount(t, f, "suma_issues"), autoCount(t, f, "suma_operations"), autoCount(t, f, "suma_activity")}
		}
		countBefore := counts()
		current := read()
		want := slices.Clone(before.FailedRuns)
		for i := range want {
			if want[i].RunID == runID {
				want[i].IssueID = issueID
			}
		}
		if current.Threshold != before.Threshold || current.ObservedAt != before.ObservedAt || !slices.Equal(current.FailedRuns, want) {
			t.Fatalf("wrong issue links or changed decision facts: got %+v, want failed runs %+v", current, want)
		}
		if counts() != countBefore {
			t.Fatal("reading the pause detail changed the issue, receipt, or activity counts")
		}
	}
	var stored string
	if err := f.s.DB.QueryRowContext(ctx, `SELECT failed_runs FROM suma_automation_pause_evidence WHERE occurrence_id=?`, triggerID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	// Editable issue metadata cannot manufacture the execution projector's link.
	manualIssue := f.call(http.MethodPost, "/issues", f.body(map[string]any{
		"title": "Manually reported failure", "data": map[string]any{"run_id": runID, "origin": "execution"},
	}), http.StatusCreated)
	manual := manualIssue["id"].(string)
	if source := f.call(http.MethodGet, issueSourcePath(f.project, manual), nil, http.StatusOK); source["issue_id"] != manual || len(source) != 2 {
		t.Fatalf("manual issue returned execution provenance: %+v", source)
	}
	assertLink("")
	if err := f.s.projectFailureIssue(ctx, f.user.TenantID, f.project, runID); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := f.s.DB.QueryRowContext(ctx, `SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, failureIssueKey(runID)).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var issue Record
	if err := json.Unmarshal([]byte(raw), &issue); err != nil || issue.ID == "" {
		t.Fatalf("invalid canonical issue receipt: %v", err)
	}
	assertLink(issue.ID)
	editedIssue := f.call(http.MethodPatch, "/issues/"+issue.ID, f.body(map[string]any{
		"expected_version": issue.Version, "data": map[string]any{"run_id": newID()},
	}), http.StatusOK)
	assertLink(issue.ID)
	sourcePath := issueSourcePath(f.project, issue.ID)
	assertIssueSource := func(source map[string]any, withOrigin, withPause bool) {
		t.Helper()
		if source["project_id"] != f.project || source["issue_id"] != issue.ID || source["run_id"] != runID {
			t.Fatalf("wrong canonical issue source: %+v", source)
		}
		origin, present := source["automation_origin"]
		if !withOrigin {
			if present {
				t.Fatalf("unverified automation origin was returned: %+v", source)
			}
			return
		}
		facts, ok := origin.(map[string]any)
		if !ok || facts["rule_id"] != rule.ID || facts["rule_name"] != rule.Name ||
			facts["occurrence_id"] != run.ID || facts["revision"] != float64(rule.Revision) ||
			facts["source"] != run.Source || facts["scheduled_at"] != run.ScheduledAt {
			t.Fatalf("wrong original automation facts: %+v", source)
		}
		pause, present := facts["failure_pause"]
		if !withPause {
			if present {
				t.Fatalf("unverified pause decision was returned: %+v", source)
			}
			return
		}
		decision, ok := pause.(map[string]any)
		if !ok || decision["occurrence_id"] != triggerID || decision["observed_at"] != before.ObservedAt ||
			decision["threshold"] != float64(before.Threshold) {
			t.Fatalf("wrong historical pause decision: %+v", source)
		}
	}
	browserSource := f.call(http.MethodGet, sourcePath, nil, http.StatusOK)
	assertIssueSource(browserSource, true, true)
	f.call(http.MethodGet, issueSourcePath(otherProject, issue.ID), nil, http.StatusNotFound)
	api := f.call(http.MethodPost, "/auth/tokens", f.body(map[string]any{"scope": "api"}), http.StatusCreated)["token"].(string)
	assertIssueSource(f.bearer(http.MethodGet, sourcePath, api, nil, http.StatusOK), false, false)
	assertFrozenRepair := verifyAutomationRepairPreparation(t, f, editedIssue, manualIssue, browserSource, api, otherProject)

	// Invalid optional links cannot remove the formally projected run source.
	setTriggerTime := func(value string) {
		t.Helper()
		if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_automation_occurrences SET scheduled_at=? WHERE id=?`, value, run.ID); err != nil {
			t.Fatal(err)
		}
	}
	setTriggerTime("invalid")
	assertIssueSource(f.call(http.MethodGet, sourcePath, nil, http.StatusOK), false, false)
	assertFrozenRepair()
	setTriggerTime(run.ScheduledAt)
	setPauseReason := func(reason string) {
		t.Helper()
		if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_automation_occurrences SET reason=? WHERE id=?`, reason, triggerID); err != nil {
			t.Fatal(err)
		}
	}
	setPauseReason("workflow_unavailable")
	assertIssueSource(f.call(http.MethodGet, sourcePath, nil, http.StatusOK), true, false)
	setPauseReason(automationFailurePauseReason)

	setReceipt := func(response string) {
		t.Helper()
		if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_operations SET response=? WHERE tenant_id=? AND project_id=? AND operation_id=?`, response, f.user.TenantID, f.project, failureIssueKey(runID)); err != nil {
			t.Fatal(err)
		}
	}
	setReceipt(`{"invalid":`)
	assertLink("")
	setReceipt(raw)
	if _, err := f.s.DB.ExecContext(ctx, `UPDATE suma_issues SET project_id=? WHERE id=?`, otherProject, issue.ID); err != nil {
		t.Fatal(err)
	}
	assertLink("")
	if _, err := f.s.DB.ExecContext(ctx, `DELETE FROM suma_issues WHERE id=?`, issue.ID); err != nil {
		t.Fatal(err)
	}
	assertLink("")
	var retained string
	if err := f.s.DB.QueryRowContext(ctx, `SELECT failed_runs FROM suma_automation_pause_evidence WHERE occurrence_id=?`, triggerID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != stored {
		t.Fatal("read-time issue projection changed the stored failure evidence")
	}
}

func verifyAutomationRepairPreparation(t *testing.T, f *fixture, issue, manual, source map[string]any, api, otherProject string) func() {
	t.Helper()
	path := func(record map[string]any) string { return "/issues/" + record["id"].(string) + "/repair" }
	body := func(record map[string]any) map[string]any {
		return f.body(map[string]any{
			"expected_issue_version": record["version"],
			"title":                  "Repair the reported workflow failure",
			"description":            "Keep the original failure evidence and independently verify the fix.",
			"owner_id":               "",
			"acceptance_criteria":    []string{"The failed workflow can be independently verified"},
			"reason":                 "Explicit fixture repair preparation",
		})
	}
	assertBasis := func(response, expected map[string]any) map[string]any {
		t.Helper()
		view := issueRepairRuntimeDecode[IssueRepairView](t, response)
		if view.Current == nil || view.Task == nil || view.Task.ID != view.Current.TaskID || view.Task.RunID != "" ||
			view.Issue.Status != "in_progress" || view.Current.Regression != nil || view.Current.Closure != nil || view.CanClose {
			t.Fatalf("preparation must leave a normal task and enforce regression and review: %+v", view)
		}
		current := response["current"].(map[string]any)
		basis, present := current["automation_source"]
		if expected == nil {
			if present {
				t.Fatalf("unverified or unauthorized repair source was exposed: %+v", basis)
			}
		} else if !present || stringify(basis) != stringify(expected) {
			t.Fatalf("repair did not retain the verified source: got %+v want %+v", basis, expected)
		}
		return current
	}
	runs := autoCount(t, f, "dag_runs")
	manualRepair := f.call(http.MethodPost, path(manual)+"/prepare", body(manual), http.StatusCreated)
	assertBasis(manualRepair, nil)

	repairPath := path(issue)
	preparePath := repairPath + "/prepare"
	readPath := repairPath + "?project_id=" + f.project
	request := body(issue)
	expected := map[string]any{"run_id": source["run_id"], "origin": source["automation_origin"]}
	original := assertBasis(f.call(http.MethodPost, preparePath, request, http.StatusCreated), expected)
	f.call(http.MethodGet, repairPath+"?project_id="+otherProject, nil, http.StatusNotFound)
	repairs := autoCount(t, f, "suma_issue_repairs")
	deliveryPath := func(taskID, projectID string) string {
		return "/tasks/" + taskID + "/delivery?project_id=" + projectID
	}
	taskID := original["task_id"].(string)
	assertDelivery := func(result, basis map[string]any) {
		t.Helper()
		if result["project_id"] != f.project || result["task_id"] != taskID ||
			result["run_basis_state"] != "not_dispatched" || result["run"] != nil || result["formal_acceptance"] != false {
			t.Fatalf("repair context changed the task's execution or acceptance state: %+v", result)
		}
		repair, ok := result["repair"].(map[string]any)
		if !ok || repair["issue_id"] != issue["id"] || repair["cycle"] != original["cycle"] || repair["state"] != "active" || repair["regression"] != nil {
			t.Fatalf("wrong repair cycle for undispatched task: %+v", result)
		}
		observed, present := repair["automation_source"]
		if basis == nil && present || basis != nil && (!present || stringify(observed) != stringify(basis)) {
			t.Fatalf("wrong automation source in repair task handoff: got %+v want %+v", observed, basis)
		}
	}
	readDelivery := func() {
		t.Helper()
		delivery := deliveryPath(taskID, f.project)
		assertDelivery(f.call(http.MethodGet, delivery, nil, http.StatusOK), expected)
		assertDelivery(f.bearer(http.MethodGet, delivery, api, nil, http.StatusOK), nil)
	}
	readDelivery()
	f.call(http.MethodGet, deliveryPath(taskID, otherProject), nil, http.StatusNotFound)
	forged := f.call(http.MethodPost, "/tasks", f.body(map[string]any{
		"title": "Ordinary task with editable repair metadata", "requirement_id": original["requirement_id"],
		"data": map[string]any{"repair": map[string]any{"issue_id": issue["id"], "cycle": 1, "state": "active", "automation_source": expected}},
	}), http.StatusCreated)
	if result := f.call(http.MethodGet, deliveryPath(forged["id"].(string), f.project), nil, http.StatusOK); result["repair"] != nil {
		t.Fatalf("editable task metadata produced a protected repair association: %+v", result)
	}

	// These reads and replays run after the live automation source is damaged.
	// The browser gets the original frozen basis, and a token for the same
	// actor can replay the operation without gaining browser-only evidence.
	return func() {
		t.Helper()
		for _, result := range []struct {
			response map[string]any
			basis    map[string]any
		}{
			{f.call(http.MethodGet, readPath, nil, http.StatusOK), expected},
			{f.call(http.MethodPost, preparePath, request, http.StatusCreated), expected},
			{f.bearer(http.MethodGet, readPath, api, nil, http.StatusOK), nil},
			{f.bearer(http.MethodPost, preparePath, api, request, http.StatusCreated), nil},
		} {
			if current := assertBasis(result.response, result.basis); current["id"] != original["id"] || current["task_id"] != original["task_id"] {
				t.Fatalf("the existing repair was replaced on read or replay: %+v", current)
			}
		}
		readDelivery()
		if autoCount(t, f, "dag_runs") != runs || autoCount(t, f, "suma_issue_repairs") != repairs {
			t.Fatal("repair preparation or replay started a run or duplicated a repair")
		}
	}
}
