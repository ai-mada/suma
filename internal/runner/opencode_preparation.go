package runner

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const OpenCodeToolPreparationInputLimit = 64 << 10
const OpenCodeCommandID = "opencode-coder"

// OpenCodeToolPreparationInput is a closed, local administrator stdin contract.
// It names already reviewed files and a fixed provider/model. Credentials are
// references only; even an empty reference must be an explicit input choice.
type OpenCodeToolPreparationInput struct {
	AdapterID           string `json:"adapter_id"`
	NodeExecutable      string `json:"node_executable"`
	RuntimeManifest     string `json:"runtime_manifest"`
	NativeCLIExecutable string `json:"native_cli_executable"`
	ProviderID          string `json:"provider_id"`
	Model               string `json:"model"`
	BaseURL             string `json:"base_url"`
	APIKeyEnvRef        string `json:"api_key_env_ref"`
	ReviewedVersion     string `json:"reviewed_version"`
}

func (input OpenCodeToolPreparationInput) command() CommandSpec {
	// The mature managed constructor owns the fixed bundle name and protocol.
	// A newly created single-command file cannot reference an absent check ID.
	return managedOpenCodeCommand(&DesktopManagedOpenCodeConfiguration{
		Executable: input.NativeCLIExecutable, NodeExecutable: input.NodeExecutable,
		RuntimeManifest: input.RuntimeManifest, ProviderID: input.ProviderID,
		Model: input.Model, BaseURL: input.BaseURL, APIKeyEnvRef: input.APIKeyEnvRef,
	})
}

func DecodeOpenCodeToolPreparationInput(raw []byte) (OpenCodeToolPreparationInput, error) {
	var input OpenCodeToolPreparationInput
	invalid := fmt.Errorf("invalid private OpenCode SDK preparation input")
	if strictjson.Decode(raw, &input, OpenCodeToolPreparationInputLimit) != nil ||
		input.AdapterID != "opencode-sdk" || input.ReviewedVersion != tc.OpenCodeSDKVersion ||
		!desktopManagedPath(input.NodeExecutable) || !desktopManagedPath(input.RuntimeManifest) ||
		!desktopManagedPath(input.NativeCLIExecutable) || filepath.Base(input.RuntimeManifest) != "suma-opencode.manifest.json" ||
		validateOpenCodeSpec(input.command()) != nil {
		return OpenCodeToolPreparationInput{}, invalid
	}
	return input, nil
}

// PrepareOpenCodeToolCommands prepares bytes only: it never launches Node,
// OpenCode, a version probe, a model, or a Worker. ReviewedVersion selects the
// fixed SDK baseline, not an observation of the native CLI's actual version.
// Actual runtime/version/configuration acknowledgement belongs to inventory.
func PrepareOpenCodeToolCommands(ctx context.Context, destination string, input io.Reader) (ToolPreparation, error) {
	result := ToolPreparation{Protocol: ToolPreparationProtocol, Remaining: []UnpreparedTool{}}
	file, parent, err := openToolCommandDestination(destination)
	result.File = file
	if err != nil {
		return result, err
	}
	defer parent.Close()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if input == nil {
		return result, fmt.Errorf("private OpenCode preparation input is required")
	}
	raw, err := io.ReadAll(io.LimitReader(input, OpenCodeToolPreparationInputLimit+1))
	if err != nil {
		return result, fmt.Errorf("private OpenCode preparation input unavailable")
	}
	selected, err := DecodeOpenCodeToolPreparationInput(raw)
	if err != nil {
		return result, err
	}
	packageRoot := filepath.Dir(selected.RuntimeManifest)
	if err = toolPackagePreparationDestination(file, parent, packageRoot); err != nil {
		return result, err
	}
	node, nodeSHA, err := qoderNativeTarget(ctx, selected.NodeExecutable)
	if err != nil {
		return result, fmt.Errorf("selected Node executable unavailable")
	}
	native, nativeSHA, err := qoderNativeTarget(ctx, selected.NativeCLIExecutable)
	if err != nil {
		return result, fmt.Errorf("selected native OpenCode executable unavailable")
	}
	selected.NodeExecutable, selected.NativeCLIExecutable = node, native
	spec := selected.command()
	manifestSHA, err := nativeManifestDigest(spec)
	if err != nil {
		return result, fmt.Errorf("reviewed OpenCode runtime assets unavailable or changed")
	}
	// Reuse the original complete private binding validation without the
	// connection-only passive version process or any credential value lookup.
	bound := &DesktopManagedOpenCodeConfiguration{
		Executable: selected.NativeCLIExecutable, ExecutableSHA256: nativeSHA,
		NodeExecutable: selected.NodeExecutable, NodeSHA256: nodeSHA,
		RuntimeManifest: selected.RuntimeManifest, RuntimeManifestSHA256: manifestSHA,
		ReviewedVersion: selected.ReviewedVersion, ProviderID: selected.ProviderID,
		Model: selected.Model, BaseURL: selected.BaseURL, APIKeyEnvRef: selected.APIKeyEnvRef,
		ConfigurationSHA256: OpenCodeConfigurationDigest(*spec.OpenCode),
	}
	if !validManagedOpenCodeConfiguration(bound) || !sameACPProbeInstallation(ctx, selected.NodeExecutable, nodeSHA) || !sameACPProbeInstallation(ctx, selected.NativeCLIExecutable, nativeSHA) {
		return result, fmt.Errorf("selected OpenCode preparation files changed")
	}
	if current, e := nativeManifestDigest(spec); e != nil || current != manifestSHA {
		return result, fmt.Errorf("reviewed OpenCode runtime assets changed")
	}
	if err = toolPackagePreparationDestination(file, parent, packageRoot); err != nil {
		return result, err
	}
	if err = writePreparedToolCommands(ctx, parent, filepath.Base(file), map[string]CommandSpec{OpenCodeCommandID: spec}); err != nil {
		return result, err
	}
	result.Prepared, result.CommandCount = true, 1
	return result, nil
}
