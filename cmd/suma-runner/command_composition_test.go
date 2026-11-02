package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/runner"
)

func compositionInput215(t *testing.T, directory string) []byte {
	t.Helper()
	files := []string{filepath.Join(directory, "first-private215.json"), filepath.Join(directory, "second-private215.json")}
	for i, name := range []string{"first", "second"} {
		raw := []byte(`{"` + name + `":{"adapter":"command","protocol":"text-v1","executable":"not-launched-215","args":["private-argument215"]}}`)
		if err := os.WriteFile(files[i], raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(map[string]any{"files": files})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCommandCompositionModeRejectsOtherFlagsBeforePrivateEffects(t *testing.T) {
	// Poison stdin, keyring and environment callbacks prove conflicting modes
	// cannot access private state. Explicit empty/false flags still conflict.
	for _, other := range []string{"--control=http://127.0.0.1:4310", "--project=x", "--worker=x", "--workspace=x", "--repository=x", "--base-commit=x", "--worktrees=x", "--merge-preflight=x", "--state=x", "--inspect-state=false", "--commands=x", "--model-deployments=x", "--external-config=x", "--allow-exec=false", "--allow-installed-tool-discovery=false", "--maintenance-only=false", "--describe-tools=false", "--prepare-tools=x", "--prepare-acp-stdin=false", "--prepare-sdk-stdin=false", "--prepare-codex-stdin=false", "--prepare-opencode-stdin=false", "--report-tools=false", "--discover-installed-tools=false", "--describe-acp-models-stdin=false", "--local-tool=x", "--pool-config=x", "--check-pool-connection=false", "--prepare-pool-config=x", "--pair-pool-runner=x", "--pool=x", "--pool-identity=x", "--pair-runner=x", "--identity=x", "--paired-runner=x", "--device-name=x"} {
		input := &setupReader191{}
		var out, diagnostic bytes.Buffer
		code := runWithRunnerSecretsInput([]string{"--language=en", "--compose-commands=x", other}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, input)
		if code != 1 || input.reads != 0 || out.Len() != 0 {
			t.Fatalf("composition reached private effects for %s: exit=%d reads=%d", other, code, input.reads)
		}
		var failure map[string]any
		if json.Unmarshal(diagnostic.Bytes(), &failure) != nil || len(failure) != 4 || failure["code"] != "mode_conflict" || failure["source_index"] != float64(0) {
			t.Fatal("mode conflict lost the fixed diagnostic contract", diagnostic.String())
		}
	}
	for _, destination := range []string{"", "relative.json", filepath.Join(t.TempDir(), "absent", "new.json")} {
		input := &setupReader191{}
		var out, diagnostic bytes.Buffer
		code := runWithRunnerSecretsInput([]string{"--compose-commands=" + destination}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, input)
		if code != 1 || input.reads != 0 || out.Len() != 0 {
			t.Fatal("destination guard did not precede private input", code, input.reads)
		}
	}
}

func TestCommandCompositionCLIReceiptAndOriginalDecode(t *testing.T) {
	directory := t.TempDir()
	input := compositionInput215(t, directory)
	destination := filepath.Join(directory, "new-combined.json")
	var out, diagnostic bytes.Buffer
	code := runWithRunnerSecretsInput([]string{"--language=en", "--compose-commands", destination}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, bytes.NewReader(input))
	var receipt map[string]any
	if code != 0 || diagnostic.Len() != 0 || json.Unmarshal(out.Bytes(), &receipt) != nil || len(receipt) != 4 || receipt["protocol"] != "suma-command-composition-v1" || receipt["source_count"] != float64(2) || receipt["command_count"] != float64(2) {
		t.Fatal("safe receipt differs", code, out.String(), diagnostic.String())
	}
	raw, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if receipt["configuration_sha256"] != hex.EncodeToString(digest[:]) {
		t.Fatal("receipt did not bind exact persisted bytes")
	}
	parsed, err := runner.DecodeCommands(raw)
	if err != nil || len(parsed) != 2 {
		t.Fatal("composed file cannot use original decoder", err)
	}
	if strings.Contains(out.String(), directory) || strings.Contains(out.String(), "private-argument215") {
		t.Fatal("receipt exposed private metadata")
	}
	out.Reset()
	diagnostic.Reset()
	if code = runWithRunnerSecretsInput([]string{"--compose-commands", destination}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, &setupReader191{}); code != 1 {
		t.Fatal("existing output was overwritten")
	}
	same, _ := os.ReadFile(destination)
	if !bytes.Equal(raw, same) {
		t.Fatal("existing output changed")
	}
}

func TestCommandCompositionCLISafeLocalizedErrorsAndUnconfirmedOutput(t *testing.T) {
	for _, language := range []string{"en-US", "zh-CN"} {
		directory := t.TempDir()
		input := compositionInput215(t, directory)
		source := filepath.Join(directory, "second-private215.json")
		if err := os.WriteFile(source, []byte(`{"first":{"executable":"private-secret215"}}`), 0600); err != nil {
			t.Fatal(err)
		}
		var out, diagnostic bytes.Buffer
		destination := filepath.Join(directory, "new.json")
		code := runWithRunnerSecretsInput([]string{"--language=" + language, "--compose-commands", destination}, &out, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, bytes.NewReader(input))
		var failure map[string]any
		if code != 1 || out.Len() != 0 || json.Unmarshal(diagnostic.Bytes(), &failure) != nil || len(failure) != 4 || failure["code"] != "duplicate_command" || failure["source_index"] != float64(2) {
			t.Fatal("safe duplicate diagnostic differs", code, diagnostic.String())
		}
		message, _ := failure["message"].(string)
		if message == "" || strings.Contains(diagnostic.String(), directory) || strings.Contains(diagnostic.String(), "private-secret215") || language == "zh-CN" && !strings.ContainsRune(message, '请') || language == "en-US" && strings.ContainsRune(message, '请') {
			t.Fatal("localized diagnostic leaked or used wrong language")
		}
		input = compositionInput215(t, directory)
		diagnostic.Reset()
		broken := &poolDiagnosticBrokenOutput211{}
		code = runWithRunnerSecretsInput([]string{"--language=" + language, "--compose-commands", destination}, broken, &diagnostic, noPairingEnvironmentCredential(t), setupSecrets191{t}, bytes.NewReader(input))
		if code != 1 || json.Unmarshal(diagnostic.Bytes(), &failure) != nil || failure["code"] != "output_unconfirmed" {
			t.Fatal("partial receipt lost uncertainty", code, diagnostic.String())
		}
		raw, err := os.ReadFile(destination)
		if err != nil {
			t.Fatal("unconfirmed receipt discarded new file", err)
		}
		if _, err = runner.DecodeCommands(raw); err != nil {
			t.Fatal(err)
		}
	}
}
