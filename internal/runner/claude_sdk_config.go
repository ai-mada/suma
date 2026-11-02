package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// ClaudeSDKSpec is administrator-owned local configuration. Neither a model
// message nor a workflow node can supply executable arguments or credentials.
type ClaudeSDKSpec struct {
	NativeCLIExecutable string `json:"native_cli_executable"`
	Model               string `json:"model"`
	CredentialEnvRef    string `json:"credential_env_ref"`
	ReviewedCLIVersion  string `json:"reviewed_cli_version"`
}

const ClaudeSDKCommandID = "local-claude-code-sdk"

// BuildClaudeSDKCommand creates the sole reviewed SDK-host shape. It inspects
// files without running the CLI, reading credentials or granting execution.
func BuildClaudeSDKCommand(node, entry string, local ClaudeSDKSpec) (CommandSpec, error) {
	var err error
	if node, err = normalizeClaudeSDKPath(node); err != nil {
		return CommandSpec{}, err
	}
	if entry, err = normalizeClaudeSDKPath(entry); err != nil {
		return CommandSpec{}, err
	}
	if local.NativeCLIExecutable, err = normalizeClaudeSDKPath(local.NativeCLIExecutable); err != nil {
		return CommandSpec{}, err
	}
	spec := CommandSpec{Adapter: "claude", Protocol: tc.ClaudeSDKProtocol, Executable: node, Args: []string{entry}, Input: "claude-sdk-jsonl", ClaudeSDK: &local}
	if _, err := localClaudeSDKCapability(spec); err != nil {
		return CommandSpec{}, err
	}
	return spec, nil
}

func normalizeClaudeSDKPath(value string) (string, error) {
	value = filepath.FromSlash(value)
	if !filepath.IsAbs(value) || strings.ContainsAny(value, "\x00\r\n") {
		return "", fmt.Errorf("Claude SDK requires an absolute source path")
	}
	for _, component := range strings.Split(strings.TrimPrefix(value, filepath.VolumeName(value)), string(filepath.Separator)) {
		if component == "." || component == ".." || runtime.GOOS == "windows" && (strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") || strings.Contains(component, ":")) {
			return "", fmt.Errorf("Claude SDK source path contains an ambiguous alias")
		}
	}
	return filepath.Clean(value), nil
}

type claudeSDKAsset struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type claudeSDKManifest struct {
	SchemaVersion int              `json:"schema_version"`
	Protocol      string           `json:"protocol"`
	SDKVersion    string           `json:"sdk_version"`
	CLIVersion    string           `json:"cli_version"`
	Entry         string           `json:"entry"`
	Assets        []claudeSDKAsset `json:"assets"`
}

func validateClaudeSDKSpec(spec CommandSpec) error {
	if spec.ClaudeSDK == nil && spec.Protocol != tc.ClaudeSDKProtocol {
		return nil
	}
	p := spec.ClaudeSDK
	if p == nil || spec.Adapter != "claude" || spec.Protocol != tc.ClaudeSDKProtocol || spec.Input != "claude-sdk-jsonl" || spec.ReadOnly || spec.NativeWorkspace != nil || spec.ModelExecutionProtocol != "" || spec.CodexAppServer != nil || spec.OpenCode != nil || spec.ACP != nil || spec.TerminalSession || spec.CandidateComposition || spec.TestEvidence || spec.TestReport || spec.ArtifactInput || spec.ArtifactOutput != "" || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.IntelligentOperations || spec.DispatchRecommendation || spec.TaskConversationReply || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 || len(spec.Args) != 1 || !filepath.IsAbs(spec.Executable) || !filepath.IsAbs(spec.Args[0]) || filepath.Base(spec.Args[0]) != "suma-claude.bundle.mjs" || !filepath.IsAbs(p.NativeCLIExecutable) || p.ReviewedCLIVersion != tc.ClaudeSDKCLIVersion {
		return fmt.Errorf("Claude SDK requires its reviewed fixed host and local configuration")
	}
	name := strings.ToLower(filepath.Base(spec.Executable))
	if name != "node" && name != "node.exe" {
		return fmt.Errorf("Claude SDK host requires an absolute Node executable")
	}
	for _, value := range []string{spec.Executable, spec.Args[0], p.NativeCLIExecutable} {
		if strings.ContainsAny(value, "\x00\r\n") || filepath.Clean(value) != value {
			return fmt.Errorf("Claude SDK paths must be exact clean absolute paths")
		}
	}
	if !tc.ValidClaudeSDKModel(p.Model) {
		return fmt.Errorf("Claude SDK requires an exact fixed model identifier")
	}
	if !tc.ValidClaudeSDKCredentialEnvRef(p.CredentialEnvRef) {
		return fmt.Errorf("Claude SDK requires a separate local credential environment reference")
	}
	return nil
}

