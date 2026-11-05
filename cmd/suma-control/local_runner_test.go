package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func TestDesktopManagedRunnerStopReceiptRequiresRequestAndCleanExit(t *testing.T) {
	operation := strings.Repeat("c", 32)
	frame := "stop " + operation + "\n"
	const prior = `{"type":"suma.local_runner.shutdown","worker_id":"desktop-22222222222222222222222222222222","pid":1,"operation_id":"dddddddddddddddddddddddddddddddd"}`
	for _, tc := range []struct {
		name, frame    string
		workerErr      error
		wantProof      bool
		maxStartup     bool
		prior          string
		markerConflict bool
		outputFailure  bool
		wantFailure    bool
	}{
		{name: "confirmed", frame: frame, wantProof: true},
		{name: "confirmed after maximal startup frame", frame: frame, wantProof: true, maxStartup: true},
		{name: "replaces older generation", frame: frame, wantProof: true, prior: prior},
		{name: "replaces malformed marker", frame: frame, wantProof: true, prior: `{"type":"suma.local_runner.shutdown",`},
		{name: "lost stdout retains proof", frame: frame, wantProof: true, outputFailure: true, wantFailure: true},
		{name: "parent loss"},
		{name: "no operation", frame: "stop\n"},
		{name: "invalid operation", frame: "stop " + strings.ToUpper(operation) + "\n"},
		{name: "unresolved cleanup", frame: frame, workerErr: errors.New(strings.Repeat("b", 64)), wantFailure: true},
		{name: "marker collision", frame: frame, markerConflict: true, wantFailure: true},
		{name: "refuses same operation", frame: frame, prior: strings.ReplaceAll(prior, strings.Repeat("d", 32), operation), wantFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			stateDirectory := filepath.Join(directory, "runtime")
			if err := os.Mkdir(stateDirectory, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(directory, "managed-shutdown.json")
			if tc.prior != "" {
				if err := os.WriteFile(marker, []byte(tc.prior), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.markerConflict {
				if err := os.Mkdir(marker, 0700); err != nil {
					t.Fatal(err)
				}
			}
			configuration := runner.DesktopManagedRunnerInput{Protocol: runner.DesktopManagedRunnerProtocol, ControlURL: "http://127.0.0.1:4319", ProjectID: strings.Repeat("1", 32), WorkerID: "desktop-" + strings.Repeat("2", 32), AdapterID: "codex-jsonl", Repository: directory, BaseCommit: strings.Repeat("a", 40), StateDirectory: stateDirectory, Token: strings.Repeat("b", 64)}
			var out, stderr bytes.Buffer
			var stdout io.Writer = &out
			if tc.outputFailure {
				reader, writer := io.Pipe()
				_ = reader.Close()
				defer writer.Close()
				stdout = writer
			}
			app := commandlocale.New("suma-control", "control", "control", stdout, &stderr, func(string) (string, bool) { return "", false })
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
				if (code != 0) != tc.wantFailure {
					t.Fatalf("unexpected exit %d", code)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("parent action did not cancel execution")
			}
			proof, proofErr := os.ReadFile(marker)
			if tc.wantProof {
				want := map[string]any{"type": "suma.local_runner.shutdown", "worker_id": configuration.WorkerID, "pid": os.Getpid(), "operation_id": operation}
				wantJSON, _ := json.Marshal(want)
				var got map[string]json.RawMessage
				if proofErr != nil || json.Unmarshal(proof, &got) != nil || bytes.Count(proof, []byte("\n")) != 1 {
					t.Fatalf("invalid persisted stop proof: %v", proofErr)
				}
				gotJSON, _ := json.Marshal(got)
				if !bytes.Equal(wantJSON, gotJSON) {
					t.Fatal("persisted stop proof differs from the requested process generation")
				}
				if !tc.outputFailure && !bytes.Equal(proof, out.Bytes()) {
					t.Fatal("persisted stop proof and receipt bytes differ")
				}
			} else if tc.prior != "" && string(proof) != tc.prior {
				t.Fatal("existing stop proof changed after conflict")
			} else if !tc.markerConflict && tc.prior == "" && !os.IsNotExist(proofErr) {
				t.Fatalf("unexpected persisted stop proof: %v", proofErr)
			}
			if (!tc.wantProof || tc.outputFailure) && out.Len() != 0 {
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
