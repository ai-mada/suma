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
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func nativeWriteFixture(t *testing.T) (*WorkspaceObservationRegistry, *nativeWorkspace, *attemptWorkspace, wwc.Lease) {
	t.Helper()
	registry, native, workspace, observation := nativeObservationFixture(t)
	native.ledger.Assignment = native.assignment
	workspace.Assignment = native.assignment
	if err := native.saveLedger(); err != nil {
		t.Fatal(err)
	}
	content := "remote saved\n"
	request := wwc.CreateRequest{ProjectID: "project", Scope: observation.Observation.Scope, WorkspacePolicySHA256: native.policySHA, Path: "file.txt", ExpectedSHA256: digest([]byte("base\n")), Content: content, ContentSHA256: wwc.ContentDigest(content)}
	before := time.Now().Add(10 * time.Second).UTC().Format(time.RFC3339Nano)
	command := wwc.Command{ID: "write-one", ProjectID: request.ProjectID, Version: 2, State: "leased", Scope: request.Scope, WorkspacePolicySHA256: request.WorkspacePolicySHA256, Path: request.Path, ExpectedSHA256: request.ExpectedSHA256, ContentSHA256: request.ContentSHA256, RequestSHA256: wwc.RequestDigest(request), ExecutionFence: 1, ExecuteClaimID: "execute-one", ExecuteBefore: before, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().Add(120 * time.Second).UTC().Format(time.RFC3339Nano)}
	return registry, native, workspace, wwc.Lease{Command: command, ClaimID: command.ExecuteClaimID, Mode: "execute", LeaseUntil: before, Content: &content}
}
func inspectWriteLease(original wwc.Lease) wwc.Lease {
	lease := original
	lease.Mode = "inspect"
	lease.Content = nil
	lease.Command.State = "reconciling"
	lease.Command.InspectionID = "inspect-one"
	lease.ClaimID = lease.Command.InspectionID
	lease.LeaseUntil = time.Now().Add(10 * time.Second).UTC().Format(time.RFC3339Nano)
	return lease
}

