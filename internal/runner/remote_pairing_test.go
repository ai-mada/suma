package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	rp "github.com/ai-mada/suma/internal/runnerpairingcontract"
)

func pairingObservation(control string, proposal rp.RunnerPairingProposalRequest, approved bool) rp.RunnerPairing {
	created := time.Now().UTC().Truncate(time.Second)
	value := rp.RunnerPairing{ID: identityTestPair, ProjectID: proposal.ProjectID, Version: 2, State: "proposed", ControlURL: control, CreatedAt: created.Format(time.RFC3339), ExpiresAt: created.Add(10 * time.Minute).Format(time.RFC3339), DeviceName: proposal.DeviceName, CredentialSHA256: proposal.CredentialSHA256}
	if approved {
		value.State = "approved"
		value.Version = 3
		value.WorkerID = "remote-" + strings.Repeat("a", 32)
		value.CredentialExpiresAt = created.Add(24 * time.Hour).Format(time.RFC3339)
	}
	return value
}

func TestRemotePairingPersistsBeforeHTTPAndRechecksApprovedStatus(t *testing.T) {
	file := filepath.Join(t.TempDir(), "identity.json")
	store := newIdentityMemoryStore()
	var proposal rp.RunnerPairingProposalRequest
	requests := []string{}
	var originalToken string
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Path)
		if r.Method != "POST" || !strings.HasPrefix(r.URL.Path, "/control/api/v1/runner-pairings/"+identityTestPair+"/") {
			t.Error("pairing reached an execution or foreign endpoint", r.URL.Path)
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if originalToken == "" {
			originalToken = token
		} else if originalToken != token {
			t.Error("approved status changed the original machine secret")
		}
		raw, err := os.ReadFile(file)
		if err != nil || bytes.Contains(raw, []byte(token)) {
			t.Error("network preceded safe local metadata", err)
		}
		if !bytes.Contains(raw, []byte(`"proposal_operation_id"`)) || store.sets != 1 {
			t.Error("original operation/system secret absent before HTTP")
		}
		if strings.HasSuffix(r.URL.Path, "/proposal") {
			if json.NewDecoder(r.Body).Decode(&proposal) != nil || proposal.CredentialSHA256 != digest([]byte(token)) {
				t.Error("proposal lost exact original digest")
			}
			_ = json.NewEncoder(w).Encode(pairingObservation(server.URL+"/control", proposal, false))
			return
		}
		var status rp.RunnerPairingStatusRequest
		if json.NewDecoder(r.Body).Decode(&status) != nil || status.ProjectID != proposal.ProjectID || status.IdempotencyKey != proposal.IdempotencyKey {
			t.Error("status lost original proposal operation")
		}
		_ = json.NewEncoder(w).Encode(pairingObservation(server.URL+"/control", proposal, true))
	}))
	defer server.Close()
	var output bytes.Buffer
	options := RunnerPairingOptions{PairingID: identityTestPair, ProjectID: identityTestProject, ControlURL: server.URL + "/control/", IdentityPath: file, DeviceName: "owned host", Secrets: store, Observe: func(value RunnerPairingResult) error { return json.NewEncoder(&output).Encode(value) }}
	result, err := PairRemoteRunner(context.Background(), options)
	if err != nil || !result.Prepared || result.State != "approved" || len(requests) != 2 || bytes.Contains(output.Bytes(), []byte(originalToken)) {
		t.Fatalf("pairing failed or leaked secret: %+v %v requests=%v", result, err, requests)
	}
	identity, err := LoadRunnerIdentity(file, store)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Identity().WorkerID != result.WorkerID || identity.Token() != originalToken {
		t.Fatal("approved identity was not persisted")
	}
	_ = identity.Close()
	if _, err = PairRemoteRunner(context.Background(), options); err != nil || len(requests) != 3 || !strings.HasSuffix(requests[2], "/status") || store.sets != 1 {
		t.Fatal("approved resume issued another proposal or secret", err, requests)
	}
}

func TestRemotePairingLostReplyReplaysExactProposalWithoutReplacingSecret(t *testing.T) {
	file := filepath.Join(t.TempDir(), "identity.json")
	store := newIdentityMemoryStore()
	bodies := [][]byte{}
	tokens := []string{}
	var proposal rp.RunnerPairingProposalRequest
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/proposal") {
			raw, _ := io.ReadAll(r.Body)
			bodies = append(bodies, raw)
			tokens = append(tokens, r.Header.Get("Authorization"))
			_ = json.Unmarshal(raw, &proposal)
			if len(bodies) == 1 {
				_, _ = w.Write([]byte(`{"id":`))
				return
			}
			_ = json.NewEncoder(w).Encode(pairingObservation(server.URL, proposal, false))
			return
		}
		_ = json.NewEncoder(w).Encode(pairingObservation(server.URL, proposal, true))
	}))
	defer server.Close()
	options := RunnerPairingOptions{PairingID: identityTestPair, ProjectID: identityTestProject, ControlURL: server.URL, IdentityPath: file, DeviceName: "owned host", Secrets: store}
	if _, err := PairRemoteRunner(context.Background(), options); err == nil {
		t.Fatal("truncated proposal ACK was considered success")
	}
	if _, err := PairRemoteRunner(context.Background(), options); err != nil || len(bodies) != 2 || !bytes.Equal(bodies[0], bodies[1]) || tokens[0] != tokens[1] || store.sets != 1 {
		t.Fatal("uncertain proposal was replaced", err)
	}
}

