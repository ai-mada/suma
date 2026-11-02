package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/runner"
)

func TestPrepareACPToolModeSupportsExplicitQoderWithoutCredentialsOrWorker(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"en", "zh"} {
		file := filepath.Join(t.TempDir(), "new-qoder.json")
		input := map[string]string{"adapter_id": "qoder-acp", "executable": executable, "reviewed_version": "administrator-reviewed221", "model": "explicit-model221", "credential_env_ref": ""}
		raw, _ := json.Marshal(input)
		lookup := func(name string) (string, bool) {
			if strings.Contains(name, "TOKEN") || name == "LOCAL_QODER_CREDENTIAL" {
				t.Fatal("preparation read an identity or provider credential")
			}
			return "", false
		}
		var out, stderr bytes.Buffer
		code := runWithRunnerSecretsInput([]string{"--language=" + locale, "--prepare-tools", file, "--prepare-acp-stdin"}, &out, &stderr, lookup, setupSecrets191{t}, bytes.NewReader(raw))
		var result runner.ToolPreparation
		if code != 0 || stderr.Len() != 0 || json.Unmarshal(out.Bytes(), &result) != nil || !result.Prepared || result.CommandCount != 1 || result.File != file {
			t.Fatal("original private ACP CLI mode did not return Qoder file facts", code, &out, &stderr)
		}
		saved, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		commands, err := runner.DecodeCommands(saved)
		if err != nil || len(commands) != 1 || commands[runner.QoderToolPreparationCommandID].Adapter != "qoder" {
			t.Fatal("CLI created another adapter", err)
		}
		out.Reset()
		stderr.Reset()
		reader := &setupReader191{}
		if code = runWithRunnerSecretsInput([]string{"--language=" + locale, "--prepare-tools", file, "--prepare-acp-stdin"}, &out, &stderr, lookup, setupSecrets191{t}, reader); code != 1 || reader.reads != 0 || out.Len() != 0 || !strings.Contains(stderr.String(), "ACP") {
			t.Fatal("unsafe duplicate output consumed private input or returned success")
		}
	}
}
