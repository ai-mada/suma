package runner

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type openCodeRuntimeAsset struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
}

type openCodeRuntimeMetadata struct {
	SDKVersion        string `json:"sdk_version"`
	ServerVersion     string `json:"server_version"`
	ToolEntry         string `json:"tool_entry"`
	PluginPackageRoot string `json:"plugin_package_root"`
}

func validateOpenCodeRuntimeAssets(spec CommandSpec, manifest nativeRuntimeManifest) error {
	m := manifest.OpenCode
	if m == nil || m.SDKVersion != tc.OpenCodeSDKVersion || m.ServerVersion != tc.OpenCodeSDKVersion || m.ToolEntry != "suma-workspace.tool.mjs" || m.PluginPackageRoot != "opencode-plugin" || manifest.Entry != "suma-opencode.bundle.mjs" || manifest.Dependencies["@opencode-ai/sdk"] != tc.OpenCodeSDKVersion || manifest.Dependencies["@opencode-ai/plugin"] != tc.OpenCodeSDKVersion || manifest.Dependencies["@modelcontextprotocol/client"] != "2.4.0" || len(manifest.Assets) < 5 || len(manifest.Assets) > 32768 {
		return fmt.Errorf("OpenCode runtime manifest does not close the supported SDK and trusted tools")
	}
	root := filepath.Dir(spec.Args[0])
	seen := map[string]bool{}
	var total int64
	for _, asset := range manifest.Assets {
		name := asset.Path
		if name == "" || strings.ContainsAny(name, "\\:\x00\r\n") || strings.HasPrefix(name, "/") || filepath.ToSlash(filepath.Clean(filepath.FromSlash(name))) != name || name == "." || strings.HasPrefix(name, "../") || seen[strings.ToLower(name)] || !tc.ValidDigest(asset.SHA256) || asset.SizeBytes < 0 || asset.SizeBytes > 16<<20 {
			return fmt.Errorf("OpenCode runtime asset identity is invalid")
		}
		if name != manifest.Entry && name != m.ToolEntry && !strings.HasPrefix(name, m.PluginPackageRoot+"/") {
			return fmt.Errorf("OpenCode runtime asset is outside the fixed payload")
		}
		total += asset.SizeBytes
		if total > 256<<20 {
			return fmt.Errorf("OpenCode runtime payload exceeds its bound")
		}
		path := filepath.Join(root, filepath.FromSlash(name))
		resolved, err := filepath.EvalSymlinks(path)
		st, statErr := os.Lstat(path)
		if err != nil || statErr != nil || !st.Mode().IsRegular() || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(path)) || st.Size() != asset.SizeBytes {
			return fmt.Errorf("OpenCode runtime asset is missing, linked or changed")
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("OpenCode runtime asset is unavailable")
		}
		opened, statErr := f.Stat()
		data, readErr := io.ReadAll(io.LimitReader(f, (16<<20)+1))
		_ = f.Close()
		if statErr != nil || !os.SameFile(st, opened) || readErr != nil || int64(len(data)) != asset.SizeBytes || digest(data) != asset.SHA256 {
			return fmt.Errorf("OpenCode runtime asset digest changed")
		}
		seen[strings.ToLower(name)] = true
	}
	for _, required := range []string{manifest.Entry, m.ToolEntry, m.PluginPackageRoot + "/package.json", m.PluginPackageRoot + "/package-lock.json", m.PluginPackageRoot + "/node_modules/@opencode-ai/plugin/package.json"} {
		if !seen[strings.ToLower(required)] {
			return fmt.Errorf("OpenCode runtime payload is incomplete")
		}
	}
	return filepath.WalkDir(filepath.Join(root, m.PluginPackageRoot), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("OpenCode plugin payload is unavailable")
		}
		resolved, resolveErr := filepath.EvalSymlinks(path)
		if entry.Type()&os.ModeSymlink != 0 || resolveErr != nil || !strings.EqualFold(filepath.Clean(resolved), filepath.Clean(path)) {
			return fmt.Errorf("OpenCode plugin payload traverses a link")
		}
		if entry.IsDir() {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil || !seen[strings.ToLower(filepath.ToSlash(relative))] {
			return fmt.Errorf("OpenCode plugin payload contains an unpinned file")
		}
		return nil
	})
}

func openCodeRuntimePaths(spec CommandSpec) (string, string, error) {
	if _, err := nativeManifestDigest(spec); err != nil {
		return "", "", err
	}
	root := filepath.Dir(spec.Args[0])
	return filepath.Join(root, "suma-workspace.tool.mjs"), filepath.Join(root, "opencode-plugin"), nil
}
