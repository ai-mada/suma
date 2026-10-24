package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCrashRecoveryNeverRerunsAndReplaysSameResult(t *testing.T) {
	dir := t.TempDir()
	s, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := Assignment{RunID: "run", NodeID: "node", Attempt: 1, Fence: 7, WorkerID: "worker"}
	if err = s.Accept(a); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenJournal(dir); err == nil {
		t.Fatal("concurrent journal owner accepted")
	}
	_ = s.Close()
	s2, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	pending := s2.Pending()
	if len(pending) != 1 || !pending[0].OutcomeUnknown || pending[0].Stopped || pending[0].Error == "" {
		t.Fatalf("unsafe recovery: %+v", pending)
	}
	id := pending[0].OperationID
	_ = s2.Close()
	s3, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s3.Pending()[0].OperationID != id {
		t.Fatal("replay changed idempotency key")
	}
	if err = s3.Ack(id); err != nil {
		t.Fatal(err)
	}
	_ = s3.Close()
	s4, _ := OpenJournal(dir)
	defer s4.Close()
	if len(s4.Pending()) != 0 {
		t.Fatal("acked result replayed")
	}
	if err = s4.Accept(a); err == nil {
		t.Fatal("duplicate attempt accepted")
	}
}

func TestClientUsesBearerAndDoesNotLoseRejectedResults(t *testing.T) {
	var received Result
	reject := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("missing bearer")
		}
		if r.URL.Path != "/api/v1/runner/results" {
			t.Errorf("path %s", r.URL.Path)
		}
		var body struct {
			ProjectID string `json:"project_id"`
			Result    Result `json:"result"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		received = body.Result
		if reject {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	s, _ := OpenJournal(t.TempDir())
	defer s.Close()
	a := Assignment{RunID: "r", NodeID: "n", Attempt: 1, Fence: 1}
	_ = s.Accept(a)
	_ = s.Finish(Result{Assignment: a, ExitCode: 0})
	c := Client{BaseURL: srv.URL, Token: "secret", ProjectID: "p"}
	if err := Replay(context.Background(), &c, s); err == nil {
		t.Fatal("expected retryable error")
	}
	if len(s.Pending()) != 1 || received.OperationID == "" {
		t.Fatal("outbox result lost")
	}
	id := received.OperationID
	reject = false
	if err := Replay(context.Background(), &c, s); err != nil {
		t.Fatal(err)
	}
	if received.OperationID != id || len(s.Pending()) != 0 {
		t.Fatal("replay did not preserve identity and acknowledge")
	}
}

func TestExecutionRequiresAllowlistAndTimeoutStopsTree(t *testing.T) {
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, "orphan-wrote.txt")
	cfg := ExecConfig{AllowExec: true, Workspace: root, Commands: map[string]CommandSpec{"helper": {Executable: exe, ReadOnly: true, Args: []string{"-test.run=TestRunnerHelperProcess", "--"}, Env: map[string]string{"SUMA_TEST_HELPER": "parent", "SUMA_TEST_MARKER": marker}}}}
	a := Assignment{Type: "test", Command: []string{"helper"}, TimeoutSeconds: 1}
	r := Execute(context.Background(), a, cfg)
	if (runtime.GOOS == "windows" && !r.Stopped) || (runtime.GOOS != "windows" && (r.Stopped || !r.OutcomeUnknown)) || r.ExitCode == 0 {
		t.Fatalf("timeout did not confirm stop: %+v", r)
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("descendant survived job termination")
	}
	cfg.AllowExec = false
	r = Execute(context.Background(), a, cfg)
	if r.ExitCode == 0 {
		t.Fatal("execution without opt in")
	}
	cfg.AllowExec = true
	a.Command = []string{"unregistered"}
	r = Execute(context.Background(), a, cfg)
	if r.ExitCode == 0 {
		t.Fatal("unregistered executable ran")
	}
	cfg.Workspace = filepath.Join(root, "missing")
	a.Command = []string{"helper"}
	r = Execute(context.Background(), a, cfg)
	if r.ExitCode == 0 {
		t.Fatal("nonexistent workspace accepted")
	}
}

func TestRunnerHelperProcess(t *testing.T) {
	if os.Getenv("SUMA_TEST_HELPER") == "parent" {
		exe, _ := os.Executable()
		child := exec.Command(exe, "-test.run=TestRunnerHelperProcess", "--")
		child.Env = append(os.Environ(), "SUMA_TEST_HELPER=child")
		if err := child.Start(); err != nil {
			os.Exit(3)
		}
		_ = child.Wait()
		os.Exit(0)
	}
	if os.Getenv("SUMA_TEST_HELPER") == "child" {
		time.Sleep(3 * time.Second)
		_ = os.WriteFile(os.Getenv("SUMA_TEST_MARKER"), []byte("survived"), 0600)
		os.Exit(0)
	}
}

func TestOutputIsBoundedAndRedacted(t *testing.T) {
	b := newOutput(48, []string{"secret-token"})
	_, _ = b.Write([]byte("hello secret-token\x1b[31m\x00"))
	for range 100 {
		_, _ = b.Write([]byte("xxxxxxxx"))
	}
	out := b.String()
	if len(out) > 80 || contains(out, "secret-token") || contains(out, "\x1b") || contains(out, "\x00") {
		t.Fatalf("unsafe output %q", out)
	}
}

func TestTruncatedSecretPrefixIsNotExposed(t *testing.T) {
	b := newOutput(9, []string{"secret-token"})
	_, _ = b.Write([]byte("ok secret-token"))
	if contains(b.String(), "secret") {
		t.Fatal("truncated credential exposed")
	}
}

func TestNativeAgentReceivesGoalWithoutRemoteExecutableArguments(t *testing.T) {
	exe, _ := os.Executable()
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"agent": {Executable: exe, ReadOnly: true, Args: []string{"-test.run=TestAgentInputHelper", "--"}, Env: map[string]string{"SUMA_INPUT_HELPER": "1"}, Input: "agent-jsonl"}}}
	r := Execute(context.Background(), Assignment{Type: "agent", Command: []string{"agent"}, Config: map[string]any{"goal": "inspect project"}}, cfg)
	if r.ExitCode != 0 || !contains(r.Output, "inspect project") {
		t.Fatalf("agent input missing: %+v", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = Execute(ctx, Assignment{Type: "agent", Command: []string{"agent"}, Config: map[string]any{"goal": "inspect project"}}, cfg)
	if r.Error != "context canceled" || r.Stopped || r.Output != "" {
		t.Fatalf("cancelled assignment was started: %+v", r)
	}
}

func TestAgentInputHelper(t *testing.T) {
	if os.Getenv("SUMA_INPUT_HELPER") == "1" {
		var input map[string]any
		if json.NewDecoder(os.Stdin).Decode(&input) != nil || input["goal"] != "inspect project" {
			os.Exit(4)
		}
		requestID, _ := input["request_id"].(string)
		if len(requestID) != 64 || strings.Contains(requestID, "/") {
			os.Exit(5)
		}
		_, _ = os.Stdout.WriteString(input["goal"].(string))
		os.Exit(0)
	}
}
