package platform

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	ec "github.com/ai-mada/suma/internal/executioncontract"
)

// This catches replacing the protected task, mutating the prior plan, or
// leaving a superseded preview able to start while its successor is ready.
func TestCandidateRepairRevisionReadyPreservesTaskAndOriginalPlan(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, _, _ := compositionRepairRequestFixture(t, h, original)
	parent, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	service, ok := any(f.s).(interface {
		GetCandidateCompositionRepairRevisionContext(context.Context, *dag.Engine, Actor, string, string) (CandidateCompositionRepairRevisionContext, error)
		ReviseCandidateCompositionRepair(context.Context, *dag.Engine, Actor, string, string, CandidateCompositionRepairRevisionRequest) (CandidateCompositionPlan, error)
	})
	if !ok {
		t.Fatal("reviewed repair revision service is unavailable")
	}
	var oldPayload string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_candidate_compositions WHERE id=?`, parent.ID).Scan(&oldPayload); err != nil {
		t.Fatal(err)
	}
	tasks, runs := expertCount(t, f, "suma_tasks"), expertCount(t, f, "dag_runs")
	view, err := service.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, parent.ID)
	if err != nil || !view.CanRevise || view.EligibilityMode != "ready" || view.ConflictBasis == nil || view.RecoveryBasis != nil {
		t.Fatal(view, err)
	}
	r := CandidateCompositionRepairRevisionRequest{ModelExecutionSelection: req.ModelExecutionSelection, ProjectID: f.project, ExpectedVersion: parent.Version, ExpectedTaskVersion: view.TaskVersion, ExpectedConflictSHA256: view.ConflictBasis.SHA256, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, ToolProfileID: req.ToolProfileID, ExpectedToolVersion: req.ExpectedToolVersion, Reason: "Explicitly review replacement execution inputs", Acknowledge: true, IdempotencyKey: newID()}
	child, err := service.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, parent.ID, r)
	if err != nil || child.ID == parent.ID || child.TaskID != parent.TaskID || child.RunID != "" || child.State != "ready" || child.RepairRevision == nil || child.RepairRevision.Ordinal != 2 || child.RepairRevision.ParentPlanID != parent.ID || !child.RepairRevision.IsCurrent {
		t.Fatal(child, err)
	}
	if output := os.Getenv("SUMA_REVISION_DTO_OUTPUT"); output != "" {
		if err = os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]any{"context": view, "parent": parent, "request": r, "plan": child} {
			raw, e := json.MarshalIndent(value, "", "  ")
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(output, name+".json"), raw, 0600); e != nil {
				t.Fatal(e)
			}
		}
	}
	var after string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_candidate_compositions WHERE id=?`, parent.ID).Scan(&after); err != nil || after != oldPayload || expertCount(t, f, "suma_tasks") != tasks || expertCount(t, f, "dag_runs") != runs {
		t.Fatal("revision changed original facts or created execution", err)
	}
	old, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, parent.ID)
	if err != nil || old.State != "ready" || old.RepairRevision == nil || old.RepairRevision.IsCurrent || old.RepairRevision.CurrentHeadPlanID != child.ID || len(old.AllowedActions) != 0 {
		t.Fatal(old, err)
	}
	_, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, parent.ID, CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: parent.Version, Acknowledge: true, IdempotencyKey: newID()})
	expertStatus(t, err, 409)
	_, err = f.s.CommandCandidateComposition(ctx, f.e, f.actor, f.project, parent.ID, CandidateCompositionCommandRequest{ProjectID: f.project, ExpectedVersion: parent.Version, Action: "dismiss", Reason: "Historical definitions must remain immutable", IdempotencyKey: newID()})
	expertStatus(t, err, 409)
	started := h.start(t, child)
	if started.RunID == "" || expertCount(t, f, "dag_runs") != runs+1 {
		t.Fatal(started)
	}
	replayed, err := service.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, parent.ID, r)
	if err != nil || replayed.ID != child.ID || replayed.RunID != "" || replayed.Version != child.Version {
		t.Fatal("revision replay lost exact prepared result", replayed, err)
	}
}

