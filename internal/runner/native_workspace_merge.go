package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func nativeMergeArguments(name string, raw []byte) (map[string]any, error) {
	if len(raw) > 2<<20 || !tc.ValidJSONUnicode(raw) {
		return nil, nativeError("native_workspace_arguments")
	}
	args, err := decodeToolObject(raw)
	if err != nil {
		return nil, nativeError("native_workspace_arguments")
	}
	if name == "merge_status" {
		if len(args) != 0 {
			return nil, nativeError("native_workspace_arguments")
		}
		return args, nil
	}
	if len(args) != 4 {
		return nil, nativeError("native_workspace_arguments")
	}
	for key, max := range map[string]int{"expected_step": 15, "expected_generation": 2048} {
		number, ok := args[key].(json.Number)
		if !ok {
			return nil, nativeError("native_workspace_arguments")
		}
		value := string(number)
		if value != "0" && !nativeDecimal(value) {
			return nil, nativeError("native_workspace_arguments")
		}
		integer, e := strconv.Atoi(value)
		if e != nil || integer < 0 || integer > max || key == "expected_step" && integer == 0 {
			return nil, nativeError("native_workspace_arguments")
		}
	}
	if !evidenceHash.MatchString(nativeString(args, "expected_state_sha256")) {
		return nil, nativeError("native_workspace_arguments")
	}
	items, ok := args["resolutions"].([]any)
	if !ok || len(items) == 0 || len(items) > 2000 {
		return nil, nativeError("native_workspace_arguments")
	}
	for _, item := range items {
		r, ok := item.(map[string]any)
		if !ok || len(r) != 2 {
			return nil, nativeError("native_workspace_arguments")
		}
		if _, err = nativeWorkspacePath(nativeString(r, "path")); err != nil {
			return nil, err
		}
		sha, hasSHA := r["expected_sha256"]
		absent, hasAbsent := r["expected_absent"]
		if hasSHA == hasAbsent || hasSHA && (nativeString(r, "expected_sha256") == "" || !evidenceHash.MatchString(fmt.Sprint(sha))) || hasAbsent && absent != true {
			return nil, nativeError("native_workspace_arguments")
		}
	}
	return args, nil
}

func repairStateDigest(s *candidateRepairState, generation int) string {
	raw, _ := json.Marshal([]any{s, generation})
	return digest(raw)
}

func (n *nativeWorkspace) mergeStatusLocked() (any, error) {
	s := n.workspace.ConflictRepair
	if s == nil || n.assignment.CandidateConflictRepair == nil {
		return nil, nativeError("native_workspace_scope")
	}
	right := ""
	if s.SourceIndex < len(n.assignment.CandidateConflictRepair.Composition.Sources) {
		right = n.assignment.CandidateConflictRepair.Composition.Sources[s.SourceIndex].HeadCommit
	}
	return map[string]any{"state": s.State, "step": s.SourceIndex, "generation": n.ledger.Generation, "state_sha256": repairStateDigest(s, n.ledger.Generation), "source_index": s.SourceIndex, "source_count": len(n.assignment.CandidateConflictRepair.Composition.Sources), "left_commit": s.CurrentCommit, "right_commit": right, "seed_tree": s.SeedTree, "conflicts": s.Conflicts, "stages": s.Stages}, nil
}

