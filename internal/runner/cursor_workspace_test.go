package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func cursorWorkspaceFixture189(t *testing.T) (string, string) {
	t.Helper()
	ancestor := filepath.Join(t.TempDir(), "ancestor")
	source := filepath.Join(ancestor, "repository", "source")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	return ancestor, source
}

func cursorWorkspaceDirectoryLink189(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Skip("directory link unavailable:", err)
		}
		if output, junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).CombinedOutput(); junctionErr != nil {
			t.Skip("directory link unavailable:", err, junctionErr, string(output))
		}
	}
}

func TestCursorWorkspaceRejectsExecutableOverridesAcrossAncestors(t *testing.T) {
	ancestor, source := cursorWorkspaceFixture189(t)
	if err := validateCursorWorkspacePaths(source, nil); err != nil {
		t.Fatal("owned source without executable configuration was rejected", err)
	}
	for _, scope := range []string{source, ancestor} {
		for _, relative := range []string{".cursor/cli.json", ".cursor/mcp.json", ".cursor/hooks.json", ".claude/settings.json", ".claude/settings.local.json"} {
			path := filepath.Join(scope, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("private synthetic body: guard must only inspect metadata"), 0000); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if validateCursorWorkspacePaths(source, nil) == nil {
				t.Fatal("source or actual ancestor executable override was accepted", scope, relative)
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("guard changed the private override instead of rejecting it", err)
			}
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestCursorWorkspaceAllowsRulesAndRejectsInvalidSource(t *testing.T) {
	ancestor, source := cursorWorkspaceFixture189(t)
	for _, relative := range []string{".cursor/rules/project.mdc", "AGENTS.md", ".claude/CLAUDE.md", ".mcp.json"} {
		path := filepath.Join(source, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("owned input remains unchanged\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateCursorWorkspacePaths(source, nil); err != nil {
		t.Fatal("plain rules or a path not loaded as workspace configuration were rejected", err)
	}
	for _, invalid := range []string{"", ".", filepath.Join(ancestor, "missing"), filepath.Join(source, "AGENTS.md")} {
		if validateCursorWorkspacePaths(invalid, nil) == nil {
			t.Fatal("invalid or non-directory source was accepted")
		}
	}
}

func TestCursorWorkspaceRejectsLinkedConfigurationAndSourceAncestors(t *testing.T) {
	ancestor, source := cursorWorkspaceFixture189(t)
	target := t.TempDir()
	cursorWorkspaceDirectoryLink189(t, target, filepath.Join(source, ".cursor"))
	if validateCursorWorkspacePaths(source, nil) == nil {
		t.Fatal("empty linked configuration directory was followed")
	}
	if err := os.Remove(filepath.Join(source, ".cursor")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "ancestor-link")
	cursorWorkspaceDirectoryLink189(t, ancestor, link)
	if validateCursorWorkspacePaths(filepath.Join(link, "repository", "source"), nil) == nil {
		t.Fatal("source traversed a linked ancestor")
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
		t.Fatal("guard changed a link target", err)
	}
}

func TestCursorWorkspaceEnterpriseHooksRefusePresenceAndLinkedParents(t *testing.T) {
	_, source := cursorWorkspaceFixture189(t)
	root := t.TempDir()
	path := filepath.Join(root, "enterprise", "hooks.json")
	if err := validateCursorWorkspacePaths(source, []string{path}); err != nil {
		t.Fatal("missing fixed enterprise hook configuration was rejected", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("private synthetic enterprise hooks"), 0000); err != nil {
		t.Fatal(err)
	}
	if validateCursorWorkspacePaths(source, []string{path}) == nil {
		t.Fatal("enterprise hook configuration escaped the workspace preflight")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if validateCursorEnterpriseHooks([]string{path}) == nil {
		t.Fatal("non-file enterprise hook node was silently ignored")
	}
	link := filepath.Join(root, "linked-enterprise")
	cursorWorkspaceDirectoryLink189(t, t.TempDir(), link)
	if validateCursorEnterpriseHooks([]string{filepath.Join(link, "hooks.json")}) == nil {
		t.Fatal("enterprise hook probe followed a linked parent")
	}
}
