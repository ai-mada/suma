package platform

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

// The test executable is an explicitly owned stdio-protocol child. It does
// not load Codex, contact a model, edit source, or claim provider attestation.
// Dispatching this child still exercises the real Runner process controller.
func init() {
	if os.Getenv("SUMA_CODEX_WORKSPACE_RPC_FIXTURE") != "1" {
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("codex-cli owned-workspace-protocol-fixture")
		os.Exit(0)
	}
	if len(os.Args) == 3 && os.Args[1] == "app-server" && os.Args[2] == "--help" {
		fmt.Println("app-server --listen stdio://")
		os.Exit(0)
	}
	if len(os.Args) != 4 || strings.Join(os.Args[1:], " ") != "app-server --listen stdio://" {
		os.Exit(71)
	}
	emit := func(v any) { raw, _ := json.Marshal(v); fmt.Println(string(raw)) }
	notify := func(method string, p any) { emit(map[string]any{"method": method, "params": p}) }
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 128<<10)
	for scanner.Scan() {
		var request struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			os.Exit(72)
		}
		reply := func(value any) { emit(map[string]any{"id": request.ID, "result": value}) }
		switch request.Method {
		case "initialize":
			reply(map[string]any{"userAgent": "owned-workspace-fixture", "codexHome": os.Getenv("CODEX_HOME")})
		case "initialized":
		case "model/list":
			reply(map[string]any{"data": []any{map[string]any{"id": "owned-workspace-model", "model": "owned-workspace-model", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "low"}}, "defaultReasoningEffort": "low"}}, "nextCursor": nil})
		case "thread/start":
			reply(map[string]any{"thread": map[string]any{"id": "workspace-thread"}, "model": "owned-workspace-model", "modelProvider": "owned103", "reasoningEffort": "low", "cwd": request.Params["cwd"], "approvalPolicy": "never", "sandbox": map[string]any{"type": "workspaceWrite", "writableRoots": []any{request.Params["cwd"]}, "networkAccess": false, "excludeTmpdirEnvVar": true, "excludeSlashTmp": true}})
		case "turn/start":
			turn := map[string]any{"id": "workspace-turn", "status": "inProgress"}
			notify("turn/started", map[string]any{"threadId": "workspace-thread", "turn": turn})
			reply(map[string]any{"turn": turn})
			if ready := os.Getenv("SUMA_CODEX_WORKSPACE_READY"); ready != "" {
				raw, _ := json.Marshal(map[string]any{"source": request.Params["cwd"]})
				if os.WriteFile(ready, raw, 0600) != nil {
					os.Exit(73)
				}
			}
		case "turn/interrupt":
			reply(map[string]any{})
			notify("turn/completed", map[string]any{"threadId": "workspace-thread", "turn": map[string]any{"id": "workspace-turn", "status": "interrupted"}})
		default:
			os.Exit(74)
		}
	}
	os.Exit(0)
}

