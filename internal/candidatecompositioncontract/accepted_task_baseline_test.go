package candidatecompositioncontract

import (
	"strings"
	"testing"
)

func TestAcceptedTaskBaselinePinsOneOriginAndSourceIdentity(t *testing.T) {
	source := compositionFixture().Sources[0]
	binding := CodeBaselineBinding{Protocol: CodeBaselineProtocol, ID: "next-binding", ProjectID: source.ProjectID, TaskID: "follow-up", TaskVersion: 2, SourceTaskID: "accepted-source", SourceTaskVersion: 4, AcceptedBasisSHA256: strings.Repeat("f", 64), Source: source}
	binding.BindingSHA256, _ = CodeBaselineBindingDigest(binding)
	if err := ValidateCodeBaselineBinding(binding); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CodeBaselineBinding){
		func(b *CodeBaselineBinding) { b.SourceTaskID = "" },
		func(b *CodeBaselineBinding) { b.SourceTaskVersion = 0 },
		func(b *CodeBaselineBinding) { b.SourceTaskID = b.TaskID },
		func(b *CodeBaselineBinding) { b.CompositionID = "other-origin" },
		func(b *CodeBaselineBinding) { b.CompositionVersion = 1 },
		func(b *CodeBaselineBinding) { b.TerminalCandidatePlanID = "terminal-origin" },
		func(b *CodeBaselineBinding) { b.TerminalCandidatePlanVersion = 1 },
	} {
		invalid := binding
		mutate(&invalid)
		invalid.BindingSHA256, _ = CodeBaselineBindingDigest(invalid)
		if ValidateCodeBaselineBinding(invalid) == nil {
			t.Fatal("incomplete, self or mixed accepted-task origin admitted")
		}
	}
	changed := binding
	changed.SourceTaskVersion++
	if ValidateCodeBaselineBinding(changed) == nil {
		t.Fatal("source task version not covered by digest")
	}
	changed = binding
	changed.Source.Source.NodeID = "other-producer"
	if ValidateCodeBaselineBinding(changed) == nil {
		t.Fatal("source producer not covered by digest")
	}
}