func TestWorkspaceWriteCASReceiptAndCheckInvalidation(t *testing.T) {
	registry, native, workspace, lease := nativeWriteFixture(t)
	native.checkIDs = []string{"unit"}
	native.check = func(context.Context, string) (NativeWorkspaceCheckResult, error) {
		return NativeWorkspaceCheckResult{ProfileSHA256: strings.Repeat("c", 64), ExitCode: 0}, nil
	}
	if _, err := nativeCall(t, native, "check", "check-before-write", map[string]any{"check_id": "unit"}); err != nil {
		t.Fatal(err)
	}
	if err := native.ReadyToSeal(); err != nil {
		t.Fatal(err)
	}
	result := ExecuteWorkspaceWrite(context.Background(), registry, lease)
	if result.Outcome != "applied" || wwc.ValidateResult(lease.Command, result) != nil || result.Effect.Generation != 1 {
		t.Fatalf("remote CAS did not produce exact native receipt: %+v", result)
	}
	if err := native.ReadyToSeal(); err == nil {
		t.Fatal("pre-save checks certified the new generation")
	}
	if _, err := nativeCall(t, native, "write", "model-later", map[string]any{"path": "file.txt", "content": "model later\n", "expected_sha256": lease.Command.ContentSHA256}); err != nil {
		t.Fatal(err)
	}
	again := ExecuteWorkspaceWrite(context.Background(), registry, lease)
	if again.Outcome != "applied" || again.Effect.Generation != 1 {
		t.Fatal("lost ACK replay lost original native receipt")
	}
	actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
	if string(actual) != "model later\n" || native.Evidence().Generation != 2 {
		t.Fatal("replay overwrote a later model edit")
	}
}
func TestWorkspaceWriteInspectionBarrierBlocksLateExecution(t *testing.T) {
	registry, native, workspace, lease := nativeWriteFixture(t)
	inspected := ExecuteWorkspaceWrite(context.Background(), registry, inspectWriteLease(lease))
	if inspected.Outcome != "not_applied" || inspected.ErrorCode != "not_started" {
		t.Fatalf("missing operation did not establish a durable barrier: %+v", inspected)
	}
	late := ExecuteWorkspaceWrite(context.Background(), registry, lease)
	if late.Outcome != "not_applied" {
		t.Fatal("late execute crossed the inspected terminal barrier")
	}
	actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
	if string(actual) != "base\n" || native.Evidence().Generation != 0 {
		t.Fatal("inspection performed a write")
	}
	missingHost := ExecuteWorkspaceWrite(context.Background(), nil, inspectWriteLease(lease))
	if missingHost.Outcome != "unknown" {
		t.Fatal("missing host was treated as proof of no effect")
	}
}
func TestWorkspaceWritePreparedRecoveryDoesNotCompleteARemoteRename(t *testing.T) {
	_, native, workspace, lease := nativeWriteFixture(t)
	op := &nativeWorkspaceOperation{ID: wwc.NativeOperationID(lease.Command.ID), Kind: "write", RequestSHA: lease.Command.RequestSHA256, State: "prepared", Path: "file.txt", PreviousSHA: lease.Command.ExpectedSHA256, SHA: lease.Command.ContentSHA256, Content: *lease.Content, Generation: 1, RemoteWrite: &nativeRemoteWrite{Command: lease.Command}}
	native.ledger.Operations[op.ID] = op
	if err := native.saveLedger(); err != nil {
		t.Fatal(err)
	}
	if err := native.Close(); err != nil {
		t.Fatal(err)
	}
	spec := CommandSpec{NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{}}}
	reopened, err := openNativeWorkspace(context.Background(), workspace.Assignment, ExecConfig{}, workspace, spec, native.policySHA, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
	if string(actual) != "base\n" || reopened.Evidence().State != "unknown" || reopened.Evidence().Generation != 0 {
		t.Fatal("remote prepared recovery rewrote source or certified success")
	}
}

func TestWorkspaceWriteOutboundLostACKReplaysOnlyOriginalReceipt(t *testing.T) {
	registry, native, workspace, lease := nativeWriteFixture(t)
	journal, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	var callbacks []wwc.ResultRequest
	var claimOperation string
	claims := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/runner/workspace-writes/claim":
			var request wwc.ClaimRequest
			_ = json.NewDecoder(r.Body).Decode(&request)
			claims++
			if claimOperation == "" {
				claimOperation = request.OperationID
			}
			if claims <= 2 && request.OperationID != claimOperation {
				t.Error("claim ACK loss changed original operation")
			}
			if claims == 1 {
				connection, _, _ := w.(http.Hijacker).Hijack()
				_ = connection.Close()
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"lease": lease})
		case "/api/v1/runner/workspace-writes/result":
			var result wwc.ResultRequest
			_ = json.NewDecoder(r.Body).Decode(&result)
			callbacks = append(callbacks, result)
			if len(callbacks) == 1 {
				if result.Outcome != "applied" {
					t.Error("actual outbound write did not apply")
				}
				if _, err := nativeCall(t, native, "write", "after-remote-save", map[string]any{"path": "file.txt", "content": "later model source\n", "expected_sha256": lease.Command.ContentSHA256}); err != nil {
					t.Error(err)
				}
				connection, _, _ := w.(http.Hijacker).Hijack()
				_ = connection.Close()
				return
			}
			_ = json.NewEncoder(w).Encode(wwc.Receipt{CommandID: result.CommandID, ClaimID: result.ClaimID, ResultSHA256: result.ResultSHA256, Acknowledged: true})
		default:
			t.Error("unexpected workspace write route", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, ProjectID: "project", Token: "owned-runner"}
	if _, err = journal.beginWorkspaceWriteClaim(server.URL, native.assignment, lease.Command.Scope, lease.Command.WorkspacePolicySHA256, "project"); err != nil {
		t.Fatal(err)
	}
	if err = ReplayWorkspaceWrites(context.Background(), client, journal, registry); err == nil {
		t.Fatal("lost claim response unexpectedly acknowledged")
	}
	if err = ReplayWorkspaceWrites(context.Background(), client, journal, registry); err == nil {
		t.Fatal("lost result response unexpectedly acknowledged")
	}
	if err = ReplayWorkspaceWrites(context.Background(), client, journal, registry); err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
	if claims != 2 || len(callbacks) != 2 || woc.Digest(callbacks[0]) != woc.Digest(callbacks[1]) || string(actual) != "later model source\n" || native.Evidence().Generation != 2 || len(journal.pendingWorkspaceWrites(server.URL)) != 0 {
		t.Fatal("write replay repeated a side effect or changed original receipt")
	}
}

