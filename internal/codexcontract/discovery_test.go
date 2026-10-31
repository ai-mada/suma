package codexcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCodexDiscoveryCatalogueIsBoundToItsObservedConfiguration(t *testing.T) {
	configuration := strings.Repeat("a", 64)
	catalogue := Catalog{Protocol: Protocol, ConfigurationSHA256: configuration, Models: []ModelOption{{Model: "observed-model", ReasoningEfforts: []string{"high"}, DefaultReasoningEffort: "high"}}}
	catalogue.CatalogSHA256 = CatalogDigest(catalogue)
	result := CodexModelDiscoveryResult{Protocol: ModelDiscoveryProtocol, State: "ready", ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "codex-cli observed-version", ConfigurationSHA256: configuration, Catalogue: &catalogue}
	raw, err := json.Marshal(result)
	var decoded CodexModelDiscoveryResult
	if err != nil || json.Unmarshal(raw, &decoded) != nil {
		t.Fatal("the exact observed catalogue could not be projected")
	}
	result.ConfigurationSHA256 = strings.Repeat("c", 64)
	if ValidateModelDiscoveryResult(result) == nil {
		t.Fatal("a catalogue from another provider configuration was accepted")
	}
	result.ConfigurationSHA256, result.State, result.ErrorCode = configuration, "unknown", "codex_discovery_unknown"
	if ValidateModelDiscoveryResult(result) == nil {
		t.Fatal("an uncertain probe retained a usable catalogue")
	}
	result.Catalogue = nil
	if ValidateModelDiscoveryResult(result) != nil {
		t.Fatal("the uncertain observation could not be retained without a catalogue")
	}
}
