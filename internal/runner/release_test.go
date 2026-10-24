package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/releasecontract"
)

func TestReleaseV2LostPOSTACKInspectsStableOperationWithoutRedispatch(t *testing.T) {
	releaseRecoveryFixture(t, false)
}
func TestReleaseV2RecoveredIntentGetsFreshInMemoryTicket(t *testing.T) {
	releaseRecoveryFixture(t, true)
}
func releaseRecoveryFixture(t *testing.T, requiresAuthorization bool) {
	ref := releasecontract.AssignmentRef{RunID: "managed-release", NodeID: "deploy", Attempt: 1, Fence: 1, WorkerID: "deployer"}
	execution := releasecontract.ReleaseExecution{Protocol: releasecontract.ReleaseProtocol, OperationID: "release-operation-test", PlanID: "plan-1", PlanRevision: 1, PlanSHA256: strings.Repeat("a", 64), EnvironmentID: "env-1", ProviderID: "static", ExpectedGeneration: 0, Generation: 1, ArtifactID: "artifact-1", ArtifactSHA256: strings.Repeat("b", 64)}
	request := releasecontract.DeploymentRequest{ReleaseExecution: execution, Assignment: ref}
	requestHash, _ := releasecontract.DeploymentRequestDigest(request)
	obs := releasecontract.DeploymentObservation{Protocol: releasecontract.ReleaseProtocol, OperationID: execution.OperationID, RequestSHA256: requestHash, ProviderID: "static", PlanID: "plan-1", PlanRevision: 1, PlanSHA256: execution.PlanSHA256, EnvironmentID: "env-1", Generation: 1, FenceGeneration: 1, State: "succeeded", Version: 1, Effect: releasecontract.DeploymentEffect{State: "applied", ActiveGeneration: 1, ActiveArtifactSHA256: execution.ArtifactSHA256}, Health: releasecontract.HealthObservation{State: "passed", Generation: 1, ArtifactSHA256: execution.ArtifactSHA256, Checks: []releasecontract.HealthCheckResult{{Path: "/", StatusCode: 200, BodySHA256: strings.Repeat("c", 64)}}}}
	obs.EvidenceSHA256, _ = releasecontract.ObservationDigest(obs)
	var mu sync.Mutex
	created := false
	posts, authorizations, legacy := 0, 0, 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer release-worker-secret" {
			http.Error(w, "auth", 403)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /v2/operations/" + execution.OperationID:
			if !created {
				if requiresAuthorization {
					received := obs
					received.State = "received"
					received.Effect = releasecontract.DeploymentEffect{State: "not_started"}
					received.Health = releasecontract.HealthObservation{State: "not_checked", Generation: execution.Generation, ArtifactSHA256: execution.ArtifactSHA256, Checks: []releasecontract.HealthCheckResult{}}
					received.Error = &releasecontract.DeploymentError{Code: "authorization_required", Message: "authorization_required"}
					received.EvidenceSHA256, _ = releasecontract.ObservationDigest(received)
					_ = json.NewEncoder(w).Encode(received)
					return
				}
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(obs)
		case "POST /v2/deployments":
			var got releasecontract.DeploymentRequest
			_ = json.NewDecoder(r.Body).Decode(&got)
			if got.AuthorizationTicket != "temporary-opaque-ticket" {
				t.Error("missing ephemeral authorization")
			}
			if hash, _ := releasecontract.DeploymentRequestDigest(got); hash != requestHash {
				t.Error("binding changed")
			}
			posts++
			created = true
			conn, _, _ := w.(http.Hijacker).Hijack()
			_ = conn.Close()
		default:
			legacy++
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/runner/release-authorizations" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		authorizations++
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(releasecontract.ReleaseAuthorization{AuthorizationID: "authorization-1", AuthorizationTicket: "temporary-opaque-ticket", OperationID: execution.OperationID, ExpiresAt: "2099-01-01T00:00:00Z", RequestSHA256: requestHash})
	}))
	defer control.Close()
	var a Assignment
	raw, _ := json.Marshal(map[string]any{"run_id": ref.RunID, "node_id": ref.NodeID, "attempt": 1, "fence": 1, "worker_id": ref.WorkerID, "type": "external_workflow", "config": map[string]any{"plugin": "static", "workflow": "deploy"}, "release_execution": execution})
	_ = json.Unmarshal(raw, &a)
	var spec ExternalSpec
	raw, _ = json.Marshal(map[string]any{"url": provider.URL, "token_env": "SUMA_RELEASE_TEST_TOKEN", "protocol": releasecontract.ReleaseProtocol, "poll_milliseconds": 1, "cancel_wait_milliseconds": 1})
	_ = json.Unmarshal(raw, &spec)
	t.Setenv("SUMA_RELEASE_TEST_TOKEN", "release-worker-secret")
	dir := t.TempDir()
	j, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	cfg := ExecConfig{Artifacts: &Client{BaseURL: control.URL, ProjectID: "project", Token: "runner-secret"}, Journal: j, External: map[string]ExternalSpec{"static": spec}}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r := Execute(ctx, a, cfg)
	if r.ExitCode != 0 || r.OutcomeUnknown || r.Error != "" {
		t.Fatalf("lost ACK not recovered by operation: %+v", r)
	}
	if err = j.Finish(r); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(filepath.Join(dir, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), "temporary-opaque-ticket") || strings.Contains(string(bytes), "release-worker-secret") {
		t.Fatal("release credential persisted in journal")
	}
	r = Execute(context.Background(), a, cfg)
	if r.ExitCode != 0 {
		t.Fatal(r)
	}
	mu.Lock()
	defer mu.Unlock()
	if posts != 1 || authorizations != 1 || legacy != 0 {
		t.Fatalf("unexpected redispatch counts posts=%d auth=%d legacy=%d", posts, authorizations, legacy)
	}
}
