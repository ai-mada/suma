package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	ti "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

type toolInspectionRecoveryCheckpoint struct {
	Source               ti.ToolInspectionRecovery   `json:"source"`
	Proof                *ti.ToolInspectionStopProof `json:"proof,omitempty"`
	ProofOperation       string                      `json:"proof_operation,omitempty"`
	ProofExpectedVersion int                         `json:"proof_expected_version,omitempty"`
	Accepted             *ti.ToolInspectionRecovery  `json:"accepted,omitempty"`
	AckOperation         string                      `json:"ack_operation,omitempty"`
	AckExpectedVersion   int                         `json:"ack_expected_version,omitempty"`
	Closed               *ti.ToolInspectionRecovery  `json:"closed,omitempty"`
	Expired              *ti.ToolInspectionRecovery  `json:"expired,omitempty"`
}

func recoveryMatchesInspection(p toolInspectionCheckpoint, r ti.ToolInspectionRecovery) bool {
	return p.Inspection != nil && ti.ValidateRecovery(r) == nil && r.ProjectID == p.Inspection.ProjectID && r.WorkerID == p.WorkerID && r.Inspection == inspectionReference(*p.Inspection) && r.SourceSHA256 == ti.InspectionSourceSHA256(*p.Inspection)
}
func sameToolRecoverySource(a, b ti.ToolInspectionRecovery) bool {
	return a.ID == b.ID && a.ProjectID == b.ProjectID && a.WorkerID == b.WorkerID && a.Inspection == b.Inspection && a.InspectionVersion == b.InspectionVersion && a.SourceSHA256 == b.SourceSHA256 && a.RequestedBy == b.RequestedBy && a.CreatedAt == b.CreatedAt && a.ExpiresAt == b.ExpiresAt
}
func validateToolRecoveryCheckpoint(p toolInspectionCheckpoint) error {
	r := p.Recovery
	if r == nil {
		return nil
	}
	if !p.Started || !recoveryMatchesInspection(p, r.Source) {
		return fmt.Errorf("recovery checkpoint differs from original inspection")
	}
	if r.Proof != nil && (ti.ValidateStopProof(*r.Proof) != nil || r.Proof.Inspection != r.Source.Inspection || r.Proof.SourceSHA256 != r.Source.SourceSHA256) {
		return fmt.Errorf("recovery checkpoint stop proof differs")
	}
	if r.ProofOperation != "" && (r.Proof == nil || !ti.Operation(r.ProofOperation) || r.ProofExpectedVersion < 1) {
		return fmt.Errorf("invalid original recovery proof intent")
	}
	if r.Accepted != nil && (!recoveryMatchesInspection(p, *r.Accepted) || !sameToolRecoverySource(r.Source, *r.Accepted) || r.Accepted.State != "proof_stored" || r.Proof == nil || r.Accepted.ProofSHA256 != ti.StopProofDigest(*r.Proof)) {
		return fmt.Errorf("invalid accepted recovery proof receipt")
	}
	if r.AckOperation != "" && (r.Accepted == nil || !ti.Operation(r.AckOperation) || r.AckExpectedVersion != r.Accepted.Version) {
		return fmt.Errorf("invalid recovery acknowledgement intent")
	}
	if r.Closed != nil && (!p.Complete || !recoveryMatchesInspection(p, *r.Closed) || !sameToolRecoverySource(r.Source, *r.Closed) || r.Closed.State != "closed_unknown" || r.Proof == nil || r.Closed.ProofSHA256 != ti.StopProofDigest(*r.Proof)) {
		return fmt.Errorf("invalid original recovery closure")
	}
	if r.Expired != nil && (!recoveryMatchesInspection(p, *r.Expired) || !sameToolRecoverySource(r.Source, *r.Expired) || r.Expired.State != "expired" || r.Expired.Version < r.Source.Version) {
		return fmt.Errorf("invalid original expired recovery observation")
	}
	return nil
}

func (j *Journal) originalToolStopProof(p toolInspectionCheckpoint) (ti.ToolInspectionStopProof, error) {
	if j.PendingInventory() != nil {
		return ti.ToolInspectionStopProof{}, fmt.Errorf("pending original inventory must be reconciled first")
	}
	r, sha, err := j.readToolProbeReceipt(p)
	if err != nil {
		return ti.ToolInspectionStopProof{}, err
	}
	proof := ti.ToolInspectionStopProof{Protocol: ti.ProbeStopProtocol, Inspection: r.Source.Inspection, SourceSHA256: r.Source.SourceSHA256, ReceiptSHA256: sha, ProbeReturned: r.ProbeReturned, AllNativeTreesStopped: r.AllNativeTreesStopped, NoPendingInventory: true, NativeProcessCount: r.NativeProcessCount, StoppedProcessCount: r.StoppedProcessCount, ObservedAt: r.ObservedAt}
	if err = ti.ValidateStopProof(proof); err != nil {
		return proof, err
	}
	return proof, nil
}

