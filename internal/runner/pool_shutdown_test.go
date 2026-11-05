package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// The worker must actually stop this child. It publishes a start marker and
// otherwise stays alive for longer than the fixture's normal shutdown wait.
func TestPoolShutdownProcessFixture(t *testing.T) {
	marker := os.Getenv("SUMA_POOL_SHUTDOWN_FIXTURE_MARKER")
	if marker == "" {
		return
	}
	file, err := os.OpenFile(marker, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil || file.Close() != nil {
		os.Exit(71)
	}
	time.Sleep(30 * time.Second)
	os.Exit(72)
}

func TestPoolShutdownJoinsOriginalSlotsAndPreservesOutbox(t *testing.T) {
	for _, tc := range []struct {
		name    string
		unknown bool
	}{
		{name: "both_processes_stop"},
		{name: "agent_outcome_unconfirmed", unknown: true},
	} {
		t.Run(tc.name, func(t *testing.T) { checkPoolShutdownSlots(t, tc.unknown) })
	}
}

func checkPoolShutdownSlots(t *testing.T, unknownAgent bool) {
	t.Helper()
	root := t.TempDir()
	state := filepath.Join(root, "pool")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	slots := []string{"0", "1"}
	assignments := make(map[string]Assignment, len(slots))
	markers := make(map[string]string, len(slots))
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: "pool", RunnerID: "runner", Slots: len(slots)}
	for i, slot := range slots {
		assignment := poolTestAssignment()
		assignment.PoolSlotID, assignment.PoolGrantID = slot, "grant-"+slot
		assignment.TenantID, assignment.ProjectID = "tenant-"+slot, "project-"+slot
		assignment.RunID, assignment.DispatchID = "run-"+slot, "dispatch-"+slot
		assignment.Attempt, assignment.Fence = 3+i, int64(17+i)
		assignment.Type, assignment.Command = "command", []string{"fixture"}
		scope := PoolLocalScope{TenantID: assignment.TenantID, ProjectID: assignment.ProjectID, Workspace: t.TempDir()}
		marker := filepath.Join(root, "started-"+slot)
		command := CommandSpec{Executable: exe, Args: []string{"-test.run=^TestPoolShutdownProcessFixture$"}, ReadOnly: true, Env: map[string]string{"SUMA_POOL_SHUTDOWN_FIXTURE_MARKER": marker}}
		if unknownAgent && slot == "1" {
			scope.Repository, scope.BaseCommit = gitFixture(t)
			scope.Workspace = scope.Repository
			command = acpRunnerSpec()
			command.Executable, command.ACP.ReviewedVersion, command.ACP.Model = exe, "Qoder fixture protocol only 170", "performance"
			assignment.Type = "agent"
		} else {
			markers[slot] = marker
		}
		scope.Commands = map[string]CommandSpec{"fixture": command}
		config.Scopes = append(config.Scopes, scope)
		assignments[slot] = assignment
	}
	var mu sync.Mutex
	claimed := make(map[string]bool)
	inventories := make(map[string][]tc.Observation)
	callbacks := make(map[string][][]byte)
	var stopping, agentStarted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		write := func(value any) { _ = json.NewEncoder(w).Encode(value) }
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		const prefix = "/api/v1/pool-runner/"
		switch strings.TrimPrefix(r.URL.Path, prefix) {
		case "session":
			_, _ = io.WriteString(w, `{"runner":{"id":"runner","pool_id":"pool","worker_id":"pool-runner","max_slots":2,"credential_version":1},"pool_active":true,"scopes":[{"tenant_id":"tenant-0","project_id":"project-0"},{"tenant_id":"tenant-1","project_id":"project-1"}]}`)
		case "tool-inspections/claim":
			_, _ = io.WriteString(w, `{"inspection":null}`)
		case "tool-inventory":
			var report struct {
				InventoryReport
				ProjectID string `json:"project_id"`
			}
			if err := json.Unmarshal(raw, &report); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			inventories[report.ProjectID] = report.Items
			mu.Unlock()
			write(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339), "maintenance": report.Maintenance})
		case "claim":
			if stopping.Load() {
				t.Error("pool attempted a new claim after host cancellation")
			}
			var request PoolClaimRequest
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			assignment, exists := assignments[request.SlotID]
			for _, scope := range request.Scopes {
				if !exists || claimed[request.SlotID] || scope.ProjectID != assignment.ProjectID || scope.TenantID != assignment.TenantID {
					continue
				}
				if assignment.Type == "agent" {
					for _, item := range inventories[scope.ProjectID] {
						if item.Available && item.Profile.CommandID == "fixture" && item.Profile.ACPExecution != nil {
							assignment.Config = map[string]any{"goal": "cancel", "tool_profile_id": "fixture", "tool_profile_sha256": item.Profile.ProfileSHA256, "acp_execution": item.Profile.ACPExecution}
						}
					}
					if assignment.Config == nil {
						continue
					}
				}
				claimed[request.SlotID] = true
				write(map[string]any{"assignment": assignment})
				return
			}
			_, _ = io.WriteString(w, `{"assignment":null}`)
		case "heartbeat":
			_, _ = io.WriteString(w, `{"cancel_requested":false}`)
		case "progress":
			var progress Progress
			if err := json.Unmarshal(raw, &progress); err != nil {
				t.Error(err)
			}
			if progress.Assignment.PoolSlotID == "1" && progress.ToolEvent != nil && strings.Contains(progress.ToolEvent.Message, "cancel-ready") {
				agentStarted.Store(true)
			}
			_, _ = io.WriteString(w, `{"acknowledged":true}`)
		case "results":
			var body struct {
				Assignment Assignment `json:"assignment"`
				Result     Result     `json:"result"`
			}
			if err := json.Unmarshal(raw, &body); err != nil || !samePoolAssignmentIdentity(body.Assignment, assignments[body.Assignment.PoolSlotID]) || body.Result.OperationID == "" {
				t.Errorf("result callback lost original pool identity: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			slot := body.Assignment.PoolSlotID
			callbacks[slot] = append(callbacks[slot], raw)
			if len(callbacks[slot]) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"acknowledged":true}`)
		default:
			t.Errorf("unexpected pool route: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	client := &Client{BaseURL: server.URL, Token: "fixture-pool-token", Pool: &PoolContext{PoolID: "pool", RunnerID: "runner"}}
	var diagnostics []PoolDiagnostic
	var diagnosticMu sync.Mutex
	worker := PoolWorker{Client: client, Config: config, StateDir: state, AllowExec: true, PollInterval: 20 * time.Millisecond, HeartbeatInterval: 50 * time.Millisecond, Diagnostic: func(d PoolDiagnostic) {
		if d.Code == "slot_shutdown_unconfirmed" {
			diagnosticMu.Lock()
			diagnostics = append(diagnostics, d)
			diagnosticMu.Unlock()
		}
	}}
	done := make(chan struct{})
	var runErr error
	go func() { runErr = worker.Run(ctx); close(done) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("pool worker did not release its fixture after cancellation")
		}
	})
	started := func() bool {
		for _, marker := range markers {
			if _, err := os.Stat(marker); err != nil {
				return false
			}
		}
		return !unknownAgent || agentStarted.Load()
	}
	for !started() {
		select {
		case <-done:
			t.Fatal("pool shut down before both original processes started", runErr)
		case <-ctx.Done():
			t.Fatal("both original processes did not reach the running barrier")
		case <-time.After(10 * time.Millisecond):
		}
	}
	stopping.Store(true)
	cancel()
	select {
	case <-done:
	case <-time.After(9 * time.Second):
		t.Fatal("pool did not finish joining original slots")
	}
	if unknownAgent {
		if runErr == nil || errors.Is(runErr, context.Canceled) || len(diagnostics) != 1 || diagnostics[0] != (PoolDiagnostic{Code: "slot_shutdown_unconfirmed", SlotID: "1"}) {
			t.Errorf("unknown agent execution lacked original slot verdict: %v (%+v)", runErr, diagnostics)
		}
	} else if runErr != nil || len(diagnostics) != 0 {
		t.Errorf("two durable known stops did not allow clean shutdown: %v (%+v)", runErr, diagnostics)
	}
	unlock, err := lockJournal(filepath.Join(state, "pool.lock"))
	if err != nil {
		t.Fatalf("pool returned before releasing machine ownership: %v", err)
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	for _, scope := range config.Scopes {
		inventory, err := OpenJournal(filepath.Join(state, "inventories", scopeKey(scope.TenantID, scope.ProjectID)))
		if err != nil {
			t.Fatalf("pool returned before releasing original inventory ownership: %v", err)
		}
		if err = inventory.Close(); err != nil {
			t.Fatal(err)
		}
	}
	for _, slot := range slots {
		journal, err := OpenJournal(filepath.Join(state, "slots", slot))
		if err != nil {
			t.Fatalf("pool returned before releasing slot %s: %v", slot, err)
		}
		defer journal.Close()
		pending := journal.Pending()
		wantUnknown := unknownAgent && slot == "1"
		if len(pending) != 1 || !samePoolAssignmentIdentity(pending[0].Assignment, assignments[slot]) || pending[0].OperationID == "" || !pending[0].Stopped || pending[0].OutcomeUnknown != wantUnknown || pending[0].ExitCode == 0 {
			t.Fatalf("slot %s did not preserve original stopped attempt: %+v", slot, pending)
		}
		if err = replayPool(context.Background(), client, journal); err == nil || len(journal.Pending()) != 1 {
			t.Fatalf("slot %s lost pending callback after dropped acknowledgement: %v", slot, err)
		}
		if err = replayPool(context.Background(), client, journal); err != nil || len(journal.Pending()) != 0 {
			t.Fatalf("slot %s did not acknowledge the retried original callback: %v", slot, err)
		}
		mu.Lock()
		calls := callbacks[slot]
		mu.Unlock()
		if len(calls) != 2 || !bytes.Equal(calls[0], calls[1]) || !bytes.Contains(calls[0], []byte(pending[0].OperationID)) {
			t.Errorf("slot %s retried with different bytes or operation", slot)
		}
	}
}