func repairWorkspaceIdentity(ctx context.Context, w *attemptWorkspace) error {
	s := w.ConflictRepair
	e := w.Assignment.CandidateConflictRepair
	if s == nil || e == nil || s.Version != 1 || s.Repository != filepath.Join(w.Directory, "repair.git") || w.Source != filepath.Join(w.Directory, "src") || w.AttemptKey != attemptKey(w.Assignment) || !within(filepath.Join(w.Repository, ".cache"), w.Directory) {
		return nativeSealFailure("repair scope changed")
	}
	for _, dir := range []string{w.Repository, w.Directory, s.Repository, w.Source} {
		actual, err := realDirectory(dir)
		if err != nil || actual != filepath.Clean(dir) {
			return nativeSealFailure("repair directory changed")
		}
	}
	raw, _ := json.Marshal(e)
	if digest(raw) != s.ExecutionSHA256 {
		return nativeSealFailure("repair execution changed")
	}
	stored, err := nativeSealReadMetadata(filepath.Join(w.Directory, "attempt.json"))
	var original attemptWorkspace
	if err != nil || json.Unmarshal(stored, &original) != nil || original.AttemptKey != w.AttemptKey || original.Repository != w.Repository || original.Source != w.Source || original.BaseCommit != w.BaseCommit || original.AdminBaseCommit != w.AdminBaseCommit {
		return nativeSealFailure("repair attempt changed")
	}
	a, _ := json.Marshal(original.Assignment)
	b, _ := json.Marshal(w.Assignment)
	if !bytes.Equal(a, b) {
		return nativeSealFailure("repair assignment changed")
	}
	common, err := compositionText(ctx, w.Source, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || filepath.Clean(common) != s.Repository {
		return nativeSealFailure("repair object repository changed")
	}
	top, err := compositionText(ctx, w.Source, "rev-parse", "--show-toplevel")
	if err != nil || filepath.Clean(top) != w.Source {
		return nativeSealFailure("repair source Git identity changed")
	}
	gitdir, err := compositionText(ctx, w.Source, "rev-parse", "--absolute-git-dir")
	if err != nil || !within(filepath.Join(s.Repository, "worktrees"), gitdir) {
		return nativeSealFailure("repair Git metadata changed")
	}
	if actual, e := realDirectory(gitdir); e != nil || actual != filepath.Clean(gitdir) {
		return nativeSealFailure("repair Git metadata traverses a link")
	}
	backlink, err := nativeSealReadMetadata(filepath.Join(gitdir, "gitdir"))
	if err != nil || filepath.Clean(strings.TrimSpace(string(backlink))) != filepath.Join(w.Source, ".git") {
		return nativeSealFailure("repair Git backlink changed")
	}
	persisted, err := nativeSealReadMetadata(filepath.Join(w.Directory, "conflict-repair.json"))
	var disk candidateRepairState
	if err != nil || json.Unmarshal(persisted, &disk) != nil {
		return nativeSealFailure("repair checkpoint is unavailable")
	}
	diskRaw, _ := json.Marshal(disk)
	currentRaw, _ := json.Marshal(s)
	if !bytes.Equal(diskRaw, currentRaw) {
		return nativeSealFailure("repair checkpoint changed")
	}
	head, err := compositionText(ctx, w.Source, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || head != s.CurrentCommit {
		return nativeSealFailure("repair prefix reference changed")
	}
	detached, err := compositionText(ctx, w.Source, "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil || detached != "HEAD" {
		return nativeSealFailure("repair reference is not detached")
	}
	return nil
}

func verifyRepairIndex(ctx context.Context, w *attemptWorkspace) error {
	raw, err := compositionGit(ctx, w.Source, 4<<20, nil, "ls-files", "--stage", "-z")
	if err != nil {
		return err
	}
	seed := *w
	seed.BaseCommit = w.ConflictRepair.SeedTree
	base, _, err := nativeSealTree(ctx, &seed)
	if err != nil {
		return err
	}
	entries := map[string][]string{}
	for name, entry := range base {
		entries[name] = []string{fmt.Sprintf("%s %s 0\t%s%c", entry.mode, entry.object, name, byte(0))}
	}
	for _, name := range w.ConflictRepair.Conflicts {
		delete(entries, name)
	}
	for _, stage := range w.ConflictRepair.Stages {
		entries[stage.Path] = append(entries[stage.Path], fmt.Sprintf("%s %s %d\t%s%c", stage.Mode, stage.Object, stage.Stage, stage.Path, byte(0)))
	}
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	var expected strings.Builder
	for _, name := range names {
		for _, entry := range entries[name] {
			expected.WriteString(entry)
		}
	}
	if !bytes.Equal(raw, []byte(expected.String())) {
		return nativeSealFailure("repair unmerged index stages changed")
	}
	return nil
}

// The seed is a tree, not an invented clean commit. Compare accepted effects
// against its raw blobs and build a fresh host-only index without filters.
func repairEditedTree(ctx context.Context, w *attemptWorkspace, evidence NativeWorkspaceEvidence) (string, error) {
	if err := repairWorkspaceIdentity(ctx, w); err != nil {
		return "", err
	}
	if err := verifyRepairIndex(ctx, w); err != nil {
		return "", err
	}
	seed := *w
	seed.BaseCommit = w.ConflictRepair.SeedTree
	base, _, err := nativeSealTree(ctx, &seed)
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(w.Source)
	if err != nil {
		return "", err
	}
	defer root.Close()
	files, paths, err := nativeSealEffects(ctx, &seed, root, evidence, base)
	if err != nil {
		return "", err
	}
	if err = nativeSealAttributes(ctx, w, files); err != nil {
		return "", err
	}
	index := filepath.Join(w.Directory, "repair-seal.index")
	if st, e := os.Lstat(index); e == nil {
		if !st.Mode().IsRegular() {
			return "", nativeSealFailure("repair index changed")
		}
		if err = os.Remove(index); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(e) {
		return "", e
	}
	if _, err = nativeSealText(ctx, w, index, "read-tree", w.ConflictRepair.SeedTree); err != nil {
		return "", err
	}
	for _, name := range paths {
		file, exists := files[name]
		if !exists {
			if _, err = nativeSealText(ctx, w, index, "update-index", "--force-remove", "--", name); err != nil {
				return "", err
			}
			continue
		}
		blob, err := nativeSealGit(ctx, w, index, file.data, 256, nil, "hash-object", "-w", "--no-filters", "--stdin")
		if err != nil {
			return "", err
		}
		object := strings.TrimSpace(string(blob))
		if object != nativeSealBlob(file.data, len(w.BaseCommit)) {
			return "", nativeSealFailure("repair content object changed")
		}
		mode := "100644"
		if prior, ok := base[name]; ok {
			mode = prior.mode
		}
		if _, err = nativeSealText(ctx, w, index, "update-index", "--add", "--cacheinfo", mode, object, name); err != nil {
			return "", err
		}
	}
	tree, err := nativeSealText(ctx, w, index, "write-tree")
	if err != nil || !commitID.MatchString(tree) {
		return "", nativeSealFailure("repair tree is unavailable")
	}
	return tree, nil
}

func (n *nativeWorkspace) mergeEvidenceLocked() NativeWorkspaceEvidence {
	e := NativeWorkspaceEvidence{SchemaVersion: 1, PolicySHA256: n.policySHA, Generation: n.ledger.Generation, State: "closed", Files: []NativeWorkspaceFileEffect{}}
	for _, effect := range n.ledger.Files {
		e.Files = append(e.Files, effect)
	}
	return e
}

func conflictMarker(data []byte) bool {
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte("<<<<<<<")) || bytes.Equal(line, []byte("=======")) || bytes.HasPrefix(line, []byte(">>>>>>>")) || bytes.HasPrefix(line, []byte("|||||||")) {
			return true
		}
	}
	return false
}

func (n *nativeWorkspace) mergeContinueLocked(ctx context.Context, args map[string]any, id string) (any, error) {
	if n.workspace.ConflictRepair == nil || !nativeOperationID(id) {
		return nil, nativeError("native_workspace_scope")
	}
	request := nativeRequestHash("merge_continue", args)
	if op, ok := n.ledger.Operations[id]; ok {
		if op.Kind != "merge_continue" || op.RequestSHA != request {
			return nil, nativeError("native_workspace_operation_conflict")
		}
		if op.State != "completed" {
			if err := n.recoverRepairOperationLocked(op); err != nil {
				return nil, err
			}
		}
		var result any
		if json.Unmarshal(op.Result, &result) != nil {
			return nil, nativeError("native_workspace_unknown")
		}
		return result, nil
	}
	s := n.workspace.ConflictRepair
	if n.ledger.Unknown || s.Pending != nil {
		return nil, nativeError("native_workspace_unknown")
	}
	if s.State != "conflicted" || nativeWorkspaceInteger(args, "expected_step", -1) != s.SourceIndex || nativeWorkspaceInteger(args, "expected_generation", -1) != n.ledger.Generation || nativeString(args, "expected_state_sha256") != repairStateDigest(s, n.ledger.Generation) {
		return nil, nativeError("native_workspace_conflict")
	}
	if _, _, err := n.verifySourceLocked(); err != nil {
		return nil, err
	}
	resolutions := args["resolutions"].([]any)
	if len(resolutions) != len(s.Conflicts) {
		return nil, nativeError("native_workspace_arguments")
	}
	resolved := map[string]bool{}
	for _, item := range resolutions {
		r := item.(map[string]any)
		name := nativeString(r, "path")
		if resolved[name] {
			return nil, nativeError("native_workspace_arguments")
		}
		resolved[name] = true
		data, exists, err := nativeReadBytes(n.root, name)
		if err != nil {
			return nil, err
		}
		absent, _ := r["expected_absent"].(bool)
		if absent && exists || !absent && (!exists || digest(data) != nativeString(r, "expected_sha256")) {
			return nil, nativeError("native_workspace_conflict")
		}
		if exists && conflictMarker(data) {
			return nil, nativeError("native_workspace_merge_unresolved")
		}
	}
	for _, name := range s.Conflicts {
		if !resolved[name] {
			return nil, nativeError("native_workspace_arguments")
		}
	}
	tree, err := repairEditedTree(ctx, n.workspace, n.mergeEvidenceLocked())
	if err != nil {
		return nil, err
	}
	op := &nativeWorkspaceOperation{ID: id, Kind: "merge_continue", RequestSHA: request, State: "prepared", Generation: n.ledger.Generation + 1}
	n.ledger.Operations[id] = op
	if err = n.saveLedger(); err != nil {
		delete(n.ledger.Operations, id)
		return nil, err
	}
	// Object writes are immutable. The complete intended reference, seed and
	// source cursor become durable before changing HEAD/index/worktree files.
	raw, _ := json.Marshal(s)
	var next candidateRepairState
	_ = json.Unmarshal(raw, &next)
	e := n.assignment.CandidateConflictRepair
	for {
		right := e.Composition.Sources[next.SourceIndex].HeadCommit
		commit, commitErr := repairCommitTree(ctx, next.Repository, tree, next.CurrentCommit, right, e.OperationID, e.FixedTimestamp, next.SourceIndex)
		if commitErr != nil {
			return n.mergeUnknownLocked(commitErr)
		}
		parents := []string{next.CurrentCommit}
		if right != next.CurrentCommit {
			parents = append(parents, right)
		}
		conflictSHA := ""
		if len(next.Stages) > 0 {
			stageRaw, _ := json.Marshal([]any{next.SeedTree, next.Stages})
			conflictSHA = digest(stageRaw)
			if next.SourceIndex == e.Conflict.ConflictSourceIndex {
				conflictSHA = e.Conflict.ConflictSHA256
			}
		}
		next.Evidence.Steps = append(next.Evidence.Steps, ccc.CandidateConflictRepairStep{SourceIndex: next.SourceIndex, LeftCommit: next.CurrentCommit, RightCommit: right, SeedTree: next.SeedTree, ConflictSHA256: conflictSHA, ResultCommit: commit, ResultTree: tree, Parents: parents})
		next.CurrentCommit = commit
		next.Evidence.MergeCommits = append(next.Evidence.MergeCommits, commit)
		next.Evidence.Parents = parents
		next.SourceIndex++
		if next.SourceIndex == len(e.Composition.Sources) {
			next.State = "resolved"
			next.SeedTree = tree
			next.Stages = nil
			next.Conflicts = nil
			break
		}
		tree, next.Stages, next.Conflicts, err = repairMergeTree(ctx, next.Repository, next.CurrentCommit, e.Composition.Sources[next.SourceIndex].HeadCommit)
		if err != nil {
			return n.mergeUnknownLocked(err)
		}
		next.SeedTree = tree
		if len(next.Stages) > 0 {
			for _, name := range next.Conflicts {
				if _, err = nativeWorkspacePath(name); err != nil {
					return n.mergeUnknownLocked(err)
				}
			}
			next.State = "conflicted"
			break
		}
	}
	next.LastOperationID = id
	next.LastRequestSHA256 = request
	next.Evidence.State = next.State
	s.Pending = &candidateRepairPending{OperationID: id, RequestSHA256: request, Next: &next}
	if err = n.workspace.saveRepair(); err != nil {
		return n.mergeUnknownLocked(err)
	}
	if _, err = compositionGit(ctx, n.workspace.Source, 8192, nil, "update-ref", "--no-deref", "HEAD", next.CurrentCommit, s.CurrentCommit); err != nil {
		return n.mergeUnknownLocked(err)
	}
	// HEAD already identifies the durable resolved prefix. A partially applied
	// checkout is uncertainty, never permission to rerun the model or continuation.
	if _, err = compositionGit(ctx, n.workspace.Source, 8192, nil, "reset", "--hard", next.CurrentCommit); err != nil {
		return n.mergeUnknownLocked(err)
	}
	n.workspace.ConflictRepair = &next
	if err = installRepairSeed(ctx, n.workspace); err != nil {
		return n.mergeUnknownLocked(err)
	}
	if err = n.workspace.saveRepair(); err != nil {
		return n.mergeUnknownLocked(err)
	}
	if err = n.workspace.save(); err != nil {
		return n.mergeUnknownLocked(err)
	}
	if err = n.completeRepairOperationLocked(op); err != nil {
		return n.mergeUnknownLocked(err)
	}
	var result any
	_ = json.Unmarshal(op.Result, &result)
	return result, nil
}

func (n *nativeWorkspace) mergeUnknownLocked(_ error) (any, error) {
	n.ledger.Unknown = true
	_ = n.saveLedger()
	return nil, nativeError("native_workspace_unknown")
}

func (n *nativeWorkspace) completeRepairOperationLocked(op *nativeWorkspaceOperation) error {
	initial, _, err := n.snapshotLocked()
	if err != nil {
		return err
	}
	n.ledger.Initial = initial
	n.ledger.Files = map[string]NativeWorkspaceFileEffect{}
	n.ledger.Checks = map[string]NativeWorkspaceCheckResult{}
	n.ledger.Generation = op.Generation
	result, err := n.mergeStatusLocked()
	if err != nil {
		return err
	}
	op.Result, _ = json.Marshal(result)
	op.State = "completed"
	return n.saveLedger()
}

// Recovery only acknowledges an already materialized exact next state. It
// never repeats an unknown checkout, chooses a new source or calls a model.
func (n *nativeWorkspace) recoverRepairOperationLocked(op *nativeWorkspaceOperation) error {
	raw, err := nativeSealReadMetadata(filepath.Join(n.workspace.Directory, "conflict-repair.json"))
	var saved candidateRepairState
	if err != nil || json.Unmarshal(raw, &saved) != nil {
		return nativeError("native_workspace_unknown")
	}
	if saved.Pending != nil {
		return nativeError("native_workspace_unknown")
	}
	if saved.LastOperationID != op.ID || saved.LastRequestSHA256 != op.RequestSHA {
		return nativeError("native_workspace_unknown")
	}
	old := n.workspace.ConflictRepair
	n.workspace.ConflictRepair = &saved
	if err = repairWorkspaceIdentity(n.ctx, n.workspace); err != nil {
		n.workspace.ConflictRepair = old
		return nativeError("native_workspace_unknown")
	}
	if err = verifyRepairIndex(n.ctx, n.workspace); err != nil {
		n.workspace.ConflictRepair = old
		return nativeError("native_workspace_unknown")
	}
	seed := *n.workspace
	seed.BaseCommit = saved.SeedTree
	base, _, err := nativeSealTree(n.ctx, &seed)
	if err != nil {
		return nativeError("native_workspace_unknown")
	}
	if _, paths, e := nativeSealEffects(n.ctx, &seed, n.root, NativeWorkspaceEvidence{Files: []NativeWorkspaceFileEffect{}}, base); e != nil || len(paths) != 0 {
		return nativeError("native_workspace_unknown")
	}
	return n.completeRepairOperationLocked(op)
}

func sealConflictRepairWorkspace(ctx context.Context, w *attemptWorkspace, evidence NativeWorkspaceEvidence) (result NativeWorkspaceSeal, sealErr error) {
	s := w.ConflictRepair
	e := w.Assignment.CandidateConflictRepair
	if s == nil || e == nil || s.State != "resolved" || s.Pending != nil || s.SourceIndex != len(e.Composition.Sources) || evidence.State != "closed" {
		return result, nativeSealFailure("all fixed sources must be resolved before sealing")
	}
	// Sealing uses the same operation and fixed time on every reconciliation.
	result = NativeWorkspaceSeal{SchemaVersion: 1, OperationID: e.OperationID, PolicySHA256: evidence.PolicySHA256, State: "prepared", PreviousCommit: s.CurrentCommit}
	defer func() {
		if sealErr != nil {
			result.State = "unknown"
		}
	}()
	tree, err := repairEditedTree(ctx, w, evidence)
	if err != nil {
		return result, err
	}
	commit := s.CurrentCommit
	changedPaths := 0
	if tree != s.SeedTree {
		names, diffErr := compositionGit(ctx, s.Repository, 4<<20, nil, "diff-tree", "--no-commit-id", "--name-only", "--no-renames", "--no-ext-diff", "--no-textconv", "-r", "-z", s.SeedTree, tree, "--")
		if diffErr != nil {
			return result, diffErr
		}
		for _, name := range bytes.Split(names, []byte{0}) {
			if len(name) > 0 {
				changedPaths++
			}
		}
		if changedPaths < 1 || changedPaths > nativeSealMaxFiles {
			return result, nativeSealFailure("repaired final changed-path count is invalid")
		}
		commit, err = repairCommitTree(ctx, s.Repository, tree, s.CurrentCommit, "", e.OperationID+"-seal", e.FixedTimestamp, len(e.Composition.Sources))
		if err != nil {
			return result, err
		}
		s.Evidence.Parents = []string{s.CurrentCommit}
	}
	for _, source := range e.Composition.Sources {
		if _, err = compositionGit(ctx, s.Repository, 8192, nil, "merge-base", "--is-ancestor", source.HeadCommit, commit); err != nil {
			return result, nativeSealFailure("a frozen source is missing from repaired lineage")
		}
	}
	s.Evidence.ResultCommit = commit
	s.Evidence.ResultTree = tree
	s.Evidence.State = "completed"
	if err = w.saveRepair(); err != nil {
		return result, err
	}
	if commit != s.CurrentCommit {
		if _, err = compositionGit(ctx, w.Source, 8192, nil, "update-ref", "--no-deref", "HEAD", commit, s.CurrentCommit); err != nil {
			return result, err
		}
	}
	// Use the already proven raw tree for the normal stage-zero index. No files
	// are overwritten at this final boundary.
	if _, err = compositionGit(ctx, w.Source, 8192, nil, "read-tree", tree); err != nil {
		return result, err
	}
	status, err := compositionText(ctx, w.Source, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil || status != "" {
		return result, nativeSealFailure("repaired candidate worktree is not clean")
	}
	result.Commit = commit
	result.Tree = tree
	result.State = "no_change"
	if commit != s.CurrentCommit {
		result.State = "sealed"
	}
	result.ChangedPaths = changedPaths
	return result, nil
}

// Stable ordering is part of the genuine Git index representation.
func sortRepairStages(stages []ccc.CandidateConflictStage) {
	sort.Slice(stages, func(i, j int) bool {
		if stages[i].Path == stages[j].Path {
			return stages[i].Stage < stages[j].Stage
		}
		return stages[i].Path < stages[j].Path
	})
}
