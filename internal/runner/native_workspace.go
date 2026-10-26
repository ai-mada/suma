package runner

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"
)

const nativeWorkspaceFileLimit = 1 << 20
const nativeWorkspaceResultLimit = 64 << 10

// NativeWorkspaceError exposes a closed diagnostic, never filesystem paths,
// user arguments, provider responses or credentials.
type NativeWorkspaceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fatal   bool   `json:"fatal"`
}

func (e *NativeWorkspaceError) Error() string { return e.Message }
func nativeError(code string) error {
	messages := map[string]string{
		"native_workspace_arguments":          "Workspace tool arguments are invalid.",
		"native_workspace_scope":              "Workspace execution scope is invalid.",
		"native_workspace_closed":             "Workspace execution scope has ended.",
		"native_workspace_unavailable":        "Workspace storage is unavailable.",
		"native_workspace_path":               "Workspace path is not allowed.",
		"native_workspace_missing":            "Workspace file does not exist.",
		"native_workspace_text":               "Workspace file must be bounded UTF-8 text.",
		"native_workspace_output_limit":       "Workspace result exceeds the output limit.",
		"native_workspace_conflict":           "Workspace file has changed. Read it again before editing.",
		"native_workspace_operation":          "Workspace operation identity is invalid.",
		"native_workspace_operation_conflict": "Workspace operation identity was used for another request.",
		"native_workspace_unknown":            "Workspace effects require reconciliation.",
		"native_workspace_check":              "Workspace check is not allowed.",
		"native_workspace_check_changed":      "Workspace check changed source files.",
		"native_workspace_checks_stale":       "Required workspace checks have not passed for current source.",
		"native_workspace_cursor":             "Workspace list cursor is invalid or stale.",
		"native_workspace_merge_unresolved":   "Resolve every active conflict before continuing the fixed merge.",
		"native_workspace_merge_incomplete":   "All frozen sources must be processed before sealing.",
	}
	fatal := code == "native_workspace_scope" || code == "native_workspace_closed" || code == "native_workspace_unavailable" || code == "native_workspace_operation_conflict" || code == "native_workspace_unknown" || code == "native_workspace_check_changed"
	return &NativeWorkspaceError{Code: code, Message: messages[code], Fatal: fatal}
}

type nativeWorkspace struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	workspace  *attemptWorkspace
	root       *os.Root
	identity   os.FileInfo
	policySHA  string
	assignment Assignment
	check      nativeWorkspaceCheck
	checkIDs   []string
	secrets    []string
	ledger     nativeWorkspaceLedger
	closed     bool
	server     *http.Server
	bridge     NativeWorkspaceBridge
}

func openNativeWorkspace(ctx context.Context, a Assignment, cfg ExecConfig, w *attemptWorkspace, spec CommandSpec, profileSHA string, check nativeWorkspaceCheck) (*nativeWorkspace, error) {
	if w == nil || spec.NativeWorkspace == nil || !evidenceHash.MatchString(profileSHA) || a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 || key(a) != key(w.Assignment) || w.AttemptKey != attemptKey(a) {
		return nil, nativeError("native_workspace_scope")
	}
	if ctx.Err() != nil {
		return nil, nativeError("native_workspace_closed")
	}
	source, err := realDirectory(w.Source)
	if err != nil || source != filepath.Clean(w.Source) || !within(w.Directory, source) || source == filepath.Clean(w.Directory) {
		return nil, nativeError("native_workspace_scope")
	}
	root, err := os.OpenRoot(source)
	if err != nil {
		return nil, nativeError("native_workspace_unavailable")
	}
	identity, err := root.Stat(".")
	if err != nil {
		root.Close()
		return nil, nativeError("native_workspace_unavailable")
	}
	runCtx, cancel := context.WithCancel(ctx)
	n := &nativeWorkspace{ctx: runCtx, cancel: cancel, workspace: w, root: root, identity: identity, policySHA: profileSHA, assignment: a, check: check, secrets: append([]string{}, cfg.Secrets...)}
	seen := map[string]bool{}
	for _, id := range spec.NativeWorkspace.Checks {
		if id == "" || seen[id] {
			root.Close()
			cancel()
			return nil, nativeError("native_workspace_scope")
		}
		seen[id] = true
		n.checkIDs = append(n.checkIDs, id)
	}
	sort.Strings(n.checkIDs)
	if err = n.loadLedger(); err != nil {
		root.Close()
		cancel()
		return nil, err
	}
	return n, nil
}

