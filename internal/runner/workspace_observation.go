package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

// A registry belongs to one executing Worker assignment (one Pool slot). It
// retains the existing live host handle, never a path which could be reopened.
type WorkspaceObservationRegistry struct {
	mu        sync.Mutex
	projectID string
	native    *nativeWorkspace
	scope     woc.Scope
}

func NewWorkspaceObservationRegistry(project string) *WorkspaceObservationRegistry {
	return &WorkspaceObservationRegistry{projectID: project}
}
func nativeObservationScope(a Assignment, profileSHA string) woc.Scope {
	profileID, _ := a.Config["tool_profile_id"].(string)
	return woc.Scope{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ToolProfileID: profileID, ProfileSHA256: profileSHA}
}
func (r *WorkspaceObservationRegistry) register(native *nativeWorkspace) (func(), error) {
	if r == nil || native == nil || r.projectID == "" {
		return nil, fmt.Errorf("workspace observation registry is unavailable")
	}
	native.mu.Lock()
	scope := nativeObservationScope(native.assignment, native.profileSHA)
	activeErr := native.activeLocked(context.Background())
	native.mu.Unlock()
	if activeErr != nil || woc.ValidateScope(scope) != nil {
		return nil, fmt.Errorf("workspace observation requires an exact active native profile")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.native != nil {
		return nil, fmt.Errorf("workspace observation registry is already active")
	}
	r.native, r.scope = native, scope
	return func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.native == native {
			r.native = nil
			r.scope = woc.Scope{}
		}
	}, nil
}
func (r *WorkspaceObservationRegistry) active() (woc.Scope, string, bool) {
	if r == nil {
		return woc.Scope{}, "", false
	}
	r.mu.Lock()
	native, scope := r.native, r.scope
	r.mu.Unlock()
	if native == nil {
		return woc.Scope{}, "", false
	}
	native.mu.Lock()
	defer native.mu.Unlock()
	if native.activeLocked(context.Background()) != nil {
		return woc.Scope{}, "", false
	}
	return scope, native.policySHA, true
}

func validObservationLease(lease woc.Lease, project string, scope woc.Scope, policy string, now time.Time) error {
	observation := lease.Observation
	expires, expiresErr := time.Parse(time.RFC3339Nano, observation.ExpiresAt)
	until, untilErr := time.Parse(time.RFC3339Nano, lease.LeaseUntil)
	if observation.ProjectID != project || observation.Scope != scope || observation.WorkspacePolicySHA256 != policy || observation.State != "leased" || observation.ID == "" || len(observation.ID) > 150 || strings.ContainsAny(observation.ID, "/\\\x00\r\n") || lease.LeaseFence < 1 || woc.ValidateScope(scope) != nil || woc.ValidateOperation(observation.Operation, observation.Arguments, observation.ExpectedSHA256) != nil || !woc.ValidDigest(policy) || expiresErr != nil || untilErr != nil || !expires.After(now) || !until.After(now) || until.After(expires) {
		return fmt.Errorf("workspace observation lease is invalid or expired")
	}
	return nil
}

