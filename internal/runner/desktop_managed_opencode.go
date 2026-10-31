package runner

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Main-process-owned paths and byte pins. The renderer supplies neither these
// process settings nor a replacement SDK entry or workspace command policy.
type DesktopManagedOpenCodeConfiguration struct {
	Executable            string `json:"executable"`
	ExecutableSHA256      string `json:"executable_sha256"`
	ReviewedVersion       string `json:"reviewed_version"`
	NodeExecutable        string `json:"node_executable"`
	NodeSHA256            string `json:"node_sha256"`
	RuntimeManifest       string `json:"runtime_manifest"`
	RuntimeManifestSHA256 string `json:"runtime_manifest_sha256"`
	ProviderID            string `json:"provider_id"`
	Model                 string `json:"model"`
	BaseURL               string `json:"base_url"`
	APIKeyEnvRef          string `json:"api_key_env_ref,omitempty"`
	ConfigurationSHA256   string `json:"configuration_sha256"`
}

func (value *DesktopManagedOpenCodeConfiguration) UnmarshalJSON(raw []byte) error {
	// Preserve the original nested bytes: re-marshalling a generic decoded map
	// would erase invalid surrogate evidence before the closed DTO sees it.
	type wire DesktopManagedOpenCodeConfiguration
	var decoded wire
	if strictjson.Decode(raw, &decoded, DesktopManagedInputLimit) != nil || !validManagedOpenCodeConfiguration((*DesktopManagedOpenCodeConfiguration)(&decoded)) {
		return fmt.Errorf("invalid private desktop OpenCode configuration")
	}
	*value = DesktopManagedOpenCodeConfiguration(decoded)
	return nil
}

func managedOpenCodeCommand(value *DesktopManagedOpenCodeConfiguration) CommandSpec {
	return CommandSpec{Adapter: "opencode", Protocol: tc.OpenCodeSDKProtocol, Executable: value.NodeExecutable, Args: []string{filepath.Join(filepath.Dir(value.RuntimeManifest), "suma-opencode.bundle.mjs")}, Input: "opencode-sdk-jsonl", NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{}, RuntimeManifest: value.RuntimeManifest}, OpenCode: &OpenCodeSpec{Executable: value.Executable, ProviderID: value.ProviderID, Model: value.Model, BaseURL: value.BaseURL, APIKeyEnvRef: value.APIKeyEnvRef}}
}

func validManagedOpenCodeConfiguration(value *DesktopManagedOpenCodeConfiguration) bool {
	if value == nil || value.ReviewedVersion != tc.OpenCodeSDKVersion || !desktopManagedPath(value.Executable) || !desktopManagedPath(value.NodeExecutable) || !desktopManagedPath(value.RuntimeManifest) || filepath.Base(value.RuntimeManifest) != "suma-opencode.manifest.json" || !tc.ValidDigest(value.ExecutableSHA256) || !tc.ValidDigest(value.NodeSHA256) || !tc.ValidDigest(value.RuntimeManifestSHA256) || !tc.ValidDigest(value.ConfigurationSHA256) {
		return false
	}
	spec := managedOpenCodeCommand(value)
	return validateOpenCodeSpec(spec) == nil && OpenCodeConfigurationDigest(*spec.OpenCode) == value.ConfigurationSHA256
}

func decodeManagedOpenCodeConfiguration(raw []byte) (*DesktopManagedOpenCodeConfiguration, error) {
	var value DesktopManagedOpenCodeConfiguration
	if strictjson.Decode(raw, &value, DesktopManagedInputLimit) != nil || !validManagedOpenCodeConfiguration(&value) {
		return nil, fmt.Errorf("invalid private desktop OpenCode configuration")
	}
	return &value, nil
}

func prepareManagedOpenCode(ctx context.Context, value *DesktopManagedOpenCodeConfiguration) (CommandSpec, error) {
	invalid := fmt.Errorf("desktop OpenCode installation or SDK resources differ from their confirmed configuration")
	if !validManagedOpenCodeConfiguration(value) {
		return CommandSpec{}, invalid
	}
	spec := managedOpenCodeCommand(value)
	// Resolve only an already selected local native target. A package wrapper is
	// not an executable and no global login or package installation is consulted.
	exe, sha, err := qoderNativeTarget(ctx, value.Executable)
	if err != nil || !sameACPPath(exe, value.Executable) || sha != value.ExecutableSHA256 {
		return CommandSpec{}, invalid
	}
	node, nodeSHA, err := qoderNativeTarget(ctx, value.NodeExecutable)
	if err != nil || !sameACPPath(node, value.NodeExecutable) || nodeSHA != value.NodeSHA256 {
		return CommandSpec{}, invalid
	}
	manifestSHA, err := nativeManifestDigest(spec)
	if err != nil || manifestSHA != value.RuntimeManifestSHA256 {
		return CommandSpec{}, invalid
	}
	version, err := passiveOpenCodeVersion(ctx, exe)
	if err != nil || version != value.ReviewedVersion || !sameACPProbeInstallation(ctx, exe, sha) || !sameACPProbeInstallation(ctx, node, nodeSHA) {
		return CommandSpec{}, invalid
	}
	// The probe can race an SDK replacement, so close the same complete asset
	// manifest again before the original Worker may publish fresh inventory.
	if current, err := nativeManifestDigest(spec); err != nil || current != manifestSHA {
		return CommandSpec{}, invalid
	}
	return spec, nil
}
