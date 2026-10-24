package platform

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
)

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
	credential := f.call("POST", "/auth/tokens", f.body(map[string]any{"label": "release fixture lease", "scope": "runner"}), 201)
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
