package runner

import (
	"bytes"
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const nativeSealMaxFiles = 2000
const nativeSealMaxBytes = 64 << 20

// This receipt is outside Source. A prepared operation fixes the author/time
// before any object or ref write; retries never manufacture another candidate.
type nativeSealCheckpoint struct {
	SchemaVersion int                 `json:"schema_version"`
	OperationID   string              `json:"operation_id"`
	AssignmentSHA string              `json:"assignment_sha256"`
	RequestSHA    string              `json:"request_sha256"`
	Repository    string              `json:"repository"`
	Source        string              `json:"source"`
	GitDirectory  string              `json:"git_directory"`
	PreviousTree  string              `json:"previous_tree"`
	AuthorDate    string              `json:"author_date"`
	State         string              `json:"state"`
	Result        NativeWorkspaceSeal `json:"result"`
}

type nativeSealEntry struct {
	mode, object string
}
type nativeSealFile struct {
	data []byte
	mode os.FileMode
}

func nativeSealFailure(reason string) error {
	return fmt.Errorf("native workspace seal requires reconciliation: %s", reason)
}

// Git receives only generated arguments and a minimal environment. In
// particular no host GIT_DIR/index/config overrides, external diff, hooks,
// fsmonitor, credentials or user identity can enter this local plumbing path.
func nativeSealGit(ctx context.Context, w *attemptWorkspace, index string, input []byte, limit int, extraEnv []string, args ...string) ([]byte, error) {
	options := []string{"-c", "core.hooksPath=" + os.DevNull, "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.autocrlf=false", "-c", "core.eol=lf", "-c", "core.safecrlf=false", "-c", "core.attributesFile=" + os.DevNull, "-c", "core.quotePath=false", "-c", "core.longpaths=true", "-c", "status.renames=false", "-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "commit.gpgsign=false", "-c", "protocol.allow=never"}
	cmd := exec.CommandContext(ctx, nativeGitExecutable(ctx), append(options, args...)...)
	cmd.Dir = w.Source
	environment, _, _ := commandEnvironment(CommandSpec{})
	cmd.Env = append(environment, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1")
	if index != "" {
		cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+index)
	}
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Stdin = bytes.NewReader(input)
	stdout, stderr := newOutput(limit, nil), newOutput(8192, nil)
	cmd.Stdout, cmd.Stderr = stdout, stderr
	if err := cmd.Run(); err != nil {
		return nil, nativeSealFailure("local Git " + args[0] + " operation failed")
	}
	if stdout.truncated || stderr.truncated {
		return nil, nativeSealFailure("Git evidence exceeds its limit")
	}
	return []byte(stdout.b.String()), nil
}

func nativeSealText(ctx context.Context, w *attemptWorkspace, index string, args ...string) (string, error) {
	b, err := nativeSealGit(ctx, w, index, nil, 4<<20, nil, args...)
	return strings.TrimSpace(string(b)), err
}

func nativeSealIdentity(ctx context.Context, w *attemptWorkspace) (string, error) {
	if w == nil || !attemptID.MatchString(w.AttemptKey) || attemptKey(w.Assignment) != w.AttemptKey || !commitID.MatchString(w.BaseCommit) || !commitID.MatchString(w.AdminBaseCommit) || w.Source != filepath.Join(w.Directory, "src") || !within(filepath.Join(w.Repository, ".cache"), w.Directory) {
		return "", nativeSealFailure("attempt identity changed")
	}
	for _, directory := range []string{w.Repository, w.Directory, w.Source} {
		resolved, err := realDirectory(directory)
		if err != nil || resolved != filepath.Clean(directory) {
			return "", nativeSealFailure("attempt directory changed")
		}
	}
	raw, err := nativeSealReadMetadata(filepath.Join(w.Directory, "attempt.json"))
	var saved attemptWorkspace
	if err != nil || json.Unmarshal(raw, &saved) != nil || saved.AttemptKey != w.AttemptKey || saved.Repository != w.Repository || saved.Directory != w.Directory || saved.Source != w.Source || saved.BaseCommit != w.BaseCommit || saved.AdminBaseCommit != w.AdminBaseCommit {
		return "", nativeSealFailure("durable attempt identity changed")
	}
	a, _ := json.Marshal(saved.Assignment)
	b, _ := json.Marshal(w.Assignment)
	if !bytes.Equal(a, b) {
		return "", nativeSealFailure("assignment identity changed")
	}
	top, err := nativeSealText(ctx, w, "", "rev-parse", "--show-toplevel")
	if err != nil || filepath.Clean(top) != w.Source {
		return "", nativeSealFailure("source Git identity changed")
	}
	detached, err := nativeSealText(ctx, w, "", "rev-parse", "--symbolic-full-name", "HEAD")
	if err != nil || detached != "HEAD" {
		return "", nativeSealFailure("attempt HEAD is not detached")
	}
	common, err := nativeSealText(ctx, w, "", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	repo := *w
	repo.Source = w.Repository
	expected, err := nativeSealText(ctx, &repo, "", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || filepath.Clean(common) != filepath.Clean(expected) {
		return "", nativeSealFailure("source repository changed")
	}
	gitDirectory, err := nativeSealText(ctx, w, "", "rev-parse", "--absolute-git-dir")
	if err != nil || !within(filepath.Join(common, "worktrees"), gitDirectory) {
		return "", nativeSealFailure("attempt Git metadata changed")
	}
	for _, directory := range []string{common, gitDirectory} {
		resolved, err := realDirectory(directory)
		if err != nil || resolved != filepath.Clean(directory) {
			return "", nativeSealFailure("attempt Git metadata traverses a link")
		}
	}
	backlink, err := nativeSealReadMetadata(filepath.Join(gitDirectory, "gitdir"))
	if err != nil || filepath.Clean(strings.TrimSpace(string(backlink))) != filepath.Join(w.Source, ".git") {
		return "", nativeSealFailure("attempt Git backlink changed")
	}
	return gitDirectory, nil
}

func nativeSealReadMetadata(name string) ([]byte, error) {
	st, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() || st.Size() > 2<<20 {
		return nil, nativeSealFailure("durable metadata is not a bounded regular file")
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(st, opened) {
		return nil, nativeSealFailure("durable metadata changed")
	}
	return io.ReadAll(io.LimitReader(f, (2<<20)+1))
}

// Exact spelling and link checks also apply to tracked protected files. They
// may be hashed to prove that they stayed unchanged, but can never be accepted
// as native edits or imported into a new tree through the effect list.
func nativeSealReadFile(root *os.Root, name string) (nativeSealFile, bool, error) {
	var out nativeSealFile
	if name == "" || strings.ContainsAny(name, "\\:\x00\r\n") || strings.HasPrefix(name, "/") {
		return out, false, nativeSealFailure("invalid source path")
	}
	parent := "."
	parts := strings.Split(name, "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return out, false, nativeSealFailure("invalid source path")
		}
		dir, err := root.Open(parent)
		if os.IsNotExist(err) {
			return out, false, nil
		}
		if err != nil {
			return out, false, nativeSealFailure("source parent is unavailable")
		}
		entries, err := dir.ReadDir(-1)
		dir.Close()
		if err != nil {
			return out, false, nativeSealFailure("source parent is unavailable")
		}
		found := false
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), part) {
				if entry.Name() != part || found {
					return out, false, nativeSealFailure("source path has a case alias")
				}
				found = true
			}
		}
		if !found {
			return out, false, nil
		}
		current := path.Join(parent, part)
		st, err := root.Lstat(current)
		if err != nil || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 || i < len(parts)-1 && !st.IsDir() {
			return out, false, nativeSealFailure("source traverses a link or non-directory")
		}
		parent = current
	}
	f, err := root.Open(name)
	if err != nil {
		return out, false, nativeSealFailure("source file is unavailable")
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > nativeSealMaxBytes {
		return out, false, nativeSealFailure("source is not a bounded regular file")
	}
	current, err := root.Lstat(name)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(st, current) {
		return out, false, nativeSealFailure("source identity changed")
	}
	out.data, err = io.ReadAll(io.LimitReader(f, nativeSealMaxBytes+1))
	if err != nil || len(out.data) > nativeSealMaxBytes {
		return out, false, nativeSealFailure("source byte limit exceeded")
	}
	out.mode = st.Mode()
	return out, true, nil
}

func nativeSealSnapshot(root *os.Root) (map[string]nativeSealFile, error) {
	files := map[string]nativeSealFile{}
	total := 0
	var walk func(string) error
	walk = func(directory string) error {
		f, err := root.Open(directory)
		if err != nil {
			return nativeSealFailure("source directory is unavailable")
		}
		entries, err := f.ReadDir(-1)
		f.Close()
		if err != nil {
			return nativeSealFailure("source directory is unavailable")
		}
		aliases := map[string]bool{}
		for _, entry := range entries {
			name := path.Join(directory, entry.Name())
			folded := strings.ToLower(entry.Name())
			if aliases[folded] {
				return nativeSealFailure("source contains case aliases")
			}
			aliases[folded] = true
			if _, err := nativeWorkspacePath(name); err != nil {
				continue
			}
			st, err := root.Lstat(name)
			if err != nil || st.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
				return nativeSealFailure("source contains a link")
			}
			if st.IsDir() {
				if err := walk(name); err != nil {
					return err
				}
				continue
			}
			file, exists, err := nativeSealReadFile(root, name)
			if err != nil || !exists {
				return nativeSealFailure("source file changed during inventory")
			}
			files[name] = file
			total += len(file.data)
			if len(files) > nativeSealMaxFiles || total > nativeSealMaxBytes {
				return nativeSealFailure("source inventory limit exceeded")
			}
		}
		return nil
	}
	if err := walk("."); err != nil {
		return nil, err
	}
	return files, nil
}

func nativeSealBlob(data []byte, oidLength int) string {
	content := append([]byte("blob "+strconv.Itoa(len(data))+"\x00"), data...)
	if oidLength == 64 {
		h := sha256.Sum256(content)
		return hex.EncodeToString(h[:])
	}
	h := sha1.Sum(content)
	return hex.EncodeToString(h[:])
}

func nativeSealTree(ctx context.Context, w *attemptWorkspace) (map[string]nativeSealEntry, string, error) {
	tree, err := nativeSealText(ctx, w, "", "rev-parse", "--verify", w.BaseCommit+"^{tree}")
	if err != nil || !commitID.MatchString(tree) {
		return nil, "", nativeSealFailure("fixed base tree is unavailable")
	}
	raw, err := nativeSealGit(ctx, w, "", nil, 4<<20, nil, "ls-tree", "-r", "-z", "--full-tree", tree)
	if err != nil {
		return nil, "", err
	}
	entries, aliases := map[string]nativeSealEntry{}, map[string]bool{}
	for _, record := range bytes.Split(raw, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		parts := bytes.SplitN(record, []byte{'\t'}, 2)
		if len(parts) != 2 {
			return nil, "", nativeSealFailure("base tree is invalid")
		}
		fields := strings.Fields(string(parts[0]))
		name := string(parts[1])
		if len(fields) != 3 || fields[1] != "blob" || fields[0] != "100644" && fields[0] != "100755" || !commitID.MatchString(fields[2]) || aliases[strings.ToLower(name)] {
			return nil, "", nativeSealFailure("base tree contains unsupported entries or aliases")
		}
		aliases[strings.ToLower(name)] = true
		entries[name] = nativeSealEntry{mode: fields[0], object: fields[2]}
		if len(entries) > nativeSealMaxFiles {
			return nil, "", nativeSealFailure("base tree file limit exceeded")
		}
	}
	return entries, tree, nil
}

func nativeSealEffects(ctx context.Context, w *attemptWorkspace, root *os.Root, evidence NativeWorkspaceEvidence, base map[string]nativeSealEntry) (map[string]nativeSealFile, []string, error) {
	files, err := nativeSealSnapshot(root)
	if err != nil {
		return nil, nil, err
	}
	// Protected tracked paths are never hidden by ignore rules or path filtering.
	for name := range base {
		if _, ok := files[name]; !ok {
			file, exists, err := nativeSealReadFile(root, name)
			if err != nil {
				return nil, nil, err
			}
			if exists {
				files[name] = file
			}
		}
	}
	if len(files) > nativeSealMaxFiles {
		return nil, nil, nativeSealFailure("source file limit exceeded")
	}
	bytesTotal := 0
	for _, file := range files {
		bytesTotal += len(file.data)
	}
	if bytesTotal > nativeSealMaxBytes {
		return nil, nil, nativeSealFailure("source byte limit exceeded")
	}
	effects := map[string]NativeWorkspaceFileEffect{}
	aliases := map[string]bool{}
	for _, effect := range evidence.Files {
		name, err := nativeWorkspacePath(effect.Path)
		if err != nil || aliases[strings.ToLower(name)] || effect.OperationID == "" || effect.Generation < 1 || effect.Generation > evidence.Generation || effect.PreviousSHA256 != "" && !evidenceHash.MatchString(effect.PreviousSHA256) {
			return nil, nil, nativeSealFailure("accepted file effect is invalid")
		}
		aliases[strings.ToLower(name)] = true
		old, wasTracked := base[name]
		if wasTracked {
			data, err := nativeSealGit(ctx, w, "", nil, nativeSealMaxBytes, nil, "cat-file", "blob", old.object)
			if err != nil || digest(data) != effect.PreviousSHA256 {
				return nil, nil, nativeSealFailure("accepted initial file hash differs from fixed base")
			}
		} else if effect.PreviousSHA256 != "" {
			return nil, nil, nativeSealFailure("accepted initial file was not in fixed base")
		}
		file, exists := files[name]
		if effect.Removed {
			if exists || effect.SHA256 != "" {
				return nil, nil, nativeSealFailure("accepted removal changed")
			}
		} else if !exists || !evidenceHash.MatchString(effect.SHA256) || digest(file.data) != effect.SHA256 || len(file.data) > 1<<20 {
			return nil, nil, nativeSealFailure("accepted file content changed")
		}
		effects[name] = effect
	}
	changed := map[string]bool{}
	for name, entry := range base {
		file, exists := files[name]
		if !exists || nativeSealBlob(file.data, len(w.BaseCommit)) != entry.object {
			changed[name] = true
		}
		if exists && runtime.GOOS != "windows" && (file.mode.Perm()&0111 != 0) != (entry.mode == "100755") {
			return nil, nil, nativeSealFailure("source executable mode changed")
		}
	}
	for name := range files {
		if _, ok := base[name]; !ok {
			changed[name] = true
		}
	}
	paths := make([]string, 0, len(changed))
	for name := range changed {
		if _, ok := effects[name]; !ok {
			return nil, nil, nativeSealFailure("source change has no accepted file effect")
		}
		paths = append(paths, name)
	}
	sort.Strings(paths)
	return files, paths, nil
}

// Only Git's built-in, UTF-8 conversions may participate in the clean check.
// Arbitrary filter commands are never launched, including during status.
func nativeSealAttributes(ctx context.Context, w *attemptWorkspace, files map[string]nativeSealFile) error {
	paths := make([]string, 0, len(files))
	for name := range files {
		paths = append(paths, name)
	}
	sort.Strings(paths)
	input := []byte(strings.Join(paths, "\x00") + "\x00")
	raw, err := nativeSealGit(ctx, w, "", input, 4<<20, nil, "check-attr", "-z", "--stdin", "filter", "working-tree-encoding")
	if err != nil {
		return err
	}
	fields := bytes.Split(raw, []byte{0})
	if len(fields) != len(paths)*6+1 {
		return nativeSealFailure("Git attributes could not be proven")
	}
	for i := 0; i+2 < len(fields); i += 3 {
		attribute, value := string(fields[i+1]), strings.ToLower(string(fields[i+2]))
		if value == "unspecified" || value == "unset" {
			continue
		}
		if attribute == "filter" || attribute == "working-tree-encoding" && value != "utf-8" && value != "utf8" {
			return nativeSealFailure("external filters or non-UTF-8 source encoding are unsupported")
		}
	}
	return nil
}

func nativeSealStatus(ctx context.Context, w *attemptWorkspace, index string, final bool) error {
	raw, err := nativeSealGit(ctx, w, index, nil, 4<<20, nil, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return err
	}
	for _, line := range bytes.Split(raw, []byte{0}) {
		if len(line) == 0 {
			continue
		}
		if final || len(line) < 4 || line[1] != ' ' || line[0] == '?' || line[0] == '!' {
			return nativeSealFailure("raw candidate bytes do not form a clean Git worktree")
		}
	}
	return nil
}

func nativeSealSave(name string, checkpoint nativeSealCheckpoint) error {
	raw, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return err
	}
	return writeEvidence(name, raw)
}

