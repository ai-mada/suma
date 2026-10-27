package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// CodexAppServerSpec is local administrator input. A workflow can select a
// catalog model/effort, but cannot supply this provider, URL or credential ref.
// Only the generated configuration digest crosses the Runner boundary.
type CodexAppServerSpec struct {
	ProviderID     string `json:"provider_id"`
	BaseURL        string `json:"base_url"`
	APIKeyEnvRef   string `json:"api_key_env_ref,omitempty"`
	WindowsSandbox string `json:"windows_sandbox,omitempty"`
}

var codexProviderID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

func validateCodexAppServerSpec(spec CommandSpec) error {
	if spec.CodexAppServer == nil && spec.Protocol != cc.Protocol {
		return nil
	}
	p := spec.CodexAppServer
	if p == nil || spec.Adapter != "codex" || spec.Protocol != cc.Protocol || spec.Input != "codex-app-server" || spec.ReadOnly || spec.NativeWorkspace != nil || spec.ModelExecutionProtocol != "" || spec.Planning || spec.KnowledgeExtraction || spec.FailureDiagnosis || spec.TestEvidence || spec.ArtifactInput || spec.ArtifactOutput != "" || len(spec.Args) != 3 || strings.Join(spec.Args, " ") != "app-server --listen stdio://" {
		return fmt.Errorf("Codex app-server requires its fixed isolated agent protocol")
	}
	if p.WindowsSandbox != "" && p.WindowsSandbox != "unelevated" && p.WindowsSandbox != "elevated" && p.WindowsSandbox != "mxc" {
		return fmt.Errorf("invalid explicit Codex Windows sandbox mode")
	}
	if runtime.GOOS == "windows" && p.WindowsSandbox == "" {
		return fmt.Errorf("Codex app-server requires an explicit local Windows sandbox mode")
	}
	u, err := url.Parse(p.BaseURL)
	if !codexProviderID.MatchString(p.ProviderID) || len(p.BaseURL) > 2048 || err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(p.BaseURL, "\x00\r\n") || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1"))) {
		return fmt.Errorf("invalid local Codex provider configuration")
	}
	ref := strings.ToUpper(p.APIKeyEnvRef)
	if p.APIKeyEnvRef != "" && (!modelEnvName.MatchString(p.APIKeyEnvRef) || strings.HasPrefix(ref, "SUMA_RUNNER") || strings.HasPrefix(ref, "SUMA_POOL") || strings.HasPrefix(ref, "SUMA_AGENT") || ref == "CODEX_HOME" || ref == "HOME" || ref == "USERPROFILE" || ref == "PATH" || ref == "APPDATA" || ref == "LOCALAPPDATA") {
		return fmt.Errorf("invalid separate Codex credential environment reference")
	}
	for name := range spec.Env {
		if codexReservedEnvironment(name) || p.APIKeyEnvRef != "" && strings.EqualFold(name, p.APIKeyEnvRef) {
			return fmt.Errorf("Codex reserved environment requires the generated local configuration")
		}
	}
	for _, name := range spec.EnvFrom {
		if codexReservedEnvironment(name) || p.APIKeyEnvRef != "" && strings.EqualFold(name, p.APIKeyEnvRef) {
			return fmt.Errorf("Codex reserved environment cannot be inherited")
		}
	}
	return nil
}

func codexReservedEnvironment(name string) bool {
	n := strings.ToUpper(name)
	return strings.HasPrefix(n, "CODEX") || strings.HasPrefix(n, "OPENAI") || strings.HasPrefix(n, "SUMA_RUNNER") || strings.HasPrefix(n, "SUMA_POOL") || strings.HasPrefix(n, "SUMA_AGENT") || n == "HOME" || n == "USERPROFILE" || n == "APPDATA" || n == "LOCALAPPDATA"
}

