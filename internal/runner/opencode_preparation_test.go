package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func openCodePreparationFixture207(t *testing.T) OpenCodeToolPreparationInput {
	t.Helper()
	spec, _ := openCodeConfigFixture(t)
	// A format-recognizable but non-executable fixture proves preparation does
	// not invoke --version or SDK serving. Its bytes would fail an actual launch.
	if err := os.WriteFile(spec.OpenCode.Executable, []byte("MZowned non-runnable native metadata fixture207"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(spec.OpenCode.Executable, 0700); err != nil {
		t.Fatal(err)
	}
	node, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return OpenCodeToolPreparationInput{AdapterID: "opencode-sdk", NodeExecutable: node, RuntimeManifest: spec.NativeWorkspace.RuntimeManifest, NativeCLIExecutable: spec.OpenCode.Executable, ProviderID: "owned207", Model: "fixed207", BaseURL: "https://models.example.test/v1", APIKeyEnvRef: "OWNED_OPENCODE_REFERENCE", ReviewedVersion: tc.OpenCodeSDKVersion}
}

func TestPrepareOpenCodeCommandsWritesOneFixedConfigurationWithoutExecution(t *testing.T) {
	input := openCodePreparationFixture207(t)
	t.Setenv(input.APIKeyEnvRef, "private-value-never-read207")
	t.Setenv("SUMA_RUNNER_TOKEN", "private-platform-token207")
	raw, _ := json.Marshal(input)
	file := filepath.Join(t.TempDir(), "commands.json")
	result, err := PrepareOpenCodeToolCommands(context.Background(), file, bytes.NewReader(raw))
	if err != nil || !result.Prepared || result.CommandCount != 1 || result.Protocol != ToolPreparationProtocol || len(result.Remaining) != 0 {
		t.Fatal("fixed OpenCode configuration not prepared", result, err)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := DecodeCommands(saved)
	if err != nil || len(commands) != 1 {
		t.Fatal("prepared file is not the original commands contract", err)
	}
	spec := commands[OpenCodeCommandID]
	if spec.Adapter != "opencode" || spec.Protocol != tc.OpenCodeSDKProtocol || spec.Input != "opencode-sdk-jsonl" || spec.Executable != input.NodeExecutable || len(spec.Args) != 1 || spec.Args[0] != filepath.Join(filepath.Dir(input.RuntimeManifest), "suma-opencode.bundle.mjs") || spec.NativeWorkspace == nil || spec.NativeWorkspace.Checks == nil || len(spec.NativeWorkspace.Checks) != 0 || spec.NativeWorkspace.RuntimeManifest != input.RuntimeManifest || spec.OpenCode == nil || *spec.OpenCode != (OpenCodeSpec{Executable: input.NativeCLIExecutable, ProviderID: input.ProviderID, Model: input.Model, BaseURL: input.BaseURL, APIKeyEnvRef: input.APIKeyEnvRef}) || len(spec.Env) != 0 || len(spec.EnvFrom) != 0 {
		t.Fatal("prepared configuration changed fixed execution policy")
	}
	if OpenCodeConfigurationDigest(*spec.OpenCode) != OpenCodeConfigurationDigest(*input.command().OpenCode) || strings.Contains(string(saved), "private-value-never-read207") || strings.Contains(string(saved), "private-platform-token207") {
		t.Fatal("configuration identity or secret reference was not preserved")
	}
	reader := &preparationReader191{}
	if _, err = PrepareOpenCodeToolCommands(context.Background(), file, reader); err == nil || reader.reads != 0 {
		t.Fatal("existing output was consumed or overwritten")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(saved, after) {
		t.Fatal("existing commands file changed")
	}
	input.APIKeyEnvRef = ""
	raw, _ = json.Marshal(input)
	if _, err = DecodeOpenCodeToolPreparationInput(raw); err != nil {
		t.Fatal("explicit unauthenticated local provider choice rejected", err)
	}
}

func TestPrepareOpenCodeRejectsUnreviewedInputAndAssetsBeforeOutput(t *testing.T) {
	input := openCodePreparationFixture207(t)
	raw, _ := json.Marshal(input)
	var original map[string]any
	_ = json.Unmarshal(raw, &original)
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["adapter_id"] = "opencode-cli" },
		func(v map[string]any) { v["reviewed_version"] = "1.19.5" },
		func(v map[string]any) { v["args"] = []string{"serve"} },
		func(v map[string]any) { v["api_key_env_ref"] = "HOME" },
		func(v map[string]any) { v["base_url"] = "https://user:private@example.test/v1" },
		func(v map[string]any) { v["runtime_manifest"] = input.RuntimeManifest + ".other" },
		func(v map[string]any) { delete(v, "api_key_env_ref") },
		func(v map[string]any) { v["api_key_env_ref"] = nil },
	} {
		value := map[string]any{}
		for k, v := range original {
			value[k] = v
		}
		change(value)
		candidate, _ := json.Marshal(value)
		file := filepath.Join(t.TempDir(), "commands.json")
		if _, err := PrepareOpenCodeToolCommands(context.Background(), file, bytes.NewReader(candidate)); err == nil {
			t.Fatal("unreviewed preparation input accepted")
		}
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("invalid input created commands")
		}
	}
	for _, candidate := range [][]byte{append([]byte(`{"adapter_id":"opencode-sdk",`), raw[1:]...), bytes.Replace(raw, []byte(`"fixed207"`), []byte(`"\ud800"`), 1), append(raw, []byte(` {}`)...), bytes.Repeat([]byte(" "), OpenCodeToolPreparationInputLimit+1)} {
		if _, err := DecodeOpenCodeToolPreparationInput(candidate); err == nil {
			t.Fatal("ambiguous or oversized private JSON accepted")
		}
	}
	reader := &preparationReader191{}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareOpenCodeToolCommands(cancelled, filepath.Join(t.TempDir(), "cancelled.json"), reader); err == nil || reader.reads != 0 {
		t.Fatal("cancelled preparation consumed private input")
	}
	packageOutput := filepath.Join(filepath.Dir(input.RuntimeManifest), "commands.json")
	if _, err := PrepareOpenCodeToolCommands(context.Background(), packageOutput, bytes.NewReader(raw)); err == nil {
		t.Fatal("commands were written into reviewed runtime package")
	}
	if _, err := os.Stat(packageOutput); !os.IsNotExist(err) {
		t.Fatal("runtime package mutated")
	}
	if err := os.WriteFile(filepath.Join(filepath.Dir(input.RuntimeManifest), "suma-workspace.tool.mjs"), []byte("changed asset207"), 0600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "changed.json")
	if _, err := PrepareOpenCodeToolCommands(context.Background(), file, bytes.NewReader(raw)); err == nil {
		t.Fatal("changed runtime asset accepted")
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("changed asset produced commands")
	}
}
