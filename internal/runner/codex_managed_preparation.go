package runner

import (
	"fmt"
	"slices"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

const managedCodexAdapterID = "codex-app-server"

// List only the approved base commands for this scope. If the configured set
// is too large, do not advertise a subset that could imply a unique choice.
func managedCodexTemplateIDs(cfg ExecConfig) []string {
	if !cfg.AllowExec {
		return nil
	}
	var ids []string
	for id, command := range cfg.Commands {
		if !tc.ValidCommandID(id) || command.InstalledCommand != nil || !fixedManagedCodexCommand(command) {
			continue
		}
		ids = append(ids, id)
		if len(ids) > tp.MaxCodexTemplateIDs {
			return nil
		}
	}
	slices.Sort(ids)
	return ids
}

func managedCodexTemplate(cfg ExecConfig, requested string) (CommandSpec, bool) {
	id, ok := tp.ResolveCodexTemplateID(managedCodexTemplateIDs(cfg), requested)
	if !ok {
		return CommandSpec{}, false
	}
	return cfg.Commands[id], true
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
	template, ok := managedCodexTemplate(w.Exec, op.Configuration.TemplateID)
	if !ok {
		return nil, fmt.Errorf("exact local Codex app-server template required")
	}
	// Own the original configuration independently of the base command map.
	provider := *template.CodexAppServer
	template.CodexAppServer = &provider
	template.Args = slices.Clone(template.Args)
	template.Executable = native.Executable
	template.InstalledCommand = &InstalledCommandPins{AdapterID: managedCodexAdapterID, HostSHA256: native.Item.ExecutableSHA256}
	intent := &installedToolPreparationInput{AdapterID: managedCodexAdapterID, Command: template}
	return intent, validateInstalledPreparationIntent(op, *intent)
}
