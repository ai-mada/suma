package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

// This checkpoint owns only an explicit read-only maintenance request. It is
// separate from business claims and never confers permission to execute a task.
type toolInspectionCheckpoint struct {
	ScopeSHA256      string                            `json:"scope_sha256"`
	WorkerID         string                            `json:"worker_id"`
	ClaimID          string                            `json:"claim_id"`
	Responded        bool                              `json:"responded"`
	Inspection       *ti.Inspection                    `json:"inspection,omitempty"`
	Started          bool                              `json:"started"`
	FailureOperation string                            `json:"failure_operation,omitempty"`
	FailureCode      string                            `json:"failure_code,omitempty"`
	FailureAcked     bool                              `json:"failure_acked"`
	Complete         bool                              `json:"complete"`
	Recovery         *toolInspectionRecoveryCheckpoint `json:"recovery,omitempty"`
}

func (j *Journal) toolInspectionPath() string {
	return filepath.Join(filepath.Dir(j.path), "tool-inspection.json")
}

func validateToolInspectionCheckpoint(p toolInspectionCheckpoint) error {
	if !ti.ID(p.WorkerID) || !ti.Operation(p.ClaimID) || !tc.ValidDigest(p.ScopeSHA256) || p.Complete && !p.Responded || p.Started && p.Inspection == nil || !p.Responded && p.Inspection != nil {
		return fmt.Errorf("invalid tool inspection checkpoint")
	}
	if p.Inspection != nil {
		if ti.ValidateInspection(*p.Inspection) != nil || p.Inspection.WorkerID != p.WorkerID || p.Inspection.ClaimID != p.ClaimID || p.Inspection.Fence < 1 {
			return fmt.Errorf("tool inspection checkpoint source differs")
		}
	}
	if p.FailureCode != "" {
		if p.Inspection == nil || !ti.Operation(p.FailureOperation) {
			return fmt.Errorf("invalid tool inspection failure checkpoint")
		}
		outcome := "unknown"
		if p.FailureCode == "not_started" {
			outcome = "failed"
		}
		if ti.ValidateFailureRequest(ti.FailureRequest{ProjectID: p.Inspection.ProjectID, WorkerID: p.WorkerID, Inspection: inspectionReference(*p.Inspection), IdempotencyKey: p.FailureOperation, Outcome: outcome, FailureCode: p.FailureCode}) != nil || outcome == "failed" && p.Started {
			return fmt.Errorf("invalid tool inspection failure boundary")
		}
	} else if p.FailureOperation != "" || p.FailureAcked {
		return fmt.Errorf("missing tool inspection failure intent")
	}
	return validateToolRecoveryCheckpoint(p)
}