func claudeSDKSamePath(first, second string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(first), filepath.Clean(second))
	}
	return filepath.Clean(first) == filepath.Clean(second)
}

// readClaudeSDKFile binds the opened object as well as its path. It does not
// follow a changed leaf or a parent directory link to silently hash another file.
func readClaudeSDKFile(file string, maximum int64) ([]byte, error) {
	st, err := os.Lstat(file)
	resolved, resolveErr := filepath.EvalSymlinks(file)
	if err != nil || resolveErr != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || !claudeSDKSamePath(resolved, file) || st.Size() > maximum {
		return nil, fmt.Errorf("Claude SDK source is unavailable, linked or oversized")
	}
	f, err := os.Open(file)
	if err != nil {
		return nil, fmt.Errorf("Claude SDK source could not be opened")
	}
	opened, statErr := f.Stat()
	data, readErr := io.ReadAll(io.LimitReader(f, maximum+1))
	_ = f.Close()
	if statErr != nil || !os.SameFile(st, opened) || readErr != nil || int64(len(data)) > maximum || int64(len(data)) != st.Size() {
		return nil, fmt.Errorf("Claude SDK source changed during inspection")
	}
	return data, nil
}

func claudeSDKManifestDigest(spec CommandSpec) (string, error) {
	if err := validateClaudeSDKSpec(spec); err != nil || spec.ClaudeSDK == nil {
		return "", fmt.Errorf("Claude SDK manifest requires its exact configuration")
	}
	root := filepath.Dir(spec.Args[0])
	manifestPath := filepath.Join(root, "suma-claude-runtime.json")
	raw, err := readClaudeSDKFile(manifestPath, 1<<20)
	if err != nil {
		return "", err
	}
	if _, err = decodeToolObjectBounded(raw, 65536); err != nil {
		return "", fmt.Errorf("Claude SDK manifest must be closed bounded JSON")
	}
	var m claudeSDKManifest
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&m); err != nil || m.SchemaVersion != 1 || m.Protocol != tc.ClaudeSDKProtocol || m.SDKVersion != tc.ClaudeSDKVersion || m.CLIVersion != tc.ClaudeSDKCLIVersion || m.Entry != filepath.Base(spec.Args[0]) || len(m.Assets) < 1 || len(m.Assets) > 8192 {
		return "", fmt.Errorf("Claude SDK manifest differs from the reviewed SDK and CLI")
	}
	seen := map[string]bool{}
	var total int64
	for _, a := range m.Assets {
		name := a.Path
		if name == "" || strings.ContainsAny(name, "\\:\x00\r\n") || strings.HasPrefix(name, "/") || filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) != name || name == "." || strings.HasPrefix(name, "../") || strings.EqualFold(name, "suma-claude-runtime.json") || seen[strings.ToLower(name)] || !tc.ValidDigest(a.SHA256) || a.SizeBytes < 0 || a.SizeBytes > 32<<20 {
			return "", fmt.Errorf("Claude SDK asset identity is invalid")
		}
		total += a.SizeBytes
		if total > 256<<20 {
			return "", fmt.Errorf("Claude SDK payload exceeds its bound")
		}
		data, readErr := readClaudeSDKFile(filepath.Join(root, filepath.FromSlash(name)), 32<<20)
		if readErr != nil || int64(len(data)) != a.SizeBytes || digest(data) != a.SHA256 {
			return "", fmt.Errorf("Claude SDK asset is missing or changed")
		}
		seen[strings.ToLower(name)] = true
	}
	if !seen[strings.ToLower(m.Entry)] {
		return "", fmt.Errorf("Claude SDK manifest does not bind its entry")
	}
	// The dedicated runtime directory is the complete loadable closure. A new
	// unpinned file cannot quietly become a dependency of a reviewed bundle.
	if err = filepath.WalkDir(root, func(file string, e os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		resolved, resolveErr := filepath.EvalSymlinks(file)
		if resolveErr != nil || e.Type()&os.ModeSymlink != 0 || !claudeSDKSamePath(resolved, file) {
			return fmt.Errorf("Claude SDK payload traverses a link")
		}
		if e.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, file)
		if relErr != nil || file != manifestPath && !seen[strings.ToLower(filepath.ToSlash(rel))] {
			return fmt.Errorf("Claude SDK payload contains an unpinned file")
		}
		return nil
	}); err != nil {
		return "", err
	}
	return digest(raw), nil
}

