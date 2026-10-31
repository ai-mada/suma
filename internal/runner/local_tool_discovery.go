package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const InstalledToolDiscoveryProtocol = "suma-installed-tools-v1"

// InstalledToolDiscovery describes this computer only. It is neither Runner
// inventory nor permission to dispatch, and never contains platform credentials.
type InstalledToolDiscovery struct {
	Protocol string                       `json:"protocol"`
	Items    []InstalledToolDiscoveryItem `json:"items"`
}

type InstalledToolDiscoveryItem struct {
	AdapterID        string       `json:"adapter_id"`
	Product          string       `json:"product"`
	Installed        bool         `json:"installed"`
	LaunchSupported  bool         `json:"launch_supported"`
	Executable       string       `json:"executable,omitempty"`
	ExecutableSHA256 string       `json:"executable_sha256,omitempty"`
	Version          string       `json:"version,omitempty"`
	ReasonCode       string       `json:"reason_code"`
	Command          *CommandSpec `json:"command,omitempty"`
}

type localToolDirectoryEntry struct {
	id, product, binary, packageName string
	command                          CommandSpec
}

func localToolDirectory() []localToolDirectoryEntry {
	return []localToolDirectoryEntry{
		{"codex-jsonl", "Codex", "codex", "@openai/codex", CommandSpec{Adapter: "codex", Protocol: "codex-jsonl-v1", Input: "goal", Args: []string{"exec", "--json", "--sandbox", "workspace-write", "--ephemeral", "-"}}},
		{"codebuddy-cli", "CodeBuddy Code", "codebuddy", "@tencent-ai/codebuddy-code", CommandSpec{Adapter: "codebuddy", Protocol: "codebuddy-jsonl-v1", Input: "goal", Args: []string{"-p", "--input-format", "text", "--output-format", "stream-json", "--verbose", "--permission-mode", "acceptEdits"}}},
		{"dsh-multica", "DeepSeek Harness / dsh", "dsh", "@deepseek-ai/dsh", CommandSpec{Adapter: "harness", Protocol: "dsh-multica-jsonl-v1", Input: "dsh-jsonl", Args: []string{"--profile", "multica", "--stdio"}}},
		{"claude-code-cli", "Claude Code", "claude", "@anthropic-ai/claude-code", claudePreset()},
		{id: "cursor-cli", product: "Cursor"},
		{id: "qoder-acp", product: "Qoder CLI"},
	}
}

