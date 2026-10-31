package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Only a uniquely named copy of the native test binary enters this bounded
// fixture. It accepts --version alone, never SDK serving or a model request.
func init() {
	exe, err := os.Executable()
	if err != nil || strings.TrimSuffix(filepath.Base(exe), ".exe") != "suma-opencode177-fixture" {
		return
	}
	code := 0
	if len(os.Args) != 2 || os.Args[1] != "--version" || os.Getenv("SUMA_RUNNER_TOKEN") != "" || os.Getenv("OPENCODE_CONFIG") != "" || os.Getenv("NODE_OPTIONS") != "" {
		code = 81
	}
	cwd, _ := os.Getwd()
	if os.Getenv("HOME") != cwd || os.Getenv("XDG_CONFIG_HOME") != cwd {
		code = 82
	}
	raw, _ := json.Marshal(map[string]any{"pid": os.Getpid(), "args": os.Args[1:], "isolated_home": os.Getenv("HOME") == cwd, "exit_code": code})
	_ = os.WriteFile(filepath.Join(filepath.Dir(exe), fmt.Sprintf("trace177-%d.json", os.Getpid())), raw, 0600)
	if code == 0 {
		fmt.Println("1.19.4")
	}
	os.Exit(code)
}

func openCode177NativeCopy(t *testing.T, root, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	source, _ := os.Executable()
	in, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	name = filepath.Join(root, name)
	out, err := os.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(out, in)
	if closeErr := out.Close(); copyErr != nil || closeErr != nil {
		t.Fatal("native fixture copy failed")
	}
	return name
}

func TestDesktopOpenCodeDiscovery(t *testing.T) {
	root := t.TempDir()
	exe := openCode177NativeCopy(t, root, "opencode")
	for _, version := range []string{"1.19.4", "1.19.5"} {
		calls := 0
		report, err := discoverInstalledTools(context.Background(), "opencode-sdk", []string{root}, func(_ context.Context, spec CommandSpec) (string, error) {
			calls++
			if spec.Executable != exe || len(spec.Args) != 0 {
				t.Fatal("passive probe accepted a launcher")
			}
			return version, nil
		})
		if err != nil || len(report.Items) != 1 {
			t.Fatalf("OpenCode missing from passive catalogue: %v", err)
		}
		item := report.Items[0]
		want := "configuration_required"
		if version != "1.19.4" {
			want = "version_unsupported"
		}
		if calls != 1 || !item.Installed || item.Product != "OpenCode" || item.Version != version || item.Executable != exe || len(item.ExecutableSHA256) != 64 || item.LaunchSupported || item.Command != nil || item.ReasonCode != want {
			t.Fatalf("incorrect passive facts: %+v", item)
		}
	}
	t.Run("official-platform-package", func(t *testing.T) {
		root := t.TempDir()
		platform := runtime.GOOS
		arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
		if arch == "" {
			t.Skip("unsupported package architecture")
		}
		name := "opencode-" + platform + "-" + arch
		pkg := filepath.Join(root, "node_modules", "opencode-ai")
		localDiscoveryFile(t, filepath.Join(root, "opencode.cmd"), "never execute launcher")
		localDiscoveryFile(t, filepath.Join(pkg, "package.json"), `{"name":"opencode-ai","optionalDependencies":{"`+name+`":"1.19.4"}}`)
		platformRoot := filepath.Join(pkg, "node_modules", name)
		localDiscoveryFile(t, filepath.Join(platformRoot, "package.json"), `{"name":"`+name+`"}`)
		exe := openCode177NativeCopy(t, filepath.Join(platformRoot, "bin"), "opencode")
		calls := 0
		report, err := discoverInstalledTools(context.Background(), "opencode-sdk", []string{root}, func(_ context.Context, spec CommandSpec) (string, error) {
			calls++
			if spec.Executable != exe {
				t.Fatal("platform native target not fixed")
			}
			return "1.19.4", nil
		})
		if err != nil || calls != 1 || report.Items[0].Executable != exe || report.Items[0].ReasonCode != "configuration_required" || report.Items[0].Command != nil {
			t.Fatalf("fixed official platform package unresolved: %+v %v", report, err)
		}
	})
}

