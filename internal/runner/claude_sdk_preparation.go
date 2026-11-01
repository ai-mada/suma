package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	cl "github.com/ai-mada/suma/internal/claudecontract"
)

// SDKToolPreparationInput is private stdin, not a public execution API.
// Hashes and arguments are computed by the original reviewed package builder.
type SDKToolPreparationInput struct {
	AdapterID           string `json:"adapter_id"`
	Executable          string `json:"executable"`
	Host                string `json:"host"`
	NativeCLIExecutable string `json:"native_cli_executable"`
	Model               string `json:"model"`
	CredentialEnvRef    string `json:"credential_env_ref"`
	ReviewedCLIVersion  string `json:"reviewed_cli_version"`
}

func DecodeSDKToolPreparationInput(raw []byte) (SDKToolPreparationInput, error) {
	var input SDKToolPreparationInput
	invalid := fmt.Errorf("invalid private Claude SDK preparation input")
	if len(raw) == 0 || len(raw) > ACPToolPreparationInputLimit {
		return input, invalid
	}
	fields, err := decodeToolObject(raw)
	if err != nil || len(fields) != 7 {
		return input, invalid
	}
	for _, name := range []string{"adapter_id", "executable", "host", "native_cli_executable", "model", "credential_env_ref", "reviewed_cli_version"} {
		v, ok := fields[name].(string)
		if !ok || v == "" {
			return input, invalid
		}
	}
	if json.Unmarshal(raw, &input) != nil || input.AdapterID != "claude-code-sdk" || input.ReviewedCLIVersion != cl.CLIVersion || !desktopManagedPath(input.Executable) || !desktopManagedPath(input.Host) || !desktopManagedPath(input.NativeCLIExecutable) {
		return SDKToolPreparationInput{}, invalid
	}
	return input, nil
}
func PrepareSDKToolCommands(ctx context.Context, destination string, input io.Reader) (ToolPreparation, error) {
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
		return result, fmt.Errorf("private SDK preparation input is required")
	}
	raw, err := io.ReadAll(io.LimitReader(input, ACPToolPreparationInputLimit+1))
	if err != nil {
		return result, fmt.Errorf("private SDK preparation input unavailable")
	}
	selected, err := DecodeSDKToolPreparationInput(raw)
	if err != nil {
		return result, err
	}
	if err = toolPackagePreparationDestination(file, parent, filepath.Dir(selected.Host)); err != nil {
		return result, err
	}
	spec, err := BuildClaudeSDKCommand(selected.Executable, selected.Host, ClaudeSDKSpec{NativeCLIExecutable: selected.NativeCLIExecutable, Model: selected.Model, CredentialEnvRef: selected.CredentialEnvRef, ReviewedCLIVersion: selected.ReviewedCLIVersion})
	if err != nil {
		return result, fmt.Errorf("reviewed Claude SDK package configuration unavailable")
	}
	if err = toolPackagePreparationDestination(file, parent, filepath.Dir(selected.Host)); err != nil {
		return result, err
	}
	if err = writePreparedToolCommands(ctx, parent, filepath.Base(file), map[string]CommandSpec{ClaudeSDKCommandID: spec}); err != nil {
		return result, err
	}
	result.Prepared = true
	result.CommandCount = 1
	return result, nil
}
