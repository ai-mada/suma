package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
)

func compositionCodeInputHTTP(t *testing.T, f *fixture, token, namespace string, r ccmp.CodeInputRequest, want int) []byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://suma.test/api/v1/"+namespace+"/code-inputs/bundle", bytes.NewReader([]byte(stringify(r))))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, req)
	if want == 0 {
		if response.Code != 403 && response.Code != 409 {
			t.Fatalf("expected scope or live-lease rejection, got %d: %s", response.Code, response.Body.String())
		}
	} else if response.Code != want {
		t.Fatalf("code input response %d want %d: %s", response.Code, want, response.Body.String())
	}
	return response.Body.Bytes()
}

// Genuine same-tenant bilateral Pool authorization and sealed byte transport;
// this test does not claim a Git process, model or physical second machine.
func TestCandidateCompositionPoolTypedAssignmentAndFrozenCodeInputTransport(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	pool, grant := dispatchAuthorizedPool(t, f)
	dispatchSetRoute(t, f, 0, &grant)
	machine, e := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "Governed composition Pool worker", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	p := h.start(t, h.create(t))
	claim := dag.PoolClaimRequest{PoolID: pool.ID, RunnerID: machine.Runner.ID, WorkerID: machine.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:suma-compose"}}}}
	reply := f.bearer("POST", "/pool-runner/claim", machine.Token, claim, 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID != p.RunID || a.CandidateComposition == nil || a.CandidateComposition.SourcesSHA256 != p.SourcesSHA256 || a.PoolGrantID != grant.ID || a.PoolRunnerID != machine.Runner.ID || a.PoolCredentialVersion != machine.Runner.CredentialVersion || a.CodeBaseline != nil || a.CandidateCommit != "" {
		t.Fatal("Pool assignment did not retain the exact frozen composition", reply)
	}
	request := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: repositoryTestRef(a), InputKind: "composition", BindingID: p.ID, TransferID: p.Sources[0].Source.TransferID}
	good := compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", request, 200)
	if string(good) != "candidate bytes" {
		t.Fatal("authorized source bytes changed")
	}
	for _, source := range p.Sources {
		request.TransferID = source.Source.TransferID
		if string(compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", request, 200)) != "candidate bytes" {
			t.Fatal("fixed second source bytes changed")
		}
	}
	request.TransferID = p.Sources[0].Source.TransferID
	bad := request
	bad.Assignment.TenantID = newID()
	compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", bad, 0)
	bad = request
	bad.Assignment.Fence++
	compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", bad, 0)
	bad = request
	bad.TransferID = newID()
	compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", bad, 403)
	bad = request
	bad.BindingID = newID()
	compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", bad, 403)
	source := h.req.Sources[0]
	_, e = f.s.Update(ctx, f.actor, f.project, "tasks", source.TaskID, f.body(map[string]any{"expected_version": source.ExpectedTaskVersion, "description": "Source changed after the current Pool command consumed its fixed bytes"}))
	if e != nil {
		t.Fatal(e)
	}
	compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", request, 409)
	// A typed known failure remains a genuine observation after source drift;
	// it cannot authorize fresh code reads, tests or acceptance of a candidate.
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 1, Error: "Known command failure at the typed protocol boundary", Output: "Fixed bundle bytes were received before source drift", OperationID: newID()}
	body := poolAssignmentBody(a)
	body["result"] = result
	f.bearer("POST", "/pool-runner/results", machine.Token, body, 200)
	f.bearer("POST", "/pool-runner/results", machine.Token, body, 200)
	attempt, e := f.e.GetAttempt(ctx, f.user.TenantID, f.project, a.RunID, a.NodeID, a.Attempt)
	if e != nil || attempt.Result == nil || attempt.Result.Output != result.Output || attempt.Result.Fence != a.Fence || attempt.Result.WorkerID != machine.Runner.WorkerID {
		t.Fatal(attempt, e)
	}
	var count int
	if e = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=? AND node_id=?`, a.RunID, a.NodeID).Scan(&count); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", request, 0)
	current, e := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if e != nil || current.RunState != "failed" || current.Valid || current.Candidate != nil {
		t.Fatal(current, e)
	}
}
