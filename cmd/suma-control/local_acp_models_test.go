package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/commandlocale"
)

func TestACPModelDiscoveryControlEntryIsPrivateAndExclusive(t *testing.T) {
	for _, args := range [][]string{
		{"--describe-acp-models-stdin", "--desktop-managed-runner"},
		{"--describe-acp-models-stdin", "--discover-installed-tools"},
		{"--describe-acp-models-stdin", "--maintenance"},
		{"--describe-acp-models-stdin", "--local-tool=qoder-acp"},
	} {
		var out, stderr bytes.Buffer
		lookup := func(key string) (string, bool) {
			if strings.HasPrefix(key, "SUMA_DB_") || key == "SUMA_DATA_DIR" || key == "SUMA_DESKTOP_WORKSPACE_KEY" || key == "SUMA_RUNNER_TOKEN" {
				t.Fatalf("private mode read a service or worker setting: %s", key)
			}
			return "", false
		}
		if run(args, &out, &stderr, lookup) != 1 || out.Len() != 0 {
			t.Fatalf("mixed mode ran or returned a directory: %v", args)
		}
	}
	for _, input := range []string{`{"adapter_id":"qoder-acp","adapter_id":"qoder-acp"}`, `{"argv":["--prompt","untrusted"]}`, strings.Repeat("x", 16385)} {
		var out, stderr bytes.Buffer
		app := commandlocale.New("suma-control", "", "", &out, &stderr, func(string) (string, bool) { return "", false })
		if runACPModelDiscovery(app, strings.NewReader(input)) != 1 || out.Len() != 0 || strings.Contains(stderr.String(), "untrusted") {
			t.Fatal("malformed private input produced a directory or leaked its body")
		}
	}
}