// Close first revokes the capability and cancels checks, then drains HTTP calls.
// The filesystem handle is closed only once the serialized call has returned.
func (n *nativeWorkspace) Close() error {
	n.cancel()
	n.mu.Lock()
	server := n.server
	n.mu.Unlock()
	var closeErr error
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		closeErr = server.Shutdown(ctx)
		cancel()
		if closeErr != nil {
			_ = server.Close()
		}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nil
	}
	n.closed = true
	if n.root != nil {
		if err := n.root.Close(); err != nil {
			closeErr = err
		}
		n.root = nil
	}
	if closeErr != nil {
		return nativeError("native_workspace_unavailable")
	}
	return nil
}

func (n *nativeWorkspace) Evidence() NativeWorkspaceEvidence {
	n.mu.Lock()
	defer n.mu.Unlock()
	state := "open"
	if n.closed {
		state = "closed"
	}
	if n.ledger.Unknown {
		state = "unknown"
	}
	e := NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: n.policySHA, Generation: n.ledger.Generation, State: state, Files: []NativeWorkspaceFileEffect{}, Checks: []NativeWorkspaceCheckResult{}}
	for _, effect := range n.ledger.Files {
		e.Files = append(e.Files, effect)
	}
	for _, check := range n.ledger.Checks {
		e.Checks = append(e.Checks, check)
	}
	sort.Slice(e.Files, func(i, j int) bool { return e.Files[i].Path < e.Files[j].Path })
	sort.Slice(e.Checks, func(i, j int) bool { return e.Checks[i].CheckID < e.Checks[j].CheckID })
	return e
}

func (n *nativeWorkspace) ReadyToSeal() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.ledger.Unknown {
		return nativeError("native_workspace_unknown")
	}
	if repair := n.workspace.ConflictRepair; repair != nil && (repair.State != "resolved" || repair.Pending != nil || repair.SourceIndex != len(n.assignment.CandidateConflictRepair.Composition.Sources)) {
		return nativeError("native_workspace_merge_incomplete")
	}
	_, sourceSHA, err := n.verifySourceLocked()
	if err != nil {
		return err
	}
	for _, id := range n.checkIDs {
		check, ok := n.ledger.Checks[id]
		if !ok || check.Generation != n.ledger.Generation || check.WorkspaceSHA256 != sourceSHA || check.ExitCode != 0 || check.Stopped || check.OutcomeUnknown {
			return nativeError("native_workspace_checks_stale")
		}
	}
	return nil
}

func (n *nativeWorkspace) activeLocked(ctx context.Context) error {
	if n.closed || n.ctx.Err() != nil || ctx.Err() != nil {
		return nativeError("native_workspace_closed")
	}
	st, err := os.Stat(n.workspace.Source)
	if err != nil || !os.SameFile(n.identity, st) {
		return nativeError("native_workspace_scope")
	}
	return nil
}

func (n *nativeWorkspace) invoke(ctx context.Context, name string, raw json.RawMessage, operationID string) (any, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := n.activeLocked(ctx); err != nil {
		return nil, err
	}
	args, err := nativeArguments(name, raw)
	if err != nil {
		return nil, err
	}
	var result any
	switch name {
	case "list":
		result, err = n.listLocked(args)
	case "read":
		result, err = n.readLocked(args)
	case "write", "edit", "remove", "check":
		result, err = n.operationLocked(ctx, name, args, operationID)
	case "merge_status":
		result, err = n.mergeStatusLocked()
	case "merge_continue":
		result, err = n.mergeContinueLocked(ctx, args, operationID)
	default:
		err = nativeError("native_workspace_arguments")
	}
	if err != nil {
		return nil, err
	}
	b, err := json.Marshal(result)
	if err != nil {
		return nil, nativeError("native_workspace_unavailable")
	}
	if len(b) > nativeWorkspaceResultLimit {
		return nil, nativeError("native_workspace_output_limit")
	}
	return result, nil
}

func nativePublicError(err error) *NativeWorkspaceError {
	var result *NativeWorkspaceError
	if errors.As(err, &result) {
		return result
	}
	return nativeError("native_workspace_unavailable").(*NativeWorkspaceError)
}

func nativeString(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return value
}
func nativeWorkspaceInteger(args map[string]any, key string, fallback int) int {
	if v, ok := args[key].(json.Number); ok {
		value, _ := strconv.Atoi(string(v))
		return value
	}
	return fallback
}

// Receipts canonicalize decoded, strictly validated JSON; model-controlled
// presentation and transport request IDs never become operation authority.
func nativeRequestHash(name string, args map[string]any) string {
	b, _ := json.Marshal(struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}{name, args})
	return digest(b)
}
