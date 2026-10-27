package platform

import (
	"context"
	"strings"
	"sync"
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

// These are genuine Control/Core/transfer/test/Review facts at a typed byte
// boundary. Git's actual three-way and bundle import are tested by the Runner.
type compositionFixture struct {
	f     *fixture
	token string
	req   CandidateCompositionCreateRequest
}

func compositionAcceptSource(t *testing.T, f *fixture, token string, r cc.StartRequest) Record {
	t.Helper()
	ctx := context.Background()
	task, e := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Accepted source " + r.HeadCommit[:4]}))
	if e != nil {
		t.Fatal(e)
	}
	bounded, a, close, e := f.s.planningAuthority(ctx, f.actor, f.project, task.ID, purposeProjectWrite)
	if e != nil {
		t.Fatal(e)
	}
	e = f.s.BindRunTx(bounded, a.tx, a, f.project, task.ID, r.Assignment.RunID, task.Version)
	if e == nil {
		e = a.tx.Commit()
	}
	close()
	if e != nil {
		t.Fatal(e)
	}
	b := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201))
	candidateBinary(t, f, "runner", token, b.TransferID, newID(), r.Assignment, "PUT", []byte("candidate bytes"), 200)
	b = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+b.TransferID+"/seal", token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: r.Assignment, OperationID: newID()}), 200))
	consumer := candidateCompleteAndClaim(t, f, token, r, b)
	a2 := consumer.Assignment
	proof := map[string]any{"head_commit": r.HeadCommit, "base_commit": r.HeadCommit, "evidence_sha256": strings.Repeat("d", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "test_command": true, "candidate_from": "code", "candidate_attempt": r.Assignment.Attempt, "candidate_fence": r.Assignment.Fence, "candidate_evidence_sha256": r.EvidenceSHA256}
	f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": dag.Result{RunID: a2.RunID, NodeID: a2.NodeID, Attempt: a2.Attempt, Fence: a2.Fence, WorkerID: a2.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}}, 200)
	task, e = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": 2, "run_id": r.Assignment.RunID, "decision": "accepted", "reason": "Explicit test-owner acceptance of typed fixture"}), f.e)
	if e != nil {
		t.Fatal(e)
	}
	return task
}
func newCompositionFixture(t *testing.T) *compositionFixture {
	t.Helper()
	ctx := context.Background()
	f, token, r := candidateTransferFixture(t)
	if e := f.s.InitializeCandidateCompositions(ctx); e != nil {
		t.Fatal(e)
	}
	first := compositionAcceptSource(t, f, token, r)
	w, e := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Second independent code source", Nodes: []dag.Node{{ID: "code", Type: "command", Command: []string{"develop"}}, {ID: "test", Type: "test", Command: []string{"check"}, DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if e != nil {
		t.Fatal(e)
	}
	a, e := f.e.Claim(ctx, f.user.TenantID, f.project, "source-worker", []string{"command:develop"})
	if e != nil || a == nil {
		t.Fatal(a, e)
	}
	r.Assignment = cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	r.HeadCommit = strings.Repeat("c", 40)
	r.OperationID = newID()
	second := compositionAcceptSource(t, f, token, r)
	template, e := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Fixed integration and formal checks", Nodes: []dag.Node{{ID: "compose", Type: "agent", Command: []string{"old-agent"}, Config: map[string]any{"goal": "Integrate explicitly selected sources"}}, {ID: "verify", Type: "test", Command: []string{"check"}, DependsOn: []string{"compose"}, Config: map[string]any{"candidate_from": "compose"}}}})
	if e != nil {
		t.Fatal(e)
	}
	task, e := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Explicit integration task"}))
	if e != nil {
		t.Fatal(e)
	}
	return &compositionFixture{f, token, CandidateCompositionCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: template.ID, WorkflowVersion: template.Version, ProducerNodeID: "compose", Sources: []CandidateCompositionSourceRequest{{first.ID, first.Version, "code"}, {second.ID, second.Version, "code"}}, IdempotencyKey: newID()}}
}
func (h *compositionFixture) create(t *testing.T) CandidateCompositionPlan {
	t.Helper()
	p, e := h.f.s.CreateCandidateComposition(context.Background(), h.f.e, h.f.actor, h.f.project, h.req)
	if e != nil {
		t.Fatal(e)
	}
	return p
}
func (h *compositionFixture) start(t *testing.T, p CandidateCompositionPlan) CandidateCompositionPlan {
	t.Helper()
	p, e := h.f.s.StartCandidateComposition(context.Background(), h.f.e, h.f.actor, h.f.project, p.ID, CandidateCompositionStartRequest{ProjectID: h.f.project, ExpectedVersion: p.Version, Acknowledge: true, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestCandidateCompositionPreviewOrderAndAtomicStartReplay(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p := h.create(t)
	if p.RunID != "" || len(p.Sources) != 2 || p.Sources[0].TaskID != h.req.Sources[0].TaskID || p.Workflow.Nodes[0].Type != "command" || p.Workflow.Nodes[0].Command[0] != ccmp.CompositionCommand || p.Workflow.Nodes[1].Config["candidate_from"] != "compose" {
		t.Fatal(p)
	}
	again, e := f.s.CreateCandidateComposition(ctx, f.e, f.actor, f.project, h.req)
	if e != nil || again.ID != p.ID {
		t.Fatal(again, e)
	}
	changed := h.req
	changed.Sources = append([]CandidateCompositionSourceRequest{}, h.req.Sources...)
	changed.Sources[0], changed.Sources[1] = changed.Sources[1], changed.Sources[0]
	_, e = f.s.CreateCandidateComposition(ctx, f.e, f.actor, f.project, changed)
	expertStatus(t, e, 409)
	req := CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: p.Version, Acknowledge: true, IdempotencyKey: newID()}
	var wg sync.WaitGroup
	out := make(chan CandidateCompositionPlan, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, e := f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, req)
			out <- v
			errs <- e
		}()
	}
	wg.Wait()
	close(out)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var run string
	for v := range out {
		if v.RunID == "" || run != "" && run != v.RunID {
			t.Fatal(v)
		}
		run = v.RunID
	}
	if expertCount(t, f, "suma_candidate_composition_executions") != 1 {
		t.Fatal("multiple execution bindings")
	}
	task, e := f.s.Get(ctx, f.actor, f.project, "tasks", p.TaskID)
	if e != nil || task.RunID != run || task.Version != h.req.ExpectedTaskVersion+1 {
		t.Fatal(task, e)
	}
	// A new current session may open the original operation without changing
	// its source delegation or making another managed-run identity.
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
	current, e := f.s.AuthenticateActor(ctx, f.cookie.Value)
	if e != nil {
		t.Fatal(e)
	}
	recovered, e := f.s.StartCandidateComposition(ctx, f.e, current, f.project, p.ID, req)
	if e != nil || recovered.RunID != run || expertCount(t, f, "suma_candidate_composition_executions") != 1 {
		t.Fatal(recovered, e)
	}
}
func TestCandidateCompositionSourceCurrentAndTemplateGuards(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p := h.create(t)
	_, e := f.s.Update(ctx, f.actor, f.project, "tasks", h.req.Sources[0].TaskID, f.body(map[string]any{"expected_version": h.req.Sources[0].ExpectedTaskVersion, "description": "Explicit source correction"}))
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: 1, Acknowledge: true, IdempotencyKey: newID()})
	if e == nil {
		t.Fatal("changed source accepted")
	}
	current, e := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if e != nil || current.Valid || len(current.Blockers) == 0 {
		t.Fatal(current, e)
	}
	if expertCount(t, f, "suma_candidate_composition_executions") != 0 {
		t.Fatal("rejected start created binding")
	}
	for _, w := range []dag.Workflow{{Nodes: []dag.Node{{ID: "compose", Type: "agent"}}}, {Nodes: []dag.Node{{ID: "compose", Type: "agent"}, {ID: "shell", Type: "command", Command: []string{"unapproved"}}}}} {
		if _, e = buildCandidateCompositionWorkflow(w, "compose"); e == nil {
			t.Fatal("weakened template accepted")
		}
	}
}
func TestCandidateCompositionInputLeaseAndOriginalAuthority(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p := h.start(t, h.create(t))
	a, e := f.e.Claim(ctx, f.user.TenantID, f.project, "compose-worker", []string{"command:suma-compose"})
	if e != nil || a == nil {
		t.Fatal(a, e)
	}
	ref := cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	actor := domainCActor(t, f.s, h.token)
	read := func(request ccmp.CodeInputRequest) error {
		bounded, tx, close, e := f.s.beginAuthorityTx(ctx)
		if e != nil {
			return e
		}
		defer close()
		defer tx.Rollback()
		proof, e := f.s.candidateAuthorityTx(bounded, tx, projectCandidateSubject(actor, f.project), request.Assignment)
		if e != nil {
			return e
		}
		_, e = f.s.codeInputBindingTx(bounded, tx, proof, request)
		return e
	}
	req := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: ref, InputKind: "composition", BindingID: p.ID, TransferID: p.Sources[0].Source.TransferID}
	if e = read(req); e != nil {
		t.Fatal(e)
	}
	bad := req
	bad.TransferID = newID()
	expertStatus(t, read(bad), 403)
	bad = req
	bad.Assignment.Fence++
	expertStatus(t, read(bad), 409)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	expertStatus(t, read(req), 409)
}
func TestCandidateCompositionDismissPaginationAndCurrentReplayAuthority(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p := h.create(t)
	req2 := h.req
	req2.IdempotencyKey = newID()
	second, e := f.s.CreateCandidateComposition(ctx, f.e, f.actor, f.project, req2)
	if e != nil {
		t.Fatal(e)
	}
	page, e := f.s.ListCandidateCompositions(ctx, f.e, f.actor, f.project, p.TaskID, 1, "")
	if e != nil || page.Total != 2 || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal(page, e)
	}
	next, e := f.s.ListCandidateCompositions(ctx, f.e, f.actor, f.project, p.TaskID, 1, page.NextCursor)
	if e != nil || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID {
		t.Fatal(next, e)
	}
	_, e = f.s.ListCandidateCompositions(ctx, f.e, f.actor, f.project, "", 1, page.NextCursor)
	expertStatus(t, e, 400)
	cmd := CandidateCompositionCommandRequest{ProjectID: f.project, ExpectedVersion: 1, Action: "dismiss", Reason: "Human explicitly closes this preview", IdempotencyKey: newID()}
	p, e = f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, cmd)
	if e != nil || p.State != "dismissed" || len(p.AllowedActions) != 0 {
		t.Fatal(p, e)
	}
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	_, e = f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, cmd)
	if e == nil {
		t.Fatal("revoked receipt recovered")
	}
	_ = second
}

