package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestOctopConfigurationPinsEntryAndExcludesAmbientSecrets(t *testing.T) {
	dir := t.TempDir()
	python, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(dir, "runtime.pyz")
	manifestPath := filepath.Join(dir, "runtime.manifest.json")
	content := []byte("fixture bytes, never executed")
	_ = os.WriteFile(entry, content, 0600)
	raw, _ := json.Marshal(nativeRuntimeManifest{SchemaVersion: 1, Entry: filepath.Base(entry), EntrySHA256: digest(content), ExternalImports: []string{}, Dependencies: map[string]string{"octop-harness": tc.OctopHarnessVersion, "agent-client-protocol": tc.OctopACPSDKVersion, "mcp": "1.30.0", "jsonschema": "4.26.0"}})
	_ = os.WriteFile(manifestPath, raw, 0600)
	spec := CommandSpec{Adapter: "octop", Protocol: tc.OctopProtocol, ModelExecutionProtocol: mc.Protocol, Executable: python, Args: []string{entry}, Input: "octop-acp-jsonl", NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{}, RuntimeManifest: manifestPath}}
	t.Setenv("OCTOP_MODEL_API_KEY", "synthetic-octop-private-key")
	t.Setenv("SUMA_RUNNER_TOKEN", "must-not-reach-sdk")
	t.Setenv("PYTHONPATH", "untrusted-personal-package-root")
	home := filepath.Join(dir, "probe-home")
	env, secrets, err := octopEnvironment(spec, home)
	if err != nil || len(secrets) != 0 {
		t.Fatal("credential-free environment failed", err)
	}
	for _, value := range env {
		if strings.Contains(value, "synthetic-octop-private-key") || strings.Contains(value, "must-not-reach-sdk") || strings.HasPrefix(value, "PYTHONPATH=") {
			t.Fatal("ambient credentials or startup hooks entered probe environment")
		}
		if strings.HasPrefix(value, "SUMA_OCTOP_CONFIG=") {
			var config map[string]string
			if json.Unmarshal([]byte(strings.TrimPrefix(value, "SUMA_OCTOP_CONFIG=")), &config) != nil || len(config) != 1 || config["home"] != home {
				t.Fatal("runtime config included provider or model authority")
			}
		}
	}
	if err := os.WriteFile(filepath.Join(home, "owned-state"), []byte("occupied"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := octopEnvironment(spec, home); err == nil {
		t.Fatal("occupied runtime state was reused")
	}
	if _, err := nativeManifestDigest(spec); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(entry, []byte("changed after inventory"), 0600)
	if _, err := nativeManifestDigest(spec); err == nil {
		t.Fatal("changed zipapp retained its approved manifest")
	}
	spec.ModelExecutionProtocol = ""
	if validateOctopSpec(spec) == nil {
		t.Fatal("coding adapter without the model gateway protocol was accepted")
	}
}
