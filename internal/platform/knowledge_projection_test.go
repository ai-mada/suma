package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	ic "github.com/ai-mada/suma/internal/indexcontract"
	"github.com/ai-mada/suma/internal/store"
)

func projectionEvents197(t *testing.T, f *fixture, id string) []ic.Request {
	t.Helper()
	rows, e := f.s.DB.Query(`SELECT payload FROM suma_index_outbox WHERE source_kind='project_knowledge' AND source_id=? ORDER BY revision`, id)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	out := []ic.Request{}
	for rows.Next() {
		var raw string
		var event ic.Request
		if e = rows.Scan(&raw); e != nil {
			t.Fatal(e)
		}
		if e = json.Unmarshal([]byte(raw), &event); e != nil {
			t.Fatal(e)
		}
		if e = ic.ValidateRequest(event); e != nil {
			t.Fatal(e)
		}
		out = append(out, event)
	}
	if e = rows.Err(); e != nil {
		t.Fatal(e)
	}
	return out
}
func TestKnowledgeProjectionPublicationRetirementReceiptAndRollback(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	created, e := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "memory", "Approved title", "Approved body"))
	if e != nil {
		t.Fatal(e)
	}
	command := ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: created.Version, IdempotencyKey: newID()}
	if _, e = f.s.DB.Exec(`CREATE TRIGGER owned_projection_rollback BEFORE INSERT ON suma_index_outbox BEGIN SELECT RAISE(ABORT,'owned rollback');END`); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, command); e == nil {
		t.Fatal("publication survived failed transactional outbox")
	}
	unchanged, e := f.s.GetProjectKnowledge(ctx, f.actor, f.project, created.ID)
	if e != nil || unchanged.Version != created.Version || unchanged.Published != nil || len(projectionEvents197(t, f, created.ID)) != 0 {
		t.Fatal("failed event committed knowledge", unchanged, e)
	}
	if _, e = f.s.DB.Exec(`DROP TRIGGER owned_projection_rollback`); e != nil {
		t.Fatal(e)
	}
	published, e := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, command)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, created.ID, command); e != nil {
		t.Fatal(e)
	}
	events := projectionEvents197(t, f, created.ID)
	if len(events) != 1 || events[0].Source.Sequence != published.Version || events[0].Source.Revision != published.PublishedRevision || events[0].Source.SHA256 != published.Published.SHA256 || events[0].Content != "Approved body" || events[0].Title != "Approved title" {
		t.Fatal(events)
	}
	initialStatus, e := f.s.ProjectKnowledgeIndexStatus(ctx, f.actor, f.project)
	if e != nil || initialStatus.CurrentPublications != 1 || initialStatus.UntrackedPublications != 0 || initialStatus.Pending != 1 || initialStatus.LastAcknowledgement != nil {
		t.Fatal(initialStatus, e)
	}
	edit := knowledgeDraft(f, "memory", "PRIVATE draft", "PRIVATE body")
	edit.ExpectedVersion = published.Version
	draft, e := f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, published.ID, edit)
	if e != nil {
		t.Fatal(e)
	}
	if len(projectionEvents197(t, f, created.ID)) != 1 {
		t.Fatal("draft emitted projection")
	}
	retired, e := f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "retire", ExpectedVersion: draft.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	events = projectionEvents197(t, f, created.ID)
	if len(events) != 2 || events[1].Action != "delete" || events[1].Source.Sequence != retired.Version || events[1].Source.Revision != published.PublishedRevision || events[1].Source.SHA256 != published.Published.SHA256 || events[1].Content != "" || events[1].Title != "" {
		t.Fatal("retire used draft or reset sequence", events)
	}
	never, e := f.s.CreateProjectKnowledge(ctx, f.actor, f.project, knowledgeDraft(f, "skill", "Never approved", "Unpublished"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, never.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "retire", ExpectedVersion: never.Version, IdempotencyKey: newID()}); e != nil {
		t.Fatal(e)
	}
	zero := projectionEvents197(t, f, never.ID)
	if len(zero) != 1 || zero[0].Source.Revision != 0 || zero[0].Source.SHA256 != "" {
		t.Fatal(zero)
	}
	status, e := f.s.ProjectKnowledgeIndexStatus(ctx, f.actor, f.project)
	if e != nil || status.Pending != 3 || status.Total != 3 || status.CurrentPublications != 0 || status.UntrackedPublications != 0 || status.LastAcknowledgement != nil || status.LatestEvent == nil || status.Coverage != "outbox_delivery_only" {
		t.Fatal(status, e)
	}
	encoded, _ := json.Marshal(status)
	if strings.Contains(string(encoded), "Approved body") || strings.Contains(string(encoded), "PRIVATE") {
		t.Fatal("status exposed full source", string(encoded))
	}
	// Delivery evidence is coupled to the immutable source/event/raw request,
	// not merely any HTTP 2xx or a highest-sequence number.
	raw := stringify(events[0])
	ack := ic.Receipt{Protocol: ic.Protocol, EventID: events[0].EventID, TenantID: f.user.TenantID, ProjectID: f.project, Source: events[0].Source, Action: "upsert", Disposition: "applied", Generation: strings.Repeat("a", 64), IndexWatermark: 2, RequestSHA256: ic.Digest([]byte(raw))}
	if _, e = f.s.DB.Exec(`UPDATE suma_index_outbox SET state='indexed',receipt=?,indexed_at='2026-11-02T00:00:00Z' WHERE id=?`, stringify(ack), ack.EventID); e != nil {
		t.Fatal(e)
	}
	status, e = f.s.ProjectKnowledgeIndexStatus(ctx, f.actor, f.project)
	if e != nil || status.Pending != 2 || status.Indexed != 1 || status.LastAcknowledgement == nil || status.LastAcknowledgement.RequestSHA256 != ack.RequestSHA256 || status.Coverage != "outbox_delivery_only" {
		t.Fatal(status, e)
	}
	ack.ProjectID = newID()
	if _, e = f.s.DB.Exec(`UPDATE suma_index_outbox SET receipt=? WHERE id=?`, stringify(ack), ack.EventID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ProjectKnowledgeIndexStatus(ctx, f.actor, f.project); e == nil {
		t.Fatal("status returned an acknowledgement from another scope")
	}
}