func TestWorkspaceWritePoolClientUsesOriginalAssignmentAndStrictWire(t *testing.T) {
	_, _, _, lease := nativeWriteFixture(t)
	a := poolTestAssignment()
	c := &lease.Command
	c.Scope.RunID, c.Scope.NodeID, c.Scope.Attempt, c.Scope.Fence, c.Scope.WorkerID = a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID
	c.ProjectID = a.ProjectID
	c.RequestSHA256 = wwc.RequestDigest(wwc.CreateRequest{ProjectID: c.ProjectID, Scope: c.Scope, WorkspacePolicySHA256: c.WorkspacePolicySHA256, Path: c.Path, ExpectedSHA256: c.ExpectedSHA256, ContentSHA256: c.ContentSHA256})
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&body)
		var got Assignment
		_ = json.Unmarshal(body["assignment"], &got)
		if !samePoolAssignmentIdentity(got, a) || string(body["runner_id"]) != `"runner"` || string(body["tenant_id"]) != `"tenant"` {
			t.Error("write lost frozen Pool scope")
		}
		switch r.URL.Path {
		case "/api/v1/pool-runner/workspace-writes/claim":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease": lease})
		case "/api/v1/pool-runner/workspace-writes/result":
			if _, exists := body["result"]; exists {
				t.Error("write used DAG result envelope")
			}
			var result wwc.ResultRequest
			raw, _ := json.Marshal(body)
			_ = json.Unmarshal(raw, &result)
			_ = json.NewEncoder(w).Encode(wwc.Receipt{CommandID: result.CommandID, ClaimID: result.ClaimID, ResultSHA256: result.ResultSHA256, Acknowledged: true})
		default:
			t.Error("unexpected Pool route", r.URL.Path)
		}
	}))
	defer server.Close()
	base := &Client{BaseURL: server.URL, Token: "owned-pool-token", Pool: &PoolContext{PoolID: a.PoolID, RunnerID: a.PoolRunnerID}}
	client, err := base.forPoolAssignment(a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.ClaimWorkspaceWrite(context.Background(), wwc.ClaimRequest{ProjectID: c.ProjectID, WorkerID: a.WorkerID, Scope: c.Scope, WorkspacePolicySHA256: c.WorkspacePolicySHA256, OperationID: operationID()}); err != nil {
		t.Fatal(err)
	}
	result := ExecuteWorkspaceWrite(context.Background(), nil, lease)
	if _, err = client.CompleteWorkspaceWrite(context.Background(), result); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("unexpected callback count", calls)
	}
	raw, _ := json.Marshal(map[string]any{"lease": lease})
	for _, invalid := range []string{strings.Replace(string(raw), `"scope":`, `"SCOPE":`, 1), `{"lease":null,"lease":null}`, `{"lease":null,"url":"https://unused.invalid"}`} {
		var response workspaceWriteClaimReply
		if json.Unmarshal([]byte(invalid), &response) == nil {
			t.Fatal("write wire accepted alias/duplicate/unknown fields")
		}
	}
}
func TestWorkspaceWriteConcurrentModelCASHasOneWinner(t *testing.T) {
	registry, native, workspace, lease := nativeWriteFixture(t)
	var group sync.WaitGroup
	group.Add(2)
	var human wwc.ResultRequest
	var modelErr error
	go func() { defer group.Done(); human = ExecuteWorkspaceWrite(context.Background(), registry, lease) }()
	go func() {
		defer group.Done()
		_, modelErr = nativeCall(t, native, "write", "concurrent-model", map[string]any{"path": "file.txt", "content": "model winner\n", "expected_sha256": lease.Command.ExpectedSHA256})
	}()
	group.Wait()
	actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
	if native.Evidence().Generation != 1 || (human.Outcome == "applied") == (modelErr == nil) || (string(actual) != *lease.Content && string(actual) != "model winner\n") {
		raw, _ := json.Marshal(human)
		t.Fatalf("concurrent CAS did not serialize exactly one accepted change: %s / %v", raw, modelErr)
	}
}

