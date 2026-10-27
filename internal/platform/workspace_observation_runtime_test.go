package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

const observationFixtureSource = "Runner attempt source, served through outbound observation only.\n真实执行工作树的第二行。\n"

// An owned no-model protocol child keeps the real native host alive. The
// fixture performs no inference and does not claim to be a vendor SDK run.
const observationHostFixture = `import {createInterface} from 'node:readline';import {existsSync,writeFileSync} from 'node:fs';
if(process.argv.includes('--version')){console.log('suma-agent-runtime owned-observation-fixture');process.exit(0)}
if(process.argv.includes('--help')){console.log('JSONL SUMA_AGENT_TOKEN owned no-model host fixture');process.exit(0)}
const input=createInterface({input:process.stdin});input.once('line',line=>{const request=JSON.parse(line);writeFileSync(process.env.SUMA_OBSERVATION_READY,JSON.stringify({cwd:process.cwd()}));const emit=event=>console.log(JSON.stringify({...event,request_id:request.request_id}));emit({type:'started',model:'owned-no-model-protocol-fixture',tools:[]});const timer=setInterval(()=>{if(existsSync(process.env.SUMA_OBSERVATION_RELEASE)){clearInterval(timer);emit({type:'completed',output:'Owned no-model observation fixture closed; no source changes or formal acceptance claimed.'});input.close()}},25)});`

