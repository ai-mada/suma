package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

func discoveryPublish196(t *testing.T, f *fixture, request ProjectKnowledgeWriteRequest) ProjectKnowledgeDetail {
	t.Helper()
	created, e := f.s.CreateProjectKnowledge(context.Background(), f.actor, f.project, request)
	if e != nil {
		t.Fatal(e)
	}
	published, e := f.s.CommandProjectKnowledge(context.Background(), f.actor, f.project, created.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: created.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	return published
}
func TestKnowledgeDiscoveryPublishedBodyDraftIsolationAndExactReferences(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	request := knowledgeDraft(f, "memory", "Reviewed original title", strings.Repeat("稳定开发。", 300)+` PUBLIC-Needle %_\" ÜNICODE `+strings.Repeat("尾部。", 100))
	request.Summary = "Reviewed original summary"
	published := discoveryPublish196(t, f, request)
	edit := knowledgeDraft(f, "memory", "PRIVATE-DRAFT-TITLE", "PRIVATE-DRAFT-CONTENT")
	edit.Summary = "PRIVATE-DRAFT-SUMMARY"
	edit.ExpectedVersion = published.Version
	if _, e := f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, published.ID, edit); e != nil {
		t.Fatal(e)
	}
	if _, e := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "skill", "PRIVATE-DRAFT-ONLY", "PRIVATE-DRAFT-ONLY")); e != nil {
		t.Fatal(e)
	}
	for _, query := range []string{" public-NEEDLE ", `%_\"`, "ünicode"} {
		page, e := f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeDiscoveryQuery{Query: query})
		if e != nil || len(page.Items) != 1 || !page.Exhausted {
			t.Fatal(query, page, e)
		}
		hit := page.Items[0]
		if hit.Title != request.Title || hit.Summary != request.Summary || hit.Kind != "memory" || hit.Reference.ID != published.ID || hit.Reference.Revision != published.PublishedRevision || hit.Reference.SHA256 != published.Published.SHA256 || hit.MatchedField != "content" || len(hit.Snippet) > 600 || !utf8.ValidString(hit.Snippet) || !strings.Contains(strings.ToLower(hit.Snippet), strings.ToLower(strings.TrimSpace(query))) {
			t.Fatal("published text or exact reference was replaced/truncated incorrectly", hit)
		}
		raw, _ := json.Marshal(page)
		if strings.Contains(string(raw), "PRIVATE-DRAFT") {
			t.Fatal("discovery exposed latest draft")
		}
	}
	page, e := f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeDiscoveryQuery{Query: "PRIVATE-DRAFT"})
	if e != nil || len(page.Items) != 0 || !page.Exhausted {
		t.Fatal("unpublished text became discoverable", page, e)
	}
	for _, kind := range []string{"spec", "skill"} {
		discoveryPublish196(t, f, knowledgeDraft(f, kind, "Reviewed "+kind, "Published "+kind+" body"))
		page, e = f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeDiscoveryQuery{Kind: kind})
		if e != nil || len(page.Items) != 1 || page.Items[0].MatchedField != "none" || page.Items[0].Kind != kind {
			t.Fatal(page, e)
		}
	}
	f.call("GET", "/project-knowledge/discovery?project_id="+f.project+"&q=PUBLIC-needle&limit=1", nil, 200)
	current, e := f.s.GetProjectKnowledge(ctx, f.actor, f.project, published.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "retire", ExpectedVersion: current.Version, IdempotencyKey: newID()}); e != nil {
		t.Fatal(e)
	}
	page, e = f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeDiscoveryQuery{Query: "public-needle"})
	if e != nil || len(page.Items) != 0 {
		t.Fatal("retirement remained eligible", page, e)
	}
}

