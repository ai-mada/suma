package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

// One checkpoint belongs to the existing inventory owner. Result and closure
// have separate immutable command intents: unknown is never rewritten in place.
type installationCheckpoint struct {
	ScopeSHA256 string            `json:"scope_sha256"`
	WorkerID    string            `json:"worker_id"`
	ClaimID     string            `json:"claim_id"`
	Responded   bool              `json:"responded"`
	Scan        *ii.Scan          `json:"scan,omitempty"`
	Started     bool              `json:"started"`
	Result      *ii.ResultRequest `json:"result,omitempty"`
	ResultAcked bool              `json:"result_acked"`
	Closure     *ii.ResultRequest `json:"closure,omitempty"`
	Complete    bool              `json:"complete"`
}

func scanReference(s ii.Scan) ii.Reference {
	return ii.Reference{ScanID: s.ID, ClaimID: s.ClaimID, Fence: s.Fence}
}
func (j *Journal) installationPath() string {
	return filepath.Join(filepath.Dir(j.path), "tool-installation-scan.json")
}
func validateInstallationCheckpoint(p installationCheckpoint) error {
	if !ii.Operation(p.ClaimID) || !ii.ID(p.WorkerID) || !tc.ValidDigest(p.ScopeSHA256) || p.Complete && !p.Responded {
		return fmt.Errorf("invalid installation checkpoint")
	}
	if !p.Responded && (p.Scan != nil || p.Started || p.Result != nil || p.ResultAcked || p.Closure != nil) || p.Responded && p.Scan == nil && (!p.Complete || p.Started || p.Result != nil || p.Closure != nil) || p.Started && p.Scan == nil {
		return fmt.Errorf("installation checkpoint has unacknowledged source facts")
	}
	if p.Scan != nil {
		if ii.ValidateScan(*p.Scan) != nil || p.Scan.WorkerID != p.WorkerID || p.Scan.ClaimID != p.ClaimID || p.Scan.Fence < 1 {
			return fmt.Errorf("installation checkpoint source differs")
		}
	}
	for _, r := range []*ii.ResultRequest{p.Result, p.Closure} {
		if r != nil && (p.Scan == nil || !p.Started || ii.ValidateResultRequest(*r) != nil || r.Scan != scanReference(*p.Scan) || r.ProjectID != p.Scan.ProjectID || r.WorkerID != p.WorkerID || r.StopProof != nil && r.StopProof.SourceSHA256 != p.Scan.SourceSHA256) {
			return fmt.Errorf("installation result source differs")
		}
	}
	if p.ResultAcked && p.Result == nil || p.Closure != nil && (p.Result == nil || p.Result.Outcome != "unknown" || !p.ResultAcked || p.Closure.Outcome != "closed_unknown" || p.Closure.IdempotencyKey == p.Result.IdempotencyKey) {
		return fmt.Errorf("invalid installation closure intent")
	}
	if p.Result != nil && p.Result.Outcome == "closed_unknown" || p.Closure != nil && p.Closure.FailureCode != p.Result.FailureCode {
		return fmt.Errorf("original installation outcome cannot be rewritten")
	}
	if p.Result != nil && p.Result.Outcome == "completed" {
		if len(p.Result.Items) != len(p.Scan.AdapterIDs) {
			return fmt.Errorf("incomplete original scan coverage")
		}
		for i, id := range p.Scan.AdapterIDs {
			if p.Result.Items[i].AdapterID != id {
				return fmt.Errorf("original scan order differs")
			}
		}
	}
	return nil
}
func (j *Journal) installationCheckpoint() (*installationCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var p installationCheckpoint
	if _, err := os.Lstat(j.installationPath()); os.IsNotExist(err) {
		return nil, nil
	}
	if _, err := readToolProbeJSON(j.installationPath(), &p); err != nil {
		return nil, err
	}
	if err := validateInstallationCheckpoint(p); err != nil {
		return nil, err
	}
	return &p, nil
}
func (j *Journal) saveInstallation(p installationCheckpoint) error {
	if err := validateInstallationCheckpoint(p); err != nil {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return writeEvidence(j.installationPath(), raw)
}

// Preserve the completed original command intents before rotating the current
// checkpoint. No archive is created for an empty polling receipt.
func (j *Journal) archiveInstallation(p installationCheckpoint) error {
	if !p.Complete || p.Scan == nil || validateInstallationCheckpoint(p) != nil {
		return fmt.Errorf("completed installation checkpoint required")
	}
	directory := filepath.Join(filepath.Dir(j.path), "tool-installation-history")
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("installation history directory unavailable")
	}
	_, err = writeToolProbeImmutable(filepath.Join(directory, p.ClaimID+".json"), p)
	return err
}
func (j *Journal) installationProbeDirectory(p installationCheckpoint) (string, toolProbeSource, error) {
	if validateInstallationCheckpoint(p) != nil || p.Scan == nil || !p.Started {
		return "", toolProbeSource{}, fmt.Errorf("original installation handling checkpoint required")
	}
	source := toolProbeSource{Protocol: ii.StopProtocol, ScopeSHA256: p.ScopeSHA256, InstallationScan: scanReference(*p.Scan), SourceSHA256: p.Scan.SourceSHA256}
	raw, _ := json.Marshal(source)
	return filepath.Join(filepath.Dir(j.path), "tool-installation-receipts", digest(raw)), source, nil
}
func (j *Journal) installationStopProof(p installationCheckpoint) (ii.StopProof, error) {
	dir, source, err := j.installationProbeDirectory(p)
	if err != nil {
		return ii.StopProof{}, err
	}
	receipt, sha, err := readToolProbeReceiptAt(dir, source)
	if err != nil {
		return ii.StopProof{}, err
	}
	proof := ii.StopProof{Protocol: ii.StopProtocol, Scan: scanReference(*p.Scan), SourceSHA256: source.SourceSHA256, ReceiptSHA256: sha, ProbeReturned: receipt.ProbeReturned, AllNativeTreesStopped: receipt.AllNativeTreesStopped, NativeProcessCount: receipt.NativeProcessCount, StoppedProcessCount: receipt.StoppedProcessCount, ObservedAt: receipt.ObservedAt}
	return proof, ii.ValidateStopProof(proof)
}
func installationItems(report InstalledToolDiscovery, ids []string) ([]ii.Item, error) {
	if report.Protocol != InstalledToolDiscoveryProtocol || len(report.Items) != len(ids) {
		return nil, fmt.Errorf("installation observation coverage differs")
	}
	result := make([]ii.Item, len(ids))
	for i, row := range report.Items {
		if row.AdapterID != ids[i] {
			return nil, fmt.Errorf("installation observation order differs")
		}
		result[i] = ii.Item{AdapterID: row.AdapterID, Protocol: ii.AdapterProtocol(row.AdapterID), Installed: row.Installed, LaunchSupported: row.LaunchSupported, ReasonCode: row.ReasonCode, Version: row.Version, ExecutableSHA256: row.ExecutableSHA256}
		if err := ii.ValidateItem(result[i]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Called only under inventoryOpMu, after original inventory and inspection
// receipts. No new scheduler, goroutine, task or arbitrary command is created.
func (w *Worker) serviceInstallationScan(ctx context.Context) (bool, error) {
	p, err := w.Journal.installationCheckpoint()
	if err != nil {
		return true, err
	}
	if !w.AllowInstalledToolDiscovery {
		if p != nil && !p.Complete {
			return true, fmt.Errorf("original installation scan requires explicit discovery opt-in")
		}
		return false, nil
	}
	scope, err := InventoryScopeSHA256(w.Client.BaseURL, w.Client.ProjectID, w.ID)
	if err != nil {
		return true, err
	}
	if p != nil && (p.ScopeSHA256 != scope || p.WorkerID != w.ID || p.Scan != nil && p.Scan.ProjectID != w.Client.ProjectID) {
		return true, fmt.Errorf("installation scan belongs to another current scope")
	}
	if p == nil || p.Complete {
		// Publish the continuous capability first; fresh inventory is not inferred
		// from a claim read or an ACK restored from an earlier process.
		if !w.canClaimInventory() {
			return false, nil
		}
		if p != nil && p.Scan != nil {
			if err = w.Journal.archiveInstallation(*p); err != nil {
				return true, err
			}
		}
		p = &installationCheckpoint{ScopeSHA256: scope, WorkerID: w.ID, ClaimID: operationID()}
		if err = w.Journal.saveInstallation(*p); err != nil {
			return true, err
		}
	}
	if !p.Responded {
		scan, e := w.Client.claimInstallationScan(ctx, w.ID, p.ClaimID)
		if e != nil {
			return true, e
		}
		p.Responded, p.Scan = true, scan
		if scan == nil {
			p.Complete = true
		} else {
			if w.maintenance == nil || scan.InstanceID != w.maintenance.InstanceID {
				return true, fmt.Errorf("installation scan logical owner differs")
			}
			if scan.State != "claimed" && scan.State != "unknown" {
				p.Complete = true
			}
		}
		if err = w.Journal.saveInstallation(*p); err != nil {
			return true, err
		}
	}
	if p.Complete {
		return false, nil
	}
	if p.Result != nil && !p.ResultAcked {
		return true, w.deliverInstallation(ctx, p, p.Result, false)
	}
	if p.Closure != nil {
		return true, w.deliverInstallation(ctx, p, p.Closure, true)
	}
	if p.ResultAcked {
		if p.Result.Outcome != "unknown" {
			return true, fmt.Errorf("unfinished acknowledged installation result")
		}
		proof, e := w.Journal.installationStopProof(*p)
		if e != nil {
			w.setInventoryUnavailable()
			return true, nil
		}
		closed := *p.Result
		closed.IdempotencyKey = operationID()
		closed.Outcome = "closed_unknown"
		closed.StopProof = &proof
		p.Closure = &closed
		if err = w.Journal.saveInstallation(*p); err != nil {
			return true, err
		}
		return true, w.deliverInstallation(ctx, p, p.Closure, true)
	}
	if p.Started || p.Scan.State == "unknown" {
		p.Started = true
		p.Result = &ii.ResultRequest{ProjectID: w.Client.ProjectID, WorkerID: w.ID, Scan: scanReference(*p.Scan), IdempotencyKey: operationID(), Outcome: "unknown", Items: []ii.Item{}, FailureCode: "worker_restarted"}
	} else {
		p.Started = true
		if err = w.Journal.saveInstallation(*p); err != nil {
			return true, err
		}
		directory, source, e := w.Journal.installationProbeDirectory(*p)
		if e != nil {
			return true, e
		}
		collector, e := newToolProbeCollectorAt(directory, source)
		if e != nil {
			return true, e
		}
		deadline, _ := time.Parse(time.RFC3339Nano, p.Scan.LeaseUntil)
		result := ii.ResultRequest{ProjectID: w.Client.ProjectID, WorkerID: w.ID, Scan: scanReference(*p.Scan), IdempotencyKey: operationID(), Items: []ii.Item{}}
		notStarted := ctx.Err() != nil || !time.Now().Before(deadline)
		var report InstalledToolDiscovery
		var probeErr error
		var interrupted error
		if !notStarted {
			probeCtx, cancel := context.WithDeadline(ctx, deadline)
			probeCtx = collector.context(probeCtx)
			report, probeErr = discoverInstalledToolSubset(probeCtx, p.Scan.AdapterIDs)
			interrupted = probeCtx.Err()
			cancel()
		}
		closeErr := collector.closeReturned()
		proof, proofErr := w.Journal.installationStopProof(*p)
		if notStarted && closeErr == nil && proofErr == nil {
			result.Outcome = "failed"
			result.FailureCode = "not_started"
			result.StopProof = &proof
		} else {
			result.Outcome = "unknown"
			result.FailureCode = "probe_interrupted"
			if interrupted == context.DeadlineExceeded {
				result.FailureCode = "probe_timeout"
			}
			if probeErr == nil && interrupted == nil && closeErr == nil && proofErr == nil {
				items, itemErr := installationItems(report, p.Scan.AdapterIDs)
				if itemErr == nil {
					if w.AllowToolPreparation {
						if err = w.Journal.saveInstallationObservation(ctx, *p, report, items); err != nil {
							return true, err
						}
					}
					result.Outcome = "completed"
					result.FailureCode = ""
					result.Items = items
					result.StopProof = &proof
				}
			}
		}
		p.Result = &result
	}
	if err = w.Journal.saveInstallation(*p); err != nil {
		return true, err
	}
	return true, w.deliverInstallation(ctx, p, p.Result, false)
}
func (w *Worker) deliverInstallation(ctx context.Context, p *installationCheckpoint, request *ii.ResultRequest, closure bool) error {
	head, err := w.Client.resultInstallationScan(ctx, *p.Scan, *request)
	if err != nil {
		return err
	}
	if closure {
		p.Complete = true
	} else {
		p.ResultAcked = true
		p.Complete = head.State != "unknown"
	}
	if err = w.Journal.saveInstallation(*p); err != nil {
		return err
	}
	if !p.Complete {
		w.setInventoryUnavailable()
	}
	return nil
}

func sameInstallationScan(a, b ii.Scan) bool {
	return a.ID == b.ID && a.ProjectID == b.ProjectID && a.WorkerID == b.WorkerID && a.SourceSHA256 == b.SourceSHA256 && a.InstanceID == b.InstanceID && a.ClaimID == b.ClaimID && a.Fence == b.Fence && a.ClaimedAt == b.ClaimedAt && a.LeaseUntil == b.LeaseUntil && slices.Equal(a.AdapterIDs, b.AdapterIDs)
}
