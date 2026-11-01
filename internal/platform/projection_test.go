package platform

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	ic "github.com/ai-mada/suma/internal/indexcontract"
)

type projectionCounts struct {
	mu     sync.Mutex
	values map[string]int
}

func (c *projectionCounts) add(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil {
		c.values = map[string]int{}
	}
	c.values[id]++
}
func (c *projectionCounts) get(id string) int { c.mu.Lock(); defer c.mu.Unlock(); return c.values[id] }

func projectionRequest(t *testing.T, r *http.Request) (ic.Request, []byte) {
	t.Helper()
	raw, err := io.ReadAll(r.Body)
	var request ic.Request
	if err != nil || json.Unmarshal(raw, &request) != nil || ic.ValidateRequest(request) != nil {
		t.Error("invalid typed projection request")
	}
	return request, raw
}
func projectionACK(w http.ResponseWriter, request ic.Request, raw []byte) {
	_ = json.NewEncoder(w).Encode(ic.Receipt{Protocol: ic.Protocol, EventID: request.EventID, TenantID: request.TenantID, ProjectID: request.ProjectID, Source: request.Source, Action: request.Action, Disposition: "applied", Generation: "fixture-typed", IndexWatermark: 1, RequestSHA256: ic.Digest(raw)})
}

func TestDocumentRejectsCombinedIndexSourceOverOneMiB(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/documents", f.body(map[string]any{"title": "oversized", "description": strings.Repeat("d", 100), "data": map[string]any{"content": strings.Repeat("x", 1<<20)}}), 400)
	var count int
	if err := f.s.DB.QueryRow("SELECT COUNT(*) FROM suma_index_outbox").Scan(&count); err != nil || count != 0 {
		t.Fatalf("invalid document leaked index event %d %v", count, err)
	}
}
func TestIndexPoisonEventIsDeadLetteredWithoutBlockingOtherSources(t *testing.T) {
	f := newFixture(t)
	bad := f.call("POST", "/documents", f.body(map[string]any{"title": "bad source", "description": "bad"}), 201)
	good := f.call("POST", "/documents", f.body(map[string]any{"title": "good source", "description": "valid"}), 201)
	seen := &projectionCounts{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, raw := projectionRequest(t, r)
		id := request.Source.ID
		seen.add(id)
		if id == bad["id"] {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		projectionACK(w, request, raw)
	}))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "fake-provider-token")
	_ = f.s.ProjectIndexes(context.Background())
	if seen.get(bad["id"].(string)) != 1 || seen.get(good["id"].(string)) != 1 {
		t.Fatal("poison source blocked later event")
	}
	var badState, goodState string
	if err := f.s.DB.QueryRow("SELECT state FROM suma_index_outbox WHERE source_id=?", bad["id"]).Scan(&badState); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow("SELECT state FROM suma_index_outbox WHERE source_id=?", good["id"]).Scan(&goodState); err != nil {
		t.Fatal(err)
	}
	if badState != "failed" || goodState != "indexed" {
		t.Fatalf("states %s %s", badState, goodState)
	}
	if err := f.s.ProjectIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if seen.get(bad["id"].(string)) != 1 {
		t.Fatal("permanent failure replayed endlessly")
	}
	status, err := f.s.IndexStatus(context.Background(), f.actor, f.project)
	if err != nil || status["failed"] != 1 || status["indexed"] != 1 {
		t.Fatalf("index status %+v %v", status, err)
	}
}
func TestIndexTransientFailureRemainsReplayableAndOtherEventsAdvance(t *testing.T) {
	f := newFixture(t)
	first := f.call("POST", "/documents", f.body(map[string]any{"title": "retry"}), 201)
	second := f.call("POST", "/documents", f.body(map[string]any{"title": "continue"}), 201)
	var fail atomic.Bool
	fail.Store(true)
	seen := &projectionCounts{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, raw := projectionRequest(t, r)
		id := request.Source.ID
		seen.add(id)
		if id == first["id"] && fail.Load() {
			w.WriteHeader(429)
			return
		}
		projectionACK(w, request, raw)
	}))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "fake-provider-token")
	_ = f.s.ProjectIndexes(context.Background())
	if seen.get(second["id"].(string)) != 1 {
		t.Fatal("transient failure blocked unrelated event")
	}
	fail.Store(false)
	if err := f.s.ProjectIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if seen.get(first["id"].(string)) != 2 {
		t.Fatal("transient source was not retried")
	}
}

