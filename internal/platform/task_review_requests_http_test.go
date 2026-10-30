package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestTaskReviewTargetsPagedCanonicalApprovalRolesAndClosedHTTP(t *testing.T) {
	h, ctx := newTaskReviewRequestFixture(t), context.Background()
	f := h.f
	page, err := f.s.ListTaskReviewTargets(ctx, f.actor, f.project, h.task.ID, TaskReviewTargetQuery{Limit: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatalf("bounded canonical approval directory: %+v %v", page, err)
	}
	next, err := f.s.ListTaskReviewTargets(ctx, f.actor, f.project, h.task.ID, TaskReviewTargetQuery{Limit: 1, Cursor: page.NextCursor})
	if err != nil || next.Total != 2 || len(next.Items) != 1 || next.Items[0].ResourceID == page.Items[0].ResourceID || next.NextCursor != "" {
		t.Fatalf("reviewer page did not advance exactly once: %+v %v", next, err)
	}
	if _, err = f.s.Update(ctx, f.actor, f.project, "members", h.resource.ID, map[string]any{"expected_version": h.resource.Version, "title": "Literal %_! reviewer", "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	literal, err := f.s.ListTaskReviewTargets(ctx, f.actor, f.project, h.task.ID, TaskReviewTargetQuery{Q: "%_!", Limit: 20})
	if err != nil || literal.Total != 1 || len(literal.Items) != 1 || literal.Items[0].ResourceID != h.resource.ID {
		t.Fatalf("directory treated literal wildcards as patterns: %+v %v", literal, err)
	}
	base := "/task-review-targets?project_id=" + f.project + "&task_id=" + h.task.ID
	for _, suffix := range []string{"&user_id=forged", "&limit=0", "&limit=51", "&limit=01", "&q=a&q=b", "&q=" + url.QueryEscape("bad\u0085query"), "&resource_ids=" + h.owner.ID + "," + h.owner.ID, "&resource_ids=" + h.owner.ID + "&q=Literal", "&resource_ids=" + h.owner.ID + "&cursor=token"} {
		f.call("GET", base+suffix, nil, 400)
	}
	other, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Other directory scope"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ListTaskReviewTargets(ctx, f.actor, f.project, other.ID, TaskReviewTargetQuery{Limit: 1, Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 400)
	agent, err := f.s.Create(ctx, f.actor, f.project, "members", f.body(map[string]any{"title": "Agent is not review authority", "kind": "agent"}))
	if err != nil {
		t.Fatal(err)
	}
	foreignProject := f.call("POST", "/projects", map[string]any{"title": "Other reviewer project", "idempotency_key": newID()}, 201)["id"].(string)
	foreign, err := f.s.Create(ctx, f.actor, foreignProject, "members", f.body(map[string]any{"title": "Foreign canonical owner", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}))
	if err != nil {
		t.Fatal(err)
	}
	forged := recordFrom(f.user, f.project, map[string]any{"title": "Duplicate resource without canonical binding", "kind": "human", "status": "active", "data": map[string]any{"user_id": f.user.ID}})
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.insertRecord(ctx, tx, forged, "members"); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	// These are real rows, but none is the current project's canonical human
	// reviewer. A label, foreign binding or duplicate row cannot authorize it.
	for _, resource := range []string{agent.ID, foreign.ID, forged.ID} {
		invalid := taskReviewRequestInput(t, h, resource)
		_, err = f.s.CreateTaskReviewRequest(ctx, f.actor, f.project, invalid)
		requireAuthorityStatus(t, err, 400)
	}
	if domainACount(t, f.s, "suma_task_review_requests") != 0 || domainACount(t, f.s, "suma_task_review_pending") != 0 {
		t.Fatal("invalid human identity retained a responsibility request")
	}
	for _, role := range []string{"developer", "viewer"} {
		if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role=? WHERE project_id=? AND user_id=?`, role, f.project, h.receiver.credential.subjectID); err != nil {
			t.Fatal(err)
		}
		resolved, err := f.s.ListTaskReviewTargets(ctx, f.actor, f.project, h.task.ID, TaskReviewTargetQuery{ResourceIDs: []string{h.resource.ID, h.owner.ID}})
		if err != nil || resolved.Total != 1 || len(resolved.Items) != 1 || resolved.Items[0].ResourceID != h.owner.ID {
			t.Fatalf("%s was treated as approval authority: %+v %v", role, resolved, err)
		}
	}
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodGet, "/api/v1"+base, nil)
		request.Header.Set("Authorization", "Bearer "+issued.Token)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, request)
		want := http.StatusForbidden
		if scope == "api" {
			want = http.StatusOK
		}
		if response.Code != want || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s directory authority/no-store: %d %s", scope, response.Code, response.Body.String())
		}
	}
}

func TestTaskReviewRequestsHTTPClosedWritesExactHistoryAndNoReadEffects(t *testing.T) {
	h := newTaskReviewRequestFixture(t)
	f := h.f
	req := taskReviewRequestInput(t, h, h.resource.ID)
	body := map[string]any{"project_id": req.ProjectID, "task_id": req.TaskID, "expected_task_version": req.ExpectedTaskVersion, "run_id": req.RunID, "expected_run_version": req.ExpectedRunVersion, "reviewer_resource_id": req.ReviewerResourceID, "note": req.Note, "idempotency_key": req.IdempotencyKey}
	for _, field := range []string{"reviewer_user_id", "requester_user_id", "decision", "notification_id"} {
		invalid := map[string]any{}
		for key, value := range body {
			invalid[key] = value
		}
		invalid[field] = "forged"
		f.call("POST", "/task-review-requests", invalid, 400)
	}
	f.call("POST", "/task-review-requests?project_id="+f.project, body, 400)
	created := f.call("POST", "/task-review-requests", body, 201)
	id, ok := created["id"].(string)
	if !ok || created["state"] != "pending" {
		t.Fatal("original POST did not create its exact request", created)
	}
	before := map[string]int{}
	for _, table := range []string{"suma_task_review_requests", "suma_task_review_pending", "suma_notification_inbox", "suma_reviews", "suma_operations", "suma_activity", "dag_runs"} {
		before[table] = domainACount(t, f.s, table)
	}
	path := "/task-review-requests/" + id + "?project_id=" + f.project
	detail := f.call("GET", path, nil, 200)
	if detail["task_id"] != h.task.ID || detail["request"].(map[string]any)["id"] != id {
		t.Fatal("exact detail substituted another request", detail)
	}
	list := "/task-review-requests?project_id=" + f.project + "&task_id=" + h.task.ID
	history := f.call("GET", list+"&limit=1", nil, 200)
	if history["total"] != float64(1) || history["pending_request_id"] != id {
		t.Fatal("history lost its full total or pending responsibility", history)
	}
	for _, suffix := range []string{"&task_id=another", "&limit=51", "&limit=", "&cursor=", "&q=forged"} {
		f.call("GET", list+suffix, nil, 400)
	}
	f.call("GET", path+"&task_id=another", nil, 400)
	for table, count := range before {
		if actual := domainACount(t, f.s, table); actual != count {
			t.Fatal("read created business/operation facts", table, count, actual)
		}
	}
	page := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if page.Counts.Unread != 1 || page.Items[0].Read {
		t.Fatal("opening request implicitly read its private notification", page)
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Unrelated authorized project", "idempotency_key": newID()}, 201)
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/task-review-requests/"+id+"?project_id="+other["id"].(string), nil)
	request.AddCookie(f.cookie)
	f.handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), req.Note) || strings.Contains(response.Body.String(), h.resource.Title) {
		t.Fatal("foreign project detail disclosed responsibility", response.Code, response.Body.String())
	}
	command := map[string]any{"project_id": f.project, "expected_version": 1, "action": "cancel", "reason": "Explicitly cancel original request", "idempotency_key": newID(), "decision": "accepted"}
	f.call("POST", "/task-review-requests/"+id+"/commands", command, 400)
	delete(command, "decision")
	f.call("POST", "/task-review-requests/"+id+"/commands?project_id="+f.project, command, 400)
}

func TestTaskReviewRequestBrokenSourcesDoNotInventDecisionOrInboxLink(t *testing.T) {
	h, ctx := newTaskReviewRequestFixture(t), context.Background()
	f := h.f
	original := createTaskReviewRequest(t, h, taskReviewRequestInput(t, h, h.resource.ID))
	page := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ReviewRequestID != original.ID || !page.Items[0].SourceAvailable {
		t.Fatal("fixture did not establish exact private source", page)
	}
	item := page.Items[0]
	task := h.current(t)
	if _, err := f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": task.RunID, "decision": "accepted", "reason": "Real review before deliberately corrupting its source relation"}), f.e); err != nil {
		t.Fatal(err)
	}
	detail, err := f.s.GetTaskReviewRequest(ctx, f.actor, f.project, original.ID)
	if err != nil || detail.Request.Decision == nil {
		t.Fatal("fixture lacks a real closed Review", detail, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_reviews SET task_version=task_version+1 WHERE id=?`, detail.Request.ClosedReviewID); err != nil {
		t.Fatal(err)
	}
	detail, err = f.s.GetTaskReviewRequest(ctx, f.actor, f.project, original.ID)
	if err != nil || detail.Request.SourceAvailable || detail.Request.Decision != nil || detail.Request.State != "closed" {
		t.Fatalf("damaged Review was presented as an authoritative decision: %+v %v", detail, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_task_review_requests SET notification_id='missing-notification' WHERE id=?`, original.ID); err != nil {
		t.Fatal(err)
	}
	broken := inboxPage(t, f, h.receiver, "inbox", 20, "")
	if broken.Total != 1 || len(broken.Items) != 1 || broken.Items[0].ID != item.ID || broken.Items[0].SourceAvailable || broken.Items[0].ReviewRequestID != "" || broken.Items[0].CurrentTask != nil || broken.Items[0].Read || broken.Items[0].Archived {
		t.Fatalf("broken relation leaked a link or erased the private event: %+v", broken)
	}
}