// DiscoverInstalledTools inspects only absolute PATH entries and fixed package
// locations relative to them. No npm invocation, login file, model, journal,
// repository, installation or task execution is involved.
func DiscoverInstalledTools(ctx context.Context, adapterID string) (InstalledToolDiscovery, error) {
	paths, err := localToolDiscoveryPaths(os.Getenv("PATH"))
	if err != nil {
		return InstalledToolDiscovery{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	return discoverInstalledTools(ctx, adapterID, paths, localToolDiscoveryVersion)
}

func localToolDiscoveryPaths(raw string) ([]string, error) {
	if len(raw) > 32768 || strings.ContainsRune(raw, 0) {
		return nil, fmt.Errorf("local tool discovery PATH exceeds its bound")
	}
	parts := filepath.SplitList(raw)
	if len(parts) > 256 {
		return nil, fmt.Errorf("local tool discovery PATH exceeds its bound")
	}
	paths := []string{}
	seen := map[string]bool{}
	for _, part := range parts {
		if !filepath.IsAbs(part) || !localToolSafeText(part, 4096) {
			continue
		}
		part = filepath.Clean(part)
		key := part
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if !seen[key] {
			seen[key] = true
			paths = append(paths, part)
		}
	}
	return paths, nil
}

func discoverInstalledTools(ctx context.Context, adapterID string, paths []string, probe func(context.Context, CommandSpec) (string, error)) (InstalledToolDiscovery, error) {
	entries := localToolDirectory()
	if adapterID != "" {
		found := false
		for _, entry := range entries {
			if entry.id == adapterID {
				entries = []localToolDirectoryEntry{entry}
				found = true
				break
			}
		}
		if !found {
			return InstalledToolDiscovery{}, fmt.Errorf("unsupported local tool directory ID")
		}
	}
	result := InstalledToolDiscovery{Protocol: InstalledToolDiscoveryProtocol, Items: make([]InstalledToolDiscoveryItem, len(entries))}
	done := make(chan struct{}, len(entries))
	for index, entry := range entries {
		go func(index int, entry localToolDirectoryEntry) {
			defer func() { done <- struct{}{} }()
			if entry.id == "qoder-acp" {
				result.Items[index] = discoverLocalQoder(ctx, paths, probe)
				return
			}
			if entry.id == "cursor-cli" {
				result.Items[index] = discoverLocalCursor(ctx, paths)
				return
			}
			item := InstalledToolDiscoveryItem{AdapterID: entry.id, Product: entry.product, ReasonCode: "not_installed"}
			spec, installed, reason := resolveLocalTool(entry, paths)
			item.Installed, item.ReasonCode = installed, reason
			if reason == "resolved" {
				item.Executable = spec.Executable
				version, err := probe(ctx, spec)
				if err != nil || !localToolSafeText(version, 256) {
					item.ReasonCode = "probe_failed"
				} else {
					item.Version, item.Command, item.LaunchSupported, item.ReasonCode = version, &spec, true, "workspace_required"
				}
			}
			result.Items[index] = item
		}(index, entry)
	}
	for range entries {
		<-done
	}
	if ctx.Err() != nil {
		return InstalledToolDiscovery{}, fmt.Errorf("local tool discovery cancelled or timed out")
	}
	return result, nil
}

func localToolDiscoveryVersion(ctx context.Context, spec CommandSpec) (string, error) {
	exe, _, err := commandExecutable(spec)
	if err != nil {
		return "", err
	}
	if spec.Adapter == "qoder" {
		directory, cleanup, err := newACPProbeDirectory()
		if err != nil {
			return "", err
		}
		defer cleanup()
		return probeQoderNativeVersion(ctx, exe, directory, nil)
	}
	// Reuse each adapter's fixed version/help/protocol proof, without changing a
	// writing development preset into a read-only execution profile.
	return toolVersion(ctx, exe, normalizedCommand(spec), nil, "")
}

func resolveLocalTool(entry localToolDirectoryEntry, paths []string) (CommandSpec, bool, string) {
	installed, reason := false, "not_installed"
	for _, directory := range paths {
		name := entry.binary
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		candidate := filepath.Join(directory, name)
		if _, err := os.Lstat(candidate); err == nil {
			installed, reason = true, "unsupported_launcher"
			if spec, ok := localToolNativeSpec(entry.command, candidate); ok {
				return spec, true, "resolved"
			}
		}
		for _, extension := range []string{".cmd", ".bat", ".ps1"} {
			if _, err := os.Lstat(filepath.Join(directory, entry.binary+extension)); err == nil {
				installed, reason = true, "unsupported_launcher"
			}
		}
	}
	seen := map[string]bool{}
	for _, directory := range paths {
		modules := []string{filepath.Join(directory, "node_modules")}
		if filepath.Base(directory) == "bin" {
			modules = append(modules, filepath.Join(filepath.Dir(directory), "lib", "node_modules"))
		}
		for _, moduleRoot := range modules {
			if seen[moduleRoot] {
				continue
			}
			seen[moduleRoot] = true
			pkg := filepath.Join(moduleRoot, filepath.FromSlash(entry.packageName))
			if _, err := os.Lstat(pkg); err != nil {
				continue
			}
			installed, reason = true, "invalid_package"
			manifest, err := localToolPackageManifest(moduleRoot, entry.packageName)
			if err != nil {
				continue
			}
			if entry.binary == "claude" {
				reason = "unsupported_launcher"
				continue
			}
			if entry.binary == "codex" {
				if spec, ok := localToolCodexVendor(entry.command, moduleRoot, pkg); ok {
					return spec, true, "resolved"
				}
				reason = "unsupported_launcher"
				continue
			}
			bin := ""
			switch value := manifest["bin"].(type) {
			case string:
				bin = value
			case map[string]any:
				bin, _ = value[entry.binary].(string)
			}
			entryPath, err := localToolPackageFile(pkg, bin, 16<<20)
			if err != nil || !localToolJavaScript(entryPath) {
				continue
			}
			node := localToolNode(paths)
			if node == "" {
				reason = "node_unavailable"
				continue
			}
			spec := entry.command
			spec.Executable, spec.Args = node, append([]string{entryPath}, spec.Args...)
			if validateTeamCLICommand(spec) != nil {
				continue
			}
			return spec, true, "resolved"
		}
	}
	return CommandSpec{}, installed, reason
}

func localToolNativeSpec(preset CommandSpec, candidate string) (CommandSpec, bool) {
	resolved, err := resolveExecutableTarget(candidate)
	if err != nil || !filepath.IsAbs(resolved) || !localToolSafeText(resolved, 4096) {
		return CommandSpec{}, false
	}
	extension := strings.ToLower(filepath.Ext(resolved))
	if extension == ".cmd" || extension == ".bat" || extension == ".ps1" || localToolJavaScript(resolved) {
		return CommandSpec{}, false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<30 || runtime.GOOS != "windows" && info.Mode()&0111 == 0 {
		return CommandSpec{}, false
	}
	preset.Executable = resolved
	return preset, true
}

func localToolNode(paths []string) string {
	for _, directory := range paths {
		name := "node"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if spec, ok := localToolNativeSpec(CommandSpec{}, filepath.Join(directory, name)); ok {
			return spec.Executable
		}
	}
	return ""
}

func localToolJavaScript(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".js", ".mjs", ".cjs":
		return true
	}
	return false
}

func localToolSafeText(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || character == '\u2028' || character == '\u2029' {
			return false
		}
	}
	return true
}

func localToolPackageManifest(modules, name string) (map[string]any, error) {
	file, err := localToolPackageFile(modules, filepath.FromSlash(name)+string(filepath.Separator)+"package.json", 64<<10)
	if err != nil {
		return nil, err
	}
	opened, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer opened.Close()
	raw, err := io.ReadAll(io.LimitReader(opened, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return nil, fmt.Errorf("invalid package metadata")
	}
	manifest, err := decodeToolObject(raw)
	if err != nil || manifest["name"] != name {
		return nil, fmt.Errorf("invalid package metadata")
	}
	return manifest, nil
}

// Every package component must remain inside its fixed module root. A package
// bin is data, never a shell command or permission to follow an external link.
func localToolPackageFile(root, relative string, limit int64) (string, error) {
	if !filepath.IsLocal(relative) || !localToolSafeText(relative, 4096) {
		return "", fmt.Errorf("invalid package entry")
	}
	if info, err := os.Lstat(root); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("invalid package root")
	}
	current := root
	parts := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("invalid package entry")
		}
		if index < len(parts)-1 {
			if !info.IsDir() {
				return "", fmt.Errorf("invalid package entry")
			}
		} else if !info.Mode().IsRegular() || info.Size() > limit {
			return "", fmt.Errorf("invalid package entry")
		}
	}
	return current, nil
}

