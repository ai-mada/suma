package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"

	wc "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

// This private parent-only reference is installed before MCP admission. The
// child never receives the ledger directory. Recovery reads receipts through
// os.Root and never invokes the workspace, a check, a model or Git.
type workspaceTraceCheckpoint struct {
	Version          int    `json:"version"`
	Directory        string `json:"directory"`
	AssignmentSHA256 string `json:"assignment_sha256"`
	PolicySHA256     string `json:"policy_sha256"`
	ProfileSHA256    string `json:"profile_sha256"`
	Truncated        bool   `json:"truncated"`
}

func cloneWorkspaceEvent(event *wc.Event) *wc.Event {
	if event == nil {
		return nil
	}
	copy := wc.CloneEvent(*event)
	return &copy
}

func validateWorkspaceTraceCheckpoint(a Assignment, checkpoint workspaceTraceCheckpoint) error {
	sha, err := workspaceTraceAssignmentSHA(a)
	if err != nil || checkpoint.Version != 1 || !filepath.IsAbs(checkpoint.Directory) || filepath.Clean(checkpoint.Directory) != checkpoint.Directory || checkpoint.AssignmentSHA256 != sha || !evidenceHash.MatchString(checkpoint.PolicySHA256) || !evidenceHash.MatchString(checkpoint.ProfileSHA256) || wc.ValidateIdentity(workspaceOperationIdentity(a)) != nil || a.Type != "agent" || len(a.Command) != 1 {
		return fmt.Errorf("workspace trace checkpoint has invalid original scope")
	}
	sha, _ = a.Config["tool_profile_sha256"].(string)
	if sha != checkpoint.ProfileSHA256 {
		return fmt.Errorf("workspace trace checkpoint differs from the original tool profile")
	}
	return nil
}

func (j *Journal) attachWorkspaceTrace(a Assignment, directory, policy, profile string) error {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil || resolved != directory {
		return fmt.Errorf("workspace trace directory must be private and direct")
	}
	assignmentSHA, err := workspaceTraceAssignmentSHA(a)
	if err != nil {
		return err
	}
	checkpoint := workspaceTraceCheckpoint{Version: 1, Directory: directory, AssignmentSHA256: assignmentSHA, PolicySHA256: policy, ProfileSHA256: profile}
	if err = validateWorkspaceTraceCheckpoint(a, checkpoint); err != nil {
		return err
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || !sameWorkspaceTraceAssignment(a, e.Assignment) {
		return fmt.Errorf("workspace trace assignment is absent or final")
	}
	if e.WorkspaceTrace != nil {
		if !reflect.DeepEqual(*e.WorkspaceTrace, checkpoint) {
			return fmt.Errorf("workspace trace reference cannot replace an original scope")
		}
		return nil
	}
	previous := e
	e.WorkspaceTrace = &checkpoint
	j.entries[k] = e
	if err = j.save(); err != nil {
		j.entries[k] = previous
	}
	return err
}

func (j *Journal) noteWorkspaceTraceCoverage(a Assignment, truncated bool) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.WorkspaceTrace == nil || !sameWorkspaceTraceAssignment(a, e.Assignment) {
		return fmt.Errorf("workspace trace reference is absent")
	}
	if e.WorkspaceTrace.Truncated || !truncated {
		return nil
	}
	previous := e
	checkpoint := *e.WorkspaceTrace
	checkpoint.Truncated = true
	e.WorkspaceTrace = &checkpoint
	j.entries[k] = e
	if err := j.save(); err != nil {
		j.entries[k] = previous
		return err
	}
	return nil
}