// The version fixes every generated policy setting as well as the complete
// endpoint and credential reference. Secret values are intentionally excluded.
func CodexConfigurationDigest(spec CodexAppServerSpec) string {
	raw, _ := json.Marshal(struct {
		Version  int                `json:"version"`
		Provider CodexAppServerSpec `json:"provider"`
	}{2, spec})
	return digest(raw)
}

func codexConfiguration(spec CodexAppServerSpec, root string) string {
	q := strconv.Quote
	text := "model_provider = " + q(spec.ProviderID) + "\napproval_policy = \"never\"\nsandbox_mode = \"workspace-write\"\nweb_search = \"disabled\"\n\n[sandbox_workspace_write]\nwritable_roots = [" + q(root) + "]\nnetwork_access = false\nexclude_tmpdir_env_var = true\nexclude_slash_tmp = true\n\n[features]\nmulti_agent = false\n\n[model_providers." + spec.ProviderID + "]\nname = " + q(spec.ProviderID) + "\nbase_url = " + q(spec.BaseURL) + "\nwire_api = \"responses\"\nrequires_openai_auth = false\nrequest_max_retries = 0\nstream_max_retries = 0\n"
	if spec.APIKeyEnvRef != "" {
		text += "env_key = " + q(spec.APIKeyEnvRef) + "\n"
	}
	if spec.WindowsSandbox != "" {
		// Without an explicit Windows backend the CLI acknowledges read-only
		// scope, regardless of sandbox_mode. This selects its restricted local
		// implementation; it does not initialize accounts or change host ACLs.
		text += "\n[windows]\nsandbox = " + q(spec.WindowsSandbox) + "\n"
	}
	// A task repository is untrusted input. In particular, its .codex/config
	// must never become a second source of execution or permission settings.
	return text + "\n[projects." + q(root) + "]\ntrust_level = \"untrusted\"\n"
}

func codexEnvironment(spec CommandSpec, home string) ([]string, []string, error) {
	base, secrets, err := commandEnvironment(spec)
	if err != nil {
		return nil, nil, err
	}
	kept := make([]string, 0, len(base)+4)
	for _, value := range base {
		name, _, _ := strings.Cut(value, "=")
		if !codexReservedEnvironment(name) {
			kept = append(kept, value)
		}
	}
	kept = append(kept, "CODEX_HOME="+home, "HOME="+home, "USERPROFILE="+home, "APPDATA="+home, "LOCALAPPDATA="+home)
	if ref := spec.CodexAppServer.APIKeyEnvRef; ref != "" {
		value, ok := os.LookupEnv(ref)
		if !ok || value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return nil, nil, fmt.Errorf("Codex credential environment reference is unavailable")
		}
		kept = append(kept, ref+"="+value)
		secrets = append(secrets, value)
	}
	return kept, secrets, nil
}

func prepareCodexHome(spec CommandSpec, home, root string) error {
	if err := safeMkdir(home); err != nil {
		return err
	}
	return writeEvidence(filepath.Join(home, "config.toml"), []byte(codexConfiguration(*spec.CodexAppServer, root)))
}

func codexProbeHome(workspace string) (string, error) {
	root, err := filepath.Abs(filepath.Join(workspace, ".cache", "runner", "codex-probes"))
	if err != nil || safeMkdir(root) != nil {
		return "", fmt.Errorf("Codex probe workspace unavailable")
	}
	return os.MkdirTemp(root, "probe-")
}

