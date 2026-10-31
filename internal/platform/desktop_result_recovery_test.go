package platform

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/desktoprecoverycontract"
)

func desktopResultRecoveryRequest(f *fixture, scope rc.Scope, plan rc.Plan) *http.Request {
	body := map[string]any{"scope": scope, "recovery_operation_id": strings.Repeat("7", 32), "plan": plan}
	r := desktopRunnerTestRequest(f, "context", body)
	r.URL.Path = "/api/v1/desktop-runners/recovery/receipts"
	fields := []string{"suma-desktop-runner-v1", "recovery-receipts", scope.ProjectID, scope.UserID, scope.TenantID, scope.WorkerID, scope.AdapterID, scope.InstanceID, scope.ExpectedBindingSHA256, scope.BaseCommit, scope.RegistrationOperationID, scope.CredentialSHA256, body["recovery_operation_id"].(string), rc.PlanSHA256(plan), r.Header.Get("X-Suma-Desktop-Nonce"), r.Header.Get("X-Suma-Desktop-Expires")}
	mac := hmac.New(sha256.New, f.s.DesktopWorkspaceKey)
	mac.Write([]byte(strings.Join(fields, "\x00")))
	r.Header.Set("X-Suma-Desktop-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func desktopResultRecoveryFixture(t *testing.T) (*fixture, rc.Scope, rc.Plan, string) {
	t.Helper()
	f, _ := desktopRunnerFixture(t)
	local := desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "context", desktopRunnerContextBody(f)), 200)
	token := newID() + newID()
	body := desktopRunnerRegistration(f, local, token)
	desktopRunnerTestCall(t, f, desktopRunnerTestRequest(f, "register", body), 201)
	scope := rc.Scope{ProjectID: f.project, UserID: f.user.ID, TenantID: f.user.TenantID, WorkerID: body["worker_id"].(string), AdapterID: body["adapter_id"].(string), InstanceID: body["instance_id"].(string), ExpectedBindingSHA256: body["expected_binding_sha256"].(string), BaseCommit: body["base_commit"].(string), RegistrationOperationID: body["idempotency_key"].(string), CredentialSHA256: digest(token)}
	wf := dag.Workflow{ID: "desktop-result179", Name: "Original committed callback", Nodes: []dag.Node{{ID: "work", Type: "command", Command: []string{"controlled179"}, Idempotent: true}}}
	ctx := context.Background()
	if _, err := f.e.Publish(ctx, f.user.TenantID, f.project, wf); err != nil {
		t.Fatal(err)
	}
	if _, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, wf.ID, newID()); err != nil {
		t.Fatal(err)
	}
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, scope.WorkerID, []string{"command:controlled179"})
	if err != nil || a == nil {
		t.Fatal("original claim", err)
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), Output: "Original durable result"}
	f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": result}, 200)
	raw, _ := json.Marshal(result)
	sha, err := rc.ServerResultSHA(raw)
	if err != nil {
		t.Fatal(err)
	}
	plan := rc.Plan{Protocol: rc.PlanProtocol, ProjectID: f.project, WorkerID: scope.WorkerID, SnapshotSHA256: strings.Repeat("c", 64), Items: []rc.Item{{JournalKey: result.RunID + "/work/1", RunID: result.RunID, NodeID: result.NodeID, Attempt: result.Attempt, Fence: strconv.FormatInt(result.Fence, 10), OperationID: result.OperationID, BodySHA256: rc.Digest(raw), ResultSHA256: sha}}}
	return f, scope, plan, token
}

func desktopResultRecoveryProof(t *testing.T, f *fixture, scope rc.Scope, plan rc.Plan) rc.Proof {
	t.Helper()
	value := desktopRunnerTestCall(t, f, desktopResultRecoveryRequest(f, scope, plan), 200)
	raw, _ := json.Marshal(value)
	var proof rc.Proof
	if err := json.Unmarshal(raw, &proof); err != nil {
		t.Fatal(err)
	}
	if proof.Scope != scope || rc.CheckProof(proof, plan, f.s.DesktopWorkspaceKey, time.Now().UTC(), false) != nil {
		t.Fatal("unbound or invalid server attestation")
	}
	return proof
}

