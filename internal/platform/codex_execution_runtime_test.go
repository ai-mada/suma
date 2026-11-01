package platform

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const codexActualOriginal = "Immutable Codex source fixture\n"
const codexActualChanged = "Codex app-server owned edit 041\n"

// This gate never runs under an ordinary test selection. A real installed CLI
// talks only to a newly owned loopback provider, using an isolated generated
// home. Preparing/compiling this test is not evidence of actual CLI support.
func TestCodexExecutionActualInstalledCLIAndObservedUsageLedger(t *testing.T) {
	if os.Getenv("SUMA_CODEX_ACTUAL") != "1" || os.Getenv("SUMA_ACTUAL_CODEX_EXECUTABLE") == "" {
		t.Skip("explicit installed-Codex execution and executable are required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	executable := os.Getenv("SUMA_ACTUAL_CODEX_EXECUTABLE")
	var pin struct {
		Executable string `json:"installed_executable"`
		SHA256     string `json:"installed_executable_sha256"`
	}
	rawPin, err := os.ReadFile(filepath.Join(root, ".cache", "e2e", "expert-model-stage-040", "codex-appserver-research.local.json"))
	if err != nil || json.Unmarshal(rawPin, &pin) != nil || !strings.EqualFold(filepath.Clean(executable), filepath.Clean(pin.Executable)) {
		t.Fatal("actual executable must match the reviewed local research receipt")
	}
	actualBytes, err := os.ReadFile(executable)
	if err != nil || codexActualHash(actualBytes) != pin.SHA256 {
		t.Fatal("reviewed installed executable changed; no process was started")
	}
	stage := filepath.Join(root, ".cache", "e2e", "codex-execution-stage-041")
	actualRoot := filepath.Join(stage, "actual")
	if err = os.MkdirAll(actualRoot, 0700); err != nil {
		t.Fatal(err)
	}
	dir := os.Getenv("SUMA_CODEX_ACTUAL_DIR")
	if dir == "" {
		dir = filepath.Join(actualRoot, "run-"+time.Now().UTC().Format("20060102T150405.000000000Z"))
	}
	dir, err = filepath.Abs(dir)
	if err != nil || !under(actualRoot, dir) || strings.EqualFold(dir, actualRoot) {
		t.Fatal("actual fixture requires its own child of the stage actual directory")
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		t.Fatal("actual fixture directory must be new; an original operation is not rerun", err)
	}
	state := map[string]any{"status": "preparing", "failing_stage": "fixture_setup", "installed_executable_sha256": pin.SHA256, "vendor_calls": 0, "formal_tests": 0, "human_acceptance": 0}
	defer func() {
		if t.Failed() {
			state["status"] = "failed"
		}
		codexActualJSON(t, filepath.Join(dir, "receipt.local.json"), state)
	}()
	f := codexActualFixture(t, dir)
	f.e.AttemptAccounting = f.s
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	repo := filepath.Join(dir, "repo")
	if err = os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, "source.txt", codexActualOriginal)
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "source.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: owned Codex source base")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	owned := &codexOwnedResponses{t: t, worktrees: filepath.Join(repo, ".cache", "runner", "worktrees")}
	defer func() { state["owned_responses"] = owned.receipt() }()
	provider := httptest.NewServer(owned)
	defer provider.Close()
	spec := runner.CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: executable, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &runner.CodexAppServerSpec{ProviderID: "owned041", BaseURL: provider.URL + "/v1", WindowsSandbox: "unelevated"}}
	cfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"codex-actual-041": spec}}
	state["failing_stage"] = "tool_discovery"
	inventory, err := runner.DescribeTools(ctx, cfg)
	state["inventory"], state["owned_responses"] = inventory, owned.receipt()
	if err != nil {
		state["discovery_error"] = err.Error()
	}
	if err != nil || len(inventory) != 1 || !inventory[0].Available || inventory[0].CodexCatalog == nil {
		t.Fatal("actual isolated app-server discovery did not produce a current catalog", err)
	}
	catalog := *inventory[0].CodexCatalog
	choice := cc.Selection{CatalogSHA256: catalog.CatalogSHA256, TokenNoticeLimit: usageInt(1)}
	for _, model := range catalog.Models {
		if len(model.ReasoningEfforts) > 0 {
			choice.Model, choice.ReasoningEffort = model.Model, model.DefaultReasoningEffort
			break
		}
	}
	if choice.Model == "" {
		t.Fatal("actual CLI did not advertise a model with an explicit reasoning effort")
	}
	owned.model = choice.Model
	state["failing_stage"] = "tool_approval"
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "codex-actual-worker-041", "profiles": []any{}, "deployment": false}), 201)
	token := worker["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "codex-actual-worker-041", Name: "Owned actual Codex app-server", Profile: inventory[0].Profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: inventory})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	workflow, _, _ := taskDispatchBuildFixture()
	workflow.Version = 1
	workflow.Nodes[0].Config["goal"] = "Use the permitted file-edit tool to update existing source.txt to the requested text, then report completion. Do not run formal tests, commit a candidate or accept a task."
	published, err := f.e.Publish(ctx, f.user.TenantID, f.project, workflow)
	if err != nil {
		t.Fatal(err)
	}
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Codex实际独立工作树开发", "description": "Change source.txt to exactly Codex app-server owned edit 041 followed by a newline. This fixture does not grant formal acceptance.", "acceptance_criteria": []string{"Independent tests and human acceptance remain separate"}}), 201)
	state["failing_stage"] = "task_preview"
	planWire := f.call("POST", "/task-dispatch-plans", f.body(map[string]any{"task_id": task["id"], "expected_task_version": task["version"], "workflow_id": published.ID, "workflow_version": published.Version, "agent_node_id": "develop", "tool_profile_id": profile.ID, "expected_tool_version": profile.Version, "codex_execution_selection": choice}), 201)
	var plan TaskDispatchPlan
	if json.Unmarshal([]byte(stringify(planWire)), &plan) != nil || plan.CodexExecution == nil || plan.ModelExecution != nil {
		t.Fatal("actual task preview did not freeze independent Codex configuration")
	}
	state["failing_stage"] = "task_start"
	f.call("POST", "/task-dispatch-plans/"+plan.ID+"/start", f.body(map[string]any{"expected_version": plan.Version}), 201)
	hold := os.Getenv("SUMA_CODEX_ACTUAL_HOLD") == "1"
	server := &Server{Service: f.s, Engine: f.e, WebDir: filepath.Join(dir, "empty-web")}
	control := httptest.NewUnstartedServer(server.Handler())
	if hold {
		web, e := filepath.Abs(os.Getenv("SUMA_CODEX_ACTUAL_WEB_DIR"))
		if e != nil || !under(stage, web) {
			t.Fatal("actual UI hold requires the owned stage Web build")
		}
		if _, e = os.Stat(filepath.Join(web, "index.html")); e != nil {
			t.Fatal("owned stage Web build is missing")
		}
		listener, e := net.Listen("tcp4", "127.0.0.1:59220")
		if e != nil {
			t.Fatal(e)
		}
		_ = control.Listener.Close()
		control.Listener = listener
		server.WebDir, server.PublicURL, server.PublicURLSource = web, "http://127.0.0.1:59220", "configured"
		server.AllowedOrigins = []string{"http://codexmodel041.localhost:59220", "http://127.0.0.1:59220"}
		control.Config.Handler = server.Handler()
	}
	control.Start()
	defer control.Close()
	f.handler = control.Config.Handler
	client := &runner.Client{BaseURL: control.URL, Token: token, ProjectID: f.project}
	state["failing_stage"] = "http_claim"
	assignment, err := client.Claim(ctx, profile.WorkerID, []string{"command:" + profile.Profile.CommandID})
	if err != nil || assignment == nil || assignment.RunID != plan.RunID {
		t.Fatal("actual HTTP claim did not return the reviewed Codex task", err)
	}
	cfg.ToolInventory = inventory
	beatCtx, stopBeat := context.WithCancel(ctx)
	beatDone := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-beatCtx.Done():
				beatDone <- nil
				return
			case <-ticker.C:
				stopping, e := client.Heartbeat(beatCtx, *assignment)
				if e != nil || stopping {
					cancel()
					beatDone <- fmt.Errorf("actual original assignment heartbeat stopped: %v", e)
					return
				}
			}
		}
	}()
	state["failing_stage"] = "cli_execution"
	// Real progress crosses the same authenticated original-fence HTTP endpoint
	// as the worker, independently of the final result callback.
	progressEvents := 0
	cfg.ToolProgress = func(event tc.Event) error {
		if err := client.Progress(ctx, runner.Progress{Assignment: *assignment, OperationID: newID(), ToolEvent: &event}); err != nil {
			return err
		}
		progressEvents++
		return nil
	}
	result := runner.Execute(ctx, *assignment, cfg)
	state["persisted_progress_events"] = progressEvents
	stopBeat()
	if err = <-beatDone; err != nil {
		t.Fatal(err)
	}
	state["tool_evidence"], state["workspace_evidence"] = result.ToolEvidence, result.WorkspaceEvidence
	state["exit_code"], state["outcome_unknown"], state["error"] = result.ExitCode, result.OutcomeUnknown, result.Error
	state["result_output"] = result.Output
	state["owned_responses"] = owned.receipt()
	// A failed/unknown first execution still drains its original facts. It is
	// never silently omitted from the ledger or replaced with another attempt.
	result.OperationID = newID()
	state["result_operation_id"] = result.OperationID
	callbackErr := client.Complete(context.Background(), result)
	state["result_acknowledged"] = callbackErr == nil
	if callbackErr != nil {
		state["result_ack_error"] = callbackErr.Error()
		t.Fatal("actual original Codex result callback was rejected", callbackErr)
	}
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.CodexExecution == nil || !cc.EvidenceMatchesBinding(*plan.CodexExecution, *result.ToolEvidence.CodexExecution) || result.ToolEvidence.UsageState != tc.UsagePartial || !result.ToolEvidence.CodexExecution.TokenNoticeExceeded {
		t.Fatal("actual app-server execution/ACK/partial-usage/notice evidence was not confirmed; inspect the owned receipt")
	}
	if err = owned.verify(); err != nil {
		t.Fatal(err)
	}
	state["failing_stage"] = "usage_ledger"
	attempts, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 10, "")
	if err != nil || len(attempts.Items) != 1 {
		t.Fatal("actual Codex observation ledger is absent", err)
	}
	usage := attempts.Items[0]
	state["usage_attempt"], state["binding"], state["catalog"] = usage, plan.CodexExecution, catalog
	if usage.CodexExecution == nil || usage.CodexExecution.BindingSHA256 != plan.CodexExecution.BindingSHA256 || usage.Rate != nil || usage.Model != "" || usage.EndpointOrigin != "" || usage.State != "partial" || usage.ReportedTokens == nil || *usage.ReportedTokens <= 1 || usage.EstimatedCostMicros != nil || usage.ReservedTokens != 0 || usage.UsageState != tc.UsagePartial {
		t.Fatal("actual Codex observations became native identity, complete coverage or a priced budget")
	}
	if workspaceGitTestCommand(t, repo, "rev-parse", "HEAD") != base {
		t.Fatal("actual CLI changed the administrator base")
	}
	original, err := os.ReadFile(filepath.Join(repo, "source.txt"))
	if err != nil || string(original) != codexActualOriginal {
		t.Fatal("actual CLI changed the administrator source")
	}
	state["status"] = "execution_confirmed"
	if hold {
		state["failing_stage"] = "gui_hold"
		previewTask := f.call("POST", "/tasks", f.body(map[string]any{"title": "Codex模型GUI仅预览", "description": "Review a fresh model choice without starting another turn", "acceptance_criteria": []string{"No additional model, formal tests or task acceptance"}}), 201)
		codexActualJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"base_url": control.URL, "browser_origin": "http://codexmodel041.localhost:59220", "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "task_id": previewTask["id"], "workflow_id": published.ID, "workflow_version": published.Version, "tool_profile_id": profile.ID, "tool_profile_version": profile.Version, "plan_id": plan.ID, "run_id": assignment.RunID})
		codexActualJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"port": 59220, "project_id": f.project, "task_id": previewTask["id"], "tool_profile_id": profile.ID, "owned_model_calls": 2, "cost_state": "unpriced", "usage_state": "partial"})
		t.Log("owned Codex UI hold ready on port 59220; private login receipt retained")
		until, refresh := time.Now().Add(15*time.Minute), time.Now().Add(30*time.Second)
		for time.Now().Before(until) {
			if _, err = os.Stat(filepath.Join(dir, "stop.request")); err == nil {
				break
			}
			if time.Now().After(refresh) {
				fresh, e := runner.DescribeTools(context.Background(), cfg)
				if e != nil {
					t.Fatal(e)
				}
				scope, e := runner.InventoryScopeSHA256(client.BaseURL, client.ProjectID, profile.WorkerID)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = client.ToolInventory(context.Background(), runner.InventoryReport{WorkerID: profile.WorkerID, OperationID: newID(), Items: fresh, ScopeSHA256: scope}); e != nil {
					t.Fatal(e)
				}
				refresh = time.Now().Add(30 * time.Second)
			}
			time.Sleep(100 * time.Millisecond)
		}
		guiTask, e := f.s.Get(context.Background(), f.actor, f.project, "tasks", previewTask["id"].(string))
		if e != nil || guiTask.RunID != "" || expertCount(t, f, "dag_runs") != 1 || owned.verify() != nil {
			t.Fatal("GUI preview started another turn or changed actual execution count", e)
		}
	}
	state["status"] = "closed"
	delete(state, "failing_stage")
	t.Log("actual installed Codex -> owned tool call -> isolated worktree edit -> exact original result -> partial unpriced counters; no vendor or formal acceptance")
}

