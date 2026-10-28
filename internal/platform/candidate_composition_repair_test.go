package platform

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/modelcontract"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func compositionConflictFixture(t *testing.T, rich bool) (*compositionFixture, CandidateCompositionPlan) {
	t.Helper()
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p := h.start(t, h.create(t))
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "conflict-worker", []string{"command:suma-compose"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	fact := map[string]any{"protocol": ccmp.CandidateCompositionProtocol, "plan_id": p.ID, "plan_version": p.Version, "sources_sha256": p.SourcesSHA256, "operation_id": p.OperationID, "base_commit": p.AdminBaseCommit, "state": "conflicted", "parents": []string{}, "source_heads": []string{p.Sources[0].Source.HeadCommit, p.Sources[1].Source.HeadCommit}, "merge_commits": []string{}, "conflicts": []string{"src/shared.go"}, "conflict_source_index": 1}
	if rich {
		fact["conflict_tree"] = strings.Repeat("e", 40)
		fact["conflict_stages"] = []map[string]any{{"path": "src/shared.go", "mode": "100644", "object": strings.Repeat("8", 40), "stage": 1}, {"path": "src/shared.go", "mode": "100644", "object": strings.Repeat("9", 40), "stage": 2}, {"path": "src/shared.go", "mode": "100644", "object": strings.Repeat("7", 40), "stage": 3}}
		var evidence ccmp.CandidateCompositionEvidence
		if err = json.Unmarshal([]byte(stringify(fact)), &evidence); err != nil {
			t.Fatal(err)
		}
		sha, e := ccmp.CandidateCompositionConflictDigest(evidence)
		if e != nil {
			t.Fatal(e)
		}
		fact["conflict_sha256"] = sha
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), ExitCode: 1, Error: "candidate composition has conflicts", WorkspaceEvidence: map[string]any{"candidate_composition": fact}}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || p.State != "conflicted" || p.Candidate != nil {
		t.Fatal(p, err)
	}
	return h, p
}

func compositionRepairRequestFixture(t *testing.T, h *compositionFixture, p CandidateCompositionPlan) (CandidateCompositionRepairCreateRequest, ToolProfile, string) {
	t.Helper()
	f, ctx := h.f, context.Background()
	view, err := f.s.GetCandidateCompositionConflict(ctx, f.actor, f.project, p.ID)
	if err != nil || !view.CanCreate || view.Basis == nil {
		t.Fatal(view, err)
	}
	profile := toolTestProfile()
	profile.WorkspaceToolsVersion = nwc.Version
	profile.ModelExecutionProtocol = "suma-model-execution-v1"
	profile.ConflictRepairProtocol = ccmp.CandidateConflictRepairProtocol
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "repair-worker", "profiles": []any{}, "deployment": false}), 201)
	token := worker["token"].(string)
	pr, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "repair-worker", Name: "Explicit native conflict repair", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: pr.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile, Available: true, ModelDeployments: []modelcontract.Deployment{nativeWorkspaceModelForTest(t).Deployment}}}})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, f.actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: pr.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return CandidateCompositionRepairCreateRequest{ModelExecutionSelection: nativeWorkspaceSelectionForTest(t), ProjectID: f.project, ExpectedCompositionVersion: p.Version, ExpectedConflictSHA256: view.Basis.SHA256, Repair: CandidateCompositionRepairDraft{Title: "Resolve the fixed shared source conflict", Description: "Preserve both reviewed source intentions and resolve the real Git conflict.", Priority: "high", AcceptanceCriteria: []string{"Both source contracts pass independent tests"}}, WorkflowID: h.req.WorkflowID, WorkflowVersion: h.req.WorkflowVersion, ProducerNodeID: h.req.ProducerNodeID, ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, IdempotencyKey: newID()}, pr, token
}

