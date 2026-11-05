package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestSharedTaskViewsSyncHTTP(t *testing.T) {
	f := newFixture(t)
	path := "/task-views/sync?project_id=" + f.project
	req := httptest.NewRequest(http.MethodGet, "http://suma.test/api/v1"+path, nil)
	req.AddCookie(f.cookie)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	var reply map[string]any
	if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" ||
		json.Unmarshal(w.Body.Bytes(), &reply) != nil || len(reply) != 2 || reply["project_id"] != f.project {
		t.Fatalf("expected a private, scoped sync response, got %d: %s", w.Code, w.Body.String())
	}
	revision, ok := reply["revision"].(string)
	if !ok || !regexp.MustCompile("^[a-f0-9]{64}$").MatchString(revision) {
		t.Fatalf("invalid opaque sync revision: %v", reply["revision"])
	}
	for _, query := range []string{"", "?project_id=", "?project_id=" + f.project + "&project_id=" + f.project, "?project_id=" + f.project + "&limit=20", "?project_id=" + f.project + "&cursor=x"} {
		f.call("GET", "/task-views/sync"+query, nil, 400)
	}
	f.call("POST", "/task-views/sync", nil, 405)
	f.call("GET", "/task-views/sync/?project_id="+f.project, nil, 404)
	f.call("POST", "/task-views/sync/archive", nil, 404)
	if got := f.call("GET", path, nil, 200)["revision"]; got != revision {
		t.Fatal("read-only sync changed the revision")
	}
}

func TestSharedTaskViewsSyncTracksAuthorizedChangesBeyondFirstPage(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	revision := func(actor Actor) string {
		t.Helper()
		out, err := f.s.SyncSharedTaskViews(ctx, actor, f.project)
		if err != nil || out.ProjectID != f.project {
			t.Fatal(out, err)
		}
		return out.Revision
	}
	ownerBefore, receiverBefore := revision(f.actor), revision(h.receiver)
	if ownerBefore == receiverBefore {
		t.Fatal("different accounts share the same sync revision")
	}
	privateReq := sharedViewRequest(f.project, "private")
	private, err := f.s.CreateSharedTaskView(ctx, h.receiver, f.project, privateReq)
	if err != nil {
		t.Fatal(err)
	}
	receiverAfter := revision(h.receiver)
	if revision(f.actor) != ownerBefore || receiverAfter == receiverBefore {
		t.Fatal("a private view changed another account's revision or was not visible to its owner")
	}
	if _, err = f.s.CreateSharedTaskView(ctx, h.receiver, f.project, privateReq); err != nil || revision(h.receiver) != receiverAfter {
		t.Fatal("replay changed the sync revision", err)
	}
	if _, err = f.s.ArchiveSharedTaskView(ctx, h.receiver, f.project, private.ID, SharedTaskViewArchiveRequest{ProjectID: f.project, ExpectedVersion: private.Version, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	if revision(f.actor) != ownerBefore || revision(h.receiver) == receiverBefore || revision(h.receiver) == receiverAfter {
		t.Fatal("a private archive failed to retain an owner-only revision")
	}
	for i := 0; i < 2; i++ {
		if _, err := f.s.CreateSharedTaskView(ctx, f.actor, f.project, sharedViewRequest(f.project, "project")); err != nil {
			t.Fatal(err)
		}
	}
	first, err := f.s.ListSharedTaskViews(ctx, f.actor, f.project, 1, "")
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatal(first, err)
	}
	next, err := f.s.ListSharedTaskViews(ctx, f.actor, f.project, 1, first.NextCursor)
	if err != nil || len(next.Items) != 1 {
		t.Fatal(next, err)
	}
	older := next.Items[0]
	sharedBefore := revision(h.receiver)
	if _, err = f.s.UpdateSharedTaskView(ctx, f.actor, f.project, older.ID, SharedTaskViewUpdateRequest{
		ProjectID: f.project, Name: "later page edit", Visibility: older.Visibility, Preferences: older.Preferences, ExpectedVersion: older.Version, IdempotencyKey: newID(),
	}); err != nil {
		t.Fatal(err)
	}
	after := revision(h.receiver)
	if after == sharedBefore {
		t.Fatal("an edit beyond the first page did not invalidate the revision")
	}
	if _, err = f.s.ArchiveSharedTaskView(ctx, f.actor, f.project, older.ID, SharedTaskViewArchiveRequest{ProjectID: f.project, ExpectedVersion: older.Version + 1, IdempotencyKey: newID()}); err != nil {
		t.Fatal(err)
	}
	if revision(h.receiver) == after {
		t.Fatal("the archive did not invalidate the revision")
	}
	last, err := f.s.ListSharedTaskViews(ctx, f.actor, f.project, 1, first.NextCursor)
	if err != nil || last.Total != 1 || len(last.Items) != 0 || last.NextCursor != "" {
		t.Fatal("existing cursor should still work and omit the tombstone", last, err)
	}
}

func TestSharedTaskViewsSyncRechecksCurrentScopeAndManagement(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	path := "/task-views/sync?project_id=" + f.project
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "task-view-sync", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		if scope != "api" {
			f.bearer("GET", path, issued.Token, nil, 403)
			continue
		}
		if got := f.bearer("GET", path, issued.Token, nil, 200)["revision"]; got != f.call("GET", path, nil, 200)["revision"] {
			t.Fatal("equally authorized sessions for one account have different revisions")
		}
		f.bearer("GET", "/task-views/sync?project_id=another-project", issued.Token, nil, 403)
	}

	out, err := f.s.SyncSharedTaskViews(ctx, h.receiver, f.project)
	if err != nil {
		t.Fatal(err)
	}
	user := h.receiver.Projection().ID
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, user); err != nil {
		t.Fatal(err)
	}
	viewer, err := f.s.SyncSharedTaskViews(ctx, h.receiver, f.project)
	if err != nil || viewer.Revision == out.Revision {
		t.Fatal("project write downgrade did not invalidate the revision", viewer, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='admin' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, user); err != nil {
		t.Fatal(err)
	}
	admin, err := f.s.SyncSharedTaskViews(ctx, h.receiver, f.project)
	if err != nil || admin.Revision == out.Revision || admin.Revision == viewer.Revision {
		t.Fatal("org administrator scope did not invalidate the revision", admin, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, user); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": user, "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	_, err = f.s.SyncSharedTaskViews(ctx, h.receiver, f.project)
	requireAuthorityStatus(t, err, 403)
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	f.call("GET", path, nil, 401)
}