func codexActualFixture(t *testing.T, dir string) *fixture {
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
	setup := f.call("POST", "/auth/setup", map[string]any{"name": "Owned Codex fixture owner", "email": "owner@example.test", "password": "test-password-strong-42", "organization": "Owned Codex protocol fixture", "project": "Codex isolated execution"}, 200)
	if json.Unmarshal([]byte(stringify(setup["user"])), &f.user) != nil {
		t.Fatal("fixture account setup response is invalid")
	}
	f.actor, err = s.AuthenticateActor(context.Background(), f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	f.project = setup["projects"].([]any)[0].(map[string]any)["id"].(string)
	return f
}

func codexActualHash(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func codexActualJSON(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil || os.WriteFile(path, raw, 0600) != nil {
		t.Error("could not retain the owned local fixture receipt")
	}
}

type codexOwnedResponses struct {
	mu                                               sync.Mutex
	t                                                *testing.T
	model, worktrees, tool                           string
	calls                                            int
	requests, responses                              []string
	toolOutput, fileChanged                          bool
	toolDeclaration, emittedTool, originalToolOutput any
	matchedFiles                                     []string
}

func (s *codexOwnedResponses) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.Method != http.MethodPost || r.URL.Path != "/v1/responses" {
		s.t.Errorf("unexpected owned provider route %s %s", r.Method, r.URL.Path)
		http.Error(w, "owned provider route unavailable", 404)
		return
	}
	if r.Header.Get("Authorization") != "" {
		s.t.Error("owned provider received an unexpected credential")
		http.Error(w, "owned provider does not accept credentials", 400)
		return
	}
	var reader io.Reader = r.Body
	if r.Header.Get("Content-Encoding") == "gzip" {
		z, err := gzip.NewReader(r.Body)
		if err != nil {
			http.Error(w, "invalid owned gzip request", 400)
			return
		}
		defer z.Close()
		reader = z
	}
	raw, err := io.ReadAll(io.LimitReader(reader, 4<<20))
	var body map[string]any
	if err != nil || len(raw) == 4<<20 || json.Unmarshal(raw, &body) != nil || body["model"] != s.model || body["stream"] != true {
		s.t.Error("owned Responses request differs from the selected actual model/stream")
		http.Error(w, "invalid owned Responses request", 400)
		return
	}
	s.calls++
	s.requests = append(s.requests, codexActualHash(raw))
	var item map[string]any
	if s.calls == 1 {
		item, s.tool = codexActualTool(body)
		s.toolDeclaration, s.emittedTool = body["tools"], item
		if item == nil {
			s.t.Error("actual CLI did not declare a supported apply_patch/shell edit tool")
			http.Error(w, "no matching actual declared edit tool", 400)
			return
		}
	} else if s.calls == 2 {
		s.toolOutput = codexActualToolOutput(body["input"])
		s.originalToolOutput = codexActualOriginalToolOutput(body["input"])
		matches, _ := filepath.Glob(filepath.Join(s.worktrees, "*", "src", "source.txt"))
		for _, file := range matches {
			relative, _ := filepath.Rel(s.worktrees, file)
			s.matchedFiles = append(s.matchedFiles, relative)
		}
		if len(matches) == 1 {
			content, e := os.ReadFile(matches[0])
			s.fileChanged = e == nil && string(content) == codexActualChanged
		}
		if !s.toolOutput || !s.fileChanged {
			s.t.Error("actual tool did not return the original call output and modify its isolated source")
			http.Error(w, "original tool effect unavailable", 400)
			return
		}
		item = map[string]any{"type": "message", "id": "msg_owned_041", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "Owned isolated source edit completed; formal tests and acceptance remain separate.", "annotations": []any{}}}}
	} else {
		s.t.Error("unexpected additional owned model request; no replay is permitted")
		http.Error(w, "owned Responses request count exceeded", 400)
		return
	}
	response := codexActualSSE(s.calls, s.model, item)
	s.responses = append(s.responses, codexActualHash(response))
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(response)
	if flush, ok := w.(http.Flusher); ok {
		flush.Flush()
	}
}

