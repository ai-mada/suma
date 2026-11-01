package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestActualWorkerClaimLostACKAndClosedDispatchNeverStart(t *testing.T) {
	for _, state := range []string{"active", "closed", "reconciling"} {
		t.Run(state, func(t *testing.T) {
			node, err := exec.LookPath("node")
			if err != nil {
				t.Skip("Node unavailable for local execution marker fixture")
			}
			dir := t.TempDir()
			entry, marker := filepath.Join(dir, "claim-fixture.js"), filepath.Join(dir, "execution-marker.txt")
			if err = os.WriteFile(entry, []byte(`require('node:fs').appendFileSync(process.argv[2],'executed\n');console.log('claim fixture only');`), 0600); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var mu sync.Mutex
			var operations []string
			claims, results := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/runner/tool-inspections/claim":
					_, _ = w.Write([]byte(`{"inspection":null}`))
				case "/api/v1/runner/tool-inventory":
					fixtureToolInventoryAck195(w, r, time.Now().UTC().Format(time.RFC3339))
				case "/api/v1/runner/claim":
					var body map[string]any
					_ = json.NewDecoder(r.Body).Decode(&body)
					operation, _ := body["idempotency_key"].(string)
					mu.Lock()
					operations = append(operations, operation)
					claims++
					n := claims
					mu.Unlock()
					if n == 1 {
						conn, _, e := w.(http.Hijacker).Hijack()
						if e != nil {
							t.Error(e)
							return
						}
						_ = conn.Close()
						return
					}
					if n > 2 {
						_ = json.NewEncoder(w).Encode(map[string]any{"assignment": nil})
						cancel()
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"assignment": map[string]any{"run_id": "r", "node_id": "n", "attempt": 1, "fence": 1, "worker_id": "w", "type": "command", "command": []string{"local"}, "dispatch_id": strings.Repeat("d", 32), "dispatch_state": state, "lease_until": time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}})
				case "/api/v1/runner/results":
					mu.Lock()
					results++
					mu.Unlock()
					_, _ = w.Write([]byte(`{"acknowledged":true}`))
					cancel()
				case "/api/v1/runner/progress":
					_, _ = w.Write([]byte(`{"acknowledged":true}`))
				case "/api/v1/runner/heartbeat":
					_, _ = w.Write([]byte(`{"cancel_requested":false}`))
				default:
					t.Errorf("unexpected fixture path %s", r.URL.Path)
				}
			}))
			defer server.Close()
			journal, err := OpenJournal(filepath.Join(dir, "journal"))
			if err != nil {
				t.Fatal(err)
			}
			defer journal.Close()
			worker := Worker{Client: &Client{BaseURL: server.URL, ProjectID: "p", Token: "fixture-registered-worker"}, Journal: journal, ID: "w", PollInterval: 10 * time.Millisecond, HeartbeatInterval: 50 * time.Millisecond, Exec: ExecConfig{AllowExec: true, Workspace: dir, Commands: map[string]CommandSpec{"local": {Executable: node, Args: []string{entry, marker}, ReadOnly: true}}}}
			_ = worker.Run(ctx)
			mu.Lock()
			defer mu.Unlock()
			if len(operations) < 2 || operations[0] == "" || operations[0] != operations[1] {
				t.Fatalf("lost ACK changed or omitted original operation: %v", operations)
			}
			raw, readErr := os.ReadFile(marker)
			if state == "active" {
				if readErr != nil || string(raw) != "executed\n" || results != 1 {
					t.Fatalf("active dispatch did not execute once: %q results=%d %v", raw, results, readErr)
				}
			} else if !os.IsNotExist(readErr) || results != 0 {
				t.Fatalf("%s dispatch executed after its lease observation closed: %q results=%d", state, raw, results)
			}
		})
	}
}

func TestClaimCheckpointRestartRestoresOriginalRequestBeforeAcceptAndAfterResponse(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	first, err := j.BeginClaim("http://127.0.0.1:4322", "p", "w", []string{"command:one"})
	if err != nil {
		t.Fatal(err)
	}
	if first.OperationID == "" {
		t.Fatal("operation not persisted")
	}
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	again, err := j.BeginClaim("http://127.0.0.1:4322/", "p", "w", []string{"command:changed"})
	if err != nil {
		t.Fatal(err)
	}
	if again.OperationID != first.OperationID || len(again.Capabilities) != 1 || again.Capabilities[0] != "command:one" {
		t.Fatal("restart changed unresolved request identity or capability fingerprint")
	}
	if _, err = j.BeginClaim("http://127.0.0.1:4323", "p", "w", nil); err == nil {
		t.Fatal("pending claim replayed to another control")
	}
	if _, err = j.BeginClaim("http://127.0.0.1:4322", "foreign", "w", nil); err == nil {
		t.Fatal("pending claim replayed to another project")
	}
	a := &Assignment{RunID: "r", NodeID: "n", Attempt: 1, Fence: 1, WorkerID: "w", DispatchID: strings.Repeat("d", 32), DispatchState: "closed", LeaseUntil: time.Now().UTC().Format(time.RFC3339)}
	if err = j.RecordClaimResponse(first.OperationID, a); err != nil {
		t.Fatal(err)
	}
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	response, err := j.BeginClaim("http://127.0.0.1:4322", "p", "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.OperationID != first.OperationID || !response.Responded || response.Assignment == nil || response.Assignment.DispatchState != "closed" {
		t.Fatal("durable response lost before pending request completion")
	}
	if len(j.Pending()) != 0 {
		t.Fatal("closed response created an execution inbox")
	}
	if err = j.CompleteClaim(first.OperationID); err != nil {
		t.Fatal(err)
	}
	next, err := j.BeginClaim("http://127.0.0.1:4322", "p", "w", []string{"command:new"})
	if err != nil {
		t.Fatal(err)
	}
	if next.OperationID == first.OperationID {
		t.Fatal("new poll reused a resolved operation")
	}
}

func TestClaimCheckpointCannotClearBeforeDurableResponseOrReplayInvalidState(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	c, err := j.BeginClaim("http://127.0.0.1:4322", "p", "w", nil)
	if err != nil {
		t.Fatal(err)
	}
	if j.CompleteClaim(c.OperationID) == nil {
		t.Fatal("pending operation cleared without response")
	}
	a := &Assignment{RunID: "r", NodeID: "n", Attempt: 1, Fence: 1, WorkerID: "w", DispatchID: "dispatch", DispatchState: "invented", LeaseUntil: time.Now().UTC().Format(time.RFC3339)}
	if j.RecordClaimResponse(c.OperationID, a) == nil {
		t.Fatal("unrecognized dispatch state persisted")
	}
	if err = j.RecordClaimResponse(c.OperationID, nil); err != nil {
		t.Fatal(err)
	}
	if err = j.CompleteClaim(c.OperationID); err != nil {
		t.Fatal(err)
	}
}
