package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPoolParallelProcess(t *testing.T) {
	if os.Getenv("SUMA_POOL_TEST_MARKER") == "" {
		return
	}
	if os.Getenv("SUMA_POOL_RUNNER_TOKEN") != "" {
		os.Exit(71)
	}
	wd, _ := os.Getwd()
	if wd != os.Getenv("SUMA_POOL_TEST_WORKSPACE") {
		os.Exit(72)
	}
	if os.WriteFile(os.Getenv("SUMA_POOL_TEST_STARTED"), []byte("actual process started"), 0600) != nil {
		os.Exit(73)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, e := os.Stat(os.Getenv("SUMA_POOL_TEST_OTHER")); e == nil {
			fmt.Println("actual independent project", os.Getenv("SUMA_POOL_TEST_MARKER"))
			os.Exit(0)
		}
		time.Sleep(10 * time.Millisecond)
	}
	os.Exit(74)
}

func TestPoolWorkerRunsTwoRealProjectProcessesInIndependentSlots(t *testing.T) {
	root := t.TempDir()
	exe, _ := os.Executable()
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: "pool", RunnerID: "runner", Slots: 2}
	for i := 0; i < 2; i++ {
		workspace := filepath.Join(root, "project-"+strconv.Itoa(i))
		if err := os.Mkdir(workspace, 0700); err != nil {
			t.Fatal(err)
		}
		config.Scopes = append(config.Scopes, PoolLocalScope{TenantID: "tenant-" + strconv.Itoa(i), ProjectID: "project-" + strconv.Itoa(i), Workspace: workspace, Commands: map[string]CommandSpec{"inspect": {Executable: exe, Args: []string{"-test.run=^TestPoolParallelProcess$"}, ReadOnly: true, Env: map[string]string{"SUMA_POOL_TEST_MARKER": strconv.Itoa(i), "SUMA_POOL_TEST_WORKSPACE": workspace, "SUMA_POOL_TEST_STARTED": filepath.Join(root, "started-"+strconv.Itoa(i)), "SUMA_POOL_TEST_OTHER": filepath.Join(root, "started-"+strconv.Itoa(1-i))}}}})
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var mu sync.Mutex
	claimed := map[string]bool{}
	finished := map[string]bool{}
	all := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/pool-runner/session" {
			json.NewEncoder(w).Encode(map[string]any{"runner": map[string]any{"id": "runner", "pool_id": "pool", "worker_id": "pool-runner", "max_slots": 2, "credential_version": 1}, "pool_active": true, "scopes": []any{map[string]string{"tenant_id": "tenant-0", "project_id": "project-0"}, map[string]string{"tenant_id": "tenant-1", "project_id": "project-1"}}})
			return
		}
		var body map[string]json.RawMessage
		if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
			t.Error(e)
			w.WriteHeader(400)
			return
		}
		switch r.URL.Path {
		case "/api/v1/pool-runner/tool-inspections/claim":
			_, _ = w.Write([]byte(`{"inspection":null}`))
		case "/api/v1/pool-runner/tool-inventory":
			json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339)})
		case "/api/v1/pool-runner/claim":
			var req PoolClaimRequest
			raw, _ := json.Marshal(body)
			json.Unmarshal(raw, &req)
			mu.Lock()
			defer mu.Unlock()
			if claimed[req.SlotID] {
				w.Write([]byte(`{"assignment":null}`))
				return
			}
			claimed[req.SlotID] = true
			a := poolTestAssignment()
			a.PoolSlotID = req.SlotID
			a.TenantID = "tenant-" + req.SlotID
			a.ProjectID = "project-" + req.SlotID
			a.PoolGrantID = "grant-" + req.SlotID
			a.RunID = "run-" + req.SlotID
			a.DispatchID = "dispatch-" + req.SlotID
			a.Command = []string{"inspect"}
			a.Type = "command"
			json.NewEncoder(w).Encode(map[string]any{"assignment": a})
		case "/api/v1/pool-runner/results":
			var a Assignment
			var result Result
			json.Unmarshal(body["assignment"], &a)
			json.Unmarshal(body["result"], &result)
			if result.ExitCode != 0 || a.TenantID != "tenant-"+a.PoolSlotID || a.ProjectID != "project-"+a.PoolSlotID {
				t.Errorf("process or callback isolation failed: slot=%s exit=%d", a.PoolSlotID, result.ExitCode)
			}
			mu.Lock()
			finished[a.PoolSlotID] = true
			if len(finished) == 2 {
				select {
				case <-all:
				default:
					close(all)
				}
			}
			mu.Unlock()
			w.Write([]byte(`{"acknowledged":true}`))
		case "/api/v1/pool-runner/progress":
			w.Write([]byte(`{"acknowledged":true}`))
		case "/api/v1/pool-runner/heartbeat":
			w.Write([]byte(`{"cancel_requested":false}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	t.Setenv("SUMA_POOL_RUNNER_TOKEN", "fixture-machine-secret-never-child")
	worker := PoolWorker{Client: &Client{BaseURL: server.URL, Token: "fixture-machine-secret-never-child", Pool: &PoolContext{PoolID: "pool", RunnerID: "runner"}}, Config: config, StateDir: filepath.Join(root, "state"), AllowExec: true, PollInterval: 10 * time.Millisecond, HeartbeatInterval: 40 * time.Millisecond}
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	select {
	case <-all:
		cancel()
	case err := <-done:
		t.Fatal("worker stopped before both actual processes completed", err)
	case <-ctx.Done():
		t.Fatal("two slots did not run concurrently")
	}
	<-done
}

func TestPoolClaimCheckpointReusesOriginalScopeAndOperation(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	req := PoolClaimRequest{PoolID: "pool", RunnerID: "runner", WorkerID: "pool-runner", SlotID: "0", Scopes: []PoolCapabilityScope{{TenantID: "tenant", ProjectID: "project", Capabilities: []string{"command:a"}}}}
	c, err := beginPoolClaim(j, "http://127.0.0.1:4310", "config", 1, req)
	if err != nil {
		t.Fatal(err)
	}
	req.Scopes[0].Capabilities = []string{"command:changed"}
	again, err := beginPoolClaim(j, "http://127.0.0.1:4310", "config", 1, req)
	if err != nil || again.Request.OperationID != c.Request.OperationID || again.Request.Scopes[0].Capabilities[0] != "command:a" {
		t.Fatal("pending request was replaced", err)
	}
	if _, err = beginPoolClaim(j, "http://127.0.0.1:4311", "config", 1, req); err == nil {
		t.Fatal("pending operation crossed control origin")
	}
}

func TestPoolConfigRejectsCredentialForwardingAndUnscopedArtifactPrivileges(t *testing.T) {
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: "pool", RunnerID: "runner", Slots: 1, Scopes: []PoolLocalScope{{TenantID: "tenant", ProjectID: "project", Workspace: ".", Commands: map[string]CommandSpec{"run": {Executable: "fixture", ReadOnly: true, EnvFrom: []string{"SUMA_POOL_RUNNER_TOKEN"}}}}}}
	if validatePoolLocalConfig(config) == nil {
		t.Fatal("machine secret forwarding accepted")
	}
	command := config.Scopes[0].Commands["run"]
	command.EnvFrom = nil
	command.ArtifactOutput = "dist"
	config.Scopes[0].Commands["run"] = command
	if validatePoolLocalConfig(config) == nil {
		t.Fatal("ungranted pool artifact capability accepted")
	}
}

func TestPoolResolvedConfigPinsNativeProjectAndDistinctDirectories(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: "pool", RunnerID: "runner", Slots: 1, Scopes: []PoolLocalScope{{TenantID: "a", ProjectID: "a", Workspace: a, Commands: map[string]CommandSpec{"agent": {Executable: "fixture", Adapter: "suma", Protocol: "suma-jsonl-v1", ReadOnly: true}}}, {TenantID: "b", ProjectID: "b", Workspace: b, Commands: map[string]CommandSpec{}}}}
	resolved, err := resolvedPoolConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Scopes[0].Commands["agent"].Env["SUMA_PROJECT_ID"] != "a" || config.Scopes[0].Commands["agent"].Env != nil {
		t.Fatal("native project was mutable or caller configuration was changed")
	}
	config.Scopes[1].Workspace = a
	if _, err = resolvedPoolConfig(config); err == nil {
		t.Fatal("two scopes shared a process working directory")
	}
}

func TestPoolDescribeReportsOnlyScopedNonSecretObservations(t *testing.T) {
	exe, _ := os.Executable()
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: "pool", RunnerID: "runner", Slots: 1, Scopes: []PoolLocalScope{{TenantID: "tenant", ProjectID: "project", Workspace: t.TempDir(), Commands: map[string]CommandSpec{"inspect": {Executable: exe, ReadOnly: true, Env: map[string]string{"API_KEY": "explicit-fixture-value-not-vendor"}}}}}}
	items, err := DescribePoolTools(context.Background(), config, true)
	if err != nil || len(items) != 1 || len(items[0].Items) != 1 || !items[0].Items[0].Available {
		t.Fatal("local description failed", err)
	}
	raw, _ := json.Marshal(items)
	if strings.Contains(string(raw), "explicit-fixture-value-not-vendor") || strings.Contains(string(raw), config.Scopes[0].Workspace) {
		t.Fatal("local configuration escaped public describe output")
	}
}

func TestPoolWorkerActuallyStopsProcessAfterScopedHeartbeatCancel(t *testing.T) {
	root := t.TempDir()
	exe, _ := os.Executable()
	workspace := filepath.Join(root, "workspace")
	os.Mkdir(workspace, 0700)
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: "pool", RunnerID: "runner", Slots: 1, Scopes: []PoolLocalScope{{TenantID: "tenant", ProjectID: "project", Workspace: workspace, Commands: map[string]CommandSpec{"inspect": {Executable: exe, Args: []string{"-test.run=^TestPoolParallelProcess$"}, ReadOnly: true, Env: map[string]string{"SUMA_POOL_TEST_MARKER": "stop-fixture", "SUMA_POOL_TEST_WORKSPACE": workspace, "SUMA_POOL_TEST_STARTED": filepath.Join(root, "started"), "SUMA_POOL_TEST_OTHER": filepath.Join(root, "never-created")}}}}}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	actual := make(chan Result, 1)
	var mu sync.Mutex
	claimed := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/pool-runner/session":
			w.Write([]byte(`{"runner":{"id":"runner","pool_id":"pool","worker_id":"pool-runner","max_slots":1,"credential_version":1},"pool_active":true,"scopes":[{"tenant_id":"tenant","project_id":"project"}]}`))
		case "/api/v1/pool-runner/tool-inspections/claim":
			_, _ = w.Write([]byte(`{"inspection":null}`))
		case "/api/v1/pool-runner/tool-inventory":
			json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339)})
		case "/api/v1/pool-runner/claim":
			mu.Lock()
			defer mu.Unlock()
			if claimed {
				w.Write([]byte(`{"assignment":null}`))
				return
			}
			claimed = true
			a := poolTestAssignment()
			a.Command = []string{"inspect"}
			a.Type = "command"
			json.NewEncoder(w).Encode(map[string]any{"assignment": a})
		case "/api/v1/pool-runner/heartbeat":
			_, started := os.Stat(filepath.Join(root, "started"))
			json.NewEncoder(w).Encode(map[string]bool{"cancel_requested": started == nil})
		case "/api/v1/pool-runner/progress":
			w.Write([]byte(`{"acknowledged":true}`))
		case "/api/v1/pool-runner/results":
			var body struct {
				Result     Result     `json:"result"`
				Assignment Assignment `json:"assignment"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Assignment.TenantID != "tenant" || body.Assignment.ProjectID != "project" {
				t.Error("stop receipt lost scope")
			}
			actual <- body.Result
			w.Write([]byte(`{"acknowledged":true}`))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	w := PoolWorker{Client: &Client{BaseURL: server.URL, Token: "fixture-pool-token", Pool: &PoolContext{PoolID: "pool", RunnerID: "runner"}}, Config: config, StateDir: filepath.Join(root, "state"), AllowExec: true, PollInterval: 10 * time.Millisecond, HeartbeatInterval: 40 * time.Millisecond}
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	select {
	case result := <-actual:
		if !result.Stopped || result.ExitCode == 0 {
			t.Fatalf("OS cancellation lacked an actual non-success stop receipt: stopped=%v exit=%d", result.Stopped, result.ExitCode)
		}
		cancel()
	case err := <-done:
		t.Fatal("worker ended without stop receipt", err)
	case <-ctx.Done():
		t.Fatal("actual process did not stop")
	}
	<-done
}
