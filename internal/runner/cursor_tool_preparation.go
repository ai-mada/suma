package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const ACPToolPreparationInputLimit = 16 << 10

// Private flat stdin uses the same local ACP field names. All hashes, argv and
// policy are reconstructed here; credential values and identity are not input.
type ACPToolPreparationInput struct {
	AdapterID        string `json:"adapter_id"`
	Executable       string `json:"executable"`
	ReviewedVersion  string `json:"reviewed_version"`
	Model            string `json:"model"`
	CredentialEnvRef string `json:"credential_env_ref"`
}

func DecodeACPToolPreparationInput(raw []byte) (ACPToolPreparationInput, error) {
	invalid := fmt.Errorf("invalid private ACP preparation input")
	var input ACPToolPreparationInput
	if len(raw) == 0 || len(raw) > ACPToolPreparationInputLimit {
		return input, invalid
	}
	fields, err := decodeToolObject(raw)
	if err != nil || len(fields) != 5 {
		return input, invalid
	}
	for _, key := range []string{"adapter_id", "executable", "reviewed_version", "model", "credential_env_ref"} {
		value, ok := fields[key].(string)
		if !ok || value == "" {
			return input, invalid
		}
	}
	if json.Unmarshal(raw, &input) != nil || input.AdapterID != "cursor-cli" || !desktopManagedPath(input.Executable) || !sameACPPath(input.Executable, filepath.Join(filepath.Dir(input.Executable), cursorNodeName())) {
		return ACPToolPreparationInput{}, invalid
	}
	return input, nil
}

// PrepareACPToolCommands creates one explicit configuration without scanning,
// starting a native process, reading credentials or connecting to a control.
func PrepareACPToolCommands(ctx context.Context, path string, input io.Reader) (ToolPreparation, error) {
	result := ToolPreparation{Protocol: ToolPreparationProtocol, Remaining: []UnpreparedTool{}}
	file, parent, err := openToolCommandDestination(path)
	result.File = file
	if err != nil {
		return result, err
	}
	defer parent.Close()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if input == nil {
		return result, fmt.Errorf("private ACP preparation input is required")
	}
	raw, err := io.ReadAll(io.LimitReader(input, ACPToolPreparationInputLimit+1))
	if err != nil {
		return result, fmt.Errorf("private ACP preparation input is unavailable")
	}
	configuration, err := DecodeACPToolPreparationInput(raw)
	if err != nil {
		return result, err
	}
	if err = cursorPreparationDestination(file, parent, filepath.Dir(configuration.Executable)); err != nil {
		return result, err
	}
	// The exact selected leaf must be the package's fixed native Node. An
	// editor or shell shim cannot become an executable by sharing its parent.
	spec, err := BuildCursorACPCommand(filepath.Dir(configuration.Executable), configuration.ReviewedVersion, configuration.Model, configuration.CredentialEnvRef)
	if err != nil || !sameACPPath(spec.Executable, configuration.Executable) {
		return result, fmt.Errorf("reviewed Cursor package configuration is unavailable")
	}
	if err = cursorPreparationDestination(file, parent, spec.ACP.CursorPackage.PackageRoot); err != nil {
		return result, err
	}
	if err = writePreparedToolCommands(ctx, parent, filepath.Base(file), map[string]CommandSpec{"local-cursor-cli": spec}); err != nil {
		return result, err
	}
	result.Prepared, result.CommandCount = true, 1
	return result, nil
}

func cursorPreparationDestination(file string, parent *os.Root, packageRoot string) error {
	_, source, err := cursorPackageRoot(packageRoot)
	if err != nil {
		return fmt.Errorf("reviewed Cursor package directory is unavailable")
	}
	_, destination, err := cursorPackageRoot(filepath.Dir(file))
	if err != nil {
		return fmt.Errorf("command destination directory is unavailable")
	}
	anchored, err := parent.Stat(".")
	if err != nil || !os.SameFile(anchored, destination[0].info) {
		return fmt.Errorf("command destination changed during preparation")
	}
	// Physical identities also cover a pre-existing Windows short-name alias;
	// a lexical path check alone cannot keep the output out of the closure.
	for _, ancestor := range destination {
		if os.SameFile(source[0].info, ancestor.info) {
			return fmt.Errorf("command destination must be outside the reviewed Cursor package")
		}
	}
	if within(packageRoot, file) || !cursorPackageParentsStable(source) || !cursorPackageParentsStable(destination) {
		return fmt.Errorf("command destination or reviewed package changed during preparation")
	}
	return nil
}