// Workspace progress has its own 4096-record budget. The original stable event
// ID is also the HTTP operation ID, so reconnects preserve the same receipt.
// This API is called only by the Go host callback, never by toolStream.
func (j *Journal) RecordWorkspaceProgress(a Assignment, event wc.Event) error {
	if wc.ValidateBoundEvent(workspaceOperationIdentity(a), event) != nil {
		return fmt.Errorf("invalid host workspace event")
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	k := key(a)
	e, ok := j.entries[k]
	if !ok || e.Result != nil || !sameWorkspaceTraceAssignment(a, e.Assignment) || e.WorkspaceTrace == nil {
		return fmt.Errorf("workspace event assignment is absent, final or unbound")
	}
	previous := e
	changed, err := appendWorkspaceProgress(&e, event)
	if err != nil || !changed {
		return err
	}
	j.entries[k] = e
	if err = j.save(); err != nil {
		j.entries[k] = previous
	}
	return err
}

func appendWorkspaceProgress(e *entry, event wc.Event) (bool, error) {
	checkpoint := e.WorkspaceTrace
	if checkpoint == nil || validateWorkspaceTraceCheckpoint(e.Assignment, *checkpoint) != nil || event.PolicySHA256 != checkpoint.PolicySHA256 || event.ProfileSHA256 != checkpoint.ProfileSHA256 || wc.ValidateBoundEvent(workspaceOperationIdentity(e.Assignment), event) != nil {
		return false, fmt.Errorf("workspace event differs from original checkpoint")
	}
	count := 0
	for _, p := range e.Progress {
		if p.HostOperation == nil {
			continue
		}
		count++
		if p.OperationID == event.EventID {
			if p.ToolEvent != nil || p.Message != "" || !reflect.DeepEqual(*p.HostOperation, event) {
				return false, fmt.Errorf("workspace event identity already has another body")
			}
			return false, nil
		}
		if p.HostOperation.Sequence == event.Sequence {
			return false, fmt.Errorf("workspace event sequence already belongs to another transition")
		}
	}
	if count >= wc.MaxRecords {
		return false, fmt.Errorf("workspace event capacity exceeded")
	}
	copy := wc.CloneEvent(event)
	e.Progress = append(append([]Progress(nil), e.Progress...), Progress{Assignment: cloneModelAssignment(e.Assignment), OperationID: event.EventID, HostOperation: &copy})
	return true, nil
}

func readWorkspaceTraceLedger(checkpoint workspaceTraceCheckpoint) (nativeWorkspaceLedger, error) {
	var ledger nativeWorkspaceLedger
	root, err := os.OpenRoot(checkpoint.Directory)
	if err != nil {
		return ledger, fmt.Errorf("workspace trace storage is unavailable")
	}
	defer root.Close()
	st, err := root.Lstat("native-workspace.json")
	if err != nil || !st.Mode().IsRegular() || st.Size() > 32<<20 {
		return ledger, fmt.Errorf("workspace trace storage is invalid")
	}
	f, err := root.Open("native-workspace.json")
	if err != nil {
		return ledger, fmt.Errorf("workspace trace storage is unavailable")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 32<<20+1))
	if err != nil || len(raw) > 32<<20 {
		return ledger, fmt.Errorf("workspace trace storage exceeds its bound")
	}
	if _, err = decodeToolObjectBounded(raw, 1<<20); err != nil {
		return ledger, fmt.Errorf("workspace trace storage has invalid JSON")
	}
	if json.Unmarshal(raw, &ledger) != nil || ledger.Version != 1 || ledger.PolicySHA != checkpoint.PolicySHA256 || ledger.Trace.ProfileSHA256 != checkpoint.ProfileSHA256 {
		return ledger, fmt.Errorf("workspace trace storage differs from its checkpoint")
	}
	return ledger, nil
}