func TestCandidateConflictRepairRollbackConcurrentIdentityAndSourceDrift(t *testing.T) {
	h, p := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, _, _ := compositionRepairRequestFixture(t, h, p)
	before := expertCount(t, f, "suma_tasks")
	bad := req
	bad.WorkflowID = newID()
	bad.IdempotencyKey = newID()
	_, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, bad)
	expertStatus(t, err, 404)
	if err == nil || expertCount(t, f, "suma_tasks") != before || expertCount(t, f, "suma_candidate_composition_repair_sources") != 0 {
		t.Fatal("failed preview left a task or repair grant", err)
	}
	var group sync.WaitGroup
	out := make(chan CandidateCompositionPlan, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			copy := req
			copy.IdempotencyKey = newID()
			r, e := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, copy)
			out <- r
			errs <- e
		}()
	}
	group.Wait()
	close(out)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var repair CandidateCompositionPlan
	for r := range out {
		if repair.ID != "" && repair.ID != r.ID {
			t.Fatal("parallel requests created separate repair plans")
		}
		repair = r
	}
	if expertCount(t, f, "suma_tasks") != before+1 || expertCount(t, f, "suma_candidate_composition_repair_sources") != 1 {
		t.Fatal("parallel creation produced duplicate repair tasks")
	}
	_, err = f.s.Update(ctx, f.actor, f.project, "tasks", p.Sources[0].TaskID, f.body(map[string]any{"expected_version": p.Sources[0].TaskVersion, "description": "Explicit source scope changed"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, repair.ID, CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: repair.Version, Acknowledge: true, IdempotencyKey: newID()})
	expertStatus(t, err, 409)
	if expertCount(t, f, "suma_candidate_composition_executions") != 1 {
		t.Fatal("blocked start left another execution")
	}
	view, err := f.s.GetCandidateCompositionConflict(ctx, f.actor, f.project, p.ID)
	if err != nil || view.CanCreate || view.RepairPlanID != repair.ID || len(view.Blockers) == 0 {
		t.Fatal(view, err)
	}
}

