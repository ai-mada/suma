package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// OctopSpec is fixed by the Runner administrator. Neither a model nor a DAG
// node can replace the provider, SDK entry, credentials or execution limits.
// Output limits are forwarded to the model; they are not a billing hard cap.
type OctopSpec struct {
	ProviderID      string `json:"provider_id"`
	Model           string `json:"model"`
	BaseURL         string `json:"base_url"`
	APIKeyEnvRef    string `json:"api_key_env_ref,omitempty"`
	MaxSteps        int    `json:"max_steps"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

func validateOctopSpec(spec CommandSpec) error {
	if spec.Octop == nil && spec.Adapter != "octop" && spec.Protocol != tc.OctopProtocol {
		return nil
	}
	p := spec.Octop
	if p == nil || spec.Adapter != "octop" || spec.Protocol != tc.OctopProtocol || spec.Input != "octop-acp-jsonl" || spec.ReadOnly || spec.NativeWorkspace == nil || spec.NativeWorkspace.ConflictRepair || spec.ModelExecutionProtocol != "" || spec.CodexAppServer != nil || spec.OpenCode != nil || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.CandidateComposition || spec.TestEvidence || spec.TestReport || spec.ArtifactInput || spec.ArtifactOutput != "" || len(spec.Args) != 1 || !filepath.IsAbs(spec.Args[0]) || strings.ToLower(filepath.Ext(spec.Args[0])) != ".pyz" || !filepath.IsAbs(spec.Executable) || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 {
		return fmt.Errorf("Octop Harness requires one fixed Python zipapp, local configuration and the isolated host workspace")
	}
	u, err := url.Parse(p.BaseURL)
	if !openCodeProviderID.MatchString(p.ProviderID) || !openCodeLocalModel.MatchString(p.Model) || p.MaxSteps < 1 || p.MaxSteps > 32 || p.MaxOutputTokens < 1 || p.MaxOutputTokens > 32768 || len(p.BaseURL) > 2048 || err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(p.BaseURL, "\x00\r\n") || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) {
		return fmt.Errorf("Octop local provider, model or limits are invalid")
	}
	ref := p.APIKeyEnvRef
	if ref != "" && (!openCodeCredentialRef.MatchString(ref) || strings.HasPrefix(ref, "SUMA_") || strings.HasPrefix(ref, "PYTHON") || strings.HasPrefix(ref, "UV_") || strings.HasPrefix(ref, "LANGCHAIN_") || strings.HasPrefix(ref, "LANGSMITH_") || strings.HasPrefix(ref, "HF_") || strings.HasPrefix(ref, "OTEL_") || map[string]bool{"HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true, "PATH": true, "SYSTEMROOT": true, "WINDIR": true, "PROGRAMDATA": true, "TEMP": true, "TMP": true, "TMPDIR": true, "OCTOP_HOME": true}[ref]) {
		return fmt.Errorf("Octop credentials require a separate local environment reference")
	}
	return nil
}

func OctopConfigurationDigest(spec OctopSpec) string {
	raw, _ := json.Marshal(struct {
		PolicyVersion int       `json:"policy_version"`
		Harness       string    `json:"harness_version"`
		ACP           string    `json:"acp_sdk_version"`
		Provider      OctopSpec `json:"provider"`
	}{1, tc.OctopHarnessVersion, tc.OctopACPSDKVersion, spec})
	return digest(raw)
}

func localOctopCapability(spec CommandSpec) (*tc.OctopExecutionCapability, error) {
	if spec.Octop == nil && spec.Adapter != "octop" && spec.Protocol != tc.OctopProtocol {
		return nil, nil
	}
	if err := validateOctopSpec(spec); err != nil {
		return nil, err
	}
	p := spec.Octop
	u, _ := url.Parse(p.BaseURL)
	c := &tc.OctopExecutionCapability{HarnessVersion: tc.OctopHarnessVersion, ACPSDKVersion: tc.OctopACPSDKVersion, ProviderID: p.ProviderID, Model: p.Model, EndpointOrigin: u.Scheme + "://" + u.Host, ConfigurationSHA256: OctopConfigurationDigest(*p), BudgetMode: "observe_only", IdentityEvidence: "configuration_ack_only"}
	return c, tc.ValidateOctopExecutionCapability(*c)
}

func octopExecutionRequest(a Assignment, spec CommandSpec, profile tc.Profile) error {
	_, present := a.Config["octop_execution"]
	if !present && spec.Octop == nil {
		return nil
	}
	if !present || spec.Octop == nil || a.Type != "agent" || !tc.SupportsOctopExecutionProfile(profile) {
		return fmt.Errorf("Octop execution requires its current fixed host-tool capability")
	}
	binding, err := tc.DecodeOctopNodeConfiguration(a.Config)
	if err != nil || binding == nil || *binding != *profile.OctopExecution {
		return fmt.Errorf("Octop execution differs from its frozen profile")
	}
	local, err := localOctopCapability(spec)
	if err != nil || local == nil || *local != *binding {
		return fmt.Errorf("Octop execution differs from the current local configuration")
	}
	return nil
}

// The Python SDK receives a fresh private state directory, never the source
// path or a personal Octop database. Source access is only the six MCP tools.
func octopEnvironment(spec CommandSpec, home string, credential bool) ([]string, []string, error) {
	if err := validateOctopSpec(spec); err != nil {
		return nil, nil, err
	}
	if !filepath.IsAbs(home) {
		return nil, nil, fmt.Errorf("Octop requires an absolute private state directory")
	}
	if err := safeMkdir(home); err != nil {
		return nil, nil, err
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		return nil, nil, fmt.Errorf("Octop requires a fresh empty private state directory")
	}
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
	environment = append(environment, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home, "OCTOP_HOME="+home, "TEMP="+home, "TMP="+home, "TMPDIR="+home, "PYTHONNOUSERSITE=1", "PYTHONUTF8=1", "PYTHONDONTWRITEBYTECODE=1", "HF_HUB_OFFLINE=1", "LANGCHAIN_TRACING_V2=false", "LANGSMITH_TRACING=false")
	config := map[string]any{"home": home, "provider_id": spec.Octop.ProviderID, "model": spec.Octop.Model, "base_url": spec.Octop.BaseURL, "configuration_sha256": OctopConfigurationDigest(*spec.Octop), "max_steps": spec.Octop.MaxSteps, "max_output_tokens": spec.Octop.MaxOutputTokens}
	secrets := []string{}
	if credential && spec.Octop.APIKeyEnvRef != "" {
		ref := spec.Octop.APIKeyEnvRef
		value, ok := os.LookupEnv(ref)
		if !ok || value == "" || len(value) > 16384 || strings.ContainsAny(value, "\x00\r\n") {
			return nil, nil, fmt.Errorf("Octop credential environment reference is unavailable")
		}
		config["api_key_env_ref"] = ref
		environment = append(environment, ref+"="+value)
		secrets = append(secrets, value)
	}
	raw, _ := json.Marshal(config)
	return append(environment, "SUMA_OCTOP_CONFIG="+string(raw)), secrets, nil
}

func probeOctop(ctx context.Context, executable string, spec CommandSpec, secrets []string, workspace string) (string, error) {
	root, err := realDirectory(workspace)
	if err != nil {
		return "", fmt.Errorf("Octop probe workspace is unavailable")
	}
	parent := filepath.Join(root, ".cache", "runner", "octop-probes")
	if err = safeMkdir(parent); err != nil {
		return "", err
	}
	directory, err := os.MkdirTemp(parent, "probe-")
	if err != nil {
		return "", err
	}
	defer func() {
		if resolved, e := realDirectory(directory); e == nil && resolved == directory && within(parent, directory) {
			_ = os.RemoveAll(directory)
		}
	}()
	environment, _, err := octopEnvironment(spec, filepath.Join(directory, "home"), false)
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
		return "", fmt.Errorf("Octop Harness read-only SDK probe failed")
	}
	values, err := decodeToolObject(bytes.TrimSpace([]byte(result)))
	if err != nil || len(values) != 10 || !closedOpenCodeFrame(values, "protocol", "runtime_version", "harness_version", "acp_sdk_version", "configuration_sha256", "provider_id", "model", "workspace_tools_version") {
		return "", fmt.Errorf("Octop Harness SDK probe is invalid")
	}
	c, err := localOctopCapability(spec)
	v, valid := nativeInteger(values, "v")
	if err != nil || !valid || v != 1 || textAt(values, "type") != "probe" || textAt(values, "protocol") != tc.OctopProtocol || textAt(values, "runtime_version") != "1.0.0" || textAt(values, "harness_version") != c.HarnessVersion || textAt(values, "acp_sdk_version") != c.ACPSDKVersion || textAt(values, "configuration_sha256") != c.ConfigurationSHA256 || textAt(values, "provider_id") != c.ProviderID || textAt(values, "model") != c.Model || textAt(values, "workspace_tools_version") != NativeWorkspaceVersion {
		return "", fmt.Errorf("Octop SDK probe differs from its fixed local configuration")
	}
	return "suma-octop-runtime 1.0.0", nil
}
