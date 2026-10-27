package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func workspaceGitTestCommand(t *testing.T, root string, args ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("Git is required for local Git integration checks")
	}
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=" + os.DevNull, "-c", "user.name=Workspace fixture", "-c", "user.email=workspace@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.autocrlf=false"}, args...)...)
	cmd.Dir = root
	for _, name := range []string{"PATH", "SystemRoot", "WINDIR", "SystemDrive", "ComSpec", "PATHEXT", "TEMP", "TMP"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Git fixture %s: %v %s", args[0], err, raw)
	}
	return strings.TrimSpace(string(raw))
}
func workspaceGitTestWrite(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func workspaceGitTestCommitted(t *testing.T) (*fixture, string) {
	t.Helper()
	f, root := workspaceFixture(t)
	workspaceGitTestCommand(t, root, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, root, "delete.txt", "remove me\n")
	workspaceGitTestWrite(t, root, "old name.txt", "rename contents\n")
	workspaceGitTestCommand(t, root, "add", "--", "main.go", "delete.txt", "old name.txt")
	workspaceGitTestCommand(t, root, "commit", "-m", "Initial fixture")
	return f, root
}

func TestWorkspaceGitExactSourcesAndRestrictedChanges(t *testing.T) {
	f, root := workspaceGitTestCommitted(t)
	ctx := context.Background()
	index := "index 🧪\r\n"
	working := "working 中文 🧪\r\n"
	workspaceGitTestWrite(t, root, "main.go", index)
	workspaceGitTestCommand(t, root, "add", "--", "main.go")
	workspaceGitTestWrite(t, root, "main.go", working)
	workspaceGitTestCommand(t, root, "mv", "--", "old name.txt", "renamed name.txt")
	if err := os.Remove(filepath.Join(root, "delete.txt")); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestWrite(t, root, "new 中文.txt", "new\n")
	workspaceGitTestWrite(t, root, "binary.bin", "private\x00bytes")
	workspaceGitTestWrite(t, root, ".env", "DO_NOT_DISCLOSE=secret")
	status, err := f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || !status.Available || status.Branch != "main" || len(status.Head) != 40 || status.Total != 5 || status.RestrictedFiles != 1 {
		t.Fatal(status, err)
	}
	for _, change := range status.Files {
		if forbiddenPath(change.Path) {
			t.Fatal("restricted name exposed")
		}
	}
	for _, tc := range []struct{ path, source, original, modified, previous string }{
		{"main.go", "index", "original", index, ""}, {"main.go", "worktree", index, working, ""},
		{"renamed name.txt", "index", "rename contents\n", "rename contents\n", "old name.txt"},
		{"new 中文.txt", "worktree", "", "new\n", ""}, {"delete.txt", "worktree", "remove me\n", "", ""},
	} {
		got, e := f.s.GetWorkspaceGitDiff(ctx, f.actor, f.project, tc.path, tc.source)
		if e != nil || got.Binary || got.Original != tc.original || got.Modified != tc.modified || got.PreviousPath != tc.previous || got.OriginalSHA256 != digest(tc.original) || got.ModifiedSHA256 != digest(tc.modified) || got.Head != status.Head {
			t.Fatal(tc.path, got, e)
		}
	}
	binary, err := f.s.GetWorkspaceGitDiff(ctx, f.actor, f.project, "binary.bin", "worktree")
	if err != nil || !binary.Binary || binary.Original != "" || binary.Modified != "" || binary.OriginalSHA256 != "" || binary.ModifiedSHA256 != "" {
		t.Fatal(binary, err)
	}
	_, err = f.s.GetWorkspaceGitDiff(ctx, f.actor, f.project, ".env", "worktree")
	if err == nil {
		t.Fatal("sensitive diff accepted")
	}
	var runs, operations int
	f.s.DB.QueryRow(`SELECT count(*) FROM dag_runs`).Scan(&runs)
	f.s.DB.QueryRow(`SELECT count(*) FROM suma_workspace_file_operations`).Scan(&operations)
	if runs != 0 || operations != 0 {
		t.Fatal("read-only Git created execution or file operations")
	}
}

func TestWorkspaceGitUnbornDetachedAndBoundedStatus(t *testing.T) {
	f, root := workspaceFixture(t)
	ctx := context.Background()
	status, err := f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || status.Available || status.ReasonCode != "not_repository" || len(status.Files) != 0 {
		t.Fatal(status, err)
	}
	workspaceGitTestCommand(t, root, "init", "--initial-branch=main")
	status, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || !status.Available || !status.Unborn || status.Detached || status.Branch != "main" || status.Head != "" {
		t.Fatal(status, err)
	}
	workspaceGitTestCommand(t, root, "add", "--", "main.go")
	workspaceGitTestCommand(t, root, "commit", "-m", "Initial fixture")
	head := workspaceGitTestCommand(t, root, "rev-parse", "HEAD")
	workspaceGitTestCommand(t, root, "checkout", "--detach", head)
	for i := 0; i < 205; i++ {
		workspaceGitTestWrite(t, root, newID()+".txt", "bounded\n")
	}
	status, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || status.Unborn || !status.Detached || status.Branch != "" || status.Head != head || status.Total != 205 || len(status.Files) != 200 || !status.Truncated {
		t.Fatal(status, err)
	}
	t.Setenv("PATH", "")
	status, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || status.Available || status.ReasonCode != "git_unavailable" {
		t.Fatal(status, err)
	}
}

func TestWorkspaceGitRejectsUnsafeConfigAndCurrentBasisChanges(t *testing.T) {
	f, root := workspaceGitTestCommitted(t)
	ctx := context.Background()
	marker := filepath.Join(root, "must-not-run.txt")
	evil := "echo forbidden > " + marker
	workspaceGitTestCommand(t, root, "config", "filter.evil.clean", evil)
	workspaceGitTestWrite(t, root, ".gitattributes", "* filter=evil\n")
	workspaceGitTestWrite(t, root, "main.go", "changed")
	status, err := f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || status.Available || status.ReasonCode != "unsafe_config" {
		t.Fatal(status, err)
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("configured program was invoked")
	}
	workspaceGitTestCommand(t, root, "config", "--unset", "filter.evil.clean")
	workspaceGitTestCommand(t, root, "config", "include.path", filepath.Join(t.TempDir(), "outside.cfg"))
	status, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || status.Available || status.ReasonCode != "unsafe_config" {
		t.Fatal(status, err)
	}
	workspaceGitTestCommand(t, root, "config", "--unset", "include.path")
	if err = os.Remove(filepath.Join(root, ".gitattributes")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "wrong.git"))
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.fsmonitor")
	t.Setenv("GIT_CONFIG_VALUE_0", evil)
	status, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || !status.Available {
		t.Fatal(status, err)
	}
	if _, e := os.Stat(marker); !os.IsNotExist(e) {
		t.Fatal("inherited Git configuration was invoked")
	}
	session, _, err := f.s.openWorkspaceGit(ctx, f.actor, f.project)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	workspaceGitTestCommand(t, root, "add", "--", "main.go")
	requireAuthorityStatus(t, session.finish(), 409)
	session, _, err = f.s.openWorkspaceGit(ctx, f.actor, f.project)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	rebound := filepath.Join(root, "rebound")
	if err = os.Mkdir(rebound, 0700); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": rebound}), 200)
	requireAuthorityStatus(t, session.finish(), 409)
	f.call("POST", "/workspace/binding", f.body(map[string]any{"path": root}), 200)
	if err = os.Rename(filepath.Join(root, ".git"), filepath.Join(root, "metadata.git")); err != nil {
		t.Fatal(err)
	}
	workspaceGitTestWrite(t, root, ".git", "gitdir: metadata.git\n")
	status, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	if err != nil || status.Available || status.ReasonCode != "unsupported_layout" {
		t.Fatal(status, err)
	}
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetWorkspaceGitStatus(ctx, f.actor, f.project)
	requireAuthorityStatus(t, err, 401)
}

