package toolpreparationcontract_test

import (
	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
	"strings"
	"testing"
)

func TestManagedPreparationSourceStaysBoundToInputAcrossClaimAndReconciliation(t *testing.T) {
	source := tp.Operation{ID: "prepare224", TenantID: "tenant224", ProjectID: "project224", WorkerID: "worker224", InstanceID: "instance224", AdapterID: "qoder-acp", Configuration: tp.Configuration{Model: "fixed-model"}, ExpectedGenerationSHA256: strings.Repeat("a", 64), InstallationScanID: "scan224", InstallationReference: ii.Reference{ScanID: "scan224", ClaimID: "source224", Fence: 1}}
	original := tp.SourceSHA256(source)
	source.SourceSHA256 = original
	source.Version = 8
	source.State = "completed"
	source.Fence = 2
	source.ClaimID = "claim224"
	source.ClaimedAt = "2026-11-03T00:00:00Z"
	source.LeaseUntil = "2026-11-03T00:01:00Z"
	source.ResultCode = "completed"
	source.Receipt = &tp.GenerationReceipt{GenerationSHA256: strings.Repeat("b", 64)}
	source.InventoryOperationID = "inventory224"
	source.InventoryObservedAt = "2026-11-03T00:00:10Z"
	source.SourceChanged = true
	if tp.SourceSHA256(source) != original {
		t.Fatal("claim/result projections changed the immutable input source")
	}
	changed := source
	changed.Configuration.Model = "another-model"
	if tp.SourceSHA256(changed) == original {
		t.Fatal("changed model retained input confirmation")
	}
	changed = source
	changed.TenantID = "other-tenant"
	if tp.SourceSHA256(changed) == original {
		t.Fatal("cross-tenant source retained input confirmation")
	}
	changed = source
	changed.InstallationReference.Fence++
	if tp.SourceSHA256(changed) == original {
		t.Fatal("another original installation source was admitted")
	}
	if tp.CommandID(source.ID) == tp.CommandID("other224") || len(tp.CommandID(source.ID)) > 100 {
		t.Fatal("managed commands collide or exceed original identifiers")
	}
}

func TestManagedPreparationRequiresIndependentOptInAndExplicitSafeConfiguration(t *testing.T) {
	discovery := &ii.Capability{Protocol: ii.Protocol, AdapterIDs: ii.AdapterIDs()}
	cap := &tp.Capability{Protocol: tp.Protocol, AdapterIDs: tp.AdapterIDs(), GenerationSHA256: strings.Repeat("a", 64)}
	declaration := ti.ToolMaintenanceCapability{Protocol: ti.MaintenanceProtocol, InstanceID: "owner224", InstalledDiscovery: discovery, ManagedPreparation: cap}
	if ti.ValidateMaintenanceCapability(declaration) != nil {
		t.Fatal("explicit bounded capability rejected")
	}
	for _, mutate := range []func(*ti.ToolMaintenanceCapability){func(v *ti.ToolMaintenanceCapability) { v.InstalledDiscovery = nil }, func(v *ti.ToolMaintenanceCapability) { v.Protocol = "suma-tool-maintenance-v2" }} {
		bad := declaration
		mutate(&bad)
		if ti.ValidateMaintenanceCapability(bad) == nil {
			t.Fatal("scan permission or other protocol silently granted a configuration write")
		}
	}
	for _, ids := range [][]string{nil, {"qoder-acp", "qoder-acp"}, {"claude-code-sdk"}} {
		bad := *cap
		bad.AdapterIDs = ids
		if tp.ValidateCapability(bad) == nil {
			t.Fatal("unsupported factory declared", ids)
		}
	}
	if tp.ValidateConfiguration("qoder-acp", tp.Configuration{Model: "fixed-model"}) != nil || tp.ValidateConfiguration("cursor-cli", tp.Configuration{Model: "fixed-model", CredentialEnvRef: "MY_CURSOR_KEY"}) != nil {
		t.Fatal("explicit provider options rejected")
	}
	for _, configuration := range []tp.Configuration{{Model: " fixed-model"}, {Model: "fixed-model", CredentialEnvRef: "PATH"}, {Model: "fixed-model", CredentialEnvRef: "NODE_OPTIONS"}, {Model: "fixed-model", CredentialEnvRef: "actual secret value"}} {
		if tp.ValidateConfiguration("qoder-acp", configuration) == nil {
			t.Fatal("unsafe option accepted")
		}
	}
	if tp.ValidateConfiguration("cursor-cli", tp.Configuration{Model: "fixed-model"}) == nil {
		t.Fatal("Cursor credential requirement was weakened")
	}
}

func TestManagedPreparationUsesPinnedConfigurationRequiredACPInstallation(t *testing.T) {
	item := ii.Item{AdapterID: "qoder-acp", Protocol: "acp-stdio-v1", Installed: true, ReasonCode: "acp_configuration_required", Version: "1.0.0", ExecutableSHA256: strings.Repeat("a", 64)}
	if ii.ValidateItem(item) != nil || !tp.CanPrepareItem(item) {
		t.Fatal("installed but not configured ACP cannot enter its preparation flow")
	}
	for _, mutate := range []func(*ii.Item){func(v *ii.Item) { v.ReasonCode = "version_unsupported" }, func(v *ii.Item) { v.ExecutableSHA256 = "" }, func(v *ii.Item) { v.LaunchSupported = true; v.ReasonCode = "workspace_required" }, func(v *ii.Item) { v.Installed = false }} {
		bad := item
		mutate(&bad)
		if tp.CanPrepareItem(bad) {
			t.Fatal("an unverified or unrelated native source was admitted", bad)
		}
	}
	item.AdapterID = "cursor-cli"
	if !tp.CanPrepareItem(item) {
		t.Fatal("second implemented ACP factory was lost")
	}
}
