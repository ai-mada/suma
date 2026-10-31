package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/commandlocale"
	rc "github.com/ai-mada/suma/internal/desktoprecoverycontract"
)

func TestDesktopResultRecoveryRejectsExtraAuthorityAndMixedModes(t *testing.T) {
	for _, raw := range []string{`{}`, `{"action":"prepare","state_directory":"relative"}`, `{"action":"prepare","state_directory":"/one","state_directory":"/two"}`, `{"action":"prepare","state_directory":"/state","token":"private"}`, `{"action":"apply","state_directory":"/state","proof_key":"not-a-key"}`} {
		var out, stderr bytes.Buffer
		app := commandlocale.New("suma-control", "核对", "Recover", &out, &stderr, func(string) (string, bool) { return "", false })
		if code := runDesktopResultRecovery(app, strings.NewReader(raw)); code != 1 || out.Len() != 0 {
			t.Fatalf("invalid private input accepted: code=%d output=%q", code, out.String())
		}
	}
	for _, mode := range []string{"--desktop-runner-renewal-check-stdin", "--describe-acp-models-stdin", "--describe-codex-models-stdin", "--desktop-managed-runner", "--discover-installed-tools", "--maintenance", "--local-tool=codex-jsonl"} {
		var out, stderr bytes.Buffer
		if code := run([]string{"--desktop-runner-result-recovery-stdin", mode}, &out, &stderr, func(string) (string, bool) { return "", false }); code != 1 || out.Len() != 0 {
			t.Fatalf("mixed recovery mode accepted: %s code=%d", mode, code)
		}
	}
}

func TestDesktopResultRecoveryPrepareDoesNotStartServiceOrCreateState(t *testing.T) {
	var out, stderr bytes.Buffer
	app := commandlocale.New("suma-control", "核对", "Recover", &out, &stderr, func(string) (string, bool) { return "", false })
	raw, _ := json.Marshal(map[string]string{"action": "prepare", "state_directory": t.TempDir(), "project_id": strings.Repeat("a", 32), "worker_id": "desktop-" + strings.Repeat("b", 32)})
	if code := runDesktopResultRecovery(app, bytes.NewReader(raw)); code != 0 {
		t.Fatalf("private prepare failed: %d %s", code, stderr.String())
	}
	var result rc.Result
	if json.Unmarshal(out.Bytes(), &result) != nil || result.Protocol != rc.ResultProtocol || result.Total != 0 || result.Recovered != 0 || result.Remaining != 0 || len(result.Reasons) != 1 || result.Reasons[0] != "invalid_state" {
		t.Fatalf("empty state observation differs: %s", out.String())
	}
}
