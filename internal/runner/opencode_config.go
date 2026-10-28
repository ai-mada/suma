package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// OpenCodeSpec is administrator-owned. The immutable node carries only its
// public configuration acknowledgement; it cannot supply process settings.
type OpenCodeSpec struct {
	Executable   string `json:"executable"`
	ProviderID   string `json:"provider_id"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
	APIKeyEnvRef string `json:"api_key_env_ref,omitempty"`
}

var openCodeProviderID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var openCodeLocalModel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,255}$`)
var openCodeCredentialRef = regexp.MustCompile(`^[A-Z_][A-Z0-9_]{0,127}$`)

func validateOpenCodeSpec(spec CommandSpec) error {
	if spec.OpenCode == nil && spec.Adapter != "opencode" && spec.Protocol != tc.OpenCodeSDKProtocol {
		return nil
	}
	p := spec.OpenCode
	if p == nil || spec.Adapter != "opencode" || spec.Protocol != tc.OpenCodeSDKProtocol || spec.Input != "opencode-sdk-jsonl" || spec.ReadOnly || spec.NativeWorkspace == nil || spec.NativeWorkspace.ConflictRepair || spec.ModelExecutionProtocol != "" || spec.CodexAppServer != nil || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.CandidateComposition || spec.TestEvidence || spec.ArtifactInput || spec.ArtifactOutput != "" || len(spec.Args) != 1 || !filepath.IsAbs(spec.Args[0]) || strings.ToLower(filepath.Ext(spec.Args[0])) != ".mjs" || !filepath.IsAbs(p.Executable) || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 {
		return fmt.Errorf("OpenCode SDK requires one fixed bundled entry, its local configuration and the isolated host workspace")
	}
	u, err := url.Parse(p.BaseURL)
	if !openCodeProviderID.MatchString(p.ProviderID) || !openCodeLocalModel.MatchString(p.Model) || len(p.BaseURL) > 2048 || err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(p.BaseURL, "\x00\r\n") || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return fmt.Errorf("OpenCode local provider and model configuration is invalid")
	}
	ref := p.APIKeyEnvRef
	if ref != "" && (!openCodeCredentialRef.MatchString(ref) || strings.HasPrefix(ref, "SUMA_") || strings.HasPrefix(ref, "OPENCODE") || strings.HasPrefix(ref, "NODE_") || strings.HasPrefix(ref, "NPM_") || map[string]bool{"HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true, "PATH": true, "SYSTEMROOT": true, "WINDIR": true, "PROGRAMDATA": true, "TEMP": true, "TMP": true, "TMPDIR": true}[ref]) {
		return fmt.Errorf("OpenCode credential must use a separate local environment reference")
	}
	return nil
}

func OpenCodeConfigurationDigest(spec OpenCodeSpec) string {
	raw, _ := json.Marshal(struct {
		PolicyVersion int          `json:"policy_version"`
		SDKVersion    string       `json:"sdk_version"`
		Provider      OpenCodeSpec `json:"provider"`
	}{1, tc.OpenCodeSDKVersion, spec})
	return digest(raw)
}

