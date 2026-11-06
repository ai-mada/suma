package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"sort"
	"testing"
)

func TestNotificationOrganizationInboxCurrentScopeAndPaging(t *testing.T) {
	ctx := context.Background()
	h := newHandoffFixture(t)
	f, recipient := h.f, h.receiver.credential.subjectID
	create := func(actor Actor, project, kind string, body map[string]any) Record {
		t.Helper()
		body["idempotency_key"] = newID()
		record, err := f.s.Create(ctx, actor, project, kind, body)
		if err != nil {
			t.Fatal(err)
		}
		return record
	}
	projectWithRecipient := func(title string) Record {
		t.Helper()
		project, err := f.s.CreateProject(ctx, f.actor, f.body(map[string]any{"title": title}))
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": project.ID, "user_id": recipient, "role": "viewer", "expected_version": float64(0), "idempotency_key": newID()})
		if err != nil {
			t.Fatal(err)
		}
		return project
	}
	other := projectWithRecipient("Second project")
	hidden := projectWithRecipient("Earlier visible project")
	for _, project := range []Record{other, hidden} {
		member := create(f.actor, project.ID, "members", map[string]any{"title": "Canonical recipient", "kind": "human", "data": map[string]any{"user_id": recipient}})
		create(f.actor, project.ID, "tasks", map[string]any{"title": project.Title + " original task", "owner_id": member.ID})
	}
	// Keep the recipient's genuine old notification, but an invalid current
	// role cannot grant access or leak that project into the aggregate counts.
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='VIEWER' WHERE project_id=? AND user_id=?`, hidden.ID, recipient); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.NotificationInbox(ctx, h.receiver, hidden.ID, "inbox", 20, "")
	requireAuthorityStatus(t, err, 403)

	mentioned := taskConversationRequest(f, h.task.ID, h.task.Version, "Organization inbox explicit mention")
	mentioned.MentionResourceIDs = []string{h.resource.ID}
	message, err := f.s.AppendTaskConversation(ctx, f.actor, f.project, mentioned)
	if err != nil {
		t.Fatal(err)
	}
	handoff := h.send(t)
	firstProject := inboxPage(t, f, h.receiver, "inbox", 20, "")
	secondProject, err := f.s.NotificationInbox(ctx, h.receiver, other.ID, "inbox", 20, "")
	if err != nil || len(secondProject.Items) != 1 || len(firstProject.Items) != 2 {
		t.Fatalf("missing real notifications: first=%+v second=%+v err=%v", firstProject, secondProject, err)
	}
	atWholeSecond := "2026-10-29T12:00:00Z"
	atNanosecond := "2026-10-29T12:00:00.000000001Z"
	tied := []string{firstProject.Items[0].ID, secondProject.Items[0].ID}
	sort.Sort(sort.Reverse(sort.StringSlice(tied)))
	for id, at := range map[string]string{tied[0]: atNanosecond, tied[1]: atNanosecond, firstProject.Items[1].ID: atWholeSecond} {
		if _, err = f.s.DB.Exec(`UPDATE suma_notification_inbox SET created_at=? WHERE id=?`, at, id); err != nil {
			t.Fatal(err)
		}
	}

	assertUnread := func(actor Actor, user string, want int) {
		t.Helper()
		got, err := f.s.NotificationOrganizationUnreadSummary(ctx, actor)
		if err != nil || got != (NotificationOrganizationUnreadSummary{OrganizationID: f.user.TenantID, UserID: user, Unread: want}) {
			t.Fatalf("unread summary = %+v, err = %v; want %d", got, err, want)
		}
	}
	assertUnread(h.receiver, recipient, 3)
	assertUnread(f.actor, f.user.ID, 1)
	first, err := f.s.NotificationOrganizationInbox(ctx, h.receiver, "unread", 1, "")
	if err != nil || first.Total != 3 || first.Counts != (NotificationInboxCounts{Inbox: 3, Unread: 3}) || len(first.Items) != 1 || first.NextCursor == "" || first.OrganizationID != f.user.TenantID || first.UserID != recipient || len(first.ScopeRevision) != 64 {
		t.Fatalf("wrong aggregate scope: %+v %v", first, err)
	}
	seen := []string{}
	page := first
	for {
		if page.ScopeRevision != first.ScopeRevision || len(page.Items) != 1 || len(seen) >= 3 {
			t.Fatalf("inconsistent scope or pagination: %+v", page)
		}
		row := page.Items[0]
		seen = append(seen, row.Notification.ID)
		switch row.Notification.Kind {
		case "task_mentioned":
			if row.ProjectID != f.project || row.Notification.MessageID != message.ID || row.Notification.CurrentTask == nil {
				t.Fatalf("incorrect mention source: %+v", row)
			}
		case "task_handoff_requested":
			if row.ProjectID != f.project || row.Notification.HandoffID != handoff.ID || row.Notification.CurrentTask == nil {
				t.Fatalf("incorrect handoff source: %+v", row)
			}
		case "task_assigned":
			if row.ProjectID != other.ID || row.ProjectTitle != other.Title || row.Notification.CurrentTask == nil || row.Notification.CurrentTask.Title != other.Title+" original task" {
				t.Fatalf("incorrect assigned source or project: %+v", row)
			}
		default:
			t.Fatalf("unexpected source: %+v", row)
		}
		if page.NextCursor == "" {
			break
		}
		page, err = f.s.NotificationOrganizationInbox(ctx, h.receiver, "unread", 1, page.NextCursor)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(seen, append(tied, firstProject.Items[1].ID)) {
		t.Fatalf("not newest-first with exact timestamp and ID tie-break: %v", seen)
	}
	ownerInbox, err := f.s.NotificationOrganizationInbox(ctx, f.actor, "", 20, "")
	if err != nil || ownerInbox.Total != 1 || ownerInbox.Items[0].Notification.TaskID != h.task.ID || ownerInbox.ScopeRevision == first.ScopeRevision {
		t.Fatalf("another user's notifications were visible: %+v %v", ownerInbox, err)
	}
	for _, request := range []struct {
		actor  Actor
		filter string
		limit  int
	}{{f.actor, "unread", 1}, {h.receiver, "inbox", 1}, {h.receiver, "unread", 2}} {
		_, err = f.s.NotificationOrganizationInbox(ctx, request.actor, request.filter, request.limit, first.NextCursor)
		requireAuthorityStatus(t, err, 400)
	}

	item := secondProject.Items[0]
	_, err = f.s.CommandNotificationInbox(ctx, h.receiver, other.ID, item.ID, NotificationInboxCommand{ProjectID: other.ID, ExpectedVersion: item.Version, Action: "archive", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := f.s.NotificationOrganizationInbox(ctx, h.receiver, "archived", 20, "")
	if err != nil || archived.Total != 1 || archived.Counts != (NotificationInboxCounts{Inbox: 2, Unread: 2, Archived: 1}) || len(archived.Items) != 1 || archived.Items[0].Notification.ID != item.ID || archived.ScopeRevision != first.ScopeRevision {
		t.Fatalf("private project command did not update aggregate: %+v %v", archived, err)
	}
	assertUnread(h.receiver, recipient, 2)
	_, err = f.s.CommandNotificationInbox(ctx, h.receiver, other.ID, item.ID, NotificationInboxCommand{ProjectID: other.ID, ExpectedVersion: item.Version + 1, Action: "restore", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	assertUnread(h.receiver, recipient, 3)
	_, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{"project_id": other.ID, "user_id": recipient, "action": "remove", "expected_version": float64(1), "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	assertUnread(h.receiver, recipient, 2)
	rejected, err := f.s.NotificationOrganizationInbox(ctx, h.receiver, "unread", 1, first.NextCursor)
	requireAuthorityStatus(t, err, 409)
	if rejected.Total != 0 || len(rejected.Items) != 0 || rejected.ScopeRevision != "" {
		t.Fatalf("scope conflict leaked rows or counts: %+v", rejected)
	}
	fresh, err := f.s.NotificationOrganizationInbox(ctx, h.receiver, "archived", 20, "")
	if err != nil || fresh.Total != 0 || fresh.Counts.Archived != 0 || fresh.Items == nil || len(fresh.Items) != 0 || fresh.ScopeRevision == first.ScopeRevision {
		t.Fatalf("revoked project remains visible: %+v %v", fresh, err)
	}
}

func TestNotificationOrganizationInboxBrowserHTTPClosedQueryAndCredentialKinds(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	path := "/api/v1/notification-inbox/organization"
	call := func(method, suffix, bearer string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path+suffix, nil)
		if cookie != nil {
			req.AddCookie(cookie)
			req.Header.Set("X-Suma-Expected-User", f.user.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s => %d want %d: %s", method, suffix, w.Code, status, w.Body.String())
		}
		return w
	}
	w := call(http.MethodGet, "?limit=20&filter=inbox", "", f.cookie, 200)
	var page NotificationOrganizationInboxPage
	if json.Unmarshal(w.Body.Bytes(), &page) != nil || page.Total != 1 || page.Limit != 20 || page.OrganizationID != f.user.TenantID || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected HTTP response: %+v %s", page, w.Body.String())
	}
	for _, suffix := range []string{"?project_id=other", "?user_id=other", "?recipient_user_id=other", "?organization_id=other", "?filter=", "?filter=all", "?filter=inbox&filter=unread", "?limit=01", "?limit=51", "?cursor=", "?cursor=invalid"} {
		call(http.MethodGet, suffix, "", f.cookie, 400)
	}
	w = call(http.MethodPost, "", "", f.cookie, 405)
	if w.Header().Get("Allow") != http.MethodGet {
		t.Fatal("missing exact method contract")
	}
	call(http.MethodGet, "", "", nil, 401)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		call(http.MethodGet, "", issued.Token, nil, 403)
		actor := domainAActor(t, f.s, issued.Token)
		_, err = f.s.NotificationOrganizationInbox(ctx, actor, "", 20, "")
		requireAuthorityStatus(t, err, 403)
	}
}

func TestNotificationOrganizationUnreadSummaryHTTP(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	const path = "/api/v1/notification-inbox/organization/unread-summary"
	call := func(method, suffix, user, bearer string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path+suffix, nil)
		if cookie != nil {
			req.AddCookie(cookie)
			req.Header.Set("X-Suma-Expected-User", user)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s returned %d, want %d: %s", method, suffix, w.Code, status, w.Body.String())
		}
		return w
	}
	assertUnread := func(want int) {
		t.Helper()
		w := call(http.MethodGet, "", f.user.ID, "", f.cookie, 200)
		var got NotificationOrganizationUnreadSummary
		var fields map[string]json.RawMessage
		if json.Unmarshal(w.Body.Bytes(), &got) != nil || json.Unmarshal(w.Body.Bytes(), &fields) != nil || len(fields) != 3 ||
			got != (NotificationOrganizationUnreadSummary{OrganizationID: f.user.TenantID, UserID: f.user.ID, Unread: want}) || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("wrong or cacheable count-only response: %s", w.Body.String())
		}
	}
	assertUnread(1)
	item := inboxPage(t, f, f.actor, "inbox", 20, "").Items[0]
	_, err := f.s.CommandNotificationInbox(ctx, f.actor, f.project, item.ID, NotificationInboxCommand{ProjectID: f.project, ExpectedVersion: item.Version, Action: "read", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	assertUnread(0)
	for _, suffix := range []string{"?", "?project_id=other", "?recipient_user_id=other", "?organization_id=other", "?limit=1"} {
		call(http.MethodGet, suffix, f.user.ID, "", f.cookie, 400)
	}
	w := call(http.MethodPost, "", f.user.ID, "", f.cookie, 405)
	if w.Header().Get("Allow") != http.MethodGet {
		t.Fatal("missing exact method contract")
	}
	call(http.MethodGet, "", "", "", nil, 401)
	call(http.MethodGet, "", h.receiver.credential.subjectID, "", f.cookie, 403)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		call(http.MethodGet, "", "", issued.Token, nil, 403)
	}
}

func TestNotificationOrganizationInboxSameAccountDoesNotCrossTenants(t *testing.T) {
	ctx := context.Background()
	h := newHandoffFixture(t)
	f := h.f
	original, err := f.s.NotificationOrganizationInbox(ctx, f.actor, "", 20, "")
	if err != nil || original.Total != 1 {
		t.Fatalf("missing original organization notification: %+v %v", original, err)
	}
	org, err := f.s.CreateOrganization(ctx, f.actor, f.body(map[string]any{"name": "Second inbox organization"}))
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := f.s.SwitchOrganization(ctx, f.actor, f.body(map[string]any{"organization_id": org["id"]}))
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	project, err := f.s.CreateProject(ctx, actor, f.body(map[string]any{"title": "Second tenant project"}))
	if err != nil {
		t.Fatal(err)
	}
	member, err := f.s.Create(ctx, actor, project.ID, "members", f.body(map[string]any{"title": "Same account", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}))
	if err != nil {
		t.Fatal(err)
	}
	task, err := f.s.Create(ctx, actor, project.ID, "tasks", f.body(map[string]any{"title": "Second tenant task", "owner_id": member.ID}))
	if err != nil {
		t.Fatal(err)
	}
	other, err := f.s.NotificationOrganizationInbox(ctx, actor, "", 20, "")
	if err != nil || other.UserID != original.UserID || other.OrganizationID == original.OrganizationID || other.Total != 1 || len(other.Items) != 1 || other.Items[0].Notification.TaskID != task.ID || other.ScopeRevision == original.ScopeRevision {
		t.Fatalf("wrong tenant projection: %+v %v", other, err)
	}
}
