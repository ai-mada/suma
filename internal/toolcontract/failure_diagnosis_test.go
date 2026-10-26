package toolcontract

import (
	"encoding/json"
	"testing"
)

func TestFailureDiagnosisProfileWireAndIsolation(t *testing.T) {
	base := fixtureProfile()
	base.Adapter, base.Protocol = "suma", "suma-jsonl-v1"
	raw, _ := json.Marshal(base)
	decode := func(capability string) Profile {
		t.Helper()
		var p Profile
		if err := json.Unmarshal(append(raw[:len(raw)-1], []byte(`,"failure_diagnosis":`+capability+`}`)...), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	p := decode(`{"protocol":"suma-failure-diagnosis-v1","model":"diagnostician","endpoint_origin":"https://models.example"}`)
	if err := ValidateProfile(p, true); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(p)
	var wire map[string]any
	json.Unmarshal(encoded, &wire)
	if wire["failure_diagnosis"] == nil {
		t.Fatal("dedicated diagnosis capability was discarded")
	}
	for _, capability := range []string{
		`{"protocol":"suma-schedule-plan-v1","model":"diagnostician","endpoint_origin":"https://models.example"}`,
		`{"protocol":"suma-failure-diagnosis-v1","model":"","endpoint_origin":"https://models.example"}`,
		`{"protocol":"suma-failure-diagnosis-v1","model":"diagnostician","endpoint_origin":"http://remote.example"}`,
		`{"protocol":"suma-failure-diagnosis-v1","model":"diagnostician","endpoint_origin":"https://models.example/v1"}`,
	} {
		if ValidateProfile(decode(capability), true) == nil {
			t.Fatalf("invalid dedicated capability accepted: %s", capability)
		}
	}
}

func TestFailureDiagnosisProfileCloneAndPurposeIsolation(t *testing.T) {
	p := fixtureProfile()
	p.Adapter, p.Protocol = "suma", "suma-jsonl-v1"
	p.FailureDiagnosis = &FailureDiagnosisCapability{Protocol: FailureDiagnosisProtocol, Model: "fixed", EndpointOrigin: "http://[::1]:8080"}
	raw, _ := json.Marshal(p)
	var decoded Profile
	if err := json.Unmarshal(raw, &decoded); err != nil || !EqualProfile(p, decoded) || p.FailureDiagnosis == decoded.FailureDiagnosis {
		t.Fatal("separate wire decode changed semantic identity", err)
	}
	for _, mutate := range []func(*Profile){
		func(v *Profile) { v.ReadOnly = false },
		func(v *Profile) {
			v.KnowledgeExtraction = &KnowledgeExtractionCapability{Protocol: KnowledgeExtractionProtocol, Model: "fixed", EndpointOrigin: "http://[::1]:8080"}
		},
		func(v *Profile) { v.WorkspaceToolsVersion = "attempt-workspace-v1" },
		func(v *Profile) {
			v.Planning = &PlanningCapability{Protocol: PlanningProtocol, Model: "fixed", EndpointOrigin: "http://[::1]:8080"}
		},
	} {
		v := CloneProfile(p)
		mutate(&v)
		if ValidateProfile(v, true) == nil {
			t.Fatal("dedicated diagnosis profile gained another purpose or workspace")
		}
	}
	copy := CloneProfile(p)
	copy.FailureDiagnosis.Model = "foreign"
	if EqualProfile(p, copy) || p.FailureDiagnosis.Model != "fixed" {
		t.Fatal("new capability aliases its original profile")
	}
}
