package runner

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type nativeWorkspaceOperation struct {
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	RequestSHA  string             `json:"request_sha256"`
	State       string             `json:"state"`
	Path        string             `json:"path,omitempty"`
	PreviousSHA string             `json:"previous_sha256,omitempty"`
	SHA         string             `json:"sha256,omitempty"`
	Content     string             `json:"content,omitempty"`
	Generation  int                `json:"generation"`
	Result      json.RawMessage    `json:"result,omitempty"`
	RemoteWrite *nativeRemoteWrite `json:"remote_write,omitempty"`
}
type nativeWorkspaceLedger struct {
	Version    int                                   `json:"version"`
	Assignment Assignment                            `json:"assignment"`
	PolicySHA  string                                `json:"policy_sha256"`
	Generation int                                   `json:"generation"`
	Unknown    bool                                  `json:"unknown"`
	Initial    map[string]string                     `json:"initial"`
	Files      map[string]NativeWorkspaceFileEffect  `json:"files"`
	Checks     map[string]NativeWorkspaceCheckResult `json:"checks"`
	Operations map[string]*nativeWorkspaceOperation  `json:"operations"`
	Trace      nativeWorkspaceTrace                  `json:"trace"`
}

func (n *nativeWorkspace) saveLedger() error {
	if len(n.ledger.Operations) > 2048 {
		return nativeError("native_workspace_output_limit")
	}
	raw, err := json.Marshal(n.ledger)
	if err != nil || len(raw) > 32<<20 {
		return nativeError("native_workspace_unavailable")
	}
	if err = writeEvidence(filepath.Join(n.workspace.Directory, "native-workspace.json"), raw); err != nil {
		return nativeError("native_workspace_unavailable")
	}
	return nil
}
func (n *nativeWorkspace) loadLedger() error {
	name := filepath.Join(n.workspace.Directory, "native-workspace.json")
	st, err := os.Lstat(name)
	if err == nil && (!st.Mode().IsRegular() || st.Size() > 32<<20) {
		return nativeError("native_workspace_scope")
	}
	if os.IsNotExist(err) {
		initial, _, e := n.snapshotLocked()
		if e != nil {
			return e
		}
		n.ledger = nativeWorkspaceLedger{Version: 1, Assignment: n.assignment, PolicySHA: n.policySHA, Initial: initial, Files: map[string]NativeWorkspaceFileEffect{}, Checks: map[string]NativeWorkspaceCheckResult{}, Operations: map[string]*nativeWorkspaceOperation{}, Trace: nativeWorkspaceTrace{Version: 1, ProfileSHA256: n.traceProfileSHA, Calls: []nativeWorkspaceInvocation{}}}
		return n.saveLedger()
	}
	if err != nil {
		return nativeError("native_workspace_unavailable")
	}
	raw, err := os.ReadFile(name)
	if err != nil {
		return nativeError("native_workspace_unavailable")
	}
	if _, err = decodeToolObjectBounded(raw, 1<<20); err != nil {
		return nativeError("native_workspace_scope")
	}
	if json.Unmarshal(raw, &n.ledger) != nil || n.ledger.Version != 1 || n.ledger.PolicySHA != n.policySHA || n.ledger.Generation < 0 || n.ledger.Initial == nil || n.ledger.Files == nil || n.ledger.Checks == nil || n.ledger.Operations == nil {
		return nativeError("native_workspace_scope")
	}
	stored, _ := json.Marshal(n.ledger.Assignment)
	current, _ := json.Marshal(n.assignment)
	if string(stored) != string(current) {
		return nativeError("native_workspace_scope")
	}
	if err := validateNativeTrace(n.assignment, n.policySHA, n.ledger.Trace); err != nil {
		return nativeError("native_workspace_scope")
	}
	if calls := n.ledger.Trace.Calls; len(calls) > 0 && calls[len(calls)-1].Terminal == nil {
		n.ledger.Unknown = true
	}
	for id, op := range n.ledger.Operations {
		if op == nil || id != op.ID || !nativeOperationID(id) || !evidenceHash.MatchString(op.RequestSHA) || op.Generation < 0 {
			return nativeError("native_workspace_scope")
		}
		if op.RemoteWrite != nil {
			if !n.validRemoteWriteOperationLocked(op) {
				return nativeError("native_workspace_scope")
			}
			if op.State == "not_applied" {
				continue
			}
			if op.State != "completed" {
				// A remote permit is not a durable authorization to retry a write.
				// Preserve prepared metadata for inspection; never resume rename.
				n.ledger.Unknown = true
				continue
			}
		}
		if op.State == "completed" {
			if len(op.Result) == 0 || !json.Valid(op.Result) {
				return nativeError("native_workspace_scope")
			}
			continue
		}
		if op.State == "prepared" && (op.Kind == "write" || op.Kind == "edit" || op.Kind == "remove") {
			if err = n.recoverFileOperationLocked(op); err != nil {
				n.ledger.Unknown = true
				_ = n.saveLedger()
			}
			continue
		}
		if op.State == "prepared" && op.Kind == "merge_continue" {
			if err = n.recoverRepairOperationLocked(op); err != nil {
				n.ledger.Unknown = true
				_ = n.saveLedger()
			}
			continue
		}
		n.ledger.Unknown = true
	}
	if _, _, verifyErr := n.verifySourceLocked(); verifyErr != nil {
		n.ledger.Unknown = true
	}
	return n.saveLedger()
}
func nativeOperationID(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_.:-", r)) {
			return false
		}
	}
	return true
}

