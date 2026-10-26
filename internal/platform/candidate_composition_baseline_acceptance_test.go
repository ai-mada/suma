package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// A downstream code-producing task is not itself a composition. Its actual
// baseline must still participate in new approval and delivery qualification.
func TestCandidateCompositionBaselineAcceptanceTraversesOriginalSources(t *testing.T) {
	for _, acceptedBeforeDrift := range []bool{false, true} {
		name := "before_review"
		if acceptedBeforeDrift {
			name = "already_accepted"
		}
		t.Run(name, func(t *testing.T) {
			h := newCompositionFixture(t)
			f, ctx := h.f, context.Background()
			m, _ := compositionCompleteFixedCandidate(t, h, h.start(t, h.create(t)))
			m = compositionAcceptFixedCandidate(t, h, m)
			profile, token, workflow := compositionDispatchProfile(t, f)
			task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Develop C from accepted composition M"}))
			if err != nil {
				t.Fatal(err)
			}
			task = compositionBindBaseline(t, h, m, task, 0)
			plan, err := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: workflow.ID, WorkflowVersion: workflow.Version, AgentNodeID: "develop", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			run, err := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
			if err != nil {
				t.Fatal(err)
			}
			claimed := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": toolFixtureCapabilities(t, f), "idempotency_key": newID()}), 200)
			var a dag.Assignment
			if json.Unmarshal([]byte(stringify(claimed["assignment"])), &a) != nil || a.RunID != run.ID || a.CodeBaseline == nil || *a.CodeBaseline != *task.CodeBaseline {
				t.Fatal(claimed)
			}
			manifest := cc.StartRequest{ProjectID: f.project, Assignment: codeInputAssignmentRef(a), OperationID: newID(), BaseCommit: task.CodeBaseline.Source.HeadCommit, BundleBaseCommit: task.CodeBaseline.Source.BundleBase(), HeadCommit: strings.Repeat("6", 40), EvidenceSHA256: strings.Repeat("7", 64), BundleSHA256: digest("candidate bytes"), SizeBytes: int64(len("candidate bytes"))}
			candidate := candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers", token, candidateJSON(t, manifest), 201))
			candidateBinary(t, f, "runner", token, candidate.TransferID, newID(), manifest.Assignment, "PUT", []byte("candidate bytes"), 200)
			candidate = candidateBinding(t, f.bearer("POST", "/runner/candidate-transfers/"+candidate.TransferID+"/seal", token, candidateJSON(t, cc.CommandRequest{ProjectID: f.project, Assignment: manifest.Assignment, OperationID: newID()}), 200))
			proof := candidateSourceProof(manifest, candidate)
			proof["admin_base_commit"] = manifest.BundleBase()
			proof["code_baseline"] = task.CodeBaseline
			result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof, ToolEvidence: &tc.Evidence{Profile: profile.Profile, Terminal: "completed", EventCount: 1, UsageState: tc.UsageUnreported}}
			f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
			checked := f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": "independent-C-check", "capabilities": []string{"command:independent-tests"}, "idempotency_key": newID()}), 200)
			var test dag.Assignment
			if json.Unmarshal([]byte(stringify(checked["assignment"])), &test) != nil || test.RunID != run.ID || test.CandidateCommit != candidate.HeadCommit || test.CandidateFrom != "develop" {
				t.Fatal(checked)
			}
			testProof := map[string]any{"head_commit": test.CandidateCommit, "base_commit": test.CandidateCommit, "evidence_sha256": strings.Repeat("8", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0, "test_command": true, "candidate_from": test.CandidateFrom, "candidate_attempt": test.CandidateAttempt, "candidate_fence": test.CandidateFence, "candidate_evidence_sha256": test.CandidateEvidenceSHA256}
			f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": dag.Result{RunID: test.RunID, NodeID: test.NodeID, Attempt: test.Attempt, Fence: test.Fence, WorkerID: test.WorkerID, OperationID: newID(), WorkspaceEvidence: testProof}}), 200)
			before, err := f.e.GetAttempt(ctx, f.user.TenantID, f.project, a.RunID, a.NodeID, a.Attempt)
			if err != nil || before.Result == nil {
				t.Fatal(before, err)
			}
			factSHA := digest(stringify(before.Result))
			// Approval uses this current browser, not the original execution
			// creator credential, which has now actually been logged out.
			f.call("POST", "/auth/logout", map[string]any{}, 200)
			f.call("POST", "/auth/login", map[string]any{"email": "owner@example.test", "password": "test-password-strong-42"}, 200)
			f.actor, err = f.s.AuthenticateActor(ctx, f.cookie.Value)
			if err != nil {
				t.Fatal(err)
			}
			currentTask, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
			if err != nil {
				t.Fatal(err)
			}
			basis := func() error {
				bounded, auth, close, e := f.s.planningAuthority(ctx, f.actor, f.project, task.ID, purposeProjectRead)
				if e != nil {
					return e
				}
				defer close()
				_, e = f.s.repositoryBasisReadTx(bounded, auth.tx, auth.currentPrincipal, f.project, task.ID, "develop")
				return e
			}
			if acceptedBeforeDrift {
				currentTask, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": currentTask.Version, "run_id": run.ID, "decision": "accepted", "reason": "Current reviewer accepts the independently tested downstream C"}), f.e)
				if err != nil {
					t.Fatal("revoked old creator erased occurred execution", err)
				}
				if err = basis(); err != nil {
					t.Fatal("current accepted baseline was incorrectly rejected", err)
				}
			}
			source := h.req.Sources[0]
			_, err = f.s.Update(ctx, f.actor, f.project, "tasks", source.TaskID, f.body(map[string]any{"expected_version": source.ExpectedTaskVersion, "description": "Fixed source A changed after C completed using composition M"}))
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": currentTask.Version, "run_id": run.ID, "decision": "accepted", "reason": "New approval must traverse C baseline M and reject stale A"}), f.e)
			expertStatus(t, err, 409)
			expertStatus(t, basis(), 409)
			after, err := f.e.GetAttempt(ctx, f.user.TenantID, f.project, a.RunID, a.NodeID, a.Attempt)
			if err != nil || after.Result == nil || digest(stringify(after.Result)) != factSHA {
				t.Fatal("source drift rewrote C execution facts", after, err)
			}
			finished, err := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
			if err != nil || finished.State != "succeeded" {
				t.Fatal(finished, err)
			}
		})
	}
}
