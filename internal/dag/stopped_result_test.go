package dag

import (
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/toolcontract"
)

// Regresses a confirmed stop being classified as success when the OS exits
// zero, including stops before any protocol evidence could be produced.
func TestConfirmedStoppedZeroExitNeverSucceeds(t *testing.T) {
	for _, evidenceKind := range []string{"generic", "none", "failed", "completed", "unknown"} {
		for _, scenario := range []struct {
			name     string
			cancel   bool
			unknown  bool
			wantNode string
			wantRun  string
		}{
			{"active_confirmed", false, false, "failed", "failed"},
			{"cancelled_confirmed", true, false, "cancelled", "cancelled"},
			{"active_unknown_effect", false, true, "reconciling", "running"},
			{"cancelling_unknown_effect", true, true, "reconciling", "cancelling"},
		} {
			t.Run(evidenceKind+"/"+scenario.name, func(t *testing.T) {
				n := toolNode()
				if evidenceKind == "generic" {
					n = Node{ID: "agent", Type: "command", Command: []string{"native-agent"}}
				}
				e, r := fixture(t, Workflow{Nodes: []Node{n, {ID: "after", Type: "command", Command: []string{"after"}, DependsOn: []string{n.ID}}}})
				a, err := e.Claim(ctx, "t", "p", "w", []string{"command:native-agent", "command:after"})
				if err != nil || a == nil || a.NodeID != n.ID {
					t.Fatal("missing active attempt", a, err)
				}
				if scenario.cancel {
					r, err = e.GetRun(ctx, "t", "p", r.ID)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = e.Command(ctx, "t", "p", r.ID, "cancel", "cancel-before-stop", r.Version, map[string]any{"reason": "confirmed stop regression"}); err != nil {
						t.Fatal(err)
					}
				}
				result := Result{RunID: r.ID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "zero-exit-confirmed-stop", ExitCode: 0, Stopped: true, OutcomeUnknown: scenario.unknown}
				if evidenceKind != "none" && evidenceKind != "generic" {
					result.ToolEvidence = &toolcontract.Evidence{UsageState: "unreported", Profile: toolcontract.Profile{CommandID: "native-agent", Adapter: "suma", Protocol: "suma-jsonl-v1", ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture"}, Terminal: evidenceKind, EventCount: 1}
				}
				if err = e.Complete(ctx, "t", "p", result); err != nil {
					t.Fatal("legitimate process stop rejected", err)
				}
				current, err := e.GetRun(ctx, "t", "p", r.ID)
				if err != nil || current.State != scenario.wantRun {
					t.Fatalf("confirmed stop run=%s want=%s err=%v", current.State, scenario.wantRun, err)
				}
				for _, node := range current.Nodes {
					if node.ID == n.ID {
						if node.State != scenario.wantNode || node.Attempt != 1 {
							t.Fatalf("stop node=%+v want=%s", node, scenario.wantNode)
						}
						if result.ToolEvidence == nil && node.ToolEvidence != nil || result.ToolEvidence != nil && (node.ToolEvidence == nil || node.ToolEvidence.Terminal != evidenceKind) {
							t.Fatal("stop fabricated or discarded protocol evidence", node.ToolEvidence)
						}
					}
					if node.ID == "after" && node.Attempt != 0 {
						t.Fatal("stopped source dispatched its dependent", node)
					}
				}
				if next, err := e.Claim(ctx, "t", "p", "w", []string{"command:after"}); err != nil || next != nil {
					t.Fatal("stopped source activated its dependent", next, err)
				}
				// A lost acknowledgement must recover this same stopped outcome.
				if err = e.Complete(ctx, "t", "p", result); err != nil {
					t.Fatal("exact stopped receipt replay failed", err)
				}
				replayed, err := e.GetRun(ctx, "t", "p", r.ID)
				if err != nil || replayed.Version != current.Version || replayed.State != current.State {
					t.Fatal("stopped receipt replay mutated run", replayed, err)
				}
			})
		}
	}
}

func TestPinnedCandidateStoppedZeroExitCannotPassTest(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "code", Type: "command", Command: []string{"test-command"}}, {ID: "test", Type: "test", Command: []string{"test-command"}, DependsOn: []string{"code"}, Config: map[string]any{"candidate_from": "code"}}}})
	a := claim(t, e)
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "code-candidate", WorkspaceEvidence: candidateEvidence("c", false)}); err != nil {
		t.Fatal(err)
	}
	a = claim(t, e)
	if a == nil || a.NodeID != "test" || a.CandidateCommit != strings.Repeat("c", 40) {
		t.Fatal("test did not receive frozen candidate", a)
	}
	proof := candidateEvidence("c", true)
	proof["candidate_from"], proof["candidate_attempt"], proof["candidate_fence"], proof["candidate_evidence_sha256"] = a.CandidateFrom, a.CandidateAttempt, a.CandidateFence, a.CandidateEvidenceSHA256
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "stopped-candidate-test", ExitCode: 0, Stopped: true, WorkspaceEvidence: proof}); err != nil {
		t.Fatal("confirmed test stop rejected", err)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.State != "failed" {
		t.Fatal("stopped test approved candidate", current, err)
	}
	for _, node := range current.Nodes {
		if node.ID == "test" && node.State != "failed" {
			t.Fatal("stopped test produced successful evidence", node)
		}
	}
}
