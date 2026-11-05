package runner

import (
	"context"
	"encoding/json"
	"fmt"
	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	term "github.com/ai-mada/suma/internal/terminalcontract"
	"github.com/ai-mada/suma/internal/terminalruntime"
)

type terminalCandidateFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}
type terminalCandidateInventoryRecord struct {
	SchemaVersion   int                         `json:"schema_version"`
	AttemptKey      string                      `json:"attempt_key"`
	BaseCommit      string                      `json:"base_commit"`
	AdminBaseCommit string                      `json:"admin_base_commit"`
	CodeBaseline    *ccc.CodeBaselineBinding    `json:"code_baseline,omitempty"`
	Files           []NativeWorkspaceFileEffect `json:"files"`
	Inventory       []terminalCandidateFile     `json:"inventory"`
	SHA256          string                      `json:"sha256"`
}

func terminalCandidateInventoryDigest(value terminalCandidateInventoryRecord) string {
	value.SHA256 = ""
	raw, _ := json.Marshal(value)
	return digest(raw)
}

// stoppedWorkspaceInventory uses the parent seal's rooted reads and protected
// paths. The caller must first prove that its original writer has stopped.
func stoppedWorkspaceInventory(ctx context.Context, w *attemptWorkspace) (terminalCandidateInventoryRecord, map[string]nativeSealFile, error) {
	out := terminalCandidateInventoryRecord{SchemaVersion: 1, AttemptKey: w.AttemptKey, BaseCommit: w.BaseCommit, AdminBaseCommit: w.AdminBaseCommit, CodeBaseline: cloneTerminalCodeBaseline(w.Assignment.CodeBaseline), Files: []NativeWorkspaceFileEffect{}, Inventory: []terminalCandidateFile{}}
	if _, err := nativeSealIdentity(ctx, w); err != nil {
		return out, nil, err
	}
	head, err := nativeSealText(ctx, w, "", "rev-parse", "HEAD")
	if err != nil || head != w.BaseCommit || !terminalWorkspaceBasis(w.Assignment, w.BaseCommit, w.AdminBaseCommit) {
		return out, nil, fmt.Errorf("Stopped workspace HEAD differs from its approved base")
	}
	base, _, err := nativeSealTree(ctx, w)
	if err != nil {
		return out, nil, err
	}
	root, err := os.OpenRoot(w.Source)
	if err != nil {
		return out, nil, err
	}
	defer root.Close()
	files, err := nativeSealSnapshot(root)
	if err != nil {
		return out, nil, err
	}
	// Protected tracked files must remain unchanged; they are never copied.
	for name, old := range base {
		file, exists, e := nativeSealReadFile(root, name)
		if e != nil {
			return out, nil, e
		}
		if _, e = nativeWorkspacePath(name); e != nil {
			if !exists || nativeSealBlob(file.data, len(w.BaseCommit)) != old.object {
				return out, nil, fmt.Errorf("Stopped workspace changed a protected tracked file")
			}
			continue
		}
		if exists {
			files[name] = file
		}
	}
	names := make([]string, 0, len(files)+len(base))
	seen := map[string]bool{}
	for name := range files {
		names = append(names, name)
		seen[name] = true
	}
	for name := range base {
		if _, e := nativeWorkspacePath(name); e == nil && !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	total := 0
	for _, name := range names {
		file, exists := files[name]
		old, tracked := base[name]
		if exists {
			total += len(file.data)
			if len(file.data) > 1<<20 || total > nativeSealMaxBytes || len(files) > nativeSealMaxFiles {
				return out, nil, fmt.Errorf("Stopped workspace exceeds its file or byte limit")
			}
			mode := uint32(file.mode.Perm())
			if runtime.GOOS == "windows" {
				mode = 0600
			}
			out.Inventory = append(out.Inventory, terminalCandidateFile{Path: name, SHA256: digest(file.data), Mode: mode})
			if tracked && runtime.GOOS != "windows" && (file.mode.Perm()&0111 != 0) != (old.mode == "100755") {
				return out, nil, fmt.Errorf("Stopped workspace changed executable modes")
			}
		}
		if tracked && exists && nativeSealBlob(file.data, len(w.BaseCommit)) == old.object {
			continue
		}
		if !tracked && !exists {
			continue
		}
		effect := NativeWorkspaceFileEffect{Path: name, OperationID: "terminal-copy-" + digest([]byte(name)), Generation: len(out.Files) + 1}
		if tracked {
			previous, e := nativeSealGit(ctx, w, "", nil, nativeSealMaxBytes, nil, "cat-file", "blob", old.object)
			if e != nil {
				return out, nil, e
			}
			effect.PreviousSHA256 = digest(previous)
		}
		if exists {
			effect.SHA256 = digest(file.data)
		} else {
			effect.Removed = true
		}
		out.Files = append(out.Files, effect)
	}
	out.SHA256 = terminalCandidateInventoryDigest(out)
	return out, files, nil
}

func terminalCandidateNativeEvidence(inventory terminalCandidateInventoryRecord, policy string) NativeWorkspaceEvidence {
	return NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: policy, State: "closed", Generation: len(inventory.Files), Files: inventory.Files, Checks: []NativeWorkspaceCheckResult{}}
}

