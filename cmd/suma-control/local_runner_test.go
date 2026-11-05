package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func TestDesktopManagedRunnerStopReceiptRequiresRequestAndCleanExit(t *testing.T) {
	configuration := runner.DesktopManagedRunnerInput{Protocol: runner.DesktopManagedRunnerProtocol, ControlURL: "http://127.0.0.1:4319", ProjectID: strings.Repeat("1", 32), WorkerID: "desktop-" + strings.Repeat("2", 32), AdapterID: "codex-jsonl", Repository: t.TempDir(), BaseCommit: strings.Repeat("a", 40), StateDirectory: t.TempDir(), Token: strings.Repeat("b", 64)}
	for _, tc := range []struct {
		name, frame string
		workerErr   error
		wantReceipt bool
		maxStartup  bool
	}{
		{name: "confirmed", frame: "stop\n", wantReceipt: true},
		{name: "confirmed after maximal startup frame", frame: "stop\n", wantReceipt: true, maxStartup: true},
		{name: "parent loss"},
		{name: "invalid frame", frame: "quit\n"},
		{name: "unresolved cleanup", frame: "stop\n", workerErr: errors.New(configuration.Token)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			app := commandlocale.New("suma-control", "control", "control", &out, &stderr, func(string) (string, bool) { return "", false })
			input, writer := io.Pipe()
			defer input.Close()
			defer writer.Close()
			started := make(chan struct{})
			done := make(chan int, 1)
			go func() {
				done <- runDesktopManagedRunnerWith(app, input, func(ctx context.Context, got runner.DesktopManagedRunnerInput) error {
					if got != configuration {
						t.Error("private startup configuration changed")
					}
					close(started)
					<-ctx.Done()
					return tc.workerErr
				})
			}()
			raw, _ := json.Marshal(configuration)
			if tc.maxStartup {
				raw = append(raw, bytes.Repeat([]byte(" "), runner.DesktopManagedInputLimit-len(raw)-1)...)
			}
			if _, err := writer.Write(append(raw, '\n')); err != nil {
				t.Fatal(err)
			}
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("private startup did not arrive")
			}
			select {
			case <-done:
				t.Fatal("child stopped before the parent acted")
			default:
			}
			if tc.frame == "" {
				_ = writer.Close()
			} else if _, err := io.WriteString(writer, tc.frame); err != nil {
				t.Fatal(err)
			}
			select {
			case code := <-done:
				if (code != 0) != (tc.workerErr != nil) {
					t.Fatalf("unexpected exit %d", code)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("parent action did not cancel execution")
			}
			if tc.wantReceipt {
				var receipt desktopRunnerShutdownReceipt
				if err := json.Unmarshal(out.Bytes(), &receipt); err != nil || strings.Count(out.String(), "\n") != 1 || receipt != (desktopRunnerShutdownReceipt{Type: "suma.local_runner.shutdown", WorkerID: configuration.WorkerID, PID: os.Getpid()}) {
					t.Fatalf("invalid stop receipt %q", out.String())
				}
			} else if out.Len() != 0 {
				t.Fatal("unexpected stop receipt")
			}
			if strings.Contains(out.String()+stderr.String(), configuration.Token) {
				t.Fatal("private configuration escaped to output")
			}
		})
	}
}

func TestDesktopManagedRunnerRejectsBadInputBeforeAnyExecution(t *testing.T) {
	var out, stderr bytes.Buffer
	app := commandlocale.New("suma-control", "control", "control", &out, &stderr, func(string) (string, bool) { return "", false })
	for _, raw := range []string{`{"token":"private-user-value"}` + "\n", strings.Repeat("x", runner.DesktopManagedInputLimit+1) + "\n"} {
		called := false
		if code := runDesktopManagedRunnerWith(app, strings.NewReader(raw), func(context.Context, runner.DesktopManagedRunnerInput) error { called = true; return nil }); code != 1 || called {
			t.Fatal("invalid configuration reached execution")
		}
	}
	if strings.Contains(stderr.String(), "private-user-value") {
		t.Fatal("input secret echoed")
	}
}

func TestDesktopManagedModeRejectsMixedServiceOptionsBeforeState(t *testing.T) {
	for _, args := range [][]string{{"--desktop-managed-runner", "--maintenance"}, {"--desktop-managed-runner", "--discover-installed-tools"}, {"--desktop-managed-runner", "--local-tool", "codex-jsonl"}} {
		var out, stderr bytes.Buffer
		code := run(args, &out, &stderr, func(name string) (string, bool) {
			if name == "SUMA_DATA_DIR" || strings.HasPrefix(name, "SUMA_DB_") || name == "SUMA_RUNNER_TOKEN" {
				t.Fatalf("mode read business state %s", name)
			}
			return "", false
		})
		if code != 1 {
			t.Fatalf("exit=%d %s", code, stderr.String())
		}
	}
}
