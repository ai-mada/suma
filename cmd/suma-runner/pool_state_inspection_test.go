package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/runner"
)

func TestPoolStateInspectionUsesOriginalStateWithoutCredentialsOrWrites(t *testing.T) {
	state := t.TempDir()
	pool := filepath.Join(state, "pool")
	if err := os.Mkdir(pool, 0700); err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	identity, err := json.Marshal([]any{digest, "fixture-pool", "fixture-runner", 1, digest})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pool, "identity.json"), identity, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ locale, message string }{
		{"zh-CN", "已读取资源池本地快照"}, {"en-US", "Local Pool snapshot read"},
	} {
		var out, stderr bytes.Buffer
		code := run([]string{"--inspect-pool-state", "--state", state, "--language", tc.locale}, &out, &stderr, func(name string) (string, bool) {
			if strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET") {
				t.Fatalf("inspection read credential environment %s", name)
			}
			return "", false
		})
		var got runner.PoolStateInspection
		if code != 0 || json.Unmarshal(out.Bytes(), &got) != nil || got.Component != "local_pool_runner_state" || !got.Complete || got.Slots == nil || len(got.Slots) != 0 || !strings.Contains(stderr.String(), tc.message) {
			t.Fatalf("invalid diagnostic, exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
		}
		for _, private := range []string{state, "fixture-pool", "fixture-runner", digest} {
			if strings.Contains(out.String()+stderr.String(), private) {
				t.Fatal("private Pool identity appeared in inspection output")
			}
		}
	}
	entries, err := os.ReadDir(pool)
	if err != nil || len(entries) != 1 || entries[0].Name() != "identity.json" {
		t.Fatalf("inspection changed private Pool files: %v, %v", entries, err)
	}
}

func TestPoolStateInspectionRejectsOtherModesByFlagPresence(t *testing.T) {
	for _, flags := range [][]string{
		{"--inspect-pool-state=false"},
		{"--inspect-pool-state", "--state="},
		{"--inspect-pool-state", "--pool-identity=private-path"},
		{"--inspect-pool-state", "--inspect-state=false"},
		{"--inspect-pool-state", "--prepare-pool-config=private-path"},
		{"--inspect-pool-state", "--allow-exec=false"},
		{"--inspect-pool-state", "--control=http://127.0.0.1:4310"},
	} {
		var out, stderr bytes.Buffer
		args := append([]string{"--language=en-US"}, flags...)
		code := run(args, &out, &stderr, func(name string) (string, bool) {
			if strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET") {
				t.Fatalf("conflicting flags read credential environment %s", name)
			}
			return "", false
		})
		if code != 2 || out.Len() != 0 || !strings.Contains(stderr.String(), "Pool state inspection only accepts") || strings.Contains(stderr.String(), "private-path") {
			t.Fatalf("mode not rejected before access, exit=%d stderr=%q", code, stderr.String())
		}
	}
}