func terminalCandidateStopped(w *attemptWorkspace, sessionID string) error {
	journal := filepath.Join(w.Directory, "terminal-journal")
	real, err := realDirectory(journal)
	if err != nil || real != journal {
		return fmt.Errorf("Original terminal journal is unavailable or linked")
	}
	inspection, err := terminalruntime.InspectJournal(journal)
	if err != nil || inspection.Session.ID != sessionID || inspection.Session.WorkDir != w.Source || !inspection.Session.TreeStopped || inspection.Session.State != "stopped" && inspection.Session.State != "exited" && inspection.Session.State != "reconciling" {
		return fmt.Errorf("Original terminal process tree is not durably stopped")
	}
	return nil
}

// Called only after the terminal runtime confirms the entire original tree.
// A later call may recover the same receipt, but cannot silently accept drift.
func captureTerminalCandidateSource(ctx context.Context, a Assignment, cfg ExecConfig, w *attemptWorkspace, sessionID string) (string, error) {
	if a.Type != "terminal_session" || w == nil || w.Assignment.DispatchID != a.DispatchID || w.Assignment.RunID != a.RunID || w.AdminBaseCommit != cfg.BaseCommit || !sameTerminalCodeBaseline(w.Assignment.CodeBaseline, a.CodeBaseline) || !terminalWorkspaceBasis(a, w.BaseCommit, w.AdminBaseCommit) {
		return "", fmt.Errorf("Original terminal source identity changed")
	}
	if err := terminalCandidateStopped(w, sessionID); err != nil {
		return "", err
	}
	inventory, _, err := stoppedWorkspaceInventory(ctx, w)
	if err != nil {
		return "", err
	}
	second, _, err := stoppedWorkspaceInventory(ctx, w)
	if err != nil || inventory.SHA256 != second.SHA256 {
		return "", fmt.Errorf("Original terminal source changed during stopped capture")
	}
	path := filepath.Join(w.Directory, "terminal-candidate-source.json")
	if raw, e := nativeSealReadMetadata(path); e == nil {
		var original terminalCandidateInventoryRecord
		if json.Unmarshal(raw, &original) != nil || original.SHA256 != inventory.SHA256 {
			return "", fmt.Errorf("Original stopped terminal snapshot changed")
		}
		return inventory.SHA256, nil
	} else if !os.IsNotExist(e) {
		return "", e
	}
	raw, _ := json.Marshal(inventory)
	if err = writeEvidence(path, raw); err != nil {
		return "", err
	}
	return inventory.SHA256, nil
}