func localOpenCodeCapability(spec CommandSpec) (*tc.OpenCodeExecutionCapability, error) {
	if spec.OpenCode == nil && spec.Adapter != "opencode" && spec.Protocol != tc.OpenCodeSDKProtocol {
		return nil, nil
	}
	if err := validateOpenCodeSpec(spec); err != nil {
		return nil, err
	}
	p := spec.OpenCode
	u, _ := url.Parse(p.BaseURL)
	c := &tc.OpenCodeExecutionCapability{SDKVersion: tc.OpenCodeSDKVersion, ServerVersion: tc.OpenCodeSDKVersion, ProviderID: p.ProviderID, Model: p.Model, EndpointOrigin: u.Scheme + "://" + u.Host, ConfigurationSHA256: OpenCodeConfigurationDigest(*p), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only"}
	if err := tc.ValidateOpenCodeExecutionCapability(*c); err != nil {
		return nil, err
	}
	return c, nil
}

func openCodeExecutionRequest(a Assignment, spec CommandSpec, profile tc.Profile) error {
	_, present := a.Config["opencode_execution"]
	if !present && spec.OpenCode == nil {
		return nil
	}
	if !present || spec.OpenCode == nil || a.Type != "agent" || profile.OpenCodeExecution == nil || !tc.SupportsOpenCodeExecutionProfile(profile) {
		return fmt.Errorf("OpenCode execution requires the exact frozen fixed configuration without native limits or provider overrides")
	}
	binding, err := tc.DecodeOpenCodeNodeConfiguration(a.Config)
	if err != nil || binding == nil || *binding != *profile.OpenCodeExecution {
		return fmt.Errorf("OpenCode execution differs from its frozen profile configuration")
	}
	local, err := localOpenCodeCapability(spec)
	if err != nil || local == nil || *binding != *local {
		return fmt.Errorf("OpenCode execution differs from current local configuration")
	}
	return nil
}

func openCodeExecutable(spec CommandSpec) (string, string, error) {
	if spec.OpenCode == nil || !filepath.IsAbs(spec.OpenCode.Executable) {
		return "", "", fmt.Errorf("OpenCode executable requires an absolute administrator path")
	}
	path := filepath.Clean(spec.OpenCode.Executable)
	st, err := os.Lstat(path)
	resolved, resolveErr := filepath.EvalSymlinks(path)
	if err != nil || resolveErr != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || !strings.EqualFold(filepath.Clean(resolved), path) {
		return "", "", fmt.Errorf("OpenCode executable is unavailable or linked")
	}
	return commandExecutable(CommandSpec{Executable: path})
}

type openCodeRuntimeConfig struct {
	Executable          string `json:"executable"`
	ExecutableSHA256    string `json:"executable_sha256"`
	Home                string `json:"home"`
	Source              string `json:"source"`
	ProviderID          string `json:"provider_id"`
	Model               string `json:"model"`
	BaseURL             string `json:"base_url"`
	APIKeyEnvRef        string `json:"api_key_env_ref,omitempty"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	ToolEntry           string `json:"tool_entry"`
	PluginPackageRoot   string `json:"plugin_package_root"`
}

func openCodeEnvironment(spec CommandSpec, home, source string, credential bool) ([]string, []string, error) {
	if err := validateOpenCodeSpec(spec); err != nil {
		return nil, nil, err
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(source) || within(source, home) {
		return nil, nil, fmt.Errorf("OpenCode home must be outside the attempt source")
	}
	if err := safeMkdir(home); err != nil {
		return nil, nil, fmt.Errorf("OpenCode isolated home is unavailable")
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		return nil, nil, fmt.Errorf("OpenCode requires a fresh empty isolated home")
	}
	executable, sha, err := openCodeExecutable(spec)
	if err != nil {
		return nil, nil, err
	}
	tool, plugin, err := openCodeRuntimePaths(spec)
	if err != nil {
		return nil, nil, err
	}
	config := openCodeRuntimeConfig{Executable: executable, ExecutableSHA256: sha, Home: home, Source: source, ProviderID: spec.OpenCode.ProviderID, Model: spec.OpenCode.Model, BaseURL: spec.OpenCode.BaseURL, ConfigurationSHA256: OpenCodeConfigurationDigest(*spec.OpenCode), ToolEntry: tool, PluginPackageRoot: plugin}
	// Ordinary OS process mechanics are enough to start Node and the private
	// server. Global homes, provider/npm/proxy credentials and startup hooks
	// never enter the wrapper's environment.
	base, _, err := commandEnvironment(CommandSpec{})
	if err != nil {
		return nil, nil, err
	}
	environment := []string{}
	for _, value := range base {
		name, _, _ := strings.Cut(value, "=")
		if map[string]bool{"PATH": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "PROGRAMDATA": true, "LANG": true}[strings.ToUpper(name)] {
			environment = append(environment, value)
		}
	}
	environment = append(environment, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home)
	secrets := []string{}
	if credential && spec.OpenCode.APIKeyEnvRef != "" {
		ref := spec.OpenCode.APIKeyEnvRef
		value, ok := os.LookupEnv(ref)
		if !ok || value == "" || len(value) > 16384 || strings.ContainsAny(value, "\x00\r\n") {
			return nil, nil, fmt.Errorf("OpenCode credential environment reference is unavailable")
		}
		config.APIKeyEnvRef = ref
		environment = append(environment, ref+"="+value)
		secrets = append(secrets, value)
	}
	raw, _ := json.Marshal(config)
	environment = append(environment, "SUMA_OPENCODE_CONFIG="+string(raw))
	return environment, secrets, nil
}

func probeOpenCode(ctx context.Context, executable string, spec CommandSpec, secrets []string, workspace string) (string, error) {
	if err := validateOpenCodeSpec(spec); err != nil {
		return "", err
	}
	root, err := realDirectory(workspace)
	if err != nil {
		return "", fmt.Errorf("OpenCode probe workspace is unavailable")
	}
	parent := filepath.Join(root, ".cache", "runner", "opencode-probes")
	if err = safeMkdir(parent); err != nil {
		return "", fmt.Errorf("OpenCode probe directory is unavailable")
	}
	directory, err := os.MkdirTemp(parent, "probe-")
	if err != nil {
		return "", fmt.Errorf("OpenCode probe directory is unavailable")
	}
	// Only the generated task child is disposable. Never follow an exchanged
	// link or broaden cleanup to the configured workspace/cache parent.
	defer func() {
		if resolved, e := realDirectory(directory); e == nil && resolved == directory && within(parent, directory) {
			_ = os.RemoveAll(directory)
		}
	}()
	home, source := filepath.Join(directory, "home"), filepath.Join(directory, "source")
	if err = safeMkdir(source); err != nil {
		return "", err
	}
	environment, _, err := openCodeEnvironment(spec, home, source, false)
	if err != nil {
		return "", err
	}
	probeSpec := CommandSpec{Env: map[string]string{}}
	for _, item := range environment {
		name, value, _ := strings.Cut(item, "=")
		probeSpec.Env[name] = value
	}
	result, err := probeCommandStreams(ctx, executable, append(append([]string{}, spec.Args...), "--probe"), probeSpec, secrets, true)
	if err != nil {
		return "", fmt.Errorf("OpenCode SDK read-only capability probe failed")
	}
	raw := bytes.TrimSpace([]byte(result))
	values, err := decodeToolObject(raw)
	if err != nil || !closedOpenCodeFrame(values, "protocol", "runtime_version", "sdk_version", "server_version", "configuration_sha256", "provider_id", "model", "workspace_tools_version") || len(values) != 10 {
		return "", fmt.Errorf("OpenCode SDK probe is invalid")
	}
	c, _ := localOpenCodeCapability(spec)
	v, valid := nativeInteger(values, "v")
	if !valid || v != 1 || textAt(values, "type") != "probe" || textAt(values, "protocol") != tc.OpenCodeSDKProtocol || textAt(values, "runtime_version") != "1.0.0" || textAt(values, "sdk_version") != c.SDKVersion || textAt(values, "server_version") != c.ServerVersion || textAt(values, "configuration_sha256") != c.ConfigurationSHA256 || textAt(values, "provider_id") != c.ProviderID || textAt(values, "model") != c.Model || textAt(values, "workspace_tools_version") != NativeWorkspaceVersion {
		return "", fmt.Errorf("OpenCode SDK probe differs from the fixed local configuration")
	}
	return "suma-opencode-runtime 1.0.0", nil
}
