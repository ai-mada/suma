package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPoolConnectionCheckBoundsSourcesAndObservesOnlyConfiguredCurrentScopes(t *testing.T) {
	pool, runnerID, token := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 64)
	config := PoolLocalConfig{SchemaVersion: 1, PoolID: pool, RunnerID: runnerID, Slots: 1, Scopes: []PoolLocalScope{{TenantID: "tenant-one", ProjectID: "project-one", Workspace: t.TempDir(), Commands: map[string]CommandSpec{}}, {TenantID: "tenant-two", ProjectID: "project-two", Workspace: t.TempDir(), Commands: map[string]CommandSpec{}}}}
	raw, _ := json.MarshalIndent(config, "", "  ")
	raw = append(raw, '\n')
	file := filepath.Join(t.TempDir(), "private-config212.json")
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, fileSHA, err := ReadPoolConfigFile(file)
	if err != nil || fileSHA != digest(raw) || len(loaded.Scopes) != 2 {
		t.Fatal("bounded original config read failed", err)
	}
	cwd, _ := os.Getwd()
	relative, _ := filepath.Rel(cwd, file)
	if _, relativeSHA, err := ReadPoolConfigFile(relative); err != nil || relativeSHA != fileSHA {
		t.Fatal("ordinary relative config filename lost original bytes", err)
	}
	body := map[string]any{"runner": map[string]any{"id": runnerID, "pool_id": pool, "worker_id": "pool-" + runnerID, "max_slots": 2, "credential_version": 1, "name": "Owned machine", "version": 1, "status": "active", "credential_expires_at": "2099-01-01T00:00:00Z", "allowed_actions": []string{}, "created_at": "2026-11-02T00:00:00Z"}, "pool_active": true, "scopes": []any{map[string]any{"tenant_id": "tenant-one", "project_id": "project-one", "pool_id": pool, "grant_id": "owned-grant", "grant_version": 1, "project_weight": 1, "status": "active"}, map[string]any{"tenant_id": "server-extra", "project_id": "not-configured"}}}
	response, _ := json.Marshal(body)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, request *http.Request) {
		calls++
		if request.Method != "GET" || request.URL.Path != "/api/v1/pool-runner/session" || request.URL.Query().Get("runner_id") != runnerID || request.Header.Get("Authorization") != "Bearer "+token {
			t.Error("connection check entered another identity or write endpoint")
		}
		out.Header().Set("Content-Type", "application/json")
		_, _ = out.Write(response)
	}))
	defer server.Close()
	identity := PoolRunnerIdentity{Protocol: PoolRunnerIdentityProtocol, Purpose: poolRunnerPurpose, ControlURL: server.URL, PoolID: pool, PairingID: strings.Repeat("d", 32), ProposalOperationID: strings.Repeat("e", 32), DeviceName: "Owned machine", CredentialSHA256: digest([]byte(token)), OwnerTenantID: strings.Repeat("f", 32), RunnerID: runnerID, WorkerID: "pool-" + runnerID, CredentialVersion: 1, CredentialExpiresAt: "2099-01-01T00:00:00Z", MaxSlots: 2}
	client := &Client{BaseURL: server.URL, Token: token, Pool: &PoolContext{PoolID: pool, RunnerID: runnerID}}
	report, err := CheckPoolConnection(context.Background(), client, loaded, identity, fileSHA)
	if err != nil || calls != 1 || !report.PoolActive || len(report.Scopes) != 2 || !report.Scopes[0].Granted || report.Scopes[1].Granted || report.Scopes[1].ProjectID != "project-two" || report.ConfigurationFileSHA256 != fileSHA || report.CurrentMaxSlots != 2 || report.ConfiguredSlots != 1 {
		t.Fatal("connection check altered local scopes or current facts", report, err)
	}
	encoded, _ := json.Marshal(report)
	var fields map[string]any
	_ = json.Unmarshal(encoded, &fields)
	if len(fields) != 11 || report.Protocol != "suma-pool-connection-check-v1" || strings.Contains(string(encoded), file) || strings.Contains(string(encoded), token) {
		t.Fatal("connection check exposed private data or changed contract")
	}
	if _, err := time.Parse(time.RFC3339Nano, report.CheckedAt); err != nil {
		t.Fatal("check has no observation timestamp")
	}
	frozen, _ := resolvedPoolConfig(loaded)
	runtimeBytes, _ := json.Marshal(frozen)
	if report.ConfigurationFileSHA256 == digest(runtimeBytes) {
		t.Fatal("raw prepared file digest was replaced by runtime identity")
	}
	body["pool_active"] = false
	body["scopes"] = []any{}
	response, _ = json.Marshal(body)
	paused, err := CheckPoolConnection(context.Background(), client, loaded, identity, fileSHA)
	if err != nil || paused.PoolActive || paused.Scopes[0].Granted || paused.Scopes[1].Granted {
		t.Fatal("paused or ungranted observations became implicit readiness", err)
	}
	before := calls
	overslots := loaded
	overslots.Slots = 3
	if _, err := CheckPoolConnection(context.Background(), client, overslots, identity, fileSHA); err == nil || calls != before {
		t.Fatal("paired capacity mismatch reached network or was clamped")
	}
	foreign := identity
	foreign.ControlURL = "https://different.example.test"
	if _, err := CheckPoolConnection(context.Background(), client, loaded, foreign, fileSHA); err == nil || calls != before {
		t.Fatal("foreign identity reached current machine")
	}
	body["pool_active"] = true
	body["scopes"] = []any{map[string]string{"tenant_id": "tenant-one", "project_id": "project-one"}}
	valid, _ := json.Marshal(body)
	for _, invalid := range [][]byte{
		bytes.Replace(valid, []byte(`"pool_active":true,`), nil, 1),
		bytes.Replace(valid, []byte(`"pool_active":true`), []byte(`"pool_active":null`), 1),
		bytes.Replace(valid, []byte(`"pool_active":true`), []byte(`"pool_active":true,"pool_active":false`), 1),
		bytes.Replace(valid, []byte(`"tenant_id":"tenant-one"`), []byte(`"tenant_id":""`), 1),
		bytes.Replace(valid, []byte(`"runner":{`), []byte(`"runner":{"unexpected":true,`), 1),
		append(append([]byte{}, valid...), []byte(`{}`)...),
		append(append([]byte{}, valid...), bytes.Repeat([]byte(" "), 2<<20)...),
	} {
		response = invalid
		if _, err := client.PoolSession(context.Background()); err == nil {
			t.Fatal("incomplete, ambiguous or unbounded session accepted")
		}
	}
	body["scopes"] = []any{map[string]string{"tenant_id": "tenant-one", "project_id": "project-one"}, map[string]string{"tenant_id": "tenant-one", "project_id": "project-one"}}
	response, _ = json.Marshal(body)
	if _, err := client.PoolSession(context.Background()); err == nil {
		t.Fatal("duplicate current session scope accepted")
	}
	oversize := filepath.Join(t.TempDir(), "too-large.json")
	if err := os.WriteFile(oversize, bytes.Repeat([]byte("x"), (2<<20)+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadPoolConfigFile(oversize); err == nil {
		t.Fatal("oversized config read without bound")
	}
	link := filepath.Join(filepath.Dir(file), "linked.json")
	if os.Symlink(file, link) == nil {
		if _, _, err := ReadPoolConfigFile(link); err == nil {
			t.Fatal("linked config accepted")
		}
	}
}