func TestWorkspaceObservationActualOutboundWorkerAndClosedHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFixture(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	holdDir := os.Getenv("SUMA_WORKSPACE_OBSERVATION_UI_HOLD")
	if holdDir != "" {
		holdDir, err = filepath.Abs(holdDir)
		if err != nil || !under(filepath.Join(root, ".cache", "e2e", "workspace-observation-stage-037"), holdDir) {
			t.Fatal("UI hold must use its owned stage cache directory")
		}
		if err = os.MkdirAll(holdDir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	fixtureDir := t.TempDir()
	readyPath := filepath.Join(fixtureDir, "host-ready.json")
	releasePath := filepath.Join(fixtureDir, "host-release.request")
	repo := t.TempDir()
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, "source.txt", observationFixtureSource)
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "source.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: outbound workspace source")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for the explicit native process gate", err)
	}
	entry := filepath.Join(fixtureDir, "observation.bundle.mjs")
	if err = os.WriteFile(entry, []byte(observationHostFixture), 0600); err != nil {
		t.Fatal(err)
	}
	fixtureSHA := sha256.Sum256([]byte(observationHostFixture))
	manifestPath := filepath.Join(fixtureDir, "observation.manifest.json")
	manifest, _ := json.Marshal(map[string]any{"schema_version": 1, "entry": filepath.Base(entry), "entry_sha256": hex.EncodeToString(fixtureSHA[:]), "external_imports": []string{"node:readline", "node:fs"}, "dependencies": map[string]string{"@modelcontextprotocol/client": "2.4.0"}})
	if err = os.WriteFile(manifestPath, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"observation-native": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{entry}, Input: "agent-jsonl", Env: map[string]string{"SUMA_OBSERVATION_READY": readyPath, "SUMA_OBSERVATION_RELEASE": releasePath}, NativeWorkspace: &runner.NativeWorkspacePolicy{RuntimeManifest: manifestPath, Checks: []string{}}}}}
	inventory, err := runner.DescribeTools(ctx, cfg)
	if err != nil || len(inventory) != 1 || !inventory[0].Available {
		t.Fatalf("owned native fixture inventory unavailable: %+v %v", inventory, err)
	}
	workerID := "workspace-observer-037"
	registered := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": workerID, "profiles": []any{}, "deployment": false}), 201)
	token := registered["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: workerID, Name: "Owned live native observation fixture", Profile: inventory[0].Profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: workerID, OperationID: newID(), Items: inventory})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	timeoutSeconds := 30
	if holdDir != "" {
		timeoutSeconds = 1200
	}
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Outbound workspace observation fixture", Nodes: []dag.Node{{ID: "code", Name: "Live native workspace", Type: "agent", Command: []string{profile.Profile.CommandID}, TimeoutSeconds: timeoutSeconds, Config: map[string]any{"goal": "Keep the owned no-model native host open for bounded read-only observation", "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}}}})
	if err != nil {
		t.Fatal(err)
	}
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow.ID, "workflow_version": workflow.Version}), 201)
	serverConfig := &Server{Service: f.s, Engine: f.e, WebDir: t.TempDir()}
	control := httptest.NewUnstartedServer(serverConfig.Handler())
	if holdDir != "" {
		listener, listenErr := net.Listen("tcp4", "127.0.0.1:59217")
		if listenErr != nil {
			t.Fatal(listenErr)
		}
		_ = control.Listener.Close()
		control.Listener = listener
		serverConfig.WebDir = filepath.Join(root, ".cache", "e2e", "workspace-observation-stage-037", "web")
		serverConfig.AllowedOrigins = []string{"http://workspace037.localhost:59217"}
		serverConfig.PublicURL = "http://127.0.0.1:59217"
		serverConfig.PublicURLSource = "configured"
		control.Config.Handler = serverConfig.Handler()
	}
	control.Start()
	defer control.Close()
	f.handler = control.Config.Handler
	journal, err := runner.OpenJournal(filepath.Join(fixtureDir, "worker-journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	worker := &runner.Worker{Client: &runner.Client{BaseURL: control.URL, ProjectID: f.project, Token: token}, Journal: journal, ID: workerID, Exec: cfg, PollInterval: 20 * time.Millisecond, HeartbeatInterval: 100 * time.Millisecond}
	if holdDir != "" {
		worker.HeartbeatInterval = time.Second
	}
	workerDone := make(chan error, 1)
	go func() { workerDone <- worker.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case <-workerDone:
		case <-time.After(5 * time.Second):
			t.Error("owned worker did not close")
		}
	}()
	deadline := time.Now().Add(6 * time.Second)
	var scope woc.Scope
	for time.Now().Before(deadline) {
		targets := f.call("GET", "/workspace/targets?project_id="+f.project, nil, 200)
		raw, _ := json.Marshal(targets)
		var page woc.TargetPage
		_ = json.Unmarshal(raw, &page)
		if len(page.Items) == 1 {
			scope = page.Items[0].WorkspaceObservationScope
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if scope.RunID != run["id"] || scope.ToolProfileID != profile.ID {
		t.Fatal("current running native target was not discovered")
	}
	for time.Now().Before(deadline) {
		if _, err = os.Stat(readyPath); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	hostRaw, err := os.ReadFile(readyPath)
	if err != nil {
		t.Fatal("actual child did not become active", err)
	}
	var host struct {
		CWD string `json:"cwd"`
	}
	if json.Unmarshal(hostRaw, &host) != nil || host.CWD == repo || !under(filepath.Join(repo, ".cache"), host.CWD) {
		t.Fatal("child did not use a separate attempt worktree")
	}
	var localBindings int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workspace_bindings WHERE project_id=?`, f.project).Scan(&localBindings); err != nil || localBindings != 0 {
		t.Fatal("Control gained a local workspace binding")
	}
	observe := func(operation string, args woc.Arguments, expected string) woc.Observation {
		t.Helper()
		created := f.call("POST", "/workspace/observations", f.body(map[string]any{"scope": scope, "operation": operation, "arguments": args, "expected_sha256": expected}), 201)
		id := created["id"].(string)
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			response := f.call("GET", "/workspace/observations/"+id+"?project_id="+f.project, nil, 200)
			raw, _ := json.Marshal(response)
			var result woc.Observation
			_ = json.Unmarshal(raw, &result)
			if result.State == "completed" || result.State == "failed" {
				if strings.Contains(string(raw), host.CWD) || strings.Contains(string(raw), repo) || strings.Contains(string(raw), token) {
					t.Fatal("remote result leaked host paths or credentials")
				}
				return result
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("real outbound workspace observation did not settle")
		return woc.Observation{}
	}
	listed := observe("list", woc.Arguments{Limit: 20}, "")
	if listed.Result == nil || listed.Result.List == nil {
		t.Fatalf("outbound native list failed: %+v", listed)
	}
	found := false
	for _, entry := range listed.Result.List.Entries {
		if entry.Path == "source.txt" && entry.Kind == "file" {
			found = true
		}
	}
	if !found {
		t.Fatal("real attempt source was absent from outbound list")
	}
	read := observe("read", woc.Arguments{Path: "source.txt", MaxLines: 10}, "")
	if read.Result == nil || read.Result.Read == nil || read.Result.Read.Content != observationFixtureSource || read.Result.Read.SHA256 != digest(observationFixtureSource) {
		t.Fatalf("outbound native read did not return real attempt source: %+v", read)
	}
	denied := observe("read", woc.Arguments{Path: "SOURCE.txt"}, "")
	if denied.State != "failed" || denied.Result != nil || denied.ErrorCode != "path_not_allowed" {
		t.Fatalf("native exact-case path boundary was not enforced: %+v", denied)
	}
	f.call("POST", "/workspace/observations", f.body(map[string]any{"scope": scope, "operation": "read", "arguments": woc.Arguments{Path: ".env"}}), 400)
	if holdDir != "" {
		privateLogin, _ := json.Marshal(map[string]any{"base_url": control.URL, "browser_origin": "http://workspace037.localhost:59217", "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "run_id": scope.RunID, "scope": scope})
		if err = os.WriteFile(filepath.Join(holdDir, "login.local.json"), privateLogin, 0600); err != nil {
			t.Fatal(err)
		}
		ready, _ := json.Marshal(map[string]any{"port": 59217, "project_id": f.project, "run_id": scope.RunID, "source_sha256": read.Result.Read.SHA256, "list_request_id": listed.ID, "read_request_id": read.ID, "control_local_workspace_bindings": localBindings, "model_calls": 0, "formal_tests": 0})
		if err = os.WriteFile(filepath.Join(holdDir, "ready.local.json"), ready, 0600); err != nil {
			t.Fatal(err)
		}
		t.Log("owned workspace observation UI hold ready on port 59217")
		holdUntil := time.Now().Add(15 * time.Minute)
		for time.Now().Before(holdUntil) {
			if _, err = os.Stat(filepath.Join(holdDir, "stop.request")); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if err = os.WriteFile(releasePath, []byte("close owned no-model child\n"), 0600); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(6 * time.Second)
	finished := false
	for time.Now().Before(until) {
		current, readErr := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
		if readErr == nil && current.State == "succeeded" {
			finished = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !finished {
		current, _ := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
		t.Fatalf("owned native process did not close normally: %+v", current)
	}
	closed := f.call("GET", "/workspace/observations/"+read.ID+"?project_id="+f.project, nil, 200)
	if (closed["state"] != "cancelled" && closed["state"] != "expired") || closed["result"] != nil {
		t.Fatal("closed target disclosed a historical source result", closed)
	}
	f.call("POST", "/workspace/observations", f.body(map[string]any{"scope": scope, "operation": "list", "arguments": woc.Arguments{}}), 409)
	if head := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD"); head != base {
		t.Fatal("outbound observation changed administrator source")
	}
	summary := map[string]any{"scope": scope, "workspace_policy_sha256": read.WorkspacePolicySHA256, "list_request_id": listed.ID, "read_request_id": read.ID, "source_sha256": read.Result.Read.SHA256, "exact_case_path_error": denied.ErrorCode, "control_local_workspace_bindings": 0, "native_protocol_processes": 1, "model_calls": 0, "formal_tests": 0, "closed_target_result_redacted": true, "closed_observation_state": closed["state"], "host_closed": true}
	if holdDir != "" {
		raw, _ := json.Marshal(summary)
		if err = os.WriteFile(filepath.Join(holdDir, "closed.local.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := json.Marshal(summary)
	t.Logf("workspace_observation_actual=%s", raw)
}