func (n *nativeWorkspace) operationLocked(ctx context.Context, kind string, args map[string]any, id string) (any, error) {
	return n.operationLockedWithAdmission(ctx, kind, args, id, nil, nil)
}

// The remote channel shares the native CAS/receipt implementation. Its guard
// is checked before preparing an effect and again immediately before rename.
func (n *nativeWorkspace) operationLockedWithAdmission(ctx context.Context, kind string, args map[string]any, id string, remote *nativeRemoteWrite, guard func() error) (any, error) {
	if !nativeOperationID(id) {
		return nil, nativeError("native_workspace_operation")
	}
	requestSHA := nativeRequestHash(kind, args)
	if remote != nil {
		requestSHA = remote.Command.RequestSHA256
	}
	if op, ok := n.ledger.Operations[id]; ok {
		if op.Kind != kind || op.RequestSHA != requestSHA || (op.RemoteWrite == nil) != (remote == nil) {
			return nil, nativeError("native_workspace_operation_conflict")
		}
		if op.State != "completed" {
			return nil, nativeError("native_workspace_unknown")
		}
		var result any
		if json.Unmarshal(op.Result, &result) != nil {
			return nil, nativeError("native_workspace_unavailable")
		}
		return result, nil
	}
	if n.ledger.Unknown {
		return nil, nativeError("native_workspace_unknown")
	}
	if len(n.ledger.Operations) >= 2048 {
		return nil, nativeError("native_workspace_output_limit")
	}
	if _, _, err := n.verifySourceLocked(); err != nil {
		n.ledger.Unknown = true
		_ = n.saveLedger()
		return nil, err
	}
	if kind == "check" {
		return n.runCheckLocked(ctx, args, id, requestSHA)
	}
	if guard != nil {
		if err := guard(); err != nil {
			return nil, err
		}
	}
	name := nativeString(args, "path")
	data, exists, err := nativeReadBytes(n.root, name)
	if err != nil {
		return nil, err
	}
	old := ""
	if exists {
		old = digest(data)
	}
	expected := nativeString(args, "expected_sha256")
	absent, _ := args["expected_absent"].(bool)
	if kind == "remove" && !exists || absent && exists || !absent && (!exists || old != expected) {
		return nil, nativeError("native_workspace_conflict")
	}
	op := &nativeWorkspaceOperation{ID: id, Kind: kind, RequestSHA: requestSHA, State: "prepared", Path: name, PreviousSHA: old, Generation: n.ledger.Generation + 1, RemoteWrite: remote}
	if kind == "write" {
		op.Content = nativeString(args, "content")
		op.SHA = digest([]byte(op.Content))
	}
	if guard != nil {
		if err := guard(); err != nil {
			return nil, err
		}
	}
	if kind == "edit" {
		op.Content, err = nativeApplyEdits(string(data), args["edits"].([]any))
		if err != nil {
			return nil, err
		}
		op.SHA = digest([]byte(op.Content))
	}
	n.ledger.Operations[id] = op
	if err = n.saveLedger(); err != nil {
		delete(n.ledger.Operations, id)
		return nil, err
	}
	if err = n.recoverFileOperationLockedWithAdmission(op, guard); err != nil {
		n.ledger.Unknown = true
		_ = n.saveLedger()
		return nil, err
	}
	var result any
	_ = json.Unmarshal(op.Result, &result)
	return result, nil
}

