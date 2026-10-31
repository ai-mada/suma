package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func localDiscoveryFile(t *testing.T, name, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	return name
}

func localDiscoveryNative(t *testing.T, root, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return localDiscoveryFile(t, filepath.Join(root, name), "owned native candidate; probe is injected")
}

func TestLocalToolDiscoveryOnlyFixedCandidatesAndNoExecutionAuthority(t *testing.T) {
	root := t.TempDir()
	localDiscoveryNative(t, root, "codex")
	calls := 0
	result, err := discoverInstalledTools(context.Background(), "", []string{root}, func(_ context.Context, spec CommandSpec) (string, error) {
		calls++
		if spec.Adapter != "codex" || spec.ReadOnly || spec.Input != "goal" || strings.Join(spec.Args, " ") != "exec --json --sandbox workspace-write --ephemeral -" {
			t.Fatalf("unsafe preset: %+v", spec)
		}
		return "codex-cli fixture", nil
	})
	if err != nil || result.Protocol != InstalledToolDiscoveryProtocol || len(result.Items) != 8 || calls != 1 {
		t.Fatalf("result=%+v calls=%d error=%v", result, calls, err)
	}
	first := result.Items[0]
	if first.AdapterID != "codex-jsonl" || !first.Installed || !first.LaunchSupported || first.ReasonCode != "workspace_required" || first.Command == nil || first.Version != "codex-cli fixture" {
		t.Fatalf("incorrect installation facts: %+v", first)
	}
	for _, item := range result.Items[1:] {
		if item.AdapterID == "codex-app-server" {
			if !item.Installed || item.LaunchSupported || item.Command != nil || item.ReasonCode != "unsupported_launcher" {
				t.Fatal("unverified native candidate acquired execution authority")
			}
			continue
		}
		if item.Installed || item.LaunchSupported || item.Command != nil || item.ReasonCode != "not_installed" {
			t.Fatalf("invented installed tool: %+v", item)
		}
	}
	raw, _ := json.Marshal(result)
	for _, forbidden := range []string{"available", "worker_id", "execution_enabled", "token"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("execution authority in local observation: %s", raw)
		}
	}
	if _, err := discoverInstalledTools(context.Background(), "../../private", []string{root}, nil); err == nil {
		t.Fatal("unknown directory ID accepted")
	}
}

func TestLocalToolDiscoveryPathBoundsDuplicatesAndRelativeEntries(t *testing.T) {
	root := t.TempDir()
	paths, err := localToolDiscoveryPaths(strings.Join([]string{"", ".", "relative", root, root}, string(os.PathListSeparator)))
	if err != nil || len(paths) != 1 || paths[0] != root {
		t.Fatalf("unsafe or duplicate PATH: %v %v", paths, err)
	}
	if _, err := localToolDiscoveryPaths(strings.Repeat("x", 32769)); err == nil {
		t.Fatal("oversized PATH accepted")
	}
	if _, err := localToolDiscoveryPaths(strings.Join(make([]string, 257), string(os.PathListSeparator))); err == nil {
		t.Fatal("unbounded PATH entries accepted")
	}
}

func TestLocalToolDiscoveryNeverExecutesShellShim(t *testing.T) {
	root := t.TempDir()
	localDiscoveryFile(t, filepath.Join(root, "codebuddy.cmd"), "@echo secret-token")
	calls := 0
	result, err := discoverInstalledTools(context.Background(), "codebuddy-cli", []string{root}, func(context.Context, CommandSpec) (string, error) { calls++; return "unexpected", nil })
	if err != nil || calls != 0 || len(result.Items) != 1 {
		t.Fatalf("shim was executed: %+v %d %v", result, calls, err)
	}
	item := result.Items[0]
	if !item.Installed || item.LaunchSupported || item.Command != nil || item.ReasonCode != "unsupported_launcher" {
		t.Fatalf("shell shim claimed supported: %+v", item)
	}
}

