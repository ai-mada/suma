package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

// Only the machine retains native paths. The browser receives the original
// public Item and can select it, but cannot substitute an executable later.
type installationNativeObservation struct {
	Item          ii.Item            `json:"item"`
	Executable    string             `json:"executable"`
	CursorPackage *CursorPackageSpec `json:"cursor_package,omitempty"`
	Command       *CommandSpec       `json:"command,omitempty"`
}
type installationPrivateReceipt struct {
	Protocol     string                          `json:"protocol"`
	ScopeSHA256  string                          `json:"scope_sha256"`
	Reference    ii.Reference                    `json:"reference"`
	SourceSHA256 string                          `json:"source_sha256"`
	Items        []installationNativeObservation `json:"items"`
}

func (j *Journal) maintenanceDirectory(name string) (string, error) {
	dir := filepath.Join(filepath.Dir(j.path), name)
	if err := os.Mkdir(dir, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	if _, _, err := cursorPackageRoot(dir); err != nil {
		return "", fmt.Errorf("private maintenance directory unavailable")
	}
	return dir, nil
}
func (j *Journal) saveInstallationObservation(ctx context.Context, p installationCheckpoint, report InstalledToolDiscovery, items []ii.Item) error {
	if p.Scan == nil || len(items) != len(report.Items) {
		return fmt.Errorf("private scan source differs")
	}
	receipt := installationPrivateReceipt{Protocol: "suma-installed-tool-private-v1", ScopeSHA256: p.ScopeSHA256, Reference: scanReference(*p.Scan), SourceSHA256: p.Scan.SourceSHA256, Items: []installationNativeObservation{}}
	for i, row := range report.Items {
		if row.AdapterID != "qoder-acp" && row.AdapterID != "cursor-cli" && !genericInstalledAdapter(row.AdapterID) || !row.Installed || row.ExecutableSHA256 == "" || row.Version == "" {
			continue
		}
		native := installationNativeObservation{Item: items[i], Executable: row.Executable}
		if !sameACPProbeInstallation(ctx, row.Executable, row.ExecutableSHA256) {
			return fmt.Errorf("scan native installation changed")
		}
		if row.AdapterID == "cursor-cli" {
			pkg, node, err := inspectCursorPackage(filepath.Dir(row.Executable))
			if err != nil || row.CursorPackage == nil || pkg != *row.CursorPackage || !sameACPPath(node, row.Executable) {
				return fmt.Errorf("scan package installation changed")
			}
			native.CursorPackage = &pkg
		}
		if genericInstalledAdapter(row.AdapterID) {
			if row.PinnedCommand == nil || row.PinnedCommand.InstalledCommand == nil || row.PinnedCommand.InstalledCommand.AdapterID != row.AdapterID || row.PinnedCommand.InstalledCommand.HostSHA256 != row.ExecutableSHA256 || !sameACPPath(row.PinnedCommand.Executable, row.Executable) || validateInstalledCommandSpec(*row.PinnedCommand) != nil {
				return fmt.Errorf("scan command source was not pinned by its original resolver")
			}
			copy, err := commandMapSnapshot(map[string]CommandSpec{"scan-source": *row.PinnedCommand})
			if err != nil {
				return err
			}
			command := copy["scan-source"]
			native.Command = &command
		}
		receipt.Items = append(receipt.Items, native)
	}
	directory, err := j.maintenanceDirectory("tool-installation-observations")
	if err != nil {
		return err
	}
	_, err = writeToolProbeImmutable(filepath.Join(directory, p.ClaimID+".json"), receipt)
	return err
}
func (j *Journal) preparationNative(ctx context.Context, scope string, op tp.Operation) (installationNativeObservation, error) {
	var r installationPrivateReceipt
	path := filepath.Join(filepath.Dir(j.path), "tool-installation-observations", op.InstallationReference.ClaimID+".json")
	if _, err := readToolProbeJSON(path, &r); err != nil {
		return installationNativeObservation{}, fmt.Errorf("rescan_required")
	}
	if r.Protocol != "suma-installed-tool-private-v1" || r.ScopeSHA256 != scope || r.Reference != op.InstallationReference || r.SourceSHA256 != op.InstallationSourceSHA256 {
		return installationNativeObservation{}, fmt.Errorf("installation_source_changed")
	}
	for _, native := range r.Items {
		if native.Item.AdapterID != op.AdapterID {
			continue
		}
		a, _ := json.Marshal(native.Item)
		b, _ := json.Marshal(op.InstallationItem)
		if string(a) != string(b) || !sameACPProbeInstallation(ctx, native.Executable, native.Item.ExecutableSHA256) {
			break
		}
		if op.AdapterID == "cursor-cli" {
			pkg, node, err := inspectCursorPackage(filepath.Dir(native.Executable))
			if err != nil || native.CursorPackage == nil || pkg != *native.CursorPackage || !sameACPPath(node, native.Executable) {
				break
			}
		}
		if genericInstalledAdapter(op.AdapterID) && (native.Command == nil || native.Command.InstalledCommand == nil || native.Command.InstalledCommand.AdapterID != op.AdapterID || native.Command.InstalledCommand.HostSHA256 != native.Item.ExecutableSHA256 || validateInstalledCommandPins(ctx, *native.Command) != nil) {
			break
		}
		return native, nil
	}
	return installationNativeObservation{}, fmt.Errorf("installation_source_changed")
}
