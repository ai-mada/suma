package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func notificationInboxBatchFixture(t *testing.T) (*handoffFixture, []NotificationInboxBatchTarget) {
	t.Helper()
	ctx := context.Background()
	h := newHandoffFixture(t)
	f := h.f
	recipient := h.receiver.credential.subjectID
	grantViewer := func(project string, version float64) {
		t.Helper()
		_, err := f.s.SaveProjectAccess(ctx, f.actor, map[string]any{
			"project_id": project, "user_id": recipient, "role": "viewer",
			"expected_version": version, "idempotency_key": newID(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	grantViewer(f.project, 1)
	inboxCreateTask(t, h, "First batch task", h.resource.ID, newID())
	second, err := f.s.CreateProject(ctx, f.actor, f.body(map[string]any{"title": "Second batch project"}))
	if err != nil {
		t.Fatal(err)
	}
	grantViewer(second.ID, 0)
	member, err := f.s.Create(ctx, f.actor, second.ID, "members", f.body(map[string]any{
		"title": "Canonical batch recipient", "kind": "human", "data": map[string]any{"user_id": recipient},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Create(ctx, f.actor, second.ID, "tasks", f.body(map[string]any{"title": "Second batch task", "owner_id": member.ID})); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.NotificationOrganizationInbox(ctx, h.receiver, "inbox", 20, "")
	if err != nil || len(page.Items) != 2 || page.Items[0].ProjectID == page.Items[1].ProjectID {
		t.Fatalf("missing real cross-project viewer inbox: %+v %v", page, err)
	}
	items := make([]NotificationInboxBatchTarget, 0, len(page.Items))
	for _, row := range page.Items {
		items = append(items, NotificationInboxBatchTarget{ProjectID: row.ProjectID, NotificationID: row.Notification.ID, ExpectedVersion: row.Notification.Version})
	}
	return h, items
}

func TestNotificationInboxBatchViewerTransitionsAndReplay(t *testing.T) {
	ctx := context.Background()
	h, items := notificationInboxBatchFixture(t)
	f := h.f
	original := NotificationInboxBatchCommand{Items: append([]NotificationInboxBatchTarget(nil), items...), Action: "archive", IdempotencyKey: newID()}
	archived, err := f.s.CommandNotificationInboxBatch(ctx, h.receiver, original)
	if err != nil || len(archived.Results) != 2 || archived.Action != original.Action || archived.IdempotencyKey != original.IdempotencyKey {
		t.Fatalf("archive receipt: %+v %v", archived, err)
	}
	for i, result := range archived.Results {
		if result.ProjectID != items[i].ProjectID || result.Receipt.NotificationID != items[i].NotificationID ||
			result.Receipt.Version != 2 || !result.Receipt.Archived || result.Receipt.Read || result.Receipt.ArchivedAt == "" ||
			result.Receipt.Action != archived.Action || result.Receipt.IdempotencyKey != archived.IdempotencyKey {
			t.Fatalf("wrong ordered item receipt: %+v", result)
		}
		items[i].ExpectedVersion = result.Receipt.Version
	}
	for _, step := range []struct {
		action         string
		version        int
		read, archived bool
	}{
		{action: "archive", version: 2, archived: true},
		{action: "read", version: 3, read: true, archived: true},
		{action: "restore", version: 4, read: true},
		{action: "unread", version: 5},
	} {
		req := NotificationInboxBatchCommand{Items: items, Action: step.action, IdempotencyKey: newID()}
		out, err := f.s.CommandNotificationInboxBatch(ctx, h.receiver, req)
		if err != nil || len(out.Results) != 2 {
			t.Fatalf("%s receipt: %+v %v", step.action, out, err)
		}
		for i, result := range out.Results {
			if result.Receipt.Version != step.version || result.Receipt.Read != step.read || result.Receipt.Archived != step.archived {
				t.Fatalf("wrong state for %s: %+v", step.action, result)
			}
			items[i].ExpectedVersion = result.Receipt.Version
		}
	}
	replay, err := f.s.CommandNotificationInboxBatch(ctx, h.receiver, original)
	if err != nil || !reflect.DeepEqual(replay, archived) {
		t.Fatalf("original receipt was not preserved: %+v %v", replay, err)
	}
	changed := original
	changed.Action = "read"
	_, err = f.s.CommandNotificationInboxBatch(ctx, h.receiver, changed)
	requireAuthorityStatus(t, err, 409)
	page, err := f.s.NotificationOrganizationInbox(ctx, h.receiver, "inbox", 20, "")
	if err != nil || page.Counts != (NotificationInboxCounts{Inbox: 2, Unread: 2}) || len(page.Items) != 2 || page.Items[0].Notification.Version != 5 || page.Items[1].Notification.Version != 5 {
		t.Fatalf("replay changed private state: %+v %v", page, err)
	}

	var secondProject string
	for _, item := range items {
		if item.ProjectID != f.project {
			secondProject = item.ProjectID
		}
	}
	_, err = f.s.SaveProjectAccess(ctx, f.actor, map[string]any{
		"project_id": secondProject, "user_id": h.receiver.credential.subjectID, "action": "remove",
		"expected_version": float64(1), "idempotency_key": newID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err = f.s.CommandNotificationInboxBatch(ctx, h.receiver, original)
	requireAuthorityStatus(t, err, 403)
	if !reflect.DeepEqual(replay, NotificationInboxBatchReceipt{}) {
		t.Fatalf("revoked receipt exposed private results: %+v", replay)
	}
}

func TestNotificationInboxBatchConflictAndFailedWriteAreAtomic(t *testing.T) {
	ctx := context.Background()
	h, items := notificationInboxBatchFixture(t)
	f := h.f
	original := append([]NotificationInboxBatchTarget(nil), items...)
	req := NotificationInboxBatchCommand{Items: items, Action: "archive", IdempotencyKey: newID()}
	assertUnchanged := func() {
		t.Helper()
		for _, item := range original {
			var version int
			var archived string
			err := f.s.DB.QueryRow(`SELECT version,archived_at FROM suma_notification_inbox WHERE id=?`, item.NotificationID).Scan(&version, &archived)
			if err != nil || version != item.ExpectedVersion || archived != "" {
				t.Fatalf("failed batch changed %s: version=%d archived=%q %v", item.NotificationID, version, archived, err)
			}
		}
		var receipts int
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, organizationScope, req.IdempotencyKey).Scan(&receipts); err != nil || receipts != 0 {
			t.Fatalf("failed batch retained a receipt: %d %v", receipts, err)
		}
	}
	items[1].ExpectedVersion++
	_, err := f.s.CommandNotificationInboxBatch(ctx, h.receiver, req)
	requireAuthorityStatus(t, err, 409)
	assertUnchanged()
	items[1] = original[1]
	items[1].ProjectID = items[0].ProjectID
	_, err = f.s.CommandNotificationInboxBatch(ctx, h.receiver, req)
	requireAuthorityStatus(t, err, 404)
	assertUnchanged()
	items[1] = original[1]
	owner := inboxPage(t, f, f.actor, "inbox", 20, "").Items[0]
	items[1] = NotificationInboxBatchTarget{ProjectID: f.project, NotificationID: owner.ID, ExpectedVersion: owner.Version}
	_, err = f.s.CommandNotificationInboxBatch(ctx, h.receiver, req)
	requireAuthorityStatus(t, err, 404)
	assertUnchanged()
	items[1] = original[1]
	// A real statement failure on the second row rolls back the already
	// processed first row and leaves the original operation key available.
	if _, err = f.s.DB.Exec(`CREATE TRIGGER batch_notify_failure BEFORE UPDATE ON suma_notification_inbox WHEN NEW.id='` + items[1].NotificationID + `' BEGIN SELECT RAISE(ABORT,'second notification update failed'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CommandNotificationInboxBatch(ctx, h.receiver, req); err == nil {
		t.Fatal("batch unexpectedly succeeded")
	}
	assertUnchanged()
	if _, err = f.s.DB.Exec(`DROP TRIGGER batch_notify_failure`); err != nil {
		t.Fatal(err)
	}
	out, err := f.s.CommandNotificationInboxBatch(ctx, h.receiver, req)
	if err != nil || len(out.Results) != 2 {
		t.Fatalf("original operation failed after rollback: %+v %v", out, err)
	}
}

func TestNotificationInboxBatchBrowserHTTPContract(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	item := inboxPage(t, f, f.actor, "inbox", 20, "").Items[0]
	target := NotificationInboxBatchTarget{ProjectID: f.project, NotificationID: item.ID, ExpectedVersion: item.Version}
	batch := NotificationInboxBatchCommand{Items: []NotificationInboxBatchTarget{target}, Action: "read", IdempotencyKey: newID()}
	body := stringify(batch)
	const route = "/api/v1/notification-inbox/batch-commands"
	call := func(method, query, body, token string, cookie *http.Cookie, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, route+query, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
			req.Header.Set("X-Suma-Expected-User", f.user.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s => %d want %d: %s", method, query, w.Code, status, w.Body.String())
		}
		return w
	}

	for _, bad := range []string{
		strings.Replace(body, `"items":`, `"other":`, 1),
		strings.Replace(body, `"project_id":`, `"project_id":"forged","project_id":`, 1),
		strings.Replace(body, `"notification_id":`, `"Notification_ID":`, 1),
		strings.Replace(body, `"expected_version":1`, `"expected_version":null`, 1),
		strings.Replace(body, `"expected_version":1`, `"expected_version":1,"extra":true`, 1),
		strings.Replace(body, stringify(target), `null`, 1),
		strings.Replace(body, stringify(target), stringify(target)+","+stringify(target), 1),
		strings.Replace(body, stringify(target), strings.Repeat(stringify(target)+",", notificationInboxBatchLimit)+stringify(target), 1),
		strings.Replace(body, stringify(target), "", 1),
		strings.Replace(body, `"action":"read"`, `"action":"remove"`, 1),
	} {
		call(http.MethodPost, "", bad, "", f.cookie, 400)
	}
	for _, query := range []string{"?project_id=scope", "?"} {
		call(http.MethodPost, query, body, "", f.cookie, 400)
	}
	w := call(http.MethodGet, "", "", "", f.cookie, 405)
	if w.Header().Get("Allow") != http.MethodPost {
		t.Fatal("missing exact method contract")
	}
	call(http.MethodPost, "", body, "", nil, 401)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		call(http.MethodPost, "", body, issued.Token, nil, 403)
	}
	w = call(http.MethodPost, "", body, "", f.cookie, 200)
	var receipt NotificationInboxBatchReceipt
	if json.Unmarshal(w.Body.Bytes(), &receipt) != nil || len(receipt.Results) != 1 || !receipt.Results[0].Receipt.Read ||
		receipt.Results[0].Receipt.Version != 2 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}