func TestWorkspaceWriteJournalRecoversOriginalClaimAndNeverReexecutesStartedDelivery(t *testing.T) {
	registry, native, workspace, lease := nativeWriteFixture(t)
	dir := t.TempDir()
	journal, err := OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	const origin = "https://control.example"
	a := native.assignment
	claim, err := journal.beginWorkspaceWriteClaim(origin, a, lease.Command.Scope, lease.Command.WorkspacePolicySHA256, "project")
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := journal.beginWorkspaceWriteClaim(origin, a, lease.Command.Scope, lease.Command.WorkspacePolicySHA256, "project")
	if err != nil || claim.Request.OperationID != repeated.Request.OperationID {
		t.Fatal("lost claim acknowledgement replaced original operation")
	}
	if err = journal.recordWorkspaceWriteClaim(origin, claim.Request.OperationID, &lease); err != nil {
		t.Fatal(err)
	}
	if err = journal.startWorkspaceWrite(origin, lease); err != nil {
		t.Fatal(err)
	}
	if err = journal.Close(); err != nil {
		t.Fatal(err)
	}
	journal, err = OpenJournal(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	pending := journal.pendingWorkspaceWrites(origin)
	if len(pending) != 1 || !pending[0].ProcessingStarted || pending[0].Lease.Content != nil {
		t.Fatal("restart preserved an executable source delivery")
	}
	unknown := executeWorkspaceWrite(context.Background(), registry, pending[0].Lease, false)
	if unknown.Outcome != "unknown" {
		t.Fatal("missing original native record became a known effect")
	}
	actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
	if string(actual) != "base\n" {
		t.Fatal("journal restart retried a previously started delivery")
	}
	if err = journal.saveWorkspaceWriteResult(origin, lease, unknown); err != nil {
		t.Fatal(err)
	}
	wrong := wwc.Receipt{CommandID: lease.Command.ID, ClaimID: "another-claim", ResultSHA256: unknown.ResultSHA256, Acknowledged: true}
	if journal.ackWorkspaceWrite(origin, wrong) == nil {
		t.Fatal("different claim acknowledged original write")
	}
	right := wwc.Receipt{CommandID: lease.Command.ID, ClaimID: lease.ClaimID, ResultSHA256: unknown.ResultSHA256, Acknowledged: true}
	if err = journal.ackWorkspaceWrite(origin, right); err != nil {
		t.Fatal(err)
	}
	if len(journal.pendingWorkspaceWrites(origin)) != 0 {
		t.Fatal("acknowledged original write remained pending")
	}
}

func TestWorkspaceWritePermitAndRenameAdmissionNeverCertifyAnUnknownEffect(t *testing.T) {
	t.Run("matching external bytes are not an original operation receipt", func(t *testing.T) {
		_, native, _, lease := nativeWriteFixture(t)
		calls := 0
		guard := func() error {
			calls++
			if calls == 2 {
				file, err := native.root.OpenFile("file.txt", os.O_WRONLY|os.O_TRUNC, 0600)
				if err != nil {
					return err
				}
				_, err = file.Write([]byte(*lease.Content))
				_ = file.Close()
				return err
			}
			return nil
		}
		native.mu.Lock()
		_, err := native.operationLockedWithAdmission(context.Background(), "write", map[string]any{"path": "file.txt", "content": *lease.Content, "expected_sha256": lease.Command.ExpectedSHA256}, wwc.NativeOperationID(lease.Command.ID), remoteWriteMetadata(lease.Command), guard)
		native.mu.Unlock()
		if err == nil || native.Evidence().Generation != 0 || native.Evidence().State != "unknown" {
			t.Fatal("matching external bytes were certified as the original remote write")
		}
	})
	t.Run("expired permit has a durable terminal barrier", func(t *testing.T) {
		registry, native, workspace, lease := nativeWriteFixture(t)
		lease.LeaseUntil = time.Now().Add(-time.Second).UTC().Format(time.RFC3339Nano)
		result := ExecuteWorkspaceWrite(context.Background(), registry, lease)
		if result.Outcome != "not_applied" || result.ErrorCode != "permit_expired" || native.ledger.Operations[wwc.NativeOperationID(lease.Command.ID)].State != "not_applied" {
			t.Fatal("expired permit was not durably fenced")
		}
		actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
		if string(actual) != "base\n" || native.Evidence().Generation != 0 {
			t.Fatal("expired permit changed source")
		}
	})
	t.Run("cancel after prepare before rename stays unknown", func(t *testing.T) {
		_, native, workspace, lease := nativeWriteFixture(t)
		calls := 0
		guard := func() error {
			calls++
			if calls == 3 {
				return &remoteWriteGuardError{code: "permit_expired"}
			}
			return nil
		}
		native.mu.Lock()
		_, err := native.operationLockedWithAdmission(context.Background(), "write", map[string]any{"path": "file.txt", "content": *lease.Content, "expected_sha256": lease.Command.ExpectedSHA256}, wwc.NativeOperationID(lease.Command.ID), remoteWriteMetadata(lease.Command), guard)
		native.mu.Unlock()
		if err == nil || calls != 3 || native.Evidence().State != "unknown" || native.Evidence().Generation != 0 {
			t.Fatal("rename admission failure certified a remote effect")
		}
		actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
		if string(actual) != "base\n" {
			t.Fatal("expired prepared permit completed rename")
		}
		registry := NewWorkspaceObservationRegistry("project")
		unregister, err := registry.register(native)
		if err != nil {
			t.Fatal(err)
		}
		defer unregister()
		if result := ExecuteWorkspaceWrite(context.Background(), registry, inspectWriteLease(lease)); result.Outcome != "unknown" {
			t.Fatal("prepared record became no-effect or applied proof")
		}
	})
	t.Run("closed host and reserved namespace", func(t *testing.T) {
		registry, native, workspace, lease := nativeWriteFixture(t)
		if _, err := nativeCall(t, native, "write", wwc.NativeOperationID("forged"), map[string]any{"path": "file.txt", "content": "bad\n", "expected_sha256": lease.Command.ExpectedSHA256}); err == nil {
			t.Fatal("MCP caller forged the remote operation namespace")
		}
		if err := native.Close(); err != nil {
			t.Fatal(err)
		}
		if result := ExecuteWorkspaceWrite(context.Background(), registry, lease); result.Outcome != "unknown" {
			t.Fatal("closed historical host became proof of no effect")
		}
		actual, _ := os.ReadFile(filepath.Join(workspace.Source, "file.txt"))
		if string(actual) != "base\n" {
			t.Fatal("closed host reopened source")
		}
	})
}
