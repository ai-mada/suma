package dag

import (
	"errors"
	"strings"
	"testing"
)

func candidateEvidence(commit string, test bool) map[string]any {
	return map[string]any{"head_commit": strings.Repeat(commit, 40), "base_commit": strings.Repeat(commit, 40), "state": "completed", "clean": true, "test_command": test, "exit_code": 0, "outcome_unknown": false, "evidence_sha256": strings.Repeat("e", 64)}
}
func TestCandidateReferenceRequiresExplicitStaticAncestor(t *testing.T) {
	for _, scenario := range []string{"missing", "not-ancestor", "wrong-type", "spoof"} {
		t.Run(scenario, func(t *testing.T) {
			w := Workflow{Nodes: []Node{{ID: "code", Type: "command"}, {ID: "test", Type: "test", DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}}
			switch scenario {
			case "missing":
				w.Nodes[1].Config["candidate_from"] = "missing"
			case "not-ancestor":
				w.Nodes[1].DependsOn = nil
			case "wrong-type":
				w.Nodes[1].Type = "noop"
			case "spoof":
				w.Nodes[1].Config["candidate_commit"] = strings.Repeat("a", 40)
			}
			if Compile(w) == nil {
				t.Fatal("invalid candidate source accepted")
			}
		})
	}
}
func TestCandidateClaimFreezesProofAndRejectsDifferentTestRevision(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "code", Type: "command"}, {ID: "test", Type: "test", DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
	a := claim(t, e)
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "candidate", WorkspaceEvidence: candidateEvidence("c", false)}); err != nil {
		t.Fatal(err)
	}
	test := claim(t, e)
	if test == nil || test.CandidateCommit != strings.Repeat("c", 40) || test.CandidateFrom != "code" || test.CandidateAttempt != a.Attempt || test.CandidateFence != a.Fence || test.CandidateEvidenceSHA256 != strings.Repeat("e", 64) {
		t.Fatalf("candidate not frozen: %+v", test)
	}
	var persisted string
	if err := e.db.QueryRow("SELECT commit_id FROM dag_candidate_revisions WHERE run_id=? AND node_id=? AND attempt=?", r.ID, test.NodeID, test.Attempt).Scan(&persisted); err != nil || persisted != test.CandidateCommit {
		t.Fatalf("pin not durable %s %v", persisted, err)
	}
	result := Result{RunID: test.RunID, NodeID: test.NodeID, Attempt: test.Attempt, Fence: test.Fence, WorkerID: test.WorkerID, OperationID: "test-wrong", WorkspaceEvidence: candidateEvidence("b", true)}
	if err := e.Complete(ctx, "t", "p", result); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong test source accepted: %v", err)
	}
	result.OperationID = "test-right"
	result.WorkspaceEvidence = candidateEvidence("c", true)
	result.WorkspaceEvidence["candidate_from"] = test.CandidateFrom
	result.WorkspaceEvidence["candidate_attempt"] = test.CandidateAttempt
	result.WorkspaceEvidence["candidate_fence"] = test.CandidateFence
	result.WorkspaceEvidence["candidate_evidence_sha256"] = test.CandidateEvidenceSHA256
	if err := e.Complete(ctx, "t", "p", result); err != nil {
		t.Fatal(err)
	}
	if err := e.Complete(ctx, "t", "p", result); err != nil {
		t.Fatal("ack replay", err)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.State != "succeeded" {
		t.Fatalf("%+v %v", current, err)
	}
}

func TestCandidateCannotSkipProofUsingStoppedOrWrongProvenance(t *testing.T) {
	for _, scenario := range []string{"stopped", "provenance"} {
		t.Run(scenario, func(t *testing.T) {
			e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "code", Type: "command"}, {ID: "test", Type: "test", DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
			a := claim(t, e)
			if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "code", WorkspaceEvidence: candidateEvidence("c", false)}); err != nil {
				t.Fatal(err)
			}
			a = claim(t, e)
			proof := candidateEvidence("c", true)
			proof["candidate_from"] = "wrong"
			proof["candidate_attempt"] = 99
			proof["candidate_fence"] = 99
			proof["candidate_evidence_sha256"] = strings.Repeat("x", 64)
			result := Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: scenario, WorkspaceEvidence: proof, Stopped: scenario == "stopped"}
			if err := e.Complete(ctx, "t", "p", result); !errors.Is(err, ErrInvalid) {
				t.Fatalf("bypassed candidate proof: %v", err)
			}
			result.ExitCode = 1
			result.Error = "candidate preparation failed"
			result.WorkspaceEvidence = nil
			if err := e.Complete(ctx, "t", "p", result); err != nil {
				t.Fatal("known failure must be durable", err)
			}
		})
	}
}

func TestCandidateWaitsForSourceDespiteEarlyAnySuccessJoin(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "code", Type: "command"}, {ID: "early", Type: "noop"}, {ID: "test", Type: "test", Join: "any_success", DependsOn: []string{"code", "early"}, Config: map[string]any{"candidate_from": "code"}}}})
	a := claim(t, e)
	if a == nil || a.NodeID != "code" {
		t.Fatalf("%+v", a)
	}
	if next := claim(t, e); next != nil {
		t.Fatal("candidate source still running")
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.State != "running" {
		t.Fatalf("premature failure %+v %v", current, err)
	}
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "code", WorkspaceEvidence: candidateEvidence("c", false)}); err != nil {
		t.Fatal(err)
	}
	if next := claim(t, e); next == nil || next.CandidateCommit != strings.Repeat("c", 40) {
		t.Fatalf("no frozen candidate %+v", next)
	}
}
func TestCandidateMissingCleanProofFailsWithoutDispatch(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "code", Type: "command"}, {ID: "test", Type: "test", DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
	done(t, e, claim(t, e), 0)
	if a := claim(t, e); a != nil {
		t.Fatal("dispatched unproved source")
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.State != "failed" {
		t.Fatalf("candidate missing proof did not fail visibly: %+v %v", current, err)
	}
}
