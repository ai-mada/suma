package platform

import (
	"context"
	"testing"
)

// Reuse the real invitation, canonical-resource and independent-test runner
// fixtures. No test holds an authority read transaction across a DB query.
func newTaskReviewRequestFixture(t *testing.T) *handoffFixture {
	t.Helper()
	h := newHandoffFixture(t)
	if _, err := h.f.s.DB.Exec(`UPDATE suma_project_members SET role='reviewer' WHERE project_id=? AND user_id=?`, h.f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	h.task = h.bindRun(t, h.task, true)
	return h
}

func taskReviewRequestInput(t *testing.T, h *handoffFixture, resource string) TaskReviewRequestCreate {
	t.Helper()
	task := h.current(t)
	run, err := h.f.e.GetRun(context.Background(), h.f.user.TenantID, h.f.project, task.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return TaskReviewRequestCreate{ProjectID: h.f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, RunID: run.ID, ExpectedRunVersion: run.Version, ReviewerResourceID: resource, Note: "Review this exact independently tested delivery; final acceptance remains explicit", IdempotencyKey: newID()}
}

func createTaskReviewRequest(t *testing.T, h *handoffFixture, req TaskReviewRequestCreate) TaskReviewRequest {
	t.Helper()
	out, err := h.f.s.CreateTaskReviewRequest(context.Background(), h.f.actor, h.f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTaskReviewRequestsCanonicalTargetsOriginalReplayAndCancellation(t *testing.T) {
	h, ctx := newTaskReviewRequestFixture(t), context.Background()
	f := h.f
	query := TaskReviewTargetQuery{ResourceIDs: []string{h.resource.ID, h.owner.ID}}
	people, err := f.s.ListTaskReviewTargets(ctx, f.actor, f.project, h.task.ID, query)
	if err != nil || people.Total != 2 || len(people.Items) != 2 || people.Items[0].ResourceID != h.resource.ID || people.Items[0].UserID != h.receiver.credential.subjectID || people.Items[1].ResourceID != h.owner.ID {
		t.Fatalf("canonical reviewer/self target resolution: %+v %v", people, err)
	}
	req := taskReviewRequestInput(t, h, h.resource.ID)
	original := createTaskReviewRequest(t, h, req)
	if original.State != "pending" || !original.BindingCurrent || !original.ReviewerAvailable || original.ReviewerUserID != h.receiver.credential.subjectID || original.ReviewerNameAtRequest != h.resource.Title || original.Decision != nil {
		t.Fatalf("request did not preserve a qualified fixed responsibility: %+v", original)
	}
	page := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].Kind != "task_review_requested" || page.Items[0].ReviewRequestID != original.ID || !page.Items[0].SourceAvailable || page.Items[0].AssignmentCurrent {
		t.Fatalf("review responsibility inferred from ownership or lost source: %+v", page)
	}
	other := req
	other.IdempotencyKey = newID()
	_, err = f.s.CreateTaskReviewRequest(ctx, f.actor, f.project, other)
	requireAuthorityStatus(t, err, 409)
	changed := req
	changed.Note = "Changed content under the original operation"
	_, err = f.s.CreateTaskReviewRequest(ctx, f.actor, f.project, changed)
	requireAuthorityStatus(t, err, 409)
	if _, err = f.s.Update(ctx, f.actor, f.project, "members", h.resource.ID, map[string]any{"expected_version": h.resource.Version, "title": "Renamed after original request", "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	replay, err := f.s.CreateTaskReviewRequest(ctx, f.actor, f.project, req)
	if err != nil || stringify(replay) != stringify(original) || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != page.Total {
		t.Fatalf("replay re-resolved the renamed/downgraded target or duplicated delivery: %+v %v", replay, err)
	}
	current, err := f.s.GetTaskReviewRequest(ctx, f.actor, f.project, original.ID)
	if err != nil || current.Request.ReviewerAvailable || current.Request.ReviewerNameAtRequest != h.resource.Title || !current.Request.BindingCurrent || current.Request.State != "pending" {
		t.Fatalf("current eligibility overwrote immutable history or closed on read: %+v %v", current, err)
	}
	people, err = f.s.ListTaskReviewTargets(ctx, f.actor, f.project, h.task.ID, query)
	if err != nil || people.Total != 1 || len(people.Items) != 1 || people.Items[0].ResourceID != h.owner.ID {
		t.Fatalf("developer remained an approver target: %+v %v", people, err)
	}
	command := TaskReviewRequestCommand{ProjectID: f.project, ExpectedVersion: original.Version, Action: "cancel", Reason: "Close the unavailable responsibility explicitly", IdempotencyKey: newID()}
	_, err = f.s.CommandTaskReviewRequest(ctx, h.receiver, f.project, original.ID, command)
	requireAuthorityStatus(t, err, 403)
	cancelled, err := f.s.CommandTaskReviewRequest(ctx, f.actor, f.project, original.ID, command)
	if err != nil || cancelled.State != "cancelled" || cancelled.CancelledByUserID != f.user.ID || cancelled.Decision != nil {
		t.Fatalf("explicit requester/admin cancellation: %+v %v", cancelled, err)
	}
	// Self-request is allowed; cancellation does not consume the delivery or
	// grant approval. The original cancel receipt cannot cancel this new row.
	self := createTaskReviewRequest(t, h, taskReviewRequestInput(t, h, h.owner.ID))
	again, err := f.s.CommandTaskReviewRequest(ctx, f.actor, f.project, original.ID, command)
	history, historyErr := f.s.ListTaskReviewRequests(ctx, f.actor, f.project, h.task.ID, 20, "")
	if err != nil || stringify(again) != stringify(cancelled) || historyErr != nil || history.Total != 2 || history.PendingRequestID != self.ID || inboxPage(t, f, h.receiver, "inbox", 20, "").Total != page.Total {
		t.Fatalf("old cancellation touched a later request or removed private history: %+v %+v %v %v", again, history, err, historyErr)
	}
}

func TestTaskReviewRequestsOriginalReviewClosesOnlyItsExactRequestAndKeepsQualityFeedback(t *testing.T) {
	h, ctx := newTaskReviewRequestFixture(t), context.Background()
	f := h.f
	first := createTaskReviewRequest(t, h, taskReviewRequestInput(t, h, h.resource.ID))
	task := h.current(t)
	// Initial1.0.0 uses the exact business decision names. Tool/deployment
	// approval vocabulary must not silently become formal task acceptance.
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": task.Version, "run_id": task.RunID, "decision": "approved", "reason": "An approval alias must not accept this delivery"}), 400)
	if current := h.current(t); current.Version != task.Version || current.Status != task.Status {
		t.Fatal("noncanonical approval vocabulary changed formal task acceptance")
	}
	accept := f.body(map[string]any{"expected_version": task.Version, "run_id": task.RunID, "decision": "accepted", "reason": "Another currently authorized reviewer accepts the real independent test"})
	accepted, err := f.s.Review(ctx, f.actor, f.project, task.ID, accept, f.e)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("designated responsibility removed another legitimate reviewer's authority: %+v %v", accepted, err)
	}
	detail, err := f.s.GetTaskReviewRequest(ctx, f.actor, f.project, first.ID)
	if err != nil || detail.Request.State != "closed" || detail.Request.ClosedReviewID == "" || detail.Request.Decision == nil || detail.Request.Decision.ReviewID != detail.Request.ClosedReviewID || detail.Request.Decision.ReviewerUserID != f.user.ID || detail.Request.Decision.Decision != "accepted" {
		t.Fatalf("closure invented a decision or failed to name the actual reviewer: %+v %v", detail, err)
	}
	firstReviewID := detail.Request.ClosedReviewID
	second := createTaskReviewRequest(t, h, taskReviewRequestInput(t, h, h.resource.ID))
	beforeReviewCount := domainACount(t, f.s, "suma_reviews")
	replayed, err := f.s.Review(ctx, f.actor, f.project, task.ID, accept, f.e)
	history, historyErr := f.s.ListTaskReviewRequests(ctx, f.actor, f.project, task.ID, 20, "")
	if err != nil || stringify(replayed) != stringify(accepted) || historyErr != nil || history.PendingRequestID != second.ID || domainACount(t, f.s, "suma_reviews") != beforeReviewCount {
		t.Fatalf("old Review replay closed a newer pending request: %+v %+v %v %v", replayed, history, err, historyErr)
	}
	beforeIssues := domainACount(t, f.s, "suma_issues")
	reject := f.body(map[string]any{"expected_version": accepted.Version, "run_id": accepted.RunID, "decision": "rejected", "reason": "Real quality rejection must preserve original issue feedback"})
	rejected, err := f.s.Review(ctx, f.actor, f.project, task.ID, reject, f.e)
	if err != nil || rejected.Status != "blocked" || domainACount(t, f.s, "suma_issues") != beforeIssues+1 {
		t.Fatalf("review request bypassed original rejection feedback: %+v %v", rejected, err)
	}
	detail, err = f.s.GetTaskReviewRequest(ctx, f.actor, f.project, second.ID)
	if err != nil || detail.Request.State != "closed" || detail.Request.Decision == nil || detail.Request.Decision.Decision != "rejected" || detail.Request.ClosedReviewID == firstReviewID {
		t.Fatalf("second exact Review did not close its own request: %+v %v", detail, err)
	}
	if _, err = f.s.Review(ctx, f.actor, f.project, task.ID, reject, f.e); err != nil || domainACount(t, f.s, "suma_issues") != beforeIssues+1 {
		t.Fatal("Review replay duplicated the original quality issue", err)
	}
	delivery, err := f.s.GetTaskDelivery(ctx, f.actor, f.project, task.ID)
	if err != nil || delivery.FormalAcceptance || delivery.Issues.Total != 1 || len(delivery.Issues.Items) != 1 || delivery.Issues.Items[0].SourceReviewID != detail.Request.ClosedReviewID {
		t.Fatalf("request lost the original trusted quality lineage: %+v %v", delivery, err)
	}
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, task.RunID)
	if err != nil || run.State != "succeeded" {
		t.Fatal("quality decision rewrote execution success", run.State, err)
	}
}

func TestTaskReviewRequestsProjectionFailureRollsBackPendingReceiptAndSequence(t *testing.T) {
	h, ctx := newTaskReviewRequestFixture(t), context.Background()
	f := h.f
	tables := []string{"suma_task_review_requests", "suma_task_review_pending", "suma_notification_inbox", "suma_operations", "suma_activity"}
	before := map[string]int{}
	for _, table := range tables {
		before[table] = domainACount(t, f.s, table)
	}
	var sequenceBefore, sequenceAfter int64
	if err := f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&sequenceBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`CREATE TRIGGER review_request_reject_projection BEFORE INSERT ON suma_notification_inbox WHEN NEW.kind='task_review_requested' BEGIN SELECT RAISE(ABORT,'owned review request projection failure'); END`); err != nil {
		t.Fatal(err)
	}
	req := taskReviewRequestInput(t, h, h.resource.ID)
	if _, err := f.s.CreateTaskReviewRequest(ctx, f.actor, f.project, req); err == nil {
		t.Fatal("request committed without its recipient notification")
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER review_request_reject_projection`); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if actual := domainACount(t, f.s, table); actual != before[table] {
			t.Fatal("projection failure retained partial domain or receipt", table, before[table], actual)
		}
	}
	if err := f.s.DB.QueryRow(`SELECT last_sequence FROM suma_notification_inbox_sequences WHERE tenant_id=? AND project_id=?`, f.user.TenantID, f.project).Scan(&sequenceAfter); err != nil || sequenceAfter != sequenceBefore {
		t.Fatal("rolled-back notification consumed sequence", sequenceBefore, sequenceAfter, err)
	}
	created := createTaskReviewRequest(t, h, req)
	if created.State != "pending" || domainACount(t, f.s, "suma_task_review_pending") != before["suma_task_review_pending"]+1 {
		t.Fatal("original operation was unusable after rollback", created)
	}
}

func TestTaskReviewRequestsStaleBindingAndCurrentCallerAuthorization(t *testing.T) {
	h, ctx := newTaskReviewRequestFixture(t), context.Background()
	f := h.f
	req := taskReviewRequestInput(t, h, h.resource.ID)
	original, err := f.s.CreateTaskReviewRequest(ctx, h.receiver, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	current := h.current(t)
	if _, err = f.s.Update(ctx, f.actor, f.project, "tasks", current.ID, f.body(map[string]any{"expected_version": current.Version, "title": "Changed after the exact delivery was requested"})); err != nil {
		t.Fatal(err)
	}
	detail, err := f.s.GetTaskReviewRequest(ctx, f.actor, f.project, original.ID)
	if err != nil || detail.Request.BindingCurrent || detail.Request.State != "pending" || detail.Request.Decision != nil {
		t.Fatalf("read repaired or accepted a stale request: %+v %v", detail, err)
	}
	// Revoke the requesting account itself. A historical operation never
	// restores current read/write authority, even when its result exists.
	if _, err = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CreateTaskReviewRequest(ctx, h.receiver, f.project, req)
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.GetTaskReviewRequest(ctx, h.receiver, f.project, original.ID)
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.CommandTaskReviewRequest(ctx, h.receiver, f.project, original.ID, TaskReviewRequestCommand{ProjectID: f.project, ExpectedVersion: original.Version, Action: "cancel", Reason: "Old requester has no current write authority", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 403)
	if domainACount(t, f.s, "suma_task_review_pending") != 1 {
		t.Fatal("revocation silently closed the pending historical request")
	}
}
