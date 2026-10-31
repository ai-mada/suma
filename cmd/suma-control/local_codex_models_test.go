package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/commandlocale"
)

func TestCodexModelDiscoveryControlEntryCannotStartAnotherMode(t *testing.T) {
	for _, other := range []string{"--describe-acp-models-stdin", "--desktop-managed-runner", "--discover-installed-tools", "--maintenance"} {
		var out, stderr bytes.Buffer
		lookup := func(name string) (string, bool) {
			if strings.HasPrefix(name, "SUMA_DB_") || name == "SUMA_RUNNER_TOKEN" || name == "SUMA_DATA_DIR" {
				t.Fatalf("private catalogue mode read a service credential or database setting: %s", name)
			}
			return "", false
		}
		if run([]string{"--describe-codex-models-stdin", other}, &out, &stderr, lookup) != 1 || out.Len() != 0 {
			t.Fatal("mixed private and service modes were accepted", other)
		}
	}
	var out, stderr bytes.Buffer
	app := commandlocale.New("suma-control", "", "", &out, &stderr, func(string) (string, bool) { return "", false })
	if runCodexModelDiscovery(app, strings.NewReader(`{"adapter_id":"codex-app-server","goal":"must-not-run"}`)) != 1 || out.Len() != 0 || strings.Contains(stderr.String(), "must-not-run") {
		t.Fatal("a development goal entered or escaped the private catalogue mode")
	}
}
