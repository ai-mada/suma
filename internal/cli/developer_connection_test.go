package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Catch a command that reports configuration as connectivity, uses another
// project, mutates through an advertised tool, or trusts a revoked credential.
func TestDeveloperCheckActualPlatformMCPReadAndRevocation(t *testing.T) {
	db, err := store.Open("sqlite", filepath.Join(t.TempDir(), "check.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc := &platform.Service{DB: db, Dialect: "sqlite", DataDir: t.TempDir()}
	if err = svc.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	handler := (&platform.Server{Service: svc, Engine: engine}).Handler()
	var cookie *http.Cookie
	var user, organization string
	call := func(route string, body map[string]any, status int) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/v1/"+route, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		if cookie != nil {
			req.AddCookie(cookie)
		}
		if user != "" {
			req.Header.Set("X-Suma-Expected-User", user)
			req.Header.Set("X-Suma-Expected-Organization", organization)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s: %d %s", route, w.Code, w.Body.String())
		}
		for _, c := range w.Result().Cookies() {
			if c.Name == "suma_session" {
				cookie = c
			}
		}
		var value map[string]any
		if json.Unmarshal(w.Body.Bytes(), &value) != nil {
			t.Fatal("invalid setup response")
		}
		return value
	}
	setup := call("auth/setup", map[string]any{"name": "Check owner", "email": "check@example.test", "password": "test-password-strong-42", "organization": "Check org", "project": "Check project"}, 200)
	principal := setup["user"].(map[string]any)
	user, organization = principal["id"].(string), principal["tenant_id"].(string)
	project := setup["projects"].([]any)[0].(map[string]any)["id"].(string)
	token := call("auth/tokens", map[string]any{"scope": "agent", "project_id": project, "idempotency_key": "developer-check-agent-token"}, 201)["token"].(string)
	var mu sync.Mutex
	methods := []string{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/mcp/projects/"+project || r.URL.RawQuery != "" || r.Header.Get("Authorization") != "Bearer "+token {
			t.Error("MCP scope or credential changed")
		}
		if r.Method == http.MethodPost {
			raw, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(raw))
			var message struct {
				Method string `json:"method"`
				Params struct {
					Name string `json:"name"`
				} `json:"params"`
			}
			if json.Unmarshal(raw, &message) == nil {
				mu.Lock()
				methods = append(methods, message.Method+":"+message.Params.Name)
				mu.Unlock()
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	invoke := func() (int, string, string) {
		var out, stderr strings.Builder
		code := Execute(context.Background(), []string{"--control", ts.URL, "--project", project, "--token", token, "--language", "en-US", "developer", "check"}, strings.NewReader(""), &out, &stderr)
		return code, out.String(), stderr.String()
	}
	before := time.Now().UTC()
	code, output, stderr := invoke()
	if code != 0 {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var report struct {
		ProjectID string    `json:"project_id"`
		Endpoint  string    `json:"endpoint"`
		CheckedAt time.Time `json:"checked_at"`
		Protocol  string    `json:"protocol_version"`
		Tools     []struct {
			Name     string `json:"name"`
			ReadOnly bool   `json:"read_only"`
		} `json:"tools"`
		Probe struct {
			Tool      string `json:"tool"`
			ProjectID string `json:"project_id"`
			SHA256    string `json:"response_sha256"`
		} `json:"read_probe"`
	}
	if json.Unmarshal([]byte(output), &report) != nil || report.ProjectID != project || report.Endpoint != ts.URL+"/api/v1/mcp/projects/"+project || report.CheckedAt.Before(before) || report.CheckedAt.After(time.Now().UTC()) || report.Protocol != "2025-11-25" || len(report.Tools) != 4 || report.Probe.Tool != "suma_project_overview" || report.Probe.ProjectID != project || len(report.Probe.SHA256) != 64 {
		t.Fatalf("inaccurate report: %s", output)
	}
	for _, tool := range report.Tools {
		if !tool.ReadOnly {
			t.Fatalf("agent was advertised writable tool %s", tool.Name)
		}
	}
	mu.Lock()
	actualMethods := strings.Join(methods, ",")
	mu.Unlock()
	if actualMethods != "initialize:,notifications/initialized:,tools/list:,tools/call:suma_project_overview" {
		t.Fatalf("unexpected effects: %s", actualMethods)
	}
	if strings.Contains(output+stderr, token) {
		t.Fatal("credential disclosed")
	}
	// A project API catalogue also advertises writes, but this diagnostic must
	// still call only the fixed overview tool.
	token = call("auth/tokens", map[string]any{"scope": "api", "project_id": project, "idempotency_key": "developer-check-api-token"}, 201)["token"].(string)
	code, output, stderr = invoke()
	if code != 0 || json.Unmarshal([]byte(output), &report) != nil {
		t.Fatalf("API check: %d %s", code, stderr)
	}
	writeDeclared := false
	for _, tool := range report.Tools {
		if tool.Name == "suma_project_requirement_draft_create" && !tool.ReadOnly {
			writeDeclared = true
		}
	}
	if !writeDeclared {
		t.Fatal("API catalogue did not report its real draft capability")
	}
	mu.Lock()
	actualMethods = strings.Join(methods, ",")
	mu.Unlock()
	if actualMethods != "initialize:,notifications/initialized:,tools/list:,tools/call:suma_project_overview,initialize:,notifications/initialized:,tools/list:,tools/call:suma_project_overview" {
		t.Fatalf("advertised writes were executed: %s", actualMethods)
	}
	for _, table := range []string{"suma_requirements", "suma_tasks", "dag_runs", "suma_tests"} {
		var count int
		if err = db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s changed: %d %v", table, count, err)
		}
	}
	hash := sha256.Sum256([]byte(token))
	if _, err = db.Exec("UPDATE suma_credential_authority SET revoked=1 WHERE token_hash=?", hex.EncodeToString(hash[:])); err != nil {
		t.Fatal(err)
	}
	code, output, stderr = invoke()
	if code != 1 || output != "" || strings.Contains(stderr, token) || !strings.Contains(stderr, "mcp_initialize_failed") {
		t.Fatalf("revocation reported success: %d %s %s", code, output, stderr)
	}
}

// Real SDK responses must not turn a denied, mismatched or non-read-only tool
// into a successful check. Arbitrary server text must stay out of CLI errors.
func TestDeveloperCheckRejectsUnverifiedMCPProbe(t *testing.T) {
	for _, tc := range []struct {
		name              string
		readonly, isError bool
		project, code     string
	}{
		{"wrong_project", true, false, "other-project", "mcp_probe_scope"},
		{"tool_error", true, true, "project-one", "mcp_probe_failed"},
		{"not_readonly", false, false, "project-one", "mcp_probe_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := mcp.NewServer(&mcp.Implementation{Name: "suma", Version: "fixture"}, nil)
			calls := 0
			mcp.AddTool[struct{}, any](server, &mcp.Tool{Name: "suma_project_overview", Annotations: &mcp.ToolAnnotations{ReadOnlyHint: tc.readonly}}, func(context.Context, *mcp.CallToolRequest, struct{}) (*mcp.CallToolResult, any, error) {
				calls++
				if tc.isError {
					return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "server-secret-diagnostic"}}}, nil, nil
				}
				return nil, map[string]any{"project": map[string]any{"id": tc.project}}, nil
			})
			handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
			ts := httptest.NewServer(handler)
			defer ts.Close()
			var out, stderr strings.Builder
			code := Execute(context.Background(), []string{"--control", ts.URL, "--project", "project-one", "--token", "fixture-token", "--language", "zh-CN", "developer", "check"}, strings.NewReader(""), &out, &stderr)
			if code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), tc.code) || strings.Contains(stderr.String(), "server-secret-diagnostic") || strings.Contains(stderr.String(), "fixture-token") || !strings.Contains(stderr.String(), "MCP") {
				t.Fatalf("unverified result accepted: %d %s %s", code, out.String(), stderr.String())
			}
			if !tc.readonly && calls != 0 {
				t.Fatal("called a tool without its read-only declaration")
			}
		})
	}
}

