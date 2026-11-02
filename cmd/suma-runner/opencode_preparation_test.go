package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareOpenCodeModeGatePrecedesPrivateInputs(t *testing.T) {
	for _, args := range [][]string{
		{"--prepare-opencode-stdin"},
		{"--prepare-opencode-stdin=false", "--prepare-tools=x"},
		{"--prepare-opencode-stdin", "--prepare-tools="},
		{"--prepare-opencode-stdin", "--prepare-tools=x", "--identity=x"},
		{"--prepare-opencode-stdin", "--prepare-tools=x", "--allow-exec=false"},
		{"--prepare-opencode-stdin", "--prepare-tools=x", "--prepare-sdk-stdin=false"},
		{"--prepare-opencode-stdin", "--prepare-tools=x", "--prepare-codex-stdin=false"},
		{"--prepare-opencode-stdin", "--prepare-tools=x", "--prepare-acp-stdin=false"},
		{"--prepare-opencode-stdin", "--prepare-tools=x", "--allow-installed-tool-discovery=false"},
		{"--prepare-opencode-stdin", "--prepare-tools=x", "--maintenance-only=false"},
		{"--prepare-opencode-stdin", "--prepare-tools=x", "--local-tool=opencode-sdk"},
	} {
		input := &setupReader191{}
		var out, stderr bytes.Buffer
		code := runWithRunnerSecretsInput(append([]string{"--language=en"}, args...), &out, &stderr, func(string) (string, bool) { return "", false }, setupSecrets191{t}, input)
		if code != 1 || input.reads != 0 || out.Len() != 0 {
			t.Fatalf("OpenCode mode reached private input/effects: %v code=%d reads=%d", args, code, input.reads)
		}
	}
	input := &setupReader191{}
	var out, stderr bytes.Buffer
	file := filepath.Join(t.TempDir(), "absent-parent", "commands.json")
	if code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-opencode-stdin", "--prepare-tools", file}, &out, &stderr, func(string) (string, bool) { return "", false }, setupSecrets191{t}, input); code != 1 || input.reads != 0 || !strings.Contains(stderr.String(), "OpenCode") {
		t.Fatal("destination rejection did not precede private read", code, input.reads, stderr.String())
	}
}