func nativeSealReceiptRef(ctx context.Context, w *attemptWorkspace, ref string) (string, error) {
	raw, err := nativeSealText(ctx, w, "", "for-each-ref", "--format=%(refname) %(objectname)", ref)
	if err != nil || raw == "" {
		return "", err
	}
	fields := strings.Fields(raw)
	if len(fields) != 2 || fields[0] != ref || !commitID.MatchString(fields[1]) {
		return "", nativeSealFailure("operation reference changed")
	}
	return fields[1], nil
}

func nativeSealIndexMatches(ctx context.Context, w *attemptWorkspace, tree string) (bool, error) {
	// A cached diff only reads the normal index. write-tree could populate its
	// cache-tree extension before the durable preparation has been accepted.
	raw, err := nativeSealGit(ctx, w, "", nil, 4<<20, nil, "diff", "--cached", "--no-ext-diff", "--no-textconv", "--name-only", "-z", tree, "--")
	return len(raw) == 0, err
}

// Installing the isolated index changes no worktree files. Its normal Git lock
// and atomic rename keep a crash on either side recoverable from seal.json.
func nativeSealInstallIndex(index, gitDirectory string) error {
	indexPath := filepath.Join(gitDirectory, "index")
	if st, err := os.Lstat(indexPath); err != nil || !st.Mode().IsRegular() {
		return nativeSealFailure("attempt index is unavailable")
	}
	raw, err := nativeSealReadMetadata(index)
	if err != nil {
		return err
	}
	lockPath := indexPath + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nativeSealFailure("attempt index is locked")
	}
	defer os.Remove(lockPath)
	_, err = f.Write(raw)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = replaceFile(lockPath, indexPath)
	}
	return err
}

