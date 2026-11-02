package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

const QoderToolPreparationCommandID = "local-qoder-acp"

// This short-lived snapshot is a local file-writing basis only. It carries no
// native version observation, provider login, model availability or permission
// to execute a task. In particular, no version/ACP process is started here.
type qoderToolPreparation struct {
	command          CommandSpec
	executableSHA256 string
}

func snapshotQoderToolPreparation(ctx context.Context, input ACPToolPreparationInput) (qoderToolPreparation, error) {
	if input.AdapterID != "qoder-acp" {
		return qoderToolPreparation{}, fmt.Errorf("Qoder preparation requires its explicit adapter")
	}
	executable, sha, err := qoderNativeTarget(ctx, input.Executable)
	if err != nil {
		return qoderToolPreparation{}, fmt.Errorf("selected native Qoder entry is unavailable")
	}
	command, err := acpCommandForAdapter(input.AdapterID, executable, input.ReviewedVersion, input.Model, input.CredentialEnvRef)
	if err != nil {
		return qoderToolPreparation{}, fmt.Errorf("Qoder configuration requires the original fixed ACP policy")
	}
	return qoderToolPreparation{command: command, executableSHA256: sha}, nil
}

// The original rooted writer owns creation, persistence and no-overwrite. This
// check only verifies that its pinned parent still names the user's selected
// destination. Qoder is one native file, not a fabricated Cursor package.
func qoderPreparationDestinationCurrent(file string, parent *os.Root) bool {
	_, parents, err := cursorPackageRoot(filepath.Dir(file))
	if err != nil || len(parents) == 0 {
		return false
	}
	anchored, err := parent.Stat(".")
	return err == nil && os.SameFile(anchored, parents[0].info) && cursorPackageParentsStable(parents)
}

func (prepared qoderToolPreparation) write(ctx context.Context, file string, parent *os.Root) error {
	if ctx.Err() != nil || prepared.command.Adapter != "qoder" || validateACPExecutionSpec(prepared.command) != nil || !qoderPreparationDestinationCurrent(file, parent) || !sameACPProbeInstallation(ctx, prepared.command.Executable, prepared.executableSHA256) {
		return fmt.Errorf("Qoder preparation source or destination changed")
	}
	if err := writePreparedToolCommands(ctx, parent, filepath.Base(file), map[string]CommandSpec{QoderToolPreparationCommandID: prepared.command}); err != nil {
		return err
	}
	if ctx.Err() != nil || !qoderPreparationDestinationCurrent(file, parent) || !sameACPProbeInstallation(ctx, prepared.command.Executable, prepared.executableSHA256) {
		// The exclusive new file may already be durable. Preserve it for explicit
		// inspection; never report success, overwrite it or delete unique output.
		return fmt.Errorf("Qoder preparation was not confirmed after persistence; inspect the new output")
	}
	return nil
}
