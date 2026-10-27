package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
	ec "github.com/ai-mada/suma/internal/executioncontract"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func startupFailureFixture(t *testing.T, pooled bool) (*fixture, CandidateCompositionPlan, ToolProfile, dag.Result) {
	t.Helper()
	h, original := compositionConflictFixture(t, true)
	f, ctx := h.f, context.Background()
	var req CandidateCompositionRepairCreateRequest
	var profile ToolProfile
	var machine ExecutionPoolCredentialResponse
	if !pooled {
		req, profile, _ = compositionRepairRequestFixture(t, h, original)
	} else {
		pool, grant := dispatchAuthorizedPool(t, f)
		dispatchSetRoute(t, f, 0, &grant)
		registered, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "Startup proof Pool", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		machine = registered
		fixed := toolTestProfile()
		fixed.WorkspaceToolsVersion, fixed.ConflictRepairProtocol = nwc.Version, ccc.CandidateConflictRepairProtocol
		f.bearer("POST", "/pool-runner/tool-inventory", machine.Token, PoolToolInventoryRequest{RunnerID: machine.Runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: fixed, Available: true}}}, 200)
		profile, err = f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: machine.Runner.WorkerID, Name: "Frozen Pool startup proof", Profile: fixed, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		view, err := f.s.GetCandidateCompositionConflict(ctx, f.actor, f.project, original.ID)
		if err != nil {
			t.Fatal(err)
		}
		req = CandidateCompositionRepairCreateRequest{ProjectID: f.project, ExpectedCompositionVersion: original.Version, ExpectedConflictSHA256: view.Basis.SHA256, Repair: CandidateCompositionRepairDraft{Title: "Recover the same repair task", Description: "Preserve the original conflict and exact sources.", AcceptanceCriteria: []string{"Independent exact candidate tests"}}, WorkflowID: h.req.WorkflowID, WorkflowVersion: h.req.WorkflowVersion, ProducerNodeID: h.req.ProducerNodeID, ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, IdempotencyKey: newID()}
	}
	f.e.AttemptAccounting = f.s
	plan, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = f.s.StartCandidateComposition(ctx, f.e, f.actor, f.project, plan.ID, CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Acknowledge: true, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	var assignment dag.Assignment
	if pooled {
		request := dag.PoolClaimRequest{PoolID: machine.Runner.PoolID, RunnerID: machine.Runner.ID, WorkerID: machine.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.Profile.CommandID}}}}
		reply := f.bearer("POST", "/pool-runner/claim", machine.Token, request, 200)
		if json.Unmarshal([]byte(stringify(reply["assignment"])), &assignment) != nil || assignment.RunID != plan.RunID {
			t.Fatal(reply)
		}
	} else {
		a, err := f.e.Claim(ctx, f.user.TenantID, f.project, profile.WorkerID, []string{"command:" + profile.Profile.CommandID})
		if err != nil || a == nil || a.RunID != plan.RunID {
			t.Fatal(a, err)
		}
		assignment = *a
	}
	proof := &ec.StartupFailureProof{Protocol: ec.StartupFailureProtocol, Stage: "before_workspace", Assignment: repositoryTestRef(assignment), Profile: profile.Profile, Purpose: "candidate_conflict_repair"}
	result := dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID(), ExitCode: -1, Error: "Literal diagnostic is not the proof", StartupFailure: proof}
	return f, plan, profile, result
}

func certifyStartupForTest(t *testing.T, f *fixture, result dag.Result) (bool, error) {
	t.Helper()
	tx, err := f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	return f.s.CertifiedCandidateRepairStartupFailureTx(context.Background(), tx, f.user.TenantID, f.project, result)
}

func TestStartupFailureFrozenPrivatePoolProofAndContradictions(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		t.Run(map[bool]string{false: "private", true: "Pool"}[pooled], func(t *testing.T) {
			f, _, profile, result := startupFailureFixture(t, pooled)
			if ok, err := certifyStartupForTest(t, f, result); err != nil || !ok {
				t.Fatal(ok, err)
			}
			if _, err := f.s.CommandToolProfile(context.Background(), f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()}); err != nil {
				t.Fatal(err)
			}
			if ok, err := certifyStartupForTest(t, f, result); err != nil || !ok {
				t.Fatal("profile pause erased the fixed host fact", ok, err)
			}
			for name, mutate := range map[string]func(*dag.Result){
				"success": func(r *dag.Result) { r.ExitCode = 0 }, "unknown": func(r *dag.Result) { r.OutcomeUnknown = true }, "stop": func(r *dag.Result) { r.Stopped = true },
				"workspace": func(r *dag.Result) { r.WorkspaceEvidence = map[string]any{} }, "tool": func(r *dag.Result) { r.ToolEvidence = &tc.Evidence{Profile: profile.Profile} },
				"assignment": func(r *dag.Result) { r.StartupFailure.Assignment.Fence++ }, "profile": func(r *dag.Result) { r.StartupFailure.Profile.ExecutableSHA256 = "wrong" },
				"process": func(r *dag.Result) { r.StartupFailure.ProcessStarted = true }, "workspace_entered": func(r *dag.Result) { r.StartupFailure.WorkspaceStarted = true },
				"pool_scope": func(r *dag.Result) { r.StartupFailure.Assignment.PoolCredentialVersion++ },
			} {
				t.Run(name, func(t *testing.T) {
					var next dag.Result
					_ = json.Unmarshal([]byte(stringify(result)), &next)
					mutate(&next)
					if ok, err := certifyStartupForTest(t, f, next); err == nil || ok {
						t.Fatal("contradictory startup proof accepted", ok, err)
					}
				})
			}
		})
	}
}

