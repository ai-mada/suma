package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdapterCatalogCurrentPermissionAndMachineIsolation(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	page, e := f.s.ListAdapterCatalog(ctx, h.receiver, f.project)
	if e != nil || len(page.Items) == 0 || page.ProjectID != f.project || !validReleaseTime(page.AsOf) {
		t.Fatal(page, e)
	}
	for _, scope := range []string{"runner", "agent"} {
		issued, e := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, Label: "catalog", IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.s.ListAdapterCatalog(ctx, domainAActor(t, f.s, issued.Token), f.project)
		requireAuthorityStatus(t, e, 403)
	}
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": h.receiver.Projection().ID, "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	_, e = f.s.ListAdapterCatalog(ctx, h.receiver, f.project)
	requireAuthorityStatus(t, e, 403)
}
func TestAdapterCatalogHTTPClosedQueryAndNoStore(t *testing.T) {
	f := newFixture(t)
	handler := &Server{Service: f.s}
	for _, raw := range []string{"?project_id=" + f.project, "?project_id=" + f.project + "&worker_id=x", "?project_id=" + f.project + "&project_id=" + f.project, "?project_id=other"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/adapter-catalog"+raw, nil)
		w := httptest.NewRecorder()
		handler.adapterCatalogHTTP(w, r, f.actor, f.project)
		expected := 400
		if raw == "?project_id="+f.project {
			expected = 200
		}
		if w.Code != expected || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(raw, w.Code, w.Body.String())
		}
	}
	w := httptest.NewRecorder()
	handler.adapterCatalogHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/adapter-catalog?project_id="+f.project, nil), f.actor, f.project)
	if w.Code != 405 {
		t.Fatal(w.Code)
	}
}

func TestAdapterCatalogActualRouteAndLogout(t *testing.T) {
	f := newFixture(t)
	page := f.call("GET", "/adapter-catalog?project_id="+f.project, nil, 200)
	// Explicit public entrypoints, not a count copied from the implementation.
	// Native CLI/SDK execution and inbound client guides are distinct products.
	want := map[string]bool{
		"suma-native": true, "command-cli": true, "codex-jsonl": true, "codex-app-server": true,
		"opencode-sdk": true, "octop-acp": true, "dsh-multica": true, "codebuddy-cli": true,
		"claude-code-cli": true, "claude-code-sdk": true, "qoder-acp": true, "cursor-cli": true,
		"cursor-client": true, "claude-code-client": true, "codex-client": true, "opencode-client": true,
		"qoder-client": true, "zed-client": true, "workbuddy-connector": true,
		"harness-guide": true, "workbuddy-cloud": true,
	}
	items, ok := page["items"].([]any)
	if page["project_id"] != f.project || !ok || len(items) != len(want) {
		t.Fatalf("catalog route returned project=%v and %d items, expected the current public entrypoints", page["project_id"], len(items))
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		id, _ := item["id"].(string)
		if !ok || !want[id] {
			t.Fatalf("unexpected or duplicate public adapter: %v", raw)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatal("catalog omitted public entrypoints", want)
	}
	f.call("GET", "/adapter-catalog?project_id="+f.project+"&worker_id=anything", nil, 400)
	f.call("GET", "/adapter-catalog", nil, 400)
	if e := logoutCredentialForTest(context.Background(), f.s, f.cookie.Value); e != nil {
		t.Fatal(e)
	}
	f.call("GET", "/adapter-catalog?project_id="+f.project, nil, 401)
}
