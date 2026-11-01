package developmentcontract

import (
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

func continuationFixture(t *testing.T) DevelopmentContinuationBinding {
	t.Helper()
	b := DevelopmentContinuationBinding{Protocol: ContinuationProtocol, ID: "new-plan", ProjectID: "project", TaskID: "task", TaskVersion: 4, SourceTaskRunID: "old-run", SourceTaskVersion: 2, SourceRunVersion: 7, Source: cc.Binding{SchemaVersion: 1, TransferID: "sealed-transfer", ProjectID: "project", State: "sealed", Source: cc.AssignmentRef{RunID: "old-run", NodeID: "code", Attempt: 2, Fence: 9, WorkerID: "producer"}, BaseCommit: strings.Repeat("a", 40), HeadCommit: strings.Repeat("b", 40), EvidenceSHA256: strings.Repeat("c", 64), BundleSHA256: strings.Repeat("d", 64), SizeBytes: 1234}}
	var err error
	b.BindingSHA256, err = BindingDigest(b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestContinuationFixesTheTaskAndOriginalSealedCandidate(t *testing.T) {
	b := continuationFixture(t)
	if err := ValidateBinding(b); err != nil {
		t.Fatal(err)
	}
	// An inherited input can have selected M and an earlier bundle anchor B.
	b.Source.BundleBaseCommit, b.Source.BaseCommit = b.Source.BaseCommit, b.Source.HeadCommit
	b.BindingSHA256, _ = BindingDigest(b)
	if err := ValidateBinding(b); err != nil {
		t.Fatal("honest inherited bundle anchor rejected", err)
	}
	for name, change := range map[string]func(*DevelopmentContinuationBinding){
		"another task":             func(v *DevelopmentContinuationBinding) { v.TaskID = "other" },
		"another source fence":     func(v *DevelopmentContinuationBinding) { v.Source.Source.Fence++ },
		"new source contents":      func(v *DevelopmentContinuationBinding) { v.Source.BundleSHA256 = strings.Repeat("e", 64) },
		"new original run version": func(v *DevelopmentContinuationBinding) { v.SourceRunVersion++ },
	} {
		t.Run(name, func(t *testing.T) {
			v := b
			change(&v)
			if ValidateBinding(v) == nil {
				t.Fatal("old digest accepted changed task/source")
			}
		})
	}
	b.Source.ProjectID = "other-project"
	if _, err := BindingDigest(b); err == nil {
		t.Fatal("a fresh digest must not legalize foreign source scope")
	}
	if ValidateSource(DevelopmentContinuationSource{RunID: "old-run", NodeID: "code", Attempt: 2, Fence: 9, ExpectedRunVersion: 0}) == nil {
		t.Fatal("source selection must name a known run version")
	}
}