// Valid immutable fixtures isolate bounded traversal/budget behavior; the first
// test and Root's actual journey use original create/publish/HTTP commands.
func discoverySeed196(t *testing.T, f *fixture, kind string, start, count int, value func(int) (string, string, string)) {
	t.Helper()
	tx, e := f.s.DB.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	for i := 0; i < count; i++ {
		title, summary, content := value(i)
		id := fmt.Sprintf("%032x", start+i)
		at := "2026-01-02T03:04:05Z"
		r := ProjectKnowledgeRevision{Revision: 1, State: "published", Kind: kind, Title: title, Summary: summary, Content: content, Sources: []ProjectKnowledgeSource{}, Method: "manual", CreatedBy: f.user.ID, CreatedAt: at}
		r.SHA256 = knowledgeRevisionHash(r)
		if _, e = tx.Exec(`INSERT INTO suma_project_knowledge(tenant_id,project_id,id,version,kind,state,latest_revision,published_revision,title,summary,created_at,updated_at)VALUES(?,?,?,1,?,'published',1,1,?,?,?,?)`, f.user.TenantID, f.project, id, kind, title, summary, at, at); e != nil {
			t.Fatal(e)
		}
		if _, e = tx.Exec(`INSERT INTO suma_project_knowledge_revisions(tenant_id,project_id,knowledge_id,revision,snapshot)VALUES(?,?,?,1,?)`, f.user.TenantID, f.project, id, stringify(r)); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
}
func TestKnowledgeDiscoveryBoundedScanCursorScopeAndEncodedBudget(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	discoverySeed196(t, f, "memory", 1, 201, func(i int) (string, string, string) {
		body := "Ordinary approved body"
		if i == 200 {
			body = "Tail NEEDLE"
		}
		return "Memory", "", body
	})
	q := ProjectKnowledgeDiscoveryQuery{Query: " tail needle ", Kind: "memory", Limit: 1}
	first, e := f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, q)
	if e != nil || first.Scanned != 200 || first.Exhausted || first.NextCursor == "" || len(first.Items) != 0 {
		t.Fatal("bounded empty page pretended to cover complete project", first, e)
	}
	next := q
	next.Query = "TAIL NEEDLE"
	next.Cursor = first.NextCursor
	last, e := f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, next)
	if e != nil || last.Scanned != 1 || !last.Exhausted || last.NextCursor != "" || len(last.Items) != 1 {
		t.Fatal("candidate cursor did not advance beyond first 200", last, e)
	}
	for _, bad := range []ProjectKnowledgeDiscoveryQuery{{Query: "changed", Kind: q.Kind, Limit: q.Limit, Cursor: first.NextCursor}, {Query: q.Query, Kind: "spec", Limit: q.Limit, Cursor: first.NextCursor}, {Query: q.Query, Kind: q.Kind, Limit: 2, Cursor: first.NextCursor}} {
		if _, e = f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, bad); applicationStatus(e) != 400 {
			t.Fatal("cursor borrowed another normalized filter", e)
		}
	}
	reviewer := invitedReviewer(t, f)
	if _, e = f.s.DiscoverProjectKnowledge(ctx, reviewer, f.project, next); applicationStatus(e) != 400 {
		t.Fatal("cursor was not actor-bound", e)
	}
	discoverySeed196(t, f, "skill", 1000, 21, func(i int) (string, string, string) {
		return strings.Repeat("<", 200), strings.Repeat("<", 1000), "Reviewed"
	})
	seen := map[string]bool{}
	q = ProjectKnowledgeDiscoveryQuery{Query: "<", Kind: "skill", Limit: 20}
	for pages := 0; pages < 22; pages++ {
		page, e := f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, q)
		if e != nil {
			t.Fatal(e)
		}
		encoded, _ := json.Marshal(page)
		if len(encoded) > 48<<10 || len(page.Items) > 20 || page.Scanned > 200 {
			t.Fatal("encoded response escaped tool budget", len(encoded), page.Scanned)
		}
		for _, hit := range page.Items {
			if seen[hit.Reference.ID] {
				t.Fatal("budget-boundary duplicated an adopted candidate")
			}
			seen[hit.Reference.ID] = true
		}
		if page.Exhausted {
			if page.NextCursor != "" {
				t.Fatal("exhausted page retained cursor")
			}
			break
		}
		if page.NextCursor == "" || page.Scanned == 0 {
			t.Fatal("nonexhausted page cannot advance")
		}
		q.Cursor = page.NextCursor
	}
	if len(seen) != 21 {
		t.Fatal("budget boundary skipped an unconsumed match", len(seen))
	}
}

