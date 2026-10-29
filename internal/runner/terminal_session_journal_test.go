package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	term "github.com/ai-mada/suma/internal/terminalcontract"
	"github.com/ai-mada/suma/internal/terminalruntime"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestTerminalStoppedExchangeSurvivesRunnerRestartBeforeResult(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "original-run", NodeID: "terminal", Type: "terminal_session", Attempt: 1, Fence: 4, WorkerID: "original-worker", DispatchID: "original-dispatch", Config: map[string]any{"terminal_session_id": "original-session"}}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	request := term.ExchangeRequest{SessionID: "original-session", RunID: a.RunID, NodeID: a.NodeID, Attempt: 1, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, OperationID: "original-stopped-operation", State: "stopped", Directory: "original-worktree", ProcessID: 130, TreeStopped: true, ExitCode: 1, OutputChunks: []term.OutputChunk{}, InputReceipts: []term.InputReceipt{}}
	if err = j.checkpointTerminalExchange(a, request); err != nil {
		t.Fatal(err)
	}
	if err = j.Finish(Result{Assignment: a, ExitCode: 1, Stopped: true, OperationID: "original-final-result"}); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var order []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runner/terminal-sessions/original-session/exchange":
			var got term.ExchangeRequest
			if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
				t.Error(err)
				return
			}
			if stringifyTerminalTest(got) != stringifyTerminalTest(request) {
				t.Error("restart replaced original stopped exchange")
			}
			order = append(order, "stop")
			_ = json.NewEncoder(w).Encode(term.ExchangeResponse{SessionID: request.SessionID, ControlEpoch: 1, Columns: 100, Rows: 28, Inputs: []term.Input{}})
		case "/api/v1/runner/results":
			order = append(order, "result")
		default:
			t.Errorf("unexpected recovery path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, ProjectID: "project", Token: "runner"}
	if err = Replay(context.Background(), c, j); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "stop" || order[1] != "result" || len(j.Pending()) != 0 {
		t.Fatal("result overtook original stop receipt", order)
	}
	if err = Replay(context.Background(), c, j); err != nil || len(order) != 2 {
		t.Fatal("acknowledged recovery replayed", order, err)
	}
}

func stringifyTerminalTest(v any) string { raw, _ := json.Marshal(v); return string(raw) }

func TestTerminalExchangeCheckpointRejectsForeignAttemptAndChangedOperation(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	a := Assignment{RunID: "run", NodeID: "terminal", Type: "terminal_session", Attempt: 1, Fence: 4, WorkerID: "worker", DispatchID: "dispatch", Config: map[string]any{"terminal_session_id": "session"}}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	r := term.ExchangeRequest{SessionID: "session", RunID: a.RunID, NodeID: a.NodeID, Attempt: 1, Fence: a.Fence, WorkerID: a.WorkerID, DispatchID: a.DispatchID, OperationID: "fixed-operation", State: "running", OutputChunks: []term.OutputChunk{}, InputReceipts: []term.InputReceipt{}}
	foreign := r
	foreign.Fence++
	if err = j.checkpointTerminalExchange(a, foreign); err == nil {
		t.Fatal("foreign fence checkpoint accepted")
	}
	if err = j.checkpointTerminalExchange(a, r); err != nil {
		t.Fatal(err)
	}
	if err = j.checkpointTerminalExchange(a, r); err != nil {
		t.Fatal(err)
	}
	r.TreeStopped = true
	r.State = "stopped"
	if err = j.checkpointTerminalExchange(a, r); err == nil {
		t.Fatal("same operation changed original bytes")
	}
}

