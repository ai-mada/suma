package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	term "github.com/ai-mada/suma/internal/terminalcontract"
)

func TestTerminalExchangeReplaysExactOriginalAfterAcknowledgementLoss(t *testing.T) {
	var first []byte
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/terminal-sessions/original-session/exchange" || r.Header.Get("Authorization") != "Bearer original-runner" {
			t.Errorf("original scope missing")
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var request term.ExchangeRequest
		if err = json.Unmarshal(raw, &request); err != nil {
			t.Error(err)
			return
		}
		calls++
		if calls == 1 {
			first = raw
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = connection.Close()
			return
		}
		if !bytes.Equal(first, raw) || request.OperationID != "original-exchange-operation" {
			t.Error("lost acknowledgement created different request bytes or operation")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(term.ExchangeResponse{SessionID: request.SessionID, ControlEpoch: 1, Columns: 100, Rows: 28, Inputs: []term.Input{}, OutputAckSequence: 1})
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "original-runner", ProjectID: "original-project"}
	response, err := terminalExchangeOriginal(context.Background(), client, term.ExchangeRequest{SessionID: "original-session", RunID: "original-run", NodeID: "terminal", Attempt: 1, Fence: 2, WorkerID: "original-worker", DispatchID: "original-dispatch", OperationID: "original-exchange-operation", State: "running", OutputChunks: []term.OutputChunk{{Sequence: 1, DataBase64: "eA=="}}, InputReceipts: []term.InputReceipt{}})
	if err != nil || calls != 2 || response.OutputAckSequence != 1 {
		t.Fatalf("original replay: calls=%d response=%+v error=%v", calls, response, err)
	}
}

func TestTerminalEnvironmentKeepsCredentialsAndUserHomesOutsideShell(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "runner-secret-never-forwarded")
	t.Setenv("OPENAI_API_KEY", "provider-secret-never-forwarded")
	t.Setenv("HOME", "original-user-home")
	t.Setenv("USERPROFILE", "original-user-profile")
	root := t.TempDir()
	env, err := terminalEnvironment(CommandSpec{}, root)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, secret := range []string{"runner-secret-never-forwarded", "provider-secret-never-forwarded", "original-user-home", "original-user-profile"} {
		if strings.Contains(joined, secret) {
			t.Fatalf("terminal inherited %q", secret)
		}
	}
	for _, name := range []string{"terminal-home", "terminal-tmp"} {
		if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.IsDir() {
			t.Fatalf("isolated %s: %v", name, err)
		}
	}
}
