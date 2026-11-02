package toolinspectioncontract

import (
	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	"testing"
)

func TestMaintenanceInstallationCapabilityIsOptionalButExplicitAndClosed(t *testing.T) {
	value := ToolMaintenanceCapability{Protocol: MaintenanceProtocol, InstanceID: "same-logical-journal"}
	if ValidateMaintenanceCapability(value) != nil {
		t.Fatal("maintenance without opt-in rejected")
	}
	value.InstalledDiscovery = &tii.Capability{Protocol: tii.Protocol, AdapterIDs: []string{"claude-code-sdk"}}
	if ValidateMaintenanceCapability(value) != nil {
		t.Fatal("explicit installation scan declaration rejected")
	}
	for _, cap := range []tii.Capability{{Protocol: "other", AdapterIDs: []string{"claude-code-sdk"}}, {Protocol: tii.Protocol, AdapterIDs: []string{}}, {Protocol: tii.Protocol, AdapterIDs: []string{"workbuddy"}}} {
		value.InstalledDiscovery = &cap
		if ValidateMaintenanceCapability(value) == nil {
			t.Fatal("invalid installation capability accepted")
		}
	}
	value.Protocol = "suma-tool-maintenance-v2"
	value.InstalledDiscovery = &tii.Capability{Protocol: tii.Protocol, AdapterIDs: tii.AdapterIDs()}
	if ValidateMaintenanceCapability(value) == nil {
		t.Fatal("unknown maintenance version borrowed v1 scan capability")
	}
}
