package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/runner"
)

func poolDiagnosticDigest211(raw []byte) string {
	value := sha256.Sum256(raw)
	return hex.EncodeToString(value[:])
}

type poolDiagnosticBrokenOutput211 struct{ written bytes.Buffer }

func (w *poolDiagnosticBrokenOutput211) Write(raw []byte) (int, error) {
	n := len(raw)
	if n > 3 {
		n = 3
	}
	_, _ = w.written.Write(raw[:n])
	return n, errors.New("private-stdout-cause211")
}

func TestPoolPreparationFailureIsOneLocalizedSafeDiagnostic(t *testing.T) {
	for _, language := range []string{"en-US", "zh-CN"} {
		t.Run(language, func(t *testing.T) {
			directory := t.TempDir()
			first, second := filepath.Join(directory, "workspace-a"), filepath.Join(directory, "workspace-b")
			if err := os.Mkdir(first, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(second, 0700); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(directory, "private-source-secret211.json")
			if err := os.WriteFile(source, []byte(`{"private-source-content211":`), 0600); err != nil {
				t.Fatal(err)
			}
			input := runner.PoolConfigPreparationInput{PoolID: "pool211", RunnerID: "runner211", Slots: 1, Scopes: []runner.PoolConfigPreparationScope{
				{TenantID: "tenant-a", ProjectID: "project-a", Workspace: first},
				{TenantID: "tenant-b", ProjectID: "project-b", Workspace: second, CommandsFile: source},
			}}
			raw, _ := json.Marshal(input)
			for _, candidate := range []struct {
				destination, code, field string
				row                      int
				inputHash                string
			}{
				{"relative-output211.json", "output_invalid", "output", 0, ""},
				{filepath.Join(directory, "new-pool.json"), "source_invalid", "commands_file", 2, poolDiagnosticDigest211(raw)},
			} {
				var out, diagnostic bytes.Buffer
				code := runWithRunnerSecretsInput([]string{"--language=" + language, "--prepare-pool-config", candidate.destination}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, bytes.NewReader(raw))
				if code != 1 || out.Len() != 0 || bytes.Count(diagnostic.Bytes(), []byte{'\n'}) != 1 {
					t.Fatal("failure did not emit exactly one stderr diagnostic", code, out.Len(), diagnostic.String())
				}
				var value map[string]any
				if err := json.Unmarshal(diagnostic.Bytes(), &value); err != nil {
					t.Fatal("failure is not the new closed JSON diagnostic", err)
				}
				if len(value) != 7 || value["protocol"] != "suma-pool-config-diagnostic-v1" || value["code"] != candidate.code || value["scope_index"] != float64(candidate.row) || value["field"] != candidate.field || value["input_sha256"] != candidate.inputHash || value["output_sha256"] != poolDiagnosticDigest211([]byte(candidate.destination)) {
					t.Fatal("failure correlation or safe location differs", value)
				}
				message, ok := value["message"].(string)
				if !ok || message == "" || strings.Contains(diagnostic.String(), directory) || strings.Contains(diagnostic.String(), "private-source-content211") || strings.Contains(diagnostic.String(), candidate.destination) {
					t.Fatal("failure exposed private input or destination")
				}
				if language == "en-US" && strings.ContainsRune(message, '请') || language == "zh-CN" && !strings.ContainsRune(message, '请') {
					t.Fatal("diagnostic message did not use requested language", message)
				}
			}
			// The file was durably prepared before stdout failed. A partial ACK
			// must remain unknown, with no fabricated input correlation or retry.
			if err := os.WriteFile(source, []byte(`{}`), 0600); err != nil {
				t.Fatal(err)
			}
			ackDestination := filepath.Join(directory, "ack-unconfirmed.json")
			var diagnostic bytes.Buffer
			broken := &poolDiagnosticBrokenOutput211{}
			code := runWithRunnerSecretsInput([]string{"--language=" + language, "--prepare-pool-config", ackDestination}, broken, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, bytes.NewReader(raw))
			var unknown map[string]any
			if code != 1 || broken.written.Len() != 3 || json.Unmarshal(diagnostic.Bytes(), &unknown) != nil || len(unknown) != 7 || unknown["code"] != "output_unconfirmed" || unknown["scope_index"] != float64(0) || unknown["field"] != "output" || unknown["input_sha256"] != "" || unknown["output_sha256"] != poolDiagnosticDigest211([]byte(ackDestination)) || strings.Contains(diagnostic.String(), "private-stdout-cause211") {
				t.Fatal("partial output ACK lost its uncertainty", code, diagnostic.String())
			}
			written, err := os.ReadFile(ackDestination)
			if _, decodeErr := runner.DecodePoolConfig(written); err != nil || decodeErr != nil {
				t.Fatal("output ACK error discarded the prepared file", err, decodeErr)
			}
		})
	}
}

func TestPoolConfigPreparationModeRejectsOtherFlagsBeforePrivateInput(t *testing.T) {
	for _, other := range []string{"--prepare-tools=x", "--control=http://127.0.0.1:4310", "--pool=x", "--pool-config=x", "--pool-identity=x", "--pair-pool-runner=x", "--project=x", "--identity=x", "--commands=x", "--allow-exec=false", "--maintenance-only=false", "--prepare-opencode-stdin=false", "--report-tools=false"} {
		input := &setupReader191{}
		var out, diagnostic bytes.Buffer
		code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-pool-config=x", other}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, input)
		if code != 1 || input.reads != 0 || out.Len() != 0 {
			t.Fatalf("Pool config preparation mixed flags reached private effects: %s exit=%d reads=%d", other, code, input.reads)
		}
	}
	input := &setupReader191{}
	var out, diagnostic bytes.Buffer
	if code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-pool-config", filepath.Join(t.TempDir(), "absent", "pool.json")}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, input); code != 1 || input.reads != 0 {
		t.Fatal("output guard did not precede private input", code, input.reads)
	}
	input = &setupReader191{}
	if code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-pool-config", "relative-pool.json"}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, input); code != 1 || input.reads != 0 {
		t.Fatal("relative output reached private input", code, input.reads)
	}
}
