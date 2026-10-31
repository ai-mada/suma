package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
)

// Only this explicitly named copied native test executable enters the fixture.
// Its isolated stdio accepts catalogue discovery, never thread/turn/auth calls.
func init() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	name := filepath.Base(exe)
	if name != "suma-codex175-fixture.exe" && name != "suma-codex175-fixture" {
		return
	}
	os.Exit(codex175NativeFixture(exe))
}
func codex175NativeFixture(exe string) (exit int) {
	methods := []string{}
	keyPresent := os.Getenv("LOCAL_CODEX_175_KEY") != ""
	defer func() {
		raw, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "methods": methods, "credential_present": keyPresent, "foreign_home": os.Getenv("CODEX_HOME") == "foreign-home175"})
		_ = os.WriteFile(filepath.Join(filepath.Dir(exe), fmt.Sprintf("trace-%d.json", os.Getpid())), raw, 0600)
	}()
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		methods = append(methods, "--version")
		fmt.Println("codex-cli controlled175")
		return 0
	}
	if strings.Join(os.Args[1:], " ") == "app-server --help" {
		methods = append(methods, "app-server --help")
		fmt.Println("--listen stdio://")
		return 0
	}
	if strings.Join(os.Args[1:], " ") != "app-server --listen stdio://" || os.Getenv("SUMA_RUNNER_TOKEN") != "" || os.Getenv("CODEX_HOME") == "foreign-home175" {
		return 81
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var m map[string]json.RawMessage
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			return 82
		}
		var method string
		_ = json.Unmarshal(m["method"], &method)
		methods = append(methods, method)
		var result any
		switch method {
		case "initialize":
			result = map[string]any{"userAgent": "controlled175", "codexHome": os.Getenv("CODEX_HOME")}
		case "initialized":
			continue
		case "model/list":
			result = map[string]any{"data": []any{map[string]any{"model": "owned-model175", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "low"}, map[string]any{"reasoningEffort": "high"}}, "defaultReasoningEffort": "low"}}, "nextCursor": nil}
		default:
			return 83
		}
		raw, _ := json.Marshal(map[string]any{"id": m["id"], "result": result})
		fmt.Println(string(raw))
	}
	if scanner.Err() != nil {
		return 84
	}
	return 0
}
func codex175NativeInput(t *testing.T) CodexModelDiscoveryInput {
	t.Helper()
	current, _ := os.Executable()
	name := "suma-codex175-fixture"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe := filepath.Join(t.TempDir(), name)
	source, err := os.Open(current)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := os.OpenFile(exe, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(target, source)
	closeErr := target.Close()
	if err != nil || closeErr != nil {
		t.Fatal("controlled native copy unavailable")
	}
	resolved, sha, err := qoderNativeTarget(context.Background(), exe)
	if err != nil {
		t.Fatal(err)
	}
	return CodexModelDiscoveryInput{AdapterID: "codex-app-server", Executable: resolved, ExecutableSHA256: sha, ReviewedVersion: "codex-cli controlled175", ProviderID: "owned175", BaseURL: "http://127.0.0.1:43175/v1", WindowsSandbox: "unelevated"}
}

func codex175ManagedRaw(t *testing.T) []byte {
	t.Helper()
	base := desktopManagedFixtureInput(t)
	raw, _ := json.Marshal(base)
	var input map[string]any
	_ = json.Unmarshal(raw, &input)
	provider := CodexAppServerSpec{ProviderID: "owned175", BaseURL: "http://127.0.0.1:43175/v1", WindowsSandbox: "unelevated"}
	input["adapter_id"] = "codex-app-server"
	input["codex_configuration"] = map[string]any{"executable": filepath.Join(t.TempDir(), "codex.exe"), "executable_sha256": strings.Repeat("a", 64), "reviewed_version": "codex-cli controlled175", "provider_id": provider.ProviderID, "base_url": provider.BaseURL, "windows_sandbox": provider.WindowsSandbox, "configuration_sha256": CodexConfigurationDigest(provider)}
	raw, _ = json.Marshal(input)
	return raw
}

func TestDesktopManagedCodexInputKeepsProviderConfigurationClosed(t *testing.T) {
	raw := codex175ManagedRaw(t)
	decoded, err := DecodeDesktopManagedRunnerInput(raw)
	if err != nil {
		t.Fatal("confirmed private native Codex configuration was rejected", err)
	}
	encoded, _ := json.Marshal(decoded)
	var roundtrip map[string]any
	_ = json.Unmarshal(encoded, &roundtrip)
	configuration, ok := roundtrip["codex_configuration"].(map[string]any)
	if !ok || configuration["provider_id"] != "owned175" || configuration["model"] != nil {
		t.Fatal("managed connection lost the provider source or fixed a task model")
	}
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["adapter_id"] = "codex-jsonl" },
		func(v map[string]any) { v["codex_configuration"] = nil },
		func(v map[string]any) { v["acp_configuration"] = map[string]any{} },
		func(v map[string]any) {
			v["codex_configuration"].(map[string]any)["model"] = "not-a-connection-setting"
		},
		func(v map[string]any) {
			v["codex_configuration"].(map[string]any)["configuration_sha256"] = strings.Repeat("b", 64)
		},
		func(v map[string]any) {
			v["codex_configuration"].(map[string]any)["api_key"] = "must-never-accept-key-values"
		},
	} {
		var input map[string]any
		_ = json.Unmarshal(raw, &input)
		change(input)
		invalid, _ := json.Marshal(input)
		if _, err = DecodeDesktopManagedRunnerInput(invalid); err == nil {
			t.Fatal("invalid or mixed Codex configuration was accepted")
		}
	}
	duplicate := strings.Replace(string(raw), `"provider_id":"owned175"`, `"provider_id":"owned175","provider_id":"changed"`, 1)
	if _, err = DecodeDesktopManagedRunnerInput([]byte(duplicate)); err == nil {
		t.Fatal("duplicate native provider was accepted")
	}
}

