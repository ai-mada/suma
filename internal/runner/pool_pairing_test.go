package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pp "github.com/ai-mada/suma/internal/poolrunnerpairingcontract"
)

func poolPairingFixture208(identity PoolRunnerIdentity, version int, state string) pp.PoolRunnerPairing {
	now := time.Date(2026, 11, 2, 8, 0, 0, 0, time.UTC)
	v := pp.PoolRunnerPairing{ID: identity.PairingID, Purpose: "pool_runner", OwnerTenantID: strings.Repeat("8", 32), PoolID: identity.PoolID, PoolVersion: 1, Version: version, State: state, ControlURL: identity.ControlURL, Name: "Administrator fixed Pool name", MaxSlots: 2, TTLSeconds: 86400, DeviceName: identity.DeviceName, CredentialSHA256: identity.CredentialSHA256, CreatedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(10 * time.Minute).Format(time.RFC3339Nano)}
	if state == "approved" {
		v.RunnerID = strings.Repeat("9", 32)
		v.WorkerID = "pool-" + v.RunnerID
		v.CredentialVersion = 1
		v.CredentialExpiresAt = now.Add(24 * time.Hour).Format(time.RFC3339Nano)
	}
	return v
}

func TestPoolPairingLostAckRestoresOriginalSecretAndRechecksCurrentStatus(t *testing.T) {
	file := filepath.Join(t.TempDir(), "identity.json")
	store := newIdentityMemoryStore()
	var proposals []pp.PoolRunnerPairingProposalRequest
	var tokens []string
	var original PoolRunnerIdentity
	statuses := 0
	deny := false
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/pool-runner-pairings/"+poolPair208+"/") {
			t.Error("pairing entered a project/Worker endpoint")
		}
		if len(store.values) != 1 {
			t.Error("HTTP preceded system secret persistence")
		}
		if strings.HasSuffix(r.URL.Path, "/proposal") {
			var req pp.PoolRunnerPairingProposalRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			proposals = append(proposals, req)
			tokens = append(tokens, strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			raw, e := os.ReadFile(file)
			if e != nil || json.Unmarshal(raw, &original) != nil {
				t.Error("proposal metadata was not durable")
			}
			if req.CredentialSHA256 != digest([]byte(tokens[len(tokens)-1])) || req.IdempotencyKey != original.ProposalOperationID {
				t.Error("wrong original digest/proposal")
			}
			if len(proposals) == 1 {
				out.WriteHeader(200)
				_, _ = fmt.Fprint(out, `{"id":`)
				return
			}
			_ = json.NewEncoder(out).Encode(poolPairingFixture208(original, 3, "approved"))
			return
		}
		statuses++
		if deny {
			out.WriteHeader(403)
			return
		}
		_ = json.NewEncoder(out).Encode(poolPairingFixture208(original, 3, "approved"))
	}))
	defer server.Close()
	options := PoolRunnerPairingOptions{PairingID: poolPair208, ControlURL: server.URL, PoolID: poolIdentity208, IdentityPath: file, DeviceName: "machine reported name", Secrets: store, pollInterval: time.Millisecond}
	if result, err := PairPoolRunner(context.Background(), options); err == nil || result.Prepared {
		t.Fatal("truncated proposal ACK became approval")
	}
	result, err := PairPoolRunner(context.Background(), options)
	if err != nil || !result.Prepared || statuses != 1 || len(proposals) != 2 || proposals[0] != proposals[1] || tokens[0] != tokens[1] || store.sets != 1 {
		t.Fatal("original proposal/secret was replaced or current status skipped", err)
	}
	deny = true
	if result, err = PairPoolRunner(context.Background(), options); err == nil || result.Prepared || statuses != 2 || len(proposals) != 2 {
		t.Fatal("old approved metadata bypassed current server authority")
	}
	public, _ := os.ReadFile(file)
	if strings.Contains(string(public), tokens[0]) {
		t.Fatal("Pool secret entered public identity")
	}
}

func TestPoolPairingRejectsChangedSourceAndCancelledWait(t *testing.T) {
	for _, mode := range []string{"changed_owner", "changed_name", "project_purpose", "cancel", "redirect", "approved_reason"} {
		t.Run(mode, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "identity.json")
			store := newIdentityMemoryStore()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var original PoolRunnerIdentity
			var basis pp.PoolRunnerPairing
			server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				if mode == "redirect" {
					http.Redirect(out, r, "https://must-not-follow.invalid", 302)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/proposal") {
					raw, _ := os.ReadFile(file)
					_ = json.Unmarshal(raw, &original)
					basis = poolPairingFixture208(original, 2, "proposed")
					if mode == "project_purpose" {
						basis.Purpose = "project_runner"
					}
					_ = json.NewEncoder(out).Encode(basis)
					return
				}
				value := basis
				value.Version++
				switch mode {
				case "changed_owner":
					value.OwnerTenantID = strings.Repeat("a", 32)
				case "changed_name":
					value.Name = "different frozen name"
				case "approved_reason":
					value = poolPairingFixture208(original, 3, "approved")
					value.ReasonCode = "credential_unavailable"
				case "cancel":
					cancel()
				}
				_ = json.NewEncoder(out).Encode(value)
			}))
			defer server.Close()
			result, err := PairPoolRunner(ctx, PoolRunnerPairingOptions{PairingID: poolPair208, ControlURL: server.URL, PoolID: poolIdentity208, IdentityPath: file, DeviceName: "machine", Secrets: store, pollInterval: time.Millisecond, waitLimit: time.Second})
			if err == nil || result.Prepared || store.sets != 1 {
				t.Fatal("unproven pairing source became approved", mode, err)
			}
			if info, e := os.Stat(file); e != nil || !info.Mode().IsRegular() {
				t.Fatal("failed pairing lost original recovery metadata")
			}
		})
	}
}