func localToolCodexVendor(preset CommandSpec, modules, pkg string) (CommandSpec, bool) {
	arch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[runtime.GOARCH]
	suffix := map[string]string{"windows": "pc-windows-msvc", "darwin": "apple-darwin", "linux": "unknown-linux-musl"}[runtime.GOOS]
	packageOS := runtime.GOOS
	if packageOS == "windows" {
		packageOS = "win32"
	}
	packageArch := runtime.GOARCH
	if packageArch == "amd64" {
		packageArch = "x64"
	}
	if arch == "" || suffix == "" {
		return CommandSpec{}, false
	}
	name := "codex"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	relative := filepath.Join("vendor", arch+"-"+suffix, "bin", name)
	if candidate, err := localToolPackageFile(pkg, relative, 1<<30); err == nil {
		if spec, ok := localToolNativeSpec(preset, candidate); ok {
			return spec, true
		}
	}
	platformPackage := "@openai/codex-" + packageOS + "-" + packageArch
	for _, root := range []string{filepath.Join(pkg, "node_modules"), modules} {
		if _, err := localToolPackageManifest(root, platformPackage); err != nil {
			continue
		}
		if candidate, err := localToolPackageFile(root, filepath.Join(filepath.FromSlash(platformPackage), relative), 1<<30); err == nil {
			if spec, ok := localToolNativeSpec(preset, candidate); ok {
				return spec, true
			}
		}
	}
	return CommandSpec{}, false
}
