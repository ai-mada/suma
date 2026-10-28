package platform

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	workspaceGitInvalid     = "Workspace Git request is invalid"
	workspaceGitChanged     = "Workspace Git metadata or binding changed; refresh and retry"
	workspaceGitFailed      = "Workspace Git inspection failed"
	workspaceGitLimit       = "Workspace Git inspection exceeds its fixed limit"
	workspaceGitConflicted  = "Resolve this conflicted file before previewing its Git differences"
	workspaceGitNotChanged  = "The selected file has no changes in this Git source"
	workspaceGitUnsupported = "The selected Git file cannot be previewed safely"
	workspaceGitFileLimit   = 1 << 20
	workspaceGitStatusLimit = 200
)

// Git views are current read observations, not candidate/test/acceptance proof.
// Empty wire fields are intentional, including detached/unborn/binary states.
type WorkspaceGitChange struct {
	Path           string `json:"path"`
	PreviousPath   string `json:"previous_path"`
	IndexStatus    string `json:"index_status"`
	WorktreeStatus string `json:"worktree_status"`
	Untracked      bool   `json:"untracked"`
	Conflicted     bool   `json:"conflicted"`
}
type WorkspaceGitStatus struct {
	ProjectID       string               `json:"project_id"`
	Available       bool                 `json:"available"`
	ReasonCode      string               `json:"reason_code"`
	Branch          string               `json:"branch"`
	Head            string               `json:"head"`
	Unborn          bool                 `json:"unborn"`
	Detached        bool                 `json:"detached"`
	Files           []WorkspaceGitChange `json:"files"`
	Total           int                  `json:"total"`
	Limit           int                  `json:"limit"`
	Truncated       bool                 `json:"truncated"`
	RestrictedFiles int                  `json:"restricted_files"`
	ObservedAt      string               `json:"observed_at"`
}
type WorkspaceGitDiff struct {
	ProjectID      string `json:"project_id"`
	Path           string `json:"path"`
	PreviousPath   string `json:"previous_path"`
	Source         string `json:"source"`
	Head           string `json:"head"`
	Original       string `json:"original"`
	Modified       string `json:"modified"`
	OriginalSHA256 string `json:"original_sha256"`
	ModifiedSHA256 string `json:"modified_sha256"`
	Binary         bool   `json:"binary"`
	ObservedAt     string `json:"observed_at"`
}

type workspaceGitSession struct {
	s                                   *Service
	actor                               Actor
	project, path, executable, metadata string
	ctx                                 context.Context
	cancel                              context.CancelFunc
	root                                *os.Root
	identity                            os.FileInfo
	changes                             []WorkspaceGitChange
}

func workspaceGitError(status int, code, message string) error {
	return &APIError{status, "workspace_git_" + code, message, nil}
}
func workspaceGitSamePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// This lexical boundary applies to both current and historical names. A
// tracked .env or an unsafe rename must never leak via status or blob lookup.
func workspaceGitPath(name string) bool {
	if name == "" || len(name) > 4096 || !utf8.ValidString(name) || filepath.IsAbs(name) || strings.ContainsAny(name, "\\:\x00") || forbiddenPath(name) || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
	}
	return true
}

func (g *workspaceGitSession) close() {
	if g.root != nil {
		g.root.Close()
	}
	if g.cancel != nil {
		g.cancel()
	}
}

// Root constrains filesystem access even if a path changes after Lstat. A
// deleted tracked directory may remove several trailing components; all still
// existing parents are checked. Only worktreeContent may interpret absence as
// empty content, after Git actually observed a worktree deletion.
func (g *workspaceGitSession) file(name string, read bool) ([]byte, bool, error) {
	if !workspaceGitPath(name) {
		return nil, false, invalid(workspaceGitInvalid)
	}
	parts := strings.Split(name, "/")
	var info os.FileInfo
	for i := range parts {
		var err error
		info, err = g.root.Lstat(strings.Join(parts[:i+1], "/"))
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, workspaceGitError(422, "unsupported", workspaceGitUnsupported)
		}
		if info.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !info.IsDir() || i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, false, workspaceGitError(422, "unsupported", workspaceGitUnsupported)
		}
	}
	if !read {
		return nil, true, nil
	}
	f, err := g.root.Open(name)
	if err != nil {
		return nil, false, workspaceGitError(409, "changed", workspaceGitChanged)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() {
		return nil, false, workspaceGitError(409, "changed", workspaceGitChanged)
	}
	raw, err := io.ReadAll(io.LimitReader(f, workspaceGitFileLimit+1))
	if err != nil {
		return nil, false, workspaceGitError(422, "failed", workspaceGitFailed)
	}
	if len(raw) > workspaceGitFileLimit {
		return nil, false, workspaceGitError(413, "limit", workspaceGitLimit)
	}
	if err = g.verifyFileRead(name, f, opened, len(raw)); err != nil {
		return nil, false, err
	}
	return raw, true, nil
}

