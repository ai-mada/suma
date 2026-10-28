package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	spec := CommandSpec{Adapter: "octop", Protocol: tc.OctopProtocol, Executable: python, Args: []string{entry}, Input: "octop-acp-jsonl", NativeWorkspace: &NativeWorkspacePolicy{Checks: []string{}, RuntimeManifest: manifestPath}, Octop: &OctopSpec{ProviderID: "local117", Model: "coding117", BaseURL: "http://127.0.0.1:49117/v1", MaxSteps: 8, MaxOutputTokens: 512, APIKeyEnvRef: "OCTOP_MODEL_API_KEY"}}
	t.Setenv("OCTOP_MODEL_API_KEY", "synthetic-octop-private-key")
	t.Setenv("SUMA_RUNNER_TOKEN", "must-not-reach-sdk")
	t.Setenv("PYTHONPATH", "untrusted-personal-package-root")
	env, secrets, err := octopEnvironment(spec, filepath.Join(dir, "probe-home"), false)
	if err != nil || len(secrets) != 0 {
		t.Fatal("credential-free environment failed", err)
	}
	for _, value := range env {
		if strings.Contains(value, "synthetic-octop-private-key") || strings.Contains(value, "must-not-reach-sdk") || strings.HasPrefix(value, "PYTHONPATH=") {
			t.Fatal("ambient credentials or startup hooks entered probe environment")
		}
	}
	if _, err := nativeManifestDigest(spec); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(entry, []byte("changed after inventory"), 0600)
	if _, err := nativeManifestDigest(spec); err == nil {
		t.Fatal("changed zipapp retained its approved manifest")
	}
	for _, ref := range []string{"HOME", "OCTOP_HOME", "PYTHONPATH", "SUMA_RUNNER_TOKEN", "LANGCHAIN_TRACING_V2", "LANGSMITH_TRACING", "HF_HUB_OFFLINE", "OTEL_EXPORTER_OTLP_ENDPOINT"} {
		copy := *spec.Octop
		copy.APIKeyEnvRef = ref
		spec.Octop = &copy
		if validateOctopSpec(spec) == nil {
			t.Fatalf("credential alias could override process policy: %s", ref)
		}
	}
}
