package candidatecompositioncontract

import (
	"strings"
	"testing"
)

// A stopped-terminal candidate is a distinct current origin. Its accepted
// source must not be confused with, or combined with, a multi-branch merge.
func TestCodeBaselineRequiresOneExactAcceptedOrigin(t *testing.T) {
	source := compositionFixture().Sources[0]
	b := CodeBaselineBinding{Protocol: CodeBaselineProtocol, ID: "binding", ProjectID: source.ProjectID, TaskID: "follow-up", TaskVersion: 2, TerminalCandidatePlanID: "terminal-plan", TerminalCandidatePlanVersion: 2, AcceptedBasisSHA256: strings.Repeat("f", 64), Source: source}
	b.BindingSHA256, _ = CodeBaselineBindingDigest(b)
	if err := ValidateCodeBaselineBinding(b); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*CodeBaselineBinding){
		func(v *CodeBaselineBinding) { v.CompositionID, v.CompositionVersion = "merge-plan", 2 },
		func(v *CodeBaselineBinding) { v.CompositionVersion = 1 },
		func(v *CodeBaselineBinding) { v.TerminalCandidatePlanID = "" },
		func(v *CodeBaselineBinding) { v.TerminalCandidatePlanVersion = 0 },
	} {
		bad := b
		change(&bad)
		bad.BindingSHA256, _ = CodeBaselineBindingDigest(bad)
		if ValidateCodeBaselineBinding(bad) == nil {
			t.Fatal("ambiguous or incomplete baseline origin admitted")
		}
	}
	b.TerminalCandidatePlanVersion++
	if ValidateCodeBaselineBinding(b) == nil {
		t.Fatal("a changed accepted source retained the old digest")
	}
}
