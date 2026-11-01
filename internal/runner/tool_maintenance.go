package runner

import (
	"fmt"
	"os"
	"path/filepath"

	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
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
	w.maintenance = &capability
	return nil
}

func cloneToolMaintenance(p *ti.ToolMaintenanceCapability) *ti.ToolMaintenanceCapability {
	if p == nil {
		return nil
	}
	copied := *p
	return &copied
}
