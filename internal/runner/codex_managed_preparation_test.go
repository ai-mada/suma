package runner

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

func codexPreparationBase(executable, providerURL string) CommandSpec {
	return codexDiscoveryCommand(CodexModelDiscoveryInput{
		Executable: executable, ProviderID: "owned-codex-repin", BaseURL: providerURL,
		WindowsSandbox: "unelevated",
	})
}

func codexPreparationScan(t *testing.T, w *Worker, report InstalledToolDiscovery) (toolPreparationCheckpoint, installationNativeObservation) {
	t.Helper()
	ctx := context.Background()
	items, err := installationItems(report, []string{managedCodexAdapterID})
	if err != nil || len(items) != 1 || !tp.CanPrepareItem(items[0]) {
		t.Fatal("exact Codex app-server scan missing", err)
	}
	op := preparationFixture224(t, w)
	scope, err := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	scan := installationFixture205(w.Client.ProjectID, w.ID, op.InstallationReference.ClaimID, w.maintenance.InstanceID)
	scan.AdapterIDs = []string{managedCodexAdapterID}
	scan.SourceSHA256 = ii.ScanSourceSHA256(scan)
	op.AdapterID, op.Configuration, op.InstallationItem = managedCodexAdapterID, tp.Configuration{}, items[0]
	op.InstallationScanID, op.InstallationReference, op.InstallationSourceSHA256 = scan.ID, scanReference(scan), scan.SourceSHA256
	op.InstallationStopProof.Scan, op.InstallationStopProof.SourceSHA256 = scanReference(scan), scan.SourceSHA256
	op.SourceSHA256 = tp.SourceSHA256(op)
	if err = w.Journal.saveInstallationObservation(ctx, installationCheckpoint{ScopeSHA256: scope, ClaimID: scan.ClaimID, Scan: &scan}, report, items); err != nil {
		t.Fatal(err)
	}
	native, err := w.Journal.preparationNative(ctx, scope, op)
	if err != nil {
		t.Fatal(err)
	}
	p := toolPreparationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: op.ClaimID, Responded: true, TenantID: op.TenantID, Operation: &op}
	return p, native
}

func TestManagedCodexPreparationUsesOneLocalTemplateAndExactScan(t *testing.T) {
	ctx := context.Background()
	oldExecutable := filepath.Join(t.TempDir(), "original.exe")
	newExecutable := filepath.Join(t.TempDir(), "scanned.exe")
	data := []byte("MZ\x00\x00owned inert Codex pin fixture")
	if err := os.WriteFile(newExecutable, data, 0700); err != nil {
		t.Fatal(err)
	}
	template := codexPreparationBase(oldExecutable, "https://owned.example.test/v1")
	template.CodexAppServer.APIKeyEnvRef = "OWNED_CODEX_PROVIDER"
	template.CodexAppServer.EnableUserInput = true
	cfg := ExecConfig{AllowExec: true, Commands: map[string]CommandSpec{"codex-base": template}}
	w := preparationWorker224(t, "http://127.0.0.1:4310", cfg)
	if !slices.Contains(w.currentToolMaintenance().ManagedPreparation.AdapterIDs, managedCodexAdapterID) {
		t.Fatal("eligible machine did not advertise native preparation")
	}
	w.managedCommands = map[string]CommandSpec{"unrelated-managed": template}
	noTemplate := cfg
	noTemplate.Commands = nil
	ambiguous := cfg
	ambiguous.Commands = map[string]CommandSpec{"codex-base": template, "second-base": template}
	disabled := cfg
	disabled.AllowExec = false
	extended := cfg
	custom := template
	custom.Env = map[string]string{"LOCAL_PROXY_PASSWORD": "do-not-copy"}
	extended.Commands = map[string]CommandSpec{"codex-base": custom}
	for _, ineligible := range []ExecConfig{noTemplate, ambiguous, disabled, extended} {
		inactive := preparationWorker224(t, "http://127.0.0.1:4310", ineligible)
		ids := inactive.currentToolMaintenance().ManagedPreparation.AdapterIDs
		if slices.Contains(ids, managedCodexAdapterID) || len(ids) != len(tp.AdapterIDs())-1 {
			t.Fatal("ineligible template changed the six original factories", ids)
		}
	}
	report := InstalledToolDiscovery{Protocol: InstalledToolDiscoveryProtocol, Items: []InstalledToolDiscoveryItem{{
		AdapterID: managedCodexAdapterID, Installed: true, ReasonCode: "configuration_required",
		Executable: newExecutable, ExecutableSHA256: digest(data), Version: "codex-cli fixture",
	}}}
	p, native := codexPreparationScan(t, w, report)
	p.InstalledIntent, _ = w.managedCodexPreparationIntent(*p.Operation, native)
	p.Started = true
	if err := w.Journal.savePreparation(p); err != nil || p.InstalledIntent == nil {
		t.Fatal("exact command intent was not recoverable", err)
	}
	restored, err := w.Journal.preparationCheckpoint()
	if err != nil || restored.InstalledIntent == nil {
		t.Fatal("prepared intent disappeared", err)
	}
	command := restored.InstalledIntent.Command
	if command.Executable != newExecutable || command.InstalledCommand.HostSHA256 != digest(data) || command.Protocol != cc.Protocol ||
		*command.CodexAppServer != *template.CodexAppServer || len(command.Env) != 0 || len(command.EnvFrom) != 0 {
		t.Fatal("managed intent did not preserve the exact original provider and newly scanned native bytes")
	}
	file := filepath.Join(t.TempDir(), "prepared.json")
	if err = writeInstalledToolPreparation(ctx, file, *restored.InstalledIntent); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(file)
	commands, err := DecodeCommands(raw)
	if err != nil || !samePermissionValue(commands["local-"+managedCodexAdapterID], command) {
		t.Fatal("exclusive preparation artifact did not round-trip the original intent", err)
	}
	if err = os.WriteFile(newExecutable, append(data, 'x'), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Journal.preparationNative(ctx, p.ScopeSHA256, *p.Operation); err == nil {
		t.Fatal("changed installation satisfied original scan")
	}
	if _, err = commandProfileDigest(command, digest(data)); err == nil {
		t.Fatal("changed installation became eligible to probe or execute")
	}
}