func TestDesktopResultRecoveryReadsCommittedReceiptAfterCredentialExpiry(t *testing.T) {
	f, scope, plan, token := desktopResultRecoveryFixture(t)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	for _, query := range []string{`UPDATE suma_sessions SET expires_at=? WHERE token_hash=?`, `UPDATE suma_credential_slots SET expires_at=? WHERE current_digest=?`, `UPDATE suma_credential_grants SET expires_at=? WHERE id IN (SELECT grant_id FROM suma_credential_authority WHERE token_hash=?)`, `UPDATE suma_release_workers SET expires_at=? WHERE token_hash=?`} {
		if _, err := f.s.DB.Exec(query, past, digest(token)); err != nil {
			t.Fatal(err)
		}
	}
	_, err := f.s.AuthenticateActor(context.Background(), token)
	requireAuthorityStatus(t, err, 401)
	before := map[string]int{}
	for _, table := range []string{"dag_operations", "dag_events", "suma_operations", "suma_credential_issuance"} {
		before[table] = domainACount(t, f.s, table)
	}
	proof := desktopResultRecoveryProof(t, f, scope, plan)
	if len(proof.Items) != 1 || proof.Items[0].Status != "matched" {
		t.Fatalf("original committed result was not matched: %+v", proof.Items)
	}
	issued, _ := time.Parse(time.RFC3339Nano, proof.IssuedAt)
	expires, _ := time.Parse(time.RFC3339Nano, proof.ExpiresAt)
	if expires.Sub(issued) != time.Minute {
		t.Fatal("proof lifetime differs")
	}
	proof.Items[0].Status = "missing"
	if rc.CheckProof(proof, plan, f.s.DesktopWorkspaceKey, time.Now(), false) == nil {
		t.Fatal("tampered attestation accepted")
	}
	for table, count := range before {
		if domainACount(t, f.s, table) != count {
			t.Fatalf("fact lookup mutated %s", table)
		}
	}
	_, err = f.s.AuthenticateActor(context.Background(), token)
	requireAuthorityStatus(t, err, 401)
	f.bearer("POST", "/runner/claim", token, map[string]any{"project_id": f.project, "worker_id": scope.WorkerID, "capabilities": []string{}}, 401)
}

func TestDesktopResultRecoveryDistinguishesMissingMismatchAndHistoricalSource(t *testing.T) {
	f, scope, plan, _ := desktopResultRecoveryFixture(t)
	original := plan.Items[0]
	missing, mismatch, changed := original, original, original
	missing.JournalKey = "missing"
	missing.OperationID = newID()
	mismatch.JournalKey = "mismatch"
	mismatch.ResultSHA256 = strings.Repeat("0", 64)
	changed.JournalKey = "changed"
	fence, _ := strconv.ParseInt(changed.Fence, 10, 64)
	changed.Fence = strconv.FormatInt(fence+1, 10)
	plan.Items = append(plan.Items, missing, mismatch, changed)
	proof := desktopResultRecoveryProof(t, f, scope, plan)
	for i, want := range []string{"matched", "missing", "mismatch", "source_changed"} {
		if proof.Items[i].Status != want {
			t.Fatalf("item %d => %s, want %s", i, proof.Items[i].Status, want)
		}
	}
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET attempt=attempt+1,fence=fence+1,worker_id='later-worker' WHERE run_id=? AND id=?`, original.RunID, original.NodeID); err != nil {
		t.Fatal(err)
	}
	if desktopResultRecoveryProof(t, f, scope, plan).Items[0].Status != "matched" {
		t.Fatal("current head replaced original committed fact")
	}
	wrong := scope
	wrong.CredentialSHA256 = strings.Repeat("0", 64)
	for _, receipt := range desktopResultRecoveryProof(t, f, wrong, plan).Items {
		if receipt.Status != "source_changed" {
			t.Fatal("changed Worker token source was trusted")
		}
	}
}

func TestDesktopResultRecoveryRequiresCurrentBrowserHMACAndClosedRequest(t *testing.T) {
	f, scope, plan, token := desktopResultRecoveryFixture(t)
	t.Run("unsigned_plan", func(t *testing.T) {
		r := desktopResultRecoveryRequest(f, scope, plan)
		changed := plan
		changed.SnapshotSHA256 = strings.Repeat("d", 64)
		r.Body = io.NopCloser(strings.NewReader(stringify(map[string]any{"scope": scope, "recovery_operation_id": strings.Repeat("7", 32), "plan": changed})))
		desktopRunnerTestCall(t, f, r, 403)
	})
	t.Run("remote", func(t *testing.T) {
		r := desktopResultRecoveryRequest(f, scope, plan)
		r.RemoteAddr = "192.0.2.1:33999"
		desktopRunnerTestCall(t, f, r, 403)
	})
	t.Run("machine", func(t *testing.T) {
		r := desktopResultRecoveryRequest(f, scope, plan)
		r.Header.Del("Cookie")
		r.Header.Set("Authorization", "Bearer "+token)
		desktopRunnerTestCall(t, f, r, 403)
	})
	t.Run("extra_field", func(t *testing.T) {
		r := desktopResultRecoveryRequest(f, scope, plan)
		r.Body = io.NopCloser(strings.NewReader(stringify(map[string]any{"scope": scope, "recovery_operation_id": strings.Repeat("7", 32), "plan": plan, "result": "must-not-write"})))
		desktopRunnerTestCall(t, f, r, 400)
	})
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_project_members SET role='developer' WHERE project_id=? AND user_id=?`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	desktopRunnerTestCall(t, f, desktopResultRecoveryRequest(f, scope, plan), 403)
}