func (n *nativeWorkspace) recoverFileOperationLocked(op *nativeWorkspaceOperation) error {
	if op.RemoteWrite != nil {
		return nativeError("native_workspace_unknown")
	}
	return n.recoverFileOperationLockedWithAdmission(op, nil)
}
func (n *nativeWorkspace) recoverFileOperationLockedWithAdmission(op *nativeWorkspaceOperation, guard func() error) error {
	if _, err := nativeWorkspacePath(op.Path); err != nil {
		return err
	}
	if op.Generation != n.ledger.Generation+1 {
		return nativeError("native_workspace_unknown")
	}
	if (op.Kind == "write" || op.Kind == "edit") && (len(op.Content) > nativeWorkspaceFileLimit || digest([]byte(op.Content)) != op.SHA) {
		return nativeError("native_workspace_scope")
	}
	data, exists, err := nativeReadBytes(n.root, op.Path)
	if err != nil {
		return err
	}
	current := ""
	if exists {
		current = digest(data)
	}
	if op.Kind == "write" || op.Kind == "edit" {
		// Ordinary local prepared recovery can recognize its already-renamed
		// bytes. A fresh remote effect must perform the original guarded CAS;
		// matching external bytes alone are never proof that it was applied.
		if current != op.SHA || op.RemoteWrite != nil {
			if current != op.PreviousSHA {
				return nativeError("native_workspace_unknown")
			}
			if err = n.atomicWriteLockedWithAdmission(op.Path, []byte(op.Content), op.PreviousSHA, op.ID, guard); err != nil {
				return err
			}
		}
	} else if op.Kind == "remove" {
		if exists {
			if current != op.PreviousSHA {
				return nativeError("native_workspace_unknown")
			}
			if err = nativeValidatePath(n.root, op.Path, false); err != nil {
				return err
			}
			if err = n.root.Remove(op.Path); err != nil {
				return nativeError("native_workspace_unavailable")
			}
			if err = n.syncParentLocked(op.Path); err != nil {
				return err
			}
		}
	} else {
		return nativeError("native_workspace_scope")
	}
	effect := NativeWorkspaceFileEffect{Path: op.Path, OperationID: op.ID, PreviousSHA256: n.ledger.Initial[op.Path], SHA256: op.SHA, Removed: op.Kind == "remove", Generation: op.Generation}
	n.ledger.Files[op.Path] = effect
	n.ledger.Generation = op.Generation
	result := map[string]any{"operation_id": op.ID, "path": op.Path, "previous_sha256": op.PreviousSHA, "sha256": op.SHA, "removed": op.Kind == "remove", "generation": op.Generation, "state": "completed"}
	op.Result, _ = json.Marshal(result)
	op.State = "completed"
	op.Content = ""
	if err = n.saveLedger(); err != nil {
		op.State = "unknown"
		n.ledger.Unknown = true
		return err
	}
	return nil
}

func nativeApplyEdits(original string, edits []any) (string, error) {
	type replacement struct {
		start, end int
		text       string
	}
	parts := make([]replacement, 0, len(edits))
	for _, item := range edits {
		edit := item.(map[string]any)
		old, next := edit["old_text"].(string), edit["new_text"].(string)
		first := strings.Index(original, old)
		if first < 0 || strings.Index(original[first+1:], old) >= 0 {
			return "", nativeError("native_workspace_conflict")
		}
		parts = append(parts, replacement{first, first + len(old), next})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].start < parts[j].start })
	var b strings.Builder
	position := 0
	for _, part := range parts {
		if part.start < position {
			return "", nativeError("native_workspace_conflict")
		}
		b.WriteString(original[position:part.start])
		b.WriteString(part.text)
		position = part.end
		if b.Len() > nativeWorkspaceFileLimit {
			return "", nativeError("native_workspace_text")
		}
	}
	b.WriteString(original[position:])
	if b.Len() > nativeWorkspaceFileLimit {
		return "", nativeError("native_workspace_text")
	}
	return b.String(), nil
}