func compositionRepairCompleteFixture(t *testing.T, h *compositionFixture, p CandidateCompositionPlan, profile ToolProfile, token string, afterSeal func()) (CandidateCompositionPlan, cc.Binding) {
	t.Helper()
	f, ctx := h.f, context.Background()
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "repair-worker", []string{"command:" + profile.Profile.CommandID})
	if err != nil || a == nil || a.RunID != p.RunID {
		t.Fatal(a, err)
	}
	ref := cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	actor := domainCActor(t, f.s, token)
	read := func(req ccmp.CodeInputRequest) error {
		bounded, tx, close, e := f.s.beginAuthorityTx(ctx)
		if e != nil {
			return e
		}
		defer close()
		defer tx.Rollback()
		authority, e := f.s.candidateAuthorityTx(bounded, tx, projectCandidateSubject(actor, f.project), req.Assignment)
		if e != nil {
			return e
		}
		_, e = f.s.codeInputBindingTx(bounded, tx, authority, req)
		return e
	}
	input := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: ref, InputKind: "conflict_repair", BindingID: p.ID, TransferID: p.Sources[0].Source.TransferID}
	if err = read(input); err != nil {
		t.Fatal(err)
	}
	wrong := input
	wrong.BindingID = p.ConflictBasis.CompositionID
	expertStatus(t, read(wrong), 403)
	wrong = input
	wrong.TransferID = newID()
	expertStatus(t, read(wrong), 403)
	wrong = input
	wrong.Assignment.Fence++
	expertStatus(t, read(wrong), 409)
	manifest := cc.StartRequest{ProjectID: f.project, Assignment: ref, OperationID: newID(), BaseCommit: p.AdminBaseCommit, HeadCommit: strings.Repeat("d", 40), EvidenceSHA256: strings.Repeat("f", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
	b := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, manifest), 201))
	candidateBinary(t, f, "runner", token, b.TransferID, newID(), ref, "PUT", []byte("candidate bytes"), 200)
	b = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+b.TransferID+"/seal", token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: ref, OperationID: newID()}), 200))
	proof := candidateSourceProof(manifest, b)
	proof["candidate_conflict_repair"] = ccmp.CandidateConflictRepairEvidence{Protocol: ccmp.CandidateConflictRepairProtocol, PlanID: p.ID, PlanVersion: p.Version, CompositionID: p.ConflictBasis.CompositionID, OriginalAssignment: p.ConflictBasis.Assignment, OriginalEvidenceSHA256: p.ConflictBasis.EvidenceSHA256, SourcesSHA256: p.SourcesSHA256, OperationID: p.OperationID, State: "completed", Steps: []ccmp.CandidateConflictRepairStep{{SourceIndex: 1, LeftCommit: p.Sources[0].Source.HeadCommit, RightCommit: p.Sources[1].Source.HeadCommit, SeedTree: p.ConflictBasis.Evidence.ConflictTree, ConflictSHA256: p.ConflictBasis.Evidence.ConflictSHA256, ResultCommit: manifest.HeadCommit, ResultTree: strings.Repeat("e", 40), Parents: []string{p.Sources[0].Source.HeadCommit, p.Sources[1].Source.HeadCommit}}}, ResultCommit: manifest.HeadCommit, ResultTree: strings.Repeat("e", 40), Parents: []string{p.Sources[0].Source.HeadCommit, p.Sources[1].Source.HeadCommit}, SourceHeads: []string{p.Sources[0].Source.HeadCommit, p.Sources[1].Source.HeadCommit}, MergeCommits: []string{manifest.HeadCommit}}
	_, tool := nativeEvidenceFixture()
	tool.Profile = profile.Profile
	tool.Terminal = "completed"
	tool.EventCount = 1
	tool.UsageState = toolcontract.UsageUnreported
	if p.ModelExecution == nil {
		t.Fatal("native repair fixture lost its model binding")
	}
	tool.ModelExecution = nativeGatewayReceiptForTest(t, *p.ModelExecution)
	proof["native_workspace"] = nwc.NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: profile.Profile.ProfileSHA256, Generation: 1, State: "closed", Files: []nwc.NativeWorkspaceFileEffect{}, Checks: []nwc.NativeWorkspaceCheckResult{{OperationID: "repair-check", CheckID: "unit", ProfileSHA256: strings.Repeat("a", 64), WorkspaceSHA256: strings.Repeat("b", 64), Generation: 1}}, Seal: &nwc.NativeWorkspaceSeal{SchemaVersion: 1, OperationID: p.OperationID, PolicySHA256: profile.Profile.ProfileSHA256, State: "no_change", PreviousCommit: manifest.HeadCommit, Commit: manifest.HeadCommit, Tree: strings.Repeat("e", 40), ChangedPaths: 0}}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof, ToolEvidence: tool}
	if afterSeal != nil {
		afterSeal()
		expertStatus(t, read(input), 409)
	}
	bounded, tx, close, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.validateCandidateCompositionRepairResultTx(bounded, tx, f.user.TenantID, f.project, result); err != nil {
		close()
		t.Fatal(err)
	}
	bad := result
	bad.WorkspaceEvidence = candidateJSON(t, proof)
	bad.WorkspaceEvidence["candidate_conflict_repair"].(map[string]any)["sources_sha256"] = strings.Repeat("0", 64)
	expertStatus(t, f.s.validateCandidateCompositionRepairResultTx(bounded, tx, f.user.TenantID, f.project, bad), 409)
	close()
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal(err)
	}
	q, err := f.e.Claim(ctx, f.user.TenantID, f.project, "repair-independent-test-worker", []string{"command:check"})
	if err != nil || q == nil || q.CandidateCommit != manifest.HeadCommit {
		t.Fatal(q, err)
	}
	quality := map[string]any{"head_commit": manifest.HeadCommit, "base_commit": manifest.HeadCommit, "evidence_sha256": strings.Repeat("8", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "test_command": true, "candidate_from": p.ProducerNodeID, "candidate_attempt": a.Attempt, "candidate_fence": a.Fence, "candidate_evidence_sha256": manifest.EvidenceSHA256}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: q.RunID, NodeID: q.NodeID, Attempt: q.Attempt, Fence: q.Fence, WorkerID: q.WorkerID, OperationID: newID(), WorkspaceEvidence: quality}); err != nil {
		t.Fatal(err)
	}
	p, err = f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || p.State != "awaiting_review" || p.Candidate == nil || *p.Candidate != b {
		t.Fatal(p, err)
	}
	return p, b
}

