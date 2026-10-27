package platform

import (
	"bytes"
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
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

// The fault is at the real outbound HTTP delivery boundary. Control leases the
// command once, but the child host never receives this source body. Recovery
// uses that original claim until expiry, then an explicit inspect lease.
type workspaceWriteDeliveryFault struct {
	mu       sync.Mutex
	command  string
	dropNext bool
	dropped  int
	results  map[string]wwc.ResultRequest
}

func (f *workspaceWriteDeliveryFault) RoundTrip(request *http.Request) (*http.Response, error) {
	if strings.HasSuffix(request.URL.Path, "/workspace-writes/result") && request.GetBody != nil {
		body, _ := request.GetBody()
		if body != nil {
			var result wwc.ResultRequest
			_ = json.NewDecoder(body).Decode(&result)
			_ = body.Close()
			f.mu.Lock()
			f.results[result.CommandID] = result
			f.mu.Unlock()
		}
	}
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil || !strings.HasSuffix(request.URL.Path, "/workspace-writes/claim") || response.StatusCode != 200 {
		return response, err
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	_ = response.Body.Close()
	if err != nil {
		return nil, err
	}
	var reply struct {
		Lease *wwc.Lease `json:"lease"`
	}
	_ = json.Unmarshal(raw, &reply)
	f.mu.Lock()
	if reply.Lease != nil && reply.Lease.Mode == "execute" && f.dropNext {
		f.command = reply.Lease.Command.ID
		f.dropNext = false
	}
	drop := reply.Lease != nil && reply.Lease.Mode == "execute" && reply.Lease.Command.ID == f.command
	if drop {
		f.dropped++
	}
	f.mu.Unlock()
	if drop {
		return nil, fmt.Errorf("owned workspace execute lease delivery fault")
	}
	response.Body = io.NopCloser(bytes.NewReader(raw))
	return response, nil
}

func TestWorkspaceWriteActualOutboundCASAndExplicitInspection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newFixture(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	holdDir := os.Getenv("SUMA_WORKSPACE_WRITE_UI_HOLD")
	if holdDir != "" {
		holdDir, err = filepath.Abs(holdDir)
		if err != nil || holdDir != filepath.Join(root, ".cache", "e2e", "workspace-write-stage-038", "ui-final") {
			t.Fatal("UI hold must use its exact owned stage directory")
		}
		if err = os.MkdirAll(holdDir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	fixtureDir, repo := t.TempDir(), t.TempDir()
	readyPath, releasePath := filepath.Join(fixtureDir, "host-ready.json"), filepath.Join(fixtureDir, "host-release.request")
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, "source.txt", observationFixtureSource)
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "source.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: outbound CAS source")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(fixtureDir, "workspace-write.bundle.mjs")
	fixture := strings.ReplaceAll(observationHostFixture, "no source changes or formal acceptance claimed", "remote edit facts retained; no formal acceptance claimed")
	if err = os.WriteFile(entry, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	fixtureSHA := sha256.Sum256([]byte(fixture))
	manifestPath := filepath.Join(fixtureDir, "workspace-write.manifest.json")
	manifest, _ := json.Marshal(map[string]any{"schema_version": 1, "entry": filepath.Base(entry), "entry_sha256": hex.EncodeToString(fixtureSHA[:]), "external_imports": []string{"node:readline", "node:fs"}, "dependencies": map[string]string{"@modelcontextprotocol/client": "2.4.0"}})
	if err = os.WriteFile(manifestPath, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"write-native": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{entry}, Input: "agent-jsonl", Env: map[string]string{"SUMA_OBSERVATION_READY": readyPath, "SUMA_OBSERVATION_RELEASE": releasePath}, NativeWorkspace: &runner.NativeWorkspacePolicy{RuntimeManifest: manifestPath, Checks: []string{}}}}}
	inventory, err := runner.DescribeTools(ctx, cfg)
	if err != nil || len(inventory) != 1 || !inventory[0].Available {
		t.Fatalf("owned native inventory unavailable: %+v %v", inventory, err)
	}
	workerID := "workspace-editor-038"
	registered := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": workerID, "profiles": []any{}, "deployment": false}), 201)
	token := registered["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: workerID, Name: "Owned live native edit fixture", Profile: inventory[0].Profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: workerID, OperationID: newID(), Items: inventory}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	timeout := 60
	if holdDir != "" {
		timeout = 1200
	}
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Outbound workspace edit fixture", Nodes: []dag.Node{{ID: "code", Name: "Live editable native workspace", Type: "agent", Command: []string{profile.Profile.CommandID}, TimeoutSeconds: timeout, Config: map[string]any{"goal": "Keep the owned no-model host open for separately authorized remote CAS saves", "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}}}})
	if err != nil {
		t.Fatal(err)
	}
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow.ID, "workflow_version": workflow.Version}), 201)
	serverConfig := &Server{Service: f.s, Engine: f.e, WebDir: t.TempDir()}
	control := httptest.NewUnstartedServer(serverConfig.Handler())
	if holdDir != "" {
		listener, e := net.Listen("tcp4", "127.0.0.1:59218")
		if e != nil {
			t.Fatal(e)
		}
		_ = control.Listener.Close()
		control.Listener = listener
		serverConfig.WebDir = filepath.Join(root, ".cache", "e2e", "workspace-write-stage-038", "web")
		serverConfig.AllowedOrigins = []string{"http://workspace038.localhost:59218", "http://127.0.0.1:59218"}
		serverConfig.PublicURL, serverConfig.PublicURLSource = "http://127.0.0.1:59218", "configured"
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
	fault := &workspaceWriteDeliveryFault{results: map[string]wwc.ResultRequest{}}
	worker := &runner.Worker{Client: &runner.Client{BaseURL: control.URL, ProjectID: f.project, Token: token, HTTP: &http.Client{Transport: fault, Timeout: 5 * time.Second}}, Journal: journal, ID: workerID, Exec: cfg, PollInterval: 20 * time.Millisecond, HeartbeatInterval: 100 * time.Millisecond}
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
	var scope woc.Scope
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		response := f.call("GET", "/workspace/targets?project_id="+f.project, nil, 200)
		raw, _ := json.Marshal(response)
		var page woc.TargetPage
		_ = json.Unmarshal(raw, &page)
		if len(page.Items) == 1 {
			scope = page.Items[0].WorkspaceObservationScope
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if scope.RunID != run["id"] || scope.ToolProfileID != profile.ID {
		t.Fatal("current native target was not discovered")
	}
	for time.Now().Before(deadline) {
		if _, err = os.Stat(readyPath); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	hostRaw, err := os.ReadFile(readyPath)
	if err != nil {
		t.Fatal(err)
	}
	var host struct {
		CWD string `json:"cwd"`
	}
	_ = json.Unmarshal(hostRaw, &host)
	if host.CWD == repo || !under(filepath.Join(repo, ".cache"), host.CWD) {
		t.Fatal("child did not use its separate attempt worktree")
	}
	var bindings int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workspace_bindings WHERE project_id=?`, f.project).Scan(&bindings); err != nil || bindings != 0 {
		t.Fatal("Control acquired local workspace access")
	}
	read := func() woc.Observation {
		t.Helper()
		created := f.call("POST", "/workspace/observations", f.body(map[string]any{"scope": scope, "operation": "read", "arguments": woc.Arguments{Path: "source.txt", FullFile: true}}), 201)
		until := time.Now().Add(5 * time.Second)
		for time.Now().Before(until) {
			response := f.call("GET", "/workspace/observations/"+created["id"].(string)+"?project_id="+f.project, nil, 200)
			raw, _ := json.Marshal(response)
			var result woc.Observation
			_ = json.Unmarshal(raw, &result)
			if result.State == "completed" {
				return result
			}
			if result.State == "failed" {
				t.Fatalf("full-file read failed: %+v", result)
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatal("outbound full-file read did not settle")
		return woc.Observation{}
	}
	original := read()
	if original.Result == nil || original.Result.Read == nil || original.Result.Read.Content != observationFixtureSource || original.Result.Read.SHA256 != digest(observationFixtureSource) || !original.Result.Read.Complete {
		t.Fatal("actual full-file source was incomplete")
	}
	create := func(expected, content string) wwc.Command {
		t.Helper()
		response := f.call("POST", "/workspace/writes", f.body(map[string]any{"scope": scope, "workspace_policy_sha256": original.WorkspacePolicySHA256, "path": "source.txt", "expected_sha256": expected, "content": content, "content_sha256": wwc.ContentDigest(content)}), 201)
		raw, _ := json.Marshal(response)
		var result wwc.Command
		_ = json.Unmarshal(raw, &result)
		if strings.Contains(string(raw), content) || strings.Contains(string(raw), host.CWD) || strings.Contains(string(raw), token) {
			t.Fatal("save metadata leaked source body or host authority")
		}
		return result
	}
	waitCommand := func(id, state string, duration time.Duration) wwc.Command {
		t.Helper()
		until := time.Now().Add(duration)
		for time.Now().Before(until) {
			response := f.call("GET", "/workspace/writes/"+id+"?project_id="+f.project, nil, 200)
			raw, _ := json.Marshal(response)
			var result wwc.Command
			_ = json.Unmarshal(raw, &result)
			if result.State == state {
				return result
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("workspace write did not reach %s", state)
		return wwc.Command{}
	}
	savedText := "Saved through the real outbound native CAS channel.\n真实人工编辑，保持原文件比较基线。\n"
	success := create(original.Result.Read.SHA256, savedText)
	success = waitCommand(success.ID, "applied", 5*time.Second)
	if success.Result == nil || success.Result.NativeOperationID != wwc.NativeOperationID(success.ID) || success.Result.PreviousSHA256 != original.Result.Read.SHA256 || success.Result.SHA256 != wwc.ContentDigest(savedText) || success.Result.Generation != 1 {
		t.Fatal("original native effect receipt was not exact")
	}
	conflict := create(original.Result.Read.SHA256, "Stale draft must remain unapplied.\n")
	conflict = waitCommand(conflict.ID, "not_applied", 5*time.Second)
	if conflict.ErrorCode != "file_changed" || conflict.Result != nil {
		t.Fatal("stale editor baseline was overwritten")
	}
	// Hold the original execute delivery at HTTP until its one permit expires.
	// Arm before creation, after earlier commands settled. Do not hold a test
	// mutex across a request that may fail: cancellation must always drain.
	fault.mu.Lock()
	fault.dropNext = true
	fault.mu.Unlock()
	uncertain := create(success.Result.SHA256, "Lost lease delivery must never change source.\n")
	uncertain = waitCommand(uncertain.ID, "reconciling", 15*time.Second)
	f.call("POST", "/workspace/writes/"+uncertain.ID+"/inspect", f.body(map[string]any{"expected_version": uncertain.Version}), 200)
	inspected := waitCommand(uncertain.ID, "not_applied", 5*time.Second)
	fault.mu.Lock()
	dropped := fault.dropped
	faultCommand := fault.command
	executeReceipt, hasExecute := fault.results[success.ID]
	inspectReceipt := fault.results[uncertain.ID]
	fault.command = ""
	fault.mu.Unlock()
	if dropped < 1 || faultCommand != uncertain.ID || inspected.ErrorCode != "not_started" || inspected.Result != nil || inspectReceipt.Mode != "inspect" || inspectReceipt.Outcome != "not_applied" || !hasExecute || executeReceipt.Mode != "execute" || wwc.ValidateResult(success, executeReceipt) != nil {
		t.Fatal("explicit inspection retried writing or lost original receipt")
	}
	actual, err := os.ReadFile(filepath.Join(host.CWD, "source.txt"))
	if err != nil || string(actual) != savedText {
		t.Fatal("delivery loss/inspection changed actual source")
	}
	current := read()
	if current.Result.Read.SHA256 != success.Result.SHA256 {
		t.Fatal("remote readback disagreed with native CAS receipt")
	}
	ready := map[string]any{"port": 59218, "scope": scope, "run_id": scope.RunID, "project_id": f.project, "source_sha256": current.Result.Read.SHA256, "workspace_policy_sha256": original.WorkspacePolicySHA256, "applied_command_id": success.ID, "conflict_command_id": conflict.ID, "inspected_command_id": uncertain.ID, "dropped_execute_deliveries": dropped, "control_local_workspace_bindings": bindings, "model_calls": 0, "formal_tests": 0}
	if holdDir != "" {
		login, _ := json.Marshal(map[string]any{"base_url": control.URL, "browser_origin": "http://workspace038.localhost:59218", "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "run_id": scope.RunID, "scope": scope})
		if err = os.WriteFile(filepath.Join(holdDir, "login.local.json"), login, 0600); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(ready)
		if err = os.WriteFile(filepath.Join(holdDir, "ready.local.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
		t.Log("owned workspace write UI hold ready on port 59218")
		until := time.Now().Add(20 * time.Minute)
		for time.Now().Before(until) {
			if _, err = os.Stat(filepath.Join(holdDir, "stop.request")); err == nil {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	if err = os.WriteFile(releasePath, []byte("close owned no-model child\n"), 0600); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(8 * time.Second)
	finished := false
	for time.Now().Before(until) {
		currentRun, e := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
		if e == nil && currentRun.State == "succeeded" {
			finished = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !finished {
		currentRun, _ := f.e.GetRun(ctx, f.user.TenantID, f.project, scope.RunID)
		t.Fatalf("owned native process did not close normally: %+v", currentRun)
	}
	persisted := waitCommand(success.ID, "applied", time.Second)
	if persisted.Result == nil || *persisted.Result != *success.Result {
		t.Fatal("closing the host erased the applied effect fact")
	}
	f.call("POST", "/workspace/writes", f.body(map[string]any{"scope": scope, "workspace_policy_sha256": original.WorkspacePolicySHA256, "path": "source.txt", "expected_sha256": success.Result.SHA256, "content": "closed host\n", "content_sha256": wwc.ContentDigest("closed host\n")}), 409)
	if workspaceGitTestCommand(t, repo, "rev-parse", "HEAD") != base {
		t.Fatal("remote save changed administrator source HEAD")
	}
	ready["host_closed"], ready["applied_fact_survived_close"] = true, true
	raw, _ := json.Marshal(ready)
	if holdDir != "" {
		if err = os.WriteFile(filepath.Join(holdDir, "closed.local.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("workspace_write_actual=%s", raw)
}
