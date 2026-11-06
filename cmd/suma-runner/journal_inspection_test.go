package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJournalInspectionCLIIsLocalReadOnlyAndCredentialIndependent(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"owned-key":{"assignment":{"run_id":"run-one","node_id":"develop","attempt":1,"fence":4,"config":{"secret":"private-payload"}},"acked":false,"result":{"outcome_unknown":true,"output":"private-output","error":"private-error"}}}`)
	if err := os.WriteFile(filepath.Join(dir, "journal.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, locale := range []string{"zh-CN", "en-US"} {
		var out, stderr bytes.Buffer
		lookup := func(name string) (string, bool) {
			if strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET") {
				t.Fatalf("inspection read credential environment %s", name)
			}
			return "", false
		}
		code := run([]string{"--inspect-state", "--state", dir, "--language", locale}, &out, &stderr, lookup)
		var got map[string]any
		if code != 0 || json.Unmarshal(out.Bytes(), &got) != nil || got["status"] != "available" || got["component"] != "local_execution_journal" {
			t.Fatalf("local CLI failed: code=%d stdout=%s stderr=%s", code, &out, &stderr)
		}
		counts, _ := got["counts"].(map[string]any)
		items, _ := got["items"].([]any)
		if counts["closed_results"] != float64(0) || len(items) != 1 {
			t.Fatalf("closed result counts must be present for an ordinary pending entry: %+v", got)
		}
		item, _ := items[0].(map[string]any)
		if item["result_rejected_closed"] != false {
			t.Fatalf("closed result fields must be present for ordinary pending entries: %+v", got)
		}
		if locale == "zh-CN" && !strings.Contains(stderr.String(), "本地执行日志") || locale == "en-US" && !strings.Contains(stderr.String(), "Local execution journal") {
			t.Fatal("inspection diagnostics are not paired Chinese/English")
		}
		for _, secret := range []string{dir, "owned-key", "private-payload", "private-output", "private-error"} {
			if strings.Contains(out.String()+stderr.String(), secret) {
				t.Fatalf("CLI exposed private data %q", secret)
			}
		}
	}
	after, err := os.ReadFile(filepath.Join(dir, "journal.json"))
	if err != nil || !bytes.Equal(after, raw) {
		t.Fatal("CLI inspection changed the journal")
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("CLI inspection initialized Runner state")
	}
}

func TestJournalInspectionCLIRejectsExecutionModesBeforeOpeningConfiguration(t *testing.T) {
	for _, conflict := range []string{"--allow-exec", "--describe-tools", "--commands=private-config", "--model-deployments=private-config", "--external-config=private-config", "--pool-config=private-config", "--merge-preflight=private-config", "--project=private-project", "--worker=private-worker", "--repository=private-repository", "--base-commit=private-commit", "--worktrees=private-worktrees", "--workspace=private-workspace", "--control=https://private-control.example"} {
		var out, stderr bytes.Buffer
		dir := filepath.Join(t.TempDir(), "absent-state")
		lookup := func(name string) (string, bool) {
			if strings.Contains(name, "TOKEN") {
				t.Fatal("conflicting inspection read Runner credentials")
			}
			return "", false
		}
		if code := run([]string{"--inspect-state", "--state", dir, "--language=en", conflict}, &out, &stderr, lookup); code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), "inspection") || strings.Contains(stderr.String(), "private-") {
			t.Fatalf("mode conflict not rejected privately: code=%d out=%q err=%q", code, out.String(), stderr.String())
		}
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("mode rejection created state")
		}
	}
}

func TestJournalInspectionCLIMissingStateDoesNotInitializeIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing", "state")
	var out, stderr bytes.Buffer
	if code := run([]string{"--inspect-state", "--state", dir, "--language=en"}, &out, &stderr, func(string) (string, bool) { return "", false }); code != 1 || !strings.Contains(out.String(), `"status":"missing"`) || !strings.Contains(stderr.String(), "Local execution journal") {
		t.Fatalf("missing-state contract failed: code=%d out=%q err=%q", code, out.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Dir(dir)); !os.IsNotExist(err) {
		t.Fatal("inspection initialized missing parent directory")
	}
}
