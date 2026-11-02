package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Decode original nested bytes before generic maps can erase duplicate keys or
// malformed Unicode. This private DTO contains environment names, never values.
func (value *DesktopManagedClaudeSDKConfiguration) UnmarshalJSON(raw []byte) error {
	type wire DesktopManagedClaudeSDKConfiguration
	var decoded wire
	if strictjson.Decode(raw, &decoded, DesktopManagedInputLimit) != nil || !validManagedClaudeSDKConfiguration((*DesktopManagedClaudeSDKConfiguration)(&decoded)) {
		return fmt.Errorf("invalid private desktop Claude SDK configuration")
	}
	*value = DesktopManagedClaudeSDKConfiguration(decoded)
	return nil
}

func managedClaudeSDKCommand(value *DesktopManagedClaudeSDKConfiguration) CommandSpec {
	return CommandSpec{Adapter: "claude", Protocol: tc.ClaudeSDKProtocol, Executable: value.NodeExecutable, Args: []string{filepath.Join(filepath.Dir(value.RuntimeManifest), "suma-claude.bundle.mjs")}, Input: "claude-sdk-jsonl", ClaudeSDK: &ClaudeSDKSpec{NativeCLIExecutable: value.NativeCLIExecutable, Model: value.Model, CredentialEnvRef: value.CredentialEnvRef, ReviewedCLIVersion: value.ReviewedCLIVersion}}
}

// Match ClaudeSDKConfigurationDigest's sole canonical GoJSON shape using the
// parent-supplied byte pins. Actual files are independently rechecked below.
func managedClaudeSDKConfigurationDigest(value *DesktopManagedClaudeSDKConfiguration) string {
	raw, _ := json.Marshal(struct {
		PolicyVersion int           `json:"policy_version"`
		SDKVersion    string        `json:"sdk_version"`
		CLI           ClaudeSDKSpec `json:"cli"`
		NodeSHA       string        `json:"node_sha256"`
		NativeSHA     string        `json:"native_cli_sha256"`
		ManifestSHA   string        `json:"manifest_sha256"`
	}{1, tc.ClaudeSDKVersion, *managedClaudeSDKCommand(value).ClaudeSDK, value.NodeSHA256, value.NativeCLISHA256, value.RuntimeManifestSHA256})
	return digest(raw)
}

func validManagedClaudeSDKConfiguration(value *DesktopManagedClaudeSDKConfiguration) bool {
	if value == nil || value.ReviewedCLIVersion != tc.ClaudeSDKCLIVersion || !desktopManagedPath(value.NativeCLIExecutable) || !desktopManagedPath(value.NodeExecutable) || !desktopManagedPath(value.RuntimeManifest) || filepath.Base(value.RuntimeManifest) != "suma-claude-runtime.json" || !tc.ValidDigest(value.NativeCLISHA256) || !tc.ValidDigest(value.NodeSHA256) || !tc.ValidDigest(value.RuntimeManifestSHA256) || !tc.ValidDigest(value.ConfigurationSHA256) {
		return false
	}
	return validateClaudeSDKSpec(managedClaudeSDKCommand(value)) == nil && managedClaudeSDKConfigurationDigest(value) == value.ConfigurationSHA256
}

func decodeManagedClaudeSDKConfiguration(raw []byte) (*DesktopManagedClaudeSDKConfiguration, error) {
	var value DesktopManagedClaudeSDKConfiguration
	if strictjson.Decode(raw, &value, DesktopManagedInputLimit) != nil || !validManagedClaudeSDKConfiguration(&value) {
		return nil, fmt.Errorf("invalid private desktop Claude SDK configuration")
	}
	return &value, nil
}

func prepareManagedClaudeSDK(ctx context.Context, value *DesktopManagedClaudeSDKConfiguration) (CommandSpec, error) {
	invalid := fmt.Errorf("desktop Claude SDK installation or resources differ from their confirmed configuration")
	if !validManagedClaudeSDKConfiguration(value) {
		return CommandSpec{}, invalid
	}
	// Inspect the selected real files, not a PATH replacement or launcher.
	native, nativeSHA, err := qoderNativeTarget(ctx, value.NativeCLIExecutable)
	if err != nil || !sameACPPath(native, value.NativeCLIExecutable) || nativeSHA != value.NativeCLISHA256 {
		return CommandSpec{}, invalid
	}
	node, nodeSHA, err := qoderNativeTarget(ctx, value.NodeExecutable)
	if err != nil || !sameACPPath(node, value.NodeExecutable) || nodeSHA != value.NodeSHA256 {
		return CommandSpec{}, invalid
	}
	local := managedClaudeSDKCommand(value)
	spec, err := BuildClaudeSDKCommand(node, local.Args[0], *local.ClaudeSDK)
	if err != nil {
		return CommandSpec{}, invalid
	}
	configurationSHA, err := ClaudeSDKConfigurationDigest(spec)
	manifestSHA, manifestErr := claudeSDKManifestDigest(spec)
	if err != nil || manifestErr != nil || configurationSHA != value.ConfigurationSHA256 || manifestSHA != value.RuntimeManifestSHA256 {
		return CommandSpec{}, invalid
	}
	// Credential availability is not account/model verification. Its value is
	// neither retained in configuration nor forwarded to this passive probe.
	credential, present := os.LookupEnv(value.CredentialEnvRef)
	if !present || credential == "" || len(credential) > 16384 || strings.ContainsAny(credential, "\x00\r\n") {
		return CommandSpec{}, fmt.Errorf("desktop Claude SDK credential environment reference is unavailable")
	}
	version, err := passiveClaudeSDKVersion(ctx, native)
	if err != nil || version != value.ReviewedCLIVersion && version != value.ReviewedCLIVersion+" (Claude Code)" || !sameACPProbeInstallation(ctx, native, nativeSHA) || !sameACPProbeInstallation(ctx, node, nodeSHA) {
		return CommandSpec{}, invalid
	}
	if current, err := ClaudeSDKConfigurationDigest(spec); err != nil || current != configurationSHA {
		return CommandSpec{}, invalid
	}
	return spec, nil
}
