package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func TestInstalledToolDiscoveryCommandOutputsOnlyBoundedLocalFacts(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)
	var out, stderr bytes.Buffer
	app := commandlocale.New("suma-runner", "测试", "test", &out, &stderr, func(name string) (string, bool) {
		if name == "SUMA_RUNNER_TOKEN" || name == "SUMA_AGENT_TOKEN" {
			t.Fatal("local discovery read a platform credential")
		}
		return "", false
	})
	if code := runInstalledToolDiscovery(app, ""); code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
	var report runner.InstalledToolDiscovery
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || report.Protocol != runner.InstalledToolDiscoveryProtocol || len(report.Items) != 3 {
		t.Fatalf("invalid local output: %s %v", out.String(), err)
	}
	for _, item := range report.Items {
		if item.Installed || item.LaunchSupported || item.Command != nil {
			t.Fatalf("invented authority: %+v", item)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("local discovery created state: %v %v", entries, err)
	}
}

func TestInstalledToolDiscoveryCommandRejectsUnknownIDWithoutEchoingInput(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var out, stderr bytes.Buffer
	app := commandlocale.New("suma-runner", "测试", "test", &out, &stderr, nil)
	if code := runInstalledToolDiscovery(app, "secret-token-do-not-echo"); code == 0 || out.Len() != 0 || stderr.Len() == 0 || bytes.Contains(stderr.Bytes(), []byte("secret-token")) {
		t.Fatalf("unsafe unknown ID response: stdout=%q stderr=%q", out.String(), stderr.String())
	}
}
