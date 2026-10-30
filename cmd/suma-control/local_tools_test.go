package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestInstalledToolDiscoveryModeDoesNotInitializeControlState(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		code int
	}{
		{"unknown tool", []string{"--discover-installed-tools", "--local-tool", "untrusted-shell"}, 1},
		{"selector requires mode", []string{"--local-tool", "codex-jsonl"}, 1},
		{"maintenance cannot mix", []string{"--discover-installed-tools", "--maintenance"}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			lookup := func(name string) (string, bool) {
				if strings.HasPrefix(name, "SUMA_DB_") || name == "SUMA_DATA_DIR" || name == "SUMA_DESKTOP_WORKSPACE_KEY" || name == "SUMA_RUNNER_TOKEN" {
					t.Errorf("read-only mode read business setting %s", name)
				}
				return "", false
			}
			if got := run(test.args, &out, &stderr, lookup); got != test.code {
				t.Fatalf("exit=%d want=%d: %s", got, test.code, stderr.String())
			}
		})
	}
}
