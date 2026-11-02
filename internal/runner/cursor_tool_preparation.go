package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ai-mada/suma/internal/strictjson"
)

const ACPToolPreparationInputLimit = 16 << 10

// Private flat stdin uses the same local ACP field names. Adapter-specific
// validation keeps Cursor's package and Qoder's native entry distinct. All
// hashes, argv and policy are reconstructed; credential values are not input.
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
		if _, ok := fields[key].(string); !ok {
			return input, invalid
		}
	}
	// Token decoding above preserves duplicate-name/shape checks, but Go's
	// ordinary JSON decoder would replace an unpaired surrogate with U+FFFD.
	// Use the shared raw-Unicode validator before losing that input evidence.
	if strictjson.Decode(raw, &input, ACPToolPreparationInputLimit) != nil || !desktopManagedPath(input.Executable) {
		return ACPToolPreparationInput{}, invalid
	}
	switch input.AdapterID {
	case "cursor-cli":
		// These are the original five nonempty Cursor inputs. Qoder's optional
		// credential must not weaken Cursor's explicit isolated-key contract.
		for _, value := range []string{input.Executable, input.ReviewedVersion, input.Model, input.CredentialEnvRef} {
			if value == "" {
				return ACPToolPreparationInput{}, invalid
			}
		}
		if !sameACPPath(input.Executable, filepath.Join(filepath.Dir(input.Executable), cursorNodeName())) {
			return ACPToolPreparationInput{}, invalid
		}
	case "qoder-acp":
		if _, err = acpCommandForAdapter(input.AdapterID, input.Executable, input.ReviewedVersion, input.Model, input.CredentialEnvRef); err != nil {
			return ACPToolPreparationInput{}, invalid
		}
	default:
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
	if configuration.AdapterID == "qoder-acp" {
		prepared, err := snapshotQoderToolPreparation(ctx, configuration)
		if err != nil {
			return result, err
		}
		if err = prepared.write(ctx, file, parent); err != nil {
			return result, err
		}
		result.Prepared, result.CommandCount = true, 1
		return result, nil
	}
	if err = toolPackagePreparationDestination(file, parent, filepath.Dir(configuration.Executable)); err != nil {
		return result, err
	}
	// The exact selected leaf must be the package's fixed native Node. An
	// editor or shell shim cannot become an executable by sharing its parent.
	spec, err := BuildCursorACPCommand(filepath.Dir(configuration.Executable), configuration.ReviewedVersion, configuration.Model, configuration.CredentialEnvRef)
	if err != nil || !sameACPPath(spec.Executable, configuration.Executable) {
		return result, fmt.Errorf("reviewed Cursor package configuration is unavailable")
	}
	if err = toolPackagePreparationDestination(file, parent, spec.ACP.CursorPackage.PackageRoot); err != nil {
		return result, err
	}
	if err = writePreparedToolCommands(ctx, parent, filepath.Base(file), map[string]CommandSpec{"local-cursor-cli": spec}); err != nil {
		return result, err
	}
	result.Prepared, result.CommandCount = true, 1
	return result, nil
}

// Shared by locked native packages: writing into a package would invalidate
// its own closure. Filesystem identities reject aliases before any new write.
func toolPackagePreparationDestination(file string, parent *os.Root, packageRoot string) error {
	_, source, err := cursorPackageRoot(packageRoot)
	if err != nil {
		return fmt.Errorf("reviewed tool package directory is unavailable")
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
			return fmt.Errorf("command destination must be outside the reviewed tool package")
		}
	}
	if within(packageRoot, file) || !cursorPackageParentsStable(source) || !cursorPackageParentsStable(destination) {
		return fmt.Errorf("command destination or reviewed package changed during preparation")
	}
	return nil
}