func (n *nativeWorkspace) atomicWriteLocked(name string, data []byte, expected, operation string) error {
	return n.atomicWriteLockedWithAdmission(name, data, expected, operation, nil)
}
func (n *nativeWorkspace) atomicWriteLockedWithAdmission(name string, data []byte, expected, operation string, guard func() error) error {
	parent := path.Dir(name)
	if parent != "." {
		current := ""
		for _, part := range strings.Split(parent, "/") {
			if current != "" {
				current += "/"
			}
			current += part
			if err := nativeValidatePath(n.root, current, true); err != nil {
				return err
			}
			if err := n.root.Mkdir(current, 0700); err != nil && !os.IsExist(err) {
				return nativeError("native_workspace_unavailable")
			}
			if err := nativeValidatePath(n.root, current, false); err != nil {
				return err
			}
		}
	}
	// The durable prepared operation owns this exact reserved temporary name.
	// A crash may leave an incomplete file; recovery can safely replace it
	// without inventing another path or touching a user-editable source file.
	temp := path.Join(parent, ".suma-native-"+digest([]byte(key(n.assignment)+":"+operation))+".tmp")
	if info, e := n.root.Lstat(temp); e == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nativeError("native_workspace_unknown")
		}
		if e = n.root.Remove(temp); e != nil {
			return nativeError("native_workspace_unavailable")
		}
	} else if !os.IsNotExist(e) {
		return nativeError("native_workspace_unavailable")
	}
	f, err := n.root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nativeError("native_workspace_unavailable")
	}
	defer n.root.Remove(temp)
	if info, e := n.root.Lstat(name); e == nil && info.Mode().IsRegular() {
		if err = f.Chmod(info.Mode().Perm()); err != nil {
			_ = f.Close()
			return nativeError("native_workspace_unavailable")
		}
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return nativeError("native_workspace_unavailable")
	}
	old, exists, err := nativeReadBytes(n.root, name)
	if err != nil {
		return err
	}
	actual := ""
	if exists {
		actual = digest(old)
	}
	if actual != expected {
		return nativeError("native_workspace_unknown")
	}
	if err = nativeValidatePath(n.root, name, true); err != nil {
		return err
	}
	if guard != nil {
		if err = guard(); err != nil {
			return err
		}
	}
	if err = n.root.Rename(temp, name); err != nil {
		return nativeError("native_workspace_unavailable")
	}
	// Flush the destination too; Windows directory fsync is unavailable.
	f, err = n.root.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return nativeError("native_workspace_unavailable")
	}
	err = f.Sync()
	_ = f.Close()
	if err != nil {
		return nativeError("native_workspace_unavailable")
	}
	return n.syncParentLocked(name)
}
func (n *nativeWorkspace) syncParentLocked(name string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	dir, err := n.root.Open(path.Dir(name))
	if err != nil {
		return nativeError("native_workspace_unavailable")
	}
	defer dir.Close()
	if err = dir.Sync(); err != nil {
		return nativeError("native_workspace_unavailable")
	}
	return nil
}

func (n *nativeWorkspace) runCheckLocked(ctx context.Context, args map[string]any, id, requestSHA string) (any, error) {
	checkID := nativeString(args, "check_id")
	allowed := false
	for _, candidate := range n.checkIDs {
		allowed = allowed || candidate == checkID
	}
	if !allowed || n.check == nil {
		return nil, nativeError("native_workspace_check")
	}
	_, before, err := n.verifySourceLocked()
	if err != nil {
		return nil, err
	}
	op := &nativeWorkspaceOperation{ID: id, Kind: "check", RequestSHA: requestSHA, State: "prepared", Generation: n.ledger.Generation}
	n.ledger.Operations[id] = op
	if err = n.saveLedger(); err != nil {
		delete(n.ledger.Operations, id)
		return nil, err
	}
	checkCtx, cancel := context.WithTimeout(n.ctx, 60*time.Second)
	stop := context.AfterFunc(ctx, cancel)
	result, checkErr := n.check(checkCtx, checkID)
	cancelled := checkCtx.Err() != nil
	stop()
	cancel()
	after := before
	var snapshotErr error
	if !result.OutcomeUnknown {
		_, after, snapshotErr = n.snapshotLocked()
	}
	result.OperationID = id
	result.CheckID = checkID
	result.Generation = n.ledger.Generation
	result.WorkspaceSHA256 = before
	out := newOutput(24<<10, n.secrets)
	_, _ = io.WriteString(out, result.Output)
	result.Output = out.String()
	result.OutputTruncated = result.OutputTruncated || out.truncated
	// JSON escaping can expand otherwise bounded output (for example '<').
	// Bound the serialized result before recording a completed receipt.
	for {
		raw, _ := json.Marshal(result)
		if len(raw) <= 48<<10 {
			break
		}
		if len(result.Output) == 0 {
			break
		}
		end := len(result.Output) * 3 / 4
		for end > 0 && !utf8.ValidString(result.Output[:end]) {
			end--
		}
		result.Output = result.Output[:end]
		result.OutputTruncated = true
	}
	if checkErr != nil || snapshotErr != nil || before != after || result.OutcomeUnknown || cancelled && !result.Stopped || !evidenceHash.MatchString(result.ProfileSHA256) {
		op.State = "unknown"
		n.ledger.Unknown = true
		result.OutcomeUnknown = true
		op.Result, _ = json.Marshal(result)
		_ = n.saveLedger()
		if before != after && snapshotErr == nil {
			return nil, nativeError("native_workspace_check_changed")
		}
		return nil, nativeError("native_workspace_unknown")
	}
	n.ledger.Checks[checkID] = result
	op.Result, _ = json.Marshal(result)
	op.State = "completed"
	if err = n.saveLedger(); err != nil {
		n.ledger.Unknown = true
		return nil, err
	}
	return result, nil
}