func (c *Client) toolRecoveryPayload(worker string) (map[string]any, error) {
	if !ti.ID(c.ProjectID) || !ti.ID(worker) {
		return nil, fmt.Errorf("invalid recovery machine scope")
	}
	payload := map[string]any{"project_id": c.ProjectID}
	if c.Pool == nil {
		payload["worker_id"] = worker
	} else if c.Pool.Assignment != nil || c.Pool.ProjectID != c.ProjectID || c.Pool.TenantID == "" {
		return nil, fmt.Errorf("recovery requires exact project machine scope")
	}
	return payload, nil
}
func (c *Client) pollToolRecovery(ctx context.Context, p toolInspectionCheckpoint) (*ti.ToolInspectionRecovery, error) {
	payload, err := c.toolRecoveryPayload(p.WorkerID)
	if err != nil {
		return nil, err
	}
	payload["inspection"] = inspectionReference(*p.Inspection)
	var raw map[string]json.RawMessage
	if err = c.post(ctx, "tool-inspections/recovery", payload, &raw); err != nil {
		return nil, err
	}
	v, ok := raw["recovery"]
	if !ok || len(raw) != 1 {
		return nil, fmt.Errorf("missing recovery poll response")
	}
	if string(v) == "null" {
		return nil, nil
	}
	var r ti.ToolInspectionRecovery
	if json.Unmarshal(v, &r) != nil || !recoveryMatchesInspection(p, r) {
		return nil, fmt.Errorf("recovery poll source differs")
	}
	return &r, nil
}
func (c *Client) postToolRecoveryProof(ctx context.Context, p toolInspectionCheckpoint) (ti.ToolInspectionRecovery, error) {
	r := p.Recovery
	payload, err := c.toolRecoveryPayload(p.WorkerID)
	if err != nil {
		return ti.ToolInspectionRecovery{}, err
	}
	payload["recovery_id"] = r.Source.ID
	payload["expected_version"] = r.ProofExpectedVersion
	payload["idempotency_key"] = r.ProofOperation
	payload["proof"] = r.Proof
	var reply ti.ToolInspectionRecovery
	if err = c.post(ctx, "tool-inspections/recovery/proofs", payload, &reply); err != nil {
		return reply, err
	}
	if !recoveryMatchesInspection(p, reply) || !sameToolRecoverySource(r.Source, reply) || reply.State != "proof_stored" || reply.Version <= r.ProofExpectedVersion || reply.ProofSHA256 != ti.StopProofDigest(*r.Proof) {
		return reply, fmt.Errorf("original recovery proof persistence was not acknowledged")
	}
	return reply, nil
}
func (c *Client) ackToolRecovery(ctx context.Context, p toolInspectionCheckpoint) (ti.ToolInspectionRecovery, error) {
	r := p.Recovery
	payload, err := c.toolRecoveryPayload(p.WorkerID)
	if err != nil {
		return ti.ToolInspectionRecovery{}, err
	}
	payload["recovery_id"] = r.Source.ID
	payload["expected_version"] = r.AckExpectedVersion
	payload["idempotency_key"] = r.AckOperation
	payload["proof_sha256"] = r.Accepted.ProofSHA256
	var reply ti.ToolInspectionRecovery
	if err = c.post(ctx, "tool-inspections/recovery/acknowledgements", payload, &reply); err != nil {
		return reply, err
	}
	if !recoveryMatchesInspection(p, reply) || !sameToolRecoverySource(r.Source, reply) || reply.State != "closed_unknown" || reply.Version <= r.AckExpectedVersion || reply.ProofSHA256 != r.Accepted.ProofSHA256 {
		return reply, fmt.Errorf("original recovery closure was not acknowledged")
	}
	return reply, nil
}

func (j *Journal) archiveClosedToolInspection(p toolInspectionCheckpoint) error {
	if p.Recovery == nil || p.Recovery.Closed == nil || !p.Complete {
		return nil
	}
	if err := validateToolRecoveryCheckpoint(p); err != nil {
		return err
	}
	dir, _, err := j.toolProbeDirectory(p)
	if err != nil {
		return err
	}
	_, err = writeToolProbeImmutable(filepath.Join(dir, "closed-checkpoint.json"), p)
	return err
}

