package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// These are real Control/Core, sealed-byte transfer, separate Worker and
// formal fixture Review facts. They do not execute a vendor or prove a Git
// bundle's contents; the separate Runner/runtime journey owns that boundary.
type acceptedTaskBaselineFixture struct {
	f          *fixture
	token      string
	request    TaskDispatchPreviewRequest
	source     Record
	candidates map[string]cc.Binding
}

func newAcceptedTaskBaselineFixture(t *testing.T, multiple bool) *acceptedTaskBaselineFixture {
	t.Helper()
	f, token, raw, _ := taskDispatchFixture(t)
	ctx := context.Background()
	var request TaskDispatchPreviewRequest
	if err := json.Unmarshal([]byte(stringify(raw)), &request); err != nil {
		t.Fatal(err)
	}
	if multiple {
		workflow, _, _ := taskDispatchBuildFixture()
		workflow.ID, workflow.Version = "", 0
		workflow.Nodes = append(workflow.Nodes,
			dag.Node{ID: "develop-two", Type: "agent", Command: []string{"native-agent"}, Config: map[string]any{"goal": "Produce the explicitly separate candidate", "tool_profile_id": request.ToolProfileID, "tool_profile_sha256": toolTestProfile().ProfileSHA256}},
			dag.Node{ID: "test-two", Type: "test", Command: []string{"independent-tests"}, DependsOn: []string{"develop-two"}, Config: map[string]any{"candidate_from": "develop-two"}},
		)
		published, err := f.e.Publish(ctx, f.user.TenantID, f.project, workflow)
		if err != nil {
			t.Fatal(err)
		}
		request.WorkflowID, request.WorkflowVersion = published.ID, published.Version
	}
	request.IdempotencyKey = newID()
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h := &acceptedTaskBaselineFixture{f: f, token: token, request: request, candidates: map[string]cc.Binding{}}
	count := 1
	if multiple {
		count = 2
	}
	for i := 0; i < count; i++ {
		response := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": "tool-worker", "capabilities": []string{"command:native-agent"}, "idempotency_key": newID()}), 200)
		var a dag.Assignment
		if json.Unmarshal([]byte(stringify(response["assignment"])), &a) != nil || a.RunID != run.ID || a.Type != "agent" {
			t.Fatal("ordinary developer assignment missing", response)
		}
		manifest := cc.StartRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(a), OperationID: newID(), BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat(string(rune('b'+i)), 40), EvidenceSHA256: strings.Repeat(string(rune('e'+i)), 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
		candidate := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, manifest), 201))
		candidateBinary(t, f, "runner", token, candidate.TransferID, newID(), manifest.Assignment, "PUT", []byte("candidate bytes"), 200)
		candidate = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+candidate.TransferID+"/seal", token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: manifest.Assignment, OperationID: newID()}), 200))
		result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: candidateSourceProof(manifest, candidate), ToolEvidence: &tc.Evidence{Profile: toolTestProfile(), Terminal: "completed", EventCount: 1, UsageState: tc.UsageUnreported}}
		f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
		h.candidates[a.NodeID] = candidate
	}
	qualityToken := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
	for i := 0; i < count; i++ {
		a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "accepted-source-quality-worker", []string{"command:independent-tests"})
		if err != nil || a == nil || a.RunID != run.ID || a.WorkerID == "tool-worker" || a.CandidateFrom == "" {
			t.Fatal("separate candidate test assignment missing", a, err)
		}
		proof := map[string]any{"head_commit": a.CandidateCommit, "base_commit": a.CandidateCommit, "evidence_sha256": strings.Repeat("9", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "test_command": true, "candidate_from": a.CandidateFrom, "candidate_attempt": a.CandidateAttempt, "candidate_fence": a.CandidateFence, "candidate_evidence_sha256": a.CandidateEvidenceSHA256}
		f.bearer("POST", "/runner/results", qualityToken, f.body(map[string]any{"result": dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}}), 200)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", request.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	h.source, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": run.ID, "decision": "accepted", "reason": "Fixture reviewer accepts exact candidates after separate Worker tests"}), f.e)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *acceptedTaskBaselineFixture) target(t *testing.T) Record {
	t.Helper()
	task, err := h.f.s.Create(context.Background(), h.f.actor, h.f.project, "tasks", h.f.body(map[string]any{"title": "Explicit follow-up of accepted ordinary code"}))
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func (h *acceptedTaskBaselineFixture) bindingRequest(target Record, node string) TaskCodeBaselineBindRequest {
	return TaskCodeBaselineBindRequest{ProjectID: h.f.project, ExpectedTaskVersion: target.Version, SourceTaskID: h.source.ID, ExpectedSourceTaskVersion: h.source.Version, SourceNodeID: node, Acknowledge: true, Reason: "Adopt this exact formally accepted ordinary agent candidate", IdempotencyKey: newID()}
}

func TestAcceptedTaskBaselineExactProducerOptionsAndOriginalReceipt(t *testing.T) {
	h := newAcceptedTaskBaselineFixture(t, true)
	f, ctx, target := h.f, context.Background(), h.target(t)
	options, err := f.s.CandidateCompositionOptions(ctx, f.e, f.actor, f.project, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, choice := range options.Sources {
		if choice.TaskID != h.source.ID {
			continue
		}
		if !choice.Eligible || !choice.CanBindBaseline || choice.TaskVersion != h.source.Version || choice.Source != h.candidates[choice.SourceNodeID] {
			t.Fatal("options lost exact eligible producer", choice)
		}
		seen[choice.SourceNodeID] = true
	}
	if len(seen) != 2 {
		t.Fatal("multiple producers were guessed or collapsed", seen)
	}
	request := h.bindingRequest(target, "develop-two")
	bound, err := f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, request)
	if err != nil || bound.CodeBaseline == nil || bound.CodeBaseline.SourceTaskID != h.source.ID || bound.CodeBaseline.SourceTaskVersion != h.source.Version || bound.CodeBaseline.Source != h.candidates["develop-two"] || bound.Version != target.Version+1 {
		t.Fatal("explicit selected producer was not bound", bound, err)
	}
	if bound.CodeBaseline.CompositionID != "" || bound.CodeBaseline.TerminalCandidatePlanID != "" || ccmp.ValidateCodeBaselineBinding(*bound.CodeBaseline) != nil {
		t.Fatal("ordinary source impersonated another origin", bound.CodeBaseline)
	}
	view, err := f.s.GetTaskCodeBaseline(ctx, f.actor, f.project, target.ID)
	if err != nil || !view.CanBind || view.Version != 1 || view.Binding == nil || *view.Binding != *bound.CodeBaseline {
		t.Fatal(view, err)
	}
	options, err = f.s.CandidateCompositionOptions(ctx, f.e, f.actor, f.project, target.ID)
	if err != nil || options.CanCreate {
		t.Fatal("existing baseline must still forbid composition creation", options, err)
	}
	for _, choice := range options.Sources {
		if choice.TaskID == h.source.ID && !choice.CanBindBaseline {
			t.Fatal("composition-create state suppressed legitimate baseline replacement", choice)
		}
	}
	// A lost response recovers the same completed write, not a fresh adoption
	// against changed source evidence or a new task/binding version.
	_, err = f.s.Update(ctx, f.actor, f.project, "tasks", h.source.ID, f.body(map[string]any{"expected_version": h.source.Version, "description": "Explicit source change after the first binding"}))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, request)
	if err != nil || stringify(recovered) != stringify(bound) {
		t.Fatal("original operation was not recovered", recovered, err)
	}
	request.ExpectedTaskVersion, request.ExpectedBindingVersion, request.IdempotencyKey = bound.Version, 1, newID()
	if _, err = f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, request); err == nil {
		t.Fatal("new adoption accepted stale source")
	}
}

