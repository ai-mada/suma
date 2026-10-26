package candidatecompositioncontract

import (
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

func compositionFixture() CandidateCompositionExecution {
	base := strings.Repeat("a", 40)
	e := CandidateCompositionExecution{Protocol: CandidateCompositionProtocol, PlanID: "plan", PlanVersion: 1, ProjectID: "project", BaseCommit: base, FixedTimestamp: "2026-10-27T00:00:00Z", OperationID: "composition-operation"}
	for i, head := range []string{strings.Repeat("b", 40), strings.Repeat("c", 40)} {
		e.Sources = append(e.Sources, cc.Binding{SchemaVersion: 1, TransferID: string(rune('a' + i)), ProjectID: e.ProjectID, State: "sealed", Source: cc.AssignmentRef{RunID: string(rune('a' + i)), NodeID: "code", WorkerID: "worker", Attempt: 1, Fence: 2}, BaseCommit: base, HeadCommit: head, EvidenceSHA256: strings.Repeat("d", 64), BundleSHA256: strings.Repeat("e", 64), SizeBytes: 100})
	}
	e.SourcesSHA256, _ = CandidateCompositionSourcesDigest(e.Sources)
	return e
}
func TestCandidateCompositionOrderedSourceAndScopeContract(t *testing.T) {
	e := compositionFixture()
	if err := ValidateCandidateCompositionExecution(e); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*CandidateCompositionExecution){
		func(v *CandidateCompositionExecution) { v.Sources = v.Sources[:1] },
		func(v *CandidateCompositionExecution) { v.Sources[0], v.Sources[1] = v.Sources[1], v.Sources[0] },
		func(v *CandidateCompositionExecution) { v.Sources[0].ProjectID = "other" },
		func(v *CandidateCompositionExecution) { v.Sources[0].BundleBaseCommit = strings.Repeat("f", 40) },
		func(v *CandidateCompositionExecution) { v.FixedTimestamp = "2026-10-27T00:00:00+01:00" },
	} {
		v := compositionFixture()
		mutate(&v)
		if ValidateCandidateCompositionExecution(v) == nil {
			t.Fatal("source order, count, scope, anchor or fixed-time mutation admitted")
		}
	}
}
func TestCodeBaselineDigestAndInheritedAnchorContract(t *testing.T) {
	e := compositionFixture()
	source := e.Sources[0]
	source.BundleBaseCommit, source.BaseCommit = source.BaseCommit, source.HeadCommit
	if !source.ValidSealed() {
		t.Fatal("inherited no-delta B..M binding was rejected")
	}
	b := CodeBaselineBinding{Protocol: CodeBaselineProtocol, ID: "binding", ProjectID: e.ProjectID, TaskID: "next-task", TaskVersion: 2, CompositionID: e.PlanID, CompositionVersion: e.PlanVersion, AcceptedBasisSHA256: strings.Repeat("f", 64), Source: source}
	b.BindingSHA256, _ = CodeBaselineBindingDigest(b)
	if err := ValidateCodeBaselineBinding(b); err != nil {
		t.Fatal(err)
	}
	b.TaskVersion++
	if ValidateCodeBaselineBinding(b) == nil {
		t.Fatal("changed downstream task accepted an old baseline digest")
	}
	q := cc.StartRequest{ProjectID: e.ProjectID, Assignment: source.Source, OperationID: "inherited-operation", BaseCommit: source.BaseCommit, BundleBaseCommit: source.BundleBaseCommit, HeadCommit: source.HeadCommit, EvidenceSHA256: source.EvidenceSHA256, BundleSHA256: source.BundleSHA256, SizeBytes: source.SizeBytes}
	if err := q.Validate(); err != nil {
		t.Fatal(err)
	}
	q.BundleBaseCommit = ""
	if q.Validate() == nil {
		t.Fatal("an actual empty B..B transfer was accepted")
	}
}