func (j *Journal) archiveExpiredToolRecovery(p toolInspectionCheckpoint, head ti.ToolInspectionRecovery) error {
	if p.Recovery == nil || !recoveryMatchesInspection(p, head) || !sameToolRecoverySource(p.Recovery.Source, head) || head.State != "expired" {
		return fmt.Errorf("expired recovery source differs")
	}
	p.Recovery.Expired = &head
	if err := validateToolRecoveryCheckpoint(p); err != nil {
		return err
	}
	parent := filepath.Join(filepath.Dir(j.path), "tool-inspection-recovery-history")
	dir := filepath.Join(parent, digest([]byte(p.ScopeSHA256+"\x00"+head.Inspection.InspectionID)))
	for _, part := range []string{parent, dir} {
		if err := os.Mkdir(part, 0700); err != nil && !os.IsExist(err) {
			return err
		}
		st, err := os.Lstat(part)
		if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("recovery history directory differs")
		}
	}
	raw, _ := json.Marshal(p.Recovery)
	_, err := writeToolProbeImmutable(filepath.Join(dir, digest([]byte(head.ID))+"-"+digest(raw)+".json"), p.Recovery)
	return err
}

// Only an authoritative expired head of the same recovery permits waiting for
// a new approval. A rejected operation never silently gets a new operation key.
func (w *Worker) reconcileToolRecoveryError(ctx context.Context, p *toolInspectionCheckpoint, cause error) error {
	if !permanentInventoryError(cause) {
		return cause
	}
	head, err := w.Client.pollToolRecovery(ctx, *p)
	if err != nil || head == nil || p.Recovery == nil || !sameToolRecoverySource(p.Recovery.Source, *head) || head.State != "expired" {
		return cause
	}
	if err = w.Journal.archiveExpiredToolRecovery(*p, *head); err != nil {
		return err
	}
	p.Recovery = nil
	return w.Journal.saveToolInspection(*p)
}

// Recovery is a read of original durable stop receipts followed by explicitly
// authorized bookkeeping. It cannot launch/kill a process or create a probe.
func (w *Worker) recoverToolInspection(ctx context.Context, p *toolInspectionCheckpoint) (bool, error) {
	w.setInventoryUnavailable()
	if w.Journal.PendingInventory() != nil {
		return true, fmt.Errorf("pending inventory prevents recovery")
	}
	if p.Recovery == nil {
		r, err := w.Client.pollToolRecovery(ctx, *p)
		if err != nil {
			return true, err
		}
		if r == nil {
			return p.FailureAcked, nil
		}
		p.Recovery = &toolInspectionRecoveryCheckpoint{Source: *r}
		if err = w.Journal.saveToolInspection(*p); err != nil {
			return true, err
		}
	}
	r := p.Recovery
	if r.Source.State == "expired" {
		if err := w.Journal.archiveExpiredToolRecovery(*p, r.Source); err != nil {
			return true, err
		}
		p.Recovery = nil
		return true, w.Journal.saveToolInspection(*p)
	}
	proof, err := w.Journal.originalToolStopProof(*p)
	if err != nil {
		return true, nil
	} // no receipt / weak containment stays blocked
	if r.Proof == nil {
		r.Proof = &proof
		if err = w.Journal.saveToolInspection(*p); err != nil {
			return true, err
		}
	}
	if ti.StopProofDigest(*r.Proof) != ti.StopProofDigest(proof) {
		return true, fmt.Errorf("original stop receipt changed before recovery")
	}
	if r.Accepted == nil {
		if r.Source.State == "proof_stored" {
			if r.Source.ProofSHA256 != ti.StopProofDigest(proof) {
				return true, fmt.Errorf("stored recovery proof differs from original local receipt")
			}
			accepted := r.Source
			r.Accepted = &accepted
		} else {
			if r.Source.State != "requested" {
				return true, fmt.Errorf("recovery cannot adopt an unprepared closure")
			}
			if r.ProofOperation == "" {
				r.ProofOperation = operationID()
				r.ProofExpectedVersion = r.Source.Version
				if err = w.Journal.saveToolInspection(*p); err != nil {
					return true, err
				}
			}
			accepted, e := w.Client.postToolRecoveryProof(ctx, *p)
			if e != nil {
				return true, w.reconcileToolRecoveryError(ctx, p, e)
			}
			r.Accepted = &accepted
		}
		// Persist the accepted proof receipt before preparing any ACK intent.
		if err = w.Journal.saveToolInspection(*p); err != nil {
			return true, err
		}
	}
	if r.AckOperation == "" {
		r.AckOperation = operationID()
		r.AckExpectedVersion = r.Accepted.Version
		if err = w.Journal.saveToolInspection(*p); err != nil {
			return true, err
		}
	}
	closed, err := w.Client.ackToolRecovery(ctx, *p)
	if err != nil {
		return true, w.reconcileToolRecoveryError(ctx, p, err)
	}
	r.Closed = &closed
	p.Complete = true
	if err = w.Journal.saveToolInspection(*p); err != nil {
		return true, err
	}
	if err = w.Journal.archiveClosedToolInspection(*p); err != nil {
		return true, err
	}
	w.inventoryNext = time.Time{}
	return true, nil // the closing tick never falls through into a periodic probe
}