func TestCandidateConflictRepairExactCandidateReviewAndBaseline(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, profile, token := compositionRepairRequestFixture(t, h, original)
	p, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	p = h.start(t, p)
	p, b := compositionRepairCompleteFixture(t, h, p, profile, token, nil)
	child, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Downstream from the independently accepted repair"}))
	if err != nil {
		t.Fatal(err)
	}
	bind := TaskCodeBaselineBindRequest{ProjectID: f.project, ExpectedTaskVersion: child.Version, ExpectedBindingVersion: 0, CompositionID: p.ID, ExpectedCompositionVersion: p.Version, Acknowledge: true, Reason: "Use only this repaired and reviewed candidate", IdempotencyKey: newID()}
	_, err = f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, child.ID, bind)
	if err == nil {
		t.Fatal("test success granted acceptance")
	}
	p = compositionAcceptFixedCandidate(t, h, p)
	bound, err := f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, child.ID, bind)
	if err != nil || bound.CodeBaseline == nil || bound.CodeBaseline.Source != b || bound.CodeBaseline.CompositionID != p.ID {
		t.Fatal(bound, err)
	}
	orig, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, original.ID)
	if err != nil || orig.State != "conflicted" || orig.Candidate != nil {
		t.Fatal(orig, err)
	}
	bind.CompositionID = original.ID
	bind.ExpectedCompositionVersion = original.Version
	bind.ExpectedTaskVersion = bound.Version
	bind.ExpectedBindingVersion = 1
	bind.IdempotencyKey = newID()
	_, err = f.s.BindTaskCodeBaseline(ctx, f.e, f.actor, f.project, child.ID, bind)
	if err == nil {
		t.Fatal("original failed composition became accepted baseline")
	}
}

