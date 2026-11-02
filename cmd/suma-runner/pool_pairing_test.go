package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pp "github.com/ai-mada/suma/internal/poolrunnerpairingcontract"
	"github.com/ai-mada/suma/internal/runner"
)

func TestPoolPairingModeRejectsProjectAndExecutionFlagsBeforeSecrets(t *testing.T) {
	for _, args := range [][]string{
		{"--pair-pool-runner="},
		{"--pair-pool-runner=" + strings.Repeat("a", 32), "--control=https://control.example.test", "--pool=" + strings.Repeat("b", 32), "--project=x"},
		{"--pair-pool-runner=x", "--pool-config=never.json"},
		{"--pair-pool-runner=x", "--identity=never.json"},
		{"--pair-pool-runner=x", "--allow-exec=false"},
		{"--pair-pool-runner=x", "--allow-installed-tool-discovery=false"},
		{"--pair-pool-runner=x", "--prepare-opencode-stdin=false"},
		{"--pool-identity="},
		{"--pool-identity=never.json", "--pool-config=never.json", "--paired-runner=x"},
		{"--pool=x"},
	} {
		var out, diagnostic bytes.Buffer
		code := runWithRunnerSecrets(append([]string{"--language=en"}, args...), &out, &diagnostic, func(name string) (string, bool) {
			if strings.Contains(name, "TOKEN") {
				t.Fatal("invalid pool pairing mode read an environment credential")
			}
			return "", false
		}, setupSecrets191{t})
		if code != 1 || out.Len() != 0 || strings.Contains(diagnostic.String(), "never.json") {
			t.Fatalf("pool pairing mode did not fail before secrets/effects: %v exit=%d", args, code)
		}
	}
}

func TestPoolPairingCLISavesOnlyPublicIdentityThenUsesOriginalPoolSession(t *testing.T) {
	pool, pair, runnerID := strings.Repeat("3", 32), strings.Repeat("4", 32), strings.Repeat("5", 32)
	file := filepath.Join(t.TempDir(), "identity.json")
	store := &pairingCLIStore{}
	var proposal pp.PoolRunnerPairingProposalRequest
	var token string
	calls, sessionCalls := 0, 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == "/api/v1/pool-runner/session" {
			sessionCalls++
			if r.Header.Get("Authorization") != "Bearer "+token || r.URL.Query().Get("runner_id") != runnerID {
				t.Error("saved identity changed Pool machine scope")
			}
			out.WriteHeader(403)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/pool-runner-pairings/"+pair+"/") {
			t.Error("CLI entered business/project endpoint", r.URL.Path)
			out.WriteHeader(404)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/proposal") {
			token = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			_ = json.NewDecoder(r.Body).Decode(&proposal)
		}
		now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
		value := pp.PoolRunnerPairing{ID: pair, Purpose: "pool_runner", OwnerTenantID: strings.Repeat("6", 32), PoolID: pool, PoolVersion: 1, Version: 3, State: "approved", ControlURL: server.URL, Name: "Administrator machine", MaxSlots: 2, TTLSeconds: 86400, DeviceName: proposal.DeviceName, CredentialSHA256: proposal.CredentialSHA256, RunnerID: runnerID, WorkerID: "pool-" + runnerID, CredentialVersion: 1, CredentialExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339Nano), CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339Nano)}
		_ = json.NewEncoder(out).Encode(value)
	}))
	defer server.Close()
	var out, diagnostic bytes.Buffer
	args := []string{"--language=en", "--pair-pool-runner", pair, "--pool", pool, "--control", server.URL, "--pool-identity", file, "--device-name", "Machine reported name"}
	if code := runWithRunnerSecrets(args, &out, &diagnostic, noPairingEnvironmentCredential(t), store); code != 0 || calls != 2 || sessionCalls != 0 {
		t.Fatal("Pool pairing did not stop after exact current approval", code, diagnostic.String())
	}
	if strings.Contains(out.String(), token) || strings.Contains(diagnostic.String(), token) || strings.Contains(out.String(), "project_id") {
		t.Fatal("secret/project identity leaked in pairing output")
	}
	var result runner.PoolRunnerPairingResult
	if json.Unmarshal(bytes.TrimSpace(out.Bytes()), &result) != nil || !result.Prepared || result.RunnerID != runnerID {
		t.Fatal("approved public receipt missing", out.String())
	}
	workspace := t.TempDir()
	config := runner.PoolLocalConfig{SchemaVersion: 1, PoolID: pool, RunnerID: runnerID, Slots: 1, Scopes: []runner.PoolLocalScope{{TenantID: strings.Repeat("7", 32), ProjectID: strings.Repeat("8", 32), Workspace: workspace}}}
	configPath := filepath.Join(t.TempDir(), "pool.json")
	raw, _ := json.Marshal(config)
	if err := os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	diagnostic.Reset()
	if code := runWithRunnerSecrets([]string{"--language=en", "--pool-identity", file, "--control", server.URL, "--pool-config", configPath}, &out, &diagnostic, noPairingEnvironmentCredential(t), store); code != 1 || sessionCalls != 1 {
		t.Fatal("saved identity did not converge on original current PoolSession", code, diagnostic.String())
	}
	if strings.Contains(diagnostic.String(), token) {
		t.Fatal("Pool startup exposed saved secret")
	}
	if _, err := os.Stat(file + ".journal"); !os.IsNotExist(err) {
		t.Fatal("current Pool denial created or executed a Journal")
	}
}
