package platform

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	dc "github.com/ai-mada/suma/internal/developmentcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func continuationSourceFixture(t *testing.T) (*fixture, string, Record, cc.Binding) {
	t.Helper()
	f, token, request := candidateTransferFixture(t)
	ctx := context.Background()
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Continue failed quality run"}))
	if err != nil {
		t.Fatal(err)
	}
	task, b := continuationFinishSource(t, f, token, task, request)
	return f, token, task, b
}

func continuationFinishSource(t *testing.T, f *fixture, token string, task Record, request cc.StartRequest) (Record, cc.Binding) {
	t.Helper()
	ctx := context.Background()
	bounded, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, task.ID, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.BindRunTx(bounded, auth.tx, auth, f.project, task.ID, request.Assignment.RunID, task.Version)
	if err == nil {
		err = auth.tx.Commit()
	}
	close()
	if err != nil {
		t.Fatal(err)
	}
	b := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, request), 201))
	candidateBinary(t, f, "runner", token, b.TransferID, newID(), request.Assignment, "PUT", []byte("candidate bytes"), 200)
	b = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+b.TransferID+"/seal", token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: request.Assignment, OperationID: newID()}), 200))
	consumer := candidateCompleteAndClaim(t, f, token, request, b)
	a := consumer.Assignment
	f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), ExitCode: 1, Error: "Independent quality test failed"}}, 200)
	task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return task, b
}