func openCode177PrivateRaw(t *testing.T) []byte {
	t.Helper()
	input := desktopManagedFixtureInput(t)
	raw, _ := json.Marshal(input)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	exe := filepath.Join(t.TempDir(), "opencode.exe")
	provider := OpenCodeSpec{Executable: exe, ProviderID: "owned177", Model: "fixed177", BaseURL: "http://127.0.0.1:43177/v1"}
	body["adapter_id"] = "opencode-sdk"
	body["opencode_configuration"] = map[string]any{"executable": exe, "executable_sha256": strings.Repeat("a", 64), "reviewed_version": "1.19.4", "node_executable": filepath.Join(t.TempDir(), "node.exe"), "node_sha256": strings.Repeat("b", 64), "runtime_manifest": filepath.Join(t.TempDir(), "suma-opencode.manifest.json"), "runtime_manifest_sha256": strings.Repeat("c", 64), "provider_id": provider.ProviderID, "model": provider.Model, "base_url": provider.BaseURL, "configuration_sha256": OpenCodeConfigurationDigest(provider)}
	raw, _ = json.Marshal(body)
	return raw
}

func TestDesktopOpenCodePrivateInput(t *testing.T) {
	raw := openCode177PrivateRaw(t)
	if _, err := DecodeDesktopManagedRunnerInput(raw); err != nil {
		t.Fatalf("fixed OpenCode configuration not accepted: %v", err)
	}
	for _, mutate := range []func(map[string]any){
		func(c map[string]any) { c["shell"] = "arbitrary" },
		func(c map[string]any) { c["model"] = nil },
		func(c map[string]any) { delete(c, "node_sha256") },
		func(c map[string]any) { c["reviewed_version"] = "1.19.5" },
		func(c map[string]any) { c["configuration_sha256"] = strings.Repeat("0", 64) },
	} {
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mutate(body["opencode_configuration"].(map[string]any))
		bad, _ := json.Marshal(body)
		if _, err := DecodeDesktopManagedRunnerInput(bad); err == nil {
			t.Fatal("unclosed or changed OpenCode configuration accepted")
		}
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"model":"fixed177"`, `"model":"fixed177","model":"injected"`, 1),
		strings.Replace(string(raw), `"model":"fixed177"`, `"model":"\ud800"`, 1),
		strings.Replace(string(raw), `node.exe`, `node\ud800.exe`, 1),
		strings.Replace(string(raw), `"opencode_configuration":`, `"acp_configuration":`, 1),
	} {
		if _, err := DecodeDesktopManagedRunnerInput([]byte(bad)); err == nil {
			t.Fatal("duplicate, malformed Unicode, or mixed private config accepted")
		}
	}
}

func openCode177ManagedFixture(t *testing.T) (DesktopManagedRunnerInput, CommandSpec) {
	t.Helper()
	spec, _ := openCodeConfigFixture(t)
	exe := openCode177NativeCopy(t, t.TempDir(), "suma-opencode177-fixture")
	spec.OpenCode.Executable = exe
	native, sha, err := qoderNativeTarget(context.Background(), exe)
	if err != nil {
		t.Fatal(err)
	}
	node, nodeSHA, err := qoderNativeTarget(context.Background(), spec.Executable)
	if err != nil {
		t.Fatal(err)
	}
	spec.Executable = node
	manifestSHA, err := nativeManifestDigest(spec)
	if err != nil {
		t.Fatal(err)
	}
	repo, base := gitFixture(t)
	input := desktopManagedFixtureInput(t)
	input.AdapterID, input.Repository, input.BaseCommit = "opencode-sdk", repo, base
	input.OpenCodeConfiguration = &DesktopManagedOpenCodeConfiguration{Executable: native, ExecutableSHA256: sha, ReviewedVersion: "1.19.4", NodeExecutable: node, NodeSHA256: nodeSHA, RuntimeManifest: spec.NativeWorkspace.RuntimeManifest, RuntimeManifestSHA256: manifestSHA, ProviderID: spec.OpenCode.ProviderID, Model: spec.OpenCode.Model, BaseURL: spec.OpenCode.BaseURL, ConfigurationSHA256: OpenCodeConfigurationDigest(*spec.OpenCode)}
	return input, spec
}

func TestDesktopOpenCodePreparationUsesOriginalWorkerPolicy(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "private177")
	t.Setenv("OPENCODE_CONFIG", "foreign177")
	t.Setenv("NODE_OPTIONS", "--foreign177")
	input, _ := openCode177ManagedFixture(t)
	cfg, err := prepareDesktopManagedExecution(context.Background(), input, func(context.Context, string) (InstalledToolDiscovery, error) {
		t.Fatal("confirmed native pin rediscovered a different installation")
		return InstalledToolDiscovery{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	spec := cfg.Commands["desktop-opencode-sdk"]
	if !cfg.AllowExec || cfg.BaseCommit != input.BaseCommit || spec.Executable != input.OpenCodeConfiguration.NodeExecutable || spec.OpenCode == nil || spec.OpenCode.Executable != input.OpenCodeConfiguration.Executable || spec.Input != "opencode-sdk-jsonl" || spec.NativeWorkspace == nil || spec.NativeWorkspace.Checks == nil || len(spec.NativeWorkspace.Checks) != 0 || len(cfg.ToolInventory) != 0 {
		t.Fatal("managed preparation changed the original fixed SDK policy")
	}
	if _, err := os.Stat(input.StateDirectory); !os.IsNotExist(err) {
		t.Fatal("preparation opened a Journal")
	}
	if got := strings.TrimSpace(gitTest(t, input.Repository, "status", "--porcelain")); got != "" {
		t.Fatal("preparation changed Git source")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(input.OpenCodeConfiguration.Executable), "trace177-*.json"))
	if len(files) != 1 {
		t.Fatalf("preparation must run only the version probe, got %d", len(files))
	}
	var trace struct {
		Args     []string `json:"args"`
		Isolated bool     `json:"isolated_home"`
		ExitCode int      `json:"exit_code"`
	}
	raw, _ := os.ReadFile(files[0])
	_ = json.Unmarshal(raw, &trace)
	if strings.Join(trace.Args, " ") != "--version" || !trace.Isolated || trace.ExitCode != 0 {
		t.Fatal("native probe inherited account context or started SDK work")
	}
}

func TestDesktopOpenCodePreparationRejectsDriftBeforeProcess(t *testing.T) {
	input, spec := openCode177ManagedFixture(t)
	for name, mutate := range map[string]func(*DesktopManagedRunnerInput){
		"server": func(c *DesktopManagedRunnerInput) { c.OpenCodeConfiguration.ExecutableSHA256 = strings.Repeat("1", 64) },
		"node":   func(c *DesktopManagedRunnerInput) { c.OpenCodeConfiguration.NodeSHA256 = strings.Repeat("2", 64) },
		"manifest": func(c *DesktopManagedRunnerInput) {
			c.OpenCodeConfiguration.RuntimeManifestSHA256 = strings.Repeat("3", 64)
		},
		"workspace": func(c *DesktopManagedRunnerInput) { c.BaseCommit = strings.Repeat("4", 40) },
	} {
		t.Run(name, func(t *testing.T) {
			copy := input
			value := *input.OpenCodeConfiguration
			copy.OpenCodeConfiguration = &value
			mutate(&copy)
			if _, err := prepareDesktopManagedExecution(context.Background(), copy, nil); err == nil {
				t.Fatal("drift acquired execution configuration")
			}
		})
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(spec.Args[0]), "suma-workspace.tool.mjs"), []byte("changed trusted tool"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareDesktopManagedExecution(context.Background(), input, nil); err == nil {
		t.Fatal("changed runtime payload accepted")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(input.OpenCodeConfiguration.Executable), "trace177-*.json"))
	if len(files) != 0 {
		t.Fatal("invalid pins or unbound Git root started native probe")
	}
}