func TestCodexPassiveDiscoveryRequiresConfigurationWithoutLaunch(t *testing.T) {
	dir := t.TempDir()
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	file := localDiscoveryFile(t, filepath.Join(dir, name), "MZcontrolled native structural fixture175; never executed")
	calls := 0
	report, err := discoverInstalledTools(context.Background(), "codex-app-server", []string{dir}, func(_ context.Context, spec CommandSpec) (string, error) {
		calls++
		if spec.Adapter != "codex" || spec.Protocol != cc.Protocol || spec.Executable != file {
			t.Error("native install probe changed protocol or executable")
		}
		return "codex-cli full controlled175", nil
	})
	if err != nil || len(report.Items) != 1 {
		t.Fatal("native Codex install detection unavailable", err)
	}
	item := report.Items[0]
	if !item.Installed || item.LaunchSupported || item.Command != nil || item.ReasonCode != "configuration_required" || item.Version != "codex-cli full controlled175" || len(item.ExecutableSHA256) != 64 || calls != 1 {
		t.Fatal("passive installation became dispatch authority", item.ReasonCode)
	}
}

func TestCodexDiscoveryPrivateInputIsClosedAndPreservesOnlyReferences(t *testing.T) {
	input := CodexModelDiscoveryInput{AdapterID: "codex-app-server", Executable: filepath.Join(t.TempDir(), "codex.exe"), ExecutableSHA256: strings.Repeat("a", 64), ReviewedVersion: "codex-cli controlled175", ProviderID: "owned175", BaseURL: "https://provider.example.test/v1", WindowsSandbox: "unelevated", APIKeyEnvRef: "LOCAL_CODEX_175_KEY"}
	raw, _ := json.Marshal(input)
	if _, err := DecodeCodexModelDiscoveryInput(raw); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"adapter_id"`, `"Adapter_ID"`, 1),
		strings.TrimSuffix(string(raw), "}") + `,"goal":"must not prompt"}`,
		strings.TrimSuffix(string(raw), "}") + `,"api_key":"must-not-accept-values"}`,
		strings.Replace(string(raw), `"provider_id":"owned175"`, `"provider_id":"owned175","provider_id":"changed"`, 1),
		strings.Replace(string(raw), `"api_key_env_ref":"LOCAL_CODEX_175_KEY"`, `"api_key_env_ref":null`, 1),
		strings.Replace(string(raw), `LOCAL_CODEX_175_KEY`, `SUMA_RUNNER_TOKEN`, 1),
		strings.Repeat("x", CodexModelDiscoveryInputLimit+1),
	} {
		if _, err := DecodeCodexModelDiscoveryInput([]byte(bad)); err == nil {
			t.Fatal("private model discovery accepted injected, duplicate or null fields")
		}
	}
}

func TestCodexModelDiscoveryUsesPinnedNativeCatalogWithoutWork(t *testing.T) {
	input := codex175NativeInput(t)
	input.APIKeyEnvRef = "LOCAL_CODEX_175_KEY"
	t.Setenv(input.APIKeyEnvRef, "private-reference175")
	t.Setenv("SUMA_RUNNER_TOKEN", "do-not-forward-platform175")
	t.Setenv("CODEX_HOME", "foreign-home175")
	result, err := DiscoverCodexModels(context.Background(), input)
	if err != nil || result.State != "ready" || result.Catalogue == nil || cc.ValidateModelDiscoveryResult(result) != nil || result.ConfigurationSHA256 != CodexConfigurationDigest(*codexDiscoveryCommand(input).CodexAppServer) {
		t.Fatalf("real controlled catalogue pipeline unavailable: %v state=%s code=%s", err, result.State, result.ErrorCode)
	}
	if len(result.Catalogue.Models) != 1 || result.Catalogue.Models[0].Model != "owned-model175" {
		t.Fatal("catalogue was guessed or replaced")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(input.Executable), "trace-*.json"))
	methods := []string{}
	credentialProcesses := 0
	for _, file := range files {
		var trace struct {
			Methods     []string `json:"methods"`
			Credential  bool     `json:"credential_present"`
			ForeignHome bool     `json:"foreign_home"`
		}
		raw, _ := os.ReadFile(file)
		if json.Unmarshal(raw, &trace) != nil || trace.ForeignHome {
			t.Fatal("controlled child inherited user context")
		}
		methods = append(methods, trace.Methods...)
		if trace.Credential {
			credentialProcesses++
		}
	}
	if len(files) != 3 || len(methods) != 5 || credentialProcesses != 1 {
		t.Fatal("unexpected native process/method/credential scope", len(files), methods, credentialProcesses)
	}
	for _, method := range methods {
		if method != "--version" && method != "app-server --help" && method != "initialize" && method != "initialized" && method != "model/list" {
			t.Fatal("catalogue discovery started work", method)
		}
	}
	changed := input
	changed.ExecutableSHA256 = strings.Repeat("b", 64)
	failed, err := DiscoverCodexModels(context.Background(), changed)
	if err != nil || failed.State != "failed" || failed.ErrorCode != "codex_discovery_installation_changed" || failed.Catalogue != nil {
		t.Fatal("changed native pin produced usable catalogue", err)
	}
	t.Setenv(input.APIKeyEnvRef, "")
	missing, err := DiscoverCodexModels(context.Background(), input)
	if err != nil || missing.ErrorCode != "codex_discovery_credential_unavailable" || missing.Catalogue != nil {
		t.Fatal("missing credential produced usable catalogue", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	unknown, err := DiscoverCodexModels(ctx, input)
	if err != nil || unknown.State != "unknown" || unknown.Catalogue != nil {
		t.Fatal("cancelled discovery became ready", err)
	}
	after, _ := filepath.Glob(filepath.Join(filepath.Dir(input.Executable), "trace-*.json"))
	if len(after) != len(files) {
		t.Fatal("failed pins or missing credentials started another child")
	}
}

func TestDesktopManagedCodexPreparationRechecksPinsWithoutSelectingTaskModel(t *testing.T) {
	input := codex175NativeInput(t)
	repo, base := gitFixture(t)
	managed := desktopManagedFixtureInput(t)
	managed.AdapterID = "codex-app-server"
	managed.Repository, managed.BaseCommit = repo, base
	managed.CodexConfiguration = &DesktopManagedCodexConfiguration{Executable: input.Executable, ExecutableSHA256: input.ExecutableSHA256, ReviewedVersion: input.ReviewedVersion, ProviderID: input.ProviderID, BaseURL: input.BaseURL, WindowsSandbox: input.WindowsSandbox, ConfigurationSHA256: CodexConfigurationDigest(*codexDiscoveryCommand(input).CodexAppServer)}
	cfg, err := prepareDesktopManagedExecution(context.Background(), managed, func(context.Context, string) (InstalledToolDiscovery, error) {
		t.Fatal("confirmed source caused another installation selection")
		return InstalledToolDiscovery{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := cfg.Commands["desktop-codex-app-server"]
	if spec.CodexAppServer == nil || spec.ACP != nil || spec.NativeWorkspace != nil || spec.ReadOnly || CodexConfigurationDigest(*spec.CodexAppServer) != managed.CodexConfiguration.ConfigurationSHA256 || len(cfg.ToolInventory) != 0 || strings.Join(spec.Args, " ") != "app-server --listen stdio://" {
		t.Fatal("preparation lost original isolated execution policy")
	}
	if _, err = os.Stat(managed.StateDirectory); !os.IsNotExist(err) {
		t.Fatal("configuration preparation opened a journal")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(input.Executable), "trace-*.json"))
	if len(files) != 2 {
		t.Fatal("preparation started catalogue/work instead of leaving fresh inventory to Worker")
	}
	managed.CodexConfiguration.ReviewedVersion = "codex-cli different175"
	if _, err = prepareDesktopManagedExecution(context.Background(), managed, nil); err == nil {
		t.Fatal("changed reviewed version reached a Worker")
	}
}