func TestTerminalRestartImportsOnlyOriginalDurableStopAndLeavesEffectUnknown(t *testing.T) {
	for _, terminalState := range []string{"", "stopped", "exited"} {
		t.Run(map[string]string{"": "missing_stop", "stopped": "durable_stop", "exited": "durable_exit"}[terminalState], func(t *testing.T) {
			stopped := terminalState != ""
			dir := t.TempDir()
			journalDir := filepath.Join(dir, "runner")
			runtimeDir := filepath.Join(dir, "attempt", "terminal-journal")
			source := filepath.Join(dir, "attempt", "src")
			if err := os.MkdirAll(runtimeDir, 0700); err != nil {
				t.Fatal(err)
			}
			a := Assignment{RunID: "run", NodeID: "terminal", Type: "terminal_session", Attempt: 1, Fence: 4, WorkerID: "worker", DispatchID: "dispatch", Config: map[string]any{"terminal_session_id": "session"}}
			profile := tc.Profile{CommandID: "shell", Adapter: "command", Protocol: "text-v1", ToolVersion: "fixture", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), TerminalSessionProtocol: term.Protocol}
			j, err := OpenJournal(journalDir)
			if err != nil {
				t.Fatal(err)
			}
			if err = j.Accept(a); err != nil {
				t.Fatal(err)
			}
			if err = j.checkpointTerminalRuntime(a, runtimeDir, source, profile); err != nil {
				t.Fatal(err)
			}
			snapshot := terminalruntime.Snapshot{ID: "session", State: "starting", WorkDir: source, ProcessID: 130, Columns: 100, Rows: 28}
			write := func(name string, value any) {
				raw, _ := json.Marshal(value)
				if e := os.WriteFile(filepath.Join(runtimeDir, name), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			write("session.json", snapshot)
			if stopped {
				snapshot.TreeStopped = true
				snapshot.State = terminalState
				exit := 1
				snapshot.ExitCode = &exit
				write("terminal.json", snapshot)
			}
			if err = j.Close(); err != nil {
				t.Fatal(err)
			}
			j, err = OpenJournal(journalDir)
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			pending := j.Pending()
			if len(pending) != 1 || !pending[0].OutcomeUnknown {
				t.Fatal("restart certified interrupted effects", pending)
			}
			exchanges := j.pendingTerminalExchanges()
			if !stopped {
				if len(exchanges) != 0 || pending[0].Stopped {
					t.Fatal("stored PID became stop proof", exchanges, pending)
				}
				return
			}
			if len(exchanges) != 1 || !exchanges[0].Request.TreeStopped || exchanges[0].Request.ProcessID != 130 || exchanges[0].Request.DispatchID != a.DispatchID || exchanges[0].Request.State != terminalState || pending[0].ToolEvidence == nil || !pending[0].Stopped {
				t.Fatal("original durable stop receipt lost", exchanges, pending)
			}
		})
	}
}

func TestTerminalReviewedRecoveryDisposesExpiredResultWithoutClaimingServerResultAck(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "run", NodeID: "terminal", Type: "terminal_session", Attempt: 1, Fence: 4, WorkerID: "worker", DispatchID: "dispatch", Config: map[string]any{"terminal_session_id": "session"}}
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	request := term.ExchangeRequest{SessionID: "session", RunID: "run", NodeID: "terminal", Attempt: 1, Fence: 4, WorkerID: "worker", DispatchID: "dispatch", OperationID: "original-stop", State: "exited", Directory: "original-source", ProcessID: 130, TreeStopped: true, OutputChunks: []term.OutputChunk{}, InputReceipts: []term.InputReceipt{}}
	if err = j.checkpointTerminalExchange(a, request); err != nil {
		t.Fatal(err)
	}
	if err = j.Finish(Result{Assignment: a, OperationID: "original-expired-result", ExitCode: 0, OutcomeUnknown: true}); err != nil {
		t.Fatal(err)
	}
	reviewed := false
	exchanges, results := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/runner/results" {
			results++
			w.WriteHeader(409)
			return
		}
		var got term.ExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
			return
		}
		if stringifyTerminalTest(got) != stringifyTerminalTest(request) {
			t.Error("recovery changed original operation")
		}
		exchanges++
		reply := term.ExchangeResponse{SessionID: "session", ControlEpoch: 1, Columns: 100, Rows: 28, Inputs: []term.Input{}, StopRequested: true}
		if reviewed {
			reply.RecoveryOperationID = "explicit-original-reviewed-operation"
		}
		_ = json.NewEncoder(w).Encode(reply)
	}))
	defer server.Close()
	c := &Client{BaseURL: server.URL, Token: "runner", ProjectID: "project"}
	if err = Replay(context.Background(), c, j); err == nil || len(j.Pending()) != 1 {
		t.Fatal("expired result was falsely acknowledged", err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	reviewed = true
	if err = Replay(context.Background(), c, j); err != nil || len(j.Pending()) != 0 || exchanges != 2 || results != 1 {
		t.Fatal("explicit recovery did not dispose original pending result", err, exchanges, results)
	}
	e := j.entries[key(a)]
	if e.Acked || e.Result.OperationID != "original-expired-result" || !e.Result.OutcomeUnknown || e.Terminal.ReconciliationOperationID != "explicit-original-reviewed-operation" {
		t.Fatal("recovery rewrote original result or fabricated its server acknowledgement", e)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}
	j, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = Replay(context.Background(), c, j); err != nil || exchanges != 2 || results != 1 {
		t.Fatal("restart lost explicit reconciliation disposition", err, exchanges, results)
	}
}