func (s *codexOwnedResponses) receipt() any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"calls": s.calls, "tool": s.tool, "tool_output": s.toolOutput, "isolated_source_changed": s.fileChanged, "request_sha256": append([]string{}, s.requests...), "response_sha256": append([]string{}, s.responses...), "declared_tools": s.toolDeclaration, "emitted_tool": s.emittedTool, "original_tool_output": s.originalToolOutput, "matched_source_files": s.matchedFiles}
}
func (s *codexOwnedResponses) verify() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.calls != 2 || !s.fileChanged || !s.toolOutput || len(s.responses) != 2 {
		return fmt.Errorf("actual owned tool/final response sequence was not confirmed")
	}
	return nil
}

func codexActualTool(body map[string]any) (map[string]any, string) {
	patch := "*** Begin Patch\n*** Update File: source.txt\n@@\n-Immutable Codex source fixture\n+Codex app-server owned edit 041\n*** End Patch"
	tools, _ := body["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		name, _ := tool["name"].(string)
		if name != "apply_patch" {
			continue
		}
		if tool["type"] == "custom" {
			return map[string]any{"type": "custom_tool_call", "id": "ctc_owned_041", "call_id": "call_owned_041", "name": name, "input": patch}, name
		}
		parameters, _ := tool["parameters"].(map[string]any)
		properties, _ := parameters["properties"].(map[string]any)
		for _, key := range []string{"input", "patch"} {
			if _, ok := properties[key]; ok && tool["type"] == "function" {
				return map[string]any{"type": "function_call", "id": "fc_owned_041", "call_id": "call_owned_041", "name": name, "arguments": stringify(map[string]any{key: patch})}, name
			}
		}
	}
	command := "[System.IO.File]::WriteAllText('source.txt', \"Codex app-server owned edit 041`n\", [System.Text.UTF8Encoding]::new($false))"
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		name, _ := tool["name"].(string)
		key := ""
		switch name {
		case "exec_command":
			key = "cmd"
		case "shell_command":
			key = "command"
		}
		if key != "" && tool["type"] == "function" {
			return map[string]any{"type": "function_call", "id": "fc_owned_041", "call_id": "call_owned_041", "name": name, "arguments": stringify(map[string]any{key: command})}, name
		}
	}
	return nil, ""
}

