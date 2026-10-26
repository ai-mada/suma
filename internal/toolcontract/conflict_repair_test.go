package toolcontract

import (
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"testing"
)

func TestConflictRepairProfileCannotEscalateGenericOrReadOnlyTools(t *testing.T) {
	p := Profile{CommandID: "native-repair", Adapter: "suma", Protocol: "suma-jsonl-v1", WorkspaceToolsVersion: "attempt-workspace-v1", ConflictRepairProtocol: ccmp.CandidateConflictRepairProtocol}
	if err := ValidateProfile(p, false); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*Profile){
		func(v *Profile) { v.ConflictRepairProtocol = "unsupported" },
		func(v *Profile) { v.Adapter = "codex"; v.Protocol = "codex-jsonl-v1" },
		func(v *Profile) { v.ReadOnly = true },
		func(v *Profile) { v.WorkspaceToolsVersion = "" },
		func(v *Profile) {
			v.Planning = &PlanningCapability{Protocol: PlanningProtocol, Model: "fixed", EndpointOrigin: "http://127.0.0.1"}
		},
		func(v *Profile) {
			v.KnowledgeExtraction = &KnowledgeExtractionCapability{Protocol: KnowledgeExtractionProtocol, Model: "fixed", EndpointOrigin: "http://127.0.0.1"}
		},
		func(v *Profile) {
			v.FailureDiagnosis = &FailureDiagnosisCapability{Protocol: FailureDiagnosisProtocol, Model: "fixed", EndpointOrigin: "http://127.0.0.1"}
		},
	}
	for i, change := range mutations {
		q := CloneProfile(p)
		change(&q)
		if ValidateProfile(q, false) == nil {
			t.Fatalf("unsafe capability variation %d accepted", i)
		}
	}
	q := CloneProfile(p)
	q.ConflictRepairProtocol = ""
	if EqualProfile(p, q) {
		t.Fatal("fixed-profile equality lost repair capability")
	}
}
