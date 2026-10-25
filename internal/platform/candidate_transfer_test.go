package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

func candidateTransferFixture(t *testing.T) (*fixture, string, cc.StartRequest) {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Candidate transfer", Nodes: []dag.Node{{ID: "code", Type: "command", Command: []string{"develop"}}, {ID: "test", Type: "test", Command: []string{"check"}, DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "source-worker", []string{"command:develop"})
	if err != nil || a == nil {
		t.Fatalf("claim: %+v %v", a, err)
	}
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	r := cc.StartRequest{ProjectID: f.project, Assignment: cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}, OperationID: newID(), BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("b", 40), EvidenceSHA256: strings.Repeat("e", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
	return f, token, r
}

func candidateJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err = json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestCandidateTransferHTTPStartsBoundToActualSourceLease(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	out := f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201)
	if out["state"] != "uploading" || out["head_commit"] != r.HeadCommit {
		t.Fatalf("wrong manifest: %+v", out)
	}
	again := f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201)
	if again["transfer_id"] != out["transfer_id"] {
		t.Fatal("replay created another transfer")
	}
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, r.Assignment.RunID, r.Assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 409)
}

func TestCandidateTransferStartRecoveryProjectsSealedStateWithoutChangingReceipt(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	ctx := context.Background()
	actor := domainCActor(t, f.s, token)
	start := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201))
	var original string
	if err := f.s.DB.QueryRow(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, r.OperationID).Scan(&original); err != nil {
		t.Fatal(err)
	}
	candidateBinary(t, f, "runner", token, start.TransferID, newID(), r.Assignment, http.MethodPut, []byte("candidate bytes"), 200)
	seal := cc.CommandRequest{ProjectID: f.project, Assignment: r.Assignment, OperationID: newID()}
	sealed := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+start.TransferID+"/seal", token, candidateJSON(t, seal), 200))
	recovered := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201))
	if recovered != sealed || !recovered.ValidSealed() {
		t.Fatalf("source-active Start recovery did not return sealed state: %+v", recovered)
	}
	var after string
	if err := f.s.DB.QueryRow(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, f.project, r.OperationID).Scan(&after); err != nil || after != original {
		t.Fatalf("Start recovery changed the immutable original receipt: %v", err)
	}
	var receipt cc.Binding
	if err := json.Unmarshal([]byte(after), &receipt); err != nil || receipt.State != "uploading" {
		t.Fatal("original receipt did not retain uploading identity", err)
	}
	for _, scenario := range []string{"manifest", "source", "owner"} {
		t.Run(scenario, func(t *testing.T) {
			changed := sealed
			owner := actor.credential.digest
			switch scenario {
			case "manifest":
				changed.BundleSHA256 = strings.Repeat("c", 64)
			case "source":
				changed.Source.WorkerID = "another-worker"
			case "owner":
				owner = strings.Repeat("c", 64)
			}
			if _, err := f.s.DB.Exec(`UPDATE suma_candidate_transfers SET binding=?,subject_hash=? WHERE id=?`, stringify(changed), owner, sealed.TransferID); err != nil {
				t.Fatal(err)
			}
			_, err := f.s.StartCandidateTransfer(ctx, actor, f.project, r)
			releaseWantStatus(t, err, 409)
			if _, err = f.s.DB.Exec(`UPDATE suma_candidate_transfers SET binding=?,subject_hash=? WHERE id=?`, stringify(sealed), actor.credential.digest, sealed.TransferID); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, r.Assignment.RunID, r.Assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.StartCandidateTransfer(ctx, actor, f.project, r)
	releaseWantStatus(t, err, 409)
	if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=unixepoch('now')+60 WHERE run_id=? AND id=?`, r.Assignment.RunID, r.Assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	target, err := f.s.ResolveLogoutTarget(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.Logout(ctx, target); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.StartCandidateTransfer(ctx, actor, f.project, r)
	releaseWantStatus(t, err, 401)
}

func candidateBinary(t *testing.T, f *fixture, namespace, token, id, op string, a cc.AssignmentRef, method string, body []byte, want int) []byte {
	t.Helper()
	raw, _ := json.Marshal(a)
	project := f.project
	if a.ProjectID != "" {
		project = a.ProjectID
	}
	req := httptest.NewRequest(method, "http://suma.test/api/v1/"+namespace+"/candidate-transfers/"+id+"/bundle?project_id="+project, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Suma-Operation-ID", op)
	req.Header.Set("X-Suma-Assignment", base64.RawURLEncoding.EncodeToString(raw))
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, req)
	if w.Code != want {
		t.Fatalf("%s candidate bundle: %d want %d: %s", method, w.Code, want, w.Body.String())
	}
	return w.Body.Bytes()
}
func candidateBinding(t *testing.T, out map[string]any) cc.Binding {
	t.Helper()
	var b cc.Binding
	if err := json.Unmarshal([]byte(stringify(out)), &b); err != nil {
		t.Fatal(err)
	}
	return b
}
func candidateSourceProof(r cc.StartRequest, b cc.Binding) map[string]any {
	return map[string]any{"head_commit": r.HeadCommit, "base_commit": r.BaseCommit, "evidence_sha256": r.EvidenceSHA256, "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "candidate_bundle": b}
}
func candidateCompleteAndClaim(t *testing.T, f *fixture, token string, r cc.StartRequest, b cc.Binding) cc.CommandRequest {
	t.Helper()
	ctx := context.Background()
	a := r.Assignment
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: candidateSourceProof(r, b)}
	f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": result}, 200)
	consumer, err := f.e.Claim(ctx, f.user.TenantID, f.project, "consumer-worker", []string{"command:check"})
	if err != nil || consumer == nil {
		t.Fatalf("consumer: %+v %v", consumer, err)
	}
	return cc.CommandRequest{ProjectID: f.project, OperationID: newID(), Assignment: cc.AssignmentRef{RunID: consumer.RunID, NodeID: consumer.NodeID, Attempt: consumer.Attempt, Fence: consumer.Fence, WorkerID: consumer.WorkerID}}
}

func TestCandidateTransferHTTPSealedSourceAndFrozenConsumer(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	body := bytes.Repeat([]byte("b"), (2<<20)+17)
	r.BundleSHA256 = digest(string(body))
	r.SizeBytes = int64(len(body))
	start := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201))
	seal := cc.CommandRequest{ProjectID: f.project, Assignment: r.Assignment, OperationID: newID()}
	f.bearer("POST", "/runner/candidate-transfers/"+start.TransferID+"/seal", token, candidateJSON(t, seal), 409)
	candidateBinary(t, f, "runner", token, start.TransferID, newID(), r.Assignment, http.MethodPut, []byte("wrong"), 409)
	op := newID()
	candidateBinary(t, f, "runner", token, start.TransferID, op, r.Assignment, http.MethodPut, body, 200)
	candidateBinary(t, f, "runner", token, start.TransferID, op, r.Assignment, http.MethodPut, body, 200)
	b := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+start.TransferID+"/seal", token, candidateJSON(t, seal), 200))
	if !b.ValidSealed() {
		t.Fatalf("not sealed: %+v", b)
	}
	f.bearer("POST", "/runner/candidate-transfers/"+start.TransferID+"/seal", token, candidateJSON(t, seal), 200)
	// Producer data remains unavailable until a successful source result exists
	// and a consumer has its own scheduler-frozen candidate and current lease.
	f.bearer("POST", "/runner/candidate-transfers/resolve", token, candidateJSON(t, seal), 409)
	candidateBinary(t, f, "runner", token, b.TransferID, newID(), r.Assignment, http.MethodGet, nil, 409)
	consumer := candidateCompleteAndClaim(t, f, token, r, b)
	resolved := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/resolve", token, candidateJSON(t, consumer), 200))
	if resolved != b {
		t.Fatal("consumer received different source")
	}
	got := candidateBinary(t, f, "runner", token, b.TransferID, newID(), consumer.Assignment, http.MethodGet, nil, 200)
	if !bytes.Equal(got, body) {
		t.Fatal("bundle bytes changed")
	}
	// A result change invalidates an old successful resolve receipt too.
	var original string
	if err := f.s.DB.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt).Scan(&original); err != nil {
		t.Fatal(err)
	}
	var result dag.Result
	_ = json.Unmarshal([]byte(original), &result)
	result.WorkspaceEvidence["base_commit"] = strings.Repeat("c", 40)
	if _, err := f.s.DB.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=? AND node_id=? AND attempt=?`, stringify(result), r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/candidate-transfers/resolve", token, candidateJSON(t, consumer), 409)
	candidateBinary(t, f, "runner", token, b.TransferID, newID(), consumer.Assignment, http.MethodGet, nil, 409)
	if _, err := f.s.DB.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=? AND node_id=? AND attempt=?`, original, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, consumer.Assignment.RunID, consumer.Assignment.NodeID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/runner/candidate-transfers/resolve", token, candidateJSON(t, consumer), 409)
}

func TestCandidateTransferManifestAndIdentityCannotDrift(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	start := f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201)
	changed := r
	changed.HeadCommit = strings.Repeat("c", 40)
	f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, changed), 409)
	changed = r
	changed.OperationID = newID()
	f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, changed), 409)
	bad := candidateJSON(t, r)
	bad["unexpected"] = true
	f.bearer("POST", "/runner/candidate-transfers", token, bad, 400)
	f.call("POST", "/runner/candidate-transfers", candidateJSON(t, r), 403)
	changed = r
	changed.Assignment.Fence++
	f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, changed), 409)
	changed = r
	changed.Assignment.PoolID = "forged"
	f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, changed), 403)
	actor := domainCActor(t, f.s, token)
	target, err := f.s.ResolveLogoutTarget(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.Logout(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.StartCandidateTransfer(context.Background(), actor, f.project, r)
	releaseWantStatus(t, err, 401)
	if start["transfer_id"] == "" {
		t.Fatal("no original transfer")
	}
}

type candidateMutationReader struct {
	io.Reader
	mutate func()
	once   bool
}

func (r *candidateMutationReader) Read(p []byte) (int, error) {
	if !r.once {
		r.once = true
		r.mutate()
	}
	return r.Reader.Read(p)
}
func TestCandidateTransferUploadRechecksAuthorityAfterUnlockedIO(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	actor := domainCActor(t, f.s, token)
	ctx := context.Background()
	b, err := f.s.StartCandidateTransfer(ctx, actor, f.project, r)
	if err != nil {
		t.Fatal(err)
	}
	reader := &candidateMutationReader{Reader: strings.NewReader("candidate bytes"), mutate: func() {
		if _, e := f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, r.Assignment.RunID, r.Assignment.NodeID); e != nil {
			t.Fatal(e)
		}
	}}
	err = f.s.PutCandidateBundle(ctx, actor, f.project, b.TransferID, r.Assignment, newID(), r.SizeBytes, reader)
	releaseWantStatus(t, err, 409)
	var path string
	if err = f.s.DB.QueryRow(`SELECT storage_path FROM suma_candidate_transfers WHERE id=?`, b.TransferID).Scan(&path); err != nil || path != "" {
		t.Fatalf("stale bytes published: %s %v", path, err)
	}
	files, err := os.ReadDir(filepath.Join(f.s.DataDir, "release-artifacts", "blobs"))
	if err != nil || len(files) != 0 {
		t.Fatalf("disposable upload leaked: %d %v", len(files), err)
	}
}

func candidatePoolRef(t *testing.T, a dag.Assignment) cc.AssignmentRef {
	t.Helper()
	var out cc.AssignmentRef
	if err := json.Unmarshal([]byte(stringify(a)), &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func candidatePoolClaim(t *testing.T, p *poolMachineFixture, credential ExecutionPoolCredentialResponse, command string) dag.Assignment {
	t.Helper()
	scope := p.scopes[0]
	scope.Capabilities = []string{"command:" + command}
	req := dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: credential.Runner.ID, WorkerID: credential.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{scope}}
	out := p.f.bearer("POST", "/pool-runner/claim", credential.Token, req, 200)
	var a dag.Assignment
	if err := json.Unmarshal([]byte(stringify(out["assignment"])), &a); err != nil || a.RunID == "" {
		t.Fatalf("pool claim: %+v %v", out, err)
	}
	return a
}
func candidatePoolStartRequest(t *testing.T, p *poolMachineFixture, a dag.Assignment) cc.StartRequest {
	t.Helper()
	return cc.StartRequest{ProjectID: a.ProjectID, TenantID: a.TenantID, RunnerID: a.PoolRunnerID, Assignment: candidatePoolRef(t, a), OperationID: newID(), BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("b", 40), EvidenceSHA256: strings.Repeat("e", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
}
func candidatePoolWorkflow(t *testing.T) (*poolMachineFixture, dag.Assignment, cc.StartRequest) {
	t.Helper()
	p := newPoolMachineFixture(t)
	f := p.f
	ctx := context.Background()
	w, err := f.e.Publish(ctx, p.scopes[0].TenantID, p.scopes[0].ProjectID, dag.Workflow{Name: "Pool candidate transfer", Nodes: []dag.Node{{ID: "code", Type: "command", Command: []string{"develop"}}, {ID: "test", Type: "test", Command: []string{"check"}, DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, p.scopes[0].TenantID, p.scopes[0].ProjectID, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	a := candidatePoolClaim(t, p, p.credential, "develop")
	return p, a, candidatePoolStartRequest(t, p, a)
}
func TestCandidateTransferPoolUsesIndependentCurrentAuthorityAndFrozenConsumer(t *testing.T) {
	p, a, r := candidatePoolWorkflow(t)
	f := p.f
	token := p.credential.Token
	start := candidateBinding(t, f.bearer("POST", "/pool-runner/candidate-transfers", token, candidateJSON(t, r), 201))
	ordinary := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	stripped := r
	stripped.TenantID = ""
	stripped.RunnerID = ""
	stripped.Assignment = cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	f.bearer("POST", "/runner/candidate-transfers", ordinary, candidateJSON(t, stripped), 403)
	f.bearer("POST", "/pool-runner/candidate-transfers", ordinary, candidateJSON(t, r), 403)
	// Withdrawing a bilateral grant still allows fact draining elsewhere, but
	// cannot recover a previously successful new-code publication receipt.
	if _, err := f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='withdrawn' WHERE id=?`, a.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/candidate-transfers", token, candidateJSON(t, r), 403)
	if _, err := f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='approved' WHERE id=?`, a.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	candidateBinary(t, f, "pool-runner", token, start.TransferID, newID(), r.Assignment, http.MethodPut, []byte("candidate bytes"), 200)
	seal := cc.CommandRequest{ProjectID: r.ProjectID, TenantID: r.TenantID, RunnerID: r.RunnerID, Assignment: r.Assignment, OperationID: newID()}
	b := candidateBinding(t, f.bearer("POST", "/pool-runner/candidate-transfers/"+start.TransferID+"/seal", token, candidateJSON(t, seal), 200))
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: candidateSourceProof(r, b)}
	resultBody := poolAssignmentBody(a)
	resultBody["result"] = result
	f.bearer("POST", "/pool-runner/results", token, resultBody, 200)
	consumerCredential, err := f.s.RegisterExecutionPoolRunner(context.Background(), f.actor, p.pool.ID, ExecutionPoolRunnerCreateRequest{Name: "Independent candidate consumer", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	consumer := candidatePoolClaim(t, p, consumerCredential, "check")
	if consumer.WorkerID == a.WorkerID {
		t.Fatal("consumer did not use an independent machine identity")
	}
	command := cc.CommandRequest{ProjectID: consumer.ProjectID, TenantID: consumer.TenantID, RunnerID: consumer.PoolRunnerID, Assignment: candidatePoolRef(t, consumer), OperationID: newID()}
	resolved := candidateBinding(t, f.bearer("POST", "/pool-runner/candidate-transfers/resolve", consumerCredential.Token, candidateJSON(t, command), 200))
	if resolved != b {
		t.Fatal("pool candidate provenance changed")
	}
	got := candidateBinary(t, f, "pool-runner", consumerCredential.Token, b.TransferID, newID(), command.Assignment, http.MethodGet, nil, 200)
	if string(got) != "candidate bytes" {
		t.Fatal("pool bytes changed")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pools SET status='paused' WHERE id=?`, p.pool.ID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/candidate-transfers/resolve", consumerCredential.Token, candidateJSON(t, command), 403)
	candidateBinary(t, f, "pool-runner", consumerCredential.Token, b.TransferID, newID(), command.Assignment, http.MethodGet, nil, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pools SET status='active' WHERE id=?`, p.pool.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='revoked' WHERE id=?`, consumer.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/candidate-transfers/resolve", consumerCredential.Token, candidateJSON(t, command), 403)
	candidateBinary(t, f, "pool-runner", consumerCredential.Token, b.TransferID, newID(), command.Assignment, http.MethodGet, nil, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='approved' WHERE id=?`, consumer.PoolGrantID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_projects SET status='archived' WHERE id=?`, consumer.ProjectID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/candidate-transfers/resolve", consumerCredential.Token, candidateJSON(t, command), 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_projects SET status='active' WHERE id=?`, consumer.ProjectID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET status='revoked' WHERE runner_id=?`, consumer.PoolRunnerID); err != nil {
		t.Fatal(err)
	}
	f.bearer("POST", "/pool-runner/candidate-transfers/resolve", consumerCredential.Token, candidateJSON(t, command), 403)
}

func TestCandidateTransferRejectsTamperedBytesAndChangedSuccessfulSource(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	ctx := context.Background()
	actor := domainCActor(t, f.s, token)
	b, err := f.s.StartCandidateTransfer(ctx, actor, f.project, r)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.PutCandidateBundle(ctx, actor, f.project, b.TransferID, r.Assignment, newID(), r.SizeBytes, strings.NewReader("candidate bytes")); err != nil {
		t.Fatal(err)
	}
	seal := cc.CommandRequest{ProjectID: f.project, Assignment: r.Assignment, OperationID: newID()}
	b, err = f.s.SealCandidateTransfer(ctx, actor, f.project, b.TransferID, seal)
	if err != nil {
		t.Fatal(err)
	}
	consumer := candidateCompleteAndClaim(t, f, token, r, b)
	if _, err = f.s.ResolveCandidateTransfer(ctx, actor, f.project, consumer); err != nil {
		t.Fatal(err)
	}
	var original string
	if err = f.s.DB.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt).Scan(&original); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"source-state", "source-fence", "result-unknown", "result-bundle"} {
		t.Run(scenario, func(t *testing.T) {
			var source dag.Result
			if err := json.Unmarshal([]byte(original), &source); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "source-state":
				_, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='failed' WHERE run_id=? AND id=?`, r.Assignment.RunID, r.Assignment.NodeID)
			case "source-fence":
				_, err = f.s.DB.Exec(`UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, r.Assignment.RunID, r.Assignment.NodeID)
			case "result-unknown":
				source.OutcomeUnknown = true
				_, err = f.s.DB.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=? AND node_id=? AND attempt=?`, stringify(source), r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt)
			case "result-bundle":
				changed := b
				changed.BundleSHA256 = strings.Repeat("c", 64)
				source.WorkspaceEvidence["candidate_bundle"] = changed
				_, err = f.s.DB.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=? AND node_id=? AND attempt=?`, stringify(source), r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt)
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.s.ResolveCandidateTransfer(ctx, actor, f.project, consumer)
			releaseWantStatus(t, err, 409)
			if _, err = f.s.DB.Exec(`UPDATE dag_nodes SET state='succeeded',fence=? WHERE run_id=? AND id=?`, r.Assignment.Fence, r.Assignment.RunID, r.Assignment.NodeID); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.DB.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=? AND node_id=? AND attempt=?`, original, r.Assignment.RunID, r.Assignment.NodeID, r.Assignment.Attempt); err != nil {
				t.Fatal(err)
			}
		})
	}
	var path string
	if err = f.s.DB.QueryRow(`SELECT storage_path FROM suma_candidate_transfers WHERE id=?`, b.TransferID).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(f.s.DataDir, "release-artifacts", filepath.FromSlash(path)), []byte("tampered bytes!"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.RunnerCandidateBundle(ctx, actor, f.project, b.TransferID, consumer.Assignment)
	releaseWantStatus(t, err, 409)
}

func TestCandidateTransferBinaryHeadersAndLimitsStayBounded(t *testing.T) {
	f, token, r := candidateTransferFixture(t)
	actor := domainCActor(t, f.s, token)
	ctx := context.Background()
	b, err := f.s.StartCandidateTransfer(ctx, actor, f.project, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, size := range []int64{-1, 0, cc.MaxBundleBytes + 1} {
		err = f.s.PutCandidateBundle(ctx, actor, f.project, b.TransferID, r.Assignment, newID(), size, strings.NewReader("candidate bytes"))
		releaseWantStatus(t, err, 400)
	}
	header := base64.RawURLEncoding.EncodeToString([]byte(stringify(r.Assignment)))
	for _, scenario := range []string{"duplicate", "unknown", "fraction", "missing-op"} {
		t.Run(scenario, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "http://suma.test/api/v1/runner/candidate-transfers/"+b.TransferID+"/bundle?project_id="+f.project, strings.NewReader("candidate bytes"))
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Suma-Operation-ID", newID())
			req.Header.Set("X-Suma-Assignment", header)
			switch scenario {
			case "duplicate":
				req.Header.Set("X-Suma-Assignment", base64.RawURLEncoding.EncodeToString([]byte(`{"run_id":"a","run_id":"b"}`)))
			case "unknown":
				req.Header.Set("X-Suma-Assignment", base64.RawURLEncoding.EncodeToString([]byte(strings.TrimSuffix(stringify(r.Assignment), "}")+`,"unexpected":true}`)))
			case "fraction":
				req.Header.Set("X-Suma-Assignment", base64.RawURLEncoding.EncodeToString([]byte(strings.Replace(stringify(r.Assignment), `"attempt":1`, `"attempt":1.5`, 1))))
			case "missing-op":
				req.Header.Del("X-Suma-Operation-ID")
			}
			res := httptest.NewRecorder()
			f.handler.ServeHTTP(res, req)
			if res.Code != 400 {
				t.Fatalf("%s header: %d %s", scenario, res.Code, res.Body.String())
			}
		})
	}
}
