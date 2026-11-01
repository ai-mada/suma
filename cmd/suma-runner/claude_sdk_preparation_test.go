package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareSDKModeRejectsPresenceAndMixedFlagsBeforeInput(t *testing.T) {
	for _, args := range [][]string{{"--prepare-sdk-stdin"}, {"--prepare-sdk-stdin=false", "--prepare-tools=x"}, {"--prepare-sdk-stdin", "--prepare-tools="}, {"--prepare-sdk-stdin", "--prepare-tools=x", "--identity=x"}, {"--prepare-sdk-stdin", "--prepare-tools=x", "--allow-exec=false"}, {"--prepare-sdk-stdin", "--prepare-tools=x", "--prepare-acp-stdin=false"}, {"--prepare-sdk-stdin", "--prepare-tools=x", "--report-tools"}} {
		input := &setupReader191{}
		var out, stderr bytes.Buffer
		code := runWithRunnerSecretsInput(append([]string{"--language=en"}, args...), &out, &stderr, func(string) (string, bool) { return "", false }, setupSecrets191{t}, input)
		if code != 1 || input.reads != 0 || out.Len() != 0 {
			t.Fatalf("mixed SDK mode reached private input/effects: %v %d %d", args, code, input.reads)
		}
	}
	input := &setupReader191{}
	var out, stderr bytes.Buffer
	file := filepath.Join(t.TempDir(), "nonexistent-parent", "commands.json")
	if code := runWithRunnerSecretsInput([]string{"--language=en", "--prepare-sdk-stdin", "--prepare-tools", file}, &out, &stderr, func(string) (string, bool) { return "", false }, setupSecrets191{t}, input); code != 1 || input.reads != 0 || !strings.Contains(stderr.String(), "SDK") {
		t.Fatal("destination rejection did not precede private read", code, input.reads, stderr.String())
	}
}
