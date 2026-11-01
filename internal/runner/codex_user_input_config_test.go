package runner

import (
	"bufio"
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

// Only an explicitly named copy of our test executable enters this offline
// fixture. It accepts read-only discovery calls, never a thread, turn or model.
func init() {
	exe, err := os.Executable()
	if err != nil || strings.TrimSuffix(filepath.Base(exe), ".exe") != "suma-codex187-probe" {
		return
	}
	os.Exit(codex187ProbeFixture(exe))
}

func codex187ProbeFixture(exe string) int {
	method := strings.Join(os.Args[1:], " ")
	home := os.Getenv("CODEX_HOME")
	if home == "" || home == "foreign-home187" || os.Getenv("SUMA_RUNNER_TOKEN") != "" {
		return 81
	}
	trace, _ := json.Marshal(map[string]any{"method": method, "home": home, "credential": os.Getenv("LOCAL_CODEX_187_KEY") != ""})
	if os.WriteFile(filepath.Join(filepath.Dir(exe), fmt.Sprintf("trace-%d.json", os.Getpid())), trace, 0600) != nil {
		return 82
	}
	switch method {
	case "--version":
		fmt.Println("codex-cli offline187")
		return 0
	case "app-server --help":
		fmt.Println("app-server --listen stdio://")
		return 0
	case "features list":
		// A feature claim is accepted only when the generated isolated config
		// actually enables it. No account credential is needed for this probe.
		config, err := os.ReadFile(filepath.Join(home, "config.toml"))
		if err != nil || !strings.Contains(string(config), "\n[features]\n") || !strings.Contains(string(config), "\ndefault_mode_request_user_input = true\n") || os.Getenv("LOCAL_CODEX_187_KEY") != "" {
			return 83
		}
		mode, _ := os.ReadFile(filepath.Join(filepath.Dir(exe), "mode.txt"))
		switch string(mode) {
		case "missing":
			fmt.Println("multi_agent under development false")
		case "disabled":
			fmt.Println("default_mode_request_user_input under development false")
		case "stderr_only":
			fmt.Fprintln(os.Stderr, "default_mode_request_user_input under development true")
		case "duplicate":
			fmt.Println("default_mode_request_user_input under development true\ndefault_mode_request_user_input under development false")
		default:
			fmt.Println("default_mode_request_user_input under development true")
		}
		return 0
	case "app-server --listen stdio://":
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			var request map[string]json.RawMessage
			if json.Unmarshal(scanner.Bytes(), &request) != nil {
				return 84
			}
			var name string
			_ = json.Unmarshal(request["method"], &name)
			var result any
			switch name {
			case "initialize":
				result = map[string]any{"userAgent": "offline187", "codexHome": home}
			case "initialized":
				continue
			case "model/list":
				result = map[string]any{"data": []any{map[string]any{"model": "owned-model187", "supportedReasoningEfforts": []any{map[string]any{"reasoningEffort": "high"}}, "defaultReasoningEffort": "high"}}, "nextCursor": nil}
			default:
				return 85
			}
			raw, _ := json.Marshal(map[string]any{"id": request["id"], "result": result})
			fmt.Println(string(raw))
		}
		if scanner.Err() != nil {
			return 86
		}
		return 0
	}
	return 87
}

func codex187ProbeExecutable(t *testing.T) string {
	t.Helper()
	current, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	name := "suma-codex187-probe"
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
	_, copyErr := io.Copy(target, source)
	if closeErr := target.Close(); copyErr != nil || closeErr != nil {
		t.Fatal("owned offline probe executable copy failed", copyErr, closeErr)
	}
	return exe
}