func TestCandidateConflictRepairLateCompletedFactsBlockNewApprovalAfterDrift(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, profile, token := compositionRepairRequestFixture(t, h, original)
	p, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	p = h.start(t, p)
	p, _ = compositionRepairCompleteFixture(t, h, p, profile, token, func() {
		_, e := f.s.Update(ctx, f.actor, f.project, "tasks", p.Sources[0].TaskID, f.body(map[string]any{"expected_version": p.Sources[0].TaskVersion, "description": "New current source scope after candidate bytes sealed"}))
		if e != nil {
			t.Fatal(e)
		}
	})
	if p.Valid {
		t.Fatal("source drift was not shown")
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", p.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.Review(ctx, f.actor, f.project, p.TaskID, f.body(map[string]any{"expected_version": task.Version, "run_id": p.RunID, "decision": "accepted", "reason": "Attempt to accept stale source proof"}), f.e)
	expertStatus(t, err, 409)
	current, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || current.Candidate == nil || current.RunState != "succeeded" || current.State == "accepted" {
		t.Fatal(current, err)
	}
}

func TestCandidateConflictRepairOriginalCredentialReplayAndFreshAccess(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, _, _ := compositionRepairRequestFixture(t, h, original)
	p, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	start := CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: p.Version, Acknowledge: true, IdempotencyKey: newID()}
	p, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
	current, err := f.s.AuthenticateActor(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := f.s.StartCandidateComposition(ctx, f.e, current, f.project, p.ID, start)
	if err != nil || replayed.RunID != p.RunID || expertCount(t, f, "suma_candidate_composition_executions") != 2 {
		t.Fatal(replayed, err)
	}
	bounded, tx, close, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	expertStatus(t, f.s.checkCandidateCompositionRepairExecutionTx(bounded, tx, f.user.TenantID, f.project, p.RunID, p.ProducerNodeID, true), 409)
	_, err = f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err == nil {
		t.Fatal("revoked browser credential recovered a mutation")
	}
}

func TestCandidateConflictRepairExplicitRefreshExpiredPreviewAndOriginalReceipt(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, _, _ := compositionRepairRequestFixture(t, h, original)
	p, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	p.ExpiresAt = "2000-01-01T00:00:00Z"
	bounded, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, p.ID, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.s.candidateCompositionTx(bounded, auth.tx, f.user.TenantID, f.project, p.ID)
	if err != nil {
		close()
		t.Fatal(err)
	}
	r.Plan.ExpiresAt = p.ExpiresAt
	if err = f.s.saveCandidateCompositionTx(bounded, auth, p.Version, r); err == nil {
		err = auth.tx.Commit()
	}
	close()
	if err != nil {
		t.Fatal(err)
	}
	expired, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || expired.Valid || !skillHasAction(expired.AllowedActions, "refresh") || skillHasAction(expired.AllowedActions, "start") {
		t.Fatal(expired, err)
	}
	_, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: p.Version, Acknowledge: true, IdempotencyKey: newID()})
	expertStatus(t, err, 409)
	refresh := CandidateCompositionCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "refresh", Reason: "Explicitly recheck and renew this same frozen repair preview", IdempotencyKey: newID()}
	renewed, err := f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, refresh)
	if err != nil || renewed.Version != p.Version+1 || renewed.RunID != "" || renewed.TaskID != p.TaskID || renewed.TaskVersion != p.TaskVersion || renewed.FixedTimestamp != p.FixedTimestamp || stringify(renewed.Workflow) != stringify(p.Workflow) || renewed.ExpiresAt <= p.ExpiresAt {
		t.Fatal(renewed, err)
	}
	stale := refresh
	stale.IdempotencyKey = newID()
	_, err = f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, stale)
	expertStatus(t, err, 409)
	started := h.start(t, renewed)
	if started.RunID == "" || started.Version != renewed.Version+1 {
		t.Fatal(started)
	}
	replayed, err := f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, refresh)
	if err != nil || replayed.Version != renewed.Version || replayed.RunID != "" || expertCount(t, f, "suma_candidate_composition_repair_sources") != 1 {
		t.Fatal(replayed, err)
	}
	newkey := refresh
	newkey.ExpectedVersion = started.Version
	newkey.IdempotencyKey = newID()
	_, err = f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, newkey)
	expertStatus(t, err, 409)
	_, err = f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, original.ID, CandidateCompositionCommandRequest{ProjectID: f.project, ExpectedVersion: original.Version, Action: "refresh", Reason: "Not a repair preview", IdempotencyKey: newID()})
	expertStatus(t, err, 409)
	t.Run("changed_source_cannot_refresh", func(t *testing.T) {
		h, original := compositionConflictFixture(t, true)
		f, ctx := h.f, context.Background()
		req, _, _ := compositionRepairRequestFixture(t, h, original)
		p, e := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.s.Update(ctx, f.actor, f.project, "tasks", p.Sources[0].TaskID, f.body(map[string]any{"expected_version": p.Sources[0].TaskVersion, "description": "New source contract must not be adopted by refresh"}))
		if e != nil {
			t.Fatal(e)
		}
		_, e = f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, CandidateCompositionCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "refresh", Reason: "Attempt to refresh stale source", IdempotencyKey: newID()})
		expertStatus(t, e, 409)
		current, e := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
		if e != nil || current.Version != p.Version || current.ExpiresAt != p.ExpiresAt || skillHasAction(current.AllowedActions, "refresh") {
			t.Fatal(current, e)
		}
	})
}

