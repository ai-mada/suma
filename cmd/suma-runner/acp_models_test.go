package main

import (
	"bytes"
	"github.com/ai-mada/suma/internal/commandlocale"
	"strings"
	"testing"
)

func TestACPModelsStdinRejectsInvalidBeforeCredentialsOrWorkerState(t *testing.T) {
	for _, input := range []string{`null`, `{"adapter_id":"qoder-acp"}`, strings.Repeat("x", (16<<10)+1)} {
		var out, stderr bytes.Buffer
		app := commandlocale.New("suma-runner", "速码执行器", "suma worker", &out, &stderr, func(name string) (string, bool) {
			if strings.HasPrefix(name, "SUMA_RUNNER") || strings.HasPrefix(name, "SUMA_POOL") {
				t.Fatal("model-only invalid input read an environment credential")
			}
			return "", false
		})
		if code := runACPModelDiscoveryStdin(app, strings.NewReader(input)); code != 1 || out.Len() != 0 {
			t.Fatal("invalid model-only input produced an observation")
		}
	}
}
func TestACPModelsModeRejectsOtherRunnerModesBeforeStdin(t *testing.T) {
	for _, args := range [][]string{{"--describe-acp-models-stdin", "--describe-tools"}, {"--describe-acp-models-stdin", "--discover-installed-tools"}, {"--describe-acp-models-stdin", "--commands", "must-not-read.json"}, {"--describe-acp-models-stdin", "--allow-exec"}, {"--describe-acp-models-stdin", "--local-tool", "qoder-acp"}} {
		var out, stderr bytes.Buffer
		if code := run(args, &out, &stderr, func(name string) (string, bool) {
			if strings.HasPrefix(name, "SUMA_RUNNER") || strings.HasPrefix(name, "SUMA_POOL") {
				t.Fatal("mixed discovery read worker credentials")
			}
			return "", false
		}); code != 1 {
			t.Fatal("model-only mode accepted an execution configuration")
		}
	}
}
