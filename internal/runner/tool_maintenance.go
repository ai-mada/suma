package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

type toolMaintenanceOwner struct {
	ScopeSHA256 string                       `json:"scope_sha256"`
	Capability  ti.ToolMaintenanceCapability `json:"capability"`
}

// Only the continuous consumer enables this declaration. Its logical owner is
// durable across the same Journal's restart, independent of PID and credentials.
func (w *Worker) enableToolMaintenance() error {
	if w.Client == nil || w.Journal == nil {
		return fmt.Errorf("maintenance owner requires current Runner scope")
	}
	scope, err := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(w.Journal.path), "tool-maintenance-owner.json")
	var owner toolMaintenanceOwner
	if _, err = os.Lstat(path); os.IsNotExist(err) {
		owner = toolMaintenanceOwner{ScopeSHA256: scope, Capability: ti.ToolMaintenanceCapability{Protocol: ti.MaintenanceProtocol, InstanceID: operationID()}}
		if _, err = writeToolProbeImmutable(path, owner); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else if _, err = readToolProbeJSON(path, &owner); err != nil {
		return err
	}
	if owner.ScopeSHA256 != scope || ti.ValidateMaintenanceCapability(owner.Capability) != nil || owner.Capability.Protocol != ti.MaintenanceProtocol {
		return fmt.Errorf("maintenance owner belongs to another scope or protocol")
	}
	capability := owner.Capability
	capability.InstalledDiscovery = nil
	capability.ManagedPreparation = nil
	if w.AllowInstalledToolDiscovery {
		capability.InstalledDiscovery = &ii.Capability{Protocol: ii.Protocol, AdapterIDs: ii.AdapterIDs()}
	}
	if w.AllowToolPreparation {
		if !w.AllowInstalledToolDiscovery {
			return fmt.Errorf("tool preparation requires installed discovery")
		}
		if err = w.initializeCommandGeneration(); err != nil {
			return err
		}
		ids := tp.AdapterIDs()
		if _, ok := managedCodexTemplate(w.Exec); !ok {
			ids = slices.DeleteFunc(ids, func(id string) bool { return id == managedCodexAdapterID })
		}
		capability.ManagedPreparation = &tp.Capability{Protocol: tp.Protocol, AdapterIDs: ids, GenerationSHA256: w.currentGenerationSHA()}
	}
	w.maintenance = &capability
	return nil
}

func cloneToolMaintenance(p *ti.ToolMaintenanceCapability) *ti.ToolMaintenanceCapability {
	if p == nil {
		return nil
	}
	copied := *p
	if p.InstalledDiscovery != nil {
		c := *p.InstalledDiscovery
		c.AdapterIDs = append([]string{}, c.AdapterIDs...)
		copied.InstalledDiscovery = &c
	}
	if p.ManagedPreparation != nil {
		c := *p.ManagedPreparation
		c.AdapterIDs = append([]string{}, c.AdapterIDs...)
		copied.ManagedPreparation = &c
	}
	return &copied
}

func sameToolMaintenance(a, b *ti.ToolMaintenanceCapability) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if a.Protocol != b.Protocol || a.InstanceID != b.InstanceID {
		return false
	}
	if a.InstalledDiscovery == nil || b.InstalledDiscovery == nil {
		if a.InstalledDiscovery != nil || b.InstalledDiscovery != nil {
			return false
		}
	} else if a.InstalledDiscovery.Protocol != b.InstalledDiscovery.Protocol || !slices.Equal(a.InstalledDiscovery.AdapterIDs, b.InstalledDiscovery.AdapterIDs) {
		return false
	}
	if a.ManagedPreparation == nil || b.ManagedPreparation == nil {
		return a.ManagedPreparation == nil && b.ManagedPreparation == nil
	}
	return a.ManagedPreparation.Protocol == b.ManagedPreparation.Protocol && a.ManagedPreparation.GenerationSHA256 == b.ManagedPreparation.GenerationSHA256 && slices.Equal(a.ManagedPreparation.AdapterIDs, b.ManagedPreparation.AdapterIDs)
}

func (w *Worker) toolMaintenanceForGeneration(generation string) *ti.ToolMaintenanceCapability {
	capability := cloneToolMaintenance(w.maintenance)
	if capability != nil && capability.ManagedPreparation != nil {
		capability.ManagedPreparation.GenerationSHA256 = generation
	}
	return capability
}
func (w *Worker) currentToolMaintenance() *ti.ToolMaintenanceCapability {
	return w.toolMaintenanceForGeneration(w.currentGenerationSHA())
}