// Recover only the already durable capture. A restart must not inventory a new
// state and call it the original stopped source. Returning this old fact grants
// no code access: candidate import still checks the actual bytes against it.
func recoverTerminalCandidateSnapshot(a Assignment, journalDir, sourceDir string) (snapshotSHA, baseCommit string, err error) {
	directory := filepath.Dir(journalDir)
	manifest := filepath.Join(directory, "terminal-candidate-source.json")
	raw, err := nativeSealReadMetadata(manifest)
	if os.IsNotExist(err) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	if a.Type != "terminal_session" || a.Attempt != 1 || a.Fence < 1 || a.DispatchID == "" || journalDir != filepath.Join(directory, "terminal-journal") || sourceDir != filepath.Join(directory, "src") {
		return "", "", fmt.Errorf("Original stopped terminal snapshot paths or assignment changed")
	}
	for _, name := range []string{directory, journalDir, sourceDir} {
		real, e := realDirectory(name)
		if e != nil || real != name {
			return "", "", fmt.Errorf("Original stopped terminal snapshot directory changed")
		}
	}
	var inventory terminalCandidateInventoryRecord
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&inventory) != nil || inventory.SchemaVersion != 1 || !evidenceHash.MatchString(inventory.SHA256) || terminalCandidateInventoryDigest(inventory) != inventory.SHA256 || inventory.AttemptKey != attemptKey(a) || !terminalWorkspaceBasis(a, inventory.BaseCommit, inventory.AdminBaseCommit) || !sameTerminalCodeBaseline(inventory.CodeBaseline, a.CodeBaseline) || len(inventory.Files) > nativeSealMaxFiles || len(inventory.Inventory) > nativeSealMaxFiles {
		return "", "", fmt.Errorf("Original stopped terminal snapshot receipt is invalid")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return "", "", fmt.Errorf("Original stopped terminal snapshot receipt has trailing data")
	}
	attemptRaw, e := nativeSealReadMetadata(filepath.Join(directory, "attempt.json"))
	var original attemptWorkspace
	if e != nil || json.Unmarshal(attemptRaw, &original) != nil || original.AttemptKey != attemptKey(a) || original.Directory != directory || original.Source != sourceDir || original.BaseCommit != inventory.BaseCommit || original.AdminBaseCommit != inventory.AdminBaseCommit || !sameTerminalCodeBaseline(original.Assignment.CodeBaseline, inventory.CodeBaseline) || original.Assignment.Type != "terminal_session" || original.Assignment.DispatchID != a.DispatchID || candidateRef(original.Assignment) != candidateRef(a) {
		return "", "", fmt.Errorf("Original stopped terminal snapshot attempt changed")
	}
	originalSession, _ := original.Assignment.Config["terminal_session_id"].(string)
	requestedSession, _ := a.Config["terminal_session_id"].(string)
	if originalSession == "" || originalSession != requestedSession {
		return "", "", fmt.Errorf("Original stopped terminal snapshot session changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, e = nativeSealIdentity(ctx, &original); e != nil {
		return "", "", e
	}
	return inventory.SHA256, inventory.BaseCommit, nil
}

func applyTerminalCandidateSource(ctx context.Context, a Assignment, cfg ExecConfig, spec CommandSpec, w *attemptWorkspace) error {
	raw, present := a.Config[term.CandidateConfigKey]
	if !present {
		return nil
	}
	source, err := term.DecodeCandidateSource(raw)
	if err != nil || source == nil || w == nil || a.Type != "command" || a.Attempt != 1 || a.Idempotent || len(a.Command) != 1 || a.WorkerID != source.Original.WorkerID || spec.Adapter != "command" || spec.Protocol != "text-v1" || spec.ReadOnly || spec.TerminalSession || spec.TestEvidence || spec.TestReport || spec.NativeWorkspace != nil || spec.ArtifactInput || spec.ArtifactOutput != "" || source.AdminBaseCommit != cfg.BaseCommit || w.AdminBaseCommit != source.AdminBaseCommit || w.BaseCommit != source.BaseCommit || !sameTerminalCodeBaseline(a.CodeBaseline, source.CodeBaseline) || !terminalWorkspaceBasis(a, w.BaseCommit, w.AdminBaseCommit) || hasCandidate(a) || a.CandidateComposition != nil || a.CandidateConflictRepair != nil {
		return fmt.Errorf("Terminal candidate requires its protected original source and fixed writable command profile")
	}
	repo, root, err := workspaceConfig(ctx, cfg)
	if err != nil {
		return err
	}
	var originalAssignment Assignment
	encoded, _ := json.Marshal(source.Original)
	_ = json.Unmarshal(encoded, &originalAssignment)
	key := attemptKey(originalAssignment)
	directory := filepath.Join(root, key)
	rawAttempt, err := nativeSealReadMetadata(filepath.Join(directory, "attempt.json"))
	var original attemptWorkspace
	if err != nil || json.Unmarshal(rawAttempt, &original) != nil || original.AttemptKey != key || original.Directory != directory || original.Source != filepath.Join(directory, "src") || original.Repository != repo || original.BaseCommit != source.BaseCommit || original.AdminBaseCommit != source.AdminBaseCommit || !sameTerminalCodeBaseline(original.Assignment.CodeBaseline, source.CodeBaseline) || original.Assignment.Type != "terminal_session" || candidateRef(original.Assignment) != source.Original {
		return fmt.Errorf("Original stopped terminal attempt identity is unavailable")
	}
	if err = terminalCandidateStopped(&original, source.SessionID); err != nil {
		return err
	}
	stored, err := nativeSealReadMetadata(filepath.Join(directory, "terminal-candidate-source.json"))
	var fixed terminalCandidateInventoryRecord
	if err != nil || json.Unmarshal(stored, &fixed) != nil || fixed.SHA256 != source.SnapshotSHA256 || terminalCandidateInventoryDigest(fixed) != source.SnapshotSHA256 || fixed.BaseCommit != source.BaseCommit || fixed.AdminBaseCommit != source.AdminBaseCommit || !sameTerminalCodeBaseline(fixed.CodeBaseline, source.CodeBaseline) {
		return fmt.Errorf("Stopped terminal snapshot receipt is unavailable")
	}
	inventory, files, err := stoppedWorkspaceInventory(ctx, &original)
	if err != nil || inventory.SHA256 != source.SnapshotSHA256 || len(inventory.Files) == 0 {
		return fmt.Errorf("Stopped terminal source changed or contains no permitted code changes")
	}
	for _, effect := range inventory.Files {
		name := filepath.Join(w.Source, filepath.FromSlash(effect.Path))
		if !within(w.Source, name) {
			return fmt.Errorf("Stopped source path escaped the candidate")
		}
		if effect.Removed {
			if err = os.Remove(name); err != nil {
				return err
			}
			continue
		}
		if err = safeMkdir(filepath.Dir(name)); err != nil {
			return err
		}
		file := files[effect.Path]
		if err = os.WriteFile(name, file.data, file.mode.Perm()); err != nil {
			return err
		}
	}
	after, _, err := stoppedWorkspaceInventory(ctx, &original)
	if err != nil || after.SHA256 != inventory.SHA256 {
		return fmt.Errorf("Original terminal source changed while copying")
	}
	rawInventory, _ := json.Marshal(inventory)
	return writeEvidence(filepath.Join(w.Directory, "terminal-candidate-import.json"), rawInventory)
}

