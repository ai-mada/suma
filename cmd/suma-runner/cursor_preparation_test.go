package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/runner"
)

type setupSecrets191 struct{ t *testing.T }

func (s setupSecrets191) Get(string, string) (string, error) {
	s.t.Fatal("preparation read system credential storage")
	return "", runner.ErrRunnerSecretNotFound
}
func (s setupSecrets191) Set(string, string, string) error {
	s.t.Fatal("preparation wrote system credential storage")
	return nil
}

type setupReader191 struct{ reads int }

func (r *setupReader191) Read([]byte) (int, error) { r.reads++; return 0, os.ErrInvalid }

func TestPrepareACPToolModeCreatesOnlyNewConfiguration(t *testing.T) {
	root := t.TempDir()
	nodeName := "node"
	if runtime.GOOS == "windows" {
		nodeName += ".exe"
	}
	node := filepath.Join(root, nodeName)
	version := "owned configuration fixture191"
	model := "owned-model191"
	file := filepath.Join(t.TempDir(), "commands.json")
	actual := os.Getenv("SUMA_CURSOR_SETUP_ACTUAL_NODE")
	if actual != "" {
		node = actual
		version = os.Getenv("SUMA_CURSOR_SETUP_ACTUAL_VERSION")
		file = os.Getenv("SUMA_CURSOR_SETUP_ACTUAL_FILE")
		model = "configuration-only-unverified-model191"
		if !filepath.IsAbs(node) || version == "" || !filepath.IsAbs(file) {
			t.Fatal("explicit actual preparation inputs missing")
		}
	} else {
		if err := os.WriteFile(node, []byte("MZowned native-format bytes; must never execute"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "index.js"), []byte("// fixed owned entry191"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	input := map[string]string{"adapter_id": "cursor-cli", "executable": node, "reviewed_version": version, "model": model, "credential_env_ref": "LOCAL_CURSOR_SETUP_191"}
	raw, _ := json.Marshal(input)
	var out, stderr bytes.Buffer
	lookup := func(name string) (string, bool) {
		if strings.Contains(name, "TOKEN") || name == input["credential_env_ref"] {
			t.Fatal("preparation read a credential environment value")
		}
		return "", false
	}
	code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-tools", file, "--prepare-acp-stdin"}, &out, &stderr, lookup, setupSecrets191{t}, strings.NewReader(string(raw)))
	var result runner.ToolPreparation
	if code != 0 || stderr.Len() != 0 || json.Unmarshal(out.Bytes(), &result) != nil || !result.Prepared || result.CommandCount != 1 || result.File != file || len(result.Remaining) != 0 {
		t.Fatal("CLI did not return only prepared file facts", code, &out, &stderr)
	}
	bytes, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := runner.DecodeCommands(bytes)
	if err != nil || len(commands) != 1 || commands["local-cursor-cli"].ACP == nil || commands["local-cursor-cli"].ACP.Model != model {
		t.Fatal("CLI did not write fixed original configuration", err)
	}
	if actual != "" {
		t.Logf("actual static preparation only: file=%s commands=1 closure=%s configuration=%s; no native process/version/auth/model proof", file, commands["local-cursor-cli"].ACP.CursorPackage.SHA256, runner.ACPConfigurationDigest(*commands["local-cursor-cli"].ACP))
	}
	t.Logf("configuration_file_sha256=%x stdout_sha256=%x stderr_sha256=%x", sha256.Sum256(bytes), sha256.Sum256(out.Bytes()), sha256.Sum256(stderr.Bytes()))
}

func TestPrepareACPToolModeRejectsPresenceAndMixedFlagsBeforeInput(t *testing.T) {
	for _, args := range [][]string{{"--prepare-acp-stdin"}, {"--prepare-acp-stdin=false"}, {"--prepare-acp-stdin", "--prepare-tools="}, {"--prepare-acp-stdin=false", "--prepare-tools=x"}, {"--prepare-acp-stdin", "--prepare-tools=x", "--allow-exec=false"}, {"--prepare-acp-stdin", "--prepare-tools=x", "--report-tools"}, {"--prepare-acp-stdin", "--prepare-tools=x", "--commands=x"}, {"--prepare-acp-stdin", "--prepare-tools=x", "--identity=x"}, {"--prepare-acp-stdin", "--prepare-tools=x", "--paired-runner=x"}, {"--prepare-acp-stdin", "--pair-runner=" + strings.Repeat("a", 32), "--control=http://127.0.0.1:4310", "--project=x"}, {"--prepare-acp-stdin", "--prepare-tools=x", "--local-tool=cursor-cli"}, {"--prepare-acp-stdin", "--prepare-tools=x", "--describe-acp-models-stdin"}} {
		reader := &setupReader191{}
		var out, stderr bytes.Buffer
		code := runWithRunnerSecretsInput(append([]string{"--language=en"}, args...), &out, &stderr, func(name string) (string, bool) {
			if strings.Contains(name, "TOKEN") {
				t.Fatal("invalid mode read credential")
			}
			return "", false
		}, setupSecrets191{t}, reader)
		if code != 1 || out.Len() != 0 || stderr.Len() == 0 || reader.reads != 0 {
			t.Fatal("invalid mode consumed input or fell through", args, code, &out, &stderr)
		}
	}
	file := filepath.Join(t.TempDir(), "exists.json")
	if err := os.WriteFile(file, []byte("preserve191"), 0600); err != nil {
		t.Fatal(err)
	}
	reader := &setupReader191{}
	var out, stderr bytes.Buffer
	if code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-tools", file, "--prepare-acp-stdin"}, &out, &stderr, func(string) (string, bool) { return "", false }, setupSecrets191{t}, reader); code != 1 || reader.reads != 0 {
		t.Fatal("existing target consumed stdin")
	}
}