func TestAcceptedTaskBaselineRejectsInvalidSourceAndDoubleCAS(t *testing.T) {
	h := newAcceptedTaskBaselineFixture(t, false)
	f, ctx, target := h.f, context.Background(), h.target(t)
	request := h.bindingRequest(target, "develop")
	for _, mutate := range []func(*TaskCodeBaselineBindRequest){
		func(r *TaskCodeBaselineBindRequest) { r.SourceTaskID = "" },
		func(r *TaskCodeBaselineBindRequest) { r.ExpectedSourceTaskVersion = 0 },
		func(r *TaskCodeBaselineBindRequest) { r.SourceNodeID = "" },
		func(r *TaskCodeBaselineBindRequest) {
			r.CompositionID, r.ExpectedCompositionVersion = "invented-composition", 1
		},
		func(r *TaskCodeBaselineBindRequest) {
			r.TerminalCandidatePlanID, r.ExpectedTerminalCandidatePlanVersion = "invented-terminal", 1
		},
	} {
		invalid := request
		mutate(&invalid)
		_, err := f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, invalid)
		expertStatus(t, err, 400)
	}
	for _, mutate := range []func(*TaskCodeBaselineBindRequest){
		func(r *TaskCodeBaselineBindRequest) { r.ExpectedSourceTaskVersion-- },
		func(r *TaskCodeBaselineBindRequest) { r.SourceNodeID = "missing-producer" },
		func(r *TaskCodeBaselineBindRequest) { r.ExpectedTaskVersion++ },
		func(r *TaskCodeBaselineBindRequest) { r.ExpectedBindingVersion = 1 },
	} {
		changed := request
		changed.IdempotencyKey = newID()
		mutate(&changed)
		_, err := f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, changed)
		expertStatus(t, err, 409)
	}
	other := f.call("POST", "/projects", map[string]any{"title": "Unrelated source project", "idempotency_key": newID()}, 201)
	foreign, err := f.s.Create(ctx, f.actor, other["id"].(string), "tasks", map[string]any{"title": "Not a source of this project", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	request.SourceTaskID, request.ExpectedSourceTaskVersion = foreign.ID, foreign.Version
	if _, err = f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, request); err == nil {
		t.Fatal("cross-project source was accepted")
	}
	current, err := f.s.GetTaskCodeBaseline(ctx, f.actor, f.project, target.ID)
	if err != nil || current.Binding != nil || current.Version != 0 {
		t.Fatal("rejected source changed the baseline", current, err)
	}
}

