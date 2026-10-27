package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

type workspaceWriteClaimCheckpoint struct {
	ControlSHA256 string           `json:"control_sha256"`
	Assignment    Assignment       `json:"assignment"`
	Request       wwc.ClaimRequest `json:"request"`
	Complete      bool             `json:"complete"`
	Retired       bool             `json:"retired,omitempty"`
}
type workspaceWriteCheckpoint struct {
	ControlSHA256     string             `json:"control_sha256"`
	Assignment        Assignment         `json:"assignment"`
	Lease             wwc.Lease          `json:"lease"`
	DeliverySHA256    string             `json:"delivery_sha256"`
	ProcessingStarted bool               `json:"processing_started"`
	Result            *wwc.ResultRequest `json:"result,omitempty"`
	Acked             bool               `json:"acked"`
	Retired           bool               `json:"retired,omitempty"`
}
type workspaceWriteJournal struct {
	Protocol string                              `json:"protocol"`
	Claim    *workspaceWriteClaimCheckpoint      `json:"claim,omitempty"`
	Entries  map[string]workspaceWriteCheckpoint `json:"entries"`
}

func cloneWorkspaceWrite[T any](value T) T {
	raw, _ := json.Marshal(value)
	var result T
	_ = json.Unmarshal(raw, &result)
	return result
}
func workspaceWriteKey(controlSHA string, lease wwc.Lease) string {
	return woc.Digest([]string{controlSHA, lease.Command.ProjectID, lease.Command.ID, lease.ClaimID})
}
func workspaceWriteDeliverySHA(lease wwc.Lease) string {
	lease.Content = nil
	c := &lease.Command
	c.Version = 0
	c.State = ""
	c.Result = nil
	c.ErrorCode = ""
	c.CreatedAt = ""
	c.ExpiresAt = ""
	c.InspectionState = ""
	if lease.Mode == "execute" {
		c.InspectionID = ""
	}
	return woc.Digest(lease)
}
func writeAssignmentMatchesScope(a Assignment, scope wwc.Scope) bool {
	return a.RunID == scope.RunID && a.NodeID == scope.NodeID && a.Attempt == scope.Attempt && a.Fence == scope.Fence && a.WorkerID == scope.WorkerID && (a.PoolID == "" || validPoolAssignment(a) == nil)
}
func validWorkspaceWriteCheckpoint(entry workspaceWriteCheckpoint) bool {
	lease, c := entry.Lease, entry.Lease.Command
	if !woc.ValidDigest(entry.ControlSHA256) || entry.DeliverySHA256 != workspaceWriteDeliverySHA(lease) || !writeAssignmentMatchesScope(entry.Assignment, c.Scope) || validateWorkspaceWriteLease(lease, c.ProjectID, c.Scope, c.WorkspacePolicySHA256, entry.ProcessingStarted) != nil {
		return false
	}
	if entry.ProcessingStarted && lease.Content != nil {
		return false
	}
	if result := entry.Result; result != nil {
		if !entry.ProcessingStarted || wwc.ValidateResult(c, *result) != nil || result.Mode != lease.Mode || result.ClaimID != lease.ClaimID || !nativeOperationID(result.OperationID) {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, result.ObservedAt); err != nil {
			return false
		}
	}
	return true
}
func validWorkspaceWriteClaim(claim workspaceWriteClaimCheckpoint) bool {
	r := claim.Request
	return woc.ValidDigest(claim.ControlSHA256) && r.ProjectID != "" && r.WorkerID == r.Scope.WorkerID && woc.ValidateScope(r.Scope) == nil && woc.ValidDigest(r.WorkspacePolicySHA256) && nativeOperationID(r.OperationID) && writeAssignmentMatchesScope(claim.Assignment, r.Scope)
}
func (j *Journal) workspaceWritePath() string {
	return filepath.Join(filepath.Dir(j.path), "workspace-writes.json")
}
func (j *Journal) loadWorkspaceWrites() error {
	state := workspaceWriteJournal{Protocol: wwc.Protocol, Entries: map[string]workspaceWriteCheckpoint{}}
	info, err := os.Lstat(j.workspaceWritePath())
	if os.IsNotExist(err) {
		j.workspaceWrites = state
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 2<<20 {
		return fmt.Errorf("workspace write journal is unavailable or invalid")
	}
	raw, err := os.ReadFile(j.workspaceWritePath())
	if err != nil {
		return err
	}
	if wwc.Decode(raw, &state) != nil || state.Protocol != wwc.Protocol || state.Entries == nil || len(state.Entries) > 256 {
		return fmt.Errorf("invalid workspace write journal")
	}
	if state.Claim != nil && !validWorkspaceWriteClaim(*state.Claim) {
		return fmt.Errorf("workspace write claim scope changed")
	}
	for key, entry := range state.Entries {
		if key != workspaceWriteKey(entry.ControlSHA256, entry.Lease) || !validWorkspaceWriteCheckpoint(entry) {
			return fmt.Errorf("workspace write receipt differs from its original command")
		}
	}
	j.workspaceWrites = state
	return nil
}
func (j *Journal) mutateWorkspaceWritesLocked(change func(*workspaceWriteJournal) error) error {
	next := workspaceWriteJournal{Protocol: wwc.Protocol, Claim: j.workspaceWrites.Claim, Entries: make(map[string]workspaceWriteCheckpoint, len(j.workspaceWrites.Entries))}
	for key, entry := range j.workspaceWrites.Entries {
		next.Entries[key] = entry
	}
	if err := change(&next); err != nil {
		return err
	}
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > 2<<20 {
		return fmt.Errorf("workspace write journal exceeds its bound")
	}
	if err = writeEvidence(j.workspaceWritePath(), raw); err != nil {
		return err
	}
	j.workspaceWrites = next
	return nil
}
func (j *Journal) beginWorkspaceWriteClaim(control string, a Assignment, scope wwc.Scope, policy, project string) (workspaceWriteClaimCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	controlSHA := observationControlSHA(control)
	if old := j.workspaceWrites.Claim; old != nil && !old.Complete && !old.Retired {
		if old.ControlSHA256 != controlSHA || old.Request.ProjectID != project || old.Request.Scope != scope || old.Request.WorkspacePolicySHA256 != policy || !samePoolAssignmentIdentity(old.Assignment, poolAssignmentIdentity(a)) {
			return workspaceWriteClaimCheckpoint{}, fmt.Errorf("pending workspace write claim belongs to its original scope")
		}
		return cloneWorkspaceWrite(*old), nil
	}
	claim := workspaceWriteClaimCheckpoint{ControlSHA256: controlSHA, Assignment: poolAssignmentIdentity(a), Request: wwc.ClaimRequest{ProjectID: project, WorkerID: a.WorkerID, Scope: scope, WorkspacePolicySHA256: policy, OperationID: operationID()}}
	if !validWorkspaceWriteClaim(claim) {
		return claim, fmt.Errorf("workspace write claim scope is invalid")
	}
	err := j.mutateWorkspaceWritesLocked(func(state *workspaceWriteJournal) error {
		copied := cloneWorkspaceWrite(claim)
		state.Claim = &copied
		return nil
	})
	return claim, err
}
func (j *Journal) pendingWorkspaceWriteClaim(control string) *workspaceWriteClaimCheckpoint {
	j.mu.Lock()
	defer j.mu.Unlock()
	claim := j.workspaceWrites.Claim
	if claim == nil || claim.Complete || claim.Retired || claim.ControlSHA256 != observationControlSHA(control) {
		return nil
	}
	copy := cloneWorkspaceWrite(*claim)
	return &copy
}
func (j *Journal) recordWorkspaceWriteClaim(control, operation string, lease *wwc.Lease) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	claim := j.workspaceWrites.Claim
	if claim == nil || claim.ControlSHA256 != observationControlSHA(control) || claim.Request.OperationID != operation || claim.Retired {
		return fmt.Errorf("original workspace write claim is absent")
	}
	return j.mutateWorkspaceWritesLocked(func(state *workspaceWriteJournal) error {
		if lease != nil {
			entry := workspaceWriteCheckpoint{ControlSHA256: claim.ControlSHA256, Assignment: claim.Assignment, Lease: *lease, DeliverySHA256: workspaceWriteDeliverySHA(*lease)}
			if validateWorkspaceWriteLease(*lease, claim.Request.ProjectID, claim.Request.Scope, claim.Request.WorkspacePolicySHA256) != nil || !validWorkspaceWriteCheckpoint(entry) {
				return fmt.Errorf("workspace write delivery differs from original claim")
			}
			key := workspaceWriteKey(entry.ControlSHA256, *lease)
			if old, exists := state.Entries[key]; exists {
				if old.DeliverySHA256 != entry.DeliverySHA256 {
					return fmt.Errorf("workspace write delivery identity was reused")
				}
			} else {
				if len(state.Entries) >= 256 {
					return fmt.Errorf("workspace write journal is full")
				}
				state.Entries[key] = cloneWorkspaceWrite(entry)
			}
		}
		copied := cloneWorkspaceWrite(*claim)
		copied.Complete = true
		state.Claim = &copied
		return nil
	})
}
func (j *Journal) retireWorkspaceWriteClaim(control, operation string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	claim := j.workspaceWrites.Claim
	if claim == nil || claim.ControlSHA256 != observationControlSHA(control) || claim.Request.OperationID != operation {
		return fmt.Errorf("original workspace write claim is absent")
	}
	return j.mutateWorkspaceWritesLocked(func(state *workspaceWriteJournal) error {
		copy := cloneWorkspaceWrite(*claim)
		copy.Retired = true
		state.Claim = &copy
		return nil
	})
}
func (j *Journal) startWorkspaceWrite(control string, lease wwc.Lease) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	key := workspaceWriteKey(observationControlSHA(control), lease)
	entry, ok := j.workspaceWrites.Entries[key]
	if !ok || entry.DeliverySHA256 != workspaceWriteDeliverySHA(lease) {
		return fmt.Errorf("original workspace write delivery is absent")
	}
	if entry.ProcessingStarted {
		return fmt.Errorf("workspace write delivery was already started")
	}
	return j.mutateWorkspaceWritesLocked(func(state *workspaceWriteJournal) error {
		copy := cloneWorkspaceWrite(entry)
		// Once durable, this delivery can only recover its original native
		// receipt. Removing the body makes accidental replay non-executable.
		copy.ProcessingStarted = true
		copy.Lease.Content = nil
		state.Entries[key] = copy
		return nil
	})
}
func (j *Journal) saveWorkspaceWriteResult(control string, lease wwc.Lease, result wwc.ResultRequest) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	key := workspaceWriteKey(observationControlSHA(control), lease)
	entry, ok := j.workspaceWrites.Entries[key]
	if !ok || !entry.ProcessingStarted || entry.DeliverySHA256 != workspaceWriteDeliverySHA(lease) {
		return fmt.Errorf("started workspace write delivery is absent")
	}
	if entry.Result != nil {
		if woc.Digest(entry.Result) != woc.Digest(result) {
			return fmt.Errorf("workspace write result is already fixed")
		}
		return nil
	}
	return j.mutateWorkspaceWritesLocked(func(state *workspaceWriteJournal) error {
		copy := cloneWorkspaceWrite(entry)
		copy.Result = &result
		copy = cloneWorkspaceWrite(copy)
		if !validWorkspaceWriteCheckpoint(copy) {
			return fmt.Errorf("workspace write result differs from original delivery")
		}
		state.Entries[key] = copy
		return nil
	})
}
func (j *Journal) pendingWorkspaceWrites(control string) []workspaceWriteCheckpoint {
	j.mu.Lock()
	defer j.mu.Unlock()
	controlSHA := observationControlSHA(control)
	keys := []string{}
	for key, entry := range j.workspaceWrites.Entries {
		if entry.ControlSHA256 == controlSHA && !entry.Acked && !entry.Retired {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]workspaceWriteCheckpoint, 0, len(keys))
	for _, key := range keys {
		result = append(result, cloneWorkspaceWrite(j.workspaceWrites.Entries[key]))
	}
	return result
}
func (j *Journal) ackWorkspaceWrite(control string, receipt wwc.Receipt) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	controlSHA := observationControlSHA(control)
	for key, entry := range j.workspaceWrites.Entries {
		if entry.ControlSHA256 != controlSHA || entry.Lease.Command.ID != receipt.CommandID || entry.Lease.ClaimID != receipt.ClaimID {
			continue
		}
		if !receipt.Acknowledged || entry.Result == nil || entry.Result.ResultSHA256 != receipt.ResultSHA256 {
			return fmt.Errorf("workspace write acknowledgement differs from original result")
		}
		return j.mutateWorkspaceWritesLocked(func(state *workspaceWriteJournal) error {
			copy := entry
			copy.Acked = true
			state.Entries[key] = copy
			return nil
		})
	}
	return fmt.Errorf("workspace write acknowledgement is absent")
}
func (j *Journal) retireWorkspaceWrite(control string, lease wwc.Lease) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	key := workspaceWriteKey(observationControlSHA(control), lease)
	entry, ok := j.workspaceWrites.Entries[key]
	if !ok {
		return fmt.Errorf("workspace write delivery is absent")
	}
	return j.mutateWorkspaceWritesLocked(func(state *workspaceWriteJournal) error {
		copy := entry
		// An authoritative rejection stops background retries; it is not an
		// acknowledgement and does not erase any already-observed effect.
		copy.Retired = true
		state.Entries[key] = copy
		return nil
	})
}
