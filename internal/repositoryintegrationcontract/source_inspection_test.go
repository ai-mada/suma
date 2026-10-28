package repositoryintegrationcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRepositorySourceInspectionClosedProofAndRoots(t *testing.T) {
	q := RepositorySourceInspectionRequest{Protocol: RepositorySourceInspectionProtocol, TenantID: "tenant", ProjectID: "project", ProviderID: "provider", TargetID: "target", TargetAlias: "main", TargetRef: "refs/heads/main", TargetConfigSHA256: strings.Repeat("a", 64), OperationID: "apply-source-123", RequestSHA256: strings.Repeat("b", 64), ReceiptSHA256: strings.Repeat("c", 64), ExactCommit: strings.Repeat("d", 40), RelativeRoots: []string{"packages/a", "packages/b"}}
	if ValidateRepositorySourceInspectionRequest(q) != nil {
		t.Fatal("valid immutable source proof rejected")
	}
	raw, _ := json.Marshal(q)
	for _, invalid := range []string{strings.Replace(string(raw), `"target_id"`, `"Target_ID"`, 1), strings.Replace(string(raw), `"target_id":"target"`, `"target_id":"first","target_id":"target"`, 1), strings.Replace(string(raw), `"relative_roots":["packages/a","packages/b"]`, `"relative_roots":["packages","packages/a"]`, 1)} {
		var decoded RepositorySourceInspectionRequest
		if json.Unmarshal([]byte(invalid), &decoded) == nil {
			t.Fatal("noncanonical source proof accepted")
		}
	}
}
