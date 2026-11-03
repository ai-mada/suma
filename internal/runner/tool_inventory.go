package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

type InventoryReport struct {
	WorkerID    string                        `json:"worker_id"`
	OperationID string                        `json:"operation_id"`
	Items       []tc.Observation              `json:"items"`
	Acked       bool                          `json:"acked"`
	ObservedAt  string                        `json:"observed_at,omitempty"`
	ScopeSHA256 string                        `json:"scope_sha256"`
	Inspection  *ti.Reference                 `json:"inspection,omitempty"`
	Maintenance *ti.ToolMaintenanceCapability `json:"maintenance,omitempty"`
	Preparation *tp.InventoryBinding          `json:"preparation,omitempty"`
}

// InventoryScopeSHA256 binds local recovery to the original control endpoint,
// project and registered worker. Credentials may renew without changing scope.
func InventoryScopeSHA256(control, project, worker string) (string, error) {
	origin, err := cloudControlOrigin(control)
	if err != nil || !localToolSafeText(project, 100) || !localToolSafeText(worker, 100) {
		return "", fmt.Errorf("invalid inventory control/project/worker scope")
	}
	raw, _ := json.Marshal(struct {
		Control string `json:"control_url"`
		Project string `json:"project_id"`
		Worker  string `json:"worker_id"`
	}{origin, project, worker})
	return digest(raw), nil
}

func cloneObservations(items []tc.Observation) []tc.Observation {
	result := append([]tc.Observation{}, items...)
	for i := range result {
		result[i].Profile = tc.CloneProfile(result[i].Profile)
		result[i].ModelDeployments = append([]mc.Deployment(nil), result[i].ModelDeployments...)
		if result[i].CodexCatalog != nil {
			copied := cc.CloneCatalog(*result[i].CodexCatalog)
			result[i].CodexCatalog = &copied
		}
	}
	return result
}

