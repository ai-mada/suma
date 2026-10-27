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

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func nativeObservationFixture(t *testing.T) (*WorkspaceObservationRegistry, *nativeWorkspace, *attemptWorkspace, woc.Lease) {
	t.Helper()
	native, workspace, _ := nativeFixture(t, nil, nil)
	native.assignment.Config = map[string]any{"tool_profile_id": "profile"}
	registry := NewWorkspaceObservationRegistry("project")
	unregister, err := registry.register(native)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unregister)
	scope, policy, active := registry.active()
	if !active {
		t.Fatal("live host was not registered")
	}
	lease := woc.Lease{Observation: woc.Observation{ID: "observation-one", ProjectID: "project", Version: 2, State: "leased", Scope: scope, Operation: "read", Arguments: woc.Arguments{Path: "file.txt"}, WorkspacePolicySHA256: policy, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}, LeaseFence: 1, LeaseUntil: time.Now().Add(10 * time.Second).UTC().Format(time.RFC3339Nano)}
	return registry, native, workspace, lease
}

func TestWorkspaceObservationUsesOnlyTheActiveRootAndExactLease(t *testing.T) {
	registry, native, _, lease := nativeObservationFixture(t)
	root := native.root
	result := registry.Observe(context.Background(), lease)
	if result.ErrorCode != "" || result.ObservationResult == nil || result.ObservationResult.Read == nil || result.ObservationResult.Read.Content != "base\n" || result.ObservationResult.Read.SHA256 != digest([]byte("base\n")) || result.LeaseFence != lease.LeaseFence || result.Scope != lease.Observation.Scope || native.root != root || native.Evidence().Generation != 0 {
		t.Fatalf("native observation did not reuse the active readonly root: %+v", result)
	}
	for _, change := range []string{"project", "attempt", "fence", "policy", "expired", "write"} {
		modified := lease
		switch change {
		case "project":
			modified.Observation.ProjectID = "other"
		case "attempt":
			modified.Observation.Scope.Attempt++
		case "fence":
			modified.Observation.Scope.Fence++
		case "policy":
			modified.Observation.WorkspacePolicySHA256 = strings.Repeat("b", 64)
		case "expired":
			modified.LeaseUntil = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
		case "write":
			modified.Observation.Operation = "write"
		}
		observed := registry.Observe(context.Background(), modified)
		if observed.ErrorCode == "" || observed.ObservationResult != nil {
			t.Fatalf("%s drift disclosed a workspace result: %+v", change, observed)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if observed := registry.Observe(ctx, lease); observed.ErrorCode != "workspace_closed" || observed.ObservationResult != nil {
		t.Fatal("cancelled observation disclosed source")
	}
	if err := native.Close(); err != nil {
		t.Fatal(err)
	}
	if observed := registry.Observe(context.Background(), lease); observed.ErrorCode != "workspace_closed" || observed.ObservationResult != nil {
		t.Fatal("closed host reopened the historical source")
	}
}

func TestWorkspaceObservationKeepsNativePathBoundsAndRejectsSensitiveSource(t *testing.T) {
	registry, native, workspace, lease := nativeObservationFixture(t)
	for _, path := range []string{"../file.txt", ".git/config", ".env", "FILE.txt", "file.txt:stream"} {
		modified := lease
		modified.Observation.Arguments.Path = path
		result := registry.Observe(context.Background(), modified)
		if result.ErrorCode == "" || result.ObservationResult != nil {
			t.Fatalf("forbidden path disclosed source: %s", path)
		}
	}
	lease.Observation.ExpectedSHA256 = strings.Repeat("b", 64)
	if result := registry.Observe(context.Background(), lease); result.ErrorCode != "file_changed" || result.ObservationResult != nil {
		t.Fatal("stale expected file digest disclosed source")
	}
	lease.Observation.ExpectedSHA256 = ""
	native.secrets = []string{"owned-private-fixture-credential"}
	native.bridge.Token = "owned-private-fixture-bridge"
	for _, content := range []string{"owned-private-fixture-credential", "owned-private-fixture-bridge", "api_key=unapproved-literal", "source root: " + workspace.Source} {
		if err := os.WriteFile(filepath.Join(workspace.Source, "file.txt"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		result := registry.Observe(context.Background(), lease)
		raw, _ := json.Marshal(result)
		if result.ErrorCode != "sensitive_content" || result.ObservationResult != nil || strings.Contains(string(raw), content) {
			t.Fatal("sensitive content was disclosed or relabelled with a false digest")
		}
	}
	lease.Observation.Operation = "list"
	lease.Observation.Arguments = woc.Arguments{Limit: 1}
	result := registry.Observe(context.Background(), lease)
	if result.ErrorCode != "" || result.ObservationResult == nil || result.ObservationResult.List == nil || len(result.ObservationResult.List.Entries) != 1 || result.ObservationResult.List.Entries[0].Path != "file.txt" {
		t.Fatalf("bounded native list failed: %+v", result)
	}
}

func TestWorkspaceObservationJournalKeepsOneResultPerExactLeaseAcrossRestart(t *testing.T) {
	registry, native, _, lease := nativeObservationFixture(t)
	dir := t.TempDir()
	journal, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	const control = "https://control.example"
	assignment := native.assignment
	if err = journal.checkpointWorkspaceObservation(control, assignment, lease); err != nil {
		t.Fatal(err)
	}
	result := registry.Observe(context.Background(), lease)
	if err = journal.saveWorkspaceObservationResult(control, lease, result); err != nil {
		t.Fatal(err)
	}
	result.ObservationResult.Read.Content = "mutated external pointer"
	pending := journal.pendingWorkspaceObservations(control)
	if len(pending) != 1 || pending[0].Result.ObservationResult.Read.Content != "base\n" {
		t.Fatal("durable original result changed through caller alias")
	}
	pending[0].Result.ObservationResult.Read.Content = "mutated readback"
	if got := journal.pendingWorkspaceObservations(control); got[0].Result.ObservationResult.Read.Content != "base\n" {
		t.Fatal("returned result changed durable receipt")
	}
	if err = journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	restored := journal.pendingWorkspaceObservations(control)
	if len(restored) != 1 || restored[0].Result.ObservationResult.Read.Content != "base\n" || restored[0].Result.ResultSHA256 != woc.ResultDigest(restored[0].Result.ObservationResult, "") {
		t.Fatal("restart did not preserve original observation receipt")
	}
	if len(journal.pendingWorkspaceObservations("https://other.example")) != 0 {
		t.Fatal("a different Control origin acquired the original receipt")
	}
	if err = journal.ackWorkspaceObservation(control, woc.Receipt{RequestID: lease.Observation.ID, LeaseFence: 2, ResultSHA256: restored[0].Result.ResultSHA256, Acknowledged: true}); err == nil {
		t.Fatal("wrong observation lease fence acknowledged the original result")
	}
	if err = journal.ackWorkspaceObservation(control, woc.Receipt{RequestID: lease.Observation.ID, LeaseFence: 1, ResultSHA256: restored[0].Result.ResultSHA256, Acknowledged: true}); err != nil {
		t.Fatal(err)
	}
	if len(journal.pendingWorkspaceObservations(control)) != 0 {
		t.Fatal("acknowledged observation remained pending")
	}
	lease.LeaseFence++
	if err = journal.checkpointWorkspaceObservation(control, assignment, lease); err != nil {
		t.Fatal(err)
	}
	if err = journal.retireWorkspaceObservation(control, lease); err != nil {
		t.Fatal(err)
	}
	retired := journal.workspaceObservations[workspaceObservationKey(observationControlSHA(control), lease)]
	if !retired.Retired || retired.Acked {
		t.Fatal("local expiry or authority rejection was misreported as a server acknowledgement")
	}
}

func TestWorkspaceObservationBrokerReplaysOriginalResultAfterLostAcknowledgement(t *testing.T) {
	registry, native, workspace, lease := nativeObservationFixture(t)
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	var mu sync.Mutex
	callbacks := []woc.ResultRequest{}
	claims := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer owned-readonly-runner" {
			t.Error("observation omitted original machine credential")
		}
		switch r.URL.Path {
		case "/api/v1/runner/workspace-observations/claim":
			var request woc.ClaimRequest
			if json.NewDecoder(r.Body).Decode(&request) != nil || request.ProjectID != "project" || request.Scope != lease.Observation.Scope || request.WorkspacePolicySHA256 != native.policySHA {
				t.Error("claim changed the live host scope")
			}
			mu.Lock()
			claims++
			count := claims
			mu.Unlock()
			var returned *woc.Lease
			if count == 1 {
				returned = &lease
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"lease": returned})
		case "/api/v1/runner/workspace-observations/result":
			var result woc.ResultRequest
			if json.NewDecoder(r.Body).Decode(&result) != nil {
				t.Error("invalid observation callback")
			}
			mu.Lock()
			callbacks = append(callbacks, result)
			count := len(callbacks)
			mu.Unlock()
			if count == 1 {
				if err := os.WriteFile(filepath.Join(workspace.Source, "file.txt"), []byte("later source\n"), 0600); err != nil {
					t.Error(err)
				}
				connection, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = connection.Close()
				return
			}
			_ = json.NewEncoder(w).Encode(woc.Receipt{RequestID: result.RequestID, LeaseFence: result.LeaseFence, ResultSHA256: result.ResultSHA256, Acknowledged: true})
		default:
			t.Error("unexpected outbound observation route", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, ProjectID: "project", Token: "owned-readonly-runner"}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		runWorkspaceObservationBroker(ctx, client, journal, native.assignment, registry, 10*time.Millisecond)
	}()
	for ctx.Err() == nil {
		mu.Lock()
		count := len(callbacks)
		mu.Unlock()
		if count >= 2 && len(journal.pendingWorkspaceObservations(server.URL)) == 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done
	mu.Lock()
	defer mu.Unlock()
	if len(callbacks) != 2 || callbacks[0].ObservationResult == nil || callbacks[0].ObservationResult.Read.Content != "base\n" || woc.Digest(callbacks[0]) != woc.Digest(callbacks[1]) {
		t.Fatal("lost acknowledgement reread source or changed original scope/result")
	}
}

func TestWorkspaceObservationPoolClientKeepsOriginalAssignmentEnvelope(t *testing.T) {
	assignment := poolTestAssignment()
	scope := woc.Scope{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, ToolProfileID: "profile", ProfileSHA256: strings.Repeat("a", 64)}
	policy := strings.Repeat("b", 64)
	lease := woc.Lease{Observation: woc.Observation{ID: "pool-observation", ProjectID: "project", State: "leased", Scope: scope, Operation: "list", Arguments: woc.Arguments{}, WorkspacePolicySHA256: policy, ExpiresAt: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}, LeaseFence: 3, LeaseUntil: time.Now().Add(10 * time.Second).UTC().Format(time.RFC3339Nano)}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]json.RawMessage
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid Pool observation body")
		}
		var got Assignment
		_ = json.Unmarshal(body["assignment"], &got)
		if !samePoolAssignmentIdentity(got, assignment) || string(body["runner_id"]) != `"runner"` || string(body["tenant_id"]) != `"tenant"` {
			t.Error("Pool observation lost frozen bilateral scope")
		}
		switch r.URL.Path {
		case "/api/v1/pool-runner/workspace-observations/claim":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease": lease})
		case "/api/v1/pool-runner/workspace-observations/result":
			if _, exists := body["result"]; exists {
				t.Error("observation used the DAG result envelope")
			}
			if _, exists := body["observation_result"]; !exists {
				t.Error("observation result wrapper missing")
			}
			var result woc.ResultRequest
			raw, _ := json.Marshal(body)
			_ = json.Unmarshal(raw, &result)
			_ = json.NewEncoder(w).Encode(woc.Receipt{RequestID: result.RequestID, LeaseFence: result.LeaseFence, ResultSHA256: result.ResultSHA256, Acknowledged: true})
		default:
			t.Error("unexpected Pool route", r.URL.Path)
		}
	}))
	defer server.Close()
	base := &Client{BaseURL: server.URL, Token: "owned-pool-token", Pool: &PoolContext{PoolID: assignment.PoolID, RunnerID: assignment.PoolRunnerID}}
	client, err := base.forPoolAssignment(assignment)
	if err != nil {
		t.Fatal(err)
	}
	received, err := client.ClaimWorkspaceObservation(context.Background(), woc.ClaimRequest{ProjectID: "project", WorkerID: assignment.WorkerID, Scope: scope, WorkspacePolicySHA256: policy, OperationID: operationID()})
	if err != nil || received == nil {
		t.Fatal("scoped Pool observation claim failed", err)
	}
	result := woc.ResultRequest{ProjectID: "project", WorkerID: assignment.WorkerID, Scope: scope, RequestID: lease.Observation.ID, LeaseFence: 3, WorkspacePolicySHA256: policy, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), ObservationResult: &woc.Result{List: &woc.List{Entries: []woc.Entry{}, Complete: true}}, OperationID: operationID()}
	result.ResultSHA256 = woc.ResultDigest(result.ObservationResult, "")
	if _, err = client.CompleteWorkspaceObservation(context.Background(), result); err != nil {
		t.Fatal("scoped Pool observation callback failed", err)
	}
	if calls != 2 {
		t.Fatal("unexpected Pool observation call count", calls)
	}
}