// A concurrent atomic save can replace the path while this descriptor still
// reads the previous inode; an in-place writer can also change size/mtime.
// Neither may be returned as an apparently coherent current file version.
func (g *workspaceGitSession) verifyFileRead(name string, f *os.File, opened os.FileInfo, bytesRead int) error {
	after, err := f.Stat()
	if err != nil || !os.SameFile(opened, after) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || int64(bytesRead) != after.Size() {
		return workspaceGitError(409, "changed", workspaceGitChanged)
	}
	parts := strings.Split(name, "/")
	for i := range parts {
		current, e := g.root.Lstat(strings.Join(parts[:i+1], "/"))
		if e != nil || current.Mode()&os.ModeSymlink != 0 || i < len(parts)-1 && !current.IsDir() {
			return workspaceGitError(409, "changed", workspaceGitChanged)
		}
		if i == len(parts)-1 && (!os.SameFile(after, current) || after.Size() != current.Size() || !after.ModTime().Equal(current.ModTime())) {
			return workspaceGitError(409, "changed", workspaceGitChanged)
		}
	}
	return nil
}

func (g *workspaceGitSession) worktreeContent(change WorkspaceGitChange) ([]byte, error) {
	raw, exists, err := g.file(change.Path, true)
	if err != nil {
		return nil, err
	}
	if !exists && change.WorktreeStatus != "D" {
		return nil, workspaceGitError(409, "changed", workspaceGitChanged)
	}
	return raw, nil
}

// No inherited Git overrides, credentials, HOME configuration or pager enter a
// request. This follows the existing Runner nativeGitCommand policy; commands
// are generated argv, never shell input. Lazy fetch is disabled independently
// from protocol policy because reading a promisor object must remain offline.
func (g *workspaceGitSession) command(ctx context.Context, args ...string) *exec.Cmd {
	flags := []string{"--no-pager", "--literal-pathspecs", "--git-dir=" + filepath.Join(g.path, ".git"), "--work-tree=" + g.path,
		"-c", "core.hooksPath=" + os.DevNull, "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.attributesFile=" + os.DevNull,
		"-c", "core.quotePath=false", "-c", "core.longpaths=true", "-c", "core.autocrlf=false", "-c", "core.eol=lf", "-c", "gc.auto=0", "-c", "maintenance.auto=false",
		"-c", "protocol.allow=never", "-c", "submodule.recurse=false", "-c", "credential.helper=", "-c", "core.pager=", "-c", "status.renames=true"}
	cmd := exec.CommandContext(ctx, g.executable, append(flags, args...)...)
	cmd.Dir = g.path
	for _, name := range []string{"PATH", "SystemRoot", "WINDIR", "SystemDrive", "ComSpec", "PATHEXT", "TEMP", "TMP", "TMPDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1", "GIT_NO_LAZY_FETCH=1", "GIT_ALLOW_PROTOCOL=", "GIT_PAGER=cat", "LC_ALL=C")
	cmd.WaitDelay = 500 * time.Millisecond
	return cmd
}

type workspaceGitOutput struct {
	bytes.Buffer
	limit    int
	exceeded bool
	cancel   context.CancelFunc
}

