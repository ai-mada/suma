package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const openCodeActualOriginal105 = "Immutable OpenCode source105\n"
const openCodeActualChanged105 = "Official OpenCode host MCP edit105\n"
const openCodeActualModel105 = "owned-coding105"

// This explicit gate runs the actual reviewed vendor binary, real bundled SDK,
// two real Workers and the ordinary Control/MCP/Git/candidate/test protocols.
// Only the owned loopback model's responses are deterministic. No paid model,
// user credentials, task acceptance or deployment is involved.
func TestOpenCodeActualInstalledSDKHostCandidateAndIndependentTest(t *testing.T) {
	if os.Getenv("SUMA_OPENCODE_ACTUAL") != "1" {
		t.Skip("explicit actual OpenCode execution opt-in is required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	executable := os.Getenv("SUMA_ACTUAL_OPENCODE_EXECUTABLE")
	pin := os.Getenv("SUMA_ACTUAL_OPENCODE_SHA256")
	if !filepath.IsAbs(executable) || !tc.ValidDigest(pin) {
		t.Fatal("reviewed absolute official executable and SHA256 are required")
	}
	st, statErr := os.Lstat(executable)
	resolved, resolveErr := filepath.EvalSymlinks(executable)
	if statErr != nil || resolveErr != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(executable)) || openCodeActualFileSHA105(t, executable) != pin {
		t.Fatal("official executable is linked, unavailable or differs from the reviewed pin")
	}
	stage := filepath.Join(root, ".cache", "e2e", "opencode-105")
	actualRoot := filepath.Join(stage, "actual")
	if err = os.MkdirAll(actualRoot, 0700); err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("SUMA_OPENCODE_ACTUAL_DIR")
	if dir == "" {
		dir = filepath.Join(actualRoot, "run-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	dir, err = filepath.Abs(dir)
	if err != nil || !under(actualRoot, dir) || strings.EqualFold(filepath.Clean(dir), filepath.Clean(actualRoot)) {
		t.Fatal("actual run must own a dedicated stage-cache child")
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("actual run directory must be new; never replay an original prompt", err)
	}
	hold := 0
	if raw := os.Getenv("SUMA_OPENCODE_ACTUAL_HOLD_SECONDS"); raw != "" {
		hold, err = strconv.Atoi(raw)
		if err != nil || hold < 0 || hold > 300 {
			t.Fatal("optional GUI hold must be 0..300 seconds")
		}
	}
	state := map[string]any{"status": "preparing", "failing_stage": "fixture_setup", "official_executable_sha256": pin, "official_version": tc.OpenCodeSDKVersion, "paid_provider_calls": 0, "human_acceptance": 0, "deployment": false}
	defer func() {
		if t.Failed() {
			state["status"] = "failed"
		}
		codexActualJSON(t, filepath.Join(dir, "receipt.local.json"), state)
	}()
	f := openCodeActualControl105(t, dir)
	f.e.AttemptAccounting = f.s
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(300+hold)*time.Second)
	defer cancel()
	developerRepo := filepath.Join(dir, "developer-repo")
	consumerRepo := filepath.Join(dir, "independent-repo")
	if err = os.Mkdir(developerRepo, 0700); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestCommand(t, developerRepo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, developerRepo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, developerRepo, "source.txt", openCodeActualOriginal105)
	workspaceGitTestCommand(t, developerRepo, "add", ".gitignore", "source.txt")
	workspaceGitTestCommand(t, developerRepo, "commit", "-m", "fixture: official OpenCode immutable base105")
	base := workspaceGitTestCommand(t, developerRepo, "rev-parse", "HEAD")
	workspaceGitTestCommand(t, dir, "clone", "--no-local", developerRepo, consumerRepo)
	if workspaceGitTestCommand(t, consumerRepo, "rev-parse", "HEAD") != base {
		t.Fatal("independent runner lacks exact administrator base")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node22 is required", err)
	}
	bundle := filepath.Join(root, "packages", "opencode-runtime", "dist", "suma-opencode.bundle.mjs")
	manifest := filepath.Join(root, "packages", "opencode-runtime", "dist", "suma-opencode.manifest.json")
	if _, err = os.Stat(manifest); err != nil {
		t.Fatal("build the real pinned SDK/assets before this gate", err)
	}
	state["sdk_manifest_sha256"] = openCodeActualFileSHA105(t, manifest)
	// Live inspection pauses are separate from the final result hold, so a
	// completed-result review never delays or repeats model work.
	owned := &openCodeOwnedChat105{dir: dir, holdFirst: hold > 0 && os.Getenv("SUMA_OPENCODE_ACTUAL_PAUSE_MODEL") == "1"}
	provider := httptest.NewServer(owned)
	defer provider.Close()
	defer func() { state["owned_model"] = owned.snapshot() }()
	checkCode := `const fs=require('node:fs');if(fs.readFileSync('source.txt','utf8')!=='Official OpenCode host MCP edit105\n')process.exit(31);console.log('exact source105 check passed');`
	developerConfig := runner.ExecConfig{AllowExec: true, Workspace: developerRepo, Repository: developerRepo, BaseCommit: base, Commands: map[string]runner.CommandSpec{
		"actual-opencode105":   {Adapter: "opencode", Protocol: tc.OpenCodeSDKProtocol, Executable: node, Args: []string{bundle}, Input: "opencode-sdk-jsonl", NativeWorkspace: &runner.NativeWorkspacePolicy{RuntimeManifest: manifest, Checks: []string{"development-check105"}}, OpenCode: &runner.OpenCodeSpec{Executable: executable, ProviderID: "owned105", Model: openCodeActualModel105, BaseURL: provider.URL + "/v1"}},
		"development-check105": {Executable: node, Args: []string{"-e", checkCode}},
	}}
	consumerConfig := runner.ExecConfig{AllowExec: true, Workspace: consumerRepo, Repository: consumerRepo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"independent-tests": {Executable: node, Args: []string{"-e", checkCode}, TestEvidence: true}}}
	state["failing_stage"] = "official_cli_sdk_discovery"
	inventory, err := runner.DescribeTools(ctx, developerConfig)
	state["inventory"] = inventory
	if err != nil {
		t.Fatal(err)
	}
	var observed tc.Observation
	for _, item := range inventory {
		if item.Profile.CommandID == "actual-opencode105" {
			observed = item
		}
	}
	if !observed.Available || !tc.SupportsOpenCodeExecutionProfile(observed.Profile) || observed.WorkspaceWriteProtocol != "workspace-write-v1" {
		t.Fatal("actual pinned binary/SDK capability probe failed; inspect receipt")
	}
	developerID, testerID := "opencode-actual-developer105", "opencode-independent-tester105"
	developerCredential := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": developerID, "profiles": []any{}, "deployment": false}), 201)
	testerCredential := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": testerID, "profiles": []any{}, "deployment": false}), 201)
	developerToken, testerToken := developerCredential["token"].(string), testerCredential["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: developerID, Name: "Actual official OpenCode105 / fixed owned model", Profile: observed.Profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, developerToken), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: developerID, OperationID: newID(), Items: inventory}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	workflow, _, _ := taskDispatchBuildFixture()
	workflow.Version = 1
	workflow.Name = "Actual OpenCode host tools and independent candidate test105"
	workflow.Nodes[0].TimeoutSeconds = 180
	workflow.Nodes[0].Config = map[string]any{"goal": "SOURCE_BASIS105: Change existing source.txt to exactly Official OpenCode host MCP edit105 followed by a newline, using authorized workspace read/write tools and the development-check105 tool. The host seals the candidate. Independent formal tests and human acceptance are separate."}
	workflow.Nodes[1].TimeoutSeconds = 60
	published, err := f.e.Publish(ctx, f.user.TenantID, f.project, workflow)
	if err != nil {
		t.Fatal(err)
	}
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "Official OpenCode toolchain105", "description": "SOURCE_BASIS105: preserve the administrator base, host CAS operations and separate independent testing.", "acceptance_criteria": []string{"Exact candidate SHA passes independent tests; human acceptance remains separate"}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "OpenCode实际编码与独立测试105", "description": "SOURCE_BASIS105: update source.txt through authorized host tools only.", "requirement_id": requirement["id"], "acceptance_criteria": []string{"Independent test receives exact machine-sealed candidate; do not auto-accept"}}), 201)
	planWire := f.call("POST", "/task-dispatch-plans", f.body(map[string]any{"task_id": task["id"], "expected_task_version": task["version"], "workflow_id": published.ID, "workflow_version": published.Version, "agent_node_id": "develop", "tool_profile_id": profile.ID, "expected_tool_version": profile.Version}), 201)
	var plan TaskDispatchPlan
	if json.Unmarshal([]byte(stringify(planWire)), &plan) != nil || plan.OpenCodeExecution == nil || *plan.OpenCodeExecution != *profile.Profile.OpenCodeExecution {
		t.Fatal("preview failed to freeze the actual approved OpenCode configuration")
	}
	f.call("POST", "/task-dispatch-plans/"+plan.ID+"/start", f.body(map[string]any{"expected_version": plan.Version}), 201)
	state["plan_id"], state["run_id"], state["task_id"] = plan.ID, plan.RunID, task["id"]
	server := &Server{Service: f.s, Engine: f.e, WebDir: filepath.Join(dir, "empty-web")}
	control := httptest.NewUnstartedServer(server.Handler())
	if hold > 0 {
		web, e := filepath.Abs(os.Getenv("SUMA_OPENCODE_ACTUAL_WEB_DIR"))
		if e != nil || !under(stage, web) {
			t.Fatal("GUI hold requires the owned stage web build")
		}
		if _, e = os.Stat(filepath.Join(web, "index.html")); e != nil {
			t.Fatal(e)
		}
		server.WebDir = web
		origin := "http://" + control.Listener.Addr().String()
		server.PublicURL, server.PublicURLSource = origin, "configured"
		server.AllowedOrigins = []string{origin}
		control.Config.Handler = server.Handler()
	}
	control.Start()
	defer control.Close()
	f.handler = control.Config.Handler
	state["origin"] = control.URL
	if hold > 0 {
		codexActualJSON(t, filepath.Join(dir, "gui.ready.json"), map[string]any{"origin": control.URL, "project_id": f.project, "task_id": task["id"], "run_id": plan.RunID})
	}
	developerJournal, err := runner.OpenJournal(filepath.Join(dir, "developer-journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = developerJournal.Close() })
	testerJournal, err := runner.OpenJournal(filepath.Join(dir, "independent-journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testerJournal.Close() })
	workerCtx, stopWorkers := context.WithCancel(ctx)
	workers := []struct {
		id, token string
		cfg       runner.ExecConfig
		journal   *runner.Journal
	}{{developerID, developerToken, developerConfig, developerJournal}, {testerID, testerToken, consumerConfig, testerJournal}}
	var completed []<-chan error
	joined := make([]bool, len(workers))
	for _, entry := range workers {
		worker := &runner.Worker{Client: &runner.Client{BaseURL: control.URL, ProjectID: f.project, Token: entry.token}, Journal: entry.journal, ID: entry.id, Exec: entry.cfg, PollInterval: 100 * time.Millisecond, HeartbeatInterval: time.Second}
		done := make(chan error, 1)
		completed = append(completed, done)
		go func() { done <- worker.Run(workerCtx) }()
	}
	defer func() {
		stopWorkers()
		for i, done := range completed {
			if joined[i] {
				continue
			}
			select {
			case e := <-done:
				if e != nil && !errors.Is(e, context.Canceled) && !errors.Is(e, context.DeadlineExceeded) {
					t.Error(e)
				}
			case <-time.After(12 * time.Second):
				t.Error("actual worker did not close")
			}
		}
	}()
	state["failing_stage"] = "actual_vendor_worker_model_host_loop"
	var current dag.Run
	for ctx.Err() == nil {
		for i, done := range completed {
			select {
			case e := <-done:
				joined[i] = true
				state["worker_error"] = fmt.Sprint(e)
				t.Fatalf("actual worker %s stopped before run/result ACK: %v", workers[i].id, e)
			default:
			}
		}
		current, err = f.e.GetRun(ctx, f.user.TenantID, f.project, plan.RunID)
		if err != nil {
			t.Fatal(err)
		}
		state["run"] = current
		if current.State == "failed" || current.State == "cancelled" || current.State == "reconciling" {
			t.Fatalf("actual tool chain failed: state=%s nodes=%+v", current.State, current.Nodes)
		}
		if current.State == "succeeded" && len(developerJournal.Pending()) == 0 && len(testerJournal.Pending()) == 0 && len(developerJournal.PendingProgress()) == 0 && len(testerJournal.PendingProgress()) == 0 {
			break
		}
		select {
		case <-ctx.Done():
		case <-time.After(50 * time.Millisecond):
		}
	}
	if ctx.Err() != nil {
		t.Fatal("actual tool chain exceeded its bound", ctx.Err())
	}
	state["failing_stage"] = "independent_candidate_and_ledger_proof"
	var developed, checked dag.NodeRun
	for _, n := range current.Nodes {
		if n.ID == "develop" {
			developed = n
		}
		if n.ID == "test" {
			checked = n
		}
	}
	if developed.WorkerID != developerID || checked.WorkerID != testerID || developed.Attempt != 1 || checked.Attempt != 1 || developed.ToolEvidence == nil || developed.ToolEvidence.Terminal != "completed" || developed.ToolEvidence.OpenCodeExecution == nil || !developed.ToolEvidence.OpenCodeExecution.ServerStopped || checked.State != "succeeded" {
		t.Fatal("actual separate workers/original execution evidence missing")
	}
	var native runner.NativeWorkspaceEvidence
	if json.Unmarshal([]byte(stringify(developed.WorkspaceEvidence["native_workspace"])), &native) != nil || native.Seal == nil || native.Seal.State != "sealed" || native.Generation != 1 || len(native.Files) != 1 || len(native.Checks) != 1 || native.Checks[0].CheckID != "development-check105" || native.Checks[0].ExitCode != 0 {
		t.Fatal("actual tools did not produce one host-ledger edit, current fixed check and machine seal")
	}
	var binding cc.Binding
	if json.Unmarshal([]byte(stringify(developed.WorkspaceEvidence["candidate_bundle"])), &binding) != nil || !binding.ValidSealed() || binding.HeadCommit != native.Seal.Commit {
		t.Fatal("machine candidate was not uploaded and sealed through Control")
	}
	var imported cc.ImportReceipt
	if json.Unmarshal([]byte(stringify(checked.WorkspaceEvidence["candidate_import"])), &imported) != nil || !imported.Imported || imported.TargetWorkerID != testerID || imported.Binding != binding || checked.WorkspaceEvidence["head_commit"] != binding.HeadCommit || checked.WorkspaceEvidence["test_command"] != true {
		t.Fatal("independent worker did not download/import/test the exact sealed candidate")
	}
	for _, repo := range []string{developerRepo, consumerRepo} {
		original, e := os.ReadFile(filepath.Join(repo, "source.txt"))
		if e != nil || string(original) != openCodeActualOriginal105 || workspaceGitTestCommand(t, repo, "rev-parse", "HEAD") != base {
			t.Fatal("actual execution changed an administrator source/base")
		}
	}
	if err = owned.verify(); err != nil {
		t.Fatal(err)
	}
	usage, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(usage.Items) != 1 {
		t.Fatal("original observed SDK attempt ledger missing", err)
	}
	accounted := usage.Items[0]
	if accounted.RunID != plan.RunID || accounted.ProfileSHA256 != profile.Profile.ProfileSHA256 || accounted.UsageState == tc.UsageComplete || accounted.Rate != nil || accounted.EstimatedCostMicros != nil || accounted.ReservedTokens != 0 || accounted.ModelExecution != nil || accounted.CodexExecution != nil {
		t.Fatal("actual SDK observation became native priced/enforced accounting")
	}
	state["usage_attempt"], state["candidate"], state["native_workspace"] = accounted, binding, native
	bound := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	if bound["status"] == "accepted" {
		t.Fatal("execution/test automatically accepted the task")
	}
	state["status"], state["independent_test"], state["task_status"] = "execution_and_independent_test_confirmed", true, bound["status"]
	if hold > 0 {
		state["failing_stage"] = "bounded_gui_result_hold"
		codexActualJSON(t, filepath.Join(dir, "result.ready.json"), map[string]any{"origin": control.URL, "project_id": f.project, "task_id": task["id"], "run_id": plan.RunID})
		until := time.Now().Add(time.Duration(hold) * time.Second)
		for time.Now().Before(until) {
			if _, e := os.Stat(filepath.Join(dir, "stop.request")); e == nil {
				break
			}
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(100 * time.Millisecond):
			}
		}
		if err = owned.verify(); err != nil {
			t.Fatal("GUI observation created another model execution", err)
		}
	}
	state["status"] = "closed"
	delete(state, "failing_stage")
	t.Logf("actual_opencode105=%s", stringify(map[string]any{"run_id": plan.RunID, "candidate_commit": binding.HeadCommit, "developer_worker": developerID, "independent_worker": testerID, "owned_model_calls": 4, "usage_state": accounted.UsageState, "human_acceptance": false, "receipt_directory": dir}))
}

func openCodeActualControl105(t *testing.T, dir string) *fixture {
	t.Helper()
	for _, child := range []string{"data", "empty-web"} {
		if err := os.Mkdir(filepath.Join(dir, child), 0700); err != nil {
			t.Fatal(err)
		}
	}
	db, err := store.Open("sqlite", filepath.Join(dir, "suma.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := &Service{DB: db, Dialect: "sqlite", DataDir: filepath.Join(dir, "data")}
	if err = s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	e, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, s: s, e: e, handler: (&Server{Service: s, Engine: e, WebDir: filepath.Join(dir, "empty-web")}).Handler()}
	setup := f.call("POST", "/auth/setup", map[string]any{"name": "Owned OpenCode fixture owner", "email": "owner@example.test", "password": "test-password-strong-42", "organization": "Owned OpenCode execution fixture", "project": "OpenCode actual toolchain105"}, 200)
	if json.Unmarshal([]byte(stringify(setup["user"])), &f.user) != nil {
		t.Fatal("invalid fixture setup")
	}
	f.actor, err = s.AuthenticateActor(context.Background(), f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.project = setup["projects"].([]any)[0].(map[string]any)["id"].(string)
	return f
}

func openCodeActualFileSHA105(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	h := sha256.New()
	if _, err = io.Copy(h, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type openCodeOwnedChat105 struct {
	mu                      sync.Mutex
	dir                     string
	holdFirst               bool
	calls                   int
	tools                   []string
	requestSHA, responseSHA []string
	failure                 string
}

func (s *openCodeOwnedChat105) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		http.Error(w, "bounded owned request required", 400)
		return
	}
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil || r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || body["model"] != openCodeActualModel105 || body["stream"] != true || r.Header.Get("Authorization") != "" {
		s.reject(w, "unexpected model request or external authentication")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != "" {
		http.Error(w, "owned model already failed", 409)
		return
	}
	if s.calls >= 4 {
		s.failure = "model mutation retried or extra request"
		http.Error(w, s.failure, 409)
		return
	}
	phase := s.calls
	s.calls++
	s.requestSHA = append(s.requestSHA, digest(string(raw)))
	var name string
	var arguments map[string]any
	switch phase {
	case 0:
		if !strings.Contains(string(raw), "SOURCE_BASIS105") {
			s.failLocked(w, "fixed task/requirement context missing")
			return
		}
		tools, ok := body["tools"].([]any)
		if !ok {
			s.failLocked(w, "actual tool catalog is missing")
			return
		}
		known := map[string]bool{}
		for _, item := range tools {
			tool, ok := item.(map[string]any)
			if !ok {
				s.failLocked(w, "malformed declared tool")
				return
			}
			fn, ok := tool["function"].(map[string]any)
			if !ok || tool["type"] != "function" {
				s.failLocked(w, "malformed declared function")
				return
			}
			toolName, _ := fn["name"].(string)
			if len(toolName) <= 128 {
				s.tools = append(s.tools, toolName)
			}
			if !map[string]bool{"suma_workspace_list": true, "suma_workspace_read": true, "suma_workspace_write": true, "suma_workspace_edit": true, "suma_workspace_remove": true, "suma_workspace_check": true}[toolName] || known[toolName] {
				s.failLocked(w, "unapproved actual built-in or duplicate tool")
				return
			}
			known[toolName] = true
		}
		if len(known) != 6 {
			s.failLocked(w, "actual tool catalog is not exactly six host tools")
			return
		}
		name, arguments = "suma_workspace_read", map[string]any{"path": "source.txt"}
		if s.holdFirst {
			openCodeActualJSONNoTest105(filepath.Join(s.dir, "model.ready.json"), map[string]any{"model_request": 1, "status": "waiting_for_optional_gui_observation"})
			until := time.Now().Add(45 * time.Second)
			for time.Now().Before(until) {
				if _, e := os.Stat(filepath.Join(s.dir, "continue-model.request")); e == nil {
					break
				}
				select {
				case <-r.Context().Done():
					s.failure = "first model request cancelled during GUI observation"
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		}
	case 1:
		result, ok := openCodeActualToolResult105(body, "call_read105")
		if !ok || result["content"] != openCodeActualOriginal105 || result["sha256"] != digest(openCodeActualOriginal105) {
			s.failLocked(w, "official read tool did not return original host content/hash")
			return
		}
		name, arguments = "suma_workspace_write", map[string]any{"path": "source.txt", "content": openCodeActualChanged105, "expected_sha256": result["sha256"]}
	case 2:
		result, ok := openCodeActualToolResult105(body, "call_write105")
		if !ok || result["state"] != "completed" || result["sha256"] != digest(openCodeActualChanged105) || result["generation"] != float64(1) {
			s.failLocked(w, "official write did not return original committed host operation receipt")
			return
		}
		name, arguments = "suma_workspace_check", map[string]any{"check_id": "development-check105"}
	case 3:
		result, ok := openCodeActualToolResult105(body, "call_check105")
		if !ok || result["check_id"] != "development-check105" || result["exit_code"] != float64(0) || result["generation"] != float64(1) {
			s.failLocked(w, "official check did not run against current changed source")
			return
		}
	}
	response := openCodeActualChatSSE105(phase, name, arguments)
	s.responseSHA = append(s.responseSHA, digest(string(response)))
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	if _, err = w.Write(response); err != nil {
		s.failure = "owned response delivery failed"
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *openCodeOwnedChat105) reject(w http.ResponseWriter, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failLocked(w, reason)
}
func (s *openCodeOwnedChat105) failLocked(w http.ResponseWriter, reason string) {
	s.failure = reason
	http.Error(w, reason, 400)
}
func (s *openCodeOwnedChat105) snapshot() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"calls": s.calls, "tools": append([]string{}, s.tools...), "request_sha256": append([]string{}, s.requestSHA...), "response_sha256": append([]string{}, s.responseSHA...), "failure": s.failure}
}
func (s *openCodeOwnedChat105) verify() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failure != "" || s.calls != 4 || len(s.requestSHA) != 4 || len(s.responseSHA) != 4 || len(s.tools) != 6 {
		return fmt.Errorf("actual owned tool loop incomplete: calls=%d failure=%s", s.calls, s.failure)
	}
	return nil
}

func openCodeActualToolResult105(body map[string]any, id string) (map[string]any, bool) {
	messages, _ := body["messages"].([]any)
	for _, item := range messages {
		m, ok := item.(map[string]any)
		if !ok || m["role"] != "tool" || m["tool_call_id"] != id {
			continue
		}
		text, ok := m["content"].(string)
		if !ok {
			return nil, false
		}
		var value map[string]any
		if json.Unmarshal([]byte(text), &value) != nil {
			return nil, false
		}
		return value, true
	}
	return nil, false
}

func openCodeActualChatSSE105(phase int, name string, args map[string]any) []byte {
	base := func(choices []any) map[string]any {
		return map[string]any{"id": fmt.Sprintf("owned_chat105_%d", phase), "object": "chat.completion.chunk", "created": int64(1793145600), "model": openCodeActualModel105, "choices": choices}
	}
	delta := map[string]any{"role": "assistant"}
	finish := "stop"
	if name != "" {
		raw, _ := json.Marshal(args)
		call := []string{"call_read105", "call_write105", "call_check105"}[phase]
		delta["tool_calls"] = []any{map[string]any{"index": 0, "id": call, "type": "function", "function": map[string]any{"name": name, "arguments": string(raw)}}}
		finish = "tool_calls"
	} else {
		delta["content"] = "The authorized host edit and development check are complete. Independent testing and human review remain separate."
	}
	chunks := []map[string]any{base([]any{map[string]any{"index": 0, "delta": delta, "finish_reason": nil}}), base([]any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}})}
	usage := base([]any{})
	usage["usage"] = map[string]any{"prompt_tokens": 200 + phase, "completion_tokens": 40 + phase, "total_tokens": 240 + 2*phase}
	chunks = append(chunks, usage)
	var output strings.Builder
	for _, chunk := range chunks {
		raw, _ := json.Marshal(chunk)
		fmt.Fprintf(&output, "data: %s\n\n", raw)
	}
	output.WriteString("data: [DONE]\n\n")
	return []byte(output.String())
}

func openCodeActualJSONNoTest105(path string, value any) {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, raw, 0600)
	}
}