func TestKnowledgeProjectionIndexSearchCanonicalReferencesAndCurrentAuthority(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	published := discoveryPublish196(t, f, knowledgeDraft(f, "spec", "Reviewed original", "前😀界"+strings.Repeat("稳定。", 180)))
	source := ic.Source{Kind: "project_knowledge", ID: published.ID, Sequence: published.Version, Revision: published.PublishedRevision, SHA256: published.Published.SHA256}
	response := ic.SearchResult{Protocol: ic.Protocol, Mode: "lancedb_hybrid", Generation: strings.Repeat("a", 64), IndexWatermark: 4, Items: []ic.SearchHit{{Source: source, Locator: ic.Locator{Unit: "unicode_codepoints", Start: 1, End: len([]rune(published.Published.Content))}}}}
	calls := 0
	var revoke bool
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var request ic.SearchRequest
		if r.URL.Path != "/search" || r.Header.Get("Authorization") != "Bearer owned-token" || json.NewDecoder(r.Body).Decode(&request) != nil || request.SourceKind != "project_knowledge" || request.TenantID != f.user.TenantID || request.ProjectID != f.project || request.Query != "稳定" || request.Limit != ic.MaxCandidates {
			t.Error("wrong scoped provider request", request)
		}
		if revoke {
			if e := logoutCredentialForTest(ctx, f.s, f.cookie.Value); e != nil {
				t.Error(e)
			}
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "owned-token")
	edit := knowledgeDraft(f, "spec", "PRIVATE-DRAFT", "PRIVATE-DRAFT")
	edit.ExpectedVersion = published.Version
	if _, e := f.s.UpdateProjectKnowledge(ctx, f.actor, f.project, published.ID, edit); e != nil {
		t.Fatal(e)
	}
	page, e := f.s.SearchProjectKnowledgeIndex(ctx, f.actor, f.project, ProjectKnowledgeIndexQuery{Query: " 稳定 "})
	if e != nil || len(page.Items) != 1 {
		t.Fatal(page, e)
	}
	hit := page.Items[0]
	if hit.Reference.ID != published.ID || hit.Reference.Revision != published.PublishedRevision || hit.Reference.SHA256 != published.Published.SHA256 || hit.Title != "Reviewed original" || !strings.HasPrefix(hit.Snippet, "😀界") || !utf8.ValidString(hit.Snippet) || len(hit.Snippet) > 600 || hit.Snippet != string([]rune(published.Published.Content)[hit.Locator.Start:hit.Locator.End]) || page.Scope != "provider_candidates" {
		t.Fatal("provider data became authority or locator split unicode", page)
	}
	for _, raw := range []string{`{"q":"稳定","limit":0}`, `{"q":"稳定","kind":null}`, `{"q":null}`, `{"q":"稳定","extra":true}`} {
		_, e = f.s.InvokeApplicationTool(ctx, f.actor, ApplicationToolCall{ProjectID: f.project, Tool: "project.knowledge.index_search", Arguments: json.RawMessage(raw)})
		if applicationStatus(e) != 400 && applicationStatus(e) != 422 {
			t.Fatal("accepted invalid index tool input", raw, e)
		}
	}
	if calls != 1 {
		t.Fatal("invalid input queried provider", calls)
	}
	value, e := f.s.InvokeApplicationTool(ctx, f.actor, ApplicationToolCall{ProjectID: f.project, Tool: "project.knowledge.index_search", Arguments: json.RawMessage(`{"q":"稳定","kind":"spec","limit":1}`)})
	if e != nil || len(value.(ProjectKnowledgeIndexSearch).Items) != 1 {
		t.Fatal(value, e)
	}
	// An exact publication reference does not make an arbitrary provider event
	// sequence official. The local publication outbox remains its authority.
	response.Items[0].Source.Sequence = ic.MaxSequence
	forged, e := f.s.SearchProjectKnowledgeIndex(ctx, f.actor, f.project, ProjectKnowledgeIndexQuery{Query: "稳定"})
	if e != nil || len(forged.Items) != 0 || forged.Discarded != 1 {
		t.Fatal("provider forged a business event sequence", forged, e)
	}
	response.Items[0].Source.Sequence = source.Sequence
	// The provider's old pin cannot survive a new publication or retired state.
	current, e := f.s.GetProjectKnowledge(ctx, f.actor, f.project, published.ID)
	if e != nil {
		t.Fatal(e)
	}
	current, e = f.s.CommandProjectKnowledge(ctx, f.actor, f.project, published.ID, ProjectKnowledgeCommandRequest{ProjectID: f.project, Action: "publish", ExpectedVersion: current.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	page, e = f.s.SearchProjectKnowledgeIndex(ctx, f.actor, f.project, ProjectKnowledgeIndexQuery{Query: "稳定"})
	if e != nil || len(page.Items) != 0 || page.Discarded != 1 {
		t.Fatal("stale source became current", page, e)
	}
	response.Items = []ic.SearchHit{}
	revoke = true
	_, e = f.s.SearchProjectKnowledgeIndex(ctx, f.actor, f.project, ProjectKnowledgeIndexQuery{Query: "稳定"})
	requireAuthorityStatus(t, e, 401)
	before := calls
	f.call("GET", "/project-knowledge/index-search?project_id="+f.project+"&limit=0", nil, 401)
	if calls != before {
		t.Fatal("unauthorized invalid query contacted provider")
	}
}

func TestKnowledgeProjectionInitialSchemaRejectsUnknownBeforeDDL(t *testing.T) {
	for _, name := range []string{"suma_index_outbox", "Suma_Index_Outbox"} {
		t.Run(name, func(t *testing.T) {
			db, e := store.Open("sqlite", filepath.Join(t.TempDir(), "unknown.sqlite"))
			if e != nil {
				t.Fatal(e)
			}
			defer db.Close()
			if _, e = db.Exec(`CREATE TABLE ` + name + `(id TEXT,unique_user_text TEXT)`); e != nil {
				t.Fatal(e)
			}
			if _, e = db.Exec(`INSERT INTO ` + name + ` VALUES('owned','PRESERVE')`); e != nil {
				t.Fatal(e)
			}
			s := &Service{DB: db, Dialect: "sqlite"}
			if e = s.Initialize(context.Background()); !errors.Is(e, store.ErrInitialSchema) {
				t.Fatal("unknown initial schema accepted", e)
			}
			var text string
			var count int
			if e = db.QueryRow(`SELECT unique_user_text FROM ` + name).Scan(&text); e != nil || text != "PRESERVE" {
				t.Fatal(text, e)
			}
			if e = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`).Scan(&count); e != nil || count != 1 {
				t.Fatal("service created tables before initial guard", count, e)
			}
		})
	}
}

func TestKnowledgeProjectionHTTPCurrentScopeAndLocalizedClosedQueries(t *testing.T) {
	f := knowledgeFixture(t)
	ctx := context.Background()
	for _, route := range []string{"index-search", "index-status"} {
		for _, suffix := range []string{"&extra=x", "&limit=0"} {
			if route == "index-status" && suffix == "&limit=0" || route == "index-search" {
				f.call("GET", "/project-knowledge/"+route+"?project_id="+f.project+suffix, nil, 400)
			}
		}
	}
	for _, suffix := range []string{"&q=", "&q=%FF", "&q=needle&kind=", "&q=needle&limit=01", "&q=needle&q=other"} {
		f.call("GET", "/project-knowledge/index-search?project_id="+f.project+suffix, nil, 400)
	}
	for locale, want := range map[string]string{"zh-CN": "项目知识索引查询无效", "en-US": "The project knowledge index query is invalid."} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/project-knowledge/index-search?project_id="+f.project+"&limit=0", nil)
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
	owner := f.cookie
	invitation := governanceInvite(f, "index-reader@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "reviewer"}})
	governanceAccept(f, invitation, "index-reader@example.test")
	cookie := f.cookie
	f.cookie = owner
	reader, e := f.s.AuthenticateActor(ctx, cookie.Value)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, reader.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	for _, route := range []string{"index-search", "index-status"} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/project-knowledge/"+route+"?project_id="+f.project+"&extra=x&limit=0", nil)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("invalid filters masked current project revocation", route, w.Code, w.Body.String())
		}
	}
	phase := "index_rebuilding"
	revoke := false
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoke {
			if err := logoutCredentialForTest(ctx, f.s, owner.Value); err != nil {
				t.Error(err)
			}
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": phase, "message": "PRIVATE provider details", "other_tenant_count": 900}})
	}))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "owned-token")
	for code, want := range map[string]string{"index_rebuild_required": "The index requires rebuilding. Keyword discovery remains available.", "index_rebuilding": "The index is rebuilding. Keyword discovery remains available.", "index_blocked": "Index rebuilding is blocked. Contact an administrator."} {
		phase = code
		r := httptest.NewRequest(http.MethodGet, "/api/v1/project-knowledge/index-search?project_id="+f.project+"&q=needle", nil)
		r.AddCookie(owner)
		r.Header.Set("Accept-Language", "en-US")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		var body struct {
			Error struct {
				Code, Message string
				Localized     bool
			}
		}
		if json.Unmarshal(w.Body.Bytes(), &body) != nil || w.Code != 503 || body.Error.Code != code || body.Error.Message != want || !body.Error.Localized || strings.Contains(w.Body.String(), "PRIVATE") || strings.Contains(w.Body.String(), "900") {
			t.Fatal("provider phase was unlocalized or leaked private evidence", w.Code, w.Body.String())
		}
	}
	revoke = true
	_, e = f.s.SearchProjectKnowledgeIndex(ctx, f.actor, f.project, ProjectKnowledgeIndexQuery{Query: "needle"})
	requireAuthorityStatus(t, e, 401)
}
