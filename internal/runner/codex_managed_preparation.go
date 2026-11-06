package runner

import (
	"fmt"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

const managedCodexAdapterID = "codex-app-server"

// A project may adopt a newly scanned executable with the scope's original
// provider settings. A second command or custom environment would make that
// selection ambiguous, so only a single fixed base command is eligible.
func managedCodexTemplate(cfg ExecConfig) (CommandSpec, bool) {
	if !cfg.AllowExec {
		return CommandSpec{}, false
	}
	var template CommandSpec
	for id, command := range cfg.Commands {
		if command.CodexAppServer == nil && command.Protocol != cc.Protocol {
			continue
		}
		if template.CodexAppServer != nil || !tc.ValidCommandID(id) || command.InstalledCommand != nil || !fixedManagedCodexCommand(command) {
			return CommandSpec{}, false
		}
		template = command
	}
	return template, template.CodexAppServer != nil
}

// Use the same command builder as the local setup. It copies provider settings
// and a credential reference, never arbitrary environment or process inputs.
func fixedManagedCodexCommand(spec CommandSpec) bool {
	if spec.CodexAppServer == nil || validateCodexAppServerSpec(spec) != nil {
		return false
	}
	provider := spec.CodexAppServer
	expected := codexDiscoveryCommand(CodexModelDiscoveryInput{
		Executable: spec.Executable, ProviderID: provider.ProviderID, BaseURL: provider.BaseURL,
		APIKeyEnvRef: provider.APIKeyEnvRef, WindowsSandbox: provider.WindowsSandbox,
		EnableUserInput: provider.EnableUserInput,
	})
	expected.InstalledCommand = spec.InstalledCommand
	return samePermissionValue(spec, expected)
}

func (w *Worker) managedCodexPreparationIntent(op tp.Operation, native installationNativeObservation) (*installedToolPreparationInput, error) {
	template, ok := managedCodexTemplate(w.Exec)
	if !ok {
		return nil, fmt.Errorf("exact local Codex app-server template required")
	}
	// Own the original configuration independently of the base command map.
	provider := *template.CodexAppServer
	template.CodexAppServer = &provider
	template.Executable = native.Executable
	template.InstalledCommand = &InstalledCommandPins{AdapterID: managedCodexAdapterID, HostSHA256: native.Item.ExecutableSHA256}
	intent := &installedToolPreparationInput{AdapterID: managedCodexAdapterID, Command: template}
	return intent, validateInstalledPreparationIntent(op, *intent)
}
