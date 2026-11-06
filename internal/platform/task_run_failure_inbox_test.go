package platform

import (
	"context"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func createFailureInboxRun(t *testing.T, f *fixture, taskID string) dag.Run {
	t.Helper()
	ctx := context.Background()
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Failed task inbox", Nodes: []dag.Node{{ID: "work", Type: "task", Idempotent: true}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	if taskID != "" {
		if err = f.s.AttachRun(ctx, f.actor, f.project, taskID, run.ID); err != nil {
			t.Fatal(err)
		}
	}
	return run
}

func failFailureInboxRun(t *testing.T, f *fixture, run dag.Run) dag.Run {
	t.Helper()
	ctx := context.Background()
	claim, err := f.e.Claim(ctx, f.user.TenantID, f.project, "failure-inbox-worker", toolFixtureCapabilities(t, f))
	if err != nil || claim == nil || claim.RunID != run.ID {
		t.Fatalf("wrong claimed run: %+v %v", claim, err)
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: run.ID, NodeID: claim.NodeID, Attempt: claim.Attempt, Fence: claim.Fence, WorkerID: claim.WorkerID, ExitCode: 1, OperationID: newID()}); err != nil {
		t.Fatal(err)
	}
	run, err = f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
	if err != nil || run.State != "failed" {
		t.Fatalf("expected a genuine DAG failure, got %s %v", run.State, err)
	}
	return run
}

func failedRunInboxItems(items []NotificationInboxItem) []NotificationInboxItem {
	result := []NotificationInboxItem{}
	for _, item := range items {
		if item.Kind == taskRunFailedInboxKind {
			result = append(result, item)
		}
	}
	return result
}

