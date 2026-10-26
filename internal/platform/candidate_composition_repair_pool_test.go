package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Authenticated Control/Core/Pool HTTP transactions at a typed byte boundary.
// Native Git, inference and physical multi-host execution are separate evidence.
func TestCandidateConflictRepairPoolExactInputAndLateFacts(t *testing.T) {
	for _, completed := range []bool{false, true} {
		name := "known_failed"
		if completed {
			name = "completed_after_source_drift"
		}
		t.Run(name, func(t *testing.T) {
			h, original := compositionConflictFixture(t, true)
			f, ctx := h.f, context.Background()
			pool, grant := dispatchAuthorizedPool(t, f)
			dispatchSetRoute(t, f, 0, &grant)
			machine, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool.ID, ExecutionPoolRunnerCreateRequest{Name: "Explicit native conflict repair Pool worker", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			profile := toolTestProfile()
			profile.WorkspaceToolsVersion = nwc.Version
			profile.ConflictRepairProtocol = ccmp.CandidateConflictRepairProtocol
			f.bearer("POST", "/pool-runner/tool-inventory", machine.Token, PoolToolInventoryRequest{RunnerID: machine.Runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: profile, Available: true}}}, 200)
			pr, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: machine.Runner.WorkerID, Name: "Project-approved native Pool conflict repair", Profile: profile, IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			pr, err = f.s.CommandToolProfile(ctx, f.actor, f.project, pr.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: pr.Version, Action: "enable", IdempotencyKey: newID()})
			if err != nil || !pr.Available {
				t.Fatal(pr, err)
			}
			var conflict CandidateCompositionConflictContext
			if err = json.Unmarshal([]byte(stringify(f.call("GET", "/candidate-compositions/"+original.ID+"/conflict?project_id="+f.project, nil, 200))), &conflict); err != nil || conflict.Basis == nil || !conflict.CanCreate {
				t.Fatal(conflict, err)
			}
			request := CandidateCompositionRepairCreateRequest{ProjectID: f.project, ExpectedCompositionVersion: original.Version, ExpectedConflictSHA256: conflict.Basis.SHA256, Repair: CandidateCompositionRepairDraft{Title: "Resolve the fixed Pool source conflict", Description: "Resolve the original native Git conflict and preserve every frozen source.", Priority: "high", AcceptanceCriteria: []string{"Independent tests must cover the exact repaired candidate"}}, WorkflowID: h.req.WorkflowID, WorkflowVersion: h.req.WorkflowVersion, ProducerNodeID: h.req.ProducerNodeID, ToolProfileID: pr.ID, ExpectedToolVersion: pr.Version, IdempotencyKey: newID()}
			var plan CandidateCompositionPlan
			if err = json.Unmarshal([]byte(stringify(f.call("POST", "/candidate-compositions/"+original.ID+"/repairs", request, 201))), &plan); err != nil || plan.RunID != "" || plan.ConflictBasis == nil || plan.TaskID == original.TaskID {
				t.Fatal(plan, err)
			}
			if err = json.Unmarshal([]byte(stringify(f.call("POST", "/candidate-compositions/"+plan.ID+"/start", CandidateCompositionStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Acknowledge: true, IdempotencyKey: newID()}, 200))), &plan); err != nil || plan.RunID == "" {
				t.Fatal(plan, err)
			}
			claim := dag.PoolClaimRequest{PoolID: pool.ID, RunnerID: machine.Runner.ID, WorkerID: machine.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: []dag.PoolCapabilityScope{{TenantID: f.user.TenantID, ProjectID: f.project, Capabilities: []string{"command:" + profile.CommandID}}}}
			reply := f.bearer("POST", "/pool-runner/claim", machine.Token, claim, 200)
			var a dag.Assignment
			if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &a); err != nil || a.RunID != plan.RunID || a.NodeID != plan.ProducerNodeID || a.Type != "agent" || a.CandidateConflictRepair == nil || a.CandidateComposition != nil || a.CodeBaseline != nil || a.CandidateCommit != "" || a.RepositoryIntegration != nil || a.ReleaseExecution != nil {
				t.Fatal("Pool did not receive exactly one protected native repair input", reply, err)
			}
			if a.TenantID != f.user.TenantID || a.ProjectID != f.project || a.PoolID != pool.ID || a.PoolRunnerID != machine.Runner.ID || a.WorkerID != machine.Runner.WorkerID || a.PoolSlotID != "0" || a.PoolGrantID != grant.ID || a.PoolGrantVersion != grant.Version || a.PoolCredentialID == "" || a.PoolCredentialVersion != machine.Runner.CredentialVersion || a.DispatchID == "" || a.DispatchState != "active" || a.LeaseUntil == "" || a.Attempt != 1 || a.Fence < 1 {
				t.Fatal("Pool scope/lease was stripped or replaced", a)
			}
			e := a.CandidateConflictRepair
			if ccmp.ValidateCandidateConflictRepairExecution(*e) != nil || e.PlanID != plan.ID || e.PlanVersion != plan.Version || e.CompositionID != original.ID || e.OriginalAssignment != plan.ConflictBasis.Assignment || e.OriginalEvidenceSHA256 != plan.ConflictBasis.EvidenceSHA256 || e.Composition.SourcesSHA256 != plan.SourcesSHA256 || stringify(e.Composition.Sources) != stringify(compositionSources(plan)) || stringify(e.Conflict) != stringify(plan.ConflictBasis.Evidence) {
				t.Fatal("Pool repair envelope differs from reviewed plan", e)
			}
			input := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: repositoryTestRef(a), InputKind: "conflict_repair", BindingID: plan.ID, TransferID: plan.Sources[0].Source.TransferID}
			for _, source := range plan.Sources {
				input.TransferID = source.Source.TransferID
				if string(compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", input, 200)) != "candidate bytes" {
					t.Fatal("frozen source bytes changed")
				}
			}
			input.TransferID = plan.Sources[0].Source.TransferID
			for _, mutate := range []func(*ccmp.CodeInputRequest){func(r *ccmp.CodeInputRequest) { r.Assignment.TenantID = newID() }, func(r *ccmp.CodeInputRequest) { r.Assignment.Fence++ }, func(r *ccmp.CodeInputRequest) { r.Assignment.PoolGrantVersion++ }, func(r *ccmp.CodeInputRequest) { r.Assignment.PoolCredentialVersion++ }, func(r *ccmp.CodeInputRequest) { r.Assignment.DispatchID = newID() }} {
				bad := input
				mutate(&bad)
				compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", bad, 0)
			}
			bad := input
			bad.BindingID = original.ID
			compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", bad, 403)
			bad = input
			bad.TransferID = newID()
			compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", bad, 403)
			if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='withdrawn' WHERE id=?`, grant.ID); err != nil {
				t.Fatal(err)
			}
			compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", input, 0)
			if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET status='approved' WHERE id=?`, grant.ID); err != nil {
				t.Fatal(err)
			}
			if string(compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", input, 200)) != "candidate bytes" {
				t.Fatal("restored current grant changed fixed bytes")
			}
			result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: 1, Error: "Known failed repair at the typed Pool boundary", Output: "Fixed sources were read before their current scope changed", OperationID: newID(), ToolEvidence: &tc.Evidence{Profile: profile, Terminal: "failed", EventCount: 1, UsageState: tc.UsageUnreported}}
			var binding *cc.Binding
			if completed {
				manifest := cc.StartRequest{ProjectID: f.project, TenantID: f.user.TenantID, RunnerID: machine.Runner.ID, Assignment: repositoryTestRef(a), OperationID: newID(), BaseCommit: plan.AdminBaseCommit, HeadCommit: strings.Repeat("d", 40), EvidenceSHA256: strings.Repeat("f", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
				b := candidateBinding(t, f.bearer("POST", "/pool-runner/candidate-transfers", machine.Token, candidateJSON(t, manifest), 201))
				candidateBinary(t, f, "pool-runner", machine.Token, b.TransferID, newID(), manifest.Assignment, "PUT", []byte("candidate bytes"), 200)
				b = candidateBinding(t, f.bearer("POST", "/pool-runner/candidate-transfers/"+b.TransferID+"/seal", machine.Token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, TenantID: f.user.TenantID, RunnerID: machine.Runner.ID, Assignment: manifest.Assignment, OperationID: newID()}), 200))
				binding = &b
				proof := candidateSourceProof(manifest, b)
				parents := []string{plan.Sources[0].Source.HeadCommit, plan.Sources[1].Source.HeadCommit}
				proof["candidate_conflict_repair"] = ccmp.CandidateConflictRepairEvidence{Protocol: e.Protocol, PlanID: e.PlanID, PlanVersion: e.PlanVersion, CompositionID: e.CompositionID, OriginalAssignment: e.OriginalAssignment, OriginalEvidenceSHA256: e.OriginalEvidenceSHA256, SourcesSHA256: plan.SourcesSHA256, OperationID: e.OperationID, State: "completed", Steps: []ccmp.CandidateConflictRepairStep{{SourceIndex: 1, LeftCommit: parents[0], RightCommit: parents[1], SeedTree: e.Conflict.ConflictTree, ConflictSHA256: e.Conflict.ConflictSHA256, ResultCommit: manifest.HeadCommit, ResultTree: strings.Repeat("e", 40), Parents: parents}}, ResultCommit: manifest.HeadCommit, ResultTree: strings.Repeat("e", 40), Parents: parents, SourceHeads: parents, MergeCommits: []string{manifest.HeadCommit}}
				proof["native_workspace"] = nwc.NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: profile.ProfileSHA256, Generation: 1, State: "closed", Files: []nwc.NativeWorkspaceFileEffect{}, Checks: []nwc.NativeWorkspaceCheckResult{{OperationID: "pool-repair-check", CheckID: "unit", ProfileSHA256: strings.Repeat("a", 64), WorkspaceSHA256: strings.Repeat("b", 64), Generation: 1}}, Seal: &nwc.NativeWorkspaceSeal{SchemaVersion: 1, OperationID: e.OperationID, PolicySHA256: profile.ProfileSHA256, State: "no_change", PreviousCommit: manifest.HeadCommit, Commit: manifest.HeadCommit, Tree: strings.Repeat("e", 40), ChangedPaths: 0}}
				result.ExitCode = 0
				result.Error = ""
				result.WorkspaceEvidence = proof
				result.ToolEvidence.Terminal = "completed"
				if err = validateNativeWorkspaceEvidence(proof, result.ToolEvidence); err != nil {
					t.Fatal("actual-shaped Pool repair seal was rejected", err)
				}
			}
			_, err = f.s.Update(ctx, f.actor, f.project, "tasks", plan.Sources[0].TaskID, f.body(map[string]any{"expected_version": plan.Sources[0].TaskVersion, "description": "Source changed after the current Pool repair consumed its fixed bytes"}))
			if err != nil {
				t.Fatal(err)
			}
			compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", input, 409)
			f.bearer("POST", "/pool-runner/heartbeat", machine.Token, poolAssignmentBody(a), 200)
			body := poolAssignmentBody(a)
			body["result"] = result
			f.bearer("POST", "/pool-runner/results", machine.Token, body, 200)
			f.bearer("POST", "/pool-runner/results", machine.Token, body, 200)
			attempt, err := f.e.GetAttempt(ctx, f.user.TenantID, f.project, a.RunID, a.NodeID, a.Attempt)
			if err != nil || attempt.Result == nil || attempt.Result.Output != result.Output || attempt.Result.ExitCode != result.ExitCode || attempt.Result.Fence != a.Fence || attempt.Result.WorkerID != machine.Runner.WorkerID {
				t.Fatal("drift erased or replaced the original exact-attempt result", attempt, err)
			}
			var attempts int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=? AND node_id=?`, a.RunID, a.NodeID).Scan(&attempts); err != nil || attempts != 1 {
				t.Fatal(attempts, err)
			}
			compositionCodeInputHTTP(t, f, machine.Token, "pool-runner", input, 0)
			claim.OperationID = newID()
			claim.Scopes[0].Capabilities = []string{"command:check"}
			claimStatus := 200
			if completed {
				claimStatus = 409
			}
			if next := f.bearer("POST", "/pool-runner/claim", machine.Token, claim, claimStatus); next["assignment"] != nil {
				t.Fatal("source drift authorized new independent test work", next)
			}
			current, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, plan.ID)
			if err != nil || current.Valid || current.State == "accepted" || completed && (current.Candidate == nil || *current.Candidate != *binding) || !completed && current.Candidate != nil {
				t.Fatal(current, err)
			}
			orig, err := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, original.ID)
			if err != nil || orig.RunID != original.RunID || orig.State != "conflicted" || orig.Candidate != nil {
				t.Fatal(orig, err)
			}
		})
	}
}