func (b *workspaceGitOutput) Write(raw []byte) (int, error) {
	n := len(raw)
	left := b.limit - b.Len()
	if len(raw) > left {
		b.exceeded = true
		raw = raw[:max(left, 0)]
		b.cancel()
	}
	b.Buffer.Write(raw)
	return n, nil
}
func (g *workspaceGitSession) run(limit int, args ...string) ([]byte, int, error) {
	ctx, cancel := context.WithCancel(g.ctx)
	defer cancel()
	// The command inherits this request's deadline; the additional cancel
	// also terminates a producer as soon as either bounded pipe overflows.
	cmd := g.command(ctx, args...)
	out, stderr := &workspaceGitOutput{limit: limit, cancel: cancel}, &workspaceGitOutput{limit: 8192, cancel: cancel}
	cmd.Stdout, cmd.Stderr = out, stderr
	err := cmd.Run()
	if out.exceeded || stderr.exceeded {
		return nil, -1, workspaceGitError(413, "limit", workspaceGitLimit)
	}
	if g.ctx.Err() != nil {
		return nil, -1, workspaceGitError(504, "timeout", workspaceGitFailed)
	}
	if err == nil {
		return append([]byte(nil), out.Bytes()...), 0, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return append([]byte(nil), out.Bytes()...), exit.ExitCode(), nil
	}
	return nil, -1, workspaceGitError(422, "failed", workspaceGitFailed)
}
func (g *workspaceGitSession) required(limit int, args ...string) ([]byte, error) {
	raw, code, err := g.run(limit, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, workspaceGitError(422, "failed", workspaceGitFailed)
	}
	return raw, nil
}

// The existing workspace binding authorizes this root, not a different
// repository's shared Git metadata. Linked worktrees need a separate explicit
// project/repository binding before their external common-dir can be supported.
func (g *workspaceGitSession) inspectMetadata() (string, string, error) {
	info, err := g.root.Lstat(".git")
	if os.IsNotExist(err) {
		return "", "not_repository", nil
	}
	if err != nil {
		return "", "", workspaceGitError(422, "failed", workspaceGitFailed)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", "unsupported_layout", nil
	}
	hash := sha256.New()
	count, total := 0, 0
	reason := ""
	err = filepath.WalkDir(filepath.Join(g.path, ".git"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if g.ctx.Err() != nil {
			return g.ctx.Err()
		}
		count++
		if count > 50000 {
			return workspaceGitError(413, "limit", workspaceGitLimit)
		}
		rel, e := filepath.Rel(filepath.Join(g.path, ".git"), path)
		if e != nil {
			return e
		}
		rel = filepath.ToSlash(rel)
		st, e := entry.Info()
		if e != nil {
			return e
		}
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() && !st.Mode().IsRegular() {
			reason = "unsupported_layout"
			return fs.SkipAll
		}
		if rel == "commondir" || rel == "objects/info/alternates" || rel == "objects/info/http-alternates" {
			reason = "unsupported_layout"
			return fs.SkipAll
		}
		if st.IsDir() {
			resolved, e := filepath.EvalSymlinks(path)
			if e != nil || !workspaceGitSamePath(resolved, path) {
				reason = "unsupported_layout"
				return fs.SkipAll
			}
			return nil
		}
		critical := rel == "HEAD" || rel == "index" || rel == "config" || rel == "config.worktree" || rel == "packed-refs" || rel == "shallow" || strings.HasPrefix(rel, "refs/") || strings.HasPrefix(rel, "info/")
		if !critical {
			return nil
		}
		if st.Size() > 32<<20 || total+int(st.Size()) > 64<<20 {
			return workspaceGitError(413, "limit", workspaceGitLimit)
		}
		f, e := g.root.Open(".git/" + rel)
		if e != nil {
			return e
		}
		opened, e := f.Stat()
		if e != nil || !os.SameFile(st, opened) {
			f.Close()
			return workspaceGitError(409, "changed", workspaceGitChanged)
		}
		raw, e := io.ReadAll(io.LimitReader(f, (32<<20)+1))
		f.Close()
		if e != nil {
			return e
		}
		if len(raw) > 32<<20 {
			return workspaceGitError(413, "limit", workspaceGitLimit)
		}
		total += len(raw)
		hash.Write([]byte(rel + "\x00"))
		hash.Write(raw)
		hash.Write([]byte{0})
		return nil
	})
	if reason != "" {
		return "", reason, nil
	}
	if err != nil {
		var domain *APIError
		if errors.As(err, &domain) {
			return "", "", err
		}
		return "", "", workspaceGitError(422, "failed", workspaceGitFailed)
	}
	return hex.EncodeToString(hash.Sum(nil)), "", nil
}
func (g *workspaceGitSession) safeConfig() (bool, error) {
	for _, name := range []string{"config", "config.worktree"} {
		if _, err := g.root.Lstat(".git/" + name); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return false, workspaceGitError(422, "failed", workspaceGitFailed)
		}
		raw, err := g.required(1<<20, "config", "--file", filepath.Join(g.path, ".git", name), "--no-includes", "--null", "--list")
		if err != nil {
			return false, err
		}
		for _, record := range bytes.Split(raw, []byte{0}) {
			if len(record) == 0 {
				continue
			}
			pair := strings.SplitN(string(record), "\n", 2)
			key := strings.ToLower(pair[0])
			value := ""
			if len(pair) > 1 {
				value = strings.ToLower(strings.TrimSpace(pair[1]))
			}
			if key == "include.path" || strings.HasPrefix(key, "includeif.") || strings.HasPrefix(key, "filter.") || key == "credential.helper" || key == "core.sshcommand" || key == "core.worktree" || key == "extensions.partialclone" || strings.HasPrefix(key, "remote.") && strings.HasSuffix(key, ".promisor") && value != "false" || key == "core.bare" && value != "false" || key == "core.fsmonitor" && value != "false" && value != "true" {
				return false, nil
			}
		}
	}
	return true, nil
}

