package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/runner"
)

type pairedCLISecrets struct {
	pairingCLIStore
	reads, writes int
}

func defaultPairedCLIIdentity(t *testing.T, control string, store *pairedCLISecrets) (string, string, string) {
	t.Helper()
	config := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", config)
	} else {
		t.Setenv("XDG_CONFIG_HOME", config)
		t.Setenv("HOME", config)
	}
	file, err := runner.DefaultRunnerIdentityPath(strings.Repeat("2", 32))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := runner.PrepareRunnerPairingIdentity(file, control, strings.Repeat("1", 32), strings.Repeat("2", 32), "owned fixture", store)
	if err != nil {
		t.Fatal(err)
	}
	token, worker := identity.Token(), "remote-"+strings.Repeat("a", 32)
	if err = identity.SaveApproved(worker, time.Now().UTC().Add(24*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	if err = identity.Close(); err != nil {
		t.Fatal(err)
	}
	store.reads, store.writes = 0, 0
	return file, token, worker
}

func TestPairedRunnerReportUsesDefaultIdentityAndPreservesScopeAssertions(t *testing.T) {
	store := &pairedCLISecrets{}
	requests := 0
	var token, worker string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var body struct {
			ProjectID string `json:"project_id"`
			WorkerID  string `json:"worker_id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if r.URL.Path != "/api/v1/runner/tool-inventory" || r.Header.Get("Authorization") != "Bearer "+token || body.ProjectID != strings.Repeat("1", 32) || body.WorkerID != worker {
			t.Error("selector changed the original reporting identity", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"acknowledged":true,"observed_at":"2026-11-01T05:00:00Z"}`))
	}))
	defer server.Close()
	file, secret, id := defaultPairedCLIIdentity(t, server.URL, store)
	token, worker = secret, id
	args := []string{"--language=en", "--report-tools", "--paired-runner=" + strings.Repeat("2", 32)}
	var out, stderr bytes.Buffer
	if code := runWithRunnerSecrets(args, &out, &stderr, noPairingEnvironmentCredential(t), store); code != 0 || requests != 1 || store.reads != 1 || store.writes != 0 || !strings.Contains(out.String(), worker) || bytes.Contains(out.Bytes(), []byte(token)) {
		t.Fatalf("default identity did not reuse the original reporter: code=%d reads=%d writes=%d out=%s err=%s", code, store.reads, store.writes, &out, &stderr)
	}
	if _, err := os.Stat(filepath.Join(file+".journal", "tool-inventory.json")); err != nil {
		t.Fatal("selector used another execution journal", err)
	}
	for _, option := range []string{"--control=" + server.URL + "/other", "--project=" + strings.Repeat("3", 32), "--worker=another-worker"} {
		t.Run(option, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "not-created")
			out.Reset()
			stderr.Reset()
			changed := append(append([]string{}, args...), option, "--state="+state)
			if code := runWithRunnerSecrets(changed, &out, &stderr, noPairingEnvironmentCredential(t), store); code != 1 || requests != 1 || out.Len() != 0 || store.writes != 0 {
				t.Fatalf("scope drift reached the reporter: code=%d out=%s err=%s", code, &out, &stderr)
			}
			if _, err := os.Lstat(state); !os.IsNotExist(err) {
				t.Fatal("scope drift changed an execution journal", err)
			}
		})
	}
}

func TestPairedRunnerMetadataSelectorMismatchNeverReadsSystemSecret(t *testing.T) {
	store := &pairedCLISecrets{}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	file, _, _ := defaultPairedCLIIdentity(t, server.URL, store)
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil {
		t.Fatal("invalid fixture metadata")
	}
	metadata["pairing_id"] = strings.Repeat("3", 32)
	raw, _ = json.Marshal(metadata)
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "not-created")
	var out, stderr bytes.Buffer
	args := []string{"--language=en", "--report-tools", "--paired-runner=" + strings.Repeat("2", 32), "--state=" + state}
	if code := runWithRunnerSecrets(args, &out, &stderr, noPairingEnvironmentCredential(t), store); code != 1 || requests != 0 || store.reads != 0 || store.writes != 0 || out.Len() != 0 {
		t.Fatalf("foreign selector metadata reached protected secret access: code=%d reads=%d writes=%d out=%s err=%s", code, store.reads, store.writes, &out, &stderr)
	}
	if after, err := os.ReadFile(file); err != nil || !bytes.Equal(raw, after) {
		t.Fatal("selector refusal rewrote original metadata", err)
	}
	if _, err = os.Lstat(state); !os.IsNotExist(err) {
		t.Fatal("selector refusal changed the execution journal", err)
	}
}

func (s *pairedCLISecrets) Get(service, account string) (string, error) {
	s.reads++
	return s.pairingCLIStore.Get(service, account)
}
func (s *pairedCLISecrets) Set(service, account, secret string) error {
	s.writes++
	return s.pairingCLIStore.Set(service, account, secret)
}

func TestPairedRunnerFlagsRejectAmbiguousOrNonExecutionModesBeforeSecrets(t *testing.T) {
	for _, args := range [][]string{
		{"--paired-runner="},
		{"--paired-runner=invalid-private-value"},
		{"--paired-runner=" + strings.Repeat("a", 32), "--identity=x"},
		{"--paired-runner=" + strings.Repeat("a", 32), "--pair-runner=" + strings.Repeat("a", 32)},
		{"--paired-runner=" + strings.Repeat("a", 32), "--describe-tools"},
		{"--paired-runner=" + strings.Repeat("a", 32), "--discover-installed-tools"},
		{"--paired-runner=" + strings.Repeat("a", 32), "--inspect-state"},
		{"--paired-runner=" + strings.Repeat("a", 32), "--pool-config=x"},
		{"--paired-runner=" + strings.Repeat("a", 32), "--prepare-tools=x"},
		{"--paired-runner=" + strings.Repeat("a", 32), "--report-tools=false"},
		{"--paired-runner=" + strings.Repeat("a", 32), "--local-tool=codex-jsonl"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			store := &pairedCLISecrets{}
			var out, stderr bytes.Buffer
			code := runWithRunnerSecrets(append([]string{"--language=en"}, args...), &out, &stderr, noPairingEnvironmentCredential(t), store)
			if code != 1 || out.Len() != 0 || stderr.Len() == 0 || strings.Contains(stderr.String(), "private-value") || store.reads != 0 || store.writes != 0 {
				t.Fatalf("invalid saved selector reached credentials/execution: code=%d reads=%d writes=%d out=%s err=%s", code, store.reads, store.writes, &out, &stderr)
			}
		})
	}
}
