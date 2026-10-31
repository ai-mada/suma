package codexcontract

import (
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/strictjson"
)

const ModelDiscoveryProtocol = "suma-codex-model-discovery-v1"

// CodexModelDiscoveryResult is the bounded public projection of an explicit
// local catalogue probe. Installation paths, endpoint configuration and secret
// values remain in the parent process. A ready catalogue is not account health,
// a model response, a provider bill, or permission to dispatch a task.
type CodexModelDiscoveryResult struct {
	Protocol            string        `json:"protocol"`
	State               string        `json:"state"`
	ExecutableSHA256    string        `json:"executable_sha256"`
	ToolVersion         string        `json:"tool_version"`
	ConfigurationSHA256 string        `json:"configuration_sha256"`
	Catalogue           *CodexCatalog `json:"catalogue,omitempty" wire:"nonnull"`
	ErrorCode           string        `json:"error_code,omitempty"`
}

// ValidateModelDiscoveryResult keeps failed or uncertain probes from carrying a
// usable catalogue. Model options retain the original Codex catalog contract;
// there is no second model directory or guessed provider identity.
func ValidateModelDiscoveryResult(value CodexModelDiscoveryResult) error {
	invalid := fmt.Errorf("invalid Codex model discovery observation")
	if value.Protocol != ModelDiscoveryProtocol || !hash.MatchString(value.ExecutableSHA256) || !hash.MatchString(value.ConfigurationSHA256) || !approvalText(value.ToolVersion, 256, true, false) || strings.TrimSpace(value.ToolVersion) != value.ToolVersion {
		return invalid
	}
	if value.State == "ready" {
		if value.ErrorCode != "" || value.Catalogue == nil || ValidateCatalog(*value.Catalogue) != nil || value.Catalogue.ConfigurationSHA256 != value.ConfigurationSHA256 {
			return invalid
		}
		return nil
	}
	if value.State != "failed" && value.State != "unknown" || value.Catalogue != nil {
		return invalid
	}
	switch value.ErrorCode {
	case "codex_discovery_invalid", "codex_discovery_unknown", "codex_discovery_installation_changed", "codex_discovery_credential_unavailable", "codex_discovery_catalogue_unavailable", "codex_discovery_not_stopped", "codex_discovery_failed":
		return nil
	default:
		return invalid
	}
}

// UnmarshalJSON rejects duplicate/unknown fields, null and missing required
// values before interpreting a child-process result. Optional nested catalogs
// still undergo their complete original semantic and digest validation.
func (value *CodexModelDiscoveryResult) UnmarshalJSON(raw []byte) error {
	type wire CodexModelDiscoveryResult
	var decoded wire
	if strictjson.Decode(raw, &decoded, 256<<10) != nil || ValidateModelDiscoveryResult(CodexModelDiscoveryResult(decoded)) != nil {
		return fmt.Errorf("invalid Codex model discovery JSON")
	}
	*value = CodexModelDiscoveryResult(decoded)
	return nil
}
