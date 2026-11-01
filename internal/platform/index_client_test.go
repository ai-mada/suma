package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ic "github.com/ai-mada/suma/internal/indexcontract"
)

func TestTypedProjectionRequiresExactOriginalACKBeforeAdvancing(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/documents", f.body(map[string]any{"title": "Original & source", "data": map[string]any{"content": "📚 <canonical>"}}), 201)
	var original string
	var attempts int
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request, raw := projectionRequest(t, r)
		attempts++
		if original == "" {
			original = string(raw)
		} else if string(raw) != original {
			t.Error("retry changed original outbox bytes")
		}
		if attempts == 1 {
			projectionACK(w, request, []byte("different transport bytes"))
			return
		}
		projectionACK(w, request, raw)
	}))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "fixture-private-token")
	if f.s.ProjectIndexes(context.Background()) == nil {
		t.Fatal("wrong raw-body digest accepted")
	}
	var state, receipt string
	if err := f.s.DB.QueryRow("SELECT state,receipt FROM suma_index_outbox").Scan(&state, &receipt); err != nil || state != "pending" || receipt != "" {
		t.Fatalf("invalid ACK advanced delivery: %s %s %v", state, receipt, err)
	}
	if err := f.s.ProjectIndexes(context.Background()); err != nil {
		t.Fatal(err)
	}
	var indexedAt string
	if err := f.s.DB.QueryRow("SELECT state,receipt,indexed_at FROM suma_index_outbox").Scan(&state, &receipt, &indexedAt); err != nil || state != "indexed" || indexedAt == "" || attempts != 2 {
		t.Fatalf("exact ACK not persisted: %s %d %v", state, attempts, err)
	}
	var ack ic.Receipt
	var request ic.Request
	if json.Unmarshal([]byte(receipt), &ack) != nil || json.Unmarshal([]byte(original), &request) != nil || !ic.ReceiptMatches(ack, request, []byte(original)) {
		t.Fatal("persisted ACK detached from original operation")
	}
}

func TestTypedIndexUsesCanonicalCodepointsAndSafeRebuildErrors(t *testing.T) {
	f := newFixture(t)
	record := f.call("POST", "/documents", f.body(map[string]any{"title": "Canonical title", "data": map[string]any{"content": "📚简体正文"}}), 201)
	var phase string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if phase != "" {
			w.WriteHeader(503)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": phase, "message": "SECRET provider operator text", "scanned_sources": 99999}})
			return
		}
		_ = json.NewEncoder(w).Encode(ic.SearchResult{Protocol: ic.Protocol, Mode: "lancedb_hybrid", Generation: "fixture-generation", IndexWatermark: 2, Items: []ic.SearchHit{{Source: ic.Source{Kind: "documents", ID: record["id"].(string), Sequence: 1, Revision: 1, SHA256: ic.Digest([]byte("\n📚简体正文"))}, Locator: ic.Locator{Unit: "unicode_codepoints", Start: 1, End: 3}}}})
	}))
	defer provider.Close()
	t.Setenv("SUMA_KNOWLEDGE_URL", provider.URL)
	t.Setenv("SUMA_KNOWLEDGE_TOKEN", "fixture-private-token")
	result, err := f.s.SearchIndex(context.Background(), f.actor, f.project, "正文")
	if err != nil {
		t.Fatal(err)
	}
	items := result["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["snippet"] != "📚简" || items[0].(map[string]any)["title"] != "Canonical title" {
		t.Fatalf("did not resolve canonical codepoints: %+v", result)
	}
	for _, code := range []string{"index_rebuild_required", "index_rebuilding", "index_blocked", "index_busy", "untrusted_remote_code"} {
		phase = code
		_, err = f.s.SearchIndex(context.Background(), f.actor, f.project, "正文")
		var api *APIError
		if !errors.As(err, &api) || api.Status != 503 || strings.Contains(err.Error(), "SECRET") || strings.Contains(err.Error(), "99999") {
			t.Fatalf("unsafe phase response: %v", err)
		}
		expected := code
		if code == "untrusted_remote_code" {
			expected = "index_unavailable"
		}
		if api.Code != expected {
			t.Fatalf("phase %s became %s", code, api.Code)
		}
	}
	// Container/private-network HTTP is opt-in by an exact reviewed origin.
	// No connection is made here: this validates credential destination policy.
	for _, test := range []struct {
		name, endpoint, origins string
		allowed                 bool
	}{
		{"remote_default_denied", "http://knowledge:4312", "", false},
		{"exact_private_origin", "http://knowledge:4312", "http://knowledge:4312", true},
		{"origin_list", "http://knowledge:4312/api", "http://other:4312, http://knowledge:4312", true},
		{"similar_hostname_denied", "http://knowledge.attacker:4312", "http://knowledge:4312", false},
		{"different_port_denied", "http://knowledge:4313", "http://knowledge:4312", false},
		{"wildcard_origin_denied", "http://knowledge:4312", "http://*:4312", false},
		{"credential_origin_denied", "http://knowledge:4312", "http://user:secret@knowledge:4312", false},
		{"origin_path_denied", "http://knowledge:4312", "http://knowledge:4312/", false},
		{"origin_query_denied", "http://knowledge:4312", "http://knowledge:4312?q=1", false},
		{"https_default", "https://knowledge:4312", "", true},
		{"loopback_default", "http://127.0.0.1:4312", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SUMA_KNOWLEDGE_URL", test.endpoint)
			t.Setenv("SUMA_KNOWLEDGE_HTTP_ORIGINS", test.origins)
			_, _, err := typedIndexEndpoint()
			if (err == nil) != test.allowed {
				t.Fatalf("origin policy allowed=%v; expected %v", err == nil, test.allowed)
			}
		})
	}
}
