package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func discoveryFixture196() map[string]any {
	return map[string]any{"items": []any{map[string]any{"reference": map[string]any{"id": strings.Repeat("a", 32), "revision": 3, "sha256": strings.Repeat("b", 64)}, "project_id": "project-one", "kind": "memory", "title": "用户原文 & publication", "summary": "发布摘要", "snippet": "正文 keyword 命中", "matched_field": "content", "source_count": 2, "published_at": "2026-11-01T23:00:00Z"}}, "next_cursor": "", "mode": "keyword", "scope": "current_project_publications", "scanned": 7, "scan_limit": 200, "exhausted": true}
}

func TestKnowledgeSearchPreservesPublishedReferencesAndReadScope(t *testing.T) {
	calls := 0
	deny := false
	wrongProject := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/v1/project-knowledge/discovery" || r.URL.RawQuery != "cursor=original%2B%2F%3D&kind=memory&limit=2&project_id=project-one&q=%E5%8E%9F%E6%96%87+%26+keyword" || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("Accept-Language") != "en-US" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) || r.ContentLength > 0 {
			t.Error("search crossed original read scope", r.Method, r.URL)
		}
		if deny {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"当前权限已撤回"}}`))
			return
		}
		response := discoveryFixture196()
		if wrongProject {
			response["items"].([]any)[0].(map[string]any)["project_id"] = "another-project"
		}
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer s.Close()
	args := []string{"knowledge", "search", "--query", "原文 & keyword", "--kind", "memory", "--limit", "2", "--cursor", "original+/="}
	code, out, stderr := contextInvoke(t, s.URL, "en-US", "", args...)
	var decoded map[string]any
	decodeErr := json.Unmarshal([]byte(out), &decoded)
	if code != 0 || stderr != "" || decodeErr != nil || !strings.Contains(out, `"revision": 3`) || !strings.Contains(out, strings.Repeat("b", 64)) || decoded["items"].([]any)[0].(map[string]any)["title"] != "用户原文 & publication" || strings.Contains(out, `"total"`) {
		t.Fatal("accurate publication result was altered or fabricated", code, out, stderr)
	}
	deny = true
	code, _, stderr = contextInvoke(t, s.URL, "en-US", "", args...)
	if code != 1 || calls != 2 || !strings.Contains(stderr, "forbidden") {
		t.Fatal("revocation was retried, hidden or treated as a valid empty page", code, calls, stderr)
	}
	deny = false
	wrongProject = true
	code, _, _ = contextInvoke(t, s.URL, "en-US", "", args...)
	if code != 1 || calls != 3 {
		t.Fatal("cross-project reference accepted", code, calls)
	}
}

func TestKnowledgeSearchBoundedInputBilingualTableAndIncompleteEmptyPage(t *testing.T) {
	calls := 0
	response := map[string]any{"items": []any{}, "next_cursor": "continue196+/=", "mode": "keyword", "scope": "current_project_publications", "scanned": 200, "scan_limit": 200, "exhausted": false}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _ = json.NewEncoder(w).Encode(response) }))
	defer s.Close()
	for _, args := range [][]string{{"--limit", "0"}, {"--limit", "21"}, {"--kind", "document"}, {"--query", strings.Repeat("中", 67)}, {"--query", "line\nquery"}, {"--cursor", strings.Repeat("x", 2049)}, {"--cursor", "line\nnext"}, {"--format", "html"}, {"--query", string([]byte{0xff})}} {
		code, _, _ := contextInvoke(t, s.URL, "zh-CN", "", append([]string{"knowledge", "search"}, args...)...)
		if code != 2 {
			t.Error("unbounded/invalid search sent", args[0], code)
		}
	}
	if calls != 0 {
		t.Fatal("invalid input reached HTTP", calls)
	}
	for _, language := range []string{"zh-CN", "en-US"} {
		code, out, stderr := contextInvoke(t, s.URL, language, "", "knowledge", "search", "--format", "table")
		if code != 0 || stderr != "" || !strings.Contains(out, "continue196+/=") || !strings.Contains(out, "200") || language == "zh-CN" && !strings.Contains(out, "继续") || language == "en-US" && !strings.Contains(out, "Continue") {
			t.Fatal("bounded empty page lost continuation", language, code, out, stderr)
		}
		code, help, _ := contextInvoke(t, s.URL, language, "", "knowledge", "search", "--help")
		if code != 0 || !strings.Contains(help, "knowledge search") || !strings.Contains(help, "--cursor") {
			t.Fatal("search help missing", language, help)
		}
	}
	response = discoveryFixture196()
	row := response["items"].([]any)[0].(map[string]any)
	row["matched_field"] = "none"
	row["title"] = "原文\x1b[31m\nfixture-token"
	row["snippet"] = "原文\nnot translated"
	code, out, stderr := contextInvoke(t, s.URL, "en-US", "", "knowledge", "search", "--format", "table")
	if code != 0 || stderr != "" || !strings.Contains(out, strings.Repeat("b", 64)) || !strings.Contains(out, `"revision":3`) || strings.Contains(out, "\x1b") || strings.Contains(out, "fixture-token") || !strings.Contains(out, "原文") {
		t.Fatal("human presentation lost exact reference, raw language or escaped text", code, out, stderr)
	}
	for _, bad := range []string{"missing_exhausted", "missing_scanned", "title_bytes", "summary_bytes", "snippet_bytes", "source_count", "nul_text"} {
		response = discoveryFixture196()
		row = response["items"].([]any)[0].(map[string]any)
		switch bad {
		case "missing_exhausted":
			response["items"] = []any{}
			response["next_cursor"] = "more196"
			delete(response, "exhausted")
		case "missing_scanned":
			response["items"] = []any{}
			delete(response, "scanned")
		case "title_bytes":
			row["title"] = strings.Repeat("中", 67)
		case "summary_bytes":
			row["summary"] = strings.Repeat("中", 334)
		case "snippet_bytes":
			row["snippet"] = strings.Repeat("中", 201)
		case "source_count":
			row["source_count"] = 9
		case "nul_text":
			row["summary"] = "text\x00hidden"
		}
		code, _, _ = contextInvoke(t, s.URL, "en-US", "", "knowledge", "search", "--query", "keyword")
		if code != 1 {
			t.Error("invalid closed discovery facts accepted", bad, code)
		}
	}
	response = discoveryFixture196()
	row = response["items"].([]any)[0].(map[string]any)
	row["title"] = strings.Repeat("a", 200)
	row["summary"] = strings.Repeat("b", 1000)
	row["snippet"] = strings.Repeat("c", 600)
	row["source_count"] = 8
	if code, _, stderr = contextInvoke(t, s.URL, "en-US", "", "knowledge", "search", "--query", "keyword"); code != 0 {
		t.Fatal("valid exact text/source boundaries rejected", code, stderr)
	}
}
