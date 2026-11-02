package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This uniquely named, owned test process accepts only a passive version
// request. It is not a vendor CLI and never enters the SDK/model runtime.
func init() {
	exe, err := os.Executable()
	if err != nil || strings.TrimSuffix(filepath.Base(exe), ".exe") != "suma-claude201-fixture" {
		return
	}
	cwd, _ := os.Getwd()
	valid := len(os.Args) == 2 && os.Args[1] == "--version" && os.Getenv("HOME") == cwd && os.Getenv("XDG_CONFIG_HOME") == cwd && os.Getenv("OWNED_CLAUDE_KEY") == "" && os.Getenv("SUMA_RUNNER_TOKEN") == "" && os.Getenv("NODE_OPTIONS") == "" && os.Getenv("ANTHROPIC_API_KEY") == ""
	if !valid {
		os.Exit(83)
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(exe), "passive-probe201"), []byte("version-only; isolated; no credentials"), 0600)
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "block-version201")); err == nil {
		time.Sleep(30 * time.Second)
	}
	fmt.Println("2.1.314 (Claude Code)")
	os.Exit(0)
}

func TestDesktopClaudeSDKCancelledPassiveProbePreservesBoundedLifecycleFacts(t *testing.T) {
	j, err := OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	client := &Client{BaseURL: "http://127.0.0.1:19500", ProjectID: "project195", Token: "fixture195"}
	cp, err := j.beginToolInspection(client, "worker195")
	if err != nil {
		t.Fatal(err)
	}
	i := inspectionFixture194(client.ProjectID, "worker195", cp.ClaimID)
	cp.Responded, cp.Inspection, cp.Started = true, &i, true
	if err = j.saveToolInspection(*cp); err != nil {
		t.Fatal(err)
	}
	collector, err := newToolProbeCollector(j, *cp)
	if err != nil {
		t.Fatal(err)
	}
	native := openCode177NativeCopy(t, t.TempDir(), "suma-claude201-fixture")
	if err = os.WriteFile(filepath.Join(filepath.Dir(native), "block-version201"), []byte("owned timeout fault"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(collector.context(context.Background()))
	defer cancel()
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(filepath.Dir(native), "passive-probe201")); err == nil {
				cancel()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()
	started := time.Now()
	version, probeErr := passiveClaudeSDKVersion(ctx, native)
	if probeErr == nil || version != "" || time.Since(started) > 8*time.Second {
		t.Fatal("cancelled passive probe was not bounded or invented a version")
	}
	if err = collector.closeReturned(); err != nil {
		t.Fatal(err)
	}
	receipt, _, err := j.readToolProbeReceipt(*cp)
	if err != nil {
		t.Fatal(err)
	}
	windowsTree := runtime.GOOS == "windows"
	stopped := 0
	if windowsTree {
		stopped = 1
	}
	if !receipt.ProbeReturned || receipt.NativeProcessCount != 1 || receipt.StoppedProcessCount != stopped || receipt.AllNativeTreesStopped != windowsTree {
		t.Fatalf("passive probe lost mature lifecycle facts: %+v", receipt)
	}
}

func TestDesktopClaudeSDKDiscoveryKeepsInstallationSeparateFromConfiguration(t *testing.T) {
	root := t.TempDir()
	exe := openCode177NativeCopy(t, root, "claude")
	for _, version := range []string{tc.ClaudeSDKCLIVersion, "2.1.999"} {
		calls := 0
		report, err := discoverInstalledTools(context.Background(), "claude-code-sdk", []string{root}, func(_ context.Context, spec CommandSpec) (string, error) {
			calls++
			if spec.Executable != exe || spec.Adapter != "claude" || len(spec.Args) != 0 {
				t.Fatal("SDK installation scan acquired a task launch shape")
			}
			return version, nil
		})
		if err != nil || len(report.Items) != 1 {
			t.Fatalf("SDK installation not discoverable: %v", err)
		}
		item := report.Items[0]
		want := "configuration_required"
		if version != tc.ClaudeSDKCLIVersion {
			want = "version_unsupported"
		}
		if calls != 1 || item.AdapterID != "claude-code-sdk" || item.Product != "Claude Code" || !item.Installed || item.LaunchSupported || item.Command != nil || item.Executable != exe || !tc.ValidDigest(item.ExecutableSHA256) || item.Version != version || item.ReasonCode != want {
			t.Fatalf("passive SDK facts differ: %+v", item)
		}
	}
	// A late replacement cannot be represented as the installation just read.
	report, err := discoverInstalledTools(context.Background(), "claude-code-sdk", []string{root}, func(context.Context, CommandSpec) (string, error) {
		if err := os.WriteFile(exe, []byte("changed native bytes"), 0700); err != nil {
			t.Fatal(err)
		}
		return tc.ClaudeSDKCLIVersion, nil
	})
	if err != nil || report.Items[0].ReasonCode != "probe_failed" || report.Items[0].Executable != "" || report.Items[0].Command != nil {
		t.Fatal("changed installation supplied a usable scan pin")
	}
	shimRoot := t.TempDir()
	localDiscoveryFile(t, filepath.Join(shimRoot, "claude.cmd"), "not executed")
	report, err = discoverInstalledTools(context.Background(), "claude-code-sdk", []string{shimRoot}, func(context.Context, CommandSpec) (string, error) {
		t.Fatal("shell launcher was executed")
		return "", nil
	})
	if err != nil || !report.Items[0].Installed || report.Items[0].ReasonCode != "unsupported_launcher" || report.Items[0].LaunchSupported {
		t.Fatal("shell launcher became SDK authority")
	}
}

func TestDesktopClaudeSDKPrivateConfigurationRequiresExactClosedBranch(t *testing.T) {
	spec := claudeSDKConfigFixture192(t)
	sha, _ := ClaudeSDKConfigurationDigest(spec)
	_, nodeSHA, _ := commandExecutable(spec)
	_, nativeSHA, _ := claudeSDKNativeExecutable(spec)
	manifestSHA, _ := claudeSDKManifestDigest(spec)
	input := desktopManagedFixtureInput(t)
	raw, _ := json.Marshal(input)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	body["adapter_id"] = "claude-code-sdk"
	body["claude_sdk_configuration"] = map[string]any{"native_cli_executable": spec.ClaudeSDK.NativeCLIExecutable, "native_cli_sha256": nativeSHA, "reviewed_cli_version": tc.ClaudeSDKCLIVersion, "node_executable": spec.Executable, "node_sha256": nodeSHA, "runtime_manifest": strings.TrimSuffix(spec.Args[0], "suma-claude.bundle.mjs") + "suma-claude-runtime.json", "runtime_manifest_sha256": manifestSHA, "model": spec.ClaudeSDK.Model, "credential_env_ref": spec.ClaudeSDK.CredentialEnvRef, "configuration_sha256": sha}
	raw, _ = json.Marshal(body)
	if _, err := DecodeDesktopManagedRunnerInput(raw); err != nil {
		t.Fatalf("valid Claude SDK private configuration rejected: %v", err)
	}
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["claude_sdk_configuration"] = nil },
		func(v map[string]any) { v["opencode_configuration"] = map[string]any{} },
		func(v map[string]any) { v["adapter_id"] = "claude-code-cli" },
		func(v map[string]any) { v["claude_sdk_configuration"].(map[string]any)["model"] = "claude-other" },
		func(v map[string]any) {
			v["claude_sdk_configuration"].(map[string]any)["credential_env_ref"] = "ANTHROPIC_AUTH_TOKEN"
		},
	} {
		var candidate map[string]any
		_ = json.Unmarshal(raw, &candidate)
		change(candidate)
		bad, _ := json.Marshal(candidate)
		if _, err := DecodeDesktopManagedRunnerInput(bad); err == nil {
			t.Fatal("mixed or changed Claude private configuration accepted")
		}
	}
	// The original complete SDK constructor and the private decoder must share
	// the same GoJSON identity, including both executables and manifest bytes.
	decoded, err := DecodeDesktopManagedRunnerInput(raw)
	if err != nil || managedClaudeSDKConfigurationDigest(decoded.ClaudeSDKConfiguration) != sha {
		t.Fatal("private identity differs from original SDK constructor")
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"model":"claude-sonnet-5"`, `"model":"claude-sonnet-5","model":"claude-other"`, 1),
		strings.Replace(string(raw), `"model":"claude-sonnet-5"`, `"model":"claude-\ud800"`, 1),
		strings.Replace(string(raw), `"credential_env_ref":"OWNED_CLAUDE_KEY"`, `"credential_env_ref":null`, 1),
	} {
		if _, err := DecodeDesktopManagedRunnerInput([]byte(bad)); err == nil {
			t.Fatal("malformed private nested bytes accepted")
		}
	}
	// Codec-only cross-language vector: these separators are deliberately not
	// valid managed paths, but must retain GoJSON's HTML and Unicode escaping.
	vector := &DesktopManagedClaudeSDKConfiguration{NativeCLIExecutable: "C:\\owned\\a<>&\u2028\u2029\\claude.exe", Model: "claude-sonnet-5", CredentialEnvRef: "OWNED_CLAUDE_KEY", ReviewedCLIVersion: "2.1.314", NodeSHA256: strings.Repeat("1", 64), NativeCLISHA256: strings.Repeat("2", 64), RuntimeManifestSHA256: strings.Repeat("3", 64)}
	if got := managedClaudeSDKConfigurationDigest(vector); got != "bc843c029ba91046dc701cb35dfc7e54724e3d50ad258cf0d086d9376fdd2ff6" {
		t.Fatalf("GoJSON codec vector differs: %s", got)
	}
	invalidModel := *decoded.ClaudeSDKConfiguration
	invalidModel.Model = "claude-foo"
	invalidModel.ConfigurationSHA256 = managedClaudeSDKConfigurationDigest(&invalidModel)
	decoded.ClaudeSDKConfiguration = &invalidModel
	invalidRaw, _ := json.Marshal(decoded)
	if _, err := DecodeDesktopManagedRunnerInput(invalidRaw); err == nil {
		t.Fatal("exact digest cannot make a model rejected by the pinned SDK host valid")
	}
	for _, ref := range []string{"LANG", "PATHEXT", "LC_MESSAGES", "LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH"} {
		reservedRef := *decoded.ClaudeSDKConfiguration
		reservedRef.Model = spec.ClaudeSDK.Model
		reservedRef.CredentialEnvRef = ref
		reservedRef.ConfigurationSHA256 = managedClaudeSDKConfigurationDigest(&reservedRef)
		decoded.ClaudeSDKConfiguration = &reservedRef
		raw, _ := json.Marshal(decoded)
		if _, err := DecodeDesktopManagedRunnerInput(raw); err == nil {
			t.Fatalf("exact digest cannot make reserved environment name a credential reference: %s", ref)
		}
	}
}

func claude201ManagedFixture(t *testing.T) DesktopManagedRunnerInput {
	t.Helper()
	spec := claudeSDKConfigFixture192(t)
	root := t.TempDir()
	spec.ClaudeSDK.NativeCLIExecutable = openCode177NativeCopy(t, root, "suma-claude201-fixture")
	nodeName := "node"
	spec.Executable = openCode177NativeCopy(t, root, nodeName)
	_, nodeSHA, err := qoderNativeTarget(context.Background(), spec.Executable)
	if err != nil {
		t.Fatal(err)
	}
	_, nativeSHA, err := qoderNativeTarget(context.Background(), spec.ClaudeSDK.NativeCLIExecutable)
	if err != nil {
		t.Fatal(err)
	}
	manifestSHA, err := claudeSDKManifestDigest(spec)
	if err != nil {
		t.Fatal(err)
	}
	sha, err := ClaudeSDKConfigurationDigest(spec)
	if err != nil {
		t.Fatal(err)
	}
	repo, base := gitFixture(t)
	input := desktopManagedFixtureInput(t)
	input.AdapterID, input.Repository, input.BaseCommit = "claude-code-sdk", repo, base
	input.ClaudeSDKConfiguration = &DesktopManagedClaudeSDKConfiguration{NativeCLIExecutable: spec.ClaudeSDK.NativeCLIExecutable, NativeCLISHA256: nativeSHA, ReviewedCLIVersion: tc.ClaudeSDKCLIVersion, NodeExecutable: spec.Executable, NodeSHA256: nodeSHA, RuntimeManifest: filepath.Join(filepath.Dir(spec.Args[0]), "suma-claude-runtime.json"), RuntimeManifestSHA256: manifestSHA, Model: spec.ClaudeSDK.Model, CredentialEnvRef: spec.ClaudeSDK.CredentialEnvRef, ConfigurationSHA256: sha}
	return input
}

func TestDesktopClaudeSDKPreparationRechecksPinsBeforeOriginalWorker(t *testing.T) {
	input := claude201ManagedFixture(t)
	t.Setenv("OWNED_CLAUDE_KEY", "owned-local-value201")
	t.Setenv("ANTHROPIC_API_KEY", "not-selected201")
	t.Setenv("SUMA_RUNNER_TOKEN", "private201")
	t.Setenv("NODE_OPTIONS", "--foreign201")
	cfg, err := prepareDesktopManagedExecution(context.Background(), input, nil)
	if err != nil {
		t.Fatal(err)
	}
	spec := cfg.Commands["desktop-claude-code-sdk"]
	cap, err := localClaudeSDKCapability(spec)
	if err != nil || cap == nil || cap.Model != input.ClaudeSDKConfiguration.Model || cap.ConfigurationSHA256 != input.ClaudeSDKConfiguration.ConfigurationSHA256 || cap.CLIVersion != tc.ClaudeSDKCLIVersion || len(cfg.ToolInventory) != 0 || !cfg.AllowExec || cfg.BaseCommit != input.BaseCommit {
		t.Fatal("managed SDK changed original immutable execution policy")
	}
	if _, err := os.Stat(input.StateDirectory); !os.IsNotExist(err) {
		t.Fatal("preparation opened the Journal")
	}
	if strings.TrimSpace(gitTest(t, input.Repository, "status", "--porcelain")) != "" {
		t.Fatal("preparation changed the bound Git source")
	}
	marker := filepath.Join(filepath.Dir(input.ClaudeSDKConfiguration.NativeCLIExecutable), "passive-probe201")
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("owned native passive version probe was not observed")
	}
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"native", "node", "manifest", "configuration", "credential", "workspace", "asset"} {
		copy := input
		value := *input.ClaudeSDKConfiguration
		copy.ClaudeSDKConfiguration = &value
		switch field {
		case "native":
			value.NativeCLISHA256 = strings.Repeat("1", 64)
		case "node":
			value.NodeSHA256 = strings.Repeat("2", 64)
		case "manifest":
			value.RuntimeManifestSHA256 = strings.Repeat("3", 64)
		case "configuration":
			value.ConfigurationSHA256 = strings.Repeat("4", 64)
		case "credential":
			t.Setenv("OWNED_CLAUDE_KEY", "")
		case "workspace":
			copy.BaseCommit = strings.Repeat("f", 40)
		case "asset":
			if err := os.WriteFile(filepath.Join(filepath.Dir(value.RuntimeManifest), "unlisted.js"), []byte("untrusted"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := prepareDesktopManagedExecution(context.Background(), copy, nil); err == nil {
			t.Fatalf("%s drift accepted", field)
		}
		t.Setenv("OWNED_CLAUDE_KEY", "owned-local-value201")
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatalf("%s drift started a native probe", field)
		}
	}
}
