package runner

import (
	"context"
	"encoding/json"
	"errors"
	wc "github.com/ai-mada/suma/internal/workspaceoperationcontract"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
		"native_workspace_trace_limit":        "Workspace operation history reached its bounded capacity.",
		"native_workspace_merge_unresolved":   "Resolve every active conflict before continuing the fixed merge.",
		"native_workspace_merge_incomplete":   "All frozen sources must be processed before sealing.",
	}
	fatal := code == "native_workspace_scope" || code == "native_workspace_closed" || code == "native_workspace_unavailable" || code == "native_workspace_operation_conflict" || code == "native_workspace_unknown" || code == "native_workspace_check_changed" || code == "native_workspace_trace_limit"
	return &NativeWorkspaceError{Code: code, Message: messages[code], Fatal: fatal}
}

type nativeWorkspace struct {
	*liveWorkspaceRead
	check           nativeWorkspaceCheck
	checkIDs        []string
	ledger          nativeWorkspaceLedger
	server          *http.Server
	bridge          NativeWorkspaceBridge
	traceProfileSHA string
	traceProgress   func(wc.Event) error
	traceJournal    *Journal
}

func openNativeWorkspace(ctx context.Context, a Assignment, cfg ExecConfig, w *attemptWorkspace, spec CommandSpec, profileSHA string, check nativeWorkspaceCheck) (*nativeWorkspace, error) {
	if spec.NativeWorkspace == nil {
		return nil, nativeError("native_workspace_scope")
	}
	reader, err := openLiveWorkspaceRead(ctx, a, cfg, w, profileSHA)
	if err != nil {
		return nil, err
	}
	n := &nativeWorkspace{liveWorkspaceRead: reader, check: check, traceProfileSHA: profileSHA, traceProgress: cfg.WorkspaceProgress, traceJournal: cfg.Journal}
	seen := map[string]bool{}
	for _, id := range spec.NativeWorkspace.Checks {
		if id == "" || seen[id] {
			_ = reader.Close()
			return nil, nativeError("native_workspace_scope")
		}
		seen[id] = true
		n.checkIDs = append(n.checkIDs, id)
	}
	sort.Strings(n.checkIDs)
	if err = n.loadLedger(); err != nil {
		_ = reader.Close()
		return nil, err
	}
	if n.traceJournal != nil {
		if err = n.traceJournal.attachWorkspaceTrace(a, n.workspace.Directory, n.policySHA, profileSHA); err != nil {
			_ = reader.Close()
			return nil, nativeError("native_workspace_unavailable")
		}
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
	if err := n.closeLocked(); err != nil {
		closeErr = err
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
	e := NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: n.policySHA, Generation: n.ledger.Generation, State: state, Files: []NativeWorkspaceFileEffect{}, Checks: []NativeWorkspaceCheckResult{}, OperationTrace: n.traceCoverageLocked()}
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
	return n.readyToSealLocked()
}
func (n *nativeWorkspace) readyToSealLocked() error {
	if n.ledger.Trace.Unavailable {
		return nativeError("native_workspace_unavailable")
	}
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

func (n *nativeWorkspace) invoke(ctx context.Context, name string, raw json.RawMessage, operationID string) (result any, callErr error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if err := n.activeLocked(ctx); err != nil {
		return nil, err
	}
	args, err := nativeArguments(name, raw)
	trace, traceErr := n.beginTraceLocked(name, raw, args, operationID)
	if traceErr != nil {
		return nil, traceErr
	}
	defer func() {
		if endErr := n.endTraceLocked(trace, result, callErr); endErr != nil {
			result, callErr = nil, endErr
		}
	}()
	if strings.HasPrefix(operationID, "remote-write:") {
		return nil, nativeError("native_workspace_operation")
	}
	if err != nil {
		return nil, err
	}
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