func probeCodexVersion(ctx context.Context, exe string, spec CommandSpec, secrets []string, workspace string) (string, error) {
	if err := validateCodexAppServerSpec(spec); err != nil {
		return "", err
	}
	home, err := codexProbeHome(workspace)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(home)
	// Even read-only --version/--help probes receive no current user home or
	// account credential. CLI startup is not allowed to consult that context.
	copy := spec
	provider := *spec.CodexAppServer
	provider.APIKeyEnvRef = ""
	copy.CodexAppServer = &provider
	environment, _, err := codexEnvironment(copy, home)
	if err != nil {
		return "", err
	}
	probe := CommandSpec{Env: map[string]string{}}
	for _, value := range environment {
		name, value, _ := strings.Cut(value, "=")
		probe.Env[name] = value
	}
	// Startup diagnostics are not the CLI version. Keep stderr bounded and
	// redacted without allowing a non-fatal warning to become the first line.
	version, err := probeCommandStreams(ctx, exe, []string{"--version"}, probe, secrets, true)
	if err != nil {
		return "", err
	}
	help, err := probeCommand(ctx, exe, []string{"app-server", "--help"}, probe, secrets)
	if err != nil || !strings.Contains(version, "codex-cli ") || !strings.Contains(help, "--listen") {
		return "", fmt.Errorf("installed Codex version/help does not match app-server stdio protocol")
	}
	version = strings.TrimSpace(strings.SplitN(version, "\n", 2)[0])
	if version == "" || len(version) > 256 {
		return "", fmt.Errorf("Codex version is missing or oversized")
	}
	return version, nil
}

func codexExecutionRequest(a Assignment, cfg ExecConfig, spec CommandSpec) (*cc.Binding, error) {
	value, present := a.Config["codex_execution"]
	if !present && spec.Protocol != cc.Protocol {
		return nil, nil
	}
	if !present || a.Type != "agent" || validateCodexAppServerSpec(spec) != nil {
		return nil, fmt.Errorf("Codex execution requires an explicit current app-server selection")
	}
	if cc.HasNodeOverrides(a.Config) {
		return nil, fmt.Errorf("Codex execution does not accept node provider or hard-limit overrides")
	}
	b, err := cc.DecodeBinding(value)
	if err != nil || b.ConfigurationSHA256 != CodexConfigurationDigest(*spec.CodexAppServer) {
		return nil, fmt.Errorf("Codex execution binding differs from local configuration")
	}
	for _, o := range cfg.ToolInventory {
		if o.Available && o.Profile.CommandID == a.Command[0] && o.CodexCatalog != nil && o.CodexCatalog.CatalogSHA256 == b.CatalogSHA256 && o.CodexCatalog.ConfigurationSHA256 == b.ConfigurationSHA256 {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("Codex execution catalog is absent from current local inventory")
}

// The probe owns a fresh home and starts no thread or turn. It uses the same
// bounded stdio state machine as execution, without borrowing a logged-in home.
func probeCodexCatalog(ctx context.Context, exe string, spec CommandSpec, cfg ExecConfig) (*cc.Catalog, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	home, err := codexProbeHome(cfg.Workspace)
	if err != nil {
		return nil, fmt.Errorf("Codex probe home unavailable")
	}
	// This is a newly owned random child of the checked probe directory; no
	// configured or historical path is ever opened to recover a session.
	defer os.RemoveAll(home)
	if err = prepareCodexHome(spec, home, home); err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, spec.Args...)
	cmd.Dir = home
	cmd.Env, _, err = codexEnvironment(spec, home)
	if err != nil {
		return nil, err
	}
	driver := newCodexAppServer(tc.Profile{}, spec, home, home, "", nil, nil)
	defer driver.Close()
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	defer pipe.Close()
	cmd.Stdout = driver
	cmd.Stderr = newOutput(32<<10, cfg.Secrets)
	ctl, err := startProcess(cmd)
	if err != nil {
		return nil, fmt.Errorf("Codex app-server probe could not start")
	}
	defer ctl.Close()
	copied := make(chan struct{})
	go func() { defer close(copied); _, _ = driver.CopyInput(pipe); _ = pipe.Close() }()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = ctl.Stop()
		<-done
		err = ctx.Err()
	}
	_ = driver.Close()
	<-copied
	if finishErr := ctl.Finish(); finishErr != nil || err != nil {
		return nil, fmt.Errorf("Codex app-server probe completion unverified")
	}
	driver.Finish()
	if driver.bad != nil || driver.catalog == nil {
		return nil, fmt.Errorf("Codex app-server catalog probe failed")
	}
	return driver.catalog, nil
}