func TestCandidateConflictRepairRequiresRichExactFailure(t *testing.T) {
	h, p := compositionConflictFixture(t, false)
	f, ctx := h.f, context.Background()
	before := expertCount(t, f, "suma_tasks")
	view, err := f.s.GetCandidateCompositionConflict(ctx, f.actor, f.project, p.ID)
	if err != nil || view.CanCreate || view.Basis != nil || len(view.Blockers) == 0 {
		t.Fatal(view, err)
	}
	if expertCount(t, f, "suma_tasks") != before {
		t.Fatal("read created repair task")
	}
}

func TestCandidateConflictRepairAtomicDraftAndReservedTask(t *testing.T) {
	h, p := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	view, err := f.s.GetCandidateCompositionConflict(ctx, f.actor, f.project, p.ID)
	if err != nil || !view.CanCreate || view.Basis == nil {
		t.Fatal(view, err)
	}
	profile := toolTestProfile()
	profile.WorkspaceToolsVersion = "attempt-workspace-v1"
	profile.ModelExecutionProtocol = "suma-model-execution-v1"
	profile.ConflictRepairProtocol = ccmp.CandidateConflictRepairProtocol
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "repair-worker", "profiles": []any{}, "deployment": false}), 201)
	pr, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "repair-worker", Name: "Explicit native conflict repair", Profile: profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, worker["token"].(string)), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: pr.WorkerID, OperationID: newID(), Items: []toolcontract.Observation{{Profile: profile, Available: true, ModelDeployments: []modelcontract.Deployment{nativeWorkspaceModelForTest(t).Deployment}}}})
	if err != nil {
		t.Fatal(err)
	}
	pr, err = f.s.CommandToolProfile(ctx, f.actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: pr.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	req := CandidateCompositionRepairCreateRequest{ModelExecutionSelection: nativeWorkspaceSelectionForTest(t), ProjectID: f.project, ExpectedCompositionVersion: p.Version, ExpectedConflictSHA256: view.Basis.SHA256, Repair: CandidateCompositionRepairDraft{Title: "Resolve the fixed shared source conflict", Description: "Preserve both reviewed source intentions and resolve the real Git conflict.", Priority: "high", AcceptanceCriteria: []string{"Both source contracts pass independent tests"}}, WorkflowID: h.req.WorkflowID, WorkflowVersion: h.req.WorkflowVersion, ProducerNodeID: h.req.ProducerNodeID, ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, IdempotencyKey: newID()}
	before := expertCount(t, f, "suma_tasks")
	repair, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, req)
	if err != nil || repair.TaskID == p.TaskID || repair.RunID != "" || repair.State != "ready" || repair.ConflictBasis == nil || repair.ConflictBasis.SHA256 != view.Basis.SHA256 {
		t.Fatal(repair, err)
	}
	if expertCount(t, f, "suma_tasks") != before+1 {
		t.Fatal("repair creation did not create exactly one task")
	}
	req.IdempotencyKey = newID()
	again, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, req)
	if err != nil || again.ID != repair.ID || again.TaskID != repair.TaskID || expertCount(t, f, "suma_tasks") != before+1 {
		t.Fatal(again, err)
	}
	_, err = f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: repair.TaskID, ExpectedTaskVersion: repair.TaskVersion, WorkflowID: h.req.WorkflowID, WorkflowVersion: h.req.WorkflowVersion, AgentNodeID: h.req.ProducerNodeID, ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, IdempotencyKey: newID()})
	expertStatus(t, err, 409)
	err = f.s.AttachRun(ctx, f.actor, f.project, repair.TaskID, p.RunID)
	expertStatus(t, err, 409)
	original, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || original.State != "conflicted" || original.RunID != p.RunID || original.Version != p.Version || original.Candidate != nil {
		t.Fatal(original, err)
	}
	started := h.start(t, repair)
	if started.RunID == "" || started.RunID == p.RunID {
		t.Fatal(started)
	}
}

