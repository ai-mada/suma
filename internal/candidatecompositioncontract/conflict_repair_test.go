package candidatecompositioncontract

import (
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

func repairContractFixture() CandidateConflictRepairExecution {
	c := compositionFixture()
	f := CandidateCompositionEvidence{Protocol: c.Protocol, PlanID: c.PlanID, PlanVersion: c.PlanVersion, SourcesSHA256: c.SourcesSHA256, OperationID: c.OperationID, BaseCommit: c.BaseCommit, State: "conflicted", Parents: []string{}, SourceHeads: []string{c.Sources[0].HeadCommit, c.Sources[1].HeadCommit}, MergeCommits: []string{}, Conflicts: []string{"file.txt"}, ConflictSourceIndex: 1, ConflictTree: strings.Repeat("d", 40), ConflictStages: []CandidateConflictStage{{Path: "file.txt", Mode: "100644", Object: strings.Repeat("e", 40), Stage: 1}, {Path: "file.txt", Mode: "100644", Object: strings.Repeat("f", 40), Stage: 2}, {Path: "file.txt", Mode: "100644", Object: strings.Repeat("a", 40), Stage: 3}}}
	f.ConflictSHA256, _ = CandidateCompositionConflictDigest(f)
	sha, _ := canonicalSHA(f)
	return CandidateConflictRepairExecution{Protocol: CandidateConflictRepairProtocol, PlanID: "repair-plan", PlanVersion: 1, ProjectID: c.ProjectID, CompositionID: c.PlanID, CompositionVersion: 2, OriginalAssignment: cc.AssignmentRef{RunID: "original", NodeID: "compose", Attempt: 1, Fence: 1, WorkerID: "worker"}, OriginalEvidenceSHA256: sha, Composition: c, Conflict: f, Instructions: "Resolve all frozen inputs.", FixedTimestamp: "2026-10-27T01:00:00Z", OperationID: "repair-operation"}
}

func TestCandidateConflictRepairRejectsPathOnlyOriginalAndSourceOrEvidenceDrift(t *testing.T) {
	if err := ValidateCandidateConflictRepairExecution(repairContractFixture()); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CandidateConflictRepairExecution){func(e *CandidateConflictRepairExecution) { e.Conflict.ConflictTree = "" }, func(e *CandidateConflictRepairExecution) { e.Conflict.ConflictStages = nil }, func(e *CandidateConflictRepairExecution) {
		e.Conflict.ConflictStages[0].Object = strings.Repeat("b", 40)
	}, func(e *CandidateConflictRepairExecution) {
		e.Composition.Sources[0], e.Composition.Sources[1] = e.Composition.Sources[1], e.Composition.Sources[0]
	}, func(e *CandidateConflictRepairExecution) { e.OriginalEvidenceSHA256 = strings.Repeat("f", 64) }} {
		e := repairContractFixture()
		mutate(&e)
		if ValidateCandidateConflictRepairExecution(e) == nil {
			t.Fatal("changed rich original evidence was accepted")
		}
	}
}
