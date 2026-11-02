package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func codexPreparationFixture203(t *testing.T) CodexAppServerToolPreparationInput {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return CodexAppServerToolPreparationInput{AdapterID: "codex-app-server", Executable: executable, ProviderID: "owned_fixture", BaseURL: "https://models.example.test/v1", APIKeyEnvRef: "OWNED_CODEX_REFERENCE", WindowsSandbox: "unelevated", EnableUserInput: false}
}

func TestPrepareCodexAppServerWritesExactNativeProtocol(t *testing.T) {
	input := codexPreparationFixture203(t)
	raw, _ := json.Marshal(input)
	file := filepath.Join(t.TempDir(), "commands.json")
	result, err := PrepareCodexAppServerToolCommands(context.Background(), file, bytes.NewReader(raw))
	if err != nil || !result.Prepared || result.CommandCount != 1 || len(result.Remaining) != 0 {
		t.Fatal("native configuration not prepared", result, err)
	}
	saved, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := DecodeCommands(saved)
	if err != nil || len(commands) != 1 {
		t.Fatal("prepared file is not the original command contract", err)
	}
	command := commands[CodexAppServerCommandID]
	if command.Adapter != "codex" || command.Protocol != "codex-app-server-v1" || command.Input != "codex-app-server" || strings.Join(command.Args, " ") != "app-server --listen stdio://" || command.CodexAppServer == nil || command.CodexAppServer.ProviderID != input.ProviderID || command.CodexAppServer.EnableUserInput || len(command.Env) != 0 || len(command.EnvFrom) != 0 {
		t.Fatal("preparation changed the isolated provider protocol", command)
	}
	if _, err = PrepareCodexAppServerToolCommands(context.Background(), file, bytes.NewReader(raw)); err == nil {
		t.Fatal("existing file was overwritten")
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(after, saved) {
		t.Fatal("original file changed on duplicate preparation")
	}
}

func TestPrepareCodexAppServerRejectsPrivateInputBeforeOutput(t *testing.T) {
	input := codexPreparationFixture203(t)
	raw, _ := json.Marshal(input)
	var generic map[string]any
	_ = json.Unmarshal(raw, &generic)
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["adapter_id"] = "codex-jsonl" },
		func(v map[string]any) { v["args"] = []string{"exec", "unreviewed"} },
		func(v map[string]any) { v["enable_user_input"] = "true" },
		func(v map[string]any) { v["api_key_env_ref"] = "HOME" },
	} {
		value := map[string]any{}
		for key, v := range generic {
			value[key] = v
		}
		change(value)
		candidate, _ := json.Marshal(value)
		file := filepath.Join(t.TempDir(), "commands.json")
		if _, err := PrepareCodexAppServerToolCommands(context.Background(), file, bytes.NewReader(candidate)); err == nil {
			t.Fatal("unreviewed private shape accepted")
		}
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			t.Fatal("invalid input produced a command file", err)
		}
	}
	duplicate := append([]byte(`{"adapter_id":"codex-jsonl",`), raw[1:]...)
	if _, err := DecodeCodexAppServerToolPreparationInput(duplicate); err == nil {
		t.Fatal("duplicate member accepted")
	}
}
