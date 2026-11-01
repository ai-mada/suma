package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func toolListProfile193(t *testing.T, f *fixture, name, command, state string) ToolProfile {
	t.Helper()
	p := toolTestProfile()
	p.CommandID = command
	p.ProfileSHA256 = digest(command)
	p.ToolVersion = "unsearched-version-marker"
	out, err := f.s.CreateToolProfile(context.Background(), f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: name, Profile: p, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if state != "paused" {
		action := "enable"
		if state == "revoked" {
			action = "revoke"
		}
		out, err = f.s.CommandToolProfile(context.Background(), f.actor, f.project, out.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: out.Version, Action: action, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestToolProfileListFiltersPublicFieldsAndCanonicalCursor(t *testing.T) {
	f, _, _ := toolFixture(t)
	ctx := context.Background()
	unicode := toolListProfile193(t, f, "ÁLPHA 中文", "unicode-command", "paused")
	toolListProfile193(t, f, "Alpha compile", "alpha-compile", "enabled")
	toolListProfile193(t, f, "Alpha extra", "alpha-extra", "paused")
	retired := toolListProfile193(t, f, "Alpha retired", "alpha-retired", "revoked")
	literal := toolListProfile193(t, f, "Percent %_ tool", "literal-command", "paused")
	before := expertCount(t, f, "suma_operations")
	current, err := f.s.ListToolProfiles(ctx, f.actor, f.project, ToolProfileListQuery{Limit: 200})
	if err != nil || current.Total != 5 || len(current.Items) != 5 {
		t.Fatal("default current list includes removed records", current, err)
	}
	for _, p := range current.Items {
		if p.ID == retired.ID {
			t.Fatal("revoked profile remained in default active view")
		}
	}
	all, err := f.s.ListToolProfiles(ctx, f.actor, f.project, ToolProfileListQuery{State: "all", Limit: 200})
	if err != nil || all.Total != 6 {
		t.Fatal(all, err)
	}
	removed, err := f.s.ListToolProfiles(ctx, f.actor, f.project, ToolProfileListQuery{State: "revoked", Limit: 200})
	if err != nil || removed.Total != 1 || removed.Items[0].ID != retired.ID {
		t.Fatal("removed history missing", removed, err)
	}
	for query, want := range map[string]string{"álpha": unicode.ID, "中文": unicode.ID, "%_": literal.ID} {
		page, err := f.s.ListToolProfiles(ctx, f.actor, f.project, ToolProfileListQuery{Query: query, Limit: 200})
		if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != want {
			t.Fatal("Unicode/literal public contains changed across stores", query, page, err)
		}
	}
	for _, query := range []string{"unsearched-version-marker", strings.Repeat("a", 24)} {
		page, err := f.s.ListToolProfiles(ctx, f.actor, f.project, ToolProfileListQuery{Query: query, Limit: 200})
		if err != nil || page.Total != 0 {
			t.Fatal("search examined raw profile JSON instead of the five public fields", query, page, err)
		}
	}
	first, err := f.s.ListToolProfiles(ctx, f.actor, f.project, ToolProfileListQuery{Query: "  ALPHA  ", Limit: 1})
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatal(first, err)
	}
	next, err := f.s.ListToolProfiles(ctx, f.actor, f.project, ToolProfileListQuery{State: "active", Query: "alpha", Limit: 1, Cursor: first.NextCursor})
	if err != nil || next.Total != 2 || len(next.Items) != 1 || next.Items[0].ID == first.Items[0].ID || next.NextCursor != "" {
		t.Fatal("normalized query did not preserve exact page boundary", next, err)
	}
	for _, q := range []ToolProfileListQuery{{State: "all", Query: "alpha", Limit: 1, Cursor: first.NextCursor}, {Query: "native", Limit: 1, Cursor: first.NextCursor}} {
		if _, err = f.s.ListToolProfiles(ctx, f.actor, f.project, q); err == nil {
			t.Fatal("cursor borrowed a different filter")
		}
	}
	f.call("GET", "/tool-profiles?project_id="+f.project+"&state=enabled&query="+url.QueryEscape("ALPHA")+"&limit=1", nil, 200)
	f.call("GET", "/tool-profiles?project_id="+f.project+"&state=active&state=all", nil, 400)
	f.call("GET", "/tool-profiles?project_id="+f.project+"&limit=0", nil, 400)
	for _, locale := range []string{"zh-CN", "en-US"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/tool-profiles?project_id="+f.project+"&state=invalid", nil)
		r.AddCookie(f.cookie)
		r.Header.Set("Accept-Language", locale)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		var body struct {
			Error struct {
				MessageKey string `json:"message_key"`
				Message    string `json:"message"`
				Localized  bool   `json:"localized"`
			}
		}
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 400 || body.Error.MessageKey != "validation.tool_profile_filter" || !body.Error.Localized || locale == "en-US" && body.Error.Message != "Invalid tool list filters." {
			t.Fatal("filter diagnostic lost request language", locale, w.Code, w.Body.String())
		}
	}
	if expertCount(t, f, "suma_operations") != before {
		t.Fatal("read filter created operation receipts")
	}
}

func TestToolProfileListCurrentAuthorityPrecedesFilterAndActorCursor(t *testing.T) {
	f, _, _ := toolFixture(t)
	toolListProfile193(t, f, "Another current tool", "second-command", "paused")
	ctx := context.Background()
	page, err := f.s.ListToolProfiles(ctx, f.actor, f.project, ToolProfileListQuery{Limit: 1})
	if err != nil || page.NextCursor == "" {
		t.Fatal(err)
	}
	reviewer := invitedReviewer(t, f)
	if _, err = f.s.ListToolProfiles(ctx, reviewer, f.project, ToolProfileListQuery{Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("another current project reader borrowed the actor cursor")
	}
	_, err = f.s.ListToolProfiles(ctx, Actor{}, f.project, ToolProfileListQuery{State: "invalid", Query: strings.Repeat("中", 41), Cursor: "invalid"})
	var api *APIError
	if err == nil || errors.As(err, &api) && api.Status == 400 {
		t.Fatal("filter validation ran before current actor/project authorization", err)
	}
	for _, q := range []ToolProfileListQuery{{State: "invalid"}, {Query: strings.Repeat("中", 41)}, {Query: string([]byte{0xff})}, {Query: "two\nlines"}, {Limit: 201}, {Cursor: strings.Repeat("a", 1025)}} {
		if _, err = f.s.ListToolProfiles(ctx, f.actor, f.project, q); err == nil {
			t.Fatal("invalid current filter accepted", q)
		}
	}
}
