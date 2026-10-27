package runner

import (
	"context"
	"os"
	"path/filepath"
	"sync"
)

// liveWorkspaceRead owns only an already opened current attempt root. Native
// writes, operation ledgers and MCP bridges belong to nativeWorkspace instead.
type liveWorkspaceRead struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	workspace  *attemptWorkspace
	root       *os.Root
	identity   os.FileInfo
	policySHA  string
	profileSHA string
	assignment Assignment
	secrets    []string
	closed     bool
}

func openLiveWorkspaceRead(ctx context.Context, a Assignment, cfg ExecConfig, w *attemptWorkspace, profileSHA string) (*liveWorkspaceRead, error) {
	if w == nil || !evidenceHash.MatchString(profileSHA) || a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 || key(a) != key(w.Assignment) || w.AttemptKey != attemptKey(a) {
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
		_ = root.Close()
		return nil, nativeError("native_workspace_unavailable")
	}
	runCtx, cancel := context.WithCancel(ctx)
	return &liveWorkspaceRead{ctx: runCtx, cancel: cancel, workspace: w, root: root, identity: identity, policySHA: profileSHA, profileSHA: profileSHA, assignment: a, secrets: append([]string{}, cfg.Secrets...)}, nil
}

func (r *liveWorkspaceRead) activeLocked(ctx context.Context) error {
	if r.closed || r.ctx.Err() != nil || ctx.Err() != nil {
		return nativeError("native_workspace_closed")
	}
	if key(r.assignment) != key(r.workspace.Assignment) || r.workspace.AttemptKey != attemptKey(r.assignment) {
		return nativeError("native_workspace_scope")
	}
	st, err := os.Stat(r.workspace.Source)
	if err != nil || !os.SameFile(r.identity, st) {
		return nativeError("native_workspace_scope")
	}
	return nil
}

func (r *liveWorkspaceRead) Close() error {
	r.cancel()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeLocked()
}

func (r *liveWorkspaceRead) closeLocked() error {
	if r.closed {
		return nil
	}
	r.closed = true
	if r.root != nil {
		err := r.root.Close()
		r.root = nil
		if err != nil {
			return nativeError("native_workspace_unavailable")
		}
	}
	return nil
}