func continuationDiscussionFixture(t *testing.T) (*taskReplyFixture, TaskDiscussionDevelopmentCreateRequest, cc.Binding, string) {
	t.Helper()
	h := newTaskReplyFixture(t)
	f, ctx := h.f, context.Background()
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.request.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Previous development", Nodes: []dag.Node{{ID: "code", Type: "command", Command: []string{"previous-code"}}, {ID: "test", Type: "test", Command: []string{"check"}, DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID()); err != nil {
		t.Fatal(err)
	}
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "previous-worker", []string{"command:previous-code"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	sourceRequest := cc.StartRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(*a), OperationID: newID(), BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("b", 40), EvidenceSHA256: strings.Repeat("e", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
	task, source := continuationFinishSource(t, f, token, task, sourceRequest)
	h.request.ExpectedTaskVersion = task.Version
	reply, result := h.start(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	reply, err = f.s.GetTaskConversationReply(ctx, f.e, f.actor, f.project, reply.ID)
	if err != nil || reply.State != "completed" {
		t.Fatal(reply, err)
	}
	request := taskDiscussionDevelopmentRequestForReply(t, h, reply)
	worker := "continuation-worker"
	workerToken := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	p, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Fixed continuation developer", Profile: toolTestProfile(), IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, workerToken), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	request.ToolProfileID, request.ExpectedToolVersion, request.ModelExecutionSelection = p.ID, p.Version, nil
	options, err := f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 20, "")
	if err != nil || len(options.Items) != 1 {
		t.Fatal(options, err)
	}
	request.ContinuationSource = &options.Items[0].Selection
	return h, request, source, workerToken
}

func TestTaskDevelopmentContinuationStartBrokerCandidateAndFreshQuality(t *testing.T) {
	h, request, source, token := continuationDiscussionFixture(t)
	f, ctx := h.f, context.Background()
	plan, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil || plan.DevelopmentContinuation == nil || plan.DispatchPlan.DevelopmentContinuation == nil || plan.DevelopmentContinuation.Source != source {
		t.Fatal(plan, err)
	}
	if again, e := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request); e != nil || again.ID != plan.ID {
		t.Fatal(again, e)
	}
	if _, err = f.s.DB.Exec(`UPDATE dag_runs SET version=version+1 WHERE id=?`, source.Source.RunID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
	if _, err = f.s.DB.Exec(`UPDATE dag_runs SET version=version-1 WHERE id=?`, source.Source.RunID); err != nil {
		t.Fatal(err)
	}
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}
	run, err := f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	if again, e := f.s.StartTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID, start); e != nil || again.ID != run.ID {
		t.Fatal(again, e)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", request.TaskID)
	if err != nil || current.RunID != run.ID || current.RunID == source.Source.RunID {
		t.Fatal(current, err)
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": "continuation-worker", "capabilities": []string{"command:native-agent"}, "idempotency_key": newID()}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &a) != nil || a.RunID != run.ID || a.DevelopmentContinuation == nil || *a.DevelopmentContinuation != *plan.DevelopmentContinuation || a.CodeBaseline != nil {
		t.Fatal(claim)
	}
	input := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(a), InputKind: "development_continuation", BindingID: plan.ID, TransferID: source.TransferID}
	read := func(req ccmp.CodeInputRequest, want int) []byte {
		r := httptest.NewRequest("POST", "/api/v1/runner/code-inputs/bundle", strings.NewReader(stringify(req)))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("broker %d want %d: %s", w.Code, want, w.Body.String())
		}
		return w.Body.Bytes()
	}
	if string(read(input, 200)) != "candidate bytes" {
		t.Fatal("original source bytes changed")
	}
	bad := input
	bad.BindingID = newID()
	read(bad, 403)
	review := f.body(map[string]any{"expected_version": current.Version, "run_id": run.ID, "decision": "accepted", "reason": "Current run needs its own test"})
	if _, err = f.s.Review(ctx, f.actor, f.project, current.ID, review, f.e); err == nil {
		t.Fatal("old quality facts accepted new running code")
	}
	r := cc.StartRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(a), OperationID: newID(), BaseCommit: source.HeadCommit, BundleBaseCommit: source.BundleBase(), HeadCommit: strings.Repeat("c", 40), EvidenceSHA256: strings.Repeat("f", 64), BundleSHA256: digest("candidate bytes N"), SizeBytes: int64(len("candidate bytes N"))}
	b := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201))
	candidateBinary(t, f, "runner", token, b.TransferID, newID(), r.Assignment, "PUT", []byte("candidate bytes N"), 200)
	b = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+b.TransferID+"/seal", token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: r.Assignment, OperationID: newID()}), 200))
	proof := candidateSourceProof(r, b)
	proof["admin_base_commit"] = source.BundleBase()
	proof["development_continuation"] = *plan.DevelopmentContinuation
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof, ToolEvidence: &tc.Evidence{Profile: toolTestProfile(), Terminal: "completed", EventCount: 3, UsageState: tc.UsageUnreported}}
	badResult := result
	badResult.OperationID = newID()
	badResult.WorkspaceEvidence = candidateJSON(t, proof)
	delete(badResult.WorkspaceEvidence, "development_continuation")
	f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": badResult}, 409)
	f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": result}, 200)
	// The original broker rejects the now-completed producer lease.
	read(input, 409)
	consumer, e := f.e.Claim(ctx, f.user.TenantID, f.project, "independent-new-worker", []string{"command:independent-tests"})
	if e != nil || consumer == nil || consumer.CandidateCommit != b.HeadCommit || consumer.CandidateCommit == source.HeadCommit || consumer.CandidateEvidenceSHA256 != b.EvidenceSHA256 {
		t.Fatal(consumer, e)
	}
	q := map[string]any{"head_commit": b.HeadCommit, "base_commit": b.HeadCommit, "evidence_sha256": strings.Repeat("9", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "test_command": true, "candidate_from": a.NodeID, "candidate_attempt": a.Attempt, "candidate_fence": a.Fence, "candidate_evidence_sha256": r.EvidenceSHA256}
	generic := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	f.bearer("POST", "/runner/results", generic, map[string]any{"project_id": f.project, "result": dag.Result{RunID: consumer.RunID, NodeID: consumer.NodeID, Attempt: consumer.Attempt, Fence: consumer.Fence, WorkerID: consumer.WorkerID, OperationID: newID(), WorkspaceEvidence: q}}, 200)
	accepted, err := f.s.Review(ctx, f.actor, f.project, current.ID, review, f.e)
	if err != nil || accepted.Status != "accepted" || accepted.RunID != run.ID {
		t.Fatal(accepted, err)
	}
}