func (j *Journal) inventoryPath() string {
	return filepath.Join(filepath.Dir(j.path), "tool-inventory.json")
}
func (j *Journal) loadInventory() error {
	st, err := os.Lstat(j.inventoryPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > 1<<20 {
		return fmt.Errorf("inventory journal is not a bounded regular file")
	}
	b, err := os.ReadFile(j.inventoryPath())
	if err != nil {
		return err
	}
	var report InventoryReport
	if err = json.Unmarshal(b, &report); err != nil || report.WorkerID == "" || report.OperationID == "" || !tc.ValidDigest(report.ScopeSHA256) || len(report.Items) > 128 {
		return fmt.Errorf("invalid inventory journal")
	}
	for _, o := range report.Items {
		if err = tc.ValidateObservation(o); err != nil {
			return fmt.Errorf("invalid inventory journal observation")
		}
	}
	if report.Inspection != nil && ti.ValidateReference(*report.Inspection) != nil {
		return fmt.Errorf("invalid inventory inspection reference")
	}
	if report.Preparation != nil && (tp.ValidateInventoryBinding(*report.Preparation) != nil || report.Inspection != nil) {
		return fmt.Errorf("invalid inventory preparation reference")
	}
	if report.Maintenance != nil && ti.ValidateMaintenanceCapability(*report.Maintenance) != nil {
		return fmt.Errorf("invalid inventory maintenance capability")
	}
	j.inventory = &report
	return nil
}
func (j *Journal) NewInventory(control, project, worker string, items []tc.Observation) (InventoryReport, error) {
	return j.newInspectionInventory(control, project, worker, items, nil)
}

// The causal inspection reference and the full original inventory are written
// together. A restart must not have to reconstruct which probe a report proves.
func (j *Journal) newInspectionInventory(control, project, worker string, items []tc.Observation, inspection *ti.Reference) (InventoryReport, error) {
	return j.newToolInventory(control, project, worker, items, inspection, nil)
}
func (j *Journal) newToolInventory(control, project, worker string, items []tc.Observation, inspection *ti.Reference, maintenance *ti.ToolMaintenanceCapability) (InventoryReport, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inventory != nil && !j.inventory.Acked {
		return InventoryReport{}, fmt.Errorf("pending inventory must recover its original operation")
	}
	scope, err := InventoryScopeSHA256(control, project, worker)
	if err != nil {
		return InventoryReport{}, err
	}
	if worker == "" || len(worker) > 100 || len(items) > 128 {
		return InventoryReport{}, fmt.Errorf("invalid inventory identity or size")
	}
	if inspection != nil && ti.ValidateReference(*inspection) != nil {
		return InventoryReport{}, fmt.Errorf("invalid inventory inspection reference")
	}
	if maintenance != nil && ti.ValidateMaintenanceCapability(*maintenance) != nil {
		return InventoryReport{}, fmt.Errorf("invalid inventory maintenance capability")
	}
	seen := map[string]bool{}
	for _, o := range items {
		if err := tc.ValidateObservation(o); err != nil {
			return InventoryReport{}, err
		}
		if seen[o.Profile.CommandID] {
			return InventoryReport{}, fmt.Errorf("duplicate inventory command")
		}
		seen[o.Profile.CommandID] = true
	}
	if items == nil {
		items = []tc.Observation{}
	}
	report := InventoryReport{WorkerID: worker, OperationID: operationID(), Items: cloneObservations(items), ScopeSHA256: scope}
	report.Maintenance = cloneToolMaintenance(maintenance)
	if inspection != nil {
		copied := *inspection
		report.Inspection = &copied
	}
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return InventoryReport{}, err
	}
	if err = writeEvidence(j.inventoryPath(), b); err != nil {
		return InventoryReport{}, err
	}
	j.inventory = &report
	returned := report
	returned.Items = cloneObservations(report.Items)
	returned.Maintenance = cloneToolMaintenance(report.Maintenance)
	if report.Inspection != nil {
		copied := *report.Inspection
		returned.Inspection = &copied
	}
	return returned, nil
}
func (j *Journal) PendingInventory() *InventoryReport {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inventory == nil || j.inventory.Acked {
		return nil
	}
	report := *j.inventory
	report.Items = cloneObservations(report.Items)
	report.Maintenance = cloneToolMaintenance(report.Maintenance)
	if report.Preparation != nil {
		copied := *report.Preparation
		report.Preparation = &copied
	}
	if report.Inspection != nil {
		copied := *report.Inspection
		report.Inspection = &copied
	}
	return &report
}
func (j *Journal) AckInventory(operationID, observed string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inventory == nil || j.inventory.OperationID != operationID {
		return fmt.Errorf("inventory operation absent")
	}
	if _, err := time.Parse(time.RFC3339, observed); err != nil {
		return fmt.Errorf("inventory acknowledgement time invalid")
	}
	report := *j.inventory
	report.Acked = true
	report.ObservedAt = observed
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err = writeEvidence(j.inventoryPath(), b); err != nil {
		return err
	}
	j.inventory = &report
	return nil
}
func (c *Client) ToolInventory(ctx context.Context, report InventoryReport) (string, error) {
	scope, err := InventoryScopeSHA256(c.BaseURL, c.ProjectID, report.WorkerID)
	if err != nil || report.ScopeSHA256 != scope {
		return "", fmt.Errorf("tool inventory belongs to a different or unknown control/project/worker scope")
	}
	var reply struct {
		Acknowledged     bool                          `json:"acknowledged"`
		ObservedAt       string                        `json:"observed_at"`
		Inspection       *ti.Reference                 `json:"inspection"`
		InspectionState  string                        `json:"inspection_state"`
		Maintenance      *ti.ToolMaintenanceCapability `json:"maintenance"`
		Preparation      *tp.InventoryBinding          `json:"preparation"`
		PreparationState string                        `json:"preparation_state"`
	}
	payload := map[string]any{"project_id": c.ProjectID, "worker_id": report.WorkerID, "operation_id": report.OperationID, "items": report.Items}
	if report.Preparation != nil {
		if tp.ValidateInventoryBinding(*report.Preparation) != nil || report.Inspection != nil {
			return "", fmt.Errorf("invalid inventory preparation binding")
		}
		payload["preparation"] = report.Preparation
	}
	if report.Maintenance != nil {
		if ti.ValidateMaintenanceCapability(*report.Maintenance) != nil {
			return "", fmt.Errorf("invalid inventory maintenance capability")
		}
		payload["maintenance"] = report.Maintenance
	}
	if report.Inspection != nil {
		if ti.ValidateReference(*report.Inspection) != nil {
			return "", fmt.Errorf("invalid inventory inspection reference")
		}
		payload["inspection"] = report.Inspection
	}
	if c.Pool != nil {
		delete(payload, "worker_id")
	}
	err = c.post(ctx, "tool-inventory", payload, &reply)
	if err != nil {
		return "", err
	}
	if !reply.Acknowledged {
		return "", fmt.Errorf("inventory persistence was not acknowledged")
	}
	if report.Maintenance != nil {
		if !sameToolMaintenance(reply.Maintenance, report.Maintenance) {
			return "", fmt.Errorf("original maintenance capability was not acknowledged")
		}
	} else if reply.Maintenance != nil {
		return "", fmt.Errorf("unrequested maintenance acknowledgement")
	}
	if report.Inspection != nil {
		if reply.Inspection == nil || *reply.Inspection != *report.Inspection || reply.InspectionState != "completed" {
			return "", fmt.Errorf("original inspection persistence was not acknowledged")
		}
	} else if reply.Inspection != nil || reply.InspectionState != "" {
		return "", fmt.Errorf("unrequested inspection acknowledgement")
	}
	if report.Preparation != nil {
		if reply.Preparation == nil || *reply.Preparation != *report.Preparation || reply.PreparationState != "completed" {
			return "", fmt.Errorf("original preparation inventory was not acknowledged")
		}
	} else if reply.Preparation != nil || reply.PreparationState != "" {
		return "", fmt.Errorf("unrequested preparation acknowledgement")
	}
	if _, err = time.Parse(time.RFC3339, reply.ObservedAt); err != nil {
		return "", fmt.Errorf("invalid inventory persistence timestamp")
	}
	return reply.ObservedAt, nil
}

