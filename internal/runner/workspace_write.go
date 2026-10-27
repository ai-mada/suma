package runner

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wwc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

// This metadata belongs to the existing native operation ledger. It contains
// no source body and binds a native write or terminal barrier to one command.
type nativeRemoteWrite struct {
	Command   wwc.Command `json:"command"`
	ErrorCode string      `json:"error_code,omitempty"`
}
type remoteWriteGuardError struct{ code string }

func (e *remoteWriteGuardError) Error() string { return e.code }

func sameRemoteWriteCommand(a, b wwc.Command) bool {
	return a.ID == b.ID && a.RequestSHA256 == b.RequestSHA256 && a.ExecutionFence == b.ExecutionFence && a.ExecuteClaimID == b.ExecuteClaimID && a.ExecuteBefore == b.ExecuteBefore
}
func remoteWriteMetadata(command wwc.Command) *nativeRemoteWrite {
	command.Result = nil
	command.ErrorCode = ""
	command.InspectionID = ""
	command.InspectionState = ""
	return &nativeRemoteWrite{Command: command}
}

func validateWorkspaceWriteLease(lease wwc.Lease, project string, scope wwc.Scope, policy string, receiptOnly ...bool) error {
	c := lease.Command
	if wwc.ValidateCommand(c) != nil || c.ProjectID != project || c.Scope != scope || c.WorkspacePolicySHA256 != policy || c.ExecutionFence != 1 || !nativeOperationID(wwc.NativeOperationID(c.ID)) || !nativeOperationID(lease.ClaimID) {
		return nativeError("native_workspace_scope")
	}
	if _, err := time.Parse(time.RFC3339Nano, lease.LeaseUntil); err != nil {
		return err
	}
	switch lease.Mode {
	case "execute":
		if lease.ClaimID != c.ExecuteClaimID || lease.Content == nil && (len(receiptOnly) == 0 || !receiptOnly[0]) {
			return nativeError("native_workspace_arguments")
		}
		if _, err := time.Parse(time.RFC3339Nano, c.ExecuteBefore); err != nil {
			return err
		}
		if lease.Content != nil {
			req := wwc.CreateRequest{ProjectID: c.ProjectID, Scope: c.Scope, WorkspacePolicySHA256: c.WorkspacePolicySHA256, Path: c.Path, ExpectedSHA256: c.ExpectedSHA256, Content: *lease.Content, ContentSHA256: c.ContentSHA256}
			return wwc.ValidateCreate(req)
		}
	case "inspect":
		if lease.ClaimID != c.InspectionID || lease.Content != nil {
			return nativeError("native_workspace_arguments")
		}
	default:
		return nativeError("native_workspace_arguments")
	}
	return nil
}

func (n *nativeWorkspace) validRemoteWriteOperationLocked(op *nativeWorkspaceOperation) bool {
	if op.RemoteWrite == nil {
		return false
	}
	c := op.RemoteWrite.Command
	if wwc.ValidateCommand(c) != nil || c.Scope != nativeObservationScope(n.assignment, n.profileSHA) || c.WorkspacePolicySHA256 != n.policySHA || c.ExecutionFence != 1 || op.ID != wwc.NativeOperationID(c.ID) || op.Kind != "write" || op.RequestSHA != c.RequestSHA256 || op.Path != c.Path || op.PreviousSHA != c.ExpectedSHA256 || op.SHA != c.ContentSHA256 {
		return false
	}
	if op.State == "not_applied" {
		return op.RemoteWrite.ErrorCode != "" && op.RemoteWrite.ErrorCode != "operation_unknown" && wwc.ValidError(op.RemoteWrite.ErrorCode)
	}
	if op.RemoteWrite.ErrorCode != "" {
		return false
	}
	if op.State == "completed" {
		return remoteNativeWriteEffect(op, c) != nil
	}
	return op.State == "prepared" || op.State == "unknown"
}
func remoteNativeWriteEffect(op *nativeWorkspaceOperation, c wwc.Command) *wwc.Effect {
	if op == nil || op.State != "completed" || op.RemoteWrite == nil || !sameRemoteWriteCommand(op.RemoteWrite.Command, c) {
		return nil
	}
	var receipt struct {
		OperationID string `json:"operation_id"`
		Path        string `json:"path"`
		PreviousSHA string `json:"previous_sha256"`
		SHA         string `json:"sha256"`
		Removed     bool   `json:"removed"`
		Generation  int    `json:"generation"`
		State       string `json:"state"`
	}
	if wwc.Decode(op.Result, &receipt) != nil || receipt.OperationID != wwc.NativeOperationID(c.ID) || receipt.Path != c.Path || receipt.PreviousSHA != c.ExpectedSHA256 || receipt.SHA != c.ContentSHA256 || receipt.Removed || receipt.Generation < 1 || receipt.Generation != op.Generation || receipt.State != "completed" {
		return nil
	}
	return &wwc.Effect{NativeOperationID: receipt.OperationID, Path: receipt.Path, PreviousSHA256: receipt.PreviousSHA, SHA256: receipt.SHA, Generation: receipt.Generation}
}