func (s *Service) openWorkspaceGit(parent context.Context, actor Actor, project string) (g *workspaceGitSession, out WorkspaceGitStatus, err error) {
	out = WorkspaceGitStatus{ProjectID: project, Files: []WorkspaceGitChange{}, Limit: workspaceGitStatusLimit, ObservedAt: now()}
	root, e := s.workspaceRoot(parent, actor, project)
	if e != nil {
		return nil, out, e
	}
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	g = &workspaceGitSession{s: s, actor: actor, project: project, path: root, ctx: ctx, cancel: cancel}
	defer func() {
		if err != nil || !out.Available {
			if err == nil {
				_, err = s.workspaceReadGuard(ctx, actor, project, root)
			}
			g.close()
			g = nil
		}
	}()
	g.root, err = s.openWorkspaceFileRoot(ctx, actor, project, root)
	if err != nil {
		return g, out, workspaceGitError(422, "failed", workspaceGitFailed)
	}
	g.identity, err = os.Stat(root)
	if err != nil {
		return g, out, err
	}
	g.metadata, out.ReasonCode, err = g.inspectMetadata()
	if err != nil || out.ReasonCode != "" {
		return g, out, err
	}
	g.executable, e = exec.LookPath("git")
	if e != nil {
		out.ReasonCode = "git_unavailable"
		return g, out, nil
	}
	safe, e := g.safeConfig()
	if e != nil {
		return g, out, e
	}
	if !safe {
		out.ReasonCode = "unsafe_config"
		return g, out, nil
	}
	top, e := g.required(8192, "rev-parse", "--show-toplevel")
	if e != nil {
		return g, out, e
	}
	if !workspaceGitSamePath(strings.TrimSpace(string(top)), root) {
		out.ReasonCode = "unsupported_layout"
		return g, out, nil
	}
	head, code, e := g.run(8192, "rev-parse", "--verify", "--quiet", "HEAD")
	if e != nil {
		return g, out, e
	}
	if code == 0 {
		out.Head = strings.TrimSpace(string(head))
		if !gitRevision.MatchString(out.Head) {
			return g, out, workspaceGitError(422, "failed", workspaceGitFailed)
		}
	} else if code != 1 {
		return g, out, workspaceGitError(422, "failed", workspaceGitFailed)
	} else {
		out.Unborn = true
	}
	branch, code, e := g.run(8192, "symbolic-ref", "--quiet", "--short", "HEAD")
	if e != nil {
		return g, out, e
	}
	if code == 0 {
		out.Branch = strings.TrimSpace(string(branch))
	} else if code == 1 && !out.Unborn {
		out.Detached = true
	} else {
		return g, out, workspaceGitError(422, "failed", workspaceGitFailed)
	}
	raw, e := g.required(2<<20, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=all")
	if e != nil {
		return g, out, e
	}
	fields := bytes.Split(raw, []byte{0})
	byPath := map[string]WorkspaceGitChange{}
	for i := 0; i < len(fields); i++ {
		if len(fields[i]) == 0 {
			continue
		}
		if len(fields[i]) < 4 || fields[i][2] != ' ' {
			return g, out, workspaceGitError(422, "failed", workspaceGitFailed)
		}
		change := WorkspaceGitChange{Path: string(fields[i][3:]), IndexStatus: string(fields[i][0]), WorktreeStatus: string(fields[i][1]), Untracked: fields[i][0] == '?' && fields[i][1] == '?'}
		if strings.ContainsAny(change.IndexStatus+change.WorktreeStatus, "RC") {
			i++
			if i >= len(fields) || len(fields[i]) == 0 {
				return g, out, workspaceGitError(422, "failed", workspaceGitFailed)
			}
			change.PreviousPath = string(fields[i])
		}
		change.Conflicted = strings.Contains(change.IndexStatus+change.WorktreeStatus, "U") || change.IndexStatus == "A" && change.WorktreeStatus == "A" || change.IndexStatus == "D" && change.WorktreeStatus == "D"
		if previous, exists := byPath[change.Path]; exists {
			// git rm followed by recreating the same path is two porcelain
			// records: staged D and a separate untracked worktree file. Keep
			// both facts in one addressable change rather than dropping one
			// source or misclassifying the staged deletion as untracked.
			switch {
			case previous.IndexStatus == "D" && previous.WorktreeStatus == " " && change.Untracked:
				previous.WorktreeStatus, previous.Untracked = "?", false
				change = previous
			case change.IndexStatus == "D" && change.WorktreeStatus == " " && previous.Untracked:
				change.WorktreeStatus, change.Untracked = "?", false
			default:
				return g, out, workspaceGitError(422, "failed", workspaceGitFailed)
			}
		}
		byPath[change.Path] = change
	}
	for _, change := range byPath {
		if !workspaceGitPath(change.Path) || change.PreviousPath != "" && !workspaceGitPath(change.PreviousPath) {
			out.RestrictedFiles++
			continue
		}
		if _, _, e = g.file(change.Path, false); e != nil {
			out.RestrictedFiles++
			continue
		}
		if change.PreviousPath != "" {
			if _, _, e = g.file(change.PreviousPath, false); e != nil {
				out.RestrictedFiles++
				continue
			}
		}
		g.changes = append(g.changes, change)
	}
	sort.Slice(g.changes, func(i, j int) bool { return g.changes[i].Path < g.changes[j].Path })
	out.Total = len(g.changes)
	out.Truncated = out.Total > out.Limit
	out.Files = append(out.Files, g.changes[:min(out.Total, out.Limit)]...)
	out.Available = true
	out.ReasonCode = "ready"
	return g, out, nil
}

func (g *workspaceGitSession) finish() error {
	metadata, reason, err := g.inspectMetadata()
	if err != nil || reason != "" || metadata != g.metadata {
		return workspaceGitError(409, "changed", workspaceGitChanged)
	}
	current, err := os.Stat(g.path)
	if err != nil || !os.SameFile(g.identity, current) {
		return workspaceGitError(409, "changed", workspaceGitChanged)
	}
	if _, err = g.s.workspaceReadGuard(g.ctx, g.actor, g.project, g.path); err != nil {
		return err
	}
	return nil
}
func (s *Service) GetWorkspaceGitStatus(ctx context.Context, actor Actor, project string) (WorkspaceGitStatus, error) {
	g, out, err := s.openWorkspaceGit(ctx, actor, project)
	if err != nil || g == nil {
		return out, err
	}
	defer g.close()
	if err = g.finish(); err != nil {
		return WorkspaceGitStatus{}, err
	}
	return out, nil
}

// Git returns blob bytes by a verified object ID, never through show/textconv or
// a working-tree filter. Index and HEAD paths are literal and independently
// checked; unmerged stages and symlinks/submodules have no text preview.
func (g *workspaceGitSession) blob(path, source string) ([]byte, error) {
	var raw []byte
	var err error
	if source == "index" {
		raw, err = g.required(2<<20, "ls-files", "--stage", "-z", "--", path)
	} else {
		raw, err = g.required(2<<20, "ls-tree", "-z", "HEAD", "--", path)
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return []byte{}, nil
	}
	records := bytes.Split(bytes.TrimSuffix(raw, []byte{0}), []byte{0})
	if len(records) != 1 {
		return nil, workspaceGitError(409, "conflicted", workspaceGitConflicted)
	}
	parts := bytes.SplitN(records[0], []byte{'\t'}, 2)
	if len(parts) != 2 || string(parts[1]) != path {
		return nil, workspaceGitError(409, "changed", workspaceGitChanged)
	}
	fields := strings.Fields(string(parts[0]))
	if len(fields) != 3 {
		return nil, workspaceGitError(422, "failed", workspaceGitFailed)
	}
	mode, oid := fields[0], fields[2]
	if source == "index" {
		oid = fields[1]
		if fields[2] != "0" {
			return nil, workspaceGitError(409, "conflicted", workspaceGitConflicted)
		}
	}
	if mode != "100644" && mode != "100755" {
		return nil, workspaceGitError(422, "unsupported", workspaceGitUnsupported)
	}
	if !gitRevision.MatchString(oid) {
		return nil, workspaceGitError(422, "failed", workspaceGitFailed)
	}
	return g.required(workspaceGitFileLimit, "cat-file", "blob", oid)
}
func (s *Service) GetWorkspaceGitDiff(ctx context.Context, actor Actor, project, path, source string) (out WorkspaceGitDiff, err error) {
	if !workspaceGitPath(path) || source != "worktree" && source != "index" {
		return out, invalid(workspaceGitInvalid)
	}
	g, status, err := s.openWorkspaceGit(ctx, actor, project)
	if err != nil {
		return out, err
	}
	if g == nil {
		return out, workspaceGitError(422, status.ReasonCode, workspaceGitFailed)
	}
	defer g.close()
	var selected *WorkspaceGitChange
	for i := range g.changes {
		if g.changes[i].Path == path {
			selected = &g.changes[i]
			break
		}
	}
	if selected == nil {
		return out, workspaceGitError(404, "not_changed", workspaceGitNotChanged)
	}
	if selected.Conflicted {
		return out, workspaceGitError(409, "conflicted", workspaceGitConflicted)
	}
	if source == "index" && (selected.IndexStatus == " " || selected.Untracked) || source == "worktree" && selected.WorktreeStatus == " " {
		return out, workspaceGitError(422, "not_changed", workspaceGitNotChanged)
	}
	originalPath := path
	if selected.PreviousPath != "" && (source == "index" && strings.ContainsAny(selected.IndexStatus, "RC") || source == "worktree" && strings.ContainsAny(selected.WorktreeStatus, "RC")) {
		originalPath = selected.PreviousPath
	}
	var original, modified []byte
	if source == "index" {
		if !status.Unborn {
			original, err = g.blob(originalPath, "head")
		}
		if err == nil {
			modified, err = g.blob(path, "index")
		}
	} else {
		if !selected.Untracked {
			original, err = g.blob(originalPath, "index")
		}
		if err == nil {
			modified, err = g.worktreeContent(*selected)
		}
	}
	if err != nil {
		return out, err
	}
	if err = g.finish(); err != nil {
		return out, err
	}
	out = WorkspaceGitDiff{ProjectID: project, Path: path, PreviousPath: selected.PreviousPath, Source: source, Head: status.Head, ObservedAt: now()}
	out.Binary = bytes.IndexByte(original, 0) >= 0 || bytes.IndexByte(modified, 0) >= 0 || !utf8.Valid(original) || !utf8.Valid(modified)
	if !out.Binary {
		out.Original, out.Modified = string(original), string(modified)
		out.OriginalSHA256, out.ModifiedSHA256 = digest(out.Original), digest(out.Modified)
	}
	return out, nil
}