func TestRemotePairingRejectsForeignOrIncompleteResponseBeforeApproval(t *testing.T) {
	for _, kind := range []string{"foreign_digest", "missing_can_approve", "unknown_field", "duplicate_field", "approved_browser_reason"} {
		t.Run(kind, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "identity.json")
			store := newIdentityMemoryStore()
			var proposal rp.RunnerPairingProposalRequest
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/proposal") {
					_ = json.NewDecoder(r.Body).Decode(&proposal)
				}
				value := pairingObservation(server.URL, proposal, true)
				if kind == "approved_browser_reason" {
					value.ReasonCode = "credential_unavailable"
				}
				raw, _ := json.Marshal(value)
				if strings.HasSuffix(r.URL.Path, "/proposal") {
					switch kind {
					case "foreign_digest":
						value.CredentialSHA256 = strings.Repeat("f", 64)
						raw, _ = json.Marshal(value)
					case "missing_can_approve":
						raw = bytes.Replace(raw, []byte(`"can_approve":false,`), nil, 1)
					case "unknown_field":
						raw = append([]byte(`{"token":"must-not-accept",`), raw[1:]...)
					case "duplicate_field":
						raw = append([]byte(`{"can_approve":false,`), raw[1:]...)
					}
				}
				_, _ = w.Write(raw)
			}))
			defer server.Close()
			_, err := PairRemoteRunner(context.Background(), RunnerPairingOptions{PairingID: identityTestPair, ProjectID: identityTestProject, ControlURL: server.URL, IdentityPath: file, DeviceName: "owned host", Secrets: store})
			if err == nil {
				t.Fatal("foreign or incomplete wire facts granted a prepared identity")
			}
			identity, err := LoadRunnerIdentity(file, store)
			if err != nil {
				t.Fatal(err)
			}
			defer identity.Close()
			if identity.Identity().WorkerID != "" {
				t.Fatal("unconfirmed identity became approved")
			}
		})
	}
}

func TestRemotePairingCancellationPreservesOriginalAndUnavailableStorePreventsHTTP(t *testing.T) {
	file := filepath.Join(t.TempDir(), "identity.json")
	store := newIdentityMemoryStore()
	requests := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		var proposal rp.RunnerPairingProposalRequest
		_ = json.NewDecoder(r.Body).Decode(&proposal)
		_ = json.NewEncoder(w).Encode(pairingObservation(server.URL, proposal, false))
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	options := RunnerPairingOptions{PairingID: identityTestPair, ProjectID: identityTestProject, ControlURL: server.URL, IdentityPath: file, DeviceName: "owned host", Secrets: store, Observe: func(RunnerPairingResult) error { cancel(); return nil }}
	if _, err := PairRemoteRunner(ctx, options); err == nil || requests != 1 {
		t.Fatal("cancelled pairing ran another operation", err, requests)
	}
	identity, err := LoadRunnerIdentity(file, store)
	if err != nil {
		t.Fatal(err)
	}
	original := identity.Identity()
	_ = identity.Close()
	store.getError = fmt.Errorf("system store is unavailable")
	options.IdentityPath = filepath.Join(t.TempDir(), "other.json")
	if _, err = PairRemoteRunner(context.Background(), options); err == nil || requests != 1 {
		t.Fatal("unavailable system store reached the control plane")
	}
	store.getError = nil
	identity, err = LoadRunnerIdentity(file, store)
	if err != nil {
		t.Fatal(err)
	}
	defer identity.Close()
	if identity.Identity() != original || store.sets != 1 {
		t.Fatal("cancellation lost the original local operation")
	}
}

func TestRemotePairingRejectsRedirectWithoutForwardingMachineSecret(t *testing.T) {
	forwarded := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forwarded++; w.WriteHeader(500) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	_, err := PairRemoteRunner(context.Background(), RunnerPairingOptions{PairingID: identityTestPair, ProjectID: identityTestProject, ControlURL: server.URL, IdentityPath: filepath.Join(t.TempDir(), "identity.json"), DeviceName: "owned host", Secrets: newIdentityMemoryStore(), HTTP: &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return nil }}})
	if err == nil || forwarded != 0 {
		t.Fatal("pairing followed a redirect with machine credentials", err, forwarded)
	}
}

func TestRemotePairingTerminalDecisionPreservesTheUnapprovedIdentity(t *testing.T) {
	for _, state := range []string{"rejected", "expired"} {
		t.Run(state, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "identity.json")
			store := newIdentityMemoryStore()
			calls := 0
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var proposal rp.RunnerPairingProposalRequest
				_ = json.NewDecoder(r.Body).Decode(&proposal)
				value := pairingObservation(server.URL, proposal, false)
				value.State = state
				_ = json.NewEncoder(w).Encode(value)
			}))
			defer server.Close()
			result, err := PairRemoteRunner(context.Background(), RunnerPairingOptions{PairingID: identityTestPair, ProjectID: identityTestProject, ControlURL: server.URL, IdentityPath: file, DeviceName: "owned host", Secrets: store})
			if err == nil || result.Prepared || result.State != state || calls != 1 || store.sets != 1 {
				t.Fatal("terminal decision created another proposal or approved identity", result, err, calls)
			}
			identity, err := LoadRunnerIdentity(file, store)
			if err != nil {
				t.Fatal(err)
			}
			defer identity.Close()
			if identity.Identity().WorkerID != "" {
				t.Fatal("terminal decision persisted an authorized worker")
			}
		})
	}
}