func TestTaskDevelopmentContinuationOptionsReadOnlyAndScoped(t *testing.T) {
	f, token, task, source := continuationSourceFixture(t)
	before := domainACount(t, f.s, "dag_runs")
	path := "/tasks/" + task.ID + "/development-continuation-options?project_id=" + f.project
	got := f.call("GET", path, nil, 200)
	if got["total"] != float64(1) || got["complete"] != true || got["source_run_state"] != "failed" || got["source_task_run_id"] != source.Source.RunID {
		t.Fatal(got)
	}
	f.bearer("GET", path, token, nil, 403)
	if domainACount(t, f.s, "dag_runs") != before {
		t.Fatal("options started a run")
	}
}

func TestTaskDevelopmentContinuationOptionsPaginationAndUnavailable(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Multiple current producers"}))
	if err != nil {
		t.Fatal(err)
	}
	empty, err := f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 1, "")
	if err != nil || empty.Reason != "no_run" || !empty.Complete || empty.Total != 0 {
		t.Fatal(empty, err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Two previous producers", Nodes: []dag.Node{{ID: "one", Name: "First producer", Type: "command", Command: []string{"previous-one"}}, {ID: "two", Name: "Second producer", Type: "command", Command: []string{"previous-two"}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, w.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	bounded, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, task.ID, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.BindRunTx(bounded, auth.tx, auth, f.project, task.ID, run.ID, task.Version)
	if err == nil {
		err = auth.tx.Commit()
	}
	close()
	if err != nil {
		t.Fatal(err)
	}
	empty, err = f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 1, "")
	if err != nil || empty.Reason != "run_not_terminal" || empty.Total != 0 {
		t.Fatal(empty, err)
	}
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	for _, id := range []string{"one", "two"} {
		a, e := f.e.Claim(ctx, f.user.TenantID, f.project, "source-"+id, []string{"command:previous-" + id})
		if e != nil || a == nil {
			t.Fatal(a, e)
		}
		r := cc.StartRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(*a), OperationID: newID(), BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat(map[string]string{"one": "b", "two": "c"}[id], 40), EvidenceSHA256: strings.Repeat("e", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
		b := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, r), 201))
		candidateBinary(t, f, "runner", token, b.TransferID, newID(), r.Assignment, "PUT", []byte("candidate bytes"), 200)
		b = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+b.TransferID+"/seal", token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: r.Assignment, OperationID: newID()}), 200))
		f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: candidateSourceProof(r, b)}}, 200)
	}
	first, err := f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 1, "")
	if err != nil || first.Total != 2 || len(first.Items) != 1 || first.Complete || first.NextCursor == "" || first.Items[0].Selection.NodeID != "one" {
		t.Fatal(first, err)
	}
	second, err := f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 1, first.NextCursor)
	if err != nil || second.Total != 2 || len(second.Items) != 1 || !second.Complete || second.Items[0].Selection.NodeID != "two" {
		t.Fatal(second, err)
	}
	_, err = f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 2, first.NextCursor)
	requireAuthorityStatus(t, err, 400)
	_, err = f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 1, strings.Repeat("a", 4097))
	requireAuthorityStatus(t, err, 400)
	if _, err = f.s.DB.Exec(`UPDATE dag_runs SET version=version+1 WHERE id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 1, first.NextCursor)
	requireAuthorityStatus(t, err, 400)
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET status='accepted' WHERE id=?`, task.ID); err != nil {
		t.Fatal(err)
	}
	empty, err = f.s.ListTaskDevelopmentContinuationOptions(ctx, f.actor, f.project, task.ID, 20, "")
	if err != nil || empty.Reason != "task_accepted" || empty.Total != 0 || !empty.Complete {
		t.Fatal(empty, err)
	}
}

