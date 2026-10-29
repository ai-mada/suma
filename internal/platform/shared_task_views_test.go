package platform

import (
	"context"
	"strings"
	"testing"
)

func sharedViewRequest(project, visibility string) SharedTaskViewCreateRequest {
	return SharedTaskViewCreateRequest{ProjectID: project, Name: "待处理任务", Visibility: visibility, Preferences: SharedTaskViewPreferences{Q: "100%_! literal", Unassigned: true}, IdempotencyKey: newID()}
}
func TestSharedTaskViewsLifecycleCASAndTombstone(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	req := sharedViewRequest(f.project, "project")
	v, e := f.s.CreateSharedTaskView(ctx, f.actor, f.project, req)
	if e != nil || v.Version != 1 || !v.CanEdit {
		t.Fatal(v, e)
	}
	replay, e := f.s.CreateSharedTaskView(ctx, f.actor, f.project, req)
	if e != nil || replay.ID != v.ID {
		t.Fatal(replay, e)
	}
	update := SharedTaskViewUpdateRequest{ProjectID: f.project, Name: "更新", Visibility: v.Visibility, Preferences: v.Preferences, ExpectedVersion: 1, IdempotencyKey: newID()}
	next, e := f.s.UpdateSharedTaskView(ctx, f.actor, f.project, v.ID, update)
	if e != nil || next.Version != 2 {
		t.Fatal(next, e)
	}
	stale := update
	stale.IdempotencyKey = newID()
	_, e = f.s.UpdateSharedTaskView(ctx, f.actor, f.project, v.ID, stale)
	requireAuthorityStatus(t, e, 409)
	stale.ExpectedVersion = 2
	stale.Visibility = "private"
	_, e = f.s.UpdateSharedTaskView(ctx, f.actor, f.project, v.ID, stale)
	requireAuthorityStatus(t, e, 400)
	arch := SharedTaskViewArchiveRequest{ProjectID: f.project, ExpectedVersion: 2, IdempotencyKey: newID()}
	tomb, e := f.s.ArchiveSharedTaskView(ctx, f.actor, f.project, v.ID, arch)
	if e != nil || tomb.State != "archived" || tomb.CanEdit {
		t.Fatal(tomb, e)
	}
	if _, e = f.s.ArchiveSharedTaskView(ctx, f.actor, f.project, v.ID, arch); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.CreateSharedTaskView(ctx, f.actor, f.project, req)
	requireAuthorityStatus(t, e, 409)
	_, e = f.s.UpdateSharedTaskView(ctx, f.actor, f.project, v.ID, update)
	requireAuthorityStatus(t, e, 409)
	page, e := f.s.ListSharedTaskViews(ctx, f.actor, f.project, 20, "")
	if e != nil || page.Total != 0 {
		t.Fatal(page, e)
	}
	tomb, e = f.s.GetSharedTaskView(ctx, f.actor, f.project, v.ID)
	if e != nil || tomb.State != "archived" {
		t.Fatal(tomb, e)
	}
}
func TestSharedTaskViewsPrivateIsolationAndSharedPermissions(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	private, e := f.s.CreateSharedTaskView(ctx, h.receiver, f.project, sharedViewRequest(f.project, "private"))
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.GetSharedTaskView(ctx, f.actor, f.project, private.ID)
	requireAuthorityStatus(t, e, 404)
	page, e := f.s.ListSharedTaskViews(ctx, f.actor, f.project, 20, "")
	if e != nil || page.Total != 0 {
		t.Fatal(page, e)
	}
	shared, e := f.s.CreateSharedTaskView(ctx, f.actor, f.project, sharedViewRequest(f.project, "project"))
	if e != nil {
		t.Fatal(e)
	}
	read, e := f.s.GetSharedTaskView(ctx, h.receiver, f.project, shared.ID)
	if e != nil || read.CanEdit || read.CanDelete {
		t.Fatal(read, e)
	}
	_, e = f.s.ArchiveSharedTaskView(ctx, h.receiver, f.project, shared.ID, SharedTaskViewArchiveRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	requireAuthorityStatus(t, e, 403)
	owned, e := f.s.CreateSharedTaskView(ctx, h.receiver, f.project, sharedViewRequest(f.project, "project"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ArchiveSharedTaskView(ctx, f.actor, f.project, owned.ID, SharedTaskViewArchiveRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}); e != nil {
		t.Fatal("current manager could not archive shared", e)
	}
}
func TestSharedTaskViewsViewerPrivateAndCurrentReplayAuthority(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	req := sharedViewRequest(f.project, "project")
	shared, e := f.s.CreateSharedTaskView(ctx, h.receiver, f.project, req)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.Projection().ID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.CreateSharedTaskView(ctx, h.receiver, f.project, req)
	requireAuthorityStatus(t, e, 403)
	read, e := f.s.GetSharedTaskView(ctx, h.receiver, f.project, shared.ID)
	if e != nil || read.CanEdit {
		t.Fatal(read, e)
	}
	privateReq := sharedViewRequest(f.project, "private")
	private, e := f.s.CreateSharedTaskView(ctx, h.receiver, f.project, privateReq)
	if e != nil || !private.CanEdit {
		t.Fatal(private, e)
	}
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": h.receiver.Projection().ID, "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	_, e = f.s.CreateSharedTaskView(ctx, h.receiver, f.project, privateReq)
	requireAuthorityStatus(t, e, 403)
	_, e = f.s.GetSharedTaskView(ctx, h.receiver, f.project, private.ID)
	requireAuthorityStatus(t, e, 403)
}
func TestSharedTaskViewsBoundPagingAndCredentialIsolation(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		if _, e := f.s.CreateSharedTaskView(ctx, f.actor, f.project, sharedViewRequest(f.project, "project")); e != nil {
			t.Fatal(e)
		}
	}
	page, e := f.s.ListSharedTaskViews(ctx, f.actor, f.project, 2, "")
	if e != nil || page.Total != 4 || len(page.Items) != 2 || page.NextCursor == "" {
		t.Fatal(page, e)
	}
	next, e := f.s.ListSharedTaskViews(ctx, f.actor, f.project, 2, page.NextCursor)
	if e != nil || len(next.Items) != 2 || next.Items[0].ID <= page.Items[1].ID || next.NextCursor != "" {
		t.Fatal(next, e)
	}
	_, e = f.s.ListSharedTaskViews(ctx, h.receiver, f.project, 2, page.NextCursor)
	requireAuthorityStatus(t, e, 400)
	_, e = f.s.ListSharedTaskViews(ctx, f.actor, f.project, 3, page.NextCursor)
	requireAuthorityStatus(t, e, 400)
	for _, scope := range []string{"runner", "agent"} {
		issued, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "task-view", IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		actor := domainAActor(t, f.s, issued.Token)
		_, e = f.s.ListSharedTaskViews(ctx, actor, f.project, 20, "")
		requireAuthorityStatus(t, e, 403)
	}
}
func TestSharedTaskViewsStrictHTTPAndPreferenceValidation(t *testing.T) {
	f := newFixture(t)
	req := f.body(map[string]any{"name": "我的视图", "visibility": "private", "preferences": map[string]any{"q": "literal", "status": "all", "sort": "title_asc", "layout": "board", "group_by": "owner"}})
	saved := f.call("POST", "/task-views", req, 201)
	id := saved["id"].(string)
	f.call("GET", "/task-views?project_id="+f.project+"&project_id="+f.project, nil, 400)
	f.call("GET", "/task-views?project_id="+f.project+"&limit=01", nil, 400)
	f.call("GET", "/task-views/"+id+"?project_id="+f.project, nil, 200)
	for _, bad := range []map[string]any{{"cursor": "forbidden"}, {"owner_id": "x", "unassigned": true}, {"sort": "random"}, {"layout": "timeline"}, {"q": strings.Repeat("中", 201)}} {
		body := f.body(map[string]any{"name": "bad", "visibility": "private", "preferences": bad})
		f.call("POST", "/task-views", body, 400)
	}
	req["idempotency_key"] = newID()
	req["name"] = "changed"
	f.call("POST", "/task-views?project_id="+f.project, req, 400)
}
func TestSharedTaskViewsPrivateActivityNeverLeaksNames(t *testing.T) {
	f := newFixture(t)
	req := sharedViewRequest(f.project, "private")
	req.Name = "sensitive private name"
	if _, e := f.s.CreateSharedTaskView(context.Background(), f.actor, f.project, req); e != nil {
		t.Fatal(e)
	}
	var count int
	if e := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity WHERE title=?`, req.Name).Scan(&count); e != nil || count != 0 {
		t.Fatal("private title leaked", count, e)
	}
}
func TestSharedTaskViewsIdempotencyBindsPayloadAndCredential(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	req := sharedViewRequest(f.project, "private")
	v, e := f.s.CreateSharedTaskView(ctx, f.actor, f.project, req)
	if e != nil {
		t.Fatal(e)
	}
	altered := req
	altered.Name = "different"
	_, e = f.s.CreateSharedTaskView(ctx, f.actor, f.project, altered)
	requireAuthorityStatus(t, e, 409)
	issued, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", Label: "second device", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	actor := domainAActor(t, f.s, issued.Token)
	same, e := f.s.GetSharedTaskView(ctx, actor, f.project, v.ID)
	if e != nil || same.ID != v.ID || !same.CanEdit {
		t.Fatal("same account device cannot read", same, e)
	}
	if e = logoutCredentialForTest(ctx, f.s, f.cookie.Value); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.CreateSharedTaskView(ctx, f.actor, f.project, req)
	requireAuthorityStatus(t, e, 401)
	if _, e = f.s.GetSharedTaskView(ctx, actor, f.project, v.ID); e != nil {
		t.Fatal("independent live credential lost access", e)
	}
}
func TestSharedTaskViewsReviewerCannotManageAnotherAuthorsView(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	v, e := f.s.CreateSharedTaskView(ctx, f.actor, f.project, sharedViewRequest(f.project, "project"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_project_members SET role='reviewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.Projection().ID); e != nil {
		t.Fatal(e)
	}
	got, e := f.s.GetSharedTaskView(ctx, h.receiver, f.project, v.ID)
	if e != nil || got.CanEdit || got.CanDelete {
		t.Fatal("review approval is not view management", got, e)
	}
	_, e = f.s.ArchiveSharedTaskView(ctx, h.receiver, f.project, v.ID, SharedTaskViewArchiveRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	requireAuthorityStatus(t, e, 403)
}