func TestAcceptedTaskBaselineDoesNotReclassifyCommandCandidate(t *testing.T) {
	f, token, manifest := candidateTransferFixture(t)
	source := compositionAcceptSource(t, f, token, manifest)
	ctx := context.Background()
	target, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Do not disguise a command as an ordinary agent"}))
	if err != nil {
		t.Fatal(err)
	}
	options, err := f.s.CandidateCompositionOptions(ctx, f.e, f.actor, f.project, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, choice := range options.Sources {
		if choice.TaskID == source.ID {
			found = true
			if !choice.Eligible || choice.CanBindBaseline {
				t.Fatal("command composition eligibility became ordinary-agent authority", choice)
			}
		}
	}
	if !found {
		t.Fatal("existing composition source disappeared")
	}
	_, err = f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, TaskCodeBaselineBindRequest{ProjectID: f.project, ExpectedTaskVersion: target.Version, SourceTaskID: source.ID, ExpectedSourceTaskVersion: source.Version, SourceNodeID: "code", Acknowledge: true, Reason: "This must remain a distinct command source", IdempotencyKey: newID()})
	expertStatus(t, err, 409)
}

func (h *acceptedTaskBaselineFixture) startDownstream(t *testing.T, node string) (Record, TaskDispatchPlan, dag.Run) {
	t.Helper()
	f, ctx, target := h.f, context.Background(), h.target(t)
	target, err := f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, target.ID, h.bindingRequest(target, node))
	if err != nil {
		t.Fatal(err)
	}
	request := h.request
	request.TaskID, request.ExpectedTaskVersion, request.IdempotencyKey = target.ID, target.Version, newID()
	plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return target, plan, run
}

func TestAcceptedTaskBaselineOriginalDispatchAndFreshInputAuthority(t *testing.T) {
	h := newAcceptedTaskBaselineFixture(t, false)
	f, ctx := h.f, context.Background()
	target, plan, run := h.startDownstream(t, "develop")
	if plan.CodeBaseline == nil || *plan.CodeBaseline != *target.CodeBaseline || plan.CodeBaseline.Source != h.candidates["develop"] {
		t.Fatal("dispatch lost the ordinary baseline", plan)
	}
	reply := f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": "tool-worker", "capabilities": []string{"command:native-agent"}, "idempotency_key": newID()}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID != run.ID || a.CodeBaseline == nil || *a.CodeBaseline != *target.CodeBaseline {
		t.Fatal(reply)
	}
	request := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(a), InputKind: "code_baseline", BindingID: target.CodeBaseline.ID, TransferID: target.CodeBaseline.Source.TransferID}
	runnerActor := toolActorForTest(t, f, h.token)
	readInput := func() error {
		bounded, tx, close, err := f.s.beginAuthorityTx(ctx)
		if err != nil {
			return err
		}
		defer close()
		defer tx.Rollback()
		authority, err := f.s.candidateAuthorityTx(bounded, tx, projectCandidateSubject(runnerActor, f.project), request.Assignment)
		if err != nil {
			return err
		}
		input, err := f.s.codeInputBindingTx(bounded, tx, authority, request)
		if err == nil && input.binding != h.candidates["develop"] {
			t.Fatal("broker changed selected source", input.binding)
		}
		return err
	}
	if err := readInput(); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.Review(ctx, f.actor, f.project, h.source.ID, f.body(map[string]any{"expected_version": h.source.Version, "run_id": h.source.RunID, "decision": "rejected", "reason": "Current reviewer returns this formerly accepted source"}), f.e)
	if err != nil {
		t.Fatal(err)
	}
	if err = readInput(); err == nil {
		t.Fatal("new bundle access survived source Review rejection")
	}
	expertStatus(t, compositionCheckBaselineExecution(t, f, run.ID, "develop", true), 409)
	if err = compositionCheckBaselineExecution(t, f, run.ID, "develop", false); err != nil {
		t.Fatal("source change erased occurred execution authority", err)
	}
}

func TestAcceptedTaskBaselineAcceptanceMemoSeparatesSiblingProducers(t *testing.T) {
	h := newAcceptedTaskBaselineFixture(t, true)
	f, ctx := h.f, context.Background()
	_, _, first := h.startDownstream(t, "develop")
	_, _, second := h.startDownstream(t, "develop-two")
	bounded, tx, close, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	defer tx.Rollback()
	bounded, walk := compositionAcceptanceContext(bounded)
	for _, run := range []dag.Run{first, second} {
		if err = f.s.checkCodeBaselineAcceptanceTx(bounded, tx, Principal{TenantID: f.user.TenantID}, f.project, run.ID, walk); err != nil {
			t.Fatal("one producer's memo entry contaminated its sibling", run.ID, err)
		}
	}
	if len(walk.bases) != 2 {
		t.Fatal("sibling sources shared one acceptance memo key", len(walk.bases))
	}
}
