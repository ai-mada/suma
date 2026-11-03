package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/strictjson"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const CodexModelDiscoveryInputLimit = 16 << 10

// These private stdin inputs belong to the desktop main process. They carry no
// task model, user login, arbitrary arguments or platform machine credential.
type CodexModelDiscoveryInput struct {
	AdapterID        string `json:"adapter_id"`
	Executable       string `json:"executable"`
	ExecutableSHA256 string `json:"executable_sha256"`
	ReviewedVersion  string `json:"reviewed_version"`
	ProviderID       string `json:"provider_id"`
	BaseURL          string `json:"base_url"`
	APIKeyEnvRef     string `json:"api_key_env_ref,omitempty"`
	WindowsSandbox   string `json:"windows_sandbox,omitempty"`
	EnableUserInput  bool   `json:"enable_user_input,omitempty"`
}

type DesktopManagedCodexConfiguration struct {
	Executable          string `json:"executable"`
	ExecutableSHA256    string `json:"executable_sha256"`
	ReviewedVersion     string `json:"reviewed_version"`
	ProviderID          string `json:"provider_id"`
	BaseURL             string `json:"base_url"`
	APIKeyEnvRef        string `json:"api_key_env_ref,omitempty"`
	WindowsSandbox      string `json:"windows_sandbox,omitempty"`
	EnableUserInput     bool   `json:"enable_user_input,omitempty"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
}

func codexDiscoveryCommand(input CodexModelDiscoveryInput) CommandSpec {
	return CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: input.Executable, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{ProviderID: input.ProviderID, BaseURL: input.BaseURL, APIKeyEnvRef: input.APIKeyEnvRef, WindowsSandbox: input.WindowsSandbox, EnableUserInput: input.EnableUserInput}}
}
func validCodexDiscoveryInput(input CodexModelDiscoveryInput) bool {
	return input.AdapterID == "codex-app-server" && desktopManagedPath(input.Executable) && tc.ValidDigest(input.ExecutableSHA256) && localToolSafeText(input.ReviewedVersion, 256) && strings.TrimSpace(input.ReviewedVersion) == input.ReviewedVersion && validateCodexAppServerSpec(codexDiscoveryCommand(input)) == nil
}
func DecodeCodexModelDiscoveryInput(raw []byte) (CodexModelDiscoveryInput, error) {
	var input CodexModelDiscoveryInput
	if strictjson.Decode(raw, &input, CodexModelDiscoveryInputLimit) != nil || !validCodexDiscoveryInput(input) {
		return CodexModelDiscoveryInput{}, fmt.Errorf("invalid private Codex model discovery input")
	}
	return input, nil
}
func managedCodexDiscoveryInput(value *DesktopManagedCodexConfiguration) CodexModelDiscoveryInput {
	return CodexModelDiscoveryInput{AdapterID: "codex-app-server", Executable: value.Executable, ExecutableSHA256: value.ExecutableSHA256, ReviewedVersion: value.ReviewedVersion, ProviderID: value.ProviderID, BaseURL: value.BaseURL, APIKeyEnvRef: value.APIKeyEnvRef, WindowsSandbox: value.WindowsSandbox, EnableUserInput: value.EnableUserInput}
}
func validManagedCodexConfiguration(value *DesktopManagedCodexConfiguration) bool {
	if value == nil {
		return false
	}
	input := managedCodexDiscoveryInput(value)
	return validCodexDiscoveryInput(input) && tc.ValidDigest(value.ConfigurationSHA256) && value.ConfigurationSHA256 == CodexConfigurationDigest(*codexDiscoveryCommand(input).CodexAppServer)
}
func decodeManagedCodexConfiguration(raw []byte) (*DesktopManagedCodexConfiguration, error) {
	var value DesktopManagedCodexConfiguration
	if strictjson.Decode(raw, &value, DesktopManagedInputLimit) != nil || !validManagedCodexConfiguration(&value) {
		return nil, fmt.Errorf("invalid private desktop Codex configuration")
	}
	return &value, nil
}

// The existing native target resolver verifies local file format, resolves a
// versioned entry and hashes its actual bytes. A shell wrapper is never run.
func checkedCodexInstallation(ctx context.Context, input CodexModelDiscoveryInput, directory string, secrets []string) (string, error) {
	exe, sha, err := qoderNativeTarget(ctx, input.Executable)
	if err != nil || !sameACPPath(exe, input.Executable) || sha != input.ExecutableSHA256 {
		return "", fmt.Errorf("Codex installation differs from its reviewed native bytes")
	}
	version, err := probeCodexVersion(ctx, exe, codexDiscoveryCommand(input), secrets, directory)
	if err != nil || version != input.ReviewedVersion {
		return "", fmt.Errorf("Codex installation differs from its reviewed version")
	}
	if !sameACPProbeInstallation(ctx, exe, sha) {
		return "", fmt.Errorf("Codex installation changed during verification")
	}
	return exe, nil
}

// Probe only version/help with an isolated HOME. No provider configuration is
// adopted or written here; the spec is solely the existing probe's fixed policy.
func passiveCodexVersion(ctx context.Context, exe string) (string, error) {
	directory, cleanup, err := newACPProbeDirectory()
	if err != nil {
		return "", err
	}
	ctx, closeScope, err := bindPrivateProbeDirectory(ctx, directory, cleanup)
	if err != nil {
		return "", err
	}
	defer closeScope()
	input := CodexModelDiscoveryInput{Executable: exe, ProviderID: "suma-install-probe", BaseURL: "http://127.0.0.1:1/v1", WindowsSandbox: "unelevated"}
	return probeCodexVersion(ctx, exe, codexDiscoveryCommand(input), nil, directory)
}

func discoverLocalCodex(ctx context.Context, entry localToolDirectoryEntry, paths []string, probe func(context.Context, CommandSpec) (string, error)) InstalledToolDiscoveryItem {
	item := InstalledToolDiscoveryItem{AdapterID: "codex-app-server", Product: "Codex", ReasonCode: "not_installed"}
	spec, _, installed, reason, diagnostic := resolveLocalToolObserved(entry, paths)
	item.Installed, item.ReasonCode = installed, reason
	item.Diagnostic = diagnostic
	if reason != "resolved" {
		return item
	}
	exe, sha, err := qoderNativeTarget(ctx, spec.Executable)
	if err != nil {
		item.ReasonCode = "unsupported_launcher"
		item.Diagnostic = probeDiagnostic(newProbeFailure("source", "source_unavailable", err))
		return item
	}
	spec.Executable = exe
	version, err := probe(ctx, spec)
	if err != nil || !localToolSafeText(version, 256) || strings.TrimSpace(version) != version {
		item.ReasonCode = "probe_failed"
		if err == nil {
			err = newProbeFailure("version", "probe_failed", nil)
		}
		item.Diagnostic = probeDiagnostic(err)
		return item
	}
	if !sameACPProbeInstallation(ctx, exe, sha) {
		item.ReasonCode = "probe_failed"
		item.Diagnostic = probeDiagnostic(newProbeFailure("source", "source_changed", nil))
		return item
	}
	item.Executable, item.ExecutableSHA256, item.Version, item.ReasonCode = exe, sha, version, "configuration_required"
	return item // No Command or LaunchSupported before explicit provider setup.
}

func prepareManagedCodex(ctx context.Context, value *DesktopManagedCodexConfiguration) (CommandSpec, error) {
	if !validManagedCodexConfiguration(value) {
		return CommandSpec{}, fmt.Errorf("invalid desktop Codex configuration")
	}
	directory, cleanup, err := newACPProbeDirectory()
	if err != nil {
		return CommandSpec{}, err
	}
	ctx, closeScope, err := bindPrivateProbeDirectory(ctx, directory, cleanup)
	if err != nil {
		return CommandSpec{}, err
	}
	defer closeScope()
	input := managedCodexDiscoveryInput(value)
	exe, err := checkedCodexInstallation(ctx, input, directory, nil)
	if err != nil {
		return CommandSpec{}, err
	}
	spec := codexDiscoveryCommand(input)
	spec.Executable = exe
	return spec, nil
}

// Explicit catalogue discovery reuses the real initialize/model-list probe.
// It opens neither a Journal nor a Worker, repository, thread or turn.
func DiscoverCodexModels(parent context.Context, input CodexModelDiscoveryInput) (cc.CodexModelDiscoveryResult, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return cc.CodexModelDiscoveryResult{}, fmt.Errorf("invalid private Codex model discovery input")
	}
	if _, err = DecodeCodexModelDiscoveryInput(raw); err != nil {
		return cc.CodexModelDiscoveryResult{}, err
	}
	spec := codexDiscoveryCommand(input)
	result := cc.CodexModelDiscoveryResult{Protocol: cc.ModelDiscoveryProtocol, State: "unknown", ExecutableSHA256: input.ExecutableSHA256, ToolVersion: input.ReviewedVersion, ConfigurationSHA256: CodexConfigurationDigest(*spec.CodexAppServer), ErrorCode: "codex_discovery_unknown"}
	finish := func() (cc.CodexModelDiscoveryResult, error) {
		if cc.ValidateModelDiscoveryResult(result) != nil {
			return cc.CodexModelDiscoveryResult{}, fmt.Errorf("Codex discovery result unavailable")
		}
		return result, nil
	}
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return finish()
	}
	secrets := []string{}
	if input.APIKeyEnvRef != "" {
		value, present := os.LookupEnv(input.APIKeyEnvRef)
		if !present || value == "" || strings.ContainsAny(value, "\x00\r\n") {
			result.State, result.ErrorCode = "failed", "codex_discovery_credential_unavailable"
			return finish()
		}
		secrets = append(secrets, value)
	}
	if progressText(input.ReviewedVersion, secrets) != input.ReviewedVersion {
		return cc.CodexModelDiscoveryResult{}, fmt.Errorf("Codex discovery version is unavailable")
	}
	directory, cleanup, err := newACPProbeDirectory()
	if err != nil {
		result.State, result.ErrorCode = "failed", "codex_discovery_failed"
		return finish()
	}
	ctx, closeScope, err := bindPrivateProbeDirectory(ctx, directory, cleanup)
	if err != nil {
		result.State, result.ErrorCode = "failed", "codex_discovery_failed"
		return finish()
	}
	defer closeScope()
	exe, err := checkedCodexInstallation(ctx, input, directory, secrets)
	if err != nil {
		if ctx.Err() == nil {
			result.State, result.ErrorCode = "failed", "codex_discovery_installation_changed"
		}
		return finish()
	}
	catalogue, err := probeCodexCatalog(ctx, exe, spec, ExecConfig{Workspace: directory, Secrets: secrets})
	if err != nil {
		if ctx.Err() == nil {
			result.State, result.ErrorCode = "failed", "codex_discovery_catalogue_unavailable"
		}
		return finish()
	}
	if !sameACPProbeInstallation(ctx, exe, input.ExecutableSHA256) {
		if ctx.Err() == nil {
			result.State, result.ErrorCode = "failed", "codex_discovery_installation_changed"
		}
		return finish()
	}
	result.State, result.ErrorCode = "ready", ""
	copy := cc.CloneCatalog(*catalogue)
	result.Catalogue = &copy
	return finish()
}