// This single combined journey covers actual outbound HTTP, live os.Root,
// read-only browser facts and cancellation. An optional bounded UI hold reuses
// this very operation; it never launches a second model or execution journey.
func TestCodexWorkspaceActualOutboundReadAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFixture(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	holdDir := os.Getenv("SUMA_CODEX_WORKSPACE_UI_DIR")
	if holdDir != "" {
		holdDir, err = filepath.Abs(holdDir)
		if err != nil || !under(filepath.Join(root, ".cache", "e2e", "codex-workspace-103"), holdDir) {
			t.Fatal("UI evidence must remain in its exact stage cache directory")
		}
		if err = os.MkdirAll(holdDir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	fixtureDir, repo := t.TempDir(), t.TempDir()
	readyPath := filepath.Join(fixtureDir, "child.ready.json")
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	const source = "Owned live Codex protocol workspace.\n真实工作树，只读观察。\n"
	workspaceGitTestWrite(t, repo, "source.txt", source)
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "source.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: live Codex workspace")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"codex-workspace": {Adapter: "codex", Protocol: cc.Protocol, Executable: executable, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", Env: map[string]string{"SUMA_CODEX_WORKSPACE_RPC_FIXTURE": "1", "SUMA_CODEX_WORKSPACE_READY": readyPath}, CodexAppServer: &runner.CodexAppServerSpec{ProviderID: "owned103", BaseURL: "http://127.0.0.1:49103/v1", WindowsSandbox: "unelevated"}}}}
	inventory, err := runner.DescribeTools(ctx, cfg)
	if err != nil || len(inventory) != 1 || !inventory[0].Available || inventory[0].CodexCatalog == nil || inventory[0].WorkspaceObservationProtocol != woc.Protocol || inventory[0].WorkspaceWriteProtocol != "" {
		t.Fatalf("current Codex read-only inventory unavailable: %+v %v", inventory, err)
	}
	workerID := "codex-workspace-103"
	registered := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": workerID, "profiles": []any{}, "deployment": false}), 201)
	token := registered["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: workerID, Name: "Owned Codex read-only workspace", Profile: inventory[0].Profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: workerID, OperationID: newID(), Items: inventory}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	catalog := *inventory[0].CodexCatalog
	binding, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: "owned-workspace-model", ReasoningEffort: "low"})
	if err != nil {
		t.Fatal(err)
	}
	timeout := 40
	if holdDir != "" {
		timeout = 900
	}
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "协议夹具：Codex工作区只读观察", Nodes: []dag.Node{{ID: "code", Name: "Codex live workspace", Type: "agent", Command: []string{profile.Profile.CommandID}, TimeoutSeconds: timeout, Config: map[string]any{"goal": "Keep the owned protocol child open for read-only observation; no inference, editing, tests or acceptance.", "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256, "codex_execution": binding}}}})
	if err != nil {
		t.Fatal(err)
	}
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow.ID, "workflow_version": workflow.Version}), 201)
	server := &Server{Service: f.s, Engine: f.e, WebDir: t.TempDir()}
	control := httptest.NewUnstartedServer(server.Handler())
	origin := ""
	if holdDir != "" {
		web, e := filepath.Abs(os.Getenv("SUMA_CODEX_WORKSPACE_WEB_DIR"))
		if e != nil || !under(filepath.Join(root, ".cache", "e2e", "codex-workspace-103"), web) {
			t.Fatal("UI build must remain in the owned stage cache")
		}
		if _, e = os.Stat(filepath.Join(web, "index.html")); e != nil {
			t.Fatal(e)
		}
		listener, e := net.Listen("tcp4", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		_ = control.Listener.Close()
		control.Listener = listener
		origin = "http://codexfiles103.localhost:" + fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)
		server.WebDir, server.PublicURL, server.PublicURLSource = web, "http://"+listener.Addr().String(), "configured"
		server.AllowedOrigins = []string{origin}
		control.Config.Handler = server.Handler()
	}
	control.Start()
	defer control.Close()
	f.handler = control.Config.Handler
	journal, err := runner.OpenJournal(filepath.Join(fixtureDir, "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	worker := &runner.Worker{Client: &runner.Client{BaseURL: control.URL, ProjectID: f.project, Token: token}, Journal: journal, ID: workerID, Exec: cfg, PollInterval: 100 * time.Millisecond, HeartbeatInterval: time.Second}
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("owned Worker did not close")
		}
	}()
	waitFor := func(label string, condition func() bool) {
		t.Helper()
		until := time.Now().Add(10 * time.Second)
		for time.Now().Before(until) {
			if condition() {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatal(label)
	}
	waitFor("owned protocol child did not start", func() bool { _, e := os.Stat(readyPath); return e == nil })
	var host struct {
		Source string `json:"source"`
	}
	rawHost, err := os.ReadFile(readyPath)
	if err != nil || json.Unmarshal(rawHost, &host) != nil || host.Source == repo || !under(filepath.Join(repo, ".cache"), host.Source) {
		t.Fatal("child did not use an exact isolated attempt source")
	}
	page := f.call("GET", "/workspace/targets?project_id="+f.project, nil, 200)
	var targets woc.TargetPage
	if json.Unmarshal([]byte(stringify(page)), &targets) != nil || len(targets.Items) != 1 || targets.Items[0].CanWrite {
		t.Fatalf("Codex target must be current and read only: %+v", page)
	}
	scope := targets.Items[0].WorkspaceObservationScope
	if scope.RunID != run["id"] || scope.ToolProfileID != profile.ID {
		t.Fatal("target is not the original exact assignment")
	}
	observe := func(operation string, arguments woc.Arguments, expected string) woc.Observation {
		t.Helper()
		created := f.call("POST", "/workspace/observations", f.body(map[string]any{"scope": scope, "operation": operation, "arguments": arguments, "expected_sha256": expected}), 201)
		var out woc.Observation
		waitFor("outbound broker did not settle", func() bool {
			read := f.call("GET", "/workspace/observations/"+created["id"].(string)+"?project_id="+f.project, nil, 200)
			raw := stringify(read)
			if strings.Contains(raw, host.Source) || strings.Contains(raw, repo) || strings.Contains(raw, token) {
				t.Fatal("broker disclosed host roots or credentials")
			}
			if json.Unmarshal([]byte(raw), &out) != nil {
				t.Fatal("invalid observation wire result")
			}
			return out.State == "completed" || out.State == "failed"
		})
		return out
	}
	listed := observe("list", woc.Arguments{Limit: 20}, "")
	if listed.Result == nil || listed.Result.List == nil || !strings.Contains(stringify(listed.Result.List), "source.txt") {
		t.Fatalf("actual outbound list failed: %+v", listed)
	}
	read := observe("read", woc.Arguments{Path: "source.txt", MaxLines: 10}, "")
	if read.Result == nil || read.Result.Read == nil || read.Result.Read.Content != source || read.Result.Read.SHA256 != digest(source) {
		t.Fatalf("actual outbound read is not the original source: %+v", read)
	}
	changed := observe("read", woc.Arguments{Path: "source.txt"}, strings.Repeat("0", 64))
	if changed.State != "failed" || changed.ErrorCode != "file_changed" || changed.Result != nil {
		t.Fatal("wrong fixed file digest was not rejected")
	}
	f.call("POST", "/workspace/observations", f.body(map[string]any{"scope": scope, "operation": "read", "arguments": woc.Arguments{Path: ".codex/config.toml"}}), 400)
	f.call("POST", "/workspace/writes", f.body(map[string]any{"scope": scope, "workspace_policy_sha256": read.WorkspacePolicySHA256, "path": "source.txt", "expected_sha256": digest(source), "content": "must not write", "content_sha256": digest("must not write")}), 409)
	if holdDir != "" {
		codexActualJSON(t, filepath.Join(holdDir, "login.local.json"), map[string]any{"origin": origin, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "run_id": scope.RunID, "scope": scope})
		codexActualJSON(t, filepath.Join(holdDir, "ready.public.json"), map[string]any{"origin": origin, "project_id": f.project, "run_id": scope.RunID, "read_only": true, "source_sha256": digest(source), "model_calls": 0, "actual_cli_calls": 0})
		t.Logf("owned Codex workspace GUI ready: %s", origin)
		until := time.Now().Add(10 * time.Minute)
		for time.Now().Before(until) {
			if _, e := os.Stat(filepath.Join(holdDir, "stop.request")); e == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	current, err := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.Command(ctx, f.user.TenantID, f.project, scope.RunID, "cancel", newID(), current.Version, nil); err != nil {
		t.Fatal(err)
	}
	waitFor("original owned protocol run did not cancel", func() bool {
		current, e := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
		return e == nil && current.State == "cancelled"
	})
	closed := f.call("GET", "/workspace/observations/"+read.ID+"?project_id="+f.project, nil, 200)
	if closed["result"] != nil || closed["state"] != "cancelled" && closed["state"] != "expired" {
		t.Fatal("closed exact attempt disclosed old file content", closed)
	}
	f.call("POST", "/workspace/observations", f.body(map[string]any{"scope": scope, "operation": "list", "arguments": woc.Arguments{}}), 409)
	if workspaceGitTestCommand(t, repo, "rev-parse", "HEAD") != base {
		t.Fatal("observation changed administrator branch")
	}
	var bindings, writes, candidates, reviews int
	for table, target := range map[string]*int{"suma_workspace_bindings": &bindings, "suma_workspace_writes": &writes, "suma_candidate_transfers": &candidates, "suma_reviews": &reviews} {
		if err = f.s.DB.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE project_id=?", f.project).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	if bindings != 0 || writes != 0 || candidates != 0 || reviews != 0 {
		t.Fatalf("read-only transport gained unrelated effects: %d/%d/%d/%d", bindings, writes, candidates, reviews)
	}
	summary := map[string]any{"kind": "owned-protocol-fixture-real-Control-Worker-Git", "scope": scope, "read_only": true, "source_sha256": digest(source), "workspace_policy_sha256": read.WorkspacePolicySHA256, "closed_target_result_redacted": true, "admin_head_unchanged": true, "local_bindings": bindings, "write_commands": writes, "candidate_transfers": candidates, "reviews": reviews, "model_calls": 0, "actual_cli_calls": 0, "vendor_calls": 0, "cancelled": true}
	if holdDir != "" {
		codexActualJSON(t, filepath.Join(holdDir, "closed.public.json"), summary)
	}
	t.Logf("codex_workspace_outbound=%s", stringify(summary))
}
