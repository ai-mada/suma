package runner

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Literal JSON exercises administrator decoding, including unknown fields,
// without granting workflows a second configuration channel.
func openCodeConfigFixture(t *testing.T) (CommandSpec, map[string]any) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	binary := filepath.Join(root, "opencode-fixture.exe")
	if err = os.WriteFile(binary, []byte("fixture binary bytes, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "suma-opencode.bundle.mjs")
	files := map[string]string{
		"suma-opencode.bundle.mjs":                                      "// deterministic fixture entry\n",
		"suma-workspace.tool.mjs":                                       "// deterministic fixture trusted tool\n",
		"opencode-plugin/package.json":                                  `{"private":true,"dependencies":{"@opencode-ai/plugin":"1.19.4"}}`,
		"opencode-plugin/package-lock.json":                             `{"lockfileVersion":3,"packages":{"":{"dependencies":{"@opencode-ai/plugin":"1.19.4"}}}}`,
		"opencode-plugin/node_modules/@opencode-ai/plugin/package.json": `{"name":"@opencode-ai/plugin","version":"1.19.4"}`,
	}
	var assets []map[string]any
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		assets = append(assets, map[string]any{"path": name, "sha256": digest([]byte(content)), "size_bytes": len(content)})
	}
	manifest := map[string]any{
		"schema_version": 1, "entry": "suma-opencode.bundle.mjs", "entry_sha256": digest([]byte(files["suma-opencode.bundle.mjs"])),
		"external_imports": []string{"node:module"}, "dependencies": map[string]string{"@opencode-ai/sdk": "1.19.4", "@opencode-ai/plugin": "1.19.4", "@modelcontextprotocol/client": "2.4.0"}, "assets": assets,
		"opencode": map[string]string{"sdk_version": "1.19.4", "server_version": "1.19.4", "tool_entry": "suma-workspace.tool.mjs", "plugin_package_root": "opencode-plugin"},
	}
	manifestPath := filepath.Join(root, "suma-opencode.manifest.json")
	rawManifest, _ := json.Marshal(manifest)
	if err = os.WriteFile(manifestPath, rawManifest, 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"adapter": "opencode", "protocol": "opencode-sdk-v1", "executable": node, "args": []string{entry}, "input": "opencode-sdk-jsonl", "native_workspace": map[string]any{"checks": []string{}, "runtime_manifest": manifestPath}, "opencode": map[string]any{"executable": binary, "provider_id": "local104", "model": "coding104", "base_url": "http://127.0.0.1:49104/v1"}}
	rawConfig, _ := json.Marshal(map[string]any{"opencode104": config})
	commands, err := DecodeCommands(rawConfig)
	if err != nil {
		t.Fatalf("fixed host-workspace SDK profile rejected: %v", err)
	}
	return commands["opencode104"], manifest
}

// A changed tool bundle/plugin/binary must invalidate an already observed
// profile before any prompt can be submitted.
func TestOpenCodeProfilePinsTrustedAssetClosureAndServerBinary(t *testing.T) {
	spec, _ := openCodeConfigFixture(t)
	cfg := ExecConfig{Commands: map[string]CommandSpec{"opencode104": spec}}
	_, sha, err := commandExecutable(spec)
	if err != nil {
		t.Fatal(err)
	}
	pin, err := effectiveCommandProfileDigest(cfg, spec, sha)
	if err != nil || len(pin) != 64 {
		t.Fatalf("fixed SDK closure could not be pinned: %s %v", pin, err)
	}
	if err = os.WriteFile(spec.OpenCode.Executable, []byte("changed fixture binary"), 0600); err != nil {
		t.Fatal(err)
	}
	next, err := effectiveCommandProfileDigest(cfg, spec, sha)
	if err != nil || next == pin {
		t.Fatalf("server binary drift did not invalidate its pin: %v", err)
	}
	tool := filepath.Join(filepath.Dir(spec.Args[0]), "suma-workspace.tool.mjs")
	if err = os.WriteFile(tool, []byte("// drift\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = effectiveCommandProfileDigest(cfg, spec, sha); err == nil {
		t.Fatal("modified trusted MCP adapter remained eligible")
	}
}

func TestOpenCodeRejectsIncompleteClosureAndNativeOverrides(t *testing.T) {
	spec, manifest := openCodeConfigFixture(t)
	manifest["assets"] = []map[string]any{}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(spec.NativeWorkspace.RuntimeManifest, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := nativeManifestDigest(spec); err == nil {
		t.Fatal("unclosed SDK/custom-tool payload accepted")
	}
	for name, mutation := range map[string]func(map[string]any){
		"no-host-ledger":      func(config map[string]any) { delete(config, "native_workspace") },
		"conflict-repair":     func(config map[string]any) { config["native_workspace"].(map[string]any)["conflict_repair"] = true },
		"native-model-budget": func(config map[string]any) { config["model_execution_protocol"] = "suma-model-execution-v1" },
		"planning":            func(config map[string]any) { config["planning"] = true },
		"extra-argv":          func(config map[string]any) { config["args"] = []string{spec.Args[0], "--unsafe"} },
		"global-env":          func(config map[string]any) { config["env"] = map[string]string{"OPENCODE_CONFIG": "untrusted.json"} },
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(spec)
			var config map[string]any
			_ = json.Unmarshal(raw, &config)
			mutation(config)
			raw, _ = json.Marshal(map[string]any{"opencode104": config})
			if _, err := DecodeCommands(raw); err == nil {
				t.Fatal("unsafe OpenCode profile accepted")
			}
		})
	}
}

func TestOpenCodeNeverAcceptsOldStepFinishProtocol(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(map[string]any{"opencode104": map[string]any{"adapter": "opencode", "protocol": "opencode-jsonl-v1", "executable": node, "args": []string{}, "input": "goal"}})
	if _, err = DecodeCommands(raw); err == nil {
		t.Fatal("unsupported native step_finish protocol declared executable")
	}
}

func TestOpenCodeConfigurationAcceptsFixedCatalogNames(t *testing.T) {
	spec, _ := openCodeConfigFixture(t)
	spec.OpenCode.ProviderID = "org.local-104"
	spec.OpenCode.Model = "repository/coding:model104"
	c, err := localOpenCodeCapability(spec)
	if err != nil || c.ProviderID != "org.local-104" || c.Model != "repository/coding:model104" {
		t.Fatalf("fixed public catalog name rejected or changed: %+v %v", c, err)
	}
}