func TestCandidateCompositionBaselineRequiresFormalAcceptance(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p := h.start(t, h.create(t))
	target, e := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Explicit downstream development"}))
	if e != nil {
		t.Fatal(e)
	}
	req := TaskCodeBaselineBindRequest{ProjectID: f.project, ExpectedTaskVersion: target.Version, CompositionID: p.ID, ExpectedCompositionVersion: p.Version, Acknowledge: true, Reason: "Select the accepted combined baseline", IdempotencyKey: newID()}
	_, e = f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, req)
	if e == nil {
		t.Fatal("unaccepted composition granted baseline")
	}
	if expertCount(t, f, "suma_task_code_baselines") != 0 {
		t.Fatal("unaccepted source wrote protected binding")
	}
	got, e := f.s.GetTaskCodeBaseline(ctx, f.actor, f.project, target.ID)
	if e != nil || got.Binding != nil || got.Version != 0 || !got.CanBind {
		t.Fatal(got, e)
	}
}

func compositionCompleteFixedCandidate(t *testing.T, h *compositionFixture, p CandidateCompositionPlan) (CandidateCompositionPlan, cc.Binding) {
	t.Helper()
	f, ctx := h.f, context.Background()
	assigned, e := f.e.Claim(ctx, f.user.TenantID, f.project, "compose-worker", []string{"command:suma-compose"})
	if e != nil || assigned == nil {
		t.Fatal(assigned, e)
	}
	r := cc.StartRequest{ProjectID: f.project, Assignment: cc.AssignmentRef{RunID: assigned.RunID, NodeID: assigned.NodeID, Attempt: assigned.Attempt, Fence: assigned.Fence, WorkerID: assigned.WorkerID}, OperationID: newID(), BaseCommit: p.AdminBaseCommit, HeadCommit: strings.Repeat("d", 40), EvidenceSHA256: strings.Repeat("f", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
	b := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", h.token, candidateJSON(t, r), 201))
	candidateBinary(t, f, "runner", h.token, b.TransferID, newID(), r.Assignment, "PUT", []byte("candidate bytes"), 200)
	b = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+b.TransferID+"/seal", h.token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: r.Assignment, OperationID: newID()}), 200))
	proof := candidateSourceProof(r, b)
	proof["candidate_composition"] = map[string]any{"protocol": ccmp.CandidateCompositionProtocol, "plan_id": p.ID, "plan_version": p.Version, "sources_sha256": p.SourcesSHA256, "operation_id": p.OperationID, "base_commit": p.AdminBaseCommit, "state": "completed", "result_commit": r.HeadCommit, "result_tree": strings.Repeat("e", 40), "parents": []string{p.Sources[0].Source.HeadCommit, p.Sources[1].Source.HeadCommit}, "source_heads": []string{p.Sources[0].Source.HeadCommit, p.Sources[1].Source.HeadCommit}, "merge_commits": []string{r.HeadCommit}, "conflicts": []string{}}
	a := r.Assignment
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}
	func() {
		bounded, tx, close, e := f.s.beginAuthorityTx(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer close()
		defer tx.Rollback()
		if e = f.s.validateCandidateCompositionResultTx(bounded, tx, f.user.TenantID, f.project, result); e != nil {
			t.Fatal(e)
		}
		bad := result
		bad.WorkspaceEvidence = candidateJSON(t, proof)
		bad.WorkspaceEvidence["candidate_composition"].(map[string]any)["sources_sha256"] = strings.Repeat("9", 64)
		expertStatus(t, f.s.validateCandidateCompositionResultTx(bounded, tx, f.user.TenantID, f.project, bad), 409)
	}()
	f.bearer("POST", "/runner/results", h.token, map[string]any{"project_id": f.project, "result": result}, 200)
	consumer, e := f.e.Claim(ctx, f.user.TenantID, f.project, "independent-check-worker", []string{"command:check"})
	if e != nil || consumer == nil || consumer.CandidateCommit != r.HeadCommit {
		t.Fatal(consumer, e)
	}
	q := map[string]any{"head_commit": r.HeadCommit, "base_commit": r.HeadCommit, "evidence_sha256": strings.Repeat("8", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "test_command": true, "candidate_from": "compose", "candidate_attempt": a.Attempt, "candidate_fence": a.Fence, "candidate_evidence_sha256": r.EvidenceSHA256}
	f.bearer("POST", "/runner/results", h.token, map[string]any{"project_id": f.project, "result": dag.Result{RunID: consumer.RunID, NodeID: consumer.NodeID, Attempt: consumer.Attempt, Fence: consumer.Fence, WorkerID: consumer.WorkerID, OperationID: newID(), WorkspaceEvidence: q}}, 200)
	p, e = f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if e != nil || p.State != "awaiting_review" {
		t.Fatal(p, e)
	}
	return p, b
}

func compositionAcceptFixedCandidate(t *testing.T, h *compositionFixture, p CandidateCompositionPlan) CandidateCompositionPlan {
	t.Helper()
	f, ctx := h.f, context.Background()
	task, e := f.s.Get(ctx, f.actor, f.project, "tasks", p.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Review(ctx, f.actor, f.project, p.TaskID, f.body(map[string]any{"expected_version": task.Version, "run_id": p.RunID, "decision": "accepted", "reason": "Explicit human acceptance after independent exact-candidate tests"}), f.e)
	if e != nil {
		t.Fatal(e)
	}
	p, e = f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if e != nil || p.State != "accepted" {
		t.Fatal(p, e)
	}
	return p
}

func TestCandidateCompositionAcceptedBaselineCASAndFrozenDevelopmentInput(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p, b := compositionCompleteFixedCandidate(t, h, h.start(t, h.create(t)))
	target, e := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Fixed inherited development"}))
	if e != nil {
		t.Fatal(e)
	}
	request := TaskCodeBaselineBindRequest{ProjectID: f.project, ExpectedTaskVersion: target.Version, CompositionID: p.ID, ExpectedCompositionVersion: p.Version, Acknowledge: true, Reason: "Use the formally accepted combined source", IdempotencyKey: newID()}
	_, e = f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, request)
	if e == nil {
		t.Fatal("successful tests silently accepted composition")
	}
	task, e := f.s.Get(ctx, f.actor, f.project, "tasks", p.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Review(ctx, f.actor, f.project, p.TaskID, f.body(map[string]any{"expected_version": task.Version, "run_id": p.RunID, "decision": "accepted", "reason": "Explicit human acceptance after independent exact-candidate tests"}), f.e)
	if e != nil {
		t.Fatal(e)
	}
	updated, e := f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, request)
	if e != nil || updated.Version != target.Version+1 {
		t.Fatal(updated, e)
	}
	again, e := f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, request)
	if e != nil || again.Version != updated.Version {
		t.Fatal(again, e)
	}
	view, e := f.s.GetTaskCodeBaseline(ctx, f.actor, f.project, target.ID)
	if e != nil || view.Version != 1 || view.Binding == nil || view.Binding.Source != b || view.Binding.TaskVersion != updated.Version {
		t.Fatal(view, e)
	}
	mixed := h.req
	mixed.TaskID = target.ID
	mixed.ExpectedTaskVersion = updated.Version
	mixed.IdempotencyKey = newID()
	_, e = f.s.CreateCandidateComposition(ctx, f.e, f.actor, f.project, mixed)
	expertStatus(t, e, 409)
	stale := request
	stale.IdempotencyKey = newID()
	stale.ExpectedTaskVersion = updated.Version
	_, e = f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, stale)
	expertStatus(t, e, 409)
	_, e = f.s.Update(ctx, f.actor, f.project, "tasks", target.ID, f.body(map[string]any{"expected_version": updated.Version, "description": "Explicit changed scope invalidates the pending baseline preview"}))
	if e != nil {
		t.Fatal(e)
	}
	view, e = f.s.GetTaskCodeBaseline(ctx, f.actor, f.project, target.ID)
	if e != nil || len(view.Blockers) == 0 {
		t.Fatal(view, e)
	}
}
