package toolcontract

import (
	"encoding/json"
	"testing"
)

func TestIntelligentOperationsProfileWireCloneAndIsolation(t *testing.T) {
	p := fixtureProfile()
	p.Adapter, p.Protocol = "suma", "suma-jsonl-v1"
	p.IntelligentOperations = &IntelligentOperationsCapability{Protocol: IntelligentOperationsProtocol, Model: "fixed", EndpointOrigin: "http://127.0.0.1:8765"}
	if err := ValidateProfile(p, true); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	var decoded Profile
	if err := json.Unmarshal(raw, &decoded); err != nil || !EqualProfile(p, decoded) {
		t.Fatal("wire identity lost", err)
	}
	copy := CloneProfile(p)
	copy.IntelligentOperations.Model = "different"
	if EqualProfile(p, copy) || p.IntelligentOperations.Model != "fixed" {
		t.Fatal("capability aliases original")
	}
	for _, mutate := range []func(*Profile){
		func(v *Profile) { v.ReadOnly = false },
		func(v *Profile) { v.WorkspaceToolsVersion = "attempt-workspace-v1" },
		func(v *Profile) {
			v.KnowledgeExtraction = &KnowledgeExtractionCapability{Protocol: KnowledgeExtractionProtocol, Model: "fixed", EndpointOrigin: "http://127.0.0.1:8765"}
		},
		func(v *Profile) { v.IntelligentOperations.EndpointOrigin = "https://model.example/path" },
	} {
		v := CloneProfile(p)
		mutate(&v)
		if ValidateProfile(v, true) == nil {
			t.Fatal("mixed purpose, workspace or non-origin endpoint accepted")
		}
	}
}
