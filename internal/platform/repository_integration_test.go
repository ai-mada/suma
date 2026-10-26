package platform

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// This fixture tests actual Control/Core/HTTP authorization against typed
// provider boundary facts. Real Git effects are independently tested by the
// repositoryprovider suite and the isolated actual journey; no model runs here.
type repositoryFixture struct {
	f                                  *fixture
	secret, inspectSecret, workerToken string
	task                               Record
	target                             RepositoryIntegrationTarget
	plan                               RepositoryIntegrationPlan
	mock                               *repositoryMock
}
type repositoryMock struct {
	h            *repositoryFixture
	Target       ri.RepositoryTargetObservation
	Observations map[string]ri.RepositoryObservation
	mu           sync.Mutex
}

func newRepositoryFixture(t *testing.T) *repositoryFixture {
	t.Helper()
	f, token, source := candidateTransferFixture(t)
	ctx := context.Background()
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Accepted code integration fixture"}))
	if err != nil {
		t.Fatal(err)
	}
	bound, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, task.ID, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.BindRunTx(bound, auth.tx, auth, f.project, task.ID, source.Assignment.RunID, task.Version); err == nil {
		err = auth.tx.Commit()
	}
	close()
	if err != nil {
		t.Fatal(err)
	}
	transfer := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, source), 201))
	candidateBinary(t, f, "runner", token, transfer.TransferID, newID(), source.Assignment, "PUT", []byte("candidate bytes"), 200)
	transfer = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+transfer.TransferID+"/seal", token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: source.Assignment, OperationID: newID()}), 200))
	consumer := candidateCompleteAndClaim(t, f, token, source, transfer)
	a := consumer.Assignment
	proof := map[string]any{"head_commit": source.HeadCommit, "base_commit": source.HeadCommit, "evidence_sha256": strings.Repeat("d", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "test_command": true, "candidate_from": "code", "candidate_attempt": source.Assignment.Attempt, "candidate_fence": source.Assignment.Fence, "candidate_evidence_sha256": source.EvidenceSHA256}
	f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}}, 200)
	task, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": 2, "run_id": a.RunID, "decision": "accepted", "reason": "Test-owner approval of typed fixture evidence, not production acceptance"}), f.e)
	if err != nil {
		t.Fatal(err)
	}
	h := &repositoryFixture{f: f, secret: newID() + newID(), inspectSecret: newID() + newID(), task: task}
	m := &repositoryMock{h: h, Target: ri.RepositoryTargetObservation{Alias: "main", Ref: "refs/heads/main", ConfigSHA256: strings.Repeat("c", 64), HeadCommit: source.BaseCommit, ObjectFormat: "sha1", Available: true, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}, Observations: map[string]ri.RepositoryObservation{}}
	h.mock = m
	server := httptest.NewServer(http.HandlerFunc(m.handle))
	t.Cleanup(server.Close)
	f.s.RepositoryProviders = []RepositoryProviderConfig{{ID: "repo-fixture", TenantID: f.user.TenantID, ProjectID: f.project, TargetAliases: []string{"main"}, TokenHash: digest(h.secret), Origin: server.URL, ReconcileToken: h.inspectSecret}}
	h.target, err = f.s.CreateRepositoryIntegrationTarget(ctx, f.actor, f.project, RepositoryIntegrationTargetCreateRequest{ProjectID: f.project, Name: "Fixture bare main", ProviderID: "repo-fixture", TargetAlias: "main", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := f.s.RegisterReleaseWorker(ctx, f.actor, f.project, map[string]any{"worker_id": "repo-worker", "profiles": []any{}, "deployment": false, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.workerToken = worker["token"].(string)
	return h
}
func (m *repositoryMock) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer "+m.h.inspectSecret {
		w.WriteHeader(403)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "GET" && r.URL.Path == "/targets/main" {
		json.NewEncoder(w).Encode(m.Target)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/operations/")
	if r.Method == "GET" {
		m.mu.Lock()
		o, ok := m.Observations[id]
		m.mu.Unlock()
		if !ok {
			w.WriteHeader(404)
			return
		}
		json.NewEncoder(w).Encode(o)
		return
	}
	if r.Method == "POST" && strings.HasSuffix(id, "/reconcile") {
		var req ri.RepositoryReconcileRequest
		if json.NewDecoder(r.Body).Decode(&req) != nil || req.Request == nil {
			w.WriteHeader(400)
			return
		}
		packet := req.Request
		grant, err := m.h.f.s.ConsumeRepositoryReconciliation(r.Context(), m.h.secret, ri.RepositoryConsumeReconciliation{ProviderID: "repo-fixture", OperationID: req.OperationID, RequestSHA256: req.RequestSHA256, ReconciliationTicket: req.ReconciliationTicket, Execution: packet.RepositoryExecution, Assignment: packet.Assignment})
		if err != nil {
			w.WriteHeader(403)
			return
		}
		g := ri.RepositoryAuthorizationGrant{OperationID: grant.OperationID, RequestSHA256: grant.RequestSHA256, Execution: grant.Execution, Assignment: grant.Assignment}
		state := "not_applied"
		if grant.Execution.Phase == "prepare" {
			state = "blocked"
		}
		o := repositoryTestObservation(g, state, grant.ReconciliationID)
		if _, err = m.h.f.s.RecordRepositoryObservation(r.Context(), m.h.secret, o); err != nil {
			w.WriteHeader(409)
			return
		}
		m.mu.Lock()
		m.Observations[o.OperationID] = o
		m.mu.Unlock()
		json.NewEncoder(w).Encode(o)
		return
	}
	w.WriteHeader(404)
}
func (h *repositoryFixture) create(t *testing.T) RepositoryIntegrationPlan {
	t.Helper()
	var err error
	h.plan, err = h.f.s.CreateRepositoryIntegrationPlan(context.Background(), h.f.e, h.f.actor, h.f.project, RepositoryIntegrationPlanCreateRequest{ProjectID: h.f.project, TaskID: h.task.ID, ExpectedTaskVersion: h.task.Version, SourceNodeID: "code", TargetID: h.target.ID, ExpectedTargetVersion: h.target.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return h.plan
}
func (h *repositoryFixture) claim(t *testing.T) dag.Assignment {
	t.Helper()
	reply := h.f.bearer("POST", "/runner/claim", h.workerToken, map[string]any{"project_id": h.f.project, "worker_id": "repo-worker", "capabilities": []string{"external_workflow:repo-fixture"}, "idempotency_key": newID()}, 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID == "" || a.RepositoryIntegration == nil {
		t.Fatal("missing actual managed repository assignment", reply)
	}
	if a.Config["purpose"] != "repository_integration" {
		t.Fatal("claim omitted the dedicated repository runtime purpose", a.Config)
	}
	return a
}
func repositoryTestRef(a dag.Assignment) cc.CandidateAssignmentRef {
	if a.PoolID == "" {
		return cc.CandidateAssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	}
	var ref cc.CandidateAssignmentRef
	json.Unmarshal([]byte(stringify(a)), &ref)
	return ref
}
func (h *repositoryFixture) consume(t *testing.T, a dag.Assignment) ri.RepositoryAuthorizationGrant {
	t.Helper()
	ticket, err := h.f.s.AuthorizeRepositoryIntegration(context.Background(), toolActorForTest(t, h.f, h.workerToken), ri.RepositoryAuthorizationRequest{ProjectID: h.f.project, Assignment: repositoryTestRef(a), OperationID: a.RepositoryIntegration.OperationID})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := h.f.s.ConsumeRepositoryAuthorization(context.Background(), h.secret, ri.RepositoryConsumeAuthorization{ProviderID: "repo-fixture", AuthorizationTicket: ticket.AuthorizationTicket, OperationID: ticket.OperationID, Assignment: repositoryTestRef(a), RequestSHA256: ticket.RequestSHA256})
	if err != nil {
		t.Fatal(err)
	}
	return grant
}
func repositoryTestObservation(g ri.RepositoryAuthorizationGrant, state, grantID string) ri.RepositoryObservation {
	o := ri.RepositoryObservation{Protocol: ri.RepositoryProtocol, OperationID: g.OperationID, RequestSHA256: g.RequestSHA256, Phase: g.Execution.Phase, State: state, Version: 1, Execution: g.Execution, EffectGrantID: grantID, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if state == "prepared" {
		p := ri.RepositoryPreview{TargetHead: g.Execution.ExpectedTargetHead, CandidateHead: g.Execution.Source.HeadCommit, Files: []ri.RepositoryFileChange{{Path: "feature.py", Status: "M", OldOID: strings.Repeat("a", 40), NewOID: strings.Repeat("b", 40)}}, Diff: "diff --git a/feature.py b/feature.py\n+typed boundary fixture\n", SourceBundleSHA256: g.Execution.Source.BundleSHA256, ConfigSHA256: g.Execution.TargetConfigSHA256, FastForward: true}
		p.DiffSHA256 = digest(p.Diff)
		p.PreparedSHA256, _ = ri.RepositoryPreviewDigest(p)
		o.Preview = &p
	}
	if state == "applied" || state == "not_applied" {
		e := g.Execution
		receipt := ri.RepositoryReceipt{Protocol: ri.RepositoryProtocol, OperationID: g.OperationID, RequestSHA256: g.RequestSHA256, TargetID: e.TargetID, TargetAlias: e.TargetAlias, TargetRef: e.TargetRef, TargetConfigSHA256: e.TargetConfigSHA256, BeforeCommit: e.ExpectedTargetHead, SourceBundleSHA256: e.Source.BundleSHA256, PreparedSHA256: e.PreparedSHA256, Generation: e.Generation, EffectGrantID: grantID, State: state}
		if state == "applied" {
			receipt.ResultCommit = e.Source.HeadCommit
		}
		o.Receipt = &receipt
		o.ReceiptSHA256, _ = ri.RepositoryReceiptDigest(receipt)
		o.ReceiptOID, _ = ri.RepositoryReceiptOID(receipt, "sha1")
	}
	o.ObservationSHA256, _ = ri.RepositoryObservationDigest(o)
	return o
}
func (h *repositoryFixture) observation(t *testing.T, g ri.RepositoryAuthorizationGrant, state, grantID string) ri.RepositoryObservation {
	t.Helper()
	return repositoryTestObservation(g, state, grantID)
}
func (h *repositoryFixture) complete(t *testing.T, a dag.Assignment, o ri.RepositoryObservation) {
	t.Helper()
	if _, err := h.f.s.RecordRepositoryObservation(context.Background(), h.secret, o); err != nil {
		t.Fatal(err)
	}
	h.mock.mu.Lock()
	h.mock.Observations[o.OperationID] = o
	h.mock.mu.Unlock()
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), IntegrationEvidence: &ri.RepositoryResult{OperationID: o.OperationID, RequestSHA256: o.RequestSHA256, ObservationSHA256: o.ObservationSHA256, Version: o.Version}}
	if !repositoryKnownObservation(o) {
		result.OutcomeUnknown = true
		result.ExitCode = -1
	} else if o.State != "prepared" && o.State != "applied" {
		result.ExitCode = 1
	}
	h.f.bearer("POST", "/runner/results", h.workerToken, map[string]any{"project_id": h.f.project, "result": result}, 200)
}
func (h *repositoryFixture) current(t *testing.T) RepositoryIntegrationPlan {
	t.Helper()
	p, err := h.f.s.GetRepositoryIntegrationPlan(context.Background(), h.f.e, h.f.actor, h.f.project, h.plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	h.plan = p
	return p
}
func (h *repositoryFixture) command(t *testing.T, action string) RepositoryIntegrationPlan {
	t.Helper()
	p := h.current(t)
	out, err := h.f.s.CommandRepositoryIntegrationPlan(context.Background(), h.f.e, h.f.actor, h.f.project, p.ID, RepositoryIntegrationPlanCommandRequest{ProjectID: h.f.project, ExpectedVersion: p.Version, Action: action, Reason: "Explicit test-owner " + action + " of typed protocol fixture; not production acceptance", Acknowledge: true, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.plan = out
	return out
}

func TestRepositoryIntegrationAcceptedBasisAuthenticatedPrepareApplyAndReplay(t *testing.T) {
	h := newRepositoryFixture(t)
	f, ctx := h.f, context.Background()
	options, err := f.s.RepositoryIntegrationOptions(ctx, f.actor, f.project, h.task.ID)
	if err != nil || !options.CanCreate || len(options.Sources) != 1 || options.Sources[0].SourceNodeID != "code" {
		t.Fatal(options, err)
	}
	h.create(t)
	a := h.claim(t)
	grant := h.consume(t, a)
	bytes, err := f.s.ProviderRepositoryBundle(ctx, h.secret, ri.RepositoryBundleRequest{AuthorizationID: grant.AuthorizationID, OperationID: grant.OperationID, Assignment: grant.Assignment, RequestSHA256: grant.RequestSHA256})
	if err != nil || string(bytes) != "candidate bytes" {
		t.Fatal("closed source requires no fake source lease", err)
	}
	o := h.observation(t, grant, "prepared", "")
	h.complete(t, a, o)
	p := h.current(t)
	if p.State != "ready" || p.Preview == nil || !p.CanApprove {
		t.Fatal(p)
	}
	h.command(t, "approve")
	p = h.command(t, "execute")
	if p.State != "applying" || p.ApplyRunID == p.PrepareRunID {
		t.Fatal(p)
	}
	a = h.claim(t)
	grant = h.consume(t, a)
	effect, err := f.s.BeginRepositoryEffect(ctx, h.secret, ri.RepositoryBeginEffect{AuthorizationID: grant.AuthorizationID, OperationID: grant.OperationID, Assignment: grant.Assignment, RequestSHA256: grant.RequestSHA256, Generation: grant.Execution.Generation})
	if err != nil {
		t.Fatal(err)
	}
	o = h.observation(t, grant, "applied", effect.EffectGrantID)
	h.complete(t, a, o)
	p = h.current(t)
	if p.State != "integrated" || p.CanExecute || p.Observation.Receipt == nil {
		t.Fatal(p)
	}
	targets, err := f.s.ListRepositoryIntegrationTargets(ctx, f.actor, f.project)
	if err != nil || targets[0].ActiveOperationID != "" || targets[0].Generation != 1 {
		t.Fatal(targets, err)
	}
	if _, err = f.s.RecordRepositoryObservation(ctx, h.secret, o); err != nil {
		t.Fatal(err)
	}
	again := h.current(t)
	if again.Version != p.Version {
		t.Fatal("provider callback replay changed original effect", again)
	}
}
