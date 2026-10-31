package platform

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// This host is opt-in and separate from ordinary tests. Its setup establishes
// an existing task/run/cloud submission; only the subsequent GUI checks T169.
func TestWorkBuddyCloudTurn169Actual(t *testing.T) {
	if os.Getenv("SUMA_CLOUD_TURN_169_ACTUAL") != "1" {
		t.Skip("actual169 requires the final source/runtime gate")
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatal("actual169 working directory unavailable")
	}
	for {
		if _, err = os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		next := filepath.Dir(root)
		if next == root {
			t.Fatal("actual169 repository root unavailable")
		}
		root = next
	}
	directory := filepath.Join(root, ".cache", "workbuddy-cloud-turns-169", "actual")
	if attempt := os.Getenv("SUMA_CLOUD_TURN_169_ATTEMPT"); attempt == "02" {
		directory = filepath.Join(directory, "attempt-02")
	} else if attempt != "" {
		t.Fatal("actual169 only the explicitly gated second attempt is supported")
	}
	requested, err := filepath.Abs(os.Getenv("SUMA_CLOUD_TURN_169_ACTUAL_DIR"))
	if err != nil || filepath.Clean(requested) != filepath.Clean(directory) {
		t.Fatal("actual169 exact owned directory required")
	}
	temporary, err := filepath.Abs(os.TempDir())
	if err != nil || !strings.HasPrefix(temporary, directory+string(filepath.Separator)) {
		t.Fatal("actual169 process temporary directory must be owned")
	}
	cloudTurn169Gate(t, root, directory)
	private := filepath.Join(directory, "private")
	if os.MkdirAll(private, 0700) != nil {
		t.Fatal("actual169 private directory unavailable")
	}
	for _, name := range []string{"private/descriptor.json", "private/snapshot.sqlite", "host.stop"} {
		if _, e := os.Stat(filepath.Join(directory, filepath.FromSlash(name))); !os.IsNotExist(e) {
			t.Fatal("actual169 retained prior state; no implicit reseed")
		}
	}
	ctx := context.Background()
	seed := newWorkBuddyTaskDispatchFixture(t)
	f, w1 := seed.f, seed.w
	f.handler = (&Server{Service: f.s, Engine: f.e, WorkBuddy: w1, WebDir: filepath.Join(directory, "absent-web")}).Handler()
	plan := workBuddyTaskPreview(t, seed)
	run := workBuddyTaskStart(t, seed, plan, newID())
	credential := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "cloud-turn169-seed-worker", "profiles": []any{}, "deployment": false}), 201)
	token, ok := credential["token"].(string)
	if !ok || token == "" {
		t.Fatal("actual169 registered private seed worker credential unavailable")
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": "cloud-turn169-seed-worker", "capabilities": []string{"external_workflow:workbuddy-cloud"}, "idempotency_key": newID()}), 200)
	var assignment dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &assignment) != nil || assignment.RunID != run.ID || assignment.Attempt != 1 || assignment.Fence != 1 {
		t.Fatal("actual169 seed original assignment unavailable")
	}
	cloud := &cloudFixture{f: f, w: w1, actor: domainCActor(t, f.s, token), claimToken: token, request: wc.CloudCreateRequest{ProjectID: f.project, DelegationID: plan.DelegationID, ExpectedVersion: 1, Assignment: repositoryTestRef(assignment), IdempotencyKey: newID()}}
	sub := createCloudFixture(t, cloud)
	a, result := cloudResultFixture(t, cloud)
	if a.DispatchID != assignment.DispatchID {
		t.Fatal("actual169 seed dispatch identity changed")
	}
	result.WorkBuddyCloudWait.SubmissionID, result.WorkBuddyCloudWait.SubmissionVersion = sub.ID, sub.Version
	f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
	if err = f.s.InitializeWorkBuddyCloudTurns(ctx); err != nil {
		t.Fatal("actual169 turn schema unavailable")
	}
	f.call("GET", "/auth/session", nil, 200)
	var sequence int
	var dbName, dbPath string
	if f.s.DB.QueryRow("PRAGMA database_list").Scan(&sequence, &dbName, &dbPath) != nil {
		t.Fatal("actual169 SQLite source unavailable")
	}
	provider := newCloudTurn169Vendor(t)
	originalOrigin, err := url.Parse(w1.app.BaseURL)
	if err != nil || originalOrigin.Hostname() != "127.0.0.1" {
		t.Fatal("actual169 original fixture provider must be loopback")
	}
	target, _ := url.Parse(provider.server.URL)
	base := http.DefaultTransport
	var blocked atomic.Int64
	client := &http.Client{Transport: cloudTurnRoundTripper(func(request *http.Request) (*http.Response, error) {
		rest := request.URL.Scheme == originalOrigin.Scheme && request.URL.Host == originalOrigin.Host && request.Method == http.MethodGet && request.URL.Path == "/openapi/v2/tasks/task-dispatch-cloud"
		acp := request.URL.String() == "https://acp.workbuddy.cn/sessions/task-dispatch-cloud" && (request.Method == http.MethodGet || request.Method == http.MethodPost)
		if !rest && !acp {
			blocked.Add(1)
			return nil, fmt.Errorf("actual169 nonlocal or unsupported outbound refused")
		}
		copy := request.Clone(request.Context())
		destination := *request.URL
		destination.Scheme, destination.Host = target.Scheme, target.Host
		copy.URL = &destination
		return base.RoundTrip(copy)
	}), CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	w1.app.HTTPClient = client
	w1.turnRuntime = w1.cloudTurnStartRuntime
	db2, err := store.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal("actual169 second SQLite connection unavailable")
	}
	defer db2.Close()
	s2 := &Service{DB: db2, Dialect: f.s.Dialect, DataDir: f.s.DataDir}
	w2, err := NewWorkBuddyConnections(s2, w1.app, w1.key)
	if err != nil {
		t.Fatal("actual169 second connection service unavailable")
	}
	var httpMu sync.Mutex
	apiRequests := map[string]int{}
	stats := func() map[string]any {
		out := provider.stats()
		out["seed_provider_creates"] = seed.creates.Load()
		out["seed_provider_queries"] = seed.queries.Load()
		out["blocked_nonlocal_outbound"] = blocked.Load()
		for name, table := range map[string]string{"runs": "dag_runs", "nodes": "dag_nodes", "attempts": "dag_attempts", "delegations": "suma_workbuddy_cloud_delegations", "submissions": "suma_workbuddy_cloud_submissions", "turn_heads": "suma_workbuddy_cloud_turn_heads", "turns": "suma_workbuddy_cloud_turns", "entries": "suma_workbuddy_cloud_turn_entries", "permissions": "suma_workbuddy_cloud_turn_permissions", "commands": "suma_workbuddy_cloud_turn_commands", "tool_rows": "suma_workbuddy_cloud_turn_tools", "candidate_revisions": "dag_candidate_revisions", "test_reports": "suma_test_plan_reports", "reviews": "suma_reviews", "release_candidates": "suma_release_candidates"} {
			var count int
			if f.s.DB.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count) != nil {
				out["database_stats_unavailable"] = true
			} else {
				out[name] = count
			}
		}
		current, e := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
		if e != nil || len(current.Nodes) != 1 {
			out["run_stats_unavailable"] = true
		} else {
			node := current.Nodes[0]
			out["run_id"], out["run_version"], out["run_state"] = current.ID, current.Version, current.State
			out["node_id"], out["node_state"], out["node_reason"] = node.ID, node.State, node.Reason
			out["node_attempt"], out["node_fence"], out["node_worker"] = node.Attempt, node.Fence, node.WorkerID
		}
		currentTask, e := f.s.Get(ctx, f.actor, f.project, "tasks", seed.task.ID)
		if e != nil {
			out["task_stats_unavailable"] = true
		} else {
			out["task_version"], out["task_state"], out["task_run_id"] = currentTask.Version, currentTask.Status, currentTask.RunID
			out["task_verified_run_present"] = currentTask.Data["verified_run_id"] != nil && currentTask.Data["verified_run_id"] != ""
		}
		var payloads []wc.CloudTurn
		rows, e := f.s.DB.Query("SELECT payload FROM suma_workbuddy_cloud_turns ORDER BY id")
		if e != nil {
			out["turn_stats_unavailable"] = true
		} else {
			for rows.Next() {
				var raw string
				var turn wc.CloudTurn
				if rows.Scan(&raw) != nil || json.Unmarshal([]byte(raw), &turn) != nil {
					out["turn_stats_unavailable"] = true
					break
				}
				payloads = append(payloads, turn)
			}
			if rows.Err() != nil {
				out["turn_stats_unavailable"] = true
			}
			rows.Close()
		}
		out["turn_facts"] = payloads
		var head int
		if e = f.s.DB.QueryRow("SELECT COALESCE(MAX(version),0) FROM suma_workbuddy_cloud_turn_heads").Scan(&head); e != nil {
			out["head_stats_unavailable"] = true
		}
		out["head_version"] = head
		httpMu.Lock()
		counts := map[string]int{}
		for k, v := range apiRequests {
			counts[k] = v
		}
		httpMu.Unlock()
		out["replica_http_requests"] = counts
		return out
	}
	const uiOrigin = "http://127.0.0.1:44047"
	serverFor := func(name string, s *Service, wc *WorkBuddyConnections) *httptest.Server {
		handler := (&Server{Service: s, Engine: f.e, WorkBuddy: wc, PublicURL: uiOrigin, AllowedOrigins: []string{uiOrigin}, WebDir: filepath.Join(directory, "absent-web")}).Handler()
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Suma-Actual-Replica", name)
			if r.URL.Path == "/__actual169/status" {
				cookie, e := r.Cookie(f.cookie.Name)
				if r.Method != http.MethodGet || r.URL.RawQuery != "" || e != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(f.cookie.Value)) != 1 {
					w.WriteHeader(403)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Cache-Control", "no-store")
				_ = json.NewEncoder(w).Encode(stats())
				return
			}
			prompt := r.URL.Path == "/api/v1/workbuddy-cloud-submissions/"+sub.ID+"/turns"
			command := strings.HasPrefix(r.URL.Path, "/api/v1/workbuddy-cloud-turns/") && (strings.HasSuffix(r.URL.Path, "/reply") || strings.HasSuffix(r.URL.Path, "/cancel"))
			if r.Method != http.MethodGet && !(r.Method == http.MethodPost && (name == "primary" && prompt || name == "secondary" && command)) {
				w.WriteHeader(403)
				return
			}
			httpMu.Lock()
			apiRequests[name+" "+r.Method+" "+r.URL.Path]++
			httpMu.Unlock()
			handler.ServeHTTP(w, r)
		}))
	}
	primary, secondary := serverFor("primary", f.s, w1), serverFor("secondary", s2, w2)
	defer primary.Close()
	defer secondary.Close()
	initial := stats()
	if initial["database_stats_unavailable"] != nil || initial["run_stats_unavailable"] != nil || initial["task_stats_unavailable"] != nil || initial["turn_stats_unavailable"] != nil {
		t.Fatal("actual169 initialized source unavailable")
	}
	if fmt.Sprint(initial["seed_provider_creates"]) != "1" || fmt.Sprint(initial["run_state"]) != "running" || fmt.Sprint(initial["node_state"]) != "waiting" || fmt.Sprint(initial["turns"]) != "0" {
		t.Fatal("actual169 seed counts or original waiting state invalid")
	}
	cloudTurn169Write(t, filepath.Join(private, "descriptor.json"), map[string]any{"stage": "T-WORKBUDDY-CLOUD-TURNS-169", "origin": primary.URL, "secondary_origin": secondary.URL, "ui_origin": uiOrigin, "pid": os.Getpid(), "project_id": f.project, "task_id": seed.task.ID, "run_id": run.ID, "node_id": sub.NodeID, "submission_id": sub.ID, "owner": map[string]any{"user_id": f.view.ID, "organization_id": f.view.TenantID, "cookie": map[string]any{"name": f.cookie.Name, "value": f.cookie.Value}}, "initial_stats": initial, "database_source": dbPath, "data_directory_source": f.s.DataDir, "seed": "Existing fake task/connection fixture; official domain preview/start, one actual runner claim/Create/result HTTP seed creates the known original task/run/submission and waiting attempt1/fence1. Not GUI and not a repeated168 acceptance run.", "vendor_boundary": "All provider REST/ACP packets use controlled loopback HTTP/SSE; fixed official ACP origin is mapped by this private transport; no real WorkBuddy or model.", "replica_boundary": "Two normal Server and WorkBuddyConnections instances with independent Service/SQLite connections to the same private database; same host, shared existing DAG reader, not physical multi-machine."})
	t.Log("actual169 isolated two-replica HTTP/SQLite host ready; connection material remains private")
	deadline := time.NewTimer(20 * time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timedOut := false
	waiting := true
	for waiting {
		select {
		case <-deadline.C:
			timedOut = true
			waiting = false
		case <-ticker.C:
			if _, e := os.Stat(filepath.Join(directory, "host.stop")); e == nil {
				waiting = false
			}
		}
	}
	primary.Close()
	secondary.Close()
	closeCtx, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	close1, close2 := w1.CloseCloudTurnRuntimes(closeCtx), w2.CloseCloudTurnRuntimes(closeCtx)
	provider.server.Close()
	final := stats()
	snapshot := filepath.Join(private, "snapshot.sqlite")
	if _, e := os.Stat(snapshot); !os.IsNotExist(e) {
		t.Fatal("actual169 original snapshot retained")
	}
	if _, e := f.s.DB.ExecContext(closeCtx, "VACUUM INTO ?", snapshot); e != nil {
		t.Fatal("actual169 consistent private snapshot failed")
	}
	if os.Chmod(snapshot, 0600) != nil {
		t.Fatal("actual169 snapshot permissions unavailable")
	}
	data, e := os.ReadFile(snapshot)
	if e != nil {
		t.Fatal("actual169 private snapshot hash unavailable")
	}
	sum := sha256.Sum256(data)
	cloudTurn169Write(t, filepath.Join(private, "host-final.json"), map[string]any{"stats": final, "host_closed": true, "host_timeout": timedOut, "primary_runtime_closed": close1 == nil, "secondary_runtime_closed": close2 == nil, "snapshot": map[string]any{"path": snapshot, "bytes": len(data), "sha256": hex.EncodeToString(sum[:]), "method": "Parameterized VACUUM INTO new absent target after local streams and both HTTP hosts close"}, "original_database_source": dbPath, "original_data_directory_source": f.s.DataDir})
	if timedOut || close1 != nil || close2 != nil {
		t.Fatal("actual169 close or deadline failed; snapshot retained")
	}
}