func TestLocalToolDiscoveryKnownPackageBinIsBoundedAndConfined(t *testing.T) {
	for _, scenario := range []struct {
		name, manifest, entry string
		supported             bool
	}{
		{"valid", `{"name":"@tencent-ai/codebuddy-code","bin":{"codebuddy":"dist/cli.cjs"}}`, "dist/cli.cjs", true},
		{"string-bin", `{"name":"@tencent-ai/codebuddy-code","bin":"dist/cli.cjs"}`, "dist/cli.cjs", true},
		{"escape", `{"name":"@tencent-ai/codebuddy-code","bin":{"codebuddy":"../../outside.cjs"}}`, "", false},
		{"absolute", `{"name":"@tencent-ai/codebuddy-code","bin":{"codebuddy":"/outside.cjs"}}`, "", false},
		{"duplicate", `{"name":"@tencent-ai/codebuddy-code","bin":{"codebuddy":"dist/cli.cjs","codebuddy":"../outside.cjs"}}`, "dist/cli.cjs", false},
		{"wrong-package", `{"name":"untrusted-package","bin":{"codebuddy":"dist/cli.cjs"}}`, "dist/cli.cjs", false},
		{"shell-entry", `{"name":"@tencent-ai/codebuddy-code","bin":{"codebuddy":"cli.cmd"}}`, "cli.cmd", false},
		{"over-limit", `{"name":"@tencent-ai/codebuddy-code","padding":"` + strings.Repeat("x", 64<<10) + `","bin":{"codebuddy":"dist/cli.cjs"}}`, "dist/cli.cjs", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			localDiscoveryNative(t, root, "node")
			pkg := filepath.Join(root, "node_modules", "@tencent-ai", "codebuddy-code")
			localDiscoveryFile(t, filepath.Join(pkg, "package.json"), scenario.manifest)
			if scenario.entry != "" {
				localDiscoveryFile(t, filepath.Join(pkg, scenario.entry), "not executed by unit probe")
			}
			calls := 0
			result, err := discoverInstalledTools(context.Background(), "codebuddy-cli", []string{root}, func(_ context.Context, spec CommandSpec) (string, error) {
				calls++
				if !filepath.IsAbs(spec.Args[0]) || spec.Adapter != "codebuddy" || spec.ReadOnly {
					t.Fatalf("not a fixed Node development preset: %+v", spec)
				}
				return "codebuddy fixture", nil
			})
			if err != nil || len(result.Items) != 1 {
				t.Fatalf("%+v %v", result, err)
			}
			item := result.Items[0]
			if !item.Installed || item.LaunchSupported != scenario.supported || (calls == 1) != scenario.supported {
				t.Fatalf("wrong confinement decision: %+v calls=%d", item, calls)
			}
		})
	}
}

func TestLocalToolDiscoveryProbeFailureIsInstalledWithoutCommandOrDiagnostics(t *testing.T) {
	root := t.TempDir()
	localDiscoveryNative(t, root, "codex")
	result, err := discoverInstalledTools(context.Background(), "codex-jsonl", []string{root}, func(context.Context, CommandSpec) (string, error) { return "", os.ErrPermission })
	if err != nil {
		t.Fatal(err)
	}
	item := result.Items[0]
	if !item.Installed || item.LaunchSupported || item.Command != nil || item.Version != "" || item.ReasonCode != "probe_failed" {
		t.Fatalf("incorrect failed probe facts: %+v", item)
	}
}

func TestLocalToolDiscoveryRejectsUnsafeVersionText(t *testing.T) {
	root := t.TempDir()
	localDiscoveryNative(t, root, "codex")
	for _, version := range []string{"codex\x1b[31m", "bad\tvalue", strings.Repeat("x", 257), string([]byte{0xff})} {
		result, err := discoverInstalledTools(context.Background(), "codex-jsonl", []string{root}, func(context.Context, CommandSpec) (string, error) { return version, nil })
		if err != nil || result.Items[0].LaunchSupported || result.Items[0].Version != "" || result.Items[0].Command != nil || result.Items[0].ReasonCode != "probe_failed" {
			t.Fatalf("unsafe version surfaced: %+v %v", result, err)
		}
	}
}