func TestTaskDevelopmentContinuationRejectsForgedSourceAndConfig(t *testing.T) {
	h, request, source, token := continuationDiscussionFixture(t)
	f, ctx := h.f, context.Background()
	for _, mutate := range []func(*dc.DevelopmentContinuationSource){func(s *dc.DevelopmentContinuationSource) { s.Fence++ }, func(s *dc.DevelopmentContinuationSource) { s.ExpectedRunVersion++ }, func(s *dc.DevelopmentContinuationSource) { s.RunID = newID() }} {
		r := request
		s := *request.ContinuationSource
		mutate(&s)
		r.ContinuationSource = &s
		r.IdempotencyKey = newID()
		if _, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, r); err == nil {
			t.Fatal("unverified source granted continuation")
		}
	}
	_, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, toolActorForTest(t, f, token), f.project, request)
	requireAuthorityStatus(t, err, 403)
	// The full binding never comes from a public template, even if correctly hashed.
	b := dc.DevelopmentContinuationBinding{Protocol: dc.ContinuationProtocol, ID: newID(), ProjectID: f.project, TaskID: request.TaskID, TaskVersion: request.ExpectedTaskVersion, SourceTaskRunID: source.Source.RunID, SourceTaskVersion: request.ExpectedTaskVersion, SourceRunVersion: request.ContinuationSource.ExpectedRunVersion, Source: source}
	b.BindingSHA256, err = dc.BindingDigest(b)
	if err != nil {
		t.Fatal(err)
	}
	w, p, context := taskDispatchBuildFixture()
	context.Task.ID = b.TaskID
	context.Task.Version = b.TaskVersion
	w.Nodes[0].Config["development_continuation"] = b
	w.Nodes[0].Config["development_continuation_id"] = b.ID
	ordinary, err := buildTaskDispatchWorkflow(w, "develop", "profile", p, context)
	if err != nil || ordinary.Nodes[0].Config["development_continuation"] != nil || ordinary.Nodes[0].Config["development_continuation_id"] != nil {
		t.Fatal(ordinary, err)
	}
	for _, purpose := range []any{"", nil, "task_conversation_reply"} {
		w.Nodes[0].Config["purpose"] = purpose
		if _, err = buildTaskDispatchWorkflowInputs(w, "develop", "profile", p, context, &b); err == nil {
			t.Fatal("purpose-bearing node became ordinary continuation")
		}
	}
	delete(w.Nodes[0].Config, "purpose")
	ancestor := ccmp.CodeBaselineBinding{Protocol: ccmp.CodeBaselineProtocol, ID: newID(), ProjectID: f.project, TaskID: context.Task.ID, TaskVersion: context.Task.Version - 1, CompositionID: newID(), CompositionVersion: 1, AcceptedBasisSHA256: strings.Repeat("d", 64), Source: source}
	ancestor.BindingSHA256, err = ccmp.CodeBaselineBindingDigest(ancestor)
	if err != nil {
		t.Fatal(err)
	}
	context.CodeBaseline = &ancestor
	continued, err := buildTaskDispatchWorkflowInputs(w, "develop", "profile", p, context, &b)
	if err != nil || continued.Nodes[0].Config["code_baseline"] != nil || continued.Nodes[0].Config["development_continuation_id"] != b.ID || context.CodeBaseline == nil || *context.CodeBaseline != ancestor {
		t.Fatal("ancestor replaced the immediate input or lost its historical context", continued, err)
	}
	// A forged marker on an unbound published command cannot advertise a code input.
	bounded, tx, close, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	defer tx.Rollback()
	eligible, err := f.s.codeInputNodeEligibleTx(bounded, tx, f.user.TenantID, f.project, source.Source.RunID, source.Source.NodeID, dag.Node{ID: source.Source.NodeID, Type: "agent", Config: map[string]any{"development_continuation_id": b.ID}})
	if err != nil || eligible {
		t.Fatal(eligible, err)
	}
}