func sealNativeWorkspace(ctx context.Context, w *attemptWorkspace, evidence NativeWorkspaceEvidence) (result NativeWorkspaceSeal, sealErr error) {
	if w != nil && w.ConflictRepair != nil {
		return sealConflictRepairWorkspace(ctx, w, evidence)
	}
	var empty NativeWorkspaceSeal
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if evidence.SchemaVersion != 1 || !evidenceHash.MatchString(evidence.PolicySHA256) || evidence.Generation < 0 || evidence.State != "closed" || len(evidence.Files) > nativeSealMaxFiles {
		return empty, nativeSealFailure("workspace evidence is not closed and valid")
	}
	gitDirectory, err := nativeSealIdentity(ctx, w)
	if err != nil {
		return empty, err
	}
	directory := filepath.Join(w.Directory, "native-workspace")
	if err := safeMkdir(directory); err != nil {
		return empty, nativeSealFailure("seal metadata directory is unavailable")
	}
	lockPath := filepath.Join(directory, "seal.lock")
	if st, err := os.Lstat(lockPath); err == nil && !st.Mode().IsRegular() || err != nil && !os.IsNotExist(err) {
		return empty, nativeSealFailure("seal operation lock is unavailable")
	}
	unlock, err := lockJournal(lockPath)
	if err != nil {
		return empty, nativeSealFailure("seal operation is already active")
	}
	defer unlock()
	evidence.Seal = nil
	evidence.Files = append([]NativeWorkspaceFileEffect{}, evidence.Files...)
	sort.Slice(evidence.Files, func(i, j int) bool { return evidence.Files[i].Path < evidence.Files[j].Path })
	request, _ := json.Marshal(evidence)
	assignment, _ := json.Marshal(w.Assignment)
	identity, _ := json.Marshal([]any{candidateRef(w.Assignment), evidence.PolicySHA256, w.BaseCommit})
	operation := "native-seal-" + digest(identity)
	metadata := filepath.Join(directory, "seal.json")
	// Preserve the original operation on any later error, including a source
	// drift found before rebuilding its objects. Unknown never grants success.
	var accepted NativeWorkspaceSeal
	if raw, err := nativeSealReadMetadata(metadata); err == nil {
		var original nativeSealCheckpoint
		if json.Unmarshal(raw, &original) == nil && original.SchemaVersion == 1 && original.OperationID == operation && original.AssignmentSHA == digest(assignment) && original.RequestSHA == digest(request) && original.Repository == w.Repository && original.Source == w.Source && original.GitDirectory == gitDirectory && original.Result.SchemaVersion == 1 && original.Result.OperationID == operation && original.Result.PreviousCommit == w.BaseCommit && original.Result.PolicySHA256 == evidence.PolicySHA256 && (original.Result.Commit == "" || commitID.MatchString(original.Result.Commit)) && (original.State == "prepared" || original.State == "refs_applied" || original.State == "completed") {
			accepted = original.Result
		}
	}
	defer func() {
		if sealErr != nil && accepted.OperationID != "" {
			result = accepted
			result.State = "unknown"
		}
	}()
	base, baseTree, err := nativeSealTree(ctx, w)
	if err != nil {
		return empty, err
	}
	root, err := os.OpenRoot(w.Source)
	if err != nil {
		return empty, nativeSealFailure("source is unavailable")
	}
	defer root.Close()
	files, paths, err := nativeSealEffects(ctx, w, root, evidence, base)
	if err != nil {
		return empty, err
	}
	if err := nativeSealAttributes(ctx, w, files); err != nil {
		return empty, err
	}
	checkpoint := nativeSealCheckpoint{SchemaVersion: 1, OperationID: operation, AssignmentSHA: digest(assignment), RequestSHA: digest(request), Repository: w.Repository, Source: w.Source, GitDirectory: gitDirectory, PreviousTree: baseTree, AuthorDate: strconv.FormatInt(time.Now().UTC().Unix(), 10) + " +0000", State: "prepared", Result: NativeWorkspaceSeal{SchemaVersion: 1, OperationID: operation, PolicySHA256: evidence.PolicySHA256, State: "prepared", PreviousCommit: w.BaseCommit, ChangedPaths: len(paths)}}
	raw, readErr := nativeSealReadMetadata(metadata)
	if readErr == nil {
		var prior nativeSealCheckpoint
		if json.Unmarshal(raw, &prior) != nil || prior.SchemaVersion != 1 || prior.OperationID != operation || prior.AssignmentSHA != checkpoint.AssignmentSHA || prior.RequestSHA != checkpoint.RequestSHA || prior.Repository != w.Repository || prior.Source != w.Source || prior.GitDirectory != gitDirectory || prior.PreviousTree != baseTree || prior.Result.SchemaVersion != 1 || prior.Result.OperationID != operation || prior.Result.PreviousCommit != w.BaseCommit || prior.Result.PolicySHA256 != evidence.PolicySHA256 || prior.Result.ChangedPaths != len(paths) || prior.AuthorDate == "" || prior.State != "prepared" && prior.State != "refs_applied" && prior.State != "completed" {
			return empty, nativeSealFailure("original seal operation changed")
		}
		checkpoint = prior
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return empty, nativeSealFailure("original seal receipt is unavailable")
	}
	head, err := nativeSealText(ctx, w, "", "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return empty, err
	}
	ref := "refs/suma/native-candidates/" + operation
	receipt, err := nativeSealReceiptRef(ctx, w, ref)
	if err != nil {
		return empty, err
	}
	if head != w.BaseCommit && (checkpoint.Result.Commit == "" || head != checkpoint.Result.Commit || receipt != checkpoint.Result.Commit) || receipt != "" && (checkpoint.Result.Commit == "" || receipt != checkpoint.Result.Commit || head != receipt) {
		return empty, nativeSealFailure("HEAD or original operation reference changed")
	}
	if checkpoint.State != "prepared" && len(paths) > 0 && (head != checkpoint.Result.Commit || receipt != checkpoint.Result.Commit) {
		return empty, nativeSealFailure("previously applied operation references changed")
	}
	indexMatches, err := nativeSealIndexMatches(ctx, w, baseTree)
	if err == nil && !indexMatches && checkpoint.Result.Tree != "" {
		indexMatches, err = nativeSealIndexMatches(ctx, w, checkpoint.Result.Tree)
	}
	if err != nil || !indexMatches {
		return empty, nativeSealFailure("attempt index changed outside native effects")
	}
	if readErr != nil {
		if err := nativeSealSave(metadata, checkpoint); err != nil {
			return empty, err
		}
	}
	accepted = checkpoint.Result
	index := filepath.Join(directory, "seal.index")
	if st, err := os.Lstat(index); err == nil {
		if !st.Mode().IsRegular() || os.Remove(index) != nil {
			return empty, nativeSealFailure("isolated seal index changed")
		}
	} else if !os.IsNotExist(err) {
		return empty, nativeSealFailure("isolated seal index is unavailable")
	}
	if _, err := nativeSealText(ctx, w, index, "read-tree", w.BaseCommit); err != nil {
		return empty, err
	}
	for _, name := range paths {
		file, exists := files[name]
		if !exists {
			if _, err := nativeSealText(ctx, w, index, "update-index", "--force-remove", "--", name); err != nil {
				return empty, err
			}
			continue
		}
		blob, err := nativeSealGit(ctx, w, index, file.data, 256, nil, "hash-object", "-w", "--no-filters", "--stdin")
		object := strings.TrimSpace(string(blob))
		if err != nil || object != nativeSealBlob(file.data, len(w.BaseCommit)) {
			return empty, nativeSealFailure("raw source object could not be fixed")
		}
		mode := "100644"
		if prior, ok := base[name]; ok {
			mode = prior.mode
		}
		if _, err := nativeSealText(ctx, w, index, "update-index", "--add", "--cacheinfo", mode, object, name); err != nil {
			return empty, err
		}
	}
	tree, err := nativeSealText(ctx, w, index, "write-tree")
	if err != nil || !commitID.MatchString(tree) || checkpoint.Result.Tree != "" && checkpoint.Result.Tree != tree {
		return empty, nativeSealFailure("prepared candidate tree changed")
	}
	// This runs before HEAD changes, with the isolated tree/index. Staged
	// differences are expected; source differences or visible untracked files
	// prove that raw bytes cannot be represented as the requested clean tree.
	if err := nativeSealStatus(ctx, w, index, false); err != nil {
		return empty, err
	}
	anchored, anchorErr := root.Stat(".")
	current, currentErr := os.Stat(w.Source)
	currentGitDirectory, identityErr := nativeSealIdentity(ctx, w)
	if anchorErr != nil || currentErr != nil || !os.SameFile(anchored, current) || identityErr != nil || currentGitDirectory != gitDirectory {
		return empty, nativeSealFailure("source identity changed before reference effect")
	}
	checkpoint.Result.Tree = tree
	if len(paths) == 0 {
		checkpoint.Result.Commit, checkpoint.Result.State = w.BaseCommit, "no_change"
		if receipt != "" || head != w.BaseCommit {
			return empty, nativeSealFailure("no-change operation has unexpected references")
		}
	} else {
		author := []string{"GIT_AUTHOR_NAME=suma native workspace", "GIT_AUTHOR_EMAIL=native-workspace@suma.invalid", "GIT_COMMITTER_NAME=suma native workspace", "GIT_COMMITTER_EMAIL=native-workspace@suma.invalid", "GIT_AUTHOR_DATE=" + checkpoint.AuthorDate, "GIT_COMMITTER_DATE=" + checkpoint.AuthorDate}
		commit, err := nativeSealGit(ctx, w, index, []byte("suma native candidate "+operation+"\n"), 256, author, "commit-tree", tree, "-p", w.BaseCommit)
		candidate := strings.TrimSpace(string(commit))
		if err != nil || !commitID.MatchString(candidate) || checkpoint.Result.Commit != "" && checkpoint.Result.Commit != candidate {
			return empty, nativeSealFailure("prepared candidate commit changed")
		}
		checkpoint.Result.Commit = candidate
		if err := nativeSealSave(metadata, checkpoint); err != nil {
			return empty, err
		}
		accepted = checkpoint.Result
		if receipt == "" {
			transaction := "start\noption no-deref\nupdate HEAD " + candidate + " " + w.BaseCommit + "\ncreate " + ref + " " + candidate + "\nprepare\ncommit\n"
			_, applyErr := nativeSealGit(ctx, w, "", []byte(transaction), 4096, nil, "update-ref", "--stdin")
			// A process/ACK error cannot mean failure after a ref side effect.
			// Inspect the same operation, never generate a replacement commit.
			head, err = nativeSealText(ctx, w, "", "rev-parse", "--verify", "HEAD^{commit}")
			receipt, readErr = nativeSealReceiptRef(ctx, w, ref)
			if err != nil || readErr != nil || head != candidate || receipt != candidate {
				if applyErr != nil {
					return empty, applyErr
				}
				return empty, nativeSealFailure("Git reference effect remains unknown")
			}
		}
		checkpoint.State = "refs_applied"
		if err := nativeSealSave(metadata, checkpoint); err != nil {
			return empty, err
		}
		checkpoint.Result.State = "sealed"
	}
	accepted = checkpoint.Result
	if err := nativeSealInstallIndex(index, gitDirectory); err != nil {
		return empty, err
	}
	if err := nativeSealStatus(ctx, w, "", true); err != nil {
		return empty, err
	}
	// Re-inventory after Git, without ignore rules. No path can acquire a new
	// content value between preflight and the successful public receipt.
	latest, latestPaths, err := nativeSealEffects(ctx, w, root, evidence, base)
	if err != nil || len(latestPaths) != len(paths) || len(latest) != len(files) {
		return empty, nativeSealFailure("source changed while sealing")
	}
	for name, file := range files {
		current, ok := latest[name]
		if !ok || !bytes.Equal(current.data, file.data) {
			return empty, nativeSealFailure("source changed while sealing")
		}
	}
	currentGitDirectory, err = nativeSealIdentity(ctx, w)
	current, currentErr = os.Stat(w.Source)
	if err != nil || currentGitDirectory != gitDirectory || currentErr != nil || !os.SameFile(anchored, current) {
		return empty, nativeSealFailure("source identity changed after reference effect")
	}
	head, err = nativeSealText(ctx, w, "", "rev-parse", "--verify", "HEAD^{commit}")
	receipt, readErr = nativeSealReceiptRef(ctx, w, ref)
	if err != nil || readErr != nil || head != checkpoint.Result.Commit || len(paths) > 0 && receipt != checkpoint.Result.Commit || len(paths) == 0 && receipt != "" {
		return empty, nativeSealFailure("candidate references changed before acknowledgement")
	}
	checkpoint.State = "completed"
	if err := nativeSealSave(metadata, checkpoint); err != nil {
		return empty, err
	}
	return checkpoint.Result, nil
}