func claudeSDKNativeExecutable(spec CommandSpec) (string, string, error) {
	if err := validateClaudeSDKSpec(spec); err != nil || spec.ClaudeSDK == nil {
		return "", "", fmt.Errorf("Claude SDK native executable requires its fixed configuration")
	}
	file := spec.ClaudeSDK.NativeCLIExecutable
	name := strings.ToLower(filepath.Base(file))
	if name == "node" || name == "node.exe" || map[string]bool{".cmd": true, ".bat": true, ".ps1": true, ".js": true, ".mjs": true, ".cjs": true}[strings.ToLower(filepath.Ext(file))] {
		return "", "", fmt.Errorf("Claude SDK requires the reviewed native CLI executable")
	}
	resolved, err := filepath.EvalSymlinks(file)
	st, statErr := os.Lstat(file)
	if err != nil || statErr != nil || !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || !claudeSDKSamePath(file, resolved) {
		return "", "", fmt.Errorf("Claude SDK native executable is unavailable or linked")
	}
	return commandExecutable(CommandSpec{Executable: file})
}

// ClaudeSDKConfigurationDigest includes the full reviewed runtime, both real
// executables and explicit local policy. A configured model alone is no identity.
func ClaudeSDKConfigurationDigest(spec CommandSpec) (string, error) {
	if err := validateClaudeSDKSpec(spec); err != nil || spec.ClaudeSDK == nil {
		return "", fmt.Errorf("Claude SDK configuration is invalid")
	}
	resolvedNode, nodeErr := filepath.EvalSymlinks(spec.Executable)
	nodeInfo, nodeStatErr := os.Lstat(spec.Executable)
	if nodeErr != nil || nodeStatErr != nil || !nodeInfo.Mode().IsRegular() || nodeInfo.Mode()&os.ModeSymlink != 0 || !claudeSDKSamePath(resolvedNode, spec.Executable) {
		return "", fmt.Errorf("Claude SDK Node executable is unavailable or linked")
	}
	_, nodeSHA, err := commandExecutable(spec)
	if err != nil {
		return "", err
	}
	_, nativeSHA, err := claudeSDKNativeExecutable(spec)
	if err != nil {
		return "", err
	}
	manifestSHA, err := claudeSDKManifestDigest(spec)
	if err != nil {
		return "", err
	}
	raw, _ := json.Marshal(struct {
		PolicyVersion int           `json:"policy_version"`
		SDKVersion    string        `json:"sdk_version"`
		CLI           ClaudeSDKSpec `json:"cli"`
		NodeSHA       string        `json:"node_sha256"`
		NativeSHA     string        `json:"native_cli_sha256"`
		ManifestSHA   string        `json:"manifest_sha256"`
	}{1, tc.ClaudeSDKVersion, *spec.ClaudeSDK, nodeSHA, nativeSHA, manifestSHA})
	return digest(raw), nil
}

func localClaudeSDKCapability(spec CommandSpec) (*tc.ClaudeSDKExecutionCapability, error) {
	if spec.ClaudeSDK == nil && spec.Protocol != tc.ClaudeSDKProtocol {
		return nil, nil
	}
	sha, err := ClaudeSDKConfigurationDigest(spec)
	if err != nil {
		return nil, err
	}
	c := &tc.ClaudeSDKExecutionCapability{SDKVersion: tc.ClaudeSDKVersion, CLIVersion: tc.ClaudeSDKCLIVersion, Model: spec.ClaudeSDK.Model, ConfigurationSHA256: sha, ModelSource: "administrator_sdk_option", BudgetMode: "observe_only", IdentityEvidence: "sdk_init_model_ack_only", AuthStrategy: "api_key_env_ref"}
	if err = tc.ValidateClaudeSDKExecutionCapability(*c); err != nil {
		return nil, err
	}
	return c, nil
}

