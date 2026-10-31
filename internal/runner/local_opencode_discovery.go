package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func discoverLocalOpenCode(ctx context.Context, paths []string, probe func(context.Context, CommandSpec) (string, error)) InstalledToolDiscoveryItem {
	item := InstalledToolDiscoveryItem{AdapterID: "opencode-sdk", Product: "OpenCode", ReasonCode: "not_installed"}
	exe, sha, installed, reason := resolveLocalOpenCode(ctx, paths)
	item.Installed, item.ReasonCode = installed, reason
	if reason != "resolved" {
		return item
	}
	version, err := probe(ctx, CommandSpec{Adapter: "opencode", Executable: exe})
	if err != nil || !localToolSafeText(version, 256) || strings.TrimSpace(version) != version || !sameACPProbeInstallation(ctx, exe, sha) {
		item.ReasonCode = "probe_failed"
		return item
	}
	item.Executable, item.ExecutableSHA256, item.Version, item.ReasonCode = exe, sha, version, "configuration_required"
	if version != tc.OpenCodeSDKVersion {
		item.ReasonCode = "version_unsupported"
	}
	return item // Passive installation facts never supply a launch command.
}

func resolveLocalOpenCode(ctx context.Context, paths []string) (string, string, bool, string) {
	installed, reason := false, "not_installed"
	name := "opencode"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	for _, directory := range paths {
		candidate := filepath.Join(directory, name)
		if _, err := os.Lstat(candidate); err == nil {
			installed, reason = true, "unsupported_launcher"
			if exe, sha, err := qoderNativeTarget(ctx, candidate); err == nil {
				return exe, sha, true, "resolved"
			}
		}
		for _, suffix := range []string{".cmd", ".bat", ".ps1"} {
			if _, err := os.Lstat(filepath.Join(directory, "opencode"+suffix)); err == nil {
				installed, reason = true, "unsupported_launcher"
			}
		}
	}
	// Official build.ts at the reviewed source pin names native optional
	// packages opencode-{windows|darwin|linux}-{x64|arm64}[-baseline][-musl]
	// and places the executable in bin/. Read package facts only; never parse
	// or execute the opencode-ai JS/shell launcher or its environment override.
	platform := runtime.GOOS
	arch := map[string]string{"amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	if arch == "" || platform != "windows" && platform != "darwin" && platform != "linux" {
		return "", "", installed, reason
	}
	packageNames := []string{"opencode-" + platform + "-" + arch}
	if arch == "x64" {
		packageNames = append(packageNames, packageNames[0]+"-baseline")
	}
	if platform == "linux" {
		packageNames = append(packageNames, "opencode-linux-"+arch+"-musl")
		if arch == "x64" {
			packageNames = append(packageNames, "opencode-linux-x64-baseline-musl")
		}
	}
	seen := map[string]bool{}
	for _, directory := range paths {
		moduleRoots := []string{filepath.Join(directory, "node_modules")}
		if filepath.Base(directory) == "bin" {
			moduleRoots = append(moduleRoots, filepath.Join(filepath.Dir(directory), "lib", "node_modules"))
		}
		for _, modules := range moduleRoots {
			if seen[modules] {
				continue
			}
			seen[modules] = true
			pkg := filepath.Join(modules, "opencode-ai")
			if _, err := os.Lstat(pkg); err != nil {
				continue
			}
			installed, reason = true, "invalid_package"
			manifest, err := localToolPackageManifest(modules, "opencode-ai")
			if err != nil {
				continue
			}
			optional, ok := manifest["optionalDependencies"].(map[string]any)
			if !ok {
				reason = "unsupported_launcher"
				continue
			}
			for _, packageName := range packageNames {
				version, declared := optional[packageName].(string)
				if !declared || !localToolSafeText(version, 256) {
					continue
				}
				for _, root := range []string{filepath.Join(pkg, "node_modules"), modules} {
					if _, err := localToolPackageManifest(root, packageName); err != nil {
						continue
					}
					candidate, err := localToolPackageFile(root, filepath.Join(packageName, "bin", name), 1<<30)
					if err != nil {
						continue
					}
					if exe, sha, err := qoderNativeTarget(ctx, candidate); err == nil {
						return exe, sha, true, "resolved"
					}
				}
			}
			reason = "unsupported_launcher"
		}
	}
	return "", "", installed, reason
}

// --version runs in its own empty HOME with only OS process mechanics. Never
// inherit provider keys, Node hooks, OpenCode config, npm state or user login.
func passiveOpenCodeVersion(parent context.Context, exe string) (string, error) {
	directory, cleanup, err := newACPProbeDirectory()
	if err != nil {
		return "", err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.Command(exe, "--version")
	cmd.Dir = directory
	base, _, err := commandEnvironment(CommandSpec{})
	if err != nil {
		return "", fmt.Errorf("OpenCode version environment unavailable")
	}
	for _, value := range base {
		name, _, _ := strings.Cut(value, "=")
		if map[string]bool{"PATH": true, "SYSTEMROOT": true, "WINDIR": true, "COMSPEC": true, "PATHEXT": true, "LANG": true}[strings.ToUpper(name)] {
			cmd.Env = append(cmd.Env, value)
		}
	}
	for _, name := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "TEMP", "TMP", "TMPDIR"} {
		cmd.Env = append(cmd.Env, name+"="+directory)
	}
	cmd.Env = append(cmd.Env, "OPENCODE_DISABLE_AUTOUPDATE=true", "OPENCODE_DISABLE_MODELS_FETCH=true")
	out, diagnostic := newOutput(32<<10, nil), newOutput(32<<10, nil)
	cmd.Stdout, cmd.Stderr = out, diagnostic
	controller, err := startProcess(cmd)
	if err != nil {
		return "", fmt.Errorf("OpenCode version probe unavailable")
	}
	defer controller.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-ctx.Done():
		_ = controller.Stop()
		waitErr = <-done
	}
	stopErr := controller.Finish()
	out.mu.Lock()
	raw, truncated := out.b.String(), out.truncated
	out.mu.Unlock()
	diagnostic.mu.Lock()
	truncated = truncated || diagnostic.truncated
	diagnostic.mu.Unlock()
	version := strings.TrimSpace(raw)
	if waitErr != nil || stopErr != nil || ctx.Err() != nil || truncated || !localToolSafeText(version, 256) || strings.TrimSpace(out.String()) != version {
		return "", fmt.Errorf("OpenCode version probe was not confirmed")
	}
	return version, nil
}
