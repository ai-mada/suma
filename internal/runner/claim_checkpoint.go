package runner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ClaimCheckpoint is private durable state, never a public diagnostic response.
// A response is saved before accepting an inbox; Complete is saved only after
// that inbox is durable. Recovery refreshes the same dispatch at the server.
type ClaimCheckpoint struct {
	SchemaVersion int         `json:"schema_version"`
	OperationID   string      `json:"idempotency_key"`
	ControlSHA256 string      `json:"control_sha256"`
	ProjectID     string      `json:"project_id"`
	WorkerID      string      `json:"worker_id"`
	Capabilities  []string    `json:"capabilities"`
	Responded     bool        `json:"responded"`
	Complete      bool        `json:"complete"`
	Assignment    *Assignment `json:"assignment,omitempty"`
}

func (j *Journal) claimPath() string { return filepath.Join(filepath.Dir(j.path), "claim.json") }
func cloneClaim(v *ClaimCheckpoint) ClaimCheckpoint {
	copy := *v
	copy.Capabilities = append([]string{}, v.Capabilities...)
	if v.Assignment != nil {
		a := *v.Assignment
		copy.Assignment = &a
	}
	return copy
}
func (j *Journal) loadClaim() error {
	st, err := os.Lstat(j.claimPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0 || st.Size() > 2<<20 {
		return fmt.Errorf("claim journal is not a bounded regular file")
	}
	raw, err := os.ReadFile(j.claimPath())
	if err != nil {
		return err
	}
	var c ClaimCheckpoint
	if err = json.Unmarshal(raw, &c); err != nil || c.SchemaVersion != 1 || len(c.OperationID) != 32 || len(c.ControlSHA256) != 64 || c.ProjectID == "" || c.WorkerID == "" || len(c.Capabilities) > 512 || c.Complete && !c.Responded {
		return fmt.Errorf("invalid claim journal; refusing new claims")
	}
	if c.Responded && c.Assignment != nil {
		if err = validateDispatch(*c.Assignment, c.WorkerID); err != nil {
			return err
		}
	}
	j.claim = &c
	return nil
}
func (j *Journal) saveClaim(c ClaimCheckpoint) error {
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err = writeEvidence(j.claimPath(), raw); err != nil {
		return err
	}
	j.claim = &c
	return nil
}
func (j *Journal) BeginClaim(control, project, worker string, capabilities []string) (ClaimCheckpoint, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	controlSHA := digest([]byte(strings.TrimRight(control, "/")))
	if project == "" || worker == "" || len(worker) > 100 || len(project) > 100 || len(capabilities) > 512 {
		return ClaimCheckpoint{}, fmt.Errorf("invalid claim scope")
	}
	if j.claim != nil && !j.claim.Complete {
		if j.claim.ProjectID != project || j.claim.WorkerID != worker || j.claim.ControlSHA256 != controlSHA {
			return ClaimCheckpoint{}, fmt.Errorf("pending claim belongs to a different control/project/worker; reconcile its original scope")
		}
		return cloneClaim(j.claim), nil
	}
	caps := append([]string{}, capabilities...)
	sort.Strings(caps)
	for i, c := range caps {
		if c == "" || len(c) > 200 || strings.ContainsAny(c, "\x00\r\n\t ") || i > 0 && caps[i-1] == c {
			return ClaimCheckpoint{}, fmt.Errorf("invalid or duplicate claim capability")
		}
	}
	c := ClaimCheckpoint{SchemaVersion: 1, OperationID: operationID(), ControlSHA256: controlSHA, ProjectID: project, WorkerID: worker, Capabilities: caps}
	if err := j.saveClaim(c); err != nil {
		return ClaimCheckpoint{}, err
	}
	return cloneClaim(&c), nil
}
func (j *Journal) RecordClaimResponse(operation string, a *Assignment) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.claim == nil || j.claim.OperationID != operation || j.claim.Complete {
		return fmt.Errorf("pending claim absent")
	}
	if a != nil {
		if err := validateDispatch(*a, j.claim.WorkerID); err != nil {
			return err
		}
	}
	c := cloneClaim(j.claim)
	c.Responded = true
	c.Assignment = a
	return j.saveClaim(c)
}
func (j *Journal) CompleteClaim(operation string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.claim == nil || j.claim.OperationID != operation || !j.claim.Responded {
		return fmt.Errorf("claim response is not durable")
	}
	c := cloneClaim(j.claim)
	c.Complete = true
	return j.saveClaim(c)
}
func validateDispatch(a Assignment, worker string) error {
	if a.WorkerID != worker || a.RunID == "" || a.NodeID == "" || a.Attempt < 1 || a.Fence < 1 || a.DispatchID == "" || len(a.DispatchID) > 150 {
		return fmt.Errorf("invalid assignment dispatch identity")
	}
	if a.DispatchState != "active" && a.DispatchState != "closed" && a.DispatchState != "reconciling" {
		return fmt.Errorf("invalid assignment dispatch state")
	}
	if _, err := time.Parse(time.RFC3339, a.LeaseUntil); err != nil {
		return fmt.Errorf("invalid assignment lease observation")
	}
	return nil
}