func TestLocalToolDiscoveryCodexNativePreferredAndKnownVendorResolved(t *testing.T) {
	for _, layout := range []string{"native", "vendor", "nested-platform", "sibling-platform"} {
		t.Run(layout, func(t *testing.T) {
			root := t.TempDir()
			pkg := filepath.Join(root, "node_modules", "@openai", "codex")
			localDiscoveryFile(t, filepath.Join(pkg, "package.json"), `{"name":"@openai/codex","bin":{"codex":"bin/codex.js"}}`)
			localDiscoveryFile(t, filepath.Join(root, "codex.cmd"), "never execute this shell launcher")
			arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
			suffix := map[string]string{"windows": "pc-windows-msvc", "linux": "unknown-linux-musl", "darwin": "apple-darwin"}[runtime.GOOS]
			if arch == "" || suffix == "" {
				t.Skip("vendor mapping not supported on this platform")
			}
			vendor := filepath.Join(pkg, "vendor", arch+"-"+suffix, "bin")
			if strings.Contains(layout, "platform") {
				platformName := runtime.GOOS
				if platformName == "windows" {
					platformName = "win32"
				}
				packageArch := runtime.GOARCH
				if packageArch == "amd64" {
					packageArch = "x64"
				}
				name := "@openai/codex-" + platformName + "-" + packageArch
				modules := filepath.Join(root, "node_modules")
				if layout == "nested-platform" {
					modules = filepath.Join(pkg, "node_modules")
				}
				platformRoot := filepath.Join(modules, filepath.FromSlash(name))
				localDiscoveryFile(t, filepath.Join(platformRoot, "package.json"), `{"name":"`+name+`"}`)
				vendor = filepath.Join(platformRoot, "vendor", arch+"-"+suffix, "bin")
			}
			want := localDiscoveryNative(t, vendor, "codex")
			if layout == "native" {
				want = localDiscoveryNative(t, root, "codex")
			}
			calls := 0
			result, err := discoverInstalledTools(context.Background(), "codex-jsonl", []string{root}, func(_ context.Context, spec CommandSpec) (string, error) {
				calls++
				if spec.Executable != want {
					t.Fatalf("wrong native target: %s want %s", spec.Executable, want)
				}
				return "codex-cli fixture", nil
			})
			if err != nil || calls != 1 || !result.Items[0].LaunchSupported {
				t.Fatalf("native/vendor discovery: %+v %v calls=%d", result, err, calls)
			}
		})
	}
}

func TestLocalToolDiscoveryPackageLinkCannotEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	localDiscoveryNative(t, root, "node")
	pkg := filepath.Join(root, "node_modules", "@tencent-ai", "codebuddy-code")
	localDiscoveryFile(t, filepath.Join(pkg, "package.json"), `{"name":"@tencent-ai/codebuddy-code","bin":{"codebuddy":"linked/cli.cjs"}}`)
	localDiscoveryFile(t, filepath.Join(outside, "cli.cjs"), "must never be probed")
	if err := os.Symlink(outside, filepath.Join(pkg, "linked")); err != nil {
		t.Skip("directory symlinks unavailable")
	}
	result, err := discoverInstalledTools(context.Background(), "codebuddy-cli", []string{root}, func(context.Context, CommandSpec) (string, error) {
		t.Error("external package link was probed")
		return "unsafe", nil
	})
	if err != nil || result.Items[0].LaunchSupported || result.Items[0].ReasonCode != "invalid_package" {
		t.Fatalf("link escaped package: %+v %v", result, err)
	}
}

