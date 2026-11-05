package repositoryintegrationcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRepositorySourceExportRequestClosedAndBound(t *testing.T) {
	request := RepositorySourceExportRequest{
		Protocol: RepositorySourceExportProtocol, TenantID: "tenant", ProjectID: "project", ProviderID: "provider",
		TargetAlias: "delivery", TargetRef: "refs/heads/delivery", TargetConfigSHA256: strings.Repeat("a", 64),
		ExpectedHead: strings.Repeat("b", 40), ExpectedBase: strings.Repeat("c", 40),
	}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RepositorySourceExportRequest
	if err := json.Unmarshal(raw, &decoded); err != nil || decoded != request {
		t.Fatalf("valid request did not round-trip: %v", err)
	}
	for name, input := range map[string]string{
		"missing generation": strings.Replace(string(raw), `"expected_generation":0,`, "", 1),
		"duplicate field":    strings.Replace(string(raw), `"expected_generation":0`, `"expected_generation":0,"expected_generation":0`, 1),
		"extra baseline ref": strings.Replace(string(raw), `"expected_generation":0`, `"expected_generation":0,"baseline_ref":"refs/heads/main"`, 1),
		"null fence":         strings.Replace(string(raw), `"expected_fence_oid":""`, `"expected_fence_oid":null`, 1),
		"fence missing":      strings.Replace(string(raw), `"expected_generation":0`, `"expected_generation":1`, 1),
		"mixed formats":      strings.Replace(string(raw), request.ExpectedBase, strings.Repeat("d", 64), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := json.Unmarshal([]byte(input), &decoded); err == nil {
				t.Fatal("invalid export request was accepted")
			}
		})
	}
	original, err := RepositorySourceExportDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedBase = strings.Repeat("d", 40)
	changed, err := RepositorySourceExportDigest(request)
	if err != nil || changed == original {
		t.Fatal("changing the exact baseline must change the export request digest")
	}
}
