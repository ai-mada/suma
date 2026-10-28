package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	wc "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

// Invocation traces are private call receipts, distinct from file-effect
// receipts. Replaying a completed write creates a new invocation referring to
// that original effect; it never changes or reexecutes the effect receipt.
type nativeWorkspaceInvocation struct {
	Started  wc.Event  `json:"started"`
	Terminal *wc.Event `json:"terminal,omitempty"`
	Replayed bool      `json:"replayed"`
}
type nativeWorkspaceTrace struct {
	Version       int                         `json:"version"`
	ProfileSHA256 string                      `json:"profile_sha256"`
	Sequence      int                         `json:"sequence"`
	Truncated     bool                        `json:"truncated"`
	Unavailable   bool                        `json:"unavailable"`
	Calls         []nativeWorkspaceInvocation `json:"calls"`
}

func workspaceOperationIdentity(a Assignment) wc.Identity {
	return wc.Identity{TenantID: a.TenantID, ProjectID: a.ProjectID, RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence}
}
func workspaceOperationTime() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func validateNativeTrace(a Assignment, policy string, trace nativeWorkspaceTrace) error {
	identity := workspaceOperationIdentity(a)
	if wc.ValidateIdentity(identity) != nil || trace.Version != 1 || !evidenceHash.MatchString(trace.ProfileSHA256) || trace.Calls == nil || len(trace.Calls) > wc.MaxCalls || trace.Sequence < 0 || trace.Sequence > wc.MaxRecords {
		return fmt.Errorf("workspace invocation trace identity is invalid")
	}
	sequence := 0
	seen := map[string]bool{}
	for index, call := range trace.Calls {
		e := call.Started
		sequence++
		if wc.ValidateBoundEvent(identity, e) != nil || e.Phase != "started" || e.Sequence != sequence || e.PolicySHA256 != policy || e.ProfileSHA256 != trace.ProfileSHA256 || seen[e.OperationID] {
			return fmt.Errorf("workspace invocation start is invalid")
		}
		seen[e.OperationID] = true
		if call.Terminal == nil {
			if index != len(trace.Calls)-1 {
				return fmt.Errorf("workspace invocation lacks its original terminal")
			}
			continue
		}
		terminal := *call.Terminal
		sequence++
		if wc.ValidateBoundEvent(identity, terminal) != nil || terminal.Phase == "started" || terminal.Sequence != sequence || terminal.OperationID != e.OperationID || terminal.Action != e.Action || terminal.RequestSHA256 != e.RequestSHA256 || terminal.PolicySHA256 != policy || terminal.ProfileSHA256 != e.ProfileSHA256 || terminal.StartedAt != e.StartedAt || terminal.GenerationBefore != e.GenerationBefore || terminal.Path != e.Path || terminal.CheckID != e.CheckID {
			return fmt.Errorf("workspace invocation terminal is invalid")
		}
	}
	if sequence != trace.Sequence {
		return fmt.Errorf("workspace invocation sequence is invalid")
	}
	return nil
}

func nativeTraceAction(action string) bool {
	switch action {
	case "list", "read", "write", "edit", "remove", "check", "merge_status", "merge_continue", "seal":
		return true
	}
	return false
}

