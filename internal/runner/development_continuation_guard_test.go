package runner

import (
	"context"
	"strings"
	"testing"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	dc "github.com/ai-mada/suma/internal/developmentcontract"
)

func continuationGuardAssignment(t *testing.T) Assignment {
	t.Helper()
	b := dc.DevelopmentContinuationBinding{Protocol: dc.ContinuationProtocol, ID: "plan", ProjectID: "project", TaskID: "task", TaskVersion: 2, SourceTaskRunID: "previous-run", SourceTaskVersion: 1, SourceRunVersion: 3, Source: cc.Binding{SchemaVersion: 1, TransferID: "transfer", ProjectID: "project", State: "sealed", Source: cc.AssignmentRef{RunID: "previous-run", NodeID: "code", Attempt: 1, Fence: 1, WorkerID: "producer"}, BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("b", 40), EvidenceSHA256: strings.Repeat("c", 64), BundleSHA256: strings.Repeat("d", 64), SizeBytes: 100}}
	var err error
	b.BindingSHA256, err = dc.BindingDigest(b)
	if err != nil {
		t.Fatal(err)
	}
	return Assignment{RunID: "next-run", NodeID: "develop", WorkerID: "consumer", Attempt: 1, Fence: 1, Type: "agent", Command: []string{"develop"}, Config: map[string]any{"goal": "Continue the reviewed change", "development_continuation_id": b.ID}, DevelopmentContinuation: &b}
}

func TestDevelopmentContinuationRejectsMixedAndDedicatedModesBeforeWorkspace(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Assignment, *CommandSpec)
	}{
		{"command", func(a *Assignment, _ *CommandSpec) { a.Type = "command" }},
		{"terminal", func(a *Assignment, _ *CommandSpec) { a.Type = "terminal_session" }},
		{"cloud", func(a *Assignment, _ *CommandSpec) { a.Config["plugin"] = "workbuddy-cloud" }},
		{"accepted_baseline", func(a *Assignment, _ *CommandSpec) { a.CodeBaseline = &ccc.CodeBaselineBinding{} }},
		{"composition", func(a *Assignment, _ *CommandSpec) { a.CandidateComposition = &ccc.CandidateCompositionExecution{} }},
		{"legacy_candidate", func(a *Assignment, _ *CommandSpec) { a.CandidateCommit = strings.Repeat("e", 40) }},
		{"purpose", func(a *Assignment, _ *CommandSpec) { a.Config["purpose"] = "task_conversation_reply" }},
		{"empty_purpose", func(a *Assignment, _ *CommandSpec) { a.Config["purpose"] = "" }},
		{"null_purpose", func(a *Assignment, _ *CommandSpec) { a.Config["purpose"] = nil }},
		{"forged_config", func(a *Assignment, _ *CommandSpec) {
			a.DevelopmentContinuation = nil
			a.Config["development_continuation"] = "transfer"
		}},
		{"missing_binding", func(a *Assignment, _ *CommandSpec) { a.DevelopmentContinuation = nil }},
		{"changed_marker", func(a *Assignment, _ *CommandSpec) { a.Config["development_continuation_id"] = "other-plan" }},
		{"changed_digest", func(a *Assignment, _ *CommandSpec) { a.DevelopmentContinuation.TaskID = "other" }},
		{"read_only", func(_ *Assignment, s *CommandSpec) { s.ReadOnly = true }},
		{"planning_profile", func(_ *Assignment, s *CommandSpec) { s.Planning = true }},
		{"test_profile", func(_ *Assignment, s *CommandSpec) { s.TestEvidence = true }},
		{"composition_profile", func(_ *Assignment, s *CommandSpec) { s.CandidateComposition = true }},
		{"artifact_profile", func(_ *Assignment, s *CommandSpec) { s.ArtifactInput = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := continuationGuardAssignment(t)
			spec := CommandSpec{Adapter: "codex", Protocol: "codex-jsonl-v1", Input: "goal"}
			test.edit(&a, &spec)
			r := Execute(context.Background(), a, ExecConfig{AllowExec: true, Commands: map[string]CommandSpec{"develop": spec}, Workspace: "unavailable"})
			if r.ExitCode == 0 || !strings.Contains(r.Error, "development continuation") || r.WorkspaceEvidence != nil {
				t.Fatalf("mixed input reached another mode: %+v", r)
			}
		})
	}
}

func TestDevelopmentContinuationCloneFreezesOriginalBinding(t *testing.T) {
	a := continuationGuardAssignment(t)
	copy := cloneModelAssignment(a)
	a.DevelopmentContinuation.Source.HeadCommit = strings.Repeat("f", 40)
	a.Config["development_continuation_id"] = "changed-plan"
	if copy.DevelopmentContinuation == a.DevelopmentContinuation || dc.ValidateBinding(*copy.DevelopmentContinuation) != nil || copy.Config["development_continuation_id"] != copy.DevelopmentContinuation.ID {
		t.Fatal("caller mutation changed the frozen continuation source")
	}
}
