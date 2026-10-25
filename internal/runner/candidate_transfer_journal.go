package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

func (j *Journal) checkpointCandidate(p candidateTransfer) error {
	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	var copy candidateTransfer
	if err = json.Unmarshal(raw, &copy); err != nil {
		return err
	}
	copy.Result.Assignment = copy.Assignment
	if err = validCandidateCheckpoint(copy); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(p.Assignment)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || e.CandidatePending != nil || e.ArtifactPending != nil || key(e.Assignment) != k || candidateRef(e.Assignment) != candidateRef(p.Assignment) {
		return fmt.Errorf("candidate assignment absent, final or already checkpointed")
	}
	previous := e
	e.CandidatePending = &copy
	j.entries[k] = e
	if err = j.save(); err != nil {
		j.entries[k] = previous
	}
	return err
}

func (j *Journal) pendingCandidates() []candidateTransfer {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := []string{}
	for k, e := range j.entries {
		if e.Result == nil && e.CandidatePending != nil {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	result := make([]candidateTransfer, 0, len(keys))
	for _, k := range keys {
		raw, _ := json.Marshal(j.entries[k].CandidatePending)
		var p candidateTransfer
		_ = json.Unmarshal(raw, &p)
		p.Result.Assignment = p.Assignment
		result = append(result, p)
	}
	return result
}

func validCandidateCheckpoint(p candidateTransfer) error {
	r := p.Result
	if p.Request.Validate() != nil || p.Request.Assignment != candidateRef(p.Assignment) || r.RunID != p.Assignment.RunID || r.NodeID != p.Assignment.NodeID || r.Attempt != p.Assignment.Attempt || r.Fence != p.Assignment.Fence || r.WorkerID != p.Assignment.WorkerID || r.ExitCode != 0 || r.Error != "" || r.Stopped || r.OutcomeUnknown || r.WorkspaceEvidence["clean"] != true || r.WorkspaceEvidence["state"] != "completed" || r.WorkspaceEvidence["head_commit"] != p.Request.HeadCommit || r.WorkspaceEvidence["admin_base_commit"] != p.Request.BaseCommit || r.WorkspaceEvidence["evidence_sha256"] != p.Request.EvidenceSHA256 || p.Request.OperationID != candidateOperation(p.Assignment, "start") || r.OperationID != candidateOperation(p.Assignment, "result") {
		return fmt.Errorf("candidate checkpoint is not the original known successful process")
	}
	if !filepath.IsAbs(p.BundlePath) || filepath.Base(p.BundlePath) != "candidate.bundle" || filepath.Base(filepath.Dir(p.BundlePath)) != attemptKey(p.Assignment) {
		return fmt.Errorf("candidate checkpoint bundle path differs from attempt")
	}
	return nil
}

// Recovery renews the same lease while transferring the already fixed bytes.
// A rejected heartbeat retains the checkpoint and blocks further claims.
func resumeCandidate(ctx context.Context, c *Client, j *Journal, p candidateTransfer) error {
	if err := validCandidateCheckpoint(p); err != nil {
		return err
	}
	if p.Assignment.PoolID != "" {
		if err := validPoolAssignment(p.Assignment); err != nil {
			return err
		}
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	requested, err := c.Heartbeat(runCtx, p.Assignment)
	if err != nil {
		return err
	}
	if requested {
		return fmt.Errorf("candidate transfer cancelled; original result retained for reconciliation")
	}
	heartbeat := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				requested, e := c.Heartbeat(runCtx, p.Assignment)
				if e != nil || requested {
					if e == nil {
						e = fmt.Errorf("candidate transfer cancelled; original result retained for reconciliation")
					}
					heartbeat <- e
					cancel()
					return
				}
			}
		}
	}()
	b, transferErr := uploadCandidate(runCtx, c, p)
	cancel()
	<-done
	select {
	case err = <-heartbeat:
		return err
	default:
	}
	if transferErr != nil {
		return transferErr
	}
	requested, err = c.Heartbeat(ctx, p.Assignment)
	if err != nil {
		return err
	}
	if requested {
		return fmt.Errorf("candidate transfer cancelled before result; original result retained")
	}
	r := p.Result
	r.Assignment = p.Assignment
	r.WorkspaceEvidence["candidate_bundle"] = b
	return j.Finish(r)
}

func ReplayCandidates(ctx context.Context, c *Client, j *Journal) error {
	for _, p := range j.pendingCandidates() {
		scoped := c
		var err error
		if c.Pool != nil {
			scoped, err = c.forPoolAssignment(p.Assignment)
			if err != nil {
				return err
			}
		}
		if err = resumeCandidate(ctx, scoped, j, p); err != nil {
			return err
		}
	}
	return nil
}