func TestCodexUserInputConfigurationPinsOptInAcrossPrivatePayloads(t *testing.T) {
	provider := CodexAppServerSpec{ProviderID: "owned187", BaseURL: "https://provider.example.test/v1", WindowsSandbox: "unelevated"}
	defaultSHA := CodexConfigurationDigest(provider)
	defaultProfile, err := commandProfileDigest(CommandSpec{CodexAppServer: &provider}, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if strings.Contains(codexConfiguration(provider, root), "default_mode_request_user_input") {
		t.Fatal("default configuration silently enables experimental input")
	}
	provider.EnableUserInput = true
	enabledProfile, err := commandProfileDigest(CommandSpec{CodexAppServer: &provider}, strings.Repeat("a", 64))
	if err != nil || enabledProfile == defaultProfile {
		t.Fatal("effective command profile does not bind the local feature opt-in", err)
	}
	home := filepath.Join(root, "isolated-home")
	spec := CommandSpec{CodexAppServer: &provider}
	if err := prepareCodexHome(spec, home, root); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(home, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	config := string(raw)
	for _, setting := range []string{"[features]\nmulti_agent = false\ndefault_mode_request_user_input = true\n", "approval_policy = \"on-request\"", "sandbox_mode = \"workspace-write\"", "network_access = false", "trust_level = \"untrusted\""} {
		if !strings.Contains(config, setting) {
			t.Fatalf("isolated opt-in config lost fixed policy: %s", setting)
		}
	}
	if strings.Contains(config, "collaboration") || strings.Contains(config, "experimental_api") || defaultSHA == CodexConfigurationDigest(provider) {
		t.Fatal("feature opt-in changed execution mode or escaped its digest")
	}
	// The digest is cross-language private protocol, so its policy version and
	// boolean omission must be pinned independently of the production builder.
	wantJSON := `{"version":4,"approval_policy":"on-request","provider":{"provider_id":"owned187","base_url":"https://provider.example.test/v1","windows_sandbox":"unelevated","enable_user_input":true}}`
	if CodexConfigurationDigest(provider) != digest([]byte(wantJSON)) {
		t.Fatal("feature configuration does not use the current exact policy digest")
	}
	input := CodexModelDiscoveryInput{AdapterID: "codex-app-server", Executable: filepath.Join(root, "codex.exe"), ExecutableSHA256: strings.Repeat("a", 64), ReviewedVersion: "codex-cli offline187", ProviderID: provider.ProviderID, BaseURL: provider.BaseURL, WindowsSandbox: provider.WindowsSandbox, EnableUserInput: true}
	body, _ := json.Marshal(input)
	decoded, err := DecodeCodexModelDiscoveryInput(body)
	if err != nil || !codexDiscoveryCommand(decoded).CodexAppServer.EnableUserInput {
		t.Fatal("private discovery dropped the explicit input feature", err)
	}
	managed := DesktopManagedCodexConfiguration{Executable: input.Executable, ExecutableSHA256: input.ExecutableSHA256, ReviewedVersion: input.ReviewedVersion, ProviderID: input.ProviderID, BaseURL: input.BaseURL, WindowsSandbox: input.WindowsSandbox, EnableUserInput: true, ConfigurationSHA256: CodexConfigurationDigest(provider)}
	body, _ = json.Marshal(managed)
	value, err := decodeManagedCodexConfiguration(body)
	if err != nil || !managedCodexDiscoveryInput(value).EnableUserInput {
		t.Fatal("managed private configuration dropped the digest-bound opt-in", err)
	}
	for _, invalid := range []string{"null", `"true"`, "1"} {
		if _, err := decodeManagedCodexConfiguration([]byte(strings.Replace(string(body), `"enable_user_input":true`, `"enable_user_input":`+invalid, 1))); err == nil {
			t.Fatal("non-boolean private feature input was accepted")
		}
	}
}

func TestCodexUserInputProbeAdvertisesOnlyConfirmedEnabledFeature(t *testing.T) {
	exe := codex187ProbeExecutable(t)
	repo, base := gitFixture(t)
	t.Setenv("CODEX_HOME", "foreign-home187")
	t.Setenv("SUMA_RUNNER_TOKEN", "private-platform187")
	t.Setenv("LOCAL_CODEX_187_KEY", "private-provider187")
	for _, test := range []struct {
		name    string
		enabled bool
		mode    string
		ready   bool
	}{
		{"default_disabled", false, "confirmed", true},
		{"explicit_confirmed", true, "confirmed", true},
		{"feature_missing", true, "missing", false},
		{"feature_disabled", true, "disabled", false},
		{"stderr_is_not_proof", true, "stderr_only", false},
		{"duplicate_is_not_proof", true, "duplicate", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(filepath.Dir(exe), "mode.txt"), []byte(test.mode), 0600); err != nil {
				t.Fatal(err)
			}
			spec := CommandSpec{Adapter: "codex", Protocol: cc.Protocol, Executable: exe, Args: []string{"app-server", "--listen", "stdio://"}, Input: "codex-app-server", CodexAppServer: &CodexAppServerSpec{ProviderID: "owned187", BaseURL: "http://127.0.0.1:43187/v1", WindowsSandbox: "unelevated", APIKeyEnvRef: "LOCAL_CODEX_187_KEY", EnableUserInput: test.enabled}}
			observations := describeForTest(t, ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"codex187": spec}})
			if len(observations) != 1 {
				t.Fatal("unexpected probe inventory size")
			}
			o := observations[0]
			if o.Available != test.ready {
				t.Fatalf("unproven feature became usable inventory: available=%v reason=%s", o.Available, o.Reason)
			}
			want := ""
			if test.enabled && test.ready {
				want = cc.UserInputProtocol
			}
			if o.Profile.UserInputProtocol != want || test.ready && o.CodexCatalog == nil {
				t.Fatal("input capability differs from explicit verified local configuration")
			}
		})
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(exe), "trace-*.json"))
	features, credentials := 0, 0
	for _, file := range files {
		var trace struct {
			Method     string `json:"method"`
			Home       string `json:"home"`
			Credential bool   `json:"credential"`
		}
		raw, _ := os.ReadFile(file)
		if json.Unmarshal(raw, &trace) != nil || trace.Home == "foreign-home187" {
			t.Fatal("probe inherited another account home")
		}
		if trace.Method == "features list" {
			features++
			if trace.Credential {
				t.Fatal("offline feature probe received a provider credential")
			}
		}
		if trace.Credential {
			credentials++
			if trace.Method != "app-server --listen stdio://" {
				t.Fatal("read-only version/help probe received a provider credential")
			}
		}
	}
	if features != 5 || credentials != 2 {
		t.Fatal("default flag was probed or failed capability entered catalogue discovery", features, credentials)
	}
}