type claudeSDKRuntimeConfig struct {
	NativeCLIExecutable string `json:"native_cli_executable"`
	NativeCLISHA256     string `json:"native_cli_sha256"`
	Home                string `json:"home"`
	Source              string `json:"source"`
	Model               string `json:"model"`
	CredentialEnvRef    string `json:"credential_env_ref"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	SDKVersion          string `json:"sdk_version"`
	CLIVersion          string `json:"cli_version"`
}

func claudeSDKEnvironment(spec CommandSpec, home, source string, credential bool) ([]string, []string, error) {
	cap, err := localClaudeSDKCapability(spec)
	if err != nil || cap == nil || !filepath.IsAbs(home) || !filepath.IsAbs(source) || within(source, home) || within(home, source) {
		return nil, nil, fmt.Errorf("Claude SDK requires its fixed configuration and separate isolated home")
	}
	if err = safeMkdir(home); err != nil {
		return nil, nil, fmt.Errorf("Claude SDK isolated home is unavailable")
	}
	entries, err := os.ReadDir(home)
	if err != nil || len(entries) != 0 {
		return nil, nil, fmt.Errorf("Claude SDK requires a fresh empty isolated home")
	}
	native, nativeSHA, err := claudeSDKNativeExecutable(spec)
	if err != nil {
		return nil, nil, err
	}
	base, _, err := commandEnvironment(CommandSpec{})
	if err != nil {
		return nil, nil, err
	}
	env := []string{}
	for _, value := range base {
		name, _, _ := strings.Cut(value, "=")
		if map[string]bool{"PATH": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "PROGRAMDATA": true, "LANG": true}[strings.ToUpper(name)] {
			env = append(env, value)
		}
	}
	env = append(env, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home)
	secrets := []string{}
	ref := spec.ClaudeSDK.CredentialEnvRef
	if credential {
		value, ok := os.LookupEnv(ref)
		if !ok || value == "" || len(value) > 16384 || strings.ContainsAny(value, "\x00\r\n") {
			return nil, nil, fmt.Errorf("Claude SDK credential environment reference is unavailable")
		}
		env = append(env, ref+"="+value)
		secrets = append(secrets, value)
	}
	config := claudeSDKRuntimeConfig{NativeCLIExecutable: native, NativeCLISHA256: nativeSHA, Home: home, Source: source, Model: cap.Model, CredentialEnvRef: ref, ConfigurationSHA256: cap.ConfigurationSHA256, SDKVersion: tc.ClaudeSDKVersion, CLIVersion: tc.ClaudeSDKCLIVersion}
	raw, _ := json.Marshal(config)
	env = append(env, "SUMA_CLAUDE_SDK_CONFIG="+string(raw))
	return env, secrets, nil
}

func claudeSDKVersion(ctx context.Context, spec CommandSpec) (string, error) {
	native, _, err := claudeSDKNativeExecutable(spec)
	if err != nil {
		return "", err
	}
	if _, err = claudeSDKManifestDigest(spec); err != nil {
		return "", err
	}
	// The installation probe starts no SDK query and forwards no credential
	// environment values. It is distinct from model or account verification.
	version, err := probeCommand(ctx, native, []string{"--version"}, CommandSpec{}, nil)
	version = strings.TrimSpace(version)
	if err != nil || version != tc.ClaudeSDKCLIVersion && version != tc.ClaudeSDKCLIVersion+" (Claude Code)" {
		return "", fmt.Errorf("Claude SDK requires the reviewed native CLI version")
	}
	return tc.ClaudeSDKCLIVersion, nil
}

func currentClaudeSDKCommand(cfg ExecConfig, spec CommandSpec, expected string) bool {
	_, executableSHA, err := commandExecutable(spec)
	if err != nil {
		return false
	}
	current, err := effectiveCommandProfileDigest(cfg, spec, executableSHA)
	return err == nil && current == expected
}