func TestTaskRunFailureInboxSealsRecipientsAndReopensHistoricalRun(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	subscriptionCommand(t, f, f.actor, h.task.ID, 0, "follow", newID())
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	if _, err := f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": f.project, "user_id": h.receiver.credential.subjectID, "role": "viewer", "expected_version": float64(1), "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	ownerCookie := f.cookie
	invitation := governanceInvite(f, "late-failure-inbox-follower@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, invitation, "late-failure-inbox-follower@example.test")
	late, err := f.s.AuthenticateActor(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.cookie = ownerCookie

	run := failFailureInboxRun(t, f, createFailureInboxRun(t, f, h.task.ID))
	var at string
	if err = f.s.DB.QueryRow(`SELECT at_time FROM dag_events WHERE run_id=? AND event_type='run_failed' ORDER BY sequence LIMIT 1`, run.ID).Scan(&at); err != nil {
		t.Fatal(err)
	}
	if err = f.s.FailureIssue(ctx, f.user.TenantID, f.project, run); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []Actor{f.actor, h.receiver} {
		items := failedRunInboxItems(inboxPage(t, f, actor, "inbox", 20, "").Items)
		if len(items) != 1 || items[0].RunID != run.ID || !items[0].SourceAvailable || items[0].CurrentTask == nil ||
			items[0].CurrentTask.ID != h.task.ID || items[0].SourceTaskVersion != h.task.Version+1 || items[0].CreatedAt != at || items[0].Read {
			t.Fatalf("recipient missing exact original run evidence or received duplicates: %+v", items)
		}
	}
	if issues := domainACount(t, f.s, "suma_issues"); issues != 1 {
		t.Fatalf("original failure should create one issue, got %d", issues)
	}

	subscriptionCommand(t, f, late, h.task.ID, 0, "follow", newID())
	subscriptionCommand(t, f, h.receiver, h.task.ID, 1, "unfollow", newID())
	next := createFailureInboxRun(t, f, h.task.ID)
	if err = f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	if issues := domainACount(t, f.s, "suma_issues"); issues != 1 {
		t.Fatalf("failure replay duplicated issue: %d", issues)
	}
	if items := failedRunInboxItems(inboxPage(t, f, late, "inbox", 20, "").Items); len(items) != 0 {
		t.Fatalf("failure replay notified a later follower: %+v", items)
	}
	current := h.current(t)
	aggregate, err := f.s.NotificationOrganizationInbox(ctx, h.receiver, "inbox", 20, "")
	if err != nil || current.RunID != next.ID || len(aggregate.Items) != 1 || aggregate.Items[0].ProjectID != f.project ||
		aggregate.Items[0].Notification.RunID != run.ID || aggregate.Items[0].Notification.CurrentTask == nil {
		t.Fatalf("rebound task lost original historical failure: %+v %+v %v", current, aggregate, err)
	}
	notification := aggregate.Items[0].Notification
	cmd := NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: notification.Version, Action: "read", IdempotencyKey: newID()}
	read, err := f.s.CommandNotificationInbox(ctx, h.receiver, f.project, notification.ID, cmd)
	if err != nil || !read.Read || h.current(t).Version != current.Version {
		t.Fatalf("viewer could not acknowledge privately: %+v %v", read, err)
	}

	if _, err = f.s.DB.Exec(`UPDATE suma_task_run_provenance SET payload='{}' WHERE run_id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	invalid := failedRunInboxItems(inboxPage(t, f, h.receiver, "inbox", 20, "").Items)
	if len(invalid) != 1 || invalid[0].SourceAvailable || invalid[0].CurrentTask != nil || invalid[0].RunID != "" || invalid[0].SourceResourceID == run.ID {
		t.Fatalf("corrupt binding disclosed current task or run link: %+v", invalid)
	}

	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.NotificationInbox(ctx, h.receiver, f.project, "inbox", 20, "")
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.CommandNotificationInbox(ctx, h.receiver, f.project, notification.ID, cmd)
	requireAuthorityStatus(t, err, 403)
}

func TestTaskRunFailureInboxAtomicWithIssueAndSkipsRevokedFollower(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	subscriptionCommand(t, f, h.receiver, h.task.ID, 0, "follow", newID())
	run := failFailureInboxRun(t, f, createFailureInboxRun(t, f, h.task.ID))
	if _, err := f.s.DB.Exec(`CREATE TRIGGER fail_run_failure_notice BEFORE INSERT ON suma_notification_inbox WHEN NEW.kind='task_run_failed' BEGIN SELECT RAISE(ABORT,'forced failed-run inbox write'); END`); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ProjectFailures(ctx); err == nil {
		t.Fatal("failed notification write committed failure projection")
	}
	if issues, ack := domainACount(t, f.s, "suma_issues"), domainACount(t, f.s, "suma_failure_projection"); issues != 0 || ack != 0 {
		t.Fatalf("partial failure projection after notification write failure: issues=%d acknowledgements=%d", issues, ack)
	}
	var receipts int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations WHERE operation_id=?`, "failure-"+run.ID).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatalf("partial failure receipt: %d %v", receipts, err)
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER fail_run_failure_notice`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_failure_projection_retries SET next_attempt_at=0 WHERE sequence IN (SELECT sequence FROM dag_events WHERE run_id=? AND event_type='run_failed')`, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	var recipient string
	if err := f.s.DB.QueryRow(`SELECT recipient_user_id FROM suma_notification_inbox WHERE kind='task_run_failed' AND source_event_key=?`, taskRunFailedEventPrefix+run.ID).Scan(&recipient); err != nil || recipient != f.user.ID {
		t.Fatalf("missing original current owner after retry: %s %v", recipient, err)
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_notification_inbox WHERE kind='task_run_failed'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("revoked follower or duplicate notification: %d %v", count, err)
	}
}

func TestTaskRunFailureInboxSkipsUnboundAndMalformedRuns(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	failFailureInboxRun(t, f, createFailureInboxRun(t, f, ""))
	bound := createFailureInboxRun(t, f, h.task.ID)
	if _, err := f.s.DB.Exec(`UPDATE suma_task_run_provenance SET payload='{}' WHERE run_id=?`, bound.ID); err != nil {
		t.Fatal(err)
	}
	failFailureInboxRun(t, f, bound)
	if err := f.s.ProjectFailures(ctx); err != nil {
		t.Fatal(err)
	}
	if items := failedRunInboxItems(inboxPage(t, f, f.actor, "inbox", 20, "").Items); len(items) != 0 {
		t.Fatalf("unbound or malformed run created a task notification: %+v", items)
	}
	if issues := domainACount(t, f.s, "suma_issues"); issues != 2 {
		t.Fatalf("failure issue lost for an unbound run: %d", issues)
	}
}
