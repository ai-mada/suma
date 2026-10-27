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
)

type InventoryReport struct {
	WorkerID    string           `json:"worker_id"`
	OperationID string           `json:"operation_id"`
	Items       []tc.Observation `json:"items"`
	Acked       bool             `json:"acked"`
	ObservedAt  string           `json:"observed_at,omitempty"`
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
	if err = json.Unmarshal(b, &report); err != nil || report.WorkerID == "" || report.OperationID == "" || len(report.Items) > 128 {
		return fmt.Errorf("invalid inventory journal")
	}
	for _, o := range report.Items {
		if err = tc.ValidateObservation(o); err != nil {
			return fmt.Errorf("invalid inventory journal observation")
		}
	}
	j.inventory = &report
	return nil
}
func (j *Journal) NewInventory(worker string, items []tc.Observation) (InventoryReport, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.inventory != nil && !j.inventory.Acked {
		return InventoryReport{}, fmt.Errorf("pending inventory must recover its original operation")
	}
	if worker == "" || len(worker) > 100 || len(items) > 128 {
		return InventoryReport{}, fmt.Errorf("invalid inventory identity or size")
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
	report := InventoryReport{WorkerID: worker, OperationID: operationID(), Items: cloneObservations(items)}
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
	var reply struct {
		Acknowledged bool   `json:"acknowledged"`
		ObservedAt   string `json:"observed_at"`
	}
	payload := map[string]any{"project_id": c.ProjectID, "worker_id": report.WorkerID, "operation_id": report.OperationID, "items": report.Items}
	if c.Pool != nil {
		delete(payload, "worker_id")
	}
	err := c.post(ctx, "tool-inventory", payload, &reply)
	if err != nil {
		return "", err
	}
	if !reply.Acknowledged {
		return "", fmt.Errorf("inventory persistence was not acknowledged")
	}
	if _, err = time.Parse(time.RFC3339, reply.ObservedAt); err != nil {
		return "", fmt.Errorf("invalid inventory persistence timestamp")
	}
	return reply.ObservedAt, nil
}
func (w *Worker) refreshInventory(ctx context.Context) error {
	if pending := w.Journal.PendingInventory(); pending != nil {
		if pending.WorkerID != w.ID {
			return fmt.Errorf("pending tool inventory belongs to another worker")
		}
		observed, err := w.Client.ToolInventory(ctx, *pending)
		if err != nil {
			return fmt.Errorf("recover tool inventory (registered worker credential required): %w", err)
		}
		if err = w.Journal.AckInventory(pending.OperationID, observed); err != nil {
			return err
		}
	}
	items, err := DescribeTools(ctx, w.Exec)
	if err != nil {
		return err
	}
	report, err := w.Journal.NewInventory(w.ID, items)
	if err != nil {
		return err
	}
	observed, err := w.Client.ToolInventory(ctx, report)
	if err != nil {
		return fmt.Errorf("report tool inventory (registered worker credential required): %w", err)
	}
	if err = w.Journal.AckInventory(report.OperationID, observed); err != nil {
		return err
	}
	w.inventoryMu.Lock()
	w.inventory = items
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
