package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

type workspaceObservationCheckpoint struct {
	ControlSHA256 string             `json:"control_sha256"`
	Assignment    Assignment         `json:"assignment"`
	Lease         woc.Lease          `json:"lease"`
	RequestSHA256 string             `json:"request_sha256"`
	Result        *woc.ResultRequest `json:"result,omitempty"`
	Acked         bool               `json:"acked"`
	Retired       bool               `json:"retired,omitempty"`
}
type workspaceObservationJournal struct {
	Protocol string                                    `json:"protocol"`
	Entries  map[string]workspaceObservationCheckpoint `json:"entries"`
}

func workspaceObservationKey(controlSHA string, lease woc.Lease) string {
	return woc.Digest([]any{controlSHA, lease.Observation.ProjectID, lease.Observation.ID, lease.LeaseFence})
}
func observationControlSHA(control string) string {
	return digest([]byte(strings.TrimRight(control, "/")))
}
func (j *Journal) workspaceObservationPath() string {
	return filepath.Join(filepath.Dir(j.path), "workspace-observations.json")
}
func cloneWorkspaceObservation(value workspaceObservationCheckpoint) workspaceObservationCheckpoint {
	raw, _ := json.Marshal(value)
	var clone workspaceObservationCheckpoint
	_ = json.Unmarshal(raw, &clone)
	return clone
}
func validWorkspaceObservationCheckpoint(value workspaceObservationCheckpoint) bool {
	lease := value.Lease
	observation := lease.Observation
	a := value.Assignment
	if !woc.ValidDigest(value.ControlSHA256) || value.RequestSHA256 != woc.Digest(lease) || woc.ValidateScope(observation.Scope) != nil || observation.ProjectID == "" || observation.ID == "" || lease.LeaseFence < 1 || !woc.ValidDigest(observation.WorkspacePolicySHA256) || woc.ValidateOperation(observation.Operation, observation.Arguments, observation.ExpectedSHA256) != nil || a.RunID != observation.Scope.RunID || a.NodeID != observation.Scope.NodeID || a.Attempt != observation.Scope.Attempt || a.Fence != observation.Scope.Fence || a.WorkerID != observation.Scope.WorkerID {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, lease.LeaseUntil); err != nil {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, observation.ExpiresAt); err != nil {
		return false
	}
	if a.PoolID != "" && validPoolAssignment(a) != nil {
		return false
	}
	if result := value.Result; result != nil {
		if result.ProjectID != observation.ProjectID || result.RequestID != observation.ID || result.Scope != observation.Scope || result.LeaseFence != lease.LeaseFence || result.WorkspacePolicySHA256 != observation.WorkspacePolicySHA256 || result.WorkerID != a.WorkerID || result.OperationID == "" || result.ResultSHA256 != woc.ResultDigest(result.ObservationResult, result.ErrorCode) || woc.ValidateResult(observation, result.ObservationResult, result.ErrorCode) != nil {
			return false
		}
		if _, err := time.Parse(time.RFC3339Nano, result.ObservedAt); err != nil {
			return false
		}
	}
	return true
}
func (j *Journal) loadWorkspaceObservations() error {
	state := workspaceObservationJournal{Protocol: woc.Protocol, Entries: map[string]workspaceObservationCheckpoint{}}
	info, err := os.Lstat(j.workspaceObservationPath())
	if os.IsNotExist(err) {
		j.workspaceObservations = state.Entries
		return nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 32<<20 {
		return fmt.Errorf("workspace observation journal is unavailable or invalid")
	}
	raw, err := os.ReadFile(j.workspaceObservationPath())
	if err != nil {
		return err
	}
	if _, err = decodeToolObject(raw); err != nil {
		return fmt.Errorf("invalid workspace observation journal JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&state) != nil || decoder.Decode(new(any)) != io.EOF || state.Protocol != woc.Protocol || state.Entries == nil || len(state.Entries) > 256 {
		return fmt.Errorf("invalid workspace observation journal")
	}
	for key, value := range state.Entries {
		if key != workspaceObservationKey(value.ControlSHA256, value.Lease) || !validWorkspaceObservationCheckpoint(value) {
			return fmt.Errorf("workspace observation receipt differs from its original lease")
		}
	}
	j.workspaceObservations = state.Entries
	return nil
}
func (j *Journal) saveWorkspaceObservationsLocked() error {
	raw, err := json.Marshal(workspaceObservationJournal{Protocol: woc.Protocol, Entries: j.workspaceObservations})
	if err != nil || len(raw) > 32<<20 {
		return fmt.Errorf("workspace observation journal exceeds its bound")
	}
	return writeEvidence(j.workspaceObservationPath(), raw)
}
func (j *Journal) checkpointWorkspaceObservation(control string, a Assignment, lease woc.Lease) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	value := workspaceObservationCheckpoint{ControlSHA256: observationControlSHA(control), Assignment: poolAssignmentIdentity(a), Lease: lease, RequestSHA256: woc.Digest(lease)}
	if !validWorkspaceObservationCheckpoint(value) {
		return fmt.Errorf("workspace observation lease scope is invalid")
	}
	key := workspaceObservationKey(value.ControlSHA256, lease)
	if existing, ok := j.workspaceObservations[key]; ok {
		if existing.RequestSHA256 != value.RequestSHA256 || !samePoolAssignmentIdentity(existing.Assignment, value.Assignment) {
			return fmt.Errorf("workspace observation lease identity was reused")
		}
		return nil
	}
	previous := j.workspaceObservations
	next := make(map[string]workspaceObservationCheckpoint, len(previous)+1)
	for id, entry := range previous {
		expires, _ := time.Parse(time.RFC3339Nano, entry.Lease.Observation.ExpiresAt)
		if (entry.Acked || entry.Retired) && !expires.After(time.Now()) {
			continue
		}
		next[id] = entry
	}
	if len(next) >= 256 {
		return fmt.Errorf("workspace observation journal is full")
	}
	next[key] = cloneWorkspaceObservation(value)
	j.workspaceObservations = next
	if err := j.saveWorkspaceObservationsLocked(); err != nil {
		j.workspaceObservations = previous
		return err
	}
	return nil
}
func (j *Journal) saveWorkspaceObservationResult(control string, lease woc.Lease, result woc.ResultRequest) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	key := workspaceObservationKey(observationControlSHA(control), lease)
	value, ok := j.workspaceObservations[key]
	if !ok || value.RequestSHA256 != woc.Digest(lease) {
		return fmt.Errorf("workspace observation lease is absent")
	}
	if value.Result != nil {
		if woc.Digest(value.Result) != woc.Digest(result) {
			return fmt.Errorf("workspace observation result is already fixed")
		}
		return nil
	}
	previous := value
	value.Result = &result
	if !validWorkspaceObservationCheckpoint(value) {
		return fmt.Errorf("workspace observation result differs from the fixed lease")
	}
	j.workspaceObservations[key] = cloneWorkspaceObservation(value)
	if err := j.saveWorkspaceObservationsLocked(); err != nil {
		j.workspaceObservations[key] = previous
		return err
	}
	return nil
}
func (j *Journal) pendingWorkspaceObservations(control string) []workspaceObservationCheckpoint {
	j.mu.Lock()
	defer j.mu.Unlock()
	keys := []string{}
	controlSHA := observationControlSHA(control)
	for key, value := range j.workspaceObservations {
		if !value.Acked && !value.Retired && value.ControlSHA256 == controlSHA {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	result := make([]workspaceObservationCheckpoint, 0, len(keys))
	for _, key := range keys {
		result = append(result, cloneWorkspaceObservation(j.workspaceObservations[key]))
	}
	return result
}
func (j *Journal) ackWorkspaceObservation(control string, receipt woc.Receipt) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	controlSHA := observationControlSHA(control)
	for key, value := range j.workspaceObservations {
		if value.ControlSHA256 != controlSHA || value.Lease.Observation.ID != receipt.RequestID || value.Lease.LeaseFence != receipt.LeaseFence {
			continue
		}
		if !receipt.Acknowledged || value.Result == nil || value.Result.ResultSHA256 != receipt.ResultSHA256 {
			return fmt.Errorf("workspace observation acknowledgement differs from original result")
		}
		previous := value
		value.Acked = true
		j.workspaceObservations[key] = value
		if err := j.saveWorkspaceObservationsLocked(); err != nil {
			j.workspaceObservations[key] = previous
			return err
		}
		return nil
	}
	return fmt.Errorf("workspace observation acknowledgement lease is absent")
}
func (j *Journal) retireWorkspaceObservation(control string, lease woc.Lease) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	key := workspaceObservationKey(observationControlSHA(control), lease)
	value, ok := j.workspaceObservations[key]
	if !ok {
		return fmt.Errorf("workspace observation lease is absent")
	}
	previous := value
	value.Retired = true
	j.workspaceObservations[key] = value
	if err := j.saveWorkspaceObservationsLocked(); err != nil {
		j.workspaceObservations[key] = previous
		return err
	}
	return nil
}
