package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func contextInvoke(t *testing.T, origin, language, input string, args ...string) (int, string, string) {
	t.Helper()
	var out, stderr bytes.Buffer
	all := []string{"--control", origin, "--project", "project-one", "--token", "fixture-token", "--language", language,
		"--expected-user", strings.Repeat("a", 32), "--expected-organization", strings.Repeat("b", 32)}
	code := Execute(context.Background(), append(all, args...), strings.NewReader(input), &out, &stderr)
	return code, out.String(), stderr.String()
}

// Wrong route, escaped query or missing project scope must be observable at HTTP.
func TestProjectContextReadRoutes(t *testing.T) {
	for _, tc := range []struct {
		name, path, query string
		args              []string
	}{
		{"knowledge_list", "/api/v1/project-knowledge", "cursor=next%2B%2F%3D&kind=spec&limit=5&project_id=project-one&q=%E5%8E%9F%E6%96%87+%26+key&state=published", []string{"knowledge", "list", "--kind", "spec", "--state", "published", "--query", "原文 & key", "--limit", "5", "--cursor", "next+/="}},
		{"knowledge_show", "/api/v1/project-knowledge/k-1", "project_id=project-one", []string{"knowledge", "show", "k-1"}},
		{"knowledge_history", "/api/v1/project-knowledge/k-1/revisions", "cursor=next%2B%2F%3D&limit=100&project_id=project-one", []string{"knowledge", "history", "k-1", "--limit", "100", "--cursor", "next+/="}},
		{"design_show", "/api/v1/requirements/r-1/design", "project_id=project-one", []string{"requirement", "design", "show", "r-1"}},
		{"design_revisions", "/api/v1/requirements/r-1/design/revisions", "limit=5&project_id=project-one", []string{"requirement", "design", "history", "r-1", "--limit", "5"}},
		{"design_adoptions", "/api/v1/requirements/r-1/design/adoptions", "cursor=cursor&limit=20&project_id=project-one", []string{"requirement", "design", "adoptions", "r-1", "--cursor", "cursor"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.Method != "GET" || r.URL.Path != tc.path || r.URL.RawQuery != tc.query || r.Header.Get("Accept-Language") != "en-US" || r.Header.Get("Authorization") != "Bearer fixture-token" {
					t.Errorf("unexpected read: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("X-Suma-Expected-User") != "" || r.ContentLength > 0 {
					t.Error("GET acquired a command body or original-view header")
				}
				_, _ = w.Write([]byte(`{"content":"用户原文 raw","revision":9007199254740993,"sources":[{"quote":"不翻译 unchanged"}]}`))
			}))
			defer server.Close()
			code, out, stderr := contextInvoke(t, server.URL, "en-US", "", tc.args...)
			if code != 0 || stderr != "" || calls != 1 || !strings.Contains(out, "9007199254740993") || !strings.Contains(out, "用户原文 raw") || !strings.Contains(out, "不翻译 unchanged") {
				t.Fatalf("code=%d calls=%d out=%s err=%s", code, calls, out, stderr)
			}
		})
	}
}

