package platform

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPPublicURLUsesTrustedOriginOnly(t *testing.T) {
	for _, tt := range []struct {
		name, configured, listener, origin, source string
		invalid                                    bool
	}{
		{"configured", "https://suma.example/", "0.0.0.0:4324", "https://suma.example", "configured", false},
		{"local", "", "127.0.0.1:4324", "http://127.0.0.1:4324", "listener_loopback", false},
		{"ipv6", "", "[::1]:4324", "http://[::1]:4324", "listener_loopback", false},
		{"external-unconfigured", "", "0.0.0.0:4324", "", "unconfigured", false},
		{"plaintext", "http://suma.example", "", "", "", true},
		{"credentials", "https://user:secret@suma.example", "", "", "", true},
		{"query", "https://suma.example?token=secret", "", "", "", true},
		{"fragment", "https://suma.example#secret", "", "", "", true},
		{"path", "https://suma.example/api", "", "", "", true},
		{"port", "https://suma.example:99999", "", "", "", true},
		{"wildcard", "http://0.0.0.0:4324", "", "", "", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			origin, source, err := ResolveMCPPublicURL(tt.configured, tt.listener)
			if (err != nil) != tt.invalid || origin != tt.origin || source != tt.source {
				t.Fatalf("origin=%q source=%q err=%v", origin, source, err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("URL validation disclosed input")
			}
		})
	}
}

type mcpTestTransport struct {
	base               http.RoundTripper
	token              string
	user, organization string
}

func (t mcpTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header = r.Header.Clone()
	r.Header.Set("Authorization", "Bearer "+t.token)
	r.Header.Set("X-Suma-Expected-User", t.user)
	r.Header.Set("X-Suma-Expected-Organization", t.organization)
	return t.base.RoundTrip(r)
}

func mcpTestClient(t *testing.T, serverURL, project, token string, user Principal, protocol string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "suma-contract-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{
		Endpoint:   serverURL + "/api/v1/mcp/projects/" + project,
		HTTPClient: &http.Client{Transport: mcpTestTransport{http.DefaultTransport, token, user.ID, user.TenantID}, Timeout: 10 * time.Second},
	}, &mcp.ClientSessionOptions{ProtocolVersion: protocol})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { session.Close() })
	return session
}

// A stale initialization or an earlier tools/list must never enlarge Agent scope.
func TestMCPRealSDKAgentListsOnlyCurrentReadsAndCurrentRevocationRejects(t *testing.T) {
	f := newFixture(t)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "agent"}), 201)["token"].(string)
	s := &Server{Service: f.s}
	ts := httptest.NewServer(s.NewMCPHandler())
	t.Cleanup(ts.Close)
	for _, protocol := range []string{"2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			session := mcpTestClient(t, ts.URL, f.project, token, f.user, protocol)
			list, err := session.ListTools(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			// T196/T197 add published-knowledge discovery, not management
			// authority. This exact allowlist must never grow from list output.
			allowed := map[string]bool{"suma_project_overview": true, "suma_project_tasks": true, "suma_project_search": true, "suma_project_knowledge": true, "suma_project_knowledge_search": true, "suma_project_knowledge_index_search": true}
			if len(list.Tools) != len(allowed) || list.CacheScope != "private" || list.TTLMs != 0 {
				t.Fatalf("unexpected Agent list/cache: %+v", list)
			}
			for _, tool := range list.Tools {
				if !allowed[tool.Name] {
					t.Fatalf("Agent discovered an unauthorized or duplicate tool %s", tool.Name)
				}
				delete(allowed, tool.Name)
			}
			if len(allowed) != 0 {
				t.Fatal("Agent lost declared read-only discovery", allowed)
			}
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suma_project_overview", Arguments: map[string]any{}})
			if err != nil || result.IsError {
				t.Fatalf("read failed: %+v %v", result, err)
			}
			denied, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suma_project_task_draft_create", Arguments: map[string]any{"title": "must not be created", "idempotency_key": "mcp-agent-denied"}})
			if err == nil && !denied.IsError {
				t.Fatal("Agent called API write tool")
			}
		})
	}
	session := mcpTestClient(t, ts.URL, f.project, token, f.user, "2025-11-25")
	if _, err := f.s.DB.Exec(`UPDATE suma_credential_authority SET revoked=1 WHERE token_hash=?`, digest(token)); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ListTools(context.Background(), nil); err == nil {
		t.Fatal("old MCP initialization survived credential revocation")
	}
}

