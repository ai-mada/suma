package toolcontract

import (
	"encoding/json"
	"testing"
)

func TestPlanningProfileCapabilityValidation(t *testing.T) {
	base := fixtureProfile()
	base.Adapter, base.Protocol = "suma", "suma-jsonl-v1"
	raw, _ := json.Marshal(base)
	for _, value := range []string{
		`{"protocol":"wrong","model":"planner","endpoint_origin":"https://models.example"}`,
		`{"protocol":"suma-schedule-plan-v1","model":"planner","endpoint_origin":"https://models.example/v1"}`,
		`{"protocol":"suma-schedule-plan-v1","model":"planner","endpoint_origin":"http://remote.example"}`,
	} {
		var p Profile
		if err := json.Unmarshal(append(raw[:len(raw)-1], []byte(`,"planning":`+value+`}`)...), &p); err != nil {
			t.Fatal(err)
		}
		if ValidateProfile(p, true) == nil {
			t.Fatalf("accepted invalid planning capability: %s", value)
		}
	}
}

func TestPlanningProfileSemanticEqualityAndClone(t *testing.T) {
	p := fixtureProfile()
	p.Adapter, p.Protocol = "suma", "suma-jsonl-v1"
	p.Planning = &PlanningCapability{Protocol: PlanningProtocol, Model: "planner", EndpointOrigin: "https://models.example"}
	if err := ValidateProfile(p, true); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(p)
	var decoded Profile
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if p.Planning == decoded.Planning || !EqualProfile(p, decoded) {
		t.Fatal("wire identity used pointer equality")
	}
	copy := CloneProfile(p)
	copy.Planning.Model = "different"
	if EqualProfile(p, copy) || p.Planning.Model != "planner" {
		t.Fatal("profile alias mutated original")
	}
	for _, mutate := range []func(*Profile){func(v *Profile) { v.ReadOnly = false }, func(v *Profile) { v.WorkspaceToolsVersion = "attempt-workspace-v1" }, func(v *Profile) { v.Planning.Model = "" }} {
		q := CloneProfile(p)
		mutate(&q)
		if ValidateProfile(q, true) == nil {
			t.Fatal("accepted incompatible planning profile")
		}
	}
}