// Called while opening the exclusive Journal, before unfinished attempts are
// classified unknown. Even if final progress delivery failed, the same private
// ledger supplies its already-persisted safe transition. A started-only call is
// conservatively unknown and is not used as authorization to retry its effect.
func (j *Journal) importWorkspaceTrace(e *entry) error {
	checkpoint := e.WorkspaceTrace
	if checkpoint == nil {
		return nil
	}
	if err := validateWorkspaceTraceCheckpoint(e.Assignment, *checkpoint); err != nil {
		return err
	}
	ledger, err := readWorkspaceTraceLedger(*checkpoint)
	if err != nil {
		return err
	}
	assignmentSHA, hashErr := workspaceTraceAssignmentSHA(ledger.Assignment)
	if hashErr != nil || assignmentSHA != checkpoint.AssignmentSHA256 || !sameWorkspaceTraceAssignment(e.Assignment, ledger.Assignment) {
		return fmt.Errorf("workspace trace original assignment changed")
	}
	events, err := traceEvents(e.Assignment, checkpoint.PolicySHA256, ledger.Trace)
	if err != nil {
		return err
	}
	for _, event := range events {
		// A restart's original unknown terminal cannot be replaced by a late
		// old process writing a newer private receipt after ownership was lost.
		preserved := false
		if event.Revision == 2 {
			for _, p := range e.Progress {
				if p.HostOperation != nil && p.OperationID == event.EventID && p.HostOperation.Phase == "unknown" && p.HostOperation.ErrorCode == "runner_restart" {
					preserved = true
					break
				}
			}
		}
		if !preserved {
			if _, err = appendWorkspaceProgress(e, event); err != nil {
				return err
			}
		}
	}
	if len(ledger.Trace.Calls) > 0 {
		call := ledger.Trace.Calls[len(ledger.Trace.Calls)-1]
		if call.Terminal == nil {
			unknown := wc.CloneEvent(call.Started)
			unknown.Revision, unknown.Sequence, unknown.Phase = 2, call.Started.Sequence+1, "unknown"
			unknown.RecordedAt, unknown.CompletedAt = workspaceOperationTime(), ""
			unknown.ErrorCode = "runner_restart"
			unknown.GenerationAfter = ledger.Generation
			unknown.EventID = wc.CanonicalEventID(workspaceOperationIdentity(e.Assignment), unknown)
			found := false
			for _, p := range e.Progress {
				if p.HostOperation != nil && p.OperationID == unknown.EventID {
					found = true
					break
				}
			}
			if !found {
				if _, err = appendWorkspaceProgress(e, unknown); err != nil {
					return err
				}
			}
		}
	}
	if ledger.Trace.Truncated && !checkpoint.Truncated {
		copy := *checkpoint
		copy.Truncated = true
		e.WorkspaceTrace = &copy
	}
	return nil
}

// Frozen Config may initially contain typed bindings and later decode into
// maps. Canonical JSON keys preserve the same assignment identity across that
// ordinary persistence boundary without weakening any input comparison.
func workspaceTraceAssignmentSHA(a Assignment) (string, error) {
	raw, err := json.Marshal(a)
	if err != nil {
		return "", err
	}
	object, err := decodeToolObjectBounded(raw, 1<<20)
	if err != nil {
		return "", err
	}
	raw, err = json.Marshal(object)
	if err != nil {
		return "", err
	}
	return digest(raw), nil
}

// Host facts are an independent durable outbox. A stale SDK chunk, artifact
// transfer or final result must not strand the original workspace history.
// Failed sends remain pending and the original error is returned after other
// independent host facts have had their delivery opportunity.
func drainWorkspaceProgress(ctx context.Context, j *Journal, send func(context.Context, Progress) error) error {
	host := []Progress{}
	for _, p := range j.PendingProgress() {
		if p.HostOperation != nil {
			host = append(host, p)
		}
	}
	sort.SliceStable(host, func(i, k int) bool {
		left, right := key(host[i].Assignment), key(host[k].Assignment)
		if left != right {
			return left < right
		}
		return host[i].HostOperation.Sequence < host[k].HostOperation.Sequence
	})
	var first error
	for _, p := range host {
		if err := send(ctx, p); err != nil {
			if first == nil {
				first = err
			}
			continue
		}
		if err := j.AckProgress(p.OperationID); err != nil && first == nil {
			first = err
		}
	}
	return first
}
