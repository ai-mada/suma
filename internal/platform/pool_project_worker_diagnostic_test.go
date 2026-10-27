package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
)

// This diagnostic uses the real legacy worker and HTTP handler. Only route,
// status, node state and durable outbox sizes are recorded, never credentials.
func TestProjectWorkerInventoryClaimResultDiagnostic(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	credential := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "legacy-diagnostic", "profiles": []any{}, "deployment": false}), 201)
	var mu sync.Mutex
	var trace []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		trace = append(trace, "start "+r.URL.Path)
		mu.Unlock()
		rec := httptest.NewRecorder()
		f.handler.ServeHTTP(rec, r)
		mu.Lock()
		trace = append(trace, fmt.Sprintf("end %s %d", r.URL.Path, rec.Code))
		mu.Unlock()
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	}))
	defer server.Close()
	client := &runner.Client{BaseURL: server.URL, Token: credential["token"].(string), ProjectID: f.project}
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "legacy actual worker diagnostic", "nodes": []dag.Node{{ID: "local", Type: "command", Command: []string{"local"}}}}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": wf["id"]}), 201)
	journal, err := runner.OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	w := runner.Worker{Client: client, Journal: journal, ID: "legacy-diagnostic", PollInterval: 10 * time.Millisecond, HeartbeatInterval: 100 * time.Millisecond, Exec: runner.ExecConfig{AllowExec: true, Workspace: t.TempDir(), Journal: journal, Commands: map[string]runner.CommandSpec{"local": {Executable: exe, Args: []string{"-test.run=^TestReleaseLeaseFixtureProcess$", "--"}, ReadOnly: true}}}}
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()
	defer func() { cancel(); <-done }()
	var last dag.Run
	for ctx.Err() == nil {
		last, err = f.e.GetRun(ctx, f.user.TenantID, f.project, run["id"].(string))
		if err != nil {
			break
		}
		if last.State == "succeeded" && len(journal.Pending()) == 0 && len(journal.PendingProgress()) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	t.Fatalf("state=%s nodes=%+v pending=%d progress=%d inventory_pending=%t err=%v routes=%v", last.State, last.Nodes, len(journal.Pending()), len(journal.PendingProgress()), journal.PendingInventory() != nil, err, trace)
}