// Each operation must target its contracted endpoint without numeric rounding,
// identity refresh or client-side approval. Server errors remain authoritative.
func TestProjectContextWriteRoutesAndExactNumbers(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		args               []string
	}{
		{"create", "POST", "/api/v1/project-knowledge", []string{"knowledge", "create"}},
		{"edit", "PATCH", "/api/v1/project-knowledge/k-1", []string{"knowledge", "edit", "k-1"}},
		{"extract", "POST", "/api/v1/project-knowledge/extractions", []string{"knowledge", "extract"}},
		{"knowledge_command", "POST", "/api/v1/project-knowledge/k-1/commands", []string{"knowledge", "command", "k-1"}},
		{"design_save", "PUT", "/api/v1/requirements/r-1/design", []string{"requirement", "design", "save", "r-1"}},
		{"design_command", "POST", "/api/v1/requirements/r-1/design/commands", []string{"requirement", "design", "command", "r-1"}},
		{"design_adopt", "POST", "/api/v1/requirements/r-1/design/adopt", []string{"requirement", "design", "adopt", "r-1"}},
		{"generic_put", "PUT", "/api/v1/requirements/r-1/design", []string{"api", "/requirements/r-1/design", "--method", "PUT"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				raw, _ := io.ReadAll(r.Body)
				if r.Method != tc.method || r.URL.Path != tc.path || r.URL.Query().Get("project_id") != "project-one" || r.Header.Get("Accept-Language") != "zh-CN" || r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("unexpected command: %s %s", r.Method, r.URL)
				}
				if r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
					t.Error("captured original view missing")
				}
				for _, exact := range []string{`"expected_version":9007199254740993`, `"expected_requirement_version":1e0`, `"revision":1.0`, `"idempotency_key":"original-operation"`, `"project_id":"project-one"`, `"content":"用户原文 raw"`} {
					if !strings.Contains(string(raw), exact) {
						t.Errorf("request lost exact JSON field %s", exact)
					}
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"服务端决定 approval denied","details":{"revision":9007199254740993}}}`))
			}))
			defer server.Close()
			body := `{"action":"publish","expected_version":9007199254740993,"expected_requirement_version":1e0,"revision":1.0,"content":"用户原文 raw","idempotency_key":"original-operation"}`
			code, _, stderr := contextInvoke(t, server.URL, "zh-CN", body, append(tc.args, "--file", "-")...)
			if code != 1 || calls != 1 || !strings.Contains(stderr, "服务端决定 approval denied") || !strings.Contains(stderr, "9007199254740993") {
				t.Fatalf("code=%d calls=%d err=%s", code, calls, stderr)
			}
		})
	}
}

func TestProjectContextStableOperationAcrossFileStdinAndLanguage(t *testing.T) {
	const body = `{"expected_version":0,"expected_requirement_version":3,"content":{"product":"保留原文","experience":"","technical":""},"tasks":[]}`
	file := filepath.Join(t.TempDir(), "draft.json")
	if err := os.WriteFile(file, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var keys []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		keys = append(keys, request["idempotency_key"].(string))
		_, _ = w.Write([]byte(`{"version":1}`))
	}))
	defer server.Close()
	for i, file := range []string{file, "-"} {
		code, _, stderr := contextInvoke(t, server.URL, []string{"zh-CN", "en-US"}[i], body, "requirement", "design", "save", "r-1", "--file", file)
		if code != 0 {
			t.Fatal(stderr)
		}
	}
	if len(keys) != 2 || !strings.HasPrefix(keys[0], "cli-") || keys[0] != keys[1] {
		t.Fatalf("unstable operation keys: %v", keys)
	}
}

func TestProjectContextPreviewIsReadWithoutOperationKey(t *testing.T) {
	for _, args := range [][]string{
		{"requirement", "design", "preview", "r-1", "--file", "-"},
		{"api", "/requirements/r-1/design/preview", "--method", "POST", "--file", "-"},
	} {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			if r.Method != "POST" || r.URL.Path != "/api/v1/requirements/r-1/design/preview" || body["project_id"] != "project-one" || body["sha256"] != "reviewed-sha" {
				t.Errorf("wrong preview: %s %s", r.Method, r.URL)
			}
			if _, exists := body["idempotency_key"]; exists {
				t.Error("read-only strict preview received mutation key")
			}
			if r.Header.Get("X-Suma-Expected-User") != strings.Repeat("a", 32) || r.Header.Get("X-Suma-Expected-Organization") != strings.Repeat("b", 32) {
				t.Error("preview changed the captured original-view headers")
			}
			_, _ = w.Write([]byte(`{"plan_sha256":"full-scope-plan","changes":[]}`))
		}))
		code, out, stderr := contextInvoke(t, server.URL, "en-US", `{"expected_version":2,"revision":2,"sha256":"reviewed-sha"}`, args...)
		server.Close()
		if code != 0 || calls != 1 || !strings.Contains(out, "full-scope-plan") {
			t.Fatalf("code=%d calls=%d err=%s", code, calls, stderr)
		}
	}
}

func TestProjectContextScopedCredentialNeverInventsApprovalIdentity(t *testing.T) {
	t.Setenv("SUMA_EXPECTED_USER", "")
	t.Setenv("SUMA_EXPECTED_ORGANIZATION", "")
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("X-Suma-Expected-User") != "" || r.Header.Get("X-Suma-Expected-Organization") != "" {
			t.Error("API credential acquired a fabricated browser view")
		}
		if r.URL.Path == "/api/v1/project-knowledge" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"draft-id","state":"draft"}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"forbidden","message":"browser approval required"}}`))
	}))
	defer server.Close()
	for i, args := range [][]string{
		{"knowledge", "create", "--file", "-"},
		{"knowledge", "command", "draft-id", "--file", "-"},
	} {
		var out, stderr bytes.Buffer
		all := append([]string{"--control", server.URL, "--project", "project-one", "--language", "en-US", "--token", "scoped-fixture"}, args...)
		code := Execute(context.Background(), all, strings.NewReader(`{"action":"publish","idempotency_key":"preserved-operation"}`), &out, &stderr)
		if i == 0 && (code != 0 || !strings.Contains(out.String(), "draft-id")) || i == 1 && (code != 1 || !strings.Contains(stderr.String(), "browser approval required")) {
			t.Fatalf("command %d code=%d out=%s err=%s", i, code, out.String(), stderr.String())
		}
	}
	if len(calls) != 2 || calls[0] != "POST /api/v1/project-knowledge" || calls[1] != "POST /api/v1/project-knowledge/draft-id/commands" {
		t.Fatalf("unexpected retry or identity-refresh side effect: %v", calls)
	}
}