func (j *Journal) toolInspectionCheckpoint() (*toolInspectionCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	st, err := os.Lstat(j.toolInspectionPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > 16<<10 {
		return nil, fmt.Errorf("tool inspection checkpoint is not a bounded regular file")
	}
	raw, err := os.ReadFile(j.toolInspectionPath())
	if err != nil {
		return nil, err
	}
	if _, err = decodeToolObject(raw); err != nil {
		return nil, fmt.Errorf("invalid tool inspection checkpoint JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var p toolInspectionCheckpoint
	if d.Decode(&p) != nil || d.Decode(new(any)) != io.EOF || validateToolInspectionCheckpoint(p) != nil {
		return nil, fmt.Errorf("invalid tool inspection checkpoint")
	}
	return &p, nil
}

func (j *Journal) saveToolInspection(p toolInspectionCheckpoint) error {
	if err := validateToolInspectionCheckpoint(p); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return writeEvidence(j.toolInspectionPath(), raw)
}

func (j *Journal) beginToolInspection(c *Client, worker string) (*toolInspectionCheckpoint, error) {
	scope, err := InventoryScopeSHA256(c.BaseURL, c.ProjectID, worker)
	if err != nil {
		return nil, err
	}
	p, err := j.toolInspectionCheckpoint()
	if err != nil {
		return nil, err
	}
	if p != nil && (p.ScopeSHA256 != scope || p.WorkerID != worker || p.Inspection != nil && p.Inspection.ProjectID != c.ProjectID) {
		return nil, fmt.Errorf("tool inspection checkpoint belongs to another control/project/worker")
	}
	if p != nil && !p.Complete {
		return p, nil
	}
	if p != nil {
		if err = j.archiveClosedToolInspection(*p); err != nil {
			return nil, err
		}
	}
	p = &toolInspectionCheckpoint{ScopeSHA256: scope, WorkerID: worker, ClaimID: operationID()}
	if err = j.saveToolInspection(*p); err != nil {
		return nil, err
	}
	return p, nil
}

func inspectionReference(i ti.Inspection) ti.Reference {
	return ti.Reference{InspectionID: i.ID, ClaimID: i.ClaimID, Fence: i.Fence}
}

func sameInspectionSource(a, b ti.Inspection) bool {
	return a.ID == b.ID && a.ProjectID == b.ProjectID && a.WorkerID == b.WorkerID && a.ToolProfileID == b.ToolProfileID && a.ToolProfileVersion == b.ToolProfileVersion && a.ProfileSHA256 == b.ProfileSHA256 && a.CommandID == b.CommandID && a.Fence == b.Fence && a.ClaimID == b.ClaimID && a.RequestedBy == b.RequestedBy && a.CreatedAt == b.CreatedAt && a.ExpiresAt == b.ExpiresAt && a.ClaimedAt == b.ClaimedAt && a.LeaseUntil == b.LeaseUntil
}

func (c *Client) claimToolInspection(ctx context.Context, worker, claim string) (*ti.Inspection, error) {
	request := ti.ClaimRequest{ProjectID: c.ProjectID, WorkerID: worker, ClaimID: claim}
	if err := ti.ValidateClaimRequest(request); err != nil {
		return nil, err
	}
	if c.Pool != nil && (c.Pool.Assignment != nil || c.Pool.ProjectID != c.ProjectID || c.Pool.TenantID == "") {
		return nil, fmt.Errorf("tool inspection requires the exact project machine scope")
	}
	payload := map[string]any{"project_id": c.ProjectID, "worker_id": worker, "claim_id": claim}
	if c.Pool != nil {
		delete(payload, "worker_id")
	}
	// Presence is checked separately: an omitted property must not be treated
	// as an authoritative no-work response that discards the original nonce.
	var raw map[string]json.RawMessage
	if err := c.post(ctx, "tool-inspections/claim", payload, &raw); err != nil {
		return nil, err
	}
	value, ok := raw["inspection"]
	if !ok || len(raw) != 1 {
		return nil, fmt.Errorf("missing tool inspection claim response")
	}
	if string(value) == "null" {
		return nil, nil
	}
	var i ti.Inspection
	if json.Unmarshal(value, &i) != nil || ti.ValidateInspection(i) != nil || i.ProjectID != c.ProjectID || i.WorkerID != worker || i.ClaimID != claim || i.Fence < 1 {
		return nil, fmt.Errorf("tool inspection claim source differs")
	}
	return &i, nil
}

func (c *Client) failToolInspection(ctx context.Context, p toolInspectionCheckpoint) (*ti.Inspection, error) {
	outcome := "unknown"
	if p.FailureCode == "not_started" {
		outcome = "failed"
	}
	r := ti.FailureRequest{ProjectID: c.ProjectID, WorkerID: p.WorkerID, Inspection: inspectionReference(*p.Inspection), IdempotencyKey: p.FailureOperation, Outcome: outcome, FailureCode: p.FailureCode}
	if err := ti.ValidateFailureRequest(r); err != nil {
		return nil, err
	}
	if c.Pool != nil && (c.Pool.Assignment != nil || c.Pool.ProjectID != c.ProjectID || c.Pool.TenantID == "") {
		return nil, fmt.Errorf("tool inspection requires the exact project machine scope")
	}
	payload := map[string]any{"project_id": r.ProjectID, "worker_id": r.WorkerID, "inspection": r.Inspection, "idempotency_key": r.IdempotencyKey, "outcome": r.Outcome, "failure_code": r.FailureCode}
	if c.Pool != nil {
		delete(payload, "worker_id")
	}
	var reply ti.Inspection
	if err := c.post(ctx, "tool-inspections/results", payload, &reply); err != nil {
		return nil, err
	}
	if ti.ValidateInspection(reply) != nil || !sameInspectionSource(*p.Inspection, reply) || reply.Version < p.Inspection.Version || reply.State != outcome || reply.ResultCode != p.FailureCode {
		return nil, fmt.Errorf("tool inspection failure receipt differs")
	}
	return &reply, nil
}

func (w *Worker) finishInspectionFailure(ctx context.Context, p *toolInspectionCheckpoint, code string) error {
	w.setInventoryUnavailable()
	if p.FailureCode == "" {
		p.FailureCode = code
		p.FailureOperation = operationID()
		if err := w.Journal.saveToolInspection(*p); err != nil {
			return err
		}
	}
	if p.FailureAcked {
		return nil
	}
	i, err := w.Client.failToolInspection(ctx, *p)
	if err != nil {
		return err
	}
	p.Inspection = i
	p.FailureAcked = true
	p.Complete = i.State == "failed"
	return w.Journal.saveToolInspection(*p)
}

func (w *Worker) acceptInventory(report InventoryReport) {
	w.inventoryMu.Lock()
	defer w.inventoryMu.Unlock()
	w.inventory = cloneObservations(report.Items)
	w.inventoryReady = true
}

// serviceToolInventory is the sole inventory owner's serial maintenance step.
// Its boolean reports a fresh snapshot, never a read or any recovered ACK.
// Unknown effects keep the original nonce.
func (w *Worker) serviceToolInventory(ctx context.Context, periodic bool) (bool, error) {
	w.inventoryOpMu.Lock()
	defer w.inventoryOpMu.Unlock()
	if w.Client == nil || w.Journal == nil {
		return false, fmt.Errorf("Runner client and durable journal are required")
	}
	scope, err := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if err != nil {
		return false, err
	}
	p, err := w.Journal.toolInspectionCheckpoint()
	if err != nil {
		return false, err
	}
	if p != nil && (p.ScopeSHA256 != scope || p.WorkerID != w.ID || p.Inspection != nil && p.Inspection.ProjectID != w.Client.ProjectID) {
		return false, fmt.Errorf("tool inspection checkpoint belongs to another scope")
	}
	if pending := w.Journal.PendingInventory(); pending != nil {
		if pending.ScopeSHA256 != scope || pending.WorkerID != w.ID {
			return false, fmt.Errorf("pending inventory belongs to another scope")
		}
		if pending.Inspection != nil && (p == nil || p.Inspection == nil || !p.Started || *pending.Inspection != inspectionReference(*p.Inspection)) {
			return false, fmt.Errorf("inventory has no original inspection checkpoint")
		}
		_, e := w.acknowledgeInventory(ctx, *pending)
		if e != nil {
			return false, e
		}
		if pending.Inspection != nil {
			p.Complete = true
			if e = w.Journal.saveToolInspection(*p); e != nil {
				return false, e
			}
			// The original operation may have been committed long before its ACK
			// reached us. That ACK completes its inspection, not the current TTL.
			w.setInventoryUnavailable()
			w.inventoryNext = time.Time{}
			return false, nil
		}
	}
	// An ACK can be saved immediately before a crash that interrupts checkpoint
	// completion. Recover that same saved report without a second native probe.
	if p != nil && !p.Complete && p.Started && p.Inspection != nil {
		w.Journal.mu.Lock()
		saved := w.Journal.inventory
		matched := saved != nil && saved.Acked && saved.Inspection != nil && *saved.Inspection == inspectionReference(*p.Inspection)
		w.Journal.mu.Unlock()
		if matched {
			p.Complete = true
			if err = w.Journal.saveToolInspection(*p); err != nil {
				return false, err
			}
			w.setInventoryUnavailable()
			w.inventoryNext = time.Time{}
			return false, nil
		}
		if handled, e := w.recoverToolInspection(ctx, p); handled || e != nil {
			return false, e
		}
	}
	p, err = w.Journal.beginToolInspection(w.Client, w.ID)
	if err != nil {
		return false, err
	}
	if !p.Responded || p.Inspection != nil && !p.Started || p.FailureAcked && !p.Complete {
		i, e := w.Client.claimToolInspection(ctx, w.ID, p.ClaimID)
		if e != nil {
			return false, e
		}
		if p.Inspection != nil && (i == nil || !sameInspectionSource(*p.Inspection, *i) || i.Version < p.Inspection.Version) {
			return false, fmt.Errorf("recovered inspection source differs")
		}
		p.Responded = true
		p.Inspection = i
		if i == nil || i.State == "completed" || i.State == "failed" || i.State == "expired" || i.State == "superseded" {
			p.Complete = true
		}
		if err = w.Journal.saveToolInspection(*p); err != nil {
			return false, err
		}
	}
	if p.Inspection != nil && !p.Complete {
		if p.FailureCode != "" {
			return false, w.finishInspectionFailure(ctx, p, p.FailureCode)
		}
		if p.Started {
			return false, w.finishInspectionFailure(ctx, p, "probe_interrupted")
		}
		if p.Inspection.State != "claimed" {
			return false, nil
		}
		deadline, _ := time.Parse(time.RFC3339Nano, p.Inspection.LeaseUntil)
		if !time.Now().Before(deadline) || ctx.Err() != nil {
			return false, w.finishInspectionFailure(ctx, p, "not_started")
		}
		p.Started = true
		if err = w.Journal.saveToolInspection(*p); err != nil {
			return false, err
		}
		probeCtx, cancel := context.WithDeadline(ctx, deadline)
		collector, collectErr := newToolProbeCollector(w.Journal, *p)
		if collectErr != nil {
			cancel()
			return false, w.finishInspectionFailure(ctx, p, "probe_failed")
		}
		probeCtx = collector.context(probeCtx)
		items, probeErr := DescribeTools(probeCtx, w.Exec)
		interrupted := probeCtx.Err()
		cancel()
		if collectErr = collector.closeReturned(); collectErr != nil || collector.hasUnverifiedNativeExit() {
			probeErr = fmt.Errorf("native probe lifecycle was not fully persisted or stopped")
		}
		if probeErr != nil || interrupted != nil {
			code := "probe_failed"
			if interrupted == context.DeadlineExceeded {
				code = "probe_timeout"
			} else if interrupted != nil {
				code = "probe_interrupted"
			}
			w.setInventoryUnavailable()
			return false, w.finishInspectionFailure(ctx, p, code)
		}
		ref := inspectionReference(*p.Inspection)
		report, e := w.Journal.newToolInventory(w.Client.BaseURL, w.Client.ProjectID, w.ID, items, &ref, w.maintenance)
		if e != nil {
			return false, e
		}
		report, e = w.acknowledgeInventory(ctx, report)
		if e != nil {
			return false, e
		}
		p.Complete = true
		if e = w.Journal.saveToolInspection(*p); e != nil {
			return false, e
		}
		w.acceptInventory(report)
		return true, nil
	}
	if periodic {
		report, _, e := w.reportTools(ctx)
		if e != nil {
			return false, e
		}
		w.acceptInventory(report)
		return true, nil
	}
	return false, nil
}

func (w *Worker) tickToolInventory(ctx context.Context) error {
	interval := w.inventoryInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	refreshed, err := w.serviceToolInventory(ctx, w.inventoryNext.IsZero() || !time.Now().Before(w.inventoryNext))
	if err != nil {
		w.setInventoryUnavailable()
		// Reopening the claim gate requires a current full snapshot after an
		// outage; a successful no-work read alone cannot restore readiness.
		w.inventoryNext = time.Time{}
		return err
	}
	if refreshed {
		w.inventoryNext = time.Now().Add(interval)
	}
	return nil
}

func (w *Worker) inventoryPollInterval() time.Duration {
	if w.PollInterval > 0 {
		return w.PollInterval
	}
	return 2 * time.Second
}
