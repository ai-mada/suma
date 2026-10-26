package runner

import (
	"encoding/json"
	"strings"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Exercise the exact model-purpose dispatch called by Execute before workspace
// preparation. This is a routing/authority regression, not a model or SDK run.
func TestCandidateConflictRepairModelPurposeDispatch(t *testing.T) {
	base := strings.Repeat("a", 40)
	sources := []cc.Binding{}
	for i, head := range []string{strings.Repeat("b", 40), strings.Repeat("c", 40)} {
		sources = append(sources, cc.Binding{SchemaVersion: 1, TransferID: string(rune('a' + i)), ProjectID: "project", State: "sealed", Source: cc.AssignmentRef{RunID: string(rune('a' + i)), NodeID: "code", WorkerID: "source", Attempt: 1, Fence: 1}, BaseCommit: base, HeadCommit: head, EvidenceSHA256: strings.Repeat("d", 64), BundleSHA256: strings.Repeat("e", 64), SizeBytes: 100})
	}
	original := compositionAssignment(t, base, sources)
	facts := ccc.CandidateCompositionEvidence{Protocol: ccc.CandidateCompositionProtocol, PlanID: "plan", PlanVersion: 1, SourcesSHA256: original.CandidateComposition.SourcesSHA256, OperationID: "compose-op", BaseCommit: base, State: "conflicted", Parents: []string{}, SourceHeads: []string{sources[0].HeadCommit, sources[1].HeadCommit}, MergeCommits: []string{}, Conflicts: []string{"file.txt"}, ConflictSourceIndex: 1, ConflictTree: strings.Repeat("d", 40), ConflictStages: []ccc.CandidateConflictStage{{Path: "file.txt", Mode: "100644", Object: strings.Repeat("e", 40), Stage: 1}, {Path: "file.txt", Mode: "100644", Object: strings.Repeat("f", 40), Stage: 2}, {Path: "file.txt", Mode: "100644", Object: strings.Repeat("a", 40), Stage: 3}}}
	facts.ConflictSHA256, _ = ccc.CandidateCompositionConflictDigest(facts)
	rawFacts, _ := json.Marshal(facts)
	a := Assignment{RunID: "repair-run", NodeID: "repair", WorkerID: "repairer", Attempt: 1, Fence: 2, Type: "agent", Command: []string{"native"}, Config: map[string]any{"purpose": "candidate_conflict_repair"}, CandidateConflictRepair: &ccc.CandidateConflictRepairExecution{Protocol: ccc.CandidateConflictRepairProtocol, PlanID: "repair-plan", PlanVersion: 1, ProjectID: "project", CompositionID: "plan", CompositionVersion: 2, OriginalAssignment: candidateRef(original), OriginalEvidenceSHA256: digest(rawFacts), Composition: *original.CandidateComposition, Conflict: facts, Instructions: "Resolve the exact frozen conflict.", FixedTimestamp: "2026-10-27T01:00:00Z", OperationID: "repair-operation"}}
	if err := ccc.ValidateCandidateConflictRepairExecution(*a.CandidateConflictRepair); err != nil {
		t.Fatal(err)
	}
	spec := CommandSpec{Adapter: "suma", Protocol: "suma-jsonl-v1", Input: "agent-jsonl", NativeWorkspace: &NativeWorkspacePolicy{ConflictRepair: true}}
	profile := tc.Profile{Adapter: "suma", Protocol: "suma-jsonl-v1", WorkspaceToolsVersion: NativeWorkspaceVersion, ConflictRepairProtocol: ccc.CandidateConflictRepairProtocol}
	input, err := modelPurposeRequest(a, spec, profile)
	if err != nil || input != nil {
		t.Fatalf("authenticated native repair was routed into a document purpose: input=%+v err=%v", input, err)
	}
	encoded, _ := json.Marshal(a)
	cases := map[string]func(*Assignment, *CommandSpec, *tc.Profile){
		"marker_without_typed_binding": func(a *Assignment, _ *CommandSpec, _ *tc.Profile) { a.CandidateConflictRepair = nil },
		"binding_without_marker":       func(a *Assignment, _ *CommandSpec, _ *tc.Profile) { delete(a.Config, "purpose") },
		"wrong_marker":                 func(a *Assignment, _ *CommandSpec, _ *tc.Profile) { a.Config["purpose"] = "planning" },
		"local_opt_out":                func(_ *Assignment, s *CommandSpec, _ *tc.Profile) { s.NativeWorkspace.ConflictRepair = false },
		"unprobed_profile":             func(_ *Assignment, _ *CommandSpec, p *tc.Profile) { p.ConflictRepairProtocol = "" },
		"wrong_workspace_protocol":     func(_ *Assignment, _ *CommandSpec, p *tc.Profile) { p.WorkspaceToolsVersion = "" },
		"readonly_profile":             func(_ *Assignment, _ *CommandSpec, p *tc.Profile) { p.ReadOnly = true },
		"mixed_planning":               func(_ *Assignment, s *CommandSpec, _ *tc.Profile) { s.Planning = true },
		"mixed_extraction":             func(_ *Assignment, s *CommandSpec, _ *tc.Profile) { s.KnowledgeExtraction = true },
		"mixed_diagnosis": func(_ *Assignment, _ *CommandSpec, p *tc.Profile) {
			p.FailureDiagnosis = &tc.FailureDiagnosisCapability{}
		},
		"different_input_mode": func(a *Assignment, _ *CommandSpec, _ *tc.Profile) { a.CodeBaseline = &ccc.CodeBaselineBinding{} },
		"invalid_fixed_execution": func(a *Assignment, _ *CommandSpec, _ *tc.Profile) {
			a.CandidateConflictRepair.OriginalEvidenceSHA256 = strings.Repeat("f", 64)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			var next Assignment
			_ = json.Unmarshal(encoded, &next)
			nextSpec, nextProfile := spec, profile
			nativePolicy := *spec.NativeWorkspace
			nextSpec.NativeWorkspace = &nativePolicy
			mutate(&next, &nextSpec, &nextProfile)
			if _, err := modelPurposeRequest(next, nextSpec, nextProfile); err == nil {
				t.Fatal("invalid repair purpose gained the native dispatch path")
			}
		})
	}
}
