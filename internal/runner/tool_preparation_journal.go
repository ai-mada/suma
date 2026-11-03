package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

type toolPreparationCheckpoint struct {
	ScopeSHA256 string                   `json:"scope_sha256"`
	WorkerID    string                   `json:"worker_id"`
	ClaimID     string                   `json:"claim_id"`
	Responded   bool                     `json:"responded"`
	TenantID    string                   `json:"tenant_id"`
	Operation   *tp.Operation            `json:"operation,omitempty"`
	Started     bool                     `json:"started"`
	Intent      *ACPToolPreparationInput `json:"intent,omitempty"`
	Result      *tp.ResultRequest        `json:"result,omitempty"`
	ResultAcked bool                     `json:"result_acked"`
	Closure     *tp.ResultRequest        `json:"closure,omitempty"`
	Complete    bool                     `json:"complete"`
}

func (j *Journal) preparationPath() string {
	return filepath.Join(filepath.Dir(j.path), "tool-preparation.json")
}
func validateToolPreparationCheckpoint(p toolPreparationCheckpoint) error {
	if !tc.ValidDigest(p.ScopeSHA256) || !ii.ID(p.WorkerID) || !ii.Operation(p.ClaimID) || p.Complete && !p.Responded || p.ResultAcked && p.Result == nil {
		return fmt.Errorf("invalid preparation checkpoint")
	}
	if p.Responded && !ii.ID(p.TenantID) || !p.Responded && (p.Operation != nil || p.Started || p.Result != nil) || p.Operation == nil && (p.Started || p.Result != nil || p.Closure != nil) {
		return fmt.Errorf("unacknowledged preparation source")
	}
	if p.Operation != nil {
		op := p.Operation
		if tp.ValidateOperation(*op) != nil || op.WorkerID != p.WorkerID || op.ClaimID != p.ClaimID || op.TenantID != p.TenantID {
			return fmt.Errorf("preparation checkpoint source differs")
		}
		for _, r := range []*tp.ResultRequest{p.Result, p.Closure} {
			if r != nil && (tp.ValidateResultRequest(*r) != nil || r.Preparation != preparationReference(*op) || r.SourceSHA256 != op.SourceSHA256 || r.ProjectID != op.ProjectID || r.WorkerID != op.WorkerID) {
				return fmt.Errorf("preparation result intent differs")
			}
		}
	}
	if p.Started && p.Intent == nil || p.Closure != nil && (p.Result == nil || !p.ResultAcked || p.Result.Outcome != "unknown" || p.Closure.Outcome != "closed_unknown" || p.Closure.IdempotencyKey == p.Result.IdempotencyKey) {
		return fmt.Errorf("preparation closure cannot rewrite original intent")
	}
	return nil
}
func (j *Journal) preparationCheckpoint() (*toolPreparationCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	var p toolPreparationCheckpoint
	if _, err := os.Lstat(j.preparationPath()); os.IsNotExist(err) {
		return nil, nil
	}
	if _, err := readToolProbeJSON(j.preparationPath(), &p); err != nil {
		return nil, err
	}
	if err := validateToolPreparationCheckpoint(p); err != nil {
		return nil, err
	}
	return &p, nil
}
func (j *Journal) savePreparation(p toolPreparationCheckpoint) error {
	if err := validateToolPreparationCheckpoint(p); err != nil {
		return err
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return writeEvidence(j.preparationPath(), raw)
}
func (j *Journal) archivePreparation(p toolPreparationCheckpoint) error {
	if !p.Complete || p.Operation == nil {
		return fmt.Errorf("completed preparation required")
	}
	dir, err := j.maintenanceDirectory("tool-preparation-history")
	if err != nil {
		return err
	}
	_, err = writeToolProbeImmutable(filepath.Join(dir, p.ClaimID+".json"), p)
	return err
}
func (j *Journal) preparationProbeDirectory(p toolPreparationCheckpoint) (string, toolProbeSource, error) {
	if p.Operation == nil || !p.Started || validateToolPreparationCheckpoint(p) != nil {
		return "", toolProbeSource{}, fmt.Errorf("original started preparation required")
	}
	source := toolProbeSource{Protocol: tp.StopProtocol, ScopeSHA256: p.ScopeSHA256, Preparation: preparationReference(*p.Operation), SourceSHA256: p.Operation.SourceSHA256}
	raw, _ := json.Marshal(source)
	return filepath.Join(filepath.Dir(j.path), "tool-preparation-receipts", digest(raw)), source, nil
}
func (j *Journal) preparationStopProof(p toolPreparationCheckpoint) (tp.StopProof, error) {
	dir, source, err := j.preparationProbeDirectory(p)
	if err != nil {
		return tp.StopProof{}, err
	}
	r, sha, err := readToolProbeReceiptAt(dir, source)
	if err != nil {
		return tp.StopProof{}, err
	}
	proof := tp.StopProof{Protocol: tp.StopProtocol, Preparation: source.Preparation, SourceSHA256: source.SourceSHA256, ReceiptSHA256: sha, ProbeReturned: r.ProbeReturned, AllNativeTreesStopped: r.AllNativeTreesStopped, NativeProcessCount: r.NativeProcessCount, StoppedProcessCount: r.StoppedProcessCount, ObservedAt: r.ObservedAt}
	return proof, tp.ValidateStopProof(proof)
}