func TestMCPRejectsCookieBrowserRunnerOriginHostAndCrossProject(t *testing.T) {
	f := newFixture(t)
	agent := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "agent"}), 201)["token"].(string)
	runner := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	s := &Server{Service: f.s}
	h := s.NewMCPHandler()
	for _, tt := range []struct {
		name, token, project, origin, host string
		cookie                             bool
		want                               int
	}{
		{"cookie", "", f.project, "", "127.0.0.1:4324", true, 401},
		{"browser", f.cookie.Value, f.project, "", "127.0.0.1:4324", false, 403},
		{"runner", runner, f.project, "", "127.0.0.1:4324", false, 403},
		{"project", agent, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "", "127.0.0.1:4324", false, 403},
		{"origin", agent, f.project, "https://evil.example", "127.0.0.1:4324", false, 403},
		{"host", agent, f.project, "", "evil.example", false, 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4324/api/v1/mcp/projects/"+tt.project, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
			r.Host = tt.host
			if tt.token != "" {
				r.Header.Set("Authorization", "Bearer "+tt.token)
			}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.cookie {
				r.AddCookie(f.cookie)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tt.want || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("code=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
			}
		})
	}
}

func TestMCPDeveloperMetadataUsesCanonicalOriginAndNeverVerifiesClient(t *testing.T) {
	f := newFixture(t)
	s := &Server{Service: f.s, PublicURL: "https://suma.example", PublicURLSource: "configured"}
	m, err := s.DeveloperConnection(context.Background(), f.actor, f.project)
	if err != nil {
		t.Fatal(err)
	}
	if m.EndpointURL != "https://suma.example/api/v1/mcp/projects/"+f.project || m.ClientConnectionVerified || len(m.Tools) != len(DeclaredApplicationTools()) {
		t.Fatalf("metadata=%+v", m)
	}
	raw, _ := json.Marshal(m)
	if strings.Contains(string(raw), f.cookie.Value) {
		t.Fatal("metadata contains credential")
	}
	agentToken := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "agent"}), 201)["token"].(string)
	agent, err := f.s.AuthenticateActor(context.Background(), agentToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeveloperConnection(context.Background(), agent, f.project); err == nil {
		t.Fatal("Agent accessed developer administration metadata")
	}
}

func mcpTestResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// The MCP surface must use the same durable domain receipt, not an SDK session cache.
func TestMCPRealSDKAPICreatesDraftAndReconcilesSameOperation(t *testing.T) {
	f := newFixture(t)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	s := &Server{Service: f.s}
	ts := httptest.NewServer(s.NewMCPHandler())
	t.Cleanup(ts.Close)
	session := mcpTestClient(t, ts.URL, f.project, token, f.user, "2025-11-25")
	list, err := session.ListTools(context.Background(), nil)
	if err != nil || len(list.Tools) != len(DeclaredApplicationTools()) {
		t.Fatalf("API tools=%+v err=%v", list, err)
	}
	args := map[string]any{"title": "MCP requirement draft", "idempotency_key": "mcp-sdk-requirement-create"}
	first, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suma_project_requirement_draft_create", Arguments: args})
	if err != nil || first.IsError {
		t.Fatalf("draft create=%+v err=%v", first, err)
	}
	id, _ := mcpTestResult(t, first)["id"].(string)
	if id == "" {
		t.Fatal("write did not return durable identifier")
	}
	replay, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suma_project_requirement_draft_create", Arguments: args})
	if err != nil || replay.IsError || mcpTestResult(t, replay)["id"] != id {
		t.Fatalf("same operation replay=%+v err=%v", replay, err)
	}
	args["title"] = "must not replace the original"
	drift, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suma_project_requirement_draft_create", Arguments: args})
	if err != nil || !drift.IsError || mcpTestResult(t, drift)["error"].(map[string]any)["status"] != float64(409) {
		t.Fatalf("drift=%+v err=%v", drift, err)
	}
	record := f.call("GET", "/requirements/"+id+"?project_id="+f.project, nil, 200)
	if record["status"] != "draft" || record["title"] != "MCP requirement draft" {
		t.Fatalf("MCP write escaped draft contract: %+v", record)
	}
}