func sealTerminalCandidate(ctx context.Context, a Assignment, w *attemptWorkspace, r *Result) {
	raw, present := a.Config[term.CandidateConfigKey]
	if !present || w == nil || r.ExitCode != 0 || r.Error != "" || r.Stopped || r.OutcomeUnknown {
		return
	}
	fail := func(err error) { r.ExitCode = -1; r.Error = "Terminal candidate seal: " + err.Error() }
	source, err := term.DecodeCandidateSource(raw)
	if err != nil || source == nil {
		fail(fmt.Errorf("original source is invalid"))
		return
	}
	data, err := nativeSealReadMetadata(filepath.Join(w.Directory, "terminal-candidate-import.json"))
	var inventory terminalCandidateInventoryRecord
	if err != nil || json.Unmarshal(data, &inventory) != nil || inventory.SHA256 != source.SnapshotSHA256 || terminalCandidateInventoryDigest(inventory) != source.SnapshotSHA256 || inventory.BaseCommit != source.BaseCommit || inventory.AdminBaseCommit != source.AdminBaseCommit || !sameTerminalCodeBaseline(inventory.CodeBaseline, source.CodeBaseline) || w.BaseCommit != source.BaseCommit || w.AdminBaseCommit != source.AdminBaseCommit || !sameTerminalCodeBaseline(a.CodeBaseline, source.CodeBaseline) {
		fail(fmt.Errorf("original captured files are unavailable"))
		return
	}
	policy, _ := a.Config["tool_profile_sha256"].(string)
	evidence := terminalCandidateNativeEvidence(inventory, policy)
	seal, err := sealNativeWorkspace(ctx, w, evidence)
	if err != nil || seal.State != "sealed" {
		fail(fmt.Errorf("fixed check changed code or parent seal failed: %v", err))
		return
	}
	proof := term.TerminalCandidateEvidence{SchemaVersion: 1, Source: *source, SnapshotSHA256: inventory.SHA256, FileCount: len(inventory.Files), Seal: seal}
	encoded, _ := json.Marshal(proof)
	if err = writeEvidence(filepath.Join(w.Directory, "terminal-candidate.json"), encoded); err != nil {
		fail(err)
	}
}

func appendTerminalCandidateEvidence(w *attemptWorkspace, evidence map[string]any) error {
	if _, present := w.Assignment.Config[term.CandidateConfigKey]; !present {
		return nil
	}
	raw, err := nativeSealReadMetadata(filepath.Join(w.Directory, "terminal-candidate.json"))
	if os.IsNotExist(err) && (w.ExitCode != 0 || w.OutcomeUnknown) {
		return nil
	}
	var proof term.TerminalCandidateEvidence
	if err != nil || json.Unmarshal(raw, &proof) != nil || proof.SchemaVersion != 1 || !proof.Source.Valid() || !strings.EqualFold(proof.SnapshotSHA256, proof.Source.SnapshotSHA256) || proof.Seal.Commit != w.HeadCommit {
		return fmt.Errorf("Original terminal candidate seal evidence is unavailable")
	}
	evidence["terminal_candidate"] = proof
	return nil
}