func codexActualOriginalToolOutput(value any) any {
	switch v := value.(type) {
	case []any:
		for _, child := range v {
			if found := codexActualOriginalToolOutput(child); found != nil {
				return found
			}
		}
	case map[string]any:
		kind, _ := v["type"].(string)
		if v["call_id"] == "call_owned_041" && (kind == "function_call_output" || kind == "custom_tool_call_output") {
			raw, _ := json.Marshal(v)
			if len(raw) > 65536 {
				return map[string]any{"truncated": true, "bytes": len(raw), "sha256": codexActualHash(raw)}
			}
			return v
		}
	}
	return nil
}

func codexActualToolOutput(value any) bool {
	switch v := value.(type) {
	case []any:
		for _, child := range v {
			if codexActualToolOutput(child) {
				return true
			}
		}
	case map[string]any:
		kind, _ := v["type"].(string)
		if v["call_id"] == "call_owned_041" && (kind == "function_call_output" || kind == "custom_tool_call_output") {
			return true
		}
	}
	return false
}

func codexActualSSE(call int, model string, item map[string]any) []byte {
	var out bytes.Buffer
	sequence := 0
	event := func(kind string, fields map[string]any) {
		fields["type"], fields["sequence_number"] = kind, sequence
		sequence++
		fmt.Fprintf(&out, "event: %s\ndata: %s\n\n", kind, stringify(fields))
	}
	response := map[string]any{"id": fmt.Sprintf("resp_owned_041_%d", call), "object": "response", "created_at": 1, "status": "in_progress", "model": model, "output": []any{}}
	event("response.created", map[string]any{"response": response})
	initial := map[string]any{}
	for k, v := range item {
		initial[k] = v
	}
	initial["status"] = "in_progress"
	kind, _ := item["type"].(string)
	if kind == "message" {
		initial["content"] = []any{}
	} else if kind == "custom_tool_call" {
		initial["input"] = ""
	} else {
		initial["arguments"] = ""
	}
	event("response.output_item.added", map[string]any{"output_index": 0, "item": initial})
	if kind == "message" {
		part := item["content"].([]any)[0].(map[string]any)
		event("response.content_part.added", map[string]any{"item_id": item["id"], "output_index": 0, "content_index": 0, "part": map[string]any{"type": "output_text", "text": "", "annotations": []any{}}})
		event("response.output_text.delta", map[string]any{"item_id": item["id"], "output_index": 0, "content_index": 0, "delta": part["text"]})
		event("response.output_text.done", map[string]any{"item_id": item["id"], "output_index": 0, "content_index": 0, "text": part["text"]})
		event("response.content_part.done", map[string]any{"item_id": item["id"], "output_index": 0, "content_index": 0, "part": part})
	} else {
		name, key := "response.function_call_arguments", "arguments"
		if kind == "custom_tool_call" {
			name, key = "response.custom_tool_call_input", "input"
		}
		event(name+".delta", map[string]any{"item_id": item["id"], "output_index": 0, "delta": item[key]})
		event(name+".done", map[string]any{"item_id": item["id"], "output_index": 0, key: item[key]})
	}
	item["status"] = "completed"
	event("response.output_item.done", map[string]any{"output_index": 0, "item": item})
	response["status"], response["output"] = "completed", []any{item}
	input, output := 23, 17
	if call == 2 {
		input, output = 29, 19
	}
	response["usage"] = map[string]any{"input_tokens": input, "output_tokens": output, "total_tokens": input + output, "input_tokens_details": map[string]any{"cached_tokens": 0}, "output_tokens_details": map[string]any{"reasoning_tokens": 0}}
	event("response.completed", map[string]any{"response": response})
	return out.Bytes()
}