// Both phase slots are admitted before an effect. This bounded history is not
// charged against the SDK log limit, and reaching its limit is never silent.
func (n *nativeWorkspace) beginTraceLocked(action string, raw json.RawMessage, args map[string]any, effectID string) (*nativeWorkspaceInvocation, error) {
	if !nativeTraceAction(action) {
		return nil, nativeError("native_workspace_arguments")
	}
	if wc.ValidateIdentity(workspaceOperationIdentity(n.assignment)) != nil {
		return nil, nativeError("native_workspace_scope")
	}
	trace := &n.ledger.Trace
	if trace.Version != 1 || trace.ProfileSHA256 != n.traceProfileSHA {
		return nil, nativeError("native_workspace_scope")
	}
	if trace.Truncated || len(trace.Calls) >= wc.MaxCalls || trace.Sequence+2 > wc.MaxRecords {
		trace.Truncated = true
		if n.saveLedger() != nil {
			return nil, nativeError("native_workspace_unavailable")
		}
		if n.traceJournal != nil {
			_ = n.traceJournal.noteWorkspaceTraceCoverage(n.assignment, true)
		}
		return nil, nativeError("native_workspace_trace_limit")
	}
	if len(trace.Calls) > 0 && trace.Calls[len(trace.Calls)-1].Terminal == nil {
		return nil, nativeError("native_workspace_unknown")
	}
	stamp := workspaceOperationTime()
	requestSHA := digest(raw)
	if args != nil {
		requestSHA = nativeRequestHash(action, args)
	}
	e := wc.Event{Protocol: wc.Protocol, Source: wc.Source, SchemaVersion: 1, Sequence: trace.Sequence + 1, OperationID: "workspace-" + operationID(), Revision: 1, Action: action, Phase: "started", GenerationBefore: n.ledger.Generation, GenerationAfter: n.ledger.Generation, StartedAt: stamp, RecordedAt: stamp, RequestSHA256: requestSHA, PolicySHA256: n.policySHA, ProfileSHA256: n.traceProfileSHA}
	if args != nil {
		if path, err := nativeWorkspacePath(nativeString(args, "path")); err == nil {
			e.Path = path
		}
		if action == "list" {
			if path, err := nativeWorkspacePath(nativeString(args, "directory")); err == nil {
				e.Path = path
			}
		}
		if action == "check" {
			for _, id := range n.checkIDs {
				if id == nativeString(args, "check_id") {
					e.CheckID = id
				}
			}
		}
	}
	if nativeOperationID(effectID) && (action == "write" || action == "edit" || action == "remove" || action == "check" || action == "merge_continue" || action == "seal") {
		e.EffectOperationID = effectID
	}
	e.EventID = wc.CanonicalEventID(workspaceOperationIdentity(n.assignment), e)
	if wc.ValidateBoundEvent(workspaceOperationIdentity(n.assignment), e) != nil {
		return nil, nativeError("native_workspace_scope")
	}
	replayed := false
	if op := n.ledger.Operations[effectID]; op != nil && op.Kind == action && op.RequestSHA == requestSHA && op.State == "completed" {
		replayed = true
	}
	trace.Calls = append(trace.Calls, nativeWorkspaceInvocation{Started: e, Replayed: replayed})
	trace.Sequence = e.Sequence
	call := &trace.Calls[len(trace.Calls)-1]
	if n.saveLedger() != nil {
		trace.Calls = trace.Calls[:len(trace.Calls)-1]
		trace.Sequence--
		return nil, nativeError("native_workspace_unavailable")
	}
	if err := n.publishTrace(e); err != nil {
		// No effect has been admitted. Preserve this failed call in the private
		// ledger even when the parent outbox is currently unavailable.
		_ = n.endTraceLocked(call, nil, nativeError("native_workspace_unavailable"))
		return nil, nativeError("native_workspace_unavailable")
	}
	return call, nil
}

func (n *nativeWorkspace) publishTrace(event wc.Event) error {
	if n.traceProgress != nil {
		return n.traceProgress(wc.CloneEvent(event))
	}
	if n.traceJournal != nil {
		return n.traceJournal.RecordWorkspaceProgress(n.assignment, event)
	}
	return nil
}

func (n *nativeWorkspace) endTraceLocked(call *nativeWorkspaceInvocation, result any, callErr error) error {
	if call == nil || call.Terminal != nil {
		return nativeError("native_workspace_scope")
	}
	e := wc.CloneEvent(call.Started)
	e.Revision, e.Sequence, e.Phase = 2, n.ledger.Trace.Sequence+1, "completed"
	e.GenerationAfter = n.ledger.Generation
	e.RecordedAt = workspaceOperationTime()
	e.CompletedAt = e.RecordedAt
	if result != nil {
		raw, err := json.Marshal(result)
		if err != nil {
			return nativeError("native_workspace_unavailable")
		}
		// Struct and original-receipt map encodings describe the same result.
		// Canonicalize keys so replays retain its semantic summary digest.
		object, err := decodeToolObjectBounded(raw, 1<<18)
		if err != nil {
			return nativeError("native_workspace_unavailable")
		}
		raw, err = json.Marshal(object)
		if err != nil {
			return nativeError("native_workspace_unavailable")
		}
		e.ResultSHA256 = digest(raw)
	}
	if callErr != nil {
		public := nativePublicError(callErr)
		e.Phase, e.ErrorCode = "failed", public.Code
		if n.ledger.Unknown || public.Code == "native_workspace_unknown" {
			e.Phase, e.CompletedAt = "unknown", ""
		}
	}
	if e.Action == "check" {
		var check NativeWorkspaceCheckResult
		found := false
		if typed, ok := result.(NativeWorkspaceCheckResult); ok {
			check, found = typed, true
		} else if op := n.ledger.Operations[e.EffectOperationID]; op != nil && len(op.Result) > 0 {
			found = json.Unmarshal(op.Result, &check) == nil
		}
		if found {
			e.ExitCode, e.Stopped = &check.ExitCode, &check.Stopped
			if check.OutcomeUnknown {
				e.Phase, e.ErrorCode, e.CompletedAt = "unknown", "native_workspace_unknown", ""
			} else if callErr == nil && check.Stopped {
				e.Phase, e.ErrorCode = "failed", "native_workspace_check_stopped"
			} else if callErr == nil && check.ExitCode != 0 {
				e.Phase, e.ErrorCode = "failed", "native_workspace_check_failed"
			}
		}
	}
	if seal, ok := result.(NativeWorkspaceSeal); ok && seal.OperationID != "" {
		e.EffectOperationID = seal.OperationID
		if seal.State == "unknown" || seal.State == "prepared" {
			e.Phase, e.ErrorCode, e.CompletedAt = "unknown", "native_workspace_unknown", ""
		}
	}
	// Replayed means a completed original effect receipt was consumed by this
	// invocation, not that a new write/check happened. begin marks the fact.
	e.Replayed = call.Replayed && e.Phase != "unknown"
	e.EventID = wc.CanonicalEventID(workspaceOperationIdentity(n.assignment), e)
	if wc.ValidateBoundEvent(workspaceOperationIdentity(n.assignment), e) != nil {
		return nativeError("native_workspace_scope")
	}
	call.Terminal = &e
	n.ledger.Trace.Sequence = e.Sequence
	if n.saveLedger() != nil {
		call.Terminal = nil
		n.ledger.Trace.Sequence--
		n.ledger.Unknown = true
		return nativeError("native_workspace_unavailable")
	}
	if n.publishTrace(e) != nil {
		n.ledger.Trace.Unavailable = true
		_ = n.saveLedger()
		return nativeError("native_workspace_unavailable")
	}
	return nil
}