func TestDocumentLimitCountsUTF8Bytes(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/documents", f.body(map[string]any{"title": "UTF8 limit", "data": map[string]any{"content": strings.Repeat("界", (1<<20)/3+1)}}), 400)
}
func TestMalformedPersistedIndexEventDoesNotPoisonQueue(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/documents", f.body(map[string]any{"title": "corrupt"}), 201)
	if _, err := f.s.DB.Exec("UPDATE suma_index_outbox SET payload='not-json'"); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("malformed event sent to provider") }))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "fake-provider-token")
	_ = f.s.ProjectIndexes(context.Background())
	var state string
	if err := f.s.DB.QueryRow("SELECT state FROM suma_index_outbox").Scan(&state); err != nil || state != "failed" {
		t.Fatalf("poison payload remained pending %q %v", state, err)
	}
}

func TestArchivedDeletedDocumentsAreExcludedFromKeywordSources(t *testing.T) {
	f := newFixture(t)
	for _, status := range []string{"active", "archived", "deleted"} {
		f.call("POST", "/documents", f.body(map[string]any{"title": "scope-keyword " + status, "status": status, "data": map[string]any{"content": "scope-keyword body"}}), 201)
	}
	result, err := f.s.SearchKeywords(context.Background(), f.actor, f.project, "scope-keyword")
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]map[string]any)
	if result["total"] != 1 || len(items) != 1 || items[0]["title"] != "scope-keyword active" {
		t.Fatalf("hidden sources revealed %+v", result)
	}
}
func TestDocumentVisibilityTransitionsProduceMonotonicTombstones(t *testing.T) {
	f := newFixture(t)
	record := f.call("POST", "/documents", f.body(map[string]any{"title": "record", "data": map[string]any{"content": "private source body"}}), 201)
	for _, status := range []string{"archived", "active", "deleted"} {
		record = f.call("PATCH", "/documents/"+record["id"].(string), f.body(map[string]any{"expected_version": record["version"], "status": status}), 200)
	}
	rows, err := f.s.DB.Query("SELECT revision,action FROM suma_index_outbox WHERE source_id=? ORDER BY revision", record["id"])
	if err != nil {
		t.Fatal(err)
	}
	actions := []string{}
	for rows.Next() {
		var revision int
		var action string
		if err = rows.Scan(&revision, &action); err != nil {
			t.Fatal(err)
		}
		if revision != len(actions)+1 {
			t.Fatal("nonmonotonic revision")
		}
		actions = append(actions, action)
	}
	rows.Close()
	if strings.Join(actions, ",") != "upsert,delete,upsert,delete" {
		t.Fatal(actions)
	}
	var tombstones atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, raw := projectionRequest(t, r)
		if request.Action == "delete" {
			tombstones.Add(1)
			if request.Content != "" {
				t.Error("tombstone disclosed source content")
			}
			if request.Title != "" {
				t.Error("tombstone disclosed source title")
			}
		}
		projectionACK(w, request, raw)
	}))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "fake-provider-token")
	if err = f.s.ProjectIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	if tombstones.Load() != 2 {
		t.Fatal(tombstones.Load())
	}
}
func TestDeletedSourceRejectsLateIndexHitAndProjectionSchemaIsInstalled(t *testing.T) {
	f := newFixture(t)
	record := f.call("POST", "/documents", f.body(map[string]any{"title": "deleted source", "data": map[string]any{"content": "old content"}}), 201)
	record = f.call("PATCH", "/documents/"+record["id"].(string), f.body(map[string]any{"expected_version": record["version"], "status": "deleted"}), 200)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ic.SearchResult{Protocol: ic.Protocol, Mode: "lancedb_fts", Generation: "fixture-typed", IndexWatermark: 1, Items: []ic.SearchHit{{Source: ic.Source{Kind: "documents", ID: record["id"].(string), Sequence: 1, Revision: 1, SHA256: ic.Digest([]byte("\nold content"))}, Locator: ic.Locator{Unit: "unicode_codepoints", Start: 0, End: 4}}}})
	}))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "fake-provider-token")
	result, err := f.s.SearchIndex(context.Background(), f.actor, f.project, "content")
	if err != nil || len(result["items"].([]any)) != 0 {
		t.Fatalf("late upsert revealed %+v %v", result, err)
	}
	for _, table := range []string{"suma_index_failures", "suma_failure_projection"} {
		var count int
		if err = f.s.DB.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count); err != nil || count != 1 {
			t.Fatalf("migration did not install %s %d %v", table, count, err)
		}
	}
}
