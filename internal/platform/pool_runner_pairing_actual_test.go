package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/runner"
	"github.com/zalando/go-keyring"
)

// Only initially absent, task-owned credential accounts may be written or
// removed. This wrapper never enumerates the user's system credential store.
type poolPairingActualVault struct {
	mu       sync.Mutex
	provider runner.RunnerSecretStore
	missing  map[string]bool
	owned    map[string][3]string // service, account, exact expected digest
}

func (v *poolPairingActualVault) Get(service, account string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	value, err := v.provider.Get(service, account)
	if errors.Is(err, runner.ErrRunnerSecretNotFound) {
		v.missing[service+":"+account] = true
	}
	return value, err
}
func (v *poolPairingActualVault) Set(service, account, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	key := service + ":" + account
	if !v.missing[key] || service != runner.PoolRunnerIdentityProtocol {
		return fmt.Errorf("refusing an unowned credential write")
	}
	v.owned[key] = [3]string{service, account, digest(value)}
	return v.provider.Set(service, account, value)
}

// This opt-in journey uses one isolated organization/SQLite and an actual
// Windows credential store. A fixture owner approves only this owned test
// machine. No user project, model, vendor, task or production environment runs.
func TestPoolRunnerPairingActualSystemVaultHTTPAndWorkspace(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("SUMA_POOL_PAIRING_208_ACTUAL") != "1" {
		t.Skip("explicit owned system-vault and browser journey required")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	node := os.Getenv("SUMA_POOL_PAIRING_208_NODE")
	if !filepath.IsAbs(node) {
		t.Fatal("existing absolute Node executable required")
	}
	dir := filepath.Join(root, ".cache", "pool-runner-pairing-208", "actual", strings.ReplaceAll(time.Now().UTC().Format("20060102T150405.000000000Z"), ".", ""))
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{"stage": "T-POOL-RUNNER-PAIRING-208", "scope": "actual isolated HTTP/SQLite, real system vault and production PoolWorkspace; fixture approval, no vendor/model/task", "passed": false}
	defer func() {
		raw, _ := json.MarshalIndent(receipt, "", "  ")
		_ = os.WriteFile(filepath.Join(dir, "receipt.json"), append(raw, '\n'), 0600)
	}()
	vault := &poolPairingActualVault{provider: runner.SystemRunnerSecretStore(), missing: map[string]bool{}, owned: map[string][3]string{}}
	defer func() {
		vault.mu.Lock()
		defer vault.mu.Unlock()
		removed := 0
		for _, entry := range vault.owned {
			value, e := vault.provider.Get(entry[0], entry[1])
			if e != nil || digest(value) != entry[2] {
				t.Error("owned credential changed; refusing deletion")
				continue
			}
			if e = keyring.Delete(entry[0], entry[1]); e != nil {
				t.Error("owned credential cleanup failed")
				continue
			}
			if _, e = vault.provider.Get(entry[0], entry[1]); !errors.Is(e, runner.ErrRunnerSecretNotFound) {
				t.Error("owned credential absence not confirmed")
				continue
			}
			removed++
		}
		receipt["owned_system_credentials_removed"] = removed
		receipt["owned_system_credentials_created"] = len(vault.owned)
	}()
	f := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool, err := f.s.CreateExecutionPool(ctx, f.e, f.actor, ExecutionPoolCreateRequest{Name: "Owned Pool pairing208", Description: "Isolated machine enrollment", Policy: ExecutionPoolPolicyInput{ConcurrencyLimit: 2, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{}}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	view := f.call("GET", "/auth/session", nil, 200)
	defer func() {
		_, e := f.s.DB.ExecContext(context.Background(), "VACUUM INTO ?", filepath.Join(dir, "private.sqlite"))
		if e != nil {
			t.Error("private fixture snapshot unavailable")
		}
	}()
	server := &Server{Service: f.s, Engine: f.e, WebDir: filepath.Join(root, ".cache", "pool-runner-pairing-208", "web", "out")}
	created := make(chan string, 1)
	machineDone := make(chan error, 1)
	actionKey := newID() + newID()
	var mu sync.Mutex
	requests := []map[string]any{}
	losses := map[string]bool{}
	var result runner.PoolRunnerPairingResult
	var firstFailure bool
	var pairID string
	identity := filepath.Join(dir, "pool-identity.json")
	platformHTTP := server.Handler()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__fixture/view" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"pool_id": pool.ID, "session": view})
			return
		}
		if r.URL.Path == "/__fixture/action" {
			if r.Method != "POST" || r.Header.Get("X-Suma-Fixture-Key") != actionKey {
				w.WriteHeader(403)
				return
			}
			var input struct {
				Action string `json:"action"`
			}
			if json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&input) != nil {
				w.WriteHeader(400)
				return
			}
			switch input.Action {
			case "machine":
				mu.Lock()
				value := map[string]any{"first_proposal_ack_lost": firstFailure, "pairing_id": pairID, "prepared": result.Prepared, "fingerprint": result.CredentialSHA256}
				mu.Unlock()
				if raw, e := os.ReadFile(identity); e == nil {
					var meta runner.PoolRunnerIdentity
					if json.Unmarshal(raw, &meta) == nil {
						value["fingerprint"] = meta.CredentialSHA256
					}
				}
				_ = json.NewEncoder(w).Encode(value)
			case "session":
				mu.Lock()
				current := result
				mu.Unlock()
				if !current.Prepared {
					w.WriteHeader(409)
					return
				}
				h, e := runner.LoadPoolRunnerIdentity(identity, server.PublicURL, pool.ID, current.RunnerID, vault)
				if e != nil {
					w.WriteHeader(500)
					return
				}
				token := h.Token()
				_ = h.Close()
				client := &runner.Client{BaseURL: server.PublicURL, Token: token, Pool: &runner.PoolContext{PoolID: pool.ID, RunnerID: current.RunnerID}}
				session, e := client.PoolSession(r.Context())
				if e != nil || session.Runner.ID != current.RunnerID || !session.PoolActive || len(session.Scopes) != 0 {
					w.WriteHeader(500)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]bool{"original_session_verified": true})
			case "revoke":
				mu.Lock()
				current := result
				mu.Unlock()
				if !current.Prepared {
					w.WriteHeader(409)
					return
				}
				_, e := f.s.RevokeExecutionPoolRunner(r.Context(), f.actor, pool.ID, current.RunnerID, ExecutionPoolCommandRequest{Action: "revoke", ExpectedVersion: 1, IdempotencyKey: newID()})
				if e != nil {
					w.WriteHeader(500)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]bool{"revoked": true})
			default:
				w.WriteHeader(400)
			}
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			platformHTTP.ServeHTTP(w, r)
			return
		}
		raw, e := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if e != nil {
			w.WriteHeader(400)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(raw))
		response := httptest.NewRecorder()
		platformHTTP.ServeHTTP(response, r)
		kind := ""
		if r.Method == "POST" && r.URL.Path == "/api/v1/execution-pools/"+pool.ID+"/runner-pairings" {
			kind = "create"
		} else if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/api/v1/pool-runner-pairings/") && strings.HasSuffix(r.URL.Path, "/proposal") {
			kind = "proposal"
		} else if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/commands") {
			kind = "approve"
		}
		mu.Lock()
		requests = append(requests, map[string]any{"method": r.Method, "path": r.URL.Path, "request_sha256": digest(string(raw)), "status": response.Code})
		if kind == "create" && response.Code == 201 && pairID == "" {
			var value PoolRunnerPairing
			if json.Unmarshal(response.Body.Bytes(), &value) == nil {
				pairID = value.ID
				created <- value.ID
			}
		}
		drop := kind != "" && response.Code >= 200 && response.Code < 300 && !losses[kind]
		if drop {
			losses[kind] = true
		}
		mu.Unlock()
		for name, values := range response.Header() {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		if drop {
			w.Header().Set("Content-Length", strconv.Itoa(response.Body.Len()+23))
			w.WriteHeader(response.Code)
			body := response.Body.Bytes()
			if len(body) > 8 {
				body = body[:8]
			}
			_, _ = w.Write(body)
			return
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	})
	host := httptest.NewUnstartedServer(handler)
	server.PublicURL = "http://" + host.Listener.Addr().String()
	host.Start()
	defer host.Close()
	receipt["origin"] = server.PublicURL
	go func() {
		var id string
		select {
		case id = <-created:
		case <-ctx.Done():
			machineDone <- ctx.Err()
			return
		}
		options := runner.PoolRunnerPairingOptions{PairingID: id, ControlURL: server.PublicURL, PoolID: pool.ID, IdentityPath: identity, DeviceName: "Owned system-vault Pool208", Secrets: vault}
		_, e := runner.PairPoolRunner(ctx, options)
		if e == nil {
			machineDone <- fmt.Errorf("lost committed proposal acknowledgement unexpectedly appeared successful")
			return
		}
		mu.Lock()
		firstFailure = true
		mu.Unlock()
		value, e := runner.PairPoolRunner(ctx, options)
		mu.Lock()
		result = value
		mu.Unlock()
		machineDone <- e
	}()
	input, _ := json.Marshal(map[string]any{"origin": server.PublicURL, "directory": dir, "pool_id": pool.ID, "identity_file": identity, "cookie": map[string]string{"name": f.cookie.Name, "value": f.cookie.Value}, "action_key": actionKey})
	driver := exec.CommandContext(ctx, node, filepath.Join(root, "tests", "integration", "pool-runner-pairing", "driver.cjs"))
	driver.Dir = root
	driver.Stdin = bytes.NewReader(input)
	output, e := driver.CombinedOutput()
	_ = os.WriteFile(filepath.Join(dir, "driver.log"), output, 0600)
	if e != nil {
		cancel()
		select {
		case <-machineDone:
		case <-time.After(5 * time.Second):
		}
		t.Fatalf("isolated browser journey failed; see %s", filepath.Join(dir, "driver.log"))
	}
	select {
	case err = <-machineDone:
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		t.Fatal("original Pool pairing did not recover its machine identity", err)
	}
	mu.Lock()
	current := result
	lost := losses
	hadFailure := firstFailure
	mu.Unlock()
	if !current.Prepared || !hadFailure || !lost["create"] || !lost["proposal"] || !lost["approve"] {
		t.Fatal("three original lost acknowledgements were not recovered")
	}
	handle, e := runner.LoadPoolRunnerIdentity(identity, server.PublicURL, pool.ID, current.RunnerID, vault)
	if e != nil {
		t.Fatal("original system credential could not be reloaded", e)
	}
	token := handle.Token()
	meta := handle.Identity()
	_ = handle.Close()
	if digest(token) != meta.CredentialSHA256 || meta.WorkerID != current.WorkerID {
		t.Fatal("identity differs from the original system credential")
	}
	if raw, e := os.ReadFile(identity); e != nil || bytes.Contains(raw, []byte(token)) {
		t.Fatal("public identity exposed the credential")
	}
	client := &runner.Client{BaseURL: server.PublicURL, Token: token, Pool: &runner.PoolContext{PoolID: pool.ID, RunnerID: current.RunnerID}}
	// The browser driver revokes only after the approved read/continue evidence.
	// The original protocol must now refuse the unchanged saved credential.
	if _, e = client.PoolSession(ctx); e == nil {
		t.Fatal("revoked Pool credential retained original session authority")
	}
	for table, want := range map[string]int{"suma_pool_runner_pairings": 1, "suma_execution_pool_runners": 1, "suma_execution_pool_credentials": 1, "suma_execution_pool_grants": 0, "suma_release_workers": 0, "dag_runs": 0} {
		if got := domainACount(t, f.s, table); got != want {
			t.Fatalf("%s: %d instead of %d", table, got, want)
		}
	}
	mu.Lock()
	receipts := append([]map[string]any(nil), requests...)
	mu.Unlock()
	receipt["pool_id"], receipt["pairing_id"], receipt["runner_id"], receipt["worker_id"] = pool.ID, current.PairingID, current.RunnerID, current.WorkerID
	receipt["lost_acknowledgements"], receipt["requests"], receipt["original_machine_recovered"], receipt["revoked_session_refused"] = lost, receipts, true, true
	receipt["grants"], receipt["tasks_or_runs"], receipt["model_calls"], receipt["passed"] = 0, 0, 0, true
	t.Log("one actual system credential and Pool machine, three original ACK recoveries, fixture browser approval, revoked original session refused, no project grants/tasks/models; exact owned credential cleanup required")
}