// ReportTools persists or recovers exactly one inventory operation and returns.
// It never starts the Worker loop, claims work, or replays another journal.
// A recovered ACK is returned as-is, without probing or issuing a fresh report.
func (w *Worker) ReportTools(ctx context.Context) (InventoryReport, bool, error) {
	w.inventoryOpMu.Lock()
	defer w.inventoryOpMu.Unlock()
	return w.reportTools(ctx)
}

func (w *Worker) reportTools(ctx context.Context) (InventoryReport, bool, error) {
	if w.Client == nil || w.Journal == nil {
		return InventoryReport{}, false, fmt.Errorf("Runner client and durable journal are required")
	}
	if !w.managedRestored {
		head, e := w.Journal.readToolGenerationHead()
		if e != nil {
			return InventoryReport{}, false, e
		}
		if head != nil {
			return InventoryReport{}, false, fmt.Errorf("managed generation requires the original continuous owner")
		}
	}
	scope, err := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if err != nil {
		return InventoryReport{}, false, err
	}
	if pending := w.Journal.PendingInventory(); pending != nil {
		if pending.Preparation != nil {
			return InventoryReport{}, true, fmt.Errorf("managed preparation requires original continuous owner reconciliation")
		}
		if pending.WorkerID != w.ID || pending.ScopeSHA256 != scope {
			return InventoryReport{}, true, fmt.Errorf("pending tool inventory belongs to a different or unknown control/project/worker scope")
		}
		report, err := w.acknowledgeInventory(ctx, *pending)
		return report, true, err
	}
	checkpoint, err := w.Journal.toolInspectionCheckpoint()
	if err != nil {
		return InventoryReport{}, false, err
	}
	if checkpoint != nil && !checkpoint.Complete && checkpoint.Inspection != nil {
		return InventoryReport{}, false, fmt.Errorf("pending tool inspection requires its original reconciliation")
	}
	scan, err := w.Journal.installationCheckpoint()
	if err != nil {
		return InventoryReport{}, false, err
	}
	if scan != nil && !scan.Complete {
		return InventoryReport{}, false, fmt.Errorf("pending installation scan requires its original discovery owner")
	}
	preparation, err := w.Journal.preparationCheckpoint()
	if err != nil {
		return InventoryReport{}, false, err
	}
	if preparation != nil && !preparation.Complete {
		return InventoryReport{}, false, fmt.Errorf("pending tool preparation requires its original owner")
	}
	items, err := w.describeToolSnapshot(ctx, w.currentExecSnapshot())
	if err != nil {
		return InventoryReport{}, false, err
	}
	if ctx.Err() != nil {
		return InventoryReport{}, false, ctx.Err()
	}
	report, err := w.Journal.newToolInventory(w.Client.BaseURL, w.Client.ProjectID, w.ID, items, nil, w.currentToolMaintenance())
	if err != nil {
		return InventoryReport{}, false, err
	}
	report, err = w.acknowledgeInventory(ctx, report)
	return report, false, err
}

func (w *Worker) acknowledgeInventory(ctx context.Context, report InventoryReport) (InventoryReport, error) {
	observed, err := w.Client.ToolInventory(ctx, report)
	if err != nil {
		return InventoryReport{}, fmt.Errorf("report tool inventory (registered worker credential required): %w", err)
	}
	if err = w.Journal.AckInventory(report.OperationID, observed); err != nil {
		return InventoryReport{}, err
	}
	report.Acked, report.ObservedAt = true, observed
	return report, nil
}

func (w *Worker) refreshInventory(ctx context.Context) error {
	report, recovered, err := w.ReportTools(ctx)
	if err != nil {
		return err
	}
	// The continuous worker still refreshes current observations after recovering
	// an older report. Only the explicit single-step mode stops at that ACK.
	if recovered {
		report, _, err = w.ReportTools(ctx)
		if err != nil {
			return err
		}
	}
	w.inventoryMu.Lock()
	w.inventory = report.Items
	w.inventoryReady = true
	w.inventoryMu.Unlock()
	return nil
}
func permanentInventoryError(err error) bool {
	var status *runnerHTTPStatusError
	return errors.As(err, &status) && status.Status >= 400 && status.Status < 500 && status.Status != 408 && status.Status != 429
}
func (w *Worker) setInventoryUnavailable() {
	w.inventoryMu.Lock()
	w.inventoryReady = false
	w.inventoryMu.Unlock()
}
func (w *Worker) canClaimInventory() bool {
	w.inventoryMu.Lock()
	defer w.inventoryMu.Unlock()
	return w.inventoryReady
}
func (w *Worker) currentInventory() []tc.Observation {
	w.inventoryMu.Lock()
	defer w.inventoryMu.Unlock()
	if w.inventory == nil {
		return cloneObservations(w.Exec.ToolInventory)
	}
	return cloneObservations(w.inventory)
}