func TestKnowledgeDiscoveryCurrentSourceAuthorityAndApplicationRead(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	doc := f.call("POST", "/documents", f.body(map[string]any{"title": "Original source", "description": "Only-source-needle", "content": "Original policy"}), 201)
	request := knowledgeDraft(f, "spec", "Published policy", "Policy-body-needle")
	request.Sources = []ProjectKnowledgeSourceInput{{Kind: "documents", ID: doc["id"].(string), Version: 1, Quote: "Only-source-needle"}}
	published := discoveryPublish196(t, f, request)
	page, e := f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeDiscoveryQuery{Query: "only-source-needle"})
	if e != nil || len(page.Items) != 0 {
		t.Fatal("raw source JSON was treated as publication body", page, e)
	}
	agent := applicationActor(t, f, "agent")
	out, e := applicationCall(t, f, agent, "project.knowledge.search", `{"q":"POLICY-BODY-NEEDLE","limit":1}`)
	if e != nil {
		t.Fatal(e)
	}
	result, ok := out.(ProjectKnowledgeDiscovery)
	if !ok || len(result.Items) != 1 || result.Items[0].Reference.SHA256 != published.Published.SHA256 {
		t.Fatal(out)
	}
	for _, bad := range []string{`{"q":null}`, `{"kind":null}`, `{"kind":""}`, `{"limit":0}`, `{"limit":null}`, `{"cursor":null}`, `{"query":"policy"}`, `{"q":"policy","scope":"other"}`} {
		want := 422
		if bad == `{"kind":""}` || bad == `{"limit":0}` {
			want = 400
		}
		if _, e = applicationCall(t, f, agent, "project.knowledge.search", bad); applicationStatus(e) != want {
			t.Fatal("invalid application discovery input used Go defaults", bad, e)
		}
	}
	f.call("PATCH", "/documents/"+doc["id"].(string), f.body(map[string]any{"expected_version": 1, "description": "Policy was replaced"}), 200)
	page, e = f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeDiscoveryQuery{Query: "policy-body-needle"})
	if e != nil || len(page.Items) != 0 {
		t.Fatal("source drift remained eligible for discovery", page, e)
	}
	for _, query := range []string{"limit=0", "limit=21", "kind=", "kind=memory&kind=skill", "q=%00", "q=%FF", "extra=x"} {
		f.call("GET", "/project-knowledge/discovery?project_id="+f.project+"&"+query, nil, 400)
	}
	for locale, want := range map[string]string{"zh-CN": "项目知识发现筛选或游标无效", "en-US": "Invalid project knowledge discovery filters or cursor."} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/project-knowledge/discovery?project_id="+f.project+"&limit=0", nil)
		r.AddCookie(f.cookie)
		r.Header.Set("Accept-Language", locale)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		var body struct {
			Error struct {
				Message   string
				Localized bool
			}
		}
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 400 || body.Error.Message != want || !body.Error.Localized {
			t.Fatal(locale, w.Code, w.Body.String())
		}
	}
	ownerCookie := f.cookie
	invitation := governanceInvite(f, "knowledge-discovery-reader@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "reviewer"}})
	governanceAccept(f, invitation, "knowledge-discovery-reader@example.test")
	revokedCookie := f.cookie
	f.cookie = ownerCookie
	revoked, e := f.s.AuthenticateActor(ctx, revokedCookie.Value)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, revoked.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	before := expertCount(t, f, "suma_operations")
	for _, suffix := range []string{"&limit=0", "&extra=private", "&cursor=malformed"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/project-knowledge/discovery?project_id="+f.project+suffix, nil)
		r.AddCookie(revokedCookie)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("HTTP parsed discovery filters before current project authority", suffix, w.Code, w.Body.String())
		}
	}
	if expertCount(t, f, "suma_operations") != before {
		t.Fatal("rejected discovery request produced effects")
	}
	if e = logoutCredentialForTest(ctx, f.s, f.cookie.Value); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DiscoverProjectKnowledge(ctx, f.actor, f.project, ProjectKnowledgeDiscoveryQuery{Cursor: "malformed", Kind: "bad"}); applicationStatus(e) != 401 {
		t.Fatal("invalid cursor was checked before current authority", e)
	}
}
