package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
)

// Each role has a real credential, durable journal and disjoint command list.
// Source tests must execute on the tester rather than borrow a package proof
// or another producer's same-commit test.
type releaseFixtureWorker struct {
	ID      string
	Client  *runner.Client
	Journal *runner.Journal
	Exec    runner.ExecConfig
}

func releaseFixtureServer(t *testing.T, f *fixture) *httptest.Server {
	t.Helper()
	var activity sync.Mutex
	active := map[int]string{}
	next := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		activity.Lock()
		next++
		id := next
		active[id] = r.URL.Path
		activity.Unlock()
		defer func() { activity.Lock(); delete(active, id); activity.Unlock() }()
		f.handler.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		server.Close()
		activity.Lock()
		defer activity.Unlock()
		stats := f.s.DB.Stats()
		t.Logf("release fixture HTTP closed: active=%v DB=%+v", active, stats)
		if len(active) != 0 || stats.InUse != 0 {
			stack := make([]byte, 128<<10)
			t.Logf("release fixture outstanding work: %s", stack[:runtime.Stack(stack, true)])
		}
	})
	return server
}

func runReleaseFixtureWorkers(t *testing.T, ctx context.Context, f *fixture, runID string, workers []releaseFixtureWorker, expectedWorkers map[string]string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	var running sync.WaitGroup
	errorsByWorker := make([]error, len(workers))
	stopped := make(chan string, len(workers))
	for i, config := range workers {
		running.Add(1)
		go func() {
			defer running.Done()
			worker := runner.Worker{Client: config.Client, Journal: config.Journal, ID: config.ID, Exec: config.Exec, PollInterval: 10 * time.Millisecond}
			errorsByWorker[i] = worker.Run(ctx)
			stopped <- config.ID
		}()
	}
	defer func() {
		cancel()
		running.Wait()
		t.Logf("release fixture Workers stopped: DB=%+v", f.s.DB.Stats())
		for i, err := range errorsByWorker {
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("release fixture worker %s: %v", workers[i].ID, err)
			}
		}
	}()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	var current dag.Run
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("release fixture workers: %v; run=%+v", ctx.Err(), current)
		case id := <-stopped:
			t.Fatalf("release fixture worker %s stopped before all result ACKs", id)
		case <-poll.C:
		}
		var err error
		current, err = f.e.GetRun(ctx, f.user.TenantID, f.project, runID)
		if err != nil {
			t.Fatal(err)
		}
		if current.State == "failed" || current.State == "cancelled" || current.State == "reconciling" {
			t.Fatalf("actual release fixture failed: %+v", current)
		}
		acked := true
		for _, worker := range workers {
			acked = acked && len(worker.Journal.Pending()) == 0 && len(worker.Journal.PendingProgress()) == 0
		}
		if current.State != "succeeded" || !acked {
			continue
		}
		if len(current.Nodes) != len(expectedWorkers) {
			t.Fatalf("release fixture has %d nodes, want %d", len(current.Nodes), len(expectedWorkers))
		}
		for _, node := range current.Nodes {
			if expectedWorkers[node.ID] == "" || node.State != "succeeded" || node.Attempt != 1 || node.Fence != 1 || node.WorkerID != expectedWorkers[node.ID] {
				t.Fatalf("release fixture changed exact role assignment: %+v", node)
			}
		}
		return
	}
}

// Real candidate fixtures use zero for the production Worker's default 5s
// heartbeat. Only the short-lease regression overrides the interval.
func runReleaseFixtureWorker(t *testing.T, ctx context.Context, f *fixture, client *runner.Client, journal *runner.Journal, workerID, runID string, config runner.ExecConfig, heartbeatInterval time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	worker := runner.Worker{Client: client, Journal: journal, ID: workerID, Exec: config, PollInterval: 10 * time.Millisecond, HeartbeatInterval: heartbeatInterval}
	done := make(chan struct{})
	var workerErr error
	go func() {
		workerErr = worker.Run(ctx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
		if workerErr != nil && !errors.Is(workerErr, context.Canceled) && !errors.Is(workerErr, context.DeadlineExceeded) {
			t.Error(workerErr)
		}
	}()
	poll := time.NewTicker(20 * time.Millisecond)
	defer poll.Stop()
	var current dag.Run
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("release fixture worker: %v; run=%+v pending=%+v progress=%d", ctx.Err(), current, journal.Pending(), len(journal.PendingProgress()))
		case <-done:
			t.Fatalf("release fixture worker stopped before result ACK: %v", workerErr)
		case <-poll.C:
		}
		var err error
		current, err = f.e.GetRun(ctx, f.user.TenantID, f.project, runID)
		if err != nil {
			t.Fatal(err)
		}
		if current.State == "failed" || current.State == "cancelled" || current.State == "reconciling" {
			t.Fatalf("actual release fixture failed: %+v", current)
		}
		// A committed run may precede the HTTP response and durable ACK. Keep
		// the real Worker alive until both the result and progress are ACKed.
		if current.State == "succeeded" && len(journal.Pending()) == 0 && len(journal.PendingProgress()) == 0 {
			for _, node := range current.Nodes {
				if node.State != "succeeded" || node.Attempt != 1 || node.Fence != 1 || node.WorkerID != workerID {
					t.Fatalf("release fixture changed assignment: %+v", node)
				}
			}
			return
		}
	}
}

func TestReleaseLeaseFixtureProcess(t *testing.T) {
	if os.Getenv("SUMA_RELEASE_LEASE_FIXTURE") == "" {
		return
	}
	time.Sleep(2200 * time.Millisecond)
	fmt.Println("actual command outlived the original lease")
	os.Exit(0)
}

func TestReleaseFixtureWorkerRenewsActualHTTPLease(t *testing.T) {
	f := newFixture(t)
	f.e.LeaseSeconds = 2
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	credential := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "slow-release-fixture", "profiles": []any{}, "deployment": false}), 201)
	var heartbeats atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/runner/heartbeat") {
			heartbeats.Add(1)
		}
		f.handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	client := &runner.Client{BaseURL: server.URL, Token: credential["token"].(string), ProjectID: f.project}
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "real command across a short lease", "nodes": []dag.Node{{ID: "slow", Type: "command", Command: []string{"slow"}}}}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"]}), 201)
	journal, err := runner.OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config := runner.ExecConfig{AllowExec: true, Workspace: t.TempDir(), Journal: journal, Commands: map[string]runner.CommandSpec{"slow": {Executable: executable, Args: []string{"-test.run=^TestReleaseLeaseFixtureProcess$", "--"}, ReadOnly: true, Env: map[string]string{"SUMA_RELEASE_LEASE_FIXTURE": "1"}}}}
	runReleaseFixtureWorker(t, ctx, f, client, journal, "slow-release-fixture", run["id"].(string), config, 100*time.Millisecond)
	if heartbeats.Load() < 2 {
		t.Fatalf("real command completed without repeated HTTP heartbeats: %d", heartbeats.Load())
	}
}