type cloudTurn169Flow struct {
	events             chan string
	permissionAnswered chan struct{}
	answerOnce         sync.Once
	promptID           json.RawMessage
	round              int
}
type cloudTurn169Vendor struct {
	server                                                                 *httptest.Server
	mu                                                                     sync.Mutex
	flows                                                                  map[string]*cloudTurn169Flow
	rest, attach, initialize, load, prompt, permission, cancel, unexpected int
}

func newCloudTurn169Vendor(t *testing.T) *cloudTurn169Vendor {
	t.Helper()
	v := &cloudTurn169Vendor{flows: map[string]*cloudTurn169Flow{}}
	v.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/openapi/v2/tasks/task-dispatch-cloud" {
			v.mu.Lock()
			v.rest++
			v.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"task_id":"task-dispatch-cloud","status":"completed","link":"https://acp.workbuddy.cn/sessions/task-dispatch-cloud","token":"private-ticket169-actual"}`)
			return
		}
		if r.URL.Path != "/sessions/task-dispatch-cloud" {
			v.mu.Lock()
			v.unexpected++
			v.mu.Unlock()
			w.WriteHeader(404)
			return
		}
		if r.Method == http.MethodGet {
			v.mu.Lock()
			v.attach++
			id := fmt.Sprintf("actual169-connection-%d", v.attach)
			flow := &cloudTurn169Flow{events: make(chan string, 32), permissionAnswered: make(chan struct{})}
			v.flows[id] = flow
			v.mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Acp-Connection-Id", id)
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			for {
				select {
				case raw := <-flow.events:
					_, _ = fmt.Fprintf(w, "data: %s\n\n", raw)
					w.(http.Flusher).Flush()
				case <-r.Context().Done():
					return
				}
			}
		}
		var body map[string]json.RawMessage
		if r.Method != http.MethodPost || json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&body) != nil {
			w.WriteHeader(400)
			return
		}
		v.mu.Lock()
		flow := v.flows[r.Header.Get("Acp-Connection-Id")]
		v.mu.Unlock()
		if flow == nil {
			w.WriteHeader(403)
			return
		}
		method := cloudTurnHTTPField(body, "method")
		switch method {
		case "initialize":
			v.mu.Lock()
			v.initialize++
			v.mu.Unlock()
			flow.events <- cloudTurnHTTPReply(body["id"], `{"protocolVersion":1,"agentCapabilities":{"loadSession":true}}`)
		case "session/load":
			v.mu.Lock()
			v.load++
			v.mu.Unlock()
			flow.events <- cloudTurnHTTPReply(body["id"], `{}`)
		case "session/prompt":
			v.mu.Lock()
			v.prompt++
			flow.round = v.prompt
			flow.promptID = append(json.RawMessage{}, body["id"]...)
			round := flow.round
			v.mu.Unlock()
			if round == 1 {
				flow.events <- cloudTurn169Update(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"正在核对后续目标。Checking the follow-up goal. "}}`)
				flow.events <- `{"jsonrpc":"2.0","id":"actual169-permission-rpc","method":"session/request_permission","params":{"sessionId":"task-dispatch-cloud","toolCall":{"toolCallId":"controlled-check169","title":"Review controlled check / 审阅受控检查","kind":"execute","rawInput":{"command":"controlled-check --verify-only","scope":"isolated local fixture"}},"options":[{"optionId":"once169","name":"Allow once","kind":"allow_once"},{"optionId":"reject169","name":"Reject once","kind":"reject_once"},{"optionId":"always169","name":"Always (not supported)","kind":"allow_always"}]}}`
				select {
				case <-flow.permissionAnswered:
				case <-r.Context().Done():
					return
				}
			} else if round == 2 {
				flow.events <- cloudTurn169Update(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"第二回合已收到，等待明确取消。Second follow-up received; awaiting the explicit cancel."}}`)
			} else {
				v.mu.Lock()
				v.unexpected++
				v.mu.Unlock()
				w.WriteHeader(409)
				return
			}
		case "":
			if string(body["id"]) != `"actual169-permission-rpc"` || !strings.Contains(string(body["result"]), `"once169"`) {
				v.mu.Lock()
				v.unexpected++
				v.mu.Unlock()
				w.WriteHeader(400)
				return
			}
			v.mu.Lock()
			v.permission++
			wire := append(json.RawMessage{}, flow.promptID...)
			v.mu.Unlock()
			flow.events <- cloudTurn169Update(`{"sessionUpdate":"tool_call","toolCallId":"controlled-check169","title":"Controlled check / 受控检查","status":"in_progress"}`)
			flow.events <- cloudTurn169Update(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"观察已保存。Known secret private-tic"}}`)
			flow.events <- cloudTurn169Update(`{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"ket169-actual remains hidden. 独立验收仍然必要。"}}`)
			flow.events <- cloudTurn169Update(`{"sessionUpdate":"tool_call_update","toolCallId":"controlled-check169","status":"completed"}`)
			flow.events <- cloudTurnHTTPReply(wire, `{"stopReason":"end_turn","usage":{"inputTokens":5,"outputTokens":8,"totalTokens":13}}`)
			flow.answerOnce.Do(func() { close(flow.permissionAnswered) })
		case "session/cancel":
			v.mu.Lock()
			v.cancel++
			wire := append(json.RawMessage{}, flow.promptID...)
			v.mu.Unlock()
			flow.events <- cloudTurnHTTPReply(wire, `{"stopReason":"cancelled"}`)
		default:
			v.mu.Lock()
			v.unexpected++
			v.mu.Unlock()
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(202)
	}))
	t.Cleanup(v.server.Close)
	return v
}
func cloudTurn169Update(update string) string {
	return `{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"task-dispatch-cloud","update":` + update + `}}`
}
func (v *cloudTurn169Vendor) stats() map[string]any {
	v.mu.Lock()
	defer v.mu.Unlock()
	return map[string]any{"session_access": v.rest, "acp_attach": v.attach, "acp_initialize": v.initialize, "acp_load": v.load, "acp_prompt": v.prompt, "acp_permission_reply": v.permission, "acp_cancel": v.cancel, "unexpected_vendor_requests": v.unexpected}
}
func cloudTurn169Write(t *testing.T, file string, value any) {
	t.Helper()
	f, e := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal("actual169 original private record retained")
	}
	e = json.NewEncoder(f).Encode(value)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil || closeErr != nil {
		t.Fatal("actual169 private record persistence unavailable")
	}
}
func cloudTurn169Gate(t *testing.T, root, directory string) {
	t.Helper()
	var gate struct {
		Stage             string                          `json:"stage"`
		RuntimeAuthorized bool                            `json:"runtime_authorized"`
		Files             []struct{ Path, SHA256 string } `json:"files"`
	}
	data, e := os.ReadFile(filepath.Join(directory, "run-authorized.json"))
	if e != nil || json.Unmarshal(data, &gate) != nil || gate.Stage != "T-WORKBUDDY-CLOUD-TURNS-169" || !gate.RuntimeAuthorized || len(gate.Files) < 1 || len(gate.Files) > 100 {
		t.Fatal("actual169 final source/runtime authorization required")
	}
	for _, input := range gate.Files {
		absolute := filepath.Join(root, filepath.FromSlash(input.Path))
		if filepath.IsAbs(input.Path) || !strings.HasPrefix(absolute, root+string(filepath.Separator)) {
			t.Fatal("actual169 gate input escaped repository")
		}
		bytes, e := os.ReadFile(absolute)
		sum := sha256.Sum256(bytes)
		if e != nil || hex.EncodeToString(sum[:]) != input.SHA256 {
			t.Fatal("actual169 source changed before seed")
		}
	}
}