func TestWorkspaceGitRejectsChangedFileReadBasis(t *testing.T) {
	f, root := workspaceGitTestCommitted(t)
	workspaceGitTestWrite(t, root, "main.go", "modified before observation")
	session, _, err := f.s.openWorkspaceGit(context.Background(), f.actor, f.project)
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	var change WorkspaceGitChange
	for _, item := range session.changes {
		if item.Path == "main.go" {
			change = item
		}
	}
	if change.WorktreeStatus != "M" {
		t.Fatal(change)
	}
	if err = os.Remove(filepath.Join(root, "main.go")); err != nil {
		t.Fatal(err)
	}
	_, err = session.worktreeContent(change)
	requireAuthorityStatus(t, err, 409)
	// Missing parent directories are legitimate when Git actually observed D.
	change.WorktreeStatus = "D"
	deleted, err := session.worktreeContent(change)
	if err != nil || len(deleted) != 0 {
		t.Fatal(deleted, err)
	}
	workspaceGitTestWrite(t, root, "main.go", "same descriptor before replacement")
	// Materialize the old-inode/current-path state without renaming an open
	// Windows handle (which the OS can legitimately refuse to share for delete).
	if err = os.Rename(filepath.Join(root, "main.go"), filepath.Join(root, "previous-version.txt")); err != nil {
		t.Fatal(err)
	}
	file, err := session.root.Open("previous-version.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	workspaceGitTestWrite(t, root, "main.go", "new inode after ordinary atomic save")
	requireAuthorityStatus(t, session.verifyFileRead("main.go", file, opened, int(opened.Size())), 409)
	current, err := session.root.Open("main.go")
	if err != nil {
		t.Fatal(err)
	}
	defer current.Close()
	version, err := current.Stat()
	if err != nil {
		t.Fatal(err)
	}
	workspaceGitTestWrite(t, root, "main.go", "changed in place")
	requireAuthorityStatus(t, session.verifyFileRead("main.go", current, version, int(version.Size())), 409)
	if err = os.WriteFile(filepath.Join(root, ".git", "objects", "info", "http-alternates"), []byte("https://example.invalid/objects\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, reason, err := session.inspectMetadata()
	if err != nil || reason != "unsupported_layout" {
		t.Fatal(reason, err)
	}
}

func TestWorkspaceGitCombinesStagedDeletionAndRecreatedPath(t *testing.T) {
	f, root := workspaceGitTestCommitted(t)
	workspaceGitTestCommand(t, root, "rm", "--", "main.go")
	workspaceGitTestWrite(t, root, "main.go", "recreated worktree 🧪\r\n")
	porcelain := workspaceGitTestCommand(t, root, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	t.Logf("Real porcelain records: %q", porcelain)
	status, err := f.s.GetWorkspaceGitStatus(context.Background(), f.actor, f.project)
	if err != nil || status.Total != 1 || len(status.Files) != 1 {
		t.Fatal(status, err)
	}
	change := status.Files[0]
	if change.Path != "main.go" || change.IndexStatus != "D" || change.WorktreeStatus != "?" || change.Untracked {
		t.Fatal(change)
	}
	for _, tc := range []struct{ source, original, modified string }{
		{"index", "original", ""},
		{"worktree", "", "recreated worktree 🧪\r\n"},
	} {
		diff, e := f.s.GetWorkspaceGitDiff(context.Background(), f.actor, f.project, "main.go", tc.source)
		if e != nil || diff.Original != tc.original || diff.Modified != tc.modified || diff.OriginalSHA256 != digest(tc.original) || diff.ModifiedSHA256 != digest(tc.modified) {
			t.Fatal(tc.source, diff, e)
		}
	}
}
