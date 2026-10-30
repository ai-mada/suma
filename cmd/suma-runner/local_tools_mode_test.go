package main

import (
	"bytes"
	"testing"
)

func TestInstalledToolDiscoveryModeNeverOpensRunnerState(t *testing.T) {
	for _, args := range [][]string{
		{"--discover-installed-tools", "--commands", "must-not-be-read.json"},
		{"--discover-installed-tools", "--pool-config", "must-not-be-read.json"},
		{"--discover-installed-tools", "--state", "must-not-be-created"},
		{"--discover-installed-tools", "--allow-exec"},
		{"--local-tool", "codex-jsonl"},
		{"--discover-installed-tools", "--local-tool", "unknown-shell"},
	} {
		var out, stderr bytes.Buffer
		lookup := func(name string) (string, bool) {
			if name == "SUMA_RUNNER_TOKEN" {
				t.Fatal("installed-tool mode read a worker token")
			}
			return "", false
		}
		if code := run(args, &out, &stderr, lookup); code != 1 {
			t.Fatalf("%v: exit=%d: %s", args, code, stderr.String())
		}
	}
}