// Observe can issue only the two native read primitives. It never dispatches
// model instructions, writes, checks, merge operations or arbitrary MCP calls.
func (r *WorkspaceObservationRegistry) Observe(ctx context.Context, lease woc.Lease) woc.ResultRequest {
	observation := lease.Observation
	receipt := woc.ResultRequest{ProjectID: observation.ProjectID, WorkerID: observation.Scope.WorkerID, RequestID: observation.ID, Scope: observation.Scope, LeaseFence: lease.LeaseFence, WorkspacePolicySHA256: observation.WorkspacePolicySHA256, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano), OperationID: operationID()}
	finish := func(result *woc.Result, code string) woc.ResultRequest {
		receipt.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
		receipt.ObservationResult, receipt.ErrorCode = result, code
		receipt.ResultSHA256 = woc.ResultDigest(result, code)
		return receipt
	}
	if r == nil || ctx.Err() != nil {
		return finish(nil, "workspace_closed")
	}
	r.mu.Lock()
	native, scope, project := r.native, r.scope, r.projectID
	r.mu.Unlock()
	if native == nil {
		return finish(nil, "workspace_closed")
	}
	if validObservationLease(lease, project, scope, native.policySHA, time.Now()) != nil {
		return finish(nil, "request_invalid")
	}
	until, _ := time.Parse(time.RFC3339Nano, lease.LeaseUntil)
	readCtx, cancel := context.WithDeadline(ctx, until)
	defer cancel()
	native.mu.Lock()
	defer native.mu.Unlock()
	if native.activeLocked(readCtx) != nil {
		return finish(nil, "workspace_closed")
	}
	nativeArgs := observation.Arguments
	nativeArgs.FullFile = false // This is broker metadata, not a native/MCP parameter.
	raw, _ := json.Marshal(nativeArgs)
	args, err := nativeArguments(observation.Operation, raw)
	if err != nil {
		return finish(nil, observationErrorCode(err))
	}
	if observation.Arguments.FullFile {
		args["max_lines"] = json.Number("1048577")
	}
	var observed any
	switch observation.Operation {
	case "list":
		observed, err = native.listLocked(args)
	case "read":
		observed, err = native.readLocked(args)
	default:
		return finish(nil, "request_invalid")
	}
	if err != nil {
		return finish(nil, observationErrorCode(err))
	}
	if native.activeLocked(readCtx) != nil {
		return finish(nil, "workspace_closed")
	}
	result := &woc.Result{}
	encoded, err := json.Marshal(observed)
	if err != nil || len(encoded) > woc.MaxResultBytes {
		return finish(nil, "output_limit")
	}
	if observation.Operation == "list" {
		result.List = &woc.List{}
		err = json.Unmarshal(encoded, result.List)
	} else {
		result.Read = &woc.Read{}
		err = json.Unmarshal(encoded, result.Read)
	}
	if err != nil {
		return finish(nil, "workspace_unavailable")
	}
	if observation.Arguments.FullFile && result.Read != nil && (!result.Read.Complete || result.Read.Bytes > 48<<10) {
		return finish(nil, "output_limit")
	}
	if result.Read != nil && observation.ExpectedSHA256 != "" && result.Read.SHA256 != observation.ExpectedSHA256 {
		return finish(nil, "file_changed")
	}
	if native.observationSensitiveLocked(result) {
		return finish(nil, "sensitive_content")
	}
	if woc.ValidateResult(observation, result, "") != nil {
		return finish(nil, "output_limit")
	}
	if readCtx.Err() != nil {
		return finish(nil, "workspace_closed")
	}
	return finish(result, "")
}

func (n *nativeWorkspace) observationSensitiveLocked(result *woc.Result) bool {
	var text string
	if result.Read != nil {
		text = result.Read.Content + "\n" + result.Read.Path
	} else if result.List != nil {
		for _, entry := range result.List.Entries {
			text += entry.Path + "\n"
		}
	}
	if credential.MatchString(text) {
		return true
	}
	secrets := append(append([]string{}, n.secrets...), n.bridge.Token, n.workspace.Source, n.workspace.Directory, n.workspace.Repository)
	for _, secret := range secrets {
		if secret != "" && strings.Contains(text, secret) {
			return true
		}
	}
	return false
}
func observationErrorCode(err error) string {
	switch nativePublicError(err).Code {
	case "native_workspace_closed", "native_workspace_scope":
		return "workspace_closed"
	case "native_workspace_path":
		return "path_not_allowed"
	case "native_workspace_missing":
		return "file_missing"
	case "native_workspace_text":
		return "text_required"
	case "native_workspace_output_limit":
		return "output_limit"
	case "native_workspace_cursor":
		return "cursor_stale"
	case "native_workspace_arguments":
		return "request_invalid"
	default:
		return "workspace_unavailable"
	}
}
