package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/commandlocale"
	"github.com/ai-mada/suma/internal/runner"
)

func TestDesktopRenewalCheckRejectsPrivateInputAndMixedModes(t *testing.T) {
	for _, raw := range []string{`{}`, `{"state_directory":"relative"}`, `{"state_directory":"/one","state_directory":"/two"}`, `{"state_directory":"/state","token":"not-accepted"}`, `{"state_directory":"/state/\ud800"}`} {
		var out, stderr bytes.Buffer
		app := commandlocale.New("suma-control", "检查", "Inspect", &out, &stderr, func(string) (string, bool) { return "", false })
		if code := runDesktopRunnerRenewalCheck(app, strings.NewReader(raw)); code != 1 || out.Len() != 0 {
			t.Fatalf("invalid private input was accepted: code=%d output=%q", code, out.String())
		}
	}
	for _, mode := range []string{"--describe-acp-models-stdin", "--describe-codex-models-stdin", "--desktop-managed-runner", "--discover-installed-tools", "--maintenance"} {
		var out, stderr bytes.Buffer
		if code := run([]string{"--desktop-runner-renewal-check-stdin", mode}, &out, &stderr, func(string) (string, bool) { return "", false }); code != 1 || out.Len() != 0 {
			t.Fatalf("mixed private inspection mode accepted: %s, code=%d", mode, code)
		}
	}
}

func TestDesktopRenewalCheckReadsEmptyStateWithoutStartingService(t *testing.T) {
	var out, stderr bytes.Buffer
	app := commandlocale.New("suma-control", "检查", "Inspect", &out, &stderr, func(string) (string, bool) { return "", false })
	raw, _ := json.Marshal(map[string]string{"state_directory": t.TempDir()})
	if code := runDesktopRunnerRenewalCheck(app, bytes.NewReader(raw)); code != 0 {
		t.Fatalf("private inspection failed: %d %s", code, stderr.String())
	}
	var result runner.DesktopRunnerRenewalCheck
	if json.Unmarshal(out.Bytes(), &result) != nil || result.Protocol != runner.DesktopRunnerRenewalCheckProtocol || !result.Safe || len(result.Reasons) != 0 {
		t.Fatalf("empty state observation was not exact: %s", out.String())
	}
}
