package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestToolInventoryLostAckKeepsOperationAcrossRestart(t *testing.T) {
	received := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/tool-inventory" || r.Header.Get("Authorization") != "Bearer runner-token" {
			t.Error("wrong inventory endpoint/auth")
		}
		var body struct {
			ProjectID   string           `json:"project_id"`
			OperationID string           `json:"operation_id"`
			WorkerID    string           `json:"worker_id"`
			Items       []tc.Observation `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.ProjectID != "p" || body.WorkerID != "worker" || len(body.Items) != 1 {
			t.Error("lost inventory")
		}
		received = append(received, body.OperationID)
		_, _ = w.Write([]byte(`{"acknowledged":true,"observed_at":"2026-10-24T03:00:00Z"}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	items := []tc.Observation{{Profile: structuredProfile("suma-jsonl-v1"), Available: true}}
	report, err := j.NewInventory(server.URL, "p", "worker", items)
	if err != nil {
		t.Fatal(err)
	}
	old := report.OperationID
	if _, err = j.NewInventory(server.URL, "p", "worker", items); err == nil {
		t.Fatal("pending operation overwritten")
	}
	_ = j.Close()
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if p := j.PendingInventory(); p == nil || p.OperationID != old {
		t.Fatal("inventory replay identity lost")
	}
	c := Client{BaseURL: server.URL, Token: "runner-token", ProjectID: "p"}
	observed, err := c.ToolInventory(context.Background(), *j.PendingInventory())
	if err != nil {
		t.Fatal(err)
	}
	if err = j.AckInventory(old, observed); err != nil {
		t.Fatal(err)
	}
	fresh, err := j.NewInventory(server.URL, "p", "worker", items)
	if err != nil || fresh.OperationID == old {
		t.Fatal("new TTL report reused old receipt")
	}
	if len(received) != 1 || received[0] != old {
		t.Fatal(received)
	}
}

func TestStructuredToolProgressAndEvidenceRoundTrip(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := Assignment{RunID: "r", NodeID: "a", Attempt: 1, Fence: 2, WorkerID: "w"}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	event := tc.Event{SchemaVersion: 1, CommandID: "agent", ProfileSHA256: strings.Repeat("a", 64), Protocol: "suma-jsonl-v1", Kind: "session_started", SessionID: "session-1"}
	if err = j.RecordToolProgress(a, event); err != nil {
		t.Fatal(err)
	}
	if len(j.PendingProgress()) != 1 || j.PendingProgress()[0].ToolEvent == nil {
		t.Fatal("typed progress not durable")
	}
	evidence := &tc.Evidence{Profile: structuredProfile("suma-jsonl-v1"), Terminal: "completed", SessionID: "session-1", EventCount: 1, UsageState: tc.UsageUnreported}
	if err = j.Finish(Result{Assignment: a, ExitCode: 0, ToolEvidence: evidence}); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(j.Pending()[0])
	var got struct {
		ToolEvidence *tc.Evidence `json:"tool_evidence"`
	}
	_ = json.Unmarshal(b, &got)
	if got.ToolEvidence == nil || got.ToolEvidence.SessionID != "session-1" {
		t.Fatal("typed final evidence dropped")
	}
}