func TestCandidateConflictRepairOptionalPriorityDefaultsBeforeReceipt(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, _, _ := compositionRepairRequestFixture(t, h, original)
	tasks, plans, runs := expertCount(t, f, "suma_tasks"), expertCount(t, f, "suma_candidate_compositions"), expertCount(t, f, "dag_runs")
	req.Repair.Priority = "urgent"
	_, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	expertStatus(t, err, 400)
	if expertCount(t, f, "suma_tasks") != tasks || expertCount(t, f, "suma_candidate_compositions") != plans || expertCount(t, f, "dag_runs") != runs || expertCount(t, f, "suma_candidate_composition_repair_sources") != 0 {
		t.Fatal("unsupported priority left a task, plan, run or repair link")
	}
	req.Repair.Priority = ""
	child, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal("omitted priority must create the default-priority repair", err)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", child.TaskID)
	if err != nil || task.Priority != "medium" || task.RunID != "" || child.State != "ready" || child.RunID != "" {
		t.Fatal(task, child, err)
	}
	// Equivalent optional and explicit defaults share one normalized receipt.
	req.Repair.Priority = "medium"
	replay, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil || replay.ID != child.ID || replay.TaskID != child.TaskID || expertCount(t, f, "suma_tasks") != tasks+1 || expertCount(t, f, "suma_candidate_compositions") != plans+1 || expertCount(t, f, "dag_runs") != runs || expertCount(t, f, "suma_candidate_composition_repair_sources") != 1 {
		t.Fatal("normalized default did not replay the same atomic creation", replay, err)
	}
}

func TestCandidateConflictRepairIdentitySurvivesOriginalTaskVersionChange(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, _, _ := compositionRepairRequestFixture(t, h, original)
	child, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	before := expertCount(t, f, "suma_tasks")
	// An opaque lookup key is not the source identity. Preserve the already
	// stored child/basis even when this row uses the whole fresh-preview digest.
	if _, err = f.s.DB.Exec(`UPDATE suma_candidate_composition_repair_sources SET conflict_sha256=? WHERE tenant_id=? AND project_id=? AND repair_plan_id=?`, req.ExpectedConflictSHA256, f.user.TenantID, f.project, child.ID); err != nil {
		t.Fatal(err)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", original.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.Update(ctx, f.actor, f.project, "tasks", task.ID, f.body(map[string]any{"expected_version": task.Version, "description": "Explicit metadata correction to the original failed task"}))
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.s.GetCandidateCompositionConflict(ctx, f.actor, f.project, original.ID)
	if err != nil || view.Basis == nil || view.Basis.SHA256 == req.ExpectedConflictSHA256 || view.Basis.ResultSHA256 != child.ConflictBasis.ResultSHA256 || view.RepairPlanID != child.ID || view.CanCreate {
		t.Fatal("same exact conflict lost its original repair link", view, err)
	}
	stale := req
	stale.IdempotencyKey = newID()
	_, err = f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, stale)
	expertStatus(t, err, 409)
	req.IdempotencyKey = newID()
	req.ExpectedConflictSHA256 = view.Basis.SHA256
	req.Repair.Title = "Must recover the existing child without overwriting it"
	recovered, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil || recovered.ID != child.ID || recovered.TaskID != child.TaskID || recovered.Context.Task.Title != child.Context.Task.Title || recovered.Valid || expertCount(t, f, "suma_tasks") != before || expertCount(t, f, "suma_candidate_composition_repair_sources") != 1 {
		t.Fatal(recovered, err)
	}
	_, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, recovered.ID, CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: recovered.Version, Acknowledge: true, IdempotencyKey: newID()})
	expertStatus(t, err, 409)
}