// Host sealing is outside MCP and remains Go-only. It is traced after the
// child capability has closed, using the original seal checkpoint identity.
func (n *nativeWorkspace) seal(ctx context.Context, evidence NativeWorkspaceEvidence) (result NativeWorkspaceSeal, sealErr error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	raw, _ := json.Marshal(evidence)
	identity, _ := json.Marshal([]any{candidateRef(n.assignment), evidence.PolicySHA256, n.workspace.BaseCommit})
	effect := "native-seal-" + digest(identity)
	if n.assignment.CandidateConflictRepair != nil {
		effect = n.assignment.CandidateConflictRepair.OperationID
	}
	call, err := n.beginTraceLocked("seal", raw, nil, effect)
	if err != nil {
		return result, err
	}
	if sealErr = n.readyToSealLocked(); sealErr == nil {
		result, sealErr = sealNativeWorkspace(ctx, n.workspace, evidence)
	}
	if err = n.endTraceLocked(call, result, sealErr); err != nil {
		return result, err
	}
	return result, sealErr
}

// Verify a private trace without reopening its workspace or resuming effects.
// Recovery only imports bounded, safe original facts and marks incomplete
// invocations unknown. This method never calls a tool, check or Git operation.
func traceEvents(a Assignment, policy string, trace nativeWorkspaceTrace) ([]wc.Event, error) {
	if err := validateNativeTrace(a, policy, trace); err != nil {
		return nil, err
	}
	out := make([]wc.Event, 0, len(trace.Calls)*2)
	for _, call := range trace.Calls {
		out = append(out, wc.CloneEvent(call.Started))
		if call.Terminal != nil {
			out = append(out, wc.CloneEvent(*call.Terminal))
		}
	}
	return out, nil
}

func sameWorkspaceTraceAssignment(a, b Assignment) bool {
	return key(a) == key(b) && workspaceOperationIdentity(a) == workspaceOperationIdentity(b) && a.Type == b.Type && reflect.DeepEqual(a.Command, b.Command)
}

func nativeTraceLedgerPath(directory string) string {
	return filepath.Join(directory, "native-workspace.json")
}

func (n *nativeWorkspace) traceCoverageLocked() *wc.Coverage {
	trace := n.ledger.Trace
	coverage := &wc.Coverage{InvocationCount: len(trace.Calls), RecordCount: trace.Sequence, MaxInvocations: wc.MaxCalls, MaxRecords: wc.MaxRecords, State: "complete"}
	for _, call := range trace.Calls {
		if call.Terminal == nil {
			coverage.PendingCount++
		}
		if call.Terminal != nil && call.Terminal.Phase == "unknown" {
			coverage.State, coverage.Reason = "unknown", "operation_unknown"
		}
	}
	if trace.Truncated {
		coverage.State, coverage.Reason = "partial", "capture_limit"
	}
	if n.ledger.Unknown {
		coverage.State, coverage.Reason = "unknown", "operation_unknown"
	}
	// A parent outbox publication failure is tracked privately for sealing.
	// Coverage remains about local capture, not transport acknowledgement.
	return coverage
}
func (n *nativeWorkspace) traceCoverage() *wc.Coverage {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.traceCoverageLocked()
}
