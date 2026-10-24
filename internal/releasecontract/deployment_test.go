package releasecontract

import (
	"strings"
	"testing"
)

func TestDeploymentContractDigestExcludesOnlyTicket(t *testing.T) {
	r := DeploymentRequest{ReleaseExecution: ReleaseExecution{Protocol: ReleaseProtocol, OperationID: "deploy-operation-001", PlanID: "plan", PlanRevision: 1, PlanSHA256: strings.Repeat("a", 64), EnvironmentID: "environment", ProviderID: "provider", ExpectedGeneration: 0, Generation: 1, ArtifactID: "artifact", ArtifactSHA256: strings.Repeat("b", 64)}, Assignment: AssignmentRef{RunID: "run", NodeID: "deploy", Attempt: 1, Fence: 1, WorkerID: "worker"}, AuthorizationTicket: "secret-one"}
	a, err := DeploymentRequestDigest(r)
	if err != nil {
		t.Fatal(err)
	}
	r.AuthorizationTicket = "secret-two"
	b, err := DeploymentRequestDigest(r)
	if err != nil || a != b {
		t.Fatal("renewal changed operation semantics")
	}
	r.Assignment.Fence++
	c, err := DeploymentRequestDigest(r)
	if err != nil || a == c {
		t.Fatal("assignment was not bound")
	}
	r.Assignment.Fence--
	r.ArtifactSHA256 = strings.Repeat("c", 64)
	c, err = DeploymentRequestDigest(r)
	if err != nil || a == c {
		t.Fatal("artifact was not bound")
	}
	first, _ := CanonicalDigest(map[string]any{"b": int64(9007199254740991), "a": 1})
	second, _ := CanonicalDigest(map[string]any{"a": 1, "b": int64(9007199254740991)})
	if first != second {
		t.Fatal("field order affected canonical digest")
	}
}
func TestDeploymentContractHealthPolicyFixedSiteOnly(t *testing.T) {
	valid := HealthPolicy{Checks: []HealthCheck{{Path: "/", ExpectedStatus: 200, ExpectedFile: "index.html"}}, TimeoutMS: 1000, Attempts: 3}
	if err := ValidateHealthPolicy(valid, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"https://example.test/", "//example.test/", "/../private", "/.suma-release", "/%2e%2e/private", "/?url=elsewhere", "/a b", "/雪", "/a\n"} {
		h := valid
		h.Checks = append([]HealthCheck{}, valid.Checks...)
		h.Checks[0].Path = path
		if ValidateHealthPolicy(h, false) == nil {
			t.Fatalf("unsafe health path accepted: %s", path)
		}
	}
	h := valid
	h.Checks = append([]HealthCheck{}, valid.Checks...)
	h.Checks[0].ExpectedSHA256 = strings.Repeat("a", 64)
	if ValidateHealthPolicy(h, false) == nil {
		t.Fatal("browser supplied expected content digest")
	}
	if ValidateHealthPolicy(h, true) != nil {
		t.Fatal("frozen policy rejected actual content digest")
	}
}
