package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExecutableTargetFollowsExactFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "installed-cli.exe")
	if err := os.WriteFile(target, []byte("owned executable identity"), 0700); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveExecutableTarget(target)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(target)
	after, err := os.Stat(resolved)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("resolved target differs from opened file")
	}
	if _, _, err = commandExecutable(CommandSpec{Executable: filepath.Join(dir, "missing.exe")}); err == nil {
		t.Fatal("missing executable accepted")
	}
	if _, _, err = commandExecutable(CommandSpec{Executable: dir}); err == nil {
		t.Fatal("directory accepted as executable")
	}
	t.Run("symlink", func(t *testing.T) {
		link := filepath.Join(dir, "cli-link.exe")
		if err := os.Symlink(target, link); err != nil {
			t.Skip("file symlinks unavailable for this test user")
		}
		resolved, err := resolveExecutableTarget(link)
		after, statErr := os.Stat(resolved)
		if err != nil || statErr != nil || !os.SameFile(before, after) || filepath.Clean(resolved) == filepath.Clean(link) {
			t.Fatal("executable link target was not resolved")
		}
	})
}
