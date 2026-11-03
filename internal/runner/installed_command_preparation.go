package runner

import (
	"context"
	"fmt"
	"path/filepath"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

// Generic intent preserves the scanner's typed command, not browser argv.
// It is mutually exclusive with the original ACP intent in the same Journal.
type installedToolPreparationInput struct {
	AdapterID string      `json:"adapter_id"`
	Command   CommandSpec `json:"command"`
}

func installedPreparationIntent(op tp.Operation, native installationNativeObservation) (*installedToolPreparationInput, error) {
	if !genericInstalledAdapter(op.AdapterID) || native.Command == nil || tp.ValidateConfiguration(op.AdapterID, op.Configuration) != nil {
		return nil, fmt.Errorf("original installed command source required")
	}
	snapshot, err := commandMapSnapshot(map[string]CommandSpec{"scanned-source": *native.Command})
	if err != nil {
		return nil, err
	}
	spec := snapshot["scanned-source"]
	if op.AdapterID == "claude-code-cli" {
		spec.Args = claudeArguments(op.Configuration.Model)
	}
	input := &installedToolPreparationInput{AdapterID: op.AdapterID, Command: spec}
	return input, validateInstalledPreparationIntent(op, *input)
}
func validateInstalledPreparationIntent(op tp.Operation, input installedToolPreparationInput) error {
	if input.AdapterID != op.AdapterID || !genericInstalledAdapter(input.AdapterID) || input.Command.InstalledCommand == nil || input.Command.InstalledCommand.AdapterID != input.AdapterID || input.Command.InstalledCommand.HostSHA256 != op.InstallationItem.ExecutableSHA256 || validateInstalledCommandSpec(input.Command) != nil || tp.ValidateConfiguration(op.AdapterID, op.Configuration) != nil {
		return fmt.Errorf("installed preparation intent differs from its exact source")
	}
	if op.AdapterID == "claude-code-cli" {
		capability, err := localClaudeCapability(input.Command)
		if err != nil || capability == nil || capability.Model != op.Configuration.Model {
			return fmt.Errorf("installed Claude intent differs from the explicit fixed model")
		}
	}
	return nil
}
func writeInstalledToolPreparation(ctx context.Context, destination string, input installedToolPreparationInput) error {
	if err := validateInstalledCommandPins(ctx, input.Command); err != nil {
		return err
	}
	file, parent, err := openToolCommandDestination(destination)
	if err != nil {
		return err
	}
	defer parent.Close()
	if root := input.Command.InstalledCommand.ResolverRoot; root != "" {
		if err = toolPackagePreparationDestination(file, parent, root); err != nil {
			return err
		}
	}
	if err = writePreparedToolCommands(ctx, parent, filepath.Base(file), map[string]CommandSpec{"local-" + input.AdapterID: input.Command}); err != nil {
		return err
	}
	// Output may already be durable. Never delete it or repeat the writer if a
	// source changed after creation; the original started operation reconciles.
	return validateInstalledCommandPins(ctx, input.Command)
}
func validateManagedCommandPins(ctx context.Context, spec CommandSpec, profile tc.Profile) error {
	if spec.InstalledCommand != nil {
		if spec.InstalledCommand.HostSHA256 != profile.ExecutableSHA256 {
			return fmt.Errorf("managed native generation changed")
		}
		return validateInstalledCommandPins(ctx, spec)
	}
	if spec.ACP == nil || validateACPExecutionSpec(spec) != nil || !sameACPProbeInstallation(ctx, spec.Executable, profile.ExecutableSHA256) {
		return fmt.Errorf("managed native generation changed")
	}
	return nil
}