func TestWorkspaceObservationLeaseDecoderRejectsCaseAliasesAndUnknownPayloads(t *testing.T) {
	_, _, _, lease := nativeObservationFixture(t)
	raw, _ := json.Marshal(map[string]any{"lease": lease})
	for _, invalid := range []string{strings.Replace(string(raw), `"scope":`, `"SCOPE":`, 1), strings.Replace(string(raw), `"operation":"read"`, `"operation":"read","endpoint":"https://unused.invalid"`, 1), `{"lease":null,"lease":null}`, `{"lease":null,"extra":true}`} {
		var response workspaceObservationClaimReply
		if json.Unmarshal([]byte(invalid), &response) == nil {
			t.Fatal("observation wire accepted a case alias, duplicate or unknown field")
		}
	}
}

func TestWorkspaceObservationFullFilePreservesEveryLineWithinExistingByteLimits(t *testing.T) {
	registry, _, workspace, lease := nativeObservationFixture(t)
	content := strings.Repeat("x\n", 3000)
	if err := os.WriteFile(filepath.Join(workspace.Source, "file.txt"), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	lease.Observation.Arguments = woc.Arguments{Path: "file.txt", FullFile: true}
	result := registry.Observe(context.Background(), lease)
	if result.ErrorCode != "" || result.ObservationResult == nil || result.ObservationResult.Read == nil || !result.ObservationResult.Read.Complete || result.ObservationResult.Read.Content != content {
		t.Fatal("bounded whole file read lost short lines or forwarded full_file as a native parameter")
	}
	if err := os.WriteFile(filepath.Join(workspace.Source, "file.txt"), []byte(strings.Repeat("x", (48<<10)+1)), 0600); err != nil {
		t.Fatal(err)
	}
	result = registry.Observe(context.Background(), lease)
	if result.ErrorCode != "output_limit" || result.ObservationResult != nil {
		t.Fatal("whole file read exceeded the bounded editable text limit")
	}
}