func TestTaskDevelopmentContinuationRechecksAcceptedAncestorBeforeImport(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	accepted, _ := compositionCompleteFixedCandidate(t, h, h.start(t, h.create(t)))
	accepted = compositionAcceptFixedCandidate(t, h, accepted)
	profile, token, workflow := compositionDispatchProfile(t, f)
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Preserve accepted ancestor while continuing"}))
	if err != nil {
		t.Fatal(err)
	}
	task = compositionBindBaseline(t, h, accepted, task, 0)
	ancestor := *task.CodeBaseline
	previous, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Previous same-task candidate", Nodes: []dag.Node{{ID: "code", Type: "command", Command: []string{"ancestor-previous"}}, {ID: "test", Type: "test", Command: []string{"check"}, DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.e.CreateRun(ctx, f.user.TenantID, f.project, previous.ID, newID()); err != nil {
		t.Fatal(err)
	}
	old, err := f.e.Claim(ctx, f.user.TenantID, f.project, "ancestor-previous-worker", []string{"command:ancestor-previous"})
	if err != nil || old == nil {
		t.Fatal(old, err)
	}
	sourceReq := cc.StartRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(*old), OperationID: newID(), BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("e", 40), EvidenceSHA256: strings.Repeat("e", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
	task, source := continuationFinishSource(t, f, h.token, task, sourceReq)
	var sourceVersion int
	if err = f.s.DB.QueryRow(`SELECT version FROM dag_runs WHERE id=?`, source.Source.RunID).Scan(&sourceVersion); err != nil {
		t.Fatal(err)
	}
	bounded, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, task.ID, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	id := newID()
	b, err := f.s.resolveDevelopmentContinuationTx(bounded, auth, f.project, task.ID, id, task.Version, dc.DevelopmentContinuationSource{RunID: source.Source.RunID, NodeID: source.Source.NodeID, Attempt: source.Source.Attempt, Fence: source.Source.Fence, ExpectedRunVersion: sourceVersion})
	var plan TaskDispatchPlan
	if err == nil {
		plan, err = f.s.buildTaskDispatchSnapshotInputsTx(bounded, f.e, auth, f.project, TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: workflow.ID, WorkflowVersion: workflow.Version, AgentNodeID: "develop", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version}, nil, b)
	}
	if err == nil {
		plan.ID, plan.RunID = id, newID()
		err = f.s.persistTaskDispatchSnapshotTx(bounded, auth, &plan)
	}
	if err == nil {
		err = auth.tx.Commit()
	}
	close()
	if err != nil || plan.Context.CodeBaseline == nil || *plan.Context.CodeBaseline != ancestor || plan.CodeBaseline != nil || plan.Workflow.Nodes[0].Config["code_baseline"] != nil {
		t.Fatal(plan, err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	claim := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}, "idempotency_key": newID()}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(claim["assignment"])), &a) != nil || a.RunID != run.ID || a.DevelopmentContinuation == nil || a.CodeBaseline != nil {
		t.Fatal(claim)
	}
	origin, err := f.s.Get(ctx, f.actor, f.project, "tasks", accepted.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Review(ctx, f.actor, f.project, origin.ID, f.body(map[string]any{"expected_version": origin.Version, "run_id": accepted.RunID, "decision": "rejected", "reason": "Withdraw original accepted ancestor"}), f.e); err != nil {
		t.Fatal(err)
	}
	input := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(a), InputKind: "development_continuation", BindingID: plan.ID, TransferID: source.TransferID}
	_, err = f.s.readCodeInputBundle(ctx, projectCandidateSubject(toolActorForTest(t, f, token), f.project), input)
	requireAuthorityStatus(t, err, 409)
	bounded, tx, cancel, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	defer tx.Rollback()
	if err = f.s.checkDevelopmentContinuationExecutionTx(bounded, tx, f.user.TenantID, f.project, run.ID, a.NodeID, false); err != nil {
		t.Fatal("drain facts were erased by ancestor withdrawal", err)
	}
}