// A no-effect answer requires a durable barrier under the same mutex used by
// model writes and sealing. A late execute must find this terminal operation.
func (n *nativeWorkspace) remoteWriteBarrierLocked(c wwc.Command, code string) bool {
	id := wwc.NativeOperationID(c.ID)
	if _, exists := n.ledger.Operations[id]; exists || len(n.ledger.Operations) >= 2048 {
		return false
	}
	metadata := remoteWriteMetadata(c)
	metadata.ErrorCode = code
	op := &nativeWorkspaceOperation{ID: id, Kind: "write", RequestSHA: c.RequestSHA256, State: "not_applied", Path: c.Path, PreviousSHA: c.ExpectedSHA256, SHA: c.ContentSHA256, Generation: n.ledger.Generation, RemoteWrite: metadata}
	n.ledger.Operations[id] = op
	if err := n.saveLedger(); err != nil {
		delete(n.ledger.Operations, id)
		return false
	}
	return true
}

// ExecuteWorkspaceWrite is deliberately separate from the read-only Observe
// API. Inspect never receives content and can only return an original receipt
// or establish a no-effect barrier while this exact host is still live.
func ExecuteWorkspaceWrite(ctx context.Context, registry *WorkspaceObservationRegistry, lease wwc.Lease) wwc.ResultRequest {
	return executeWorkspaceWrite(ctx, registry, lease, true)
}
func executeWorkspaceWrite(ctx context.Context, registry *WorkspaceObservationRegistry, lease wwc.Lease, allowApply bool) wwc.ResultRequest {
	c := lease.Command
	result := wwc.ResultRequest{ProjectID: c.ProjectID, WorkerID: c.Scope.WorkerID, CommandID: c.ID, Scope: c.Scope, WorkspacePolicySHA256: c.WorkspacePolicySHA256, RequestSHA256: c.RequestSHA256, ExecutionFence: c.ExecutionFence, ClaimID: lease.ClaimID, Mode: lease.Mode, OperationID: operationID()}
	finish := func(outcome, code string, effect *wwc.Effect) wwc.ResultRequest {
		result.Outcome, result.ErrorCode, result.Effect = outcome, code, effect
		result.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
		result.ResultSHA256 = wwc.ResultDigest(outcome, effect, code)
		return result
	}
	unknown := func() wwc.ResultRequest { return finish("unknown", "operation_unknown", nil) }
	if registry == nil {
		return unknown()
	}
	registry.mu.Lock()
	native, scope, project := registry.native, registry.scope, registry.projectID
	registry.mu.Unlock()
	if native == nil || validateWorkspaceWriteLease(lease, project, scope, native.policySHA, !allowApply) != nil {
		return unknown()
	}
	native.mu.Lock()
	defer native.mu.Unlock()
	id := wwc.NativeOperationID(c.ID)
	if op := native.ledger.Operations[id]; op != nil {
		if !native.validRemoteWriteOperationLocked(op) || !sameRemoteWriteCommand(op.RemoteWrite.Command, c) {
			return unknown()
		}
		if effect := remoteNativeWriteEffect(op, c); effect != nil {
			return finish("applied", "", effect)
		}
		if op.State == "not_applied" {
			return finish("not_applied", op.RemoteWrite.ErrorCode, nil)
		}
		return unknown()
	}
	if !allowApply {
		return unknown()
	}
	// A missing registry/closed handle is not proof that an earlier writer did
	// not run. Do not reopen a historical path or infer effects from file bytes.
	if native.activeLocked(context.Background()) != nil {
		return unknown()
	}
	until, _ := time.Parse(time.RFC3339Nano, lease.LeaseUntil)
	if lease.Mode == "inspect" {
		if ctx.Err() != nil || !until.After(time.Now()) {
			return unknown()
		}
		if native.remoteWriteBarrierLocked(c, "not_started") {
			return finish("not_applied", "not_started", nil)
		}
		return unknown()
	}
	before, _ := time.Parse(time.RFC3339Nano, c.ExecuteBefore)
	if until.Before(before) {
		before = until
	}
	guard := func() error {
		if native.activeLocked(ctx) != nil {
			return &remoteWriteGuardError{code: "workspace_closed"}
		}
		if !before.After(time.Now()) {
			return &remoteWriteGuardError{code: "permit_expired"}
		}
		return nil
	}
	noEffect := func(code string) wwc.ResultRequest {
		if native.remoteWriteBarrierLocked(c, code) {
			return finish("not_applied", code, nil)
		}
		return unknown()
	}
	if err := guard(); err != nil {
		return noEffect(err.(*remoteWriteGuardError).code)
	}
	argsRaw, _ := json.Marshal(map[string]any{"path": c.Path, "content": *lease.Content, "expected_sha256": c.ExpectedSHA256})
	args, err := nativeArguments("write", argsRaw)
	if err != nil {
		return noEffect(writeErrorCode(err))
	}
	previous, exists, err := nativeReadBytes(native.root, c.Path)
	if err != nil {
		return noEffect(writeErrorCode(err))
	}
	if !exists {
		return noEffect("file_missing")
	}
	if len(previous) > wwc.MaxContentBytes {
		return noEffect("text_required")
	}
	if native.observationSensitiveLocked(&woc.Result{Read: &woc.Read{Path: c.Path, Content: string(previous)}}) || native.observationSensitiveLocked(&woc.Result{Read: &woc.Read{Path: c.Path, Content: *lease.Content}}) {
		return noEffect("sensitive_content")
	}
	_, err = native.operationLockedWithAdmission(ctx, "write", args, id, remoteWriteMetadata(c), guard)
	if err != nil {
		if native.ledger.Operations[id] != nil || native.ledger.Unknown {
			return unknown()
		}
		var gate *remoteWriteGuardError
		if errors.As(err, &gate) {
			return noEffect(gate.code)
		}
		code := writeErrorCode(err)
		if code == "workspace_unavailable" || code == "operation_unknown" {
			return unknown()
		}
		return noEffect(code)
	}
	if effect := remoteNativeWriteEffect(native.ledger.Operations[id], c); effect != nil {
		return finish("applied", "", effect)
	}
	return unknown()
}
func writeErrorCode(err error) string {
	switch nativePublicError(err).Code {
	case "native_workspace_conflict":
		return "file_changed"
	case "native_workspace_missing":
		return "file_missing"
	case "native_workspace_path":
		return "path_not_allowed"
	case "native_workspace_text":
		return "text_required"
	case "native_workspace_output_limit":
		return "output_limit"
	case "native_workspace_arguments":
		return "request_invalid"
	case "native_workspace_operation_conflict":
		return "operation_conflict"
	case "native_workspace_closed", "native_workspace_scope":
		return "workspace_closed"
	case "native_workspace_unknown":
		return "operation_unknown"
	default:
		return "workspace_unavailable"
	}
}