func TestMCPSDKValidationPreservesRawArgumentsAndRedactsValues(t *testing.T) {
	f := newFixture(t)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	s := &Server{Service: f.s}
	ts := httptest.NewServer(s.NewMCPHandler())
	t.Cleanup(ts.Close)
	session := mcpTestClient(t, ts.URL, f.project, token, f.user, "2026-07-28")
	for _, tt := range []struct{ name, tool, raw string }{
		{"schema", "suma_project_tasks", `{"limit":"secret-value-must-not-echo"}`},
		{"duplicate", "suma_project_tasks", `{"limit":1,"limit":2}`},
		{"fractional-lexeme", "suma_project_tasks", `{"limit":1.0000000000000001}`},
		{"null", "suma_project_tasks", `null`},
		{"unknown", "suma_project_tasks", `{"project_id":"secret-value-must-not-echo"}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: tt.tool, Arguments: json.RawMessage(tt.raw)})
			if err != nil || !result.IsError {
				t.Fatalf("invalid input accepted: %+v %v", result, err)
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "secret-value-must-not-echo") {
				t.Fatalf("validation disclosed argument: %s", raw)
			}
			if _, ok := mcpTestResult(t, result)["error"].(map[string]any); !ok {
				t.Fatalf("missing safe machine error: %s", raw)
			}
		})
	}
}

func TestMCPStatelessUnsupportedMethodsBodyLimitAndMissingWriteView(t *testing.T) {
	f := newFixture(t)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api"}), 201)["token"].(string)
	s := &Server{Service: f.s}
	h := s.NewMCPHandler()
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		r := httptest.NewRequest(method, "http://127.0.0.1:4324/api/v1/mcp/projects/"+f.project, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 405 || w.Header().Get("Allow") != "POST" {
			t.Fatalf("%s=%d %s", method, w.Code, w.Body.String())
		}
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4324/api/v1/mcp/projects/"+f.project, strings.NewReader(strings.Repeat(" ", (1<<20)+1)))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 413 {
		t.Fatalf("unbounded body: %d %s", w.Code, w.Body.String())
	}
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	session := mcpTestClient(t, ts.URL, f.project, token, Principal{}, "2025-11-25")
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "suma_project_task_draft_create", Arguments: map[string]any{"title": "no original view", "idempotency_key": "mcp-missing-view"}})
	if err != nil || !result.IsError || mcpTestResult(t, result)["error"].(map[string]any)["code"] != "view_context_required" {
		t.Fatalf("missing write view accepted: %+v %v", result, err)
	}
}

type mcpGatedBody struct {
	io.ReadCloser
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *mcpGatedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started); <-b.release })
	return b.ReadCloser.Read(p)
}

// With the SDK's new request lifecycle, disconnecting a call must cancel the
// actual domain wait even while the DB is unavailable. A local client error
// alone would not prove that server work stopped.
func TestMCPNewProtocolCancellationStopsCurrentAuthorityWait(t *testing.T) {
	f := newFixture(t)
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "agent"}), 201)["token"].(string)
	h := (&Server{Service: f.s}).NewMCPHandler()
	var gateNext atomic.Bool
	started, release, handled := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gateNext.Swap(false) {
			r.Body = &mcpGatedBody{ReadCloser: r.Body, started: started, release: release}
			defer close(handled)
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	session := mcpTestClient(t, ts.URL, f.project, token, f.user, "2026-07-28")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gateNext.Store(true)
	callFinished := make(chan error, 1)
	go func() {
		_, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "suma_project_overview", Arguments: map[string]any{}})
		callFinished <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("MCP request did not reach SDK body reader")
	}
	f.s.DB.SetMaxOpenConns(1)
	connection, err := f.s.DB.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	waits := f.s.DB.Stats().WaitCount
	releaseOnce.Do(func() { close(release) })
	deadline := time.Now().Add(3 * time.Second)
	for f.s.DB.Stats().WaitCount == waits && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if f.s.DB.Stats().WaitCount == waits {
		t.Fatal("tool did not enter real current-authority DB wait")
	}
	cancel()
	select {
	case err = <-callFinished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("client cancellation=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("client call did not cancel")
	}
	select {
	case <-handled:
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled MCP request kept domain work running")
	}
}