func repairRevisionFailedFixture(t *testing.T, certified bool) (*compositionFixture, CandidateCompositionPlan, CandidateCompositionRepairCreateRequest, ToolProfile, CandidateCompositionStartRequest, dag.Result) {
	t.Helper()
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, profile, _ := compositionRepairRequestFixture(t, h, original)
	p, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	start := CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: p.Version, Acknowledge: true, IdempotencyKey: newID()}
	p, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, p.ID, start)
	if err != nil {
		t.Fatal(err)
	}
	a, err := f.e.Claim(ctx, f.user.TenantID, f.project, "repair-worker", []string{"command:" + profile.Profile.CommandID})
	if err != nil || a == nil || a.RunID != p.RunID {
		t.Fatal(a, err)
	}
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: -1, Error: "Owned workspace root unavailable before preparation", OperationID: newID()}
	if certified {
		result.StartupFailure = &ec.StartupFailureProof{Protocol: ec.StartupFailureProtocol, Stage: "before_workspace", Assignment: cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}, Profile: profile.Profile, Purpose: "candidate_conflict_repair"}
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal(err)
	}
	return h, p, req, profile, start, result
}

// A current node with a different fence cannot be certified by an older exact
// failed result, even if its human-readable state still says failed.
func TestCandidateRepairRevisionRejectsChangedCurrentFailureFence(t *testing.T) {
	h, p, _, _, _, _ := repairRevisionFailedFixture(t, true)
	f, ctx := h.f, context.Background()
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET fence=fence+1 WHERE run_id=? AND id=?`, p.RunID, p.ProducerNodeID); err != nil {
		t.Fatal(err)
	}
	view, err := f.s.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || view.CanRevise || len(view.Blockers) == 0 {
		t.Fatal("changed current fence admitted old failure", view, err)
	}
}

func TestCandidateRepairRevisionCertifiedFailureNewRunAndOriginalReceipt(t *testing.T) {
	h, parent, req, profile, start, result := repairRevisionFailedFixture(t, true)
	f, ctx := h.f, context.Background()
	view, err := f.s.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, parent.ID)
	if err != nil || !view.CanRevise || view.EligibilityMode != "certified_startup_failure" || view.RecoveryBasis == nil || len(view.RecoveryBasis.Attempts) != 1 {
		t.Fatal(view, err)
	}
	// Re-review the existing exact configuration rather than bypassing the
	// registry's duplicate-profile guard with a new label or identity.
	profiles := expertCount(t, f, "suma_tool_profiles")
	paused, err := f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := f.s.CommandToolProfile(ctx, f.actor, f.project, paused.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: paused.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil || healthy.ID != profile.ID || healthy.Version <= profile.Version || expertCount(t, f, "suma_tool_profiles") != profiles {
		t.Fatal("explicit reapproval did not preserve the original exact profile", healthy, err)
	}
	r := CandidateCompositionRepairRevisionRequest{ModelExecutionSelection: req.ModelExecutionSelection, ProjectID: f.project, ExpectedVersion: parent.Version, ExpectedTaskVersion: view.TaskVersion, ExpectedConflictSHA256: view.ConflictBasis.SHA256, ExpectedRecoverySHA256: view.RecoveryBasis.SHA256, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, ToolProfileID: healthy.ID, ExpectedToolVersion: healthy.Version, Reason: "Use the explicitly reapproved current configuration after certified startup failure", Acknowledge: true, IdempotencyKey: newID()}
	tasks, runs := expertCount(t, f, "suma_tasks"), expertCount(t, f, "dag_runs")
	child, err := f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, parent.ID, r)
	if err != nil || child.TaskID != parent.TaskID || child.RunID != "" || child.ToolProfileID != healthy.ID {
		t.Fatal(child, err)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", parent.TaskID)
	if err != nil || task.RunID != parent.RunID || expertCount(t, f, "suma_tasks") != tasks || expertCount(t, f, "dag_runs") != runs {
		t.Fatal("revision replaced task binding before Start", task, err)
	}
	bounded, auth, close, err := f.s.planningAuthority(ctx, f.actor, f.project, parent.RunID, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.authorizeCandidateCompositionRunCommandTx(bounded, auth.tx, auth, f.project, parent.RunID, "retry")
	close()
	expertStatus(t, err, 409)
	started := h.start(t, child)
	if started.RunID == parent.RunID || started.TaskID != parent.TaskID {
		t.Fatal(started)
	}
	replay, err := f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, parent.ID, start)
	if err != nil || replay.RunID != parent.RunID || replay.ID != parent.ID || replay.Version != parent.Version {
		t.Fatal("old Start receipt became replacement", replay, err)
	}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, result); err != nil {
		t.Fatal("exact old result replay rejected", err)
	}
	bounded, tx, done, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	frozen, found, err := f.s.candidateCompositionExecutionRecordTx(bounded, tx, f.user.TenantID, f.project, parent.RunID)
	if err != nil || !found || frozen.Plan.ID != parent.ID || frozen.Plan.RunID != parent.RunID || !validCandidateCompositionExecutionRecord(frozen) {
		done()
		t.Fatal(frozen, err)
	}
	if err = f.s.validateCandidateCompositionRepairResultTx(bounded, tx, f.user.TenantID, f.project, result); err != nil {
		done()
		t.Fatal(err)
	}
	done()
}

func TestCandidateRepairRevisionMissingProofAndConcurrentCAS(t *testing.T) {
	t.Run("missing_proof", func(t *testing.T) {
		h, p, _, _, _, _ := repairRevisionFailedFixture(t, false)
		view, err := h.f.s.GetCandidateCompositionRepairRevisionContext(context.Background(), h.f.e, h.f.actor, h.f.project, p.ID)
		if err != nil || view.CanRevise || view.RecoveryBasis != nil || len(view.Blockers) == 0 {
			t.Fatal(view, err)
		}
	})
	t.Run("one_successor", func(t *testing.T) {
		h, original := compositionConflictFixture(t, true)
		f, ctx := h.f, context.Background()
		req, _, _ := compositionRepairRequestFixture(t, h, original)
		p, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
		if err != nil {
			t.Fatal(err)
		}
		view, err := f.s.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		r := CandidateCompositionRepairRevisionRequest{ModelExecutionSelection: req.ModelExecutionSelection, ProjectID: f.project, ExpectedVersion: p.Version, ExpectedTaskVersion: view.TaskVersion, ExpectedConflictSHA256: view.ConflictBasis.SHA256, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, ToolProfileID: req.ToolProfileID, ExpectedToolVersion: req.ExpectedToolVersion, Reason: "Concurrent reviewed revision", Acknowledge: true}
		var group sync.WaitGroup
		results := make(chan error, 2)
		for i := 0; i < 2; i++ {
			group.Add(1)
			go func() {
				defer group.Done()
				copy := r
				copy.IdempotencyKey = newID()
				_, e := f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, copy)
				results <- e
			}()
		}
		group.Wait()
		close(results)
		success, blocked := 0, 0
		for e := range results {
			if e == nil {
				success++
			} else {
				expertStatus(t, e, 409)
				blocked++
			}
		}
		if success != 1 || blocked != 1 || expertCount(t, f, "suma_candidate_composition_repair_revisions") != 2 {
			t.Fatal("concurrent revision forked family", success, blocked)
		}
	})
}

func TestCandidateRepairRevisionFreshConflictCASAndAtomicInvalidSelection(t *testing.T) {
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	req, _, _ := compositionRepairRequestFixture(t, h, original)
	p, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	old, err := f.s.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	// This changes freshness, while the original failed assignment/result and
	// immutable conflict identity remain the same reserved business issue.
	_, err = f.s.Update(ctx, f.actor, f.project, "tasks", original.TaskID, f.body(map[string]any{"expected_version": old.ConflictBasis.TaskVersion, "description": "Explicit updated original task context"}))
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.s.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || !view.CanRevise || view.ConflictBasis.SHA256 == old.ConflictBasis.SHA256 {
		t.Fatal(view, err)
	}
	r := CandidateCompositionRepairRevisionRequest{ModelExecutionSelection: req.ModelExecutionSelection, ProjectID: f.project, ExpectedVersion: p.Version, ExpectedTaskVersion: view.TaskVersion, ExpectedConflictSHA256: old.ConflictBasis.SHA256, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, ToolProfileID: req.ToolProfileID, ExpectedToolVersion: req.ExpectedToolVersion, Reason: "Review changed original task freshness", Acknowledge: true, IdempotencyKey: newID()}
	_, err = f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, r)
	expertStatus(t, err, 409)
	r.ExpectedConflictSHA256 = view.ConflictBasis.SHA256
	r.WorkflowID = newID()
	r.IdempotencyKey = newID()
	_, err = f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, r)
	expertStatus(t, err, 404)
	if expertCount(t, f, "suma_candidate_composition_repair_revisions") != 0 {
		t.Fatal("failed selection archived or created a partial revision")
	}
	r.WorkflowID = req.WorkflowID
	r.IdempotencyKey = newID()
	child, err := f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, r)
	if err != nil || child.TaskID != p.TaskID || child.ConflictBasis.SHA256 != view.ConflictBasis.SHA256 || child.SourcesSHA256 != p.SourcesSHA256 || stringify(child.Sources) != stringify(p.Sources) {
		t.Fatal(child, err)
	}
	// Re-reading the immutable original conflict discovers the new current
	// plan; another Create must not bypass its single protected repair task.
	conflict, err := f.s.GetCandidateCompositionConflict(ctx, f.actor, f.project, original.ID)
	if err != nil || conflict.RepairPlanID != child.ID || conflict.CanCreate {
		t.Fatal(conflict, err)
	}
}

func TestCandidateRepairRevisionStartRechecksPredecessorAndAllAttempts(t *testing.T) {
	t.Run("predecessor_version", func(t *testing.T) {
		h, p, req, _, _, _ := repairRevisionFailedFixture(t, true)
		f, ctx := h.f, context.Background()
		view, err := f.s.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		r := CandidateCompositionRepairRevisionRequest{ModelExecutionSelection: req.ModelExecutionSelection, ProjectID: f.project, ExpectedVersion: p.Version, ExpectedTaskVersion: view.TaskVersion, ExpectedConflictSHA256: view.ConflictBasis.SHA256, ExpectedRecoverySHA256: view.RecoveryBasis.SHA256, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, ProducerNodeID: req.ProducerNodeID, ToolProfileID: req.ToolProfileID, ExpectedToolVersion: req.ExpectedToolVersion, Reason: "Review safe replacement", Acknowledge: true, IdempotencyKey: newID()}
		child, err := f.s.ReviseCandidateCompositionRepair(ctx, f.e, f.actor, f.project, p.ID, r)
		if err != nil {
			t.Fatal(err)
		}
		before := expertCount(t, f, "dag_runs")
		if _, err = f.s.DB.Exec(`UPDATE dag_runs SET version=version+1 WHERE id=?`, p.RunID); err != nil {
			t.Fatal(err)
		}
		_, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, child.ID, CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: child.Version, Acknowledge: true, IdempotencyKey: newID()})
		expertStatus(t, err, 409)
		if expertCount(t, f, "dag_runs") != before {
			t.Fatal("stale predecessor created replacement run")
		}
	})
	t.Run("earlier_uncertified_attempt", func(t *testing.T) {
		h, p, _, _, _, result := repairRevisionFailedFixture(t, true)
		f, ctx := h.f, context.Background()
		second := result
		proof := *result.StartupFailure
		second.StartupFailure = &proof
		second.Attempt = 2
		second.Fence++
		second.OperationID = newID()
		second.StartupFailure.Assignment.Attempt = 2
		second.StartupFailure.Assignment.Fence = second.Fence
		if _, err := f.s.DB.Exec(`INSERT INTO dag_attempts(run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result) SELECT run_id,node_id,2,?,worker_id,state,started_at,completed_at,? FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=1`, second.Fence, stringify(second), p.RunID, p.ProducerNodeID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET attempt=2,fence=? WHERE run_id=? AND id=?`, second.Fence, p.RunID, p.ProducerNodeID); err != nil {
			t.Fatal(err)
		}
		result.StartupFailure = nil
		if _, err := f.s.DB.Exec(`UPDATE dag_attempts SET result=? WHERE run_id=? AND node_id=? AND attempt=1`, stringify(result), p.RunID, p.ProducerNodeID); err != nil {
			t.Fatal(err)
		}
		view, err := f.s.GetCandidateCompositionRepairRevisionContext(ctx, f.e, f.actor, f.project, p.ID)
		if err != nil || view.CanRevise || len(view.Blockers) == 0 {
			t.Fatal("latest certification hid earlier unknown execution", view, err)
		}
	})
}
