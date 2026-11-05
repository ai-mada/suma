package repositoryintegrationcontract

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

const RepositorySourceExportProtocol = "suma.repository-source-export.v1"

// RepositorySourceExportRequest selects an exact pair of already configured
// source and baseline commits. ExpectedBase asserts the provider's fixed
// baseline ref; it cannot select an arbitrary object or another repository.
type RepositorySourceExportRequest struct {
	Protocol           string `json:"protocol"`
	TenantID           string `json:"tenant_id"`
	ProjectID          string `json:"project_id"`
	ProviderID         string `json:"provider_id"`
	TargetAlias        string `json:"target_alias"`
	TargetRef          string `json:"target_ref"`
	TargetConfigSHA256 string `json:"target_config_sha256"`
	ExpectedHead       string `json:"expected_head"`
	ExpectedFenceOID   string `json:"expected_fence_oid"`
	ExpectedGeneration int64  `json:"expected_generation"`
	ExpectedBase       string `json:"expected_base"`
}

func ValidateRepositorySourceExportRequest(q RepositorySourceExportRequest) error {
	for _, id := range []string{q.TenantID, q.ProjectID, q.ProviderID, q.TargetAlias} {
		if !sourceIdentity.MatchString(id) {
			return fmt.Errorf("invalid repository export identity")
		}
	}
	if q.Protocol != RepositorySourceExportProtocol || !RepositoryValidRef(q.TargetRef) || !cc.ValidHash(q.TargetConfigSHA256) ||
		!cc.ValidCommit(q.ExpectedHead) || !cc.ValidCommit(q.ExpectedBase) || len(q.ExpectedHead) != len(q.ExpectedBase) || q.ExpectedHead == q.ExpectedBase ||
		q.ExpectedFenceOID != "" && (!cc.ValidCommit(q.ExpectedFenceOID) || len(q.ExpectedFenceOID) != len(q.ExpectedHead)) ||
		q.ExpectedGeneration < 0 || q.ExpectedGeneration >= 9007199254740990 || (q.ExpectedGeneration == 0) != (q.ExpectedFenceOID == "") {
		return fmt.Errorf("invalid repository export binding")
	}
	return nil
}

func (q *RepositorySourceExportRequest) UnmarshalJSON(raw []byte) error {
	if len(raw) > 8192 || !utf8.Valid(raw) {
		return fmt.Errorf("invalid repository export request size or encoding")
	}
	keys := []string{"protocol", "tenant_id", "project_id", "provider_id", "target_alias", "target_ref", "target_config_sha256", "expected_head", "expected_fence_oid", "expected_generation", "expected_base"}
	if err := exactRepositorySourceFields(raw, keys); err != nil {
		return err
	}
	type wire RepositorySourceExportRequest
	var value wire
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if err := ValidateRepositorySourceExportRequest(RepositorySourceExportRequest(value)); err != nil {
		return err
	}
	*q = RepositorySourceExportRequest(value)
	return nil
}

func RepositorySourceExportDigest(q RepositorySourceExportRequest) (string, error) {
	if err := ValidateRepositorySourceExportRequest(q); err != nil {
		return "", err
	}
	return RepositoryCanonicalDigest(q)
}