func TestStartupFailureKnownZeroSettlesOnceWithoutInventingPrice(t *testing.T) {
	f, plan, _, result := startupFailureFixture(t, false)
	complete := func() error {
		return f.e.CompleteBound(context.Background(), f.user.TenantID, f.project, result, func(*sql.Tx) error { return nil }, func(tx *sql.Tx, _ string) error {
			return f.s.validateStartupFailureResultTx(context.Background(), tx, f.user.TenantID, f.project, result)
		})
	}
	if err := complete(); err != nil {
		t.Fatal(err)
	}
	var first string
	if err := f.s.DB.QueryRow(`SELECT payload FROM suma_usage_attempts WHERE run_id=?`, plan.RunID).Scan(&first); err != nil {
		t.Fatal(err)
	}
	var record usageRecord
	if json.Unmarshal([]byte(first), &record) != nil || record.Unresolved || record.Attempt.UsageSource != "host_startup_proof" || record.Attempt.State != "unpriced" || record.Attempt.ReportedTokens == nil || *record.Attempt.ReportedTokens != 0 || record.Attempt.EstimatedCostMicros != nil || record.Attempt.Rate != nil || record.Attempt.HeldTokens != 0 {
		t.Fatal("certified zero/unpriced settlement mismatch", first)
	}
	if err := complete(); err != nil {
		t.Fatal("original callback replay", err)
	}
	var after string
	if err := f.s.DB.QueryRow(`SELECT payload FROM suma_usage_attempts WHERE run_id=?`, plan.RunID).Scan(&after); err != nil || after != first {
		t.Fatal("same result settled twice", err)
	}
	if ok, err := certifyStartupForTest(t, f, result); err != nil || !ok {
		t.Fatal("stored result proof became invalid after zero settlement", ok, err)
	}
}

func TestStartupFailureCannotEraseEarlierUsageOrRefundOldUncertifiedResult(t *testing.T) {
	for _, oldResult := range []bool{false, true} {
		t.Run(map[bool]string{false: "previous-counter", true: "older-uncertified-result"}[oldResult], func(t *testing.T) {
			f, plan, _, result := startupFailureFixture(t, false)
			var raw string
			if err := f.s.DB.QueryRow(`SELECT payload FROM suma_usage_attempts WHERE run_id=?`, plan.RunID).Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var record usageRecord
			if err := json.Unmarshal([]byte(raw), &record); err != nil {
				t.Fatal(err)
			}
			if oldResult {
				record.Attempt.SourceResultSHA256 = digest("original evidence-free failure")
			} else {
				record.Attempt.Usage = &tc.Usage{InputTokens: usageInt(3)}
				record.Attempt.UsageSource = "tool_evidence"
			}
			if _, err := f.s.DB.Exec(`UPDATE suma_usage_attempts SET payload=? WHERE run_id=?`, stringify(record), plan.RunID); err != nil {
				t.Fatal(err)
			}
			if ok, err := certifyStartupForTest(t, f, result); err == nil || ok {
				t.Fatal("new startup assertion erased an earlier fact", ok, err)
			}
		})
	}
}

func TestStartupFailureRejectsRecordedToolExecutionBeforeFinalReceipt(t *testing.T) {
	f, _, profile, result := startupFailureFixture(t, false)
	event := &tc.Event{SchemaVersion: 1, CommandID: profile.Profile.CommandID, ProfileSHA256: profile.Profile.ProfileSHA256, Protocol: profile.Profile.Protocol, Kind: "session_started"}
	assignment := dag.Assignment{RunID: result.RunID, NodeID: result.NodeID, Attempt: result.Attempt, Fence: result.Fence, WorkerID: result.WorkerID}
	if err := f.e.Progress(context.Background(), f.user.TenantID, f.project, assignment, newID(), "", event); err != nil {
		t.Fatal(err)
	}
	if ok, err := certifyStartupForTest(t, f, result); err == nil || ok {
		t.Fatal("recorded tool start was erased by a later startup assertion", ok, err)
	}
}
