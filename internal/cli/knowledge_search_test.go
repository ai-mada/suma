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

func indexSearchFixture197() map[string]any {
	result := discoveryFixture196()
	hit := result["items"].([]any)[0].(map[string]any)
	delete(hit, "matched_field")
	hit["source"] = map[string]any{"kind": "project_knowledge", "id": strings.Repeat("a", 32), "sequence": 5, "revision": 3, "sha256": strings.Repeat("b", 64)}
	hit["locator"] = map[string]any{"unit": "unicode_codepoints", "start": 2, "end": 5}
	hit["snippet"] = "原文🌏"
	return map[string]any{"items": []any{hit}, "mode": "lancedb_hybrid", "generation": "generation-197", "index_watermark": 7, "candidates_examined": 2, "discarded": 1, "scope": "provider_candidates"}
}

func TestKnowledgeIndexSearchPreservesSourceLocatorAndReadScope(t *testing.T) {
	calls, status := 0, 200
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/api/v1/project-knowledge/index-search" || r.URL.RawQuery != "kind=memory&limit=2&project_id=project-one&q=%E5%8E%9F%E6%96%87+%26+keyword" || r.Header.Get("Authorization") != "Bearer fixture-token" || r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) || r.ContentLength > 0 {
			t.Error("indexed search changed the captured scoped GET", r.Method, r.URL)
		}
		if status != 200 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":{"code":"index_unavailable","message":"Index unavailable"}}`))
			return
		}
		_ = json.NewEncoder(w).Encode(indexSearchFixture197())
	}))
	defer s.Close()
	args := []string{"knowledge", "search", "--mode", "index", "--query", "原文 & keyword", "--kind", "memory", "--limit", "2"}
	code, out, stderr := contextInvoke(t, s.URL, "en-US", "", args...)
	var decoded map[string]any
	if code != 0 || stderr != "" || json.Unmarshal([]byte(out), &decoded) != nil || stringifyCLI197(decoded) != stringifyCLI197(indexSearchFixture197()) {
		t.Fatal("indexed source/locator/mode facts were changed", code, out, stderr)
	}
	for _, language := range []string{"en-US", "zh-CN"} {
		code, out, stderr = contextInvoke(t, s.URL, language, "", append(args, "--format", "table")...)
		if code != 0 || stderr != "" || !strings.Contains(out, "lancedb_hybrid") || !strings.Contains(out, "generation-197") || !strings.Contains(out, "unicode_codepoints") || !strings.Contains(out, `"sequence":5`) || !strings.Contains(out, "原文🌏") || language == "en-US" && !strings.Contains(out, "Provider candidates") || language == "zh-CN" && (!strings.Contains(out, "提供方候选") || !strings.Contains(out, "记忆")) {
			t.Fatal("indexed table omitted actual scope/source facts", language, code, out, stderr)
		}
	}
	for _, rejected := range []int{403, 503} {
		status = rejected
		before := calls
		code, _, _ = contextInvoke(t, s.URL, "en-US", "", args...)
		if code != 1 || calls != before+1 {
			t.Fatal("indexed error retried or silently fell back", rejected, code, calls-before)
		}
	}
}

func stringifyCLI197(value any) string { raw, _ := json.Marshal(value); return string(raw) }

func TestKnowledgeIndexSearchRejectsInvalidFactsAndSeparatesModes(t *testing.T) {
	calls := 0
	response := indexSearchFixture197()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _ = json.NewEncoder(w).Encode(response) }))
	defer s.Close()
	for _, flags := range [][]string{{"--mode", "other"}, {"--mode", "index"}, {"--mode", "index", "--query", "  "}, {"--mode", "index", "--query", "keyword", "--cursor", ""}, {"--mode", "index", "--query", "keyword", "--cursor", "original"}} {
		if code, _, _ := contextInvoke(t, s.URL, "en-US", "", append([]string{"knowledge", "search"}, flags...)...); code != 2 {
			t.Error("invalid index mode reached request", flags, code)
		}
	}
	if calls != 0 {
		t.Fatal("invalid mode sent HTTP", calls)
	}
	args := []string{"knowledge", "search", "--mode", "index", "--query", "keyword"}
	for _, bad := range []string{"missing_watermark", "missing_examined", "missing_discarded", "missing_locator_start", "foreign_project", "foreign_domain", "ref_mismatch", "sequence_before_revision", "utf16_locator", "negative_locator", "empty_locator", "invalid_mode", "empty_generation", "negative_watermark", "over_candidates", "over_discarded", "title_bytes", "summary_bytes", "snippet_bytes", "source_count", "nul_text", "unexpected_cursor"} {
		response = indexSearchFixture197()
		hit := response["items"].([]any)[0].(map[string]any)
		source := hit["source"].(map[string]any)
		locator := hit["locator"].(map[string]any)
		switch bad {
		case "missing_watermark":
			delete(response, "index_watermark")
		case "missing_examined":
			delete(response, "candidates_examined")
		case "missing_discarded":
			delete(response, "discarded")
		case "missing_locator_start":
			delete(locator, "start")
		case "foreign_project":
			hit["project_id"] = "other-project"
		case "foreign_domain":
			source["kind"] = "documents"
		case "ref_mismatch":
			source["sha256"] = strings.Repeat("c", 64)
		case "sequence_before_revision":
			source["sequence"] = 2
		case "utf16_locator":
			locator["unit"] = "utf16"
		case "negative_locator":
			locator["start"] = -1
		case "empty_locator":
			locator["end"] = 2
		case "invalid_mode":
			response["mode"] = "keyword"
		case "empty_generation":
			response["generation"] = ""
		case "negative_watermark":
			response["index_watermark"] = -1
		case "over_candidates":
			response["candidates_examined"] = 101
		case "over_discarded":
			response["discarded"] = 3
		case "title_bytes":
			hit["title"] = strings.Repeat("中", 67)
		case "summary_bytes":
			hit["summary"] = strings.Repeat("中", 334)
		case "snippet_bytes":
			hit["snippet"] = strings.Repeat("中", 201)
		case "source_count":
			hit["source_count"] = 9
		case "nul_text":
			hit["snippet"] = "bad\x00text"
		case "unexpected_cursor":
			response["next_cursor"] = "not-supported"
		}
		if code, _, _ := contextInvoke(t, s.URL, "en-US", "", args...); code != 1 {
			t.Error("invalid index result accepted", bad, code)
		}
	}
	response = map[string]any{"items": []any{}, "mode": "oceanbase_fts", "generation": "generation-197", "index_watermark": 0, "candidates_examined": 0, "discarded": 0, "scope": "provider_candidates"}
	if code, out, stderr := contextInvoke(t, s.URL, "zh-CN", "", append(args, "--format", "table")...); code != 0 || stderr != "" || !strings.Contains(out, "提供方候选") || strings.Contains(out, "项目没有") {
		t.Fatal("explicit zero candidate result became global empty or failed", code, out, stderr)
	}
	for _, language := range []string{"en-US", "zh-CN"} {
		if code, help, _ := contextInvoke(t, s.URL, language, "", "knowledge", "search", "--help"); code != 0 || !strings.Contains(help, "--mode keyword|index") {
			t.Fatal("mode help missing", language, code, help)
		}
	}
}