func TestDeveloperCheckRejectsRedirectTimeoutAndOversize(t *testing.T) {
	destinationCalls := 0
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { destinationCalls++; w.WriteHeader(500) }))
	defer destination.Close()
	for _, mode := range []string{"redirect", "timeout", "oversize"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			timeout := "2s"
			if mode == "timeout" {
				timeout = "50ms"
			}
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch mode {
				case "redirect":
					http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
				case "timeout":
					select {
					case <-r.Context().Done():
					case <-time.After(200 * time.Millisecond):
					}
				case "oversize":
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, strings.Repeat(" ", (1<<20)+1))
				}
			}))
			defer ts.Close()
			var out, stderr strings.Builder
			code := Execute(context.Background(), []string{"--control", ts.URL, "--project", "project-one", "--token", "fixture-token", "--timeout", timeout, "developer", "check"}, strings.NewReader(""), &out, &stderr)
			if code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "mcp_initialize_failed") || calls != 1 {
				t.Fatalf("unsafe transport accepted/retried: %d calls=%d %s %s", code, calls, out.String(), stderr.String())
			}
		})
	}
	if destinationCalls != 0 {
		t.Fatal("Bearer diagnostic followed a redirect")
	}
}

// Invalid scope and plaintext remote destinations must fail before any request.
func TestDeveloperCheckRejectsInvalidScopeAndRemotePlaintext(t *testing.T) {
	calls := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer ts.Close()
	for _, tc := range []struct {
		control, project, token string
		args                    []string
	}{
		{ts.URL, "", "secret", []string{"check"}}, {ts.URL, "../other", "secret", []string{"check"}}, {ts.URL, "project?token=secret", "secret", []string{"check"}}, {ts.URL, "project-one", "", []string{"check"}}, {"http://remote.example", "project-one", "secret", []string{"check"}}, {ts.URL, "project-one", "secret", []string{"check", "--endpoint", "https://other.example"}},
	} {
		var out, stderr strings.Builder
		args := append([]string{"--control", tc.control, "--project", tc.project, "--token", tc.token, "developer"}, tc.args...)
		if code := Execute(context.Background(), args, strings.NewReader(""), &out, &stderr); code != 2 || out.Len() != 0 || strings.Contains(stderr.String(), "secret") {
			t.Fatalf("invalid scope accepted: %d %s %s", code, out.String(), stderr.String())
		}
	}
	if calls != 0 {
		t.Fatalf("invalid input sent %d requests", calls)
	}
}
