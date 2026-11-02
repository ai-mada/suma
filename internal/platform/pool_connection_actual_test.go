package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	"github.com/zalando/go-keyring"
)

// This one opt-in journey owns its database, prepared files and initially
// absent system-vault account. The Node driver runs one copied CLI check. The
// foreground start commands are observed; explicit fixture actions use the
// same original PoolWorker library/configuration, not a second dispatcher.
func TestPoolConnectionActualPreparedIdentityInventoryAndExecution(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("SUMA_POOL_CONNECTION_212_ACTUAL") != "1" {
		t.Skip("explicit isolated system-vault/HTTP/browser journey required")
	}
	root, e := filepath.Abs(filepath.Join("..", ".."))
	if e != nil {
		t.Fatal(e)
	}
	node, compiled := os.Getenv("SUMA_POOL_CONNECTION_212_NODE"), os.Getenv("SUMA_POOL_CONNECTION_212_RUNNER")
	for _, path := range []string{node, compiled} {
		if !filepath.IsAbs(path) {
			t.Fatal("existing absolute Node and compiled Runner required")
		}
		if st, e := os.Stat(path); e != nil || !st.Mode().IsRegular() {
			t.Fatal("existing executable unavailable")
		}
	}
	web := filepath.Join(root, ".cache", "pool-connection-212", "web", "out")
	if _, e = os.Stat(filepath.Join(web, "index.html")); e != nil {
		t.Fatal("isolated production UI build required before actual journey")
	}
	dir := filepath.Join(root, ".cache", "pool-connection-212", "actual", strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", ""))
	if e = os.MkdirAll(dir, 0700); e != nil {
		t.Fatal(e)
	}
	receipt := map[string]any{"stage": "T-POOL-CONNECTION-212", "scope": "Actual isolated HTTP/SQLite, one system-backed original Pool pairing, one real compiled CLI check, original library PoolWorker maintenance then explicitly allowed owned read-only command; no vendor/model/code acceptance", "passed": false, "foreground_start_command_executed": false}
	defer func() {
		if t.Failed() {
			receipt["passed"] = false
		}
		raw, _ := json.MarshalIndent(receipt, "", "  ")
		_ = os.WriteFile(filepath.Join(dir, "receipt.json"), append(raw, '\n'), 0600)
	}()
	vault := &poolPairingActualVault{provider: runner.SystemRunnerSecretStore(), missing: map[string]bool{}, owned: map[string][3]string{}}
	defer func() {
		vault.mu.Lock()
		defer vault.mu.Unlock()
		removed := 0
		for _, entry := range vault.owned {
			value, e := vault.provider.Get(entry[0], entry[1])
			if e != nil || digest(value) != entry[2] {
				t.Error("owned credential changed; refusing cleanup")
				continue
			}
			if e = keyring.Delete(entry[0], entry[1]); e != nil {
				t.Error("owned credential cleanup failed")
				continue
			}
			if _, e = vault.provider.Get(entry[0], entry[1]); !errors.Is(e, runner.ErrRunnerSecretNotFound) {
				t.Error("owned credential absence unconfirmed")
				continue
			}
			removed++
		}
		receipt["owned_system_credentials_created"], receipt["owned_system_credentials_removed"] = len(vault.owned), removed
	}()
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	defer func() {
		if _, e := f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", filepath.Join(dir, "private.sqlite")); e != nil {
			t.Error("owned fixture snapshot failed")
		}
	}()
	pool, e := f.s.CreateExecutionPool(ctx, f.e, f.actor, ExecutionPoolCreateRequest{Name: "Owned Pool connection212", Policy: ExecutionPoolPolicyInput{ConcurrencyLimit: 1, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{}}, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	second, e := f.s.CreateProject(ctx, f.actor, map[string]any{"title": "Owned Pool212 second scope", "idempotency_key": newID()})
	if e != nil {
		t.Fatal(e)
	}
	projects := []string{f.project, second.ID}
	grants := []ExecutionPoolGrant{}
	for _, project := range projects {
		g, e := f.s.RequestExecutionPoolGrant(ctx, f.actor, ExecutionPoolGrantRequest{ProjectID: project, PoolID: pool.ID, ProjectWeight: 1, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		g, e = f.s.CommandOwnedPoolGrant(ctx, f.e, f.actor, pool.ID, g.ID, ExecutionPoolGrantCommand{Action: "approve", ExpectedVersion: g.Version, IdempotencyKey: newID()})
		if e != nil {
			t.Fatal(e)
		}
		grants = append(grants, g)
		tx, e := f.s.DB.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.s.authorizeTx(ctx, tx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: project}, purposeBrowserProjectApprove)
		if e == nil {
			_, e = f.e.WriteProjectExecutionRouteTx(ctx, tx, f.user.TenantID, project, 0, dag.ProjectExecutionRouteInput{PoolID: pool.ID, GrantID: g.ID, GrantVersion: g.Version})
		}
		if e != nil {
			tx.Rollback()
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
	}
	session := f.call("GET", "/auth/session", nil, 200)
	platformServer := &Server{Service: f.s, Engine: f.e, WebDir: web}
	platformHTTP := platformServer.Handler()
	actionKey := newID() + newID()
	identityPath, configPath := filepath.Join(dir, "paired-pool.json"), filepath.Join(dir, "prepared-pool.json")
	stateDir := configPath + ".runner-state"
	var paired runner.PoolRunnerPairingResult
	var registered ExecutionPoolRunner
	var config runner.PoolLocalConfig
	var fileSHA string
	var preparation runner.PoolConfigPreparation
	publicScopes := []map[string]string{}
	var requestsMu sync.Mutex
	requests := []map[string]any{}
	var workerMu sync.Mutex
	var workerCancel context.CancelFunc
	var workerDone chan error
	workerStarts, workerStops := 0, 0
	mode := "not_started"
	var executionRun dag.Run
	diagnostics := []string{}
	var diagnosticsMu sync.Mutex
	stopWorker := func() error {
		workerMu.Lock()
		defer workerMu.Unlock()
		if workerCancel == nil {
			return nil
		}
		workerCancel()
		select {
		case e := <-workerDone:
			if e != nil && !errors.Is(e, context.Canceled) && !errors.Is(e, context.DeadlineExceeded) {
				return e
			}
		case <-time.After(10 * time.Second):
			return fmt.Errorf("owned original PoolWorker drain unconfirmed")
		}
		workerCancel = nil
		workerDone = nil
		workerStops++
		mode = "stopped"
		return nil
	}
	startWorker := func(tasks bool) error {
		workerMu.Lock()
		defer workerMu.Unlock()
		if workerCancel != nil {
			return fmt.Errorf("drain original worker before another explicit start")
		}
		loaded, loadedSHA, e := runner.ReadPoolConfigFile(configPath)
		if e != nil || loadedSHA != fileSHA {
			return fmt.Errorf("prepared local file changed")
		}
		handle, e := runner.LoadPoolRunnerIdentity(identityPath, platformServer.PublicURL, pool.ID, paired.RunnerID, vault)
		if e != nil {
			return e
		}
		token := handle.Token()
		_ = handle.Close()
		child, close := context.WithCancel(ctx)
		w := &runner.PoolWorker{Client: &runner.Client{BaseURL: platformServer.PublicURL, Token: token, Pool: &runner.PoolContext{PoolID: pool.ID, RunnerID: paired.RunnerID}}, Config: loaded, StateDir: filepath.Join(stateDir, "pool"), AllowExec: tasks, AllowInstalledToolDiscovery: !tasks, MaintenanceOnly: !tasks, PollInterval: 100 * time.Millisecond, HeartbeatInterval: 250 * time.Millisecond, Diagnostic: func(v runner.PoolDiagnostic) {
			diagnosticsMu.Lock()
			if len(diagnostics) < 64 {
				diagnostics = append(diagnostics, v.Code)
			}
			diagnosticsMu.Unlock()
		}}
		workerCancel = close
		workerDone = make(chan error, 1)
		done := workerDone
		workerStarts++
		mode = "maintenance"
		if tasks {
			mode = "tasks"
		}
		go func() { done <- w.Run(child) }()
		return nil
	}
	facts := func() (map[string]any, error) {
		current, e := f.s.GetExecutionPoolRunnerReadiness(ctx, f.actor, pool.ID, paired.RunnerID, f.user.TenantID, f.project)
		if e != nil {
			return nil, e
		}
		workerMu.Lock()
		currentMode, starts, stops, run := mode, workerStarts, workerStops, executionRun
		workerMu.Unlock()
		value := map[string]any{"mode": currentMode, "worker_starts": starts, "worker_stops": stops, "source_current": current.Scope.SourceCurrent, "source_reason": current.Scope.SourceReason, "inventory_item_count": current.Scope.InventoryItemCount, "historical_last_seen_at": current.Runner.HistoricalLastSeenAt, "run_id": run.ID, "task_state": "not_created", "output": ""}
		for key, table := range map[string]string{"attempt_count": "dag_attempts", "profile_count": "suma_tool_profiles", "run_count": "dag_runs"} {
			var n int
			if e = f.s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); e != nil {
				return nil, e
			}
			value[key] = n
		}
		if run.ID != "" {
			run, e = f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
			if e != nil {
				return nil, e
			}
			value["task_state"] = run.State
			if len(run.Nodes) == 1 {
				value["output"] = run.Nodes[0].Output
				value["node_state"] = run.Nodes[0].State
				value["worker_id"] = run.Nodes[0].WorkerID
				value["fence"] = run.Nodes[0].Fence
			}
		}
		if _, e = os.Stat(stateDir); os.IsNotExist(e) {
			value["journal_exists"] = false
		} else if e == nil {
			value["journal_exists"] = true
		} else {
			return nil, e
		}
		return value, nil
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__fixture/view" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"session": session, "pool": pool, "runner": registered, "grants": grants, "project_id": f.project, "tenant_id": f.user.TenantID, "worker_id": paired.WorkerID, "pool_id": pool.ID, "runner_id": paired.RunnerID, "pool_config_file": configPath, "identity_file": identityPath, "workspace": publicScopes[0]["workspace"], "commands_file": publicScopes[0]["commands_file"], "scopes": publicScopes})
			return
		}
		if r.URL.Path == "/__fixture/action" {
			if r.Method != "POST" || r.Header.Get("X-Suma-Fixture-Key") != actionKey {
				w.WriteHeader(403)
				return
			}
			var input struct {
				Action string `json:"action"`
			}
			if json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input) != nil {
				w.WriteHeader(400)
				return
			}
			var e error
			switch input.Action {
			case "start_maintenance":
				e = startWorker(false)
			case "stop_worker":
				e = stopWorker()
			case "start_tasks":
				if e = stopWorker(); e == nil {
					workerMu.Lock()
					already := executionRun.ID != ""
					workerMu.Unlock()
					if already {
						e = fmt.Errorf("owned execution already created")
					} else {
						var workflow dag.Workflow
						workflow, e = f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Owned local data verification212", Nodes: []dag.Node{{ID: "verify", Name: "Verify owned project data", Type: "command", Command: []string{"owned-verify"}, TimeoutSeconds: 10, MaxRetries: 0}}})
						if e == nil {
							var run dag.Run
							run, e = f.e.CreateRun(ctx, f.user.TenantID, f.project, workflow.ID, newID())
							if e == nil {
								workerMu.Lock()
								executionRun = run
								workerMu.Unlock()
								e = startWorker(true)
							}
						}
					}
				}
			case "facts":
			default:
				w.WriteHeader(400)
				return
			}
			if e != nil {
				w.WriteHeader(500)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "owned fixture action failed"})
				return
			}
			value, e := facts()
			if e != nil {
				w.WriteHeader(500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(value)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			platformHTTP.ServeHTTP(w, r)
			return
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		if e != nil {
			w.WriteHeader(400)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		response := httptest.NewRecorder()
		platformHTTP.ServeHTTP(response, r)
		requestsMu.Lock()
		requests = append(requests, map[string]any{"method": r.Method, "path": r.URL.Path, "status": response.Code, "request_sha256": digest(string(raw))})
		requestsMu.Unlock()
		for name, values := range response.Header() {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	})
	host := httptest.NewUnstartedServer(handler)
	platformServer.PublicURL = "http://" + host.Listener.Addr().String()
	host.Start()
	defer host.Close()
	defer func() {
		if e := stopWorker(); e != nil {
			t.Error(e)
		}
	}()
	receipt["origin"] = platformServer.PublicURL
	pairing, e := f.s.CreatePoolRunnerPairing(ctx, f.actor, PoolRunnerPairingCreateRequest{PoolID: pool.ID, Name: "Owned Pool212 machine", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()}, platformServer.PublicURL)
	if e != nil {
		t.Fatal(e)
	}
	machineDone := make(chan error, 1)
	proposed := make(chan struct{}, 1)
	go func() {
		var e error
		paired, e = runner.PairPoolRunner(ctx, runner.PoolRunnerPairingOptions{PairingID: pairing.ID, ControlURL: platformServer.PublicURL, PoolID: pool.ID, IdentityPath: identityPath, DeviceName: "Owned connection212", Secrets: vault, Observe: func(v runner.PoolRunnerPairingResult) error {
			if v.State == "proposed" {
				select {
				case proposed <- struct{}{}:
				default:
				}
			}
			return nil
		}})
		machineDone <- e
	}()
	select {
	case <-proposed:
	case e := <-machineDone:
		t.Fatal("original machine proposal failed", e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	current, e := f.s.GetPoolRunnerPairing(ctx, f.actor, pool.ID, pairing.ID, platformServer.PublicURL)
	if e != nil {
		t.Fatal(e)
	}
	approved, e := f.s.CommandPoolRunnerPairing(ctx, f.actor, pairing.ID, PoolRunnerPairingCommandRequest{PoolID: pool.ID, Action: "approve", ExpectedVersion: current.Version, IdempotencyKey: newID()}, platformServer.PublicURL)
	if e != nil {
		t.Fatal(e)
	}
	select {
	case e = <-machineDone:
		if e != nil {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !paired.Prepared || paired.RunnerID != approved.RunnerID {
		t.Fatal("original paired identity not prepared")
	}
	tx, e := f.s.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	registered, e = f.s.poolRunnerTx(ctx, tx, pool.ID, paired.RunnerID)
	_ = tx.Rollback()
	if e != nil {
		t.Fatal(e)
	}
	script := filepath.Join(dir, "owned-verify.cjs")
	source := `'use strict';const fs=require('node:fs'),path=require('node:path'),crypto=require('node:crypto');const data=fs.readFileSync(path.join(process.cwd(),'owned-project-data.json'));const value=JSON.parse(data);if(value.stage!=='T-POOL-CONNECTION-212'||value.project_id!==process.argv[2]||process.env.SUMA_POOL_RUNNER_TOKEN||process.env.SUMA_RUNNER_TOKEN)process.exit(61);console.log(JSON.stringify({kind:'owned-pool212-verification',project_id:value.project_id,evidence_sha256:crypto.createHash('sha256').update(data).digest('hex'),pid:process.pid,passed:true}));`
	if e = os.WriteFile(script, []byte(source), 0600); e != nil {
		t.Fatal(e)
	}
	scopes := []runner.PoolConfigPreparationScope{}
	for i, project := range projects {
		workspace := filepath.Join(dir, fmt.Sprintf("owned-project-%d", i+1))
		if e = os.MkdirAll(workspace, 0700); e != nil {
			t.Fatal(e)
		}
		data, _ := json.Marshal(map[string]string{"stage": "T-POOL-CONNECTION-212", "project_id": project})
		if e = os.WriteFile(filepath.Join(workspace, "owned-project-data.json"), data, 0600); e != nil {
			t.Fatal(e)
		}
		commandsFile := filepath.Join(dir, fmt.Sprintf("owned-commands-%d.json", i+1))
		commands, _ := json.Marshal(map[string]runner.CommandSpec{"owned-verify": {Executable: node, Args: []string{script, project}, ReadOnly: true}})
		if e = os.WriteFile(commandsFile, commands, 0600); e != nil {
			t.Fatal(e)
		}
		scopes = append(scopes, runner.PoolConfigPreparationScope{TenantID: f.user.TenantID, ProjectID: project, Workspace: workspace, CommandsFile: commandsFile})
		publicScopes = append(publicScopes, map[string]string{"tenant_id": f.user.TenantID, "project_id": project, "workspace": workspace, "commands_file": commandsFile})
	}
	preparationInput, _ := json.Marshal(runner.PoolConfigPreparationInput{PoolID: pool.ID, RunnerID: paired.RunnerID, Slots: 1, Scopes: scopes})
	preparation, e = runner.PreparePoolLocalConfig(ctx, configPath, bytes.NewReader(preparationInput))
	if e != nil {
		t.Fatal(e)
	}
	config, fileSHA, e = runner.ReadPoolConfigFile(configPath)
	if e != nil || fileSHA != preparation.ConfigurationSHA256 || len(config.Scopes) != 2 {
		t.Fatal("original prepared config file not loaded", e)
	}
	rawIdentity, e := os.ReadFile(identityPath)
	if e != nil {
		t.Fatal(e)
	}
	handle, e := runner.LoadPoolRunnerIdentity(identityPath, platformServer.PublicURL, pool.ID, paired.RunnerID, vault)
	if e != nil {
		t.Fatal(e)
	}
	secret := handle.Token()
	_ = handle.Close()
	if bytes.Contains(rawIdentity, []byte(secret)) {
		t.Fatal("identity file exposed secret")
	}
	secret = ""
	before, e := facts()
	if e != nil || before["historical_last_seen_at"] != "" || before["journal_exists"] != false || before["run_count"] != 0 {
		t.Fatal("preparation/pairing started a Worker or advanced last_seen", e)
	}
	receipt["prepared_configuration_sha256"], receipt["paired_identity_sha256"] = fileSHA, digest(string(rawIdentity))
	input, _ := json.Marshal(map[string]any{"protocol": "suma-pool-connection-actual-v1", "origin": platformServer.PublicURL, "directory": dir, "project": f.project, "tenant_id": f.user.TenantID, "pool_id": pool.ID, "runner_id": paired.RunnerID, "worker_id": paired.WorkerID, "cookie": map[string]string{"name": f.cookie.Name, "value": f.cookie.Value}, "action_key": actionKey, "runner_executable": compiled, "identity_file": identityPath, "pool_config_file": configPath, "workspace": scopes[0].Workspace, "commands_file": scopes[0].CommandsFile})
	driver := exec.CommandContext(ctx, node, filepath.Join(root, "tests", "integration", "pool-connection", "driver.cjs"))
	driver.Dir = root
	driver.Stdin = bytes.NewReader(input)
	output, e := driver.CombinedOutput()
	_ = os.WriteFile(filepath.Join(dir, "driver.log"), output, 0600)
	if e != nil {
		t.Fatalf("isolated actual browser journey failed; see %s", filepath.Join(dir, "driver.log"))
	}
	// Bind the receipt to the actual driver observations, not assumptions made
	// by this Go host about how many commands or native processes it launched.
	driverRaw, e := os.ReadFile(filepath.Join(dir, "result.json"))
	if e != nil || len(driverRaw) > 2<<20 {
		t.Fatal("bounded actual driver receipt unavailable")
	}
	var driverResult struct {
		Passed             bool `json:"passed"`
		CompiledCheckCalls int  `json:"compiled_check_calls"`
		NativePID          int  `json:"native_pid"`
		NativeExit         struct {
			Code   *int    `json:"code"`
			Signal *string `json:"signal"`
		} `json:"native_exit"`
	}
	if json.Unmarshal(driverRaw, &driverResult) != nil || !driverResult.Passed || driverResult.CompiledCheckCalls != 1 || driverResult.NativePID < 1 || driverResult.NativeExit.Code == nil || *driverResult.NativeExit.Code != 0 || driverResult.NativeExit.Signal != nil && *driverResult.NativeExit.Signal != "" {
		t.Fatal("actual driver did not confirm one compiled check and normal native exit")
	}
	receipt["driver_result_sha256"], receipt["native_pid"], receipt["native_exit"] = digest(string(driverRaw)), driverResult.NativePID, driverResult.NativeExit
	if e = stopWorker(); e != nil {
		t.Fatal(e)
	}
	final, e := facts()
	if e != nil {
		t.Fatal(e)
	}
	if final["task_state"] != "succeeded" || final["node_state"] != "succeeded" || final["worker_id"] != paired.WorkerID || final["attempt_count"] != 1 || final["run_count"] != 1 || final["profile_count"] != 0 || !strings.Contains(fmt.Sprint(final["output"]), "owned-pool212-verification") {
		t.Fatal("real original Pool command did not complete", final)
	}
	if workerStarts != 2 || workerStops != 2 {
		t.Fatal("maintenance/task modes not separately started and drained")
	}
	afterBytes, e := os.ReadFile(configPath)
	if e != nil || digest(string(afterBytes)) != fileSHA {
		t.Fatal("actual changed original prepared file")
	}
	requestsMu.Lock()
	receipt["requests"] = append([]map[string]any(nil), requests...)
	requestsMu.Unlock()
	diagnosticsMu.Lock()
	receipt["worker_diagnostics"] = append([]string(nil), diagnostics...)
	diagnosticsMu.Unlock()
	receipt["pool_id"], receipt["runner_id"], receipt["worker_id"], receipt["final_facts"] = pool.ID, paired.RunnerID, paired.WorkerID, final
	receipt["compiled_check_calls"], receipt["library_worker_starts"], receipt["library_worker_stops"] = driverResult.CompiledCheckCalls, workerStarts, workerStops
	receipt["model_calls"], receipt["vendor_calls"], receipt["installation_scan_requests"], receipt["code_candidates"], receipt["human_acceptance"] = 0, 0, domainACount(t, f.s, "suma_tool_installation_scans"), 0, false
	receipt["prepared_file_preserved"], receipt["passed"] = true, true
	t.Log("one original system pairing/configuration, compiled check and two explicit original library Worker modes; one owned read-only command on current Pool route, no vendor/model/formal acceptance")
}