func TestWorkerClaimsOnlyAvailableCommandCapabilities(t *testing.T) {
	exe, _ := os.Executable()
	seenClaim := false
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runner/tool-inspections/claim":
			_, _ = w.Write([]byte(`{"inspection":null}`))
		case "/api/v1/runner/tool-inventory":
			fixtureToolInventoryAck195(w, r, "2026-10-24T03:00:00Z")
		case "/api/v1/runner/claim":
			var body struct {
				Capabilities []string `json:"capabilities"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			commands := []string{}
			for _, capability := range body.Capabilities {
				if strings.HasPrefix(capability, "command:") {
					commands = append(commands, capability)
				}
			}
			if len(commands) != 1 || commands[0] != "command:local" {
				t.Errorf("unsupported claim capabilities: %v", body.Capabilities)
			}
			seenClaim = true
			cancel()
			_, _ = w.Write([]byte(`{"assignment":null}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	j, _ := OpenJournal(t.TempDir())
	defer j.Close()
	w := Worker{Client: &Client{BaseURL: server.URL, Token: "registered-worker", ProjectID: "p"}, Journal: j, ID: "worker", Exec: ExecConfig{AllowExec: true, Commands: map[string]CommandSpec{"local": {Executable: exe, ReadOnly: true}, "missing": {Executable: "suma-nonexistent-binary", ReadOnly: true}}}}
	_ = w.Run(ctx)
	if !seenClaim {
		t.Fatal("no actual claim")
	}
}

func TestInvalidToolConfigurationRejectsInsteadOfDroppingEntries(t *testing.T) {
	exe, _ := os.Executable()
	for name, commands := range map[string]map[string]CommandSpec{"bad-id": {"bad id": {Executable: exe}}, "bad-protocol": {"valid": {Executable: exe, Adapter: "codex", Protocol: "text-v1"}}, "too-many": {}} {
		t.Run(name, func(t *testing.T) {
			if name == "too-many" {
				for i := 0; i < 129; i++ {
					commands[fmt.Sprintf("command-%d", i)] = CommandSpec{Executable: exe}
				}
			}
			items, err := DescribeTools(context.Background(), ExecConfig{Commands: commands})
			if err == nil || items != nil {
				t.Fatalf("invalid config silently accepted %d %v", len(items), err)
			}
		})
	}
}

func TestInventoryOutageKeepsCurrentProcessAndHeartbeatThenRecovers(t *testing.T) {
	exe, _ := os.Executable()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var inventories, claims, beats atomic.Int32
	var received atomic.Bool
	ops := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runner/tool-inspections/claim":
			_, _ = w.Write([]byte(`{"inspection":null}`))
		case "/api/v1/runner/tool-inventory":
			var body struct {
				OperationID string          `json:"operation_id"`
				Maintenance json.RawMessage `json:"maintenance"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ops = append(ops, body.OperationID)
			n := inventories.Add(1)
			if n > 1 && !received.Load() {
				w.WriteHeader(503)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"acknowledged": true, "observed_at": time.Now().UTC().Format(time.RFC3339), "maintenance": body.Maintenance})
		case "/api/v1/runner/claim":
			n := claims.Add(1)
			if n == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"assignment": Assignment{RunID: "r", NodeID: "n", Attempt: 1, Fence: 1, WorkerID: "w", Type: "command", Command: []string{"local"}, DispatchID: strings.Repeat("d", 32), DispatchState: "active", LeaseUntil: time.Now().Add(time.Minute).UTC().Format(time.RFC3339)}})
			} else {
				if !received.Load() {
					t.Error("new attempt claimed during inventory outage")
				}
				cancel()
				_, _ = w.Write([]byte(`{"assignment":null}`))
			}
		case "/api/v1/runner/heartbeat":
			beats.Add(1)
			_, _ = w.Write([]byte(`{"cancel_requested":false}`))
		case "/api/v1/runner/progress":
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		case "/api/v1/runner/results":
			var body struct {
				Result Result `json:"result"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Result.ExitCode != 0 || body.Result.Stopped || body.Result.OutcomeUnknown {
				t.Errorf("inventory error killed healthy execution: %+v", body.Result)
			}
			received.Store(true)
			_, _ = w.Write([]byte(`{"acknowledged":true}`))
		default:
			t.Errorf("unexpected worker request %s", r.URL.Path)
		}
	}))
	defer server.Close()
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	w := Worker{Client: &Client{BaseURL: server.URL, Token: "fixture-registered-worker", ProjectID: "p"}, Journal: j, ID: "w", PollInterval: 5 * time.Millisecond, HeartbeatInterval: 10 * time.Millisecond, inventoryInterval: 20 * time.Millisecond, Exec: ExecConfig{AllowExec: true, Workspace: t.TempDir(), Secrets: []string{"private-secret"}, Commands: map[string]CommandSpec{"local": {Executable: exe, Args: []string{"-test.run=TestProgressOutputHelper", "--"}, ReadOnly: true, Env: map[string]string{"SUMA_PROGRESS_HELPER": "long"}}}}}
	_ = w.Run(ctx)
	if !received.Load() || claims.Load() != 2 || beats.Load() < 2 || inventories.Load() < 4 {
		t.Fatalf("outage recovery missing: result=%v claims=%d beats=%d inventories=%d", received.Load(), claims.Load(), beats.Load(), inventories.Load())
	}
	if len(ops) < 4 || ops[1] != ops[2] || ops[len(ops)-1] == ops[1] {
		t.Fatalf("pending report operation was not recovered before fresh TTL report: %v", ops)
	}
}