func TestProjectContextReadTimeoutIsNotUnknownMutation(t *testing.T) {
	for _, tc := range []struct {
		resource string
		unknown  bool
	}{
		{"/requirements/r-1/design/preview", false},
		{"/requirements/r-1/design/commands", true},
		{"/requirements/r-1/design/preview/other", true},
		{"/unrelated/r-1/design/preview", true},
	} {
		calls := make(chan struct{}, 2)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls <- struct{}{}
			select {
			case <-r.Context().Done():
			case <-time.After(100 * time.Millisecond):
			}
		}))
		var out, stderr bytes.Buffer
		code := Execute(context.Background(), []string{"--control", server.URL, "--project", "project-one", "--language", "en-US", "--timeout", "20ms", "api", tc.resource, "--method", "POST", "--file", "-"}, strings.NewReader(`{"expected_version":1}`), &out, &stderr)
		server.Close()
		if code != 1 || len(calls) != 1 || strings.Contains(stderr.String(), "outcome may be unknown") != tc.unknown {
			t.Fatalf("path=%s code=%d calls=%d err=%s", tc.resource, code, len(calls), stderr.String())
		}
	}
}

func TestProjectContextUsageAndBoundedStdinNeverSend(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; _, _ = w.Write([]byte(`{}`)) }))
	defer server.Close()
	for _, tc := range []struct {
		args  []string
		input string
	}{
		{[]string{"knowledge", "show", "../other"}, ""},
		{[]string{"knowledge", "show", "id?project_id=other"}, ""},
		{[]string{"knowledge", "list", "--limit", "0"}, ""},
		{[]string{"knowledge", "history", "k-1", "--limit", "101"}, ""},
		{[]string{"knowledge", "list", "--state", "invented"}, ""},
		{[]string{"knowledge", "list", "--kind", "invented"}, ""},
		{[]string{"knowledge", "create"}, ""},
		{[]string{"requirement", "design", "show", "r-1", "--file", "-"}, `{}`},
		{[]string{"requirement", "design", "save", "r-1", "--file", "-"}, `{"project_id":"other"}`},
		{[]string{"knowledge", "create", "--file", "-"}, `{} {}`},
		{[]string{"knowledge", "create", "--file", "-"}, strings.Repeat(" ", 4*1024*1024+1)},
		{[]string{"knowledge", "create", "--file", "-"}, "{\"content\":\"\xff\"}"},
	} {
		if code, _, _ := contextInvoke(t, server.URL, "en-US", tc.input, tc.args...); code != 2 {
			t.Errorf("accepted invalid input: %v code=%d", tc.args, code)
		}
	}
	if calls != 0 {
		t.Fatalf("invalid command sent %d requests", calls)
	}
}