func TestLocalToolDiscoveryPackageWithoutNodeRemainsInstalled(t *testing.T) {
	root := t.TempDir()
	pkg := filepath.Join(root, "node_modules", "@deepseek-ai", "dsh")
	localDiscoveryFile(t, filepath.Join(pkg, "package.json"), `{"name":"@deepseek-ai/dsh","bin":{"dsh":"bin/dsh.mjs"}}`)
	localDiscoveryFile(t, filepath.Join(pkg, "bin", "dsh.mjs"), "not executable without Node")
	result, err := discoverInstalledTools(context.Background(), "dsh-multica", []string{root}, nil)
	if err != nil || !result.Items[0].Installed || result.Items[0].LaunchSupported || result.Items[0].ReasonCode != "node_unavailable" || result.Items[0].Command != nil {
		t.Fatalf("missing Node reported incorrectly: %+v %v", result, err)
	}
}

func TestLocalToolDiscoveryActualNodePackageRunsOnlyVersionAndHelp(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("actual Node unavailable")
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	pkg := filepath.Join(root, "node_modules", "@tencent-ai", "codebuddy-code")
	localDiscoveryFile(t, filepath.Join(pkg, "package.json"), `{"name":"@tencent-ai/codebuddy-code","bin":{"codebuddy":"cli.cjs"}}`)
	localDiscoveryFile(t, filepath.Join(pkg, "cli.cjs"), `const a=process.argv.slice(2); if(process.env.SUMA_RUNNER_TOKEN||process.env.SUMA_AGENT_TOKEN)process.exit(81); if(a.length===1&&a[0]==='--version')console.log('codebuddy owned transport fixture'); else if(a.length===1&&a[0]==='--help')console.log('--input-format --output-format --verbose'); else process.exit(82);`)
	t.Setenv("SUMA_RUNNER_TOKEN", "must-not-reach-probe")
	t.Setenv("SUMA_AGENT_TOKEN", "must-not-reach-probe")
	result, err := discoverInstalledTools(context.Background(), "codebuddy-cli", []string{root, filepath.Dir(node)}, localToolDiscoveryVersion)
	if err != nil || len(result.Items) != 1 || !result.Items[0].LaunchSupported || result.Items[0].Version != "codebuddy owned transport fixture" {
		t.Fatalf("actual protocol probe: %+v %v", result, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "node_modules" {
		t.Fatalf("probe created state: %v %v", entries, err)
	}
}

func TestLocalToolDiscoveryActualHarnessRequiresItsMulticaProbe(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("actual Node unavailable")
	}
	node, err = filepath.Abs(node)
	if err != nil {
		t.Fatal(err)
	}
	for _, supported := range []bool{true, false} {
		name := "missing-profile"
		if supported {
			name = "supported-profile"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			pkg := filepath.Join(root, "node_modules", "@deepseek-ai", "dsh")
			localDiscoveryFile(t, filepath.Join(pkg, "package.json"), `{"name":"@deepseek-ai/dsh","bin":{"dsh":"dsh.cjs"}}`)
			probe := `console.error('not configured, token=not-public');process.exit(17);`
			if supported {
				probe = `console.log(JSON.stringify({v:1,type:'probe',runtime:'dsh',plugin_version:'owned-fixture',protocol_version:1}));`
			}
			localDiscoveryFile(t, filepath.Join(pkg, "dsh.cjs"), `const a=process.argv.slice(2).join(' ');if(a==='--version')console.log('dsh owned transport fixture');else if(a==='--profile multica --probe'){`+probe+`}else process.exit(82);`)
			result, err := discoverInstalledTools(context.Background(), "dsh-multica", []string{root, filepath.Dir(node)}, localToolDiscoveryVersion)
			if err != nil || !result.Items[0].Installed || result.Items[0].LaunchSupported != supported {
				t.Fatalf("actual Harness probe: %+v %v", result, err)
			}
			raw, _ := json.Marshal(result)
			if strings.Contains(string(raw), "not-public") || (!supported && result.Items[0].Command != nil) {
				t.Fatalf("unproven profile or diagnostics leaked: %s", raw)
			}
		})
	}
}
