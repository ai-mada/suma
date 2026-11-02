package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
)

const CodexAppServerCommandID = "local-codex-app-server"

// CodexAppServerToolPreparationInput is an administrator-owned stdin contract.
// It accepts a provider definition and references only; command arguments,
// credentials, model choices and platform execution authority are not inputs.
type CodexAppServerToolPreparationInput struct {
	AdapterID       string `json:"adapter_id"`
	Executable      string `json:"executable"`
	ProviderID      string `json:"provider_id"`
	BaseURL         string `json:"base_url"`
	APIKeyEnvRef    string `json:"api_key_env_ref"`
	WindowsSandbox  string `json:"windows_sandbox"`
	EnableUserInput bool   `json:"enable_user_input"`
}

func (input CodexAppServerToolPreparationInput) command() CommandSpec {
	// The original builder remains the single fixed app-server argument shape.
	// Discovery-only identity fields are deliberately not fabricated here.
	return codexDiscoveryCommand(CodexModelDiscoveryInput{
		Executable: input.Executable, ProviderID: input.ProviderID, BaseURL: input.BaseURL,
		APIKeyEnvRef: input.APIKeyEnvRef, WindowsSandbox: input.WindowsSandbox,
		EnableUserInput: input.EnableUserInput,
	})
}

func DecodeCodexAppServerToolPreparationInput(raw []byte) (CodexAppServerToolPreparationInput, error) {
	var input CodexAppServerToolPreparationInput
	invalid := fmt.Errorf("invalid private Codex app-server preparation input")
	if len(raw) == 0 || len(raw) > CodexModelDiscoveryInputLimit {
		return input, invalid
	}
	fields, err := decodeToolObject(raw)
	if err != nil || len(fields) != 7 {
		return input, invalid
	}
	for _, name := range []string{"adapter_id", "executable", "provider_id", "base_url", "api_key_env_ref", "windows_sandbox"} {
		if _, ok := fields[name].(string); !ok {
			return input, invalid
		}
	}
	if _, ok := fields["enable_user_input"].(bool); !ok {
		return input, invalid
	}
	if json.Unmarshal(raw, &input) != nil || input.AdapterID != "codex-app-server" || !desktopManagedPath(input.Executable) || validateCodexAppServerSpec(input.command()) != nil {
		return CodexAppServerToolPreparationInput{}, invalid
	}
	return input, nil
}

// PrepareCodexAppServerToolCommands creates exactly one new allowlist using
// the existing rooted, exclusive writer. The native file is inspected, never
// launched. A prepared file is not a model catalogue, login or Worker receipt.
func PrepareCodexAppServerToolCommands(ctx context.Context, destination string, input io.Reader) (ToolPreparation, error) {
	result := ToolPreparation{Protocol: ToolPreparationProtocol, Remaining: []UnpreparedTool{}}
	file, parent, err := openToolCommandDestination(destination)
	result.File = file
	if err != nil {
		return result, err
	}
	defer parent.Close()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if input == nil {
		return result, fmt.Errorf("private Codex preparation input is required")
	}
	raw, err := io.ReadAll(io.LimitReader(input, CodexModelDiscoveryInputLimit+1))
	if err != nil {
		return result, fmt.Errorf("private Codex preparation input unavailable")
	}
	selected, err := DecodeCodexAppServerToolPreparationInput(raw)
	if err != nil {
		return result, err
	}
	// Reuse the reviewed native-file/target policy also used by the original
	// Codex discovery path. No version process, model or credential is accessed.
	selected.Executable, _, err = qoderNativeTarget(ctx, selected.Executable)
	if err != nil {
		return result, fmt.Errorf("selected native Codex executable unavailable")
	}
	if err = writePreparedToolCommands(ctx, parent, filepath.Base(file), map[string]CommandSpec{CodexAppServerCommandID: selected.command()}); err != nil {
		return result, err
	}
	result.Prepared = true
	result.CommandCount = 1
	return result, nil
}
