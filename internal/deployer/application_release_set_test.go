package deployer

import (
	rc "github.com/ai-mada/suma/internal/releasecontract"
	"strings"
	"testing"
)

// Even a structurally valid original authorization cannot carry a partial or
// traversing application reference. Recomputing the plan digest is insufficient.
func TestProviderApplicationReleaseReferenceMustRemainComplete(t *testing.T) {
	f, control, cfg := providerFixture(t)
	defer control.Close()
	server, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	request := f.request("operation-release-set124", "artifact-A", 1, "deploy", "")
	grant := f.plans[request.OperationID]
	grant.Plan.ApplicationReleaseSet = &rc.ApplicationReleaseSetRef{ReleaseSetID: "release-set124", ReleaseSetSHA256: strings.Repeat("a", 64), ApplicationID: "application124", ApplicationVersionID: "code-version124", ApplicationVersionSHA256: strings.Repeat("b", 64)}
	bind := func() operation {
		request.PlanSHA256, _ = rc.CanonicalDigest(grant.Plan)
		grant.RequestSHA256, _ = rc.DeploymentRequestDigest(request)
		return operation{Request: rc.NonSecretDeploymentRequest{ReleaseExecution: request.ReleaseExecution, Assignment: request.Assignment}, Observation: rc.DeploymentObservation{RequestSHA256: grant.RequestSHA256}}
	}
	operation := bind()
	if !server.validGrant(&operation, grant) {
		t.Fatal("exact original set member grant rejected")
	}
	grant.Plan.ApplicationReleaseSet.ApplicationID = "../other"
	operation = bind()
	if server.validGrant(&operation, grant) {
		t.Fatal("correctly digested malformed set member grant accepted")
	}
}
