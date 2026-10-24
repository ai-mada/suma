package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func gitFixture(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	gitTest(t, repo, "init")
	gitTest(t, repo, "config", "user.name", "Runner Test")
	gitTest(t, repo, "config", "user.email", "runner@example.invalid")
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".cache/\n"), 0600)
	gitTest(t, repo, "add", "file.txt", ".gitignore")
	gitTest(t, repo, "commit", "-m", "fixture base")
	return repo, strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
}
func gitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, b)
	}
	return string(b)
}

func TestWriteCommandsUsePinnedAttemptWorktreesAndKeepDirtyCheckout(t *testing.T) {
	repo, base := gitFixture(t)
	dirty := []byte("user draft\n")
	_ = os.WriteFile(filepath.Join(repo, "file.txt"), dirty, 0600)
	exe, _ := os.Executable()
	cfg := ExecConfig{AllowExec: true, Repository: repo, BaseCommit: base, Workspace: repo, Commands: map[string]CommandSpec{"edit": {Executable: exe, Args: []string{"-test.run=TestWorkspaceEditHelper", "--"}, Env: map[string]string{"SUMA_WORKSPACE_HELPER": "1"}, TestEvidence: true}}}
	a := Assignment{RunID: "run/unsafe", NodeID: "node", Attempt: 1, Fence: 2, WorkerID: "worker", Type: "command", Command: []string{"edit"}}
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode != 0 || r.WorkspaceEvidence == nil {
		t.Fatalf("isolated execution: %+v", r)
	}
	b, _ := os.ReadFile(filepath.Join(repo, "file.txt"))
	if string(b) != string(dirty) {
		t.Fatal("user checkout changed")
	}
	if got := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD")); got != base {
		t.Fatal("source HEAD changed")
	}
	if r.WorkspaceEvidence["base_commit"] != base || r.WorkspaceEvidence["diff_sha256"] == "" {
		t.Fatal("missing evidence")
	}
	paths, ok := r.WorkspaceEvidence["changed_paths"].([]string)
	if !ok || len(paths) != 2 || paths[0] != "file.txt" || paths[1] != "new.txt" || r.WorkspaceEvidence["changed_paths_truncated"] != false {
		t.Fatalf("changed scope: %+v", r.WorkspaceEvidence)
	}
	again := Execute(context.Background(), a, cfg)
	if again.ExitCode == 0 {
		t.Fatal("attempt executed twice")
	}
	a.Attempt++
	a.Fence++
	second := Execute(context.Background(), a, cfg)
	if second.ExitCode != 0 || second.WorkspaceEvidence["attempt_key"] == r.WorkspaceEvidence["attempt_key"] {
		t.Fatal("attempts shared workspace")
	}
	cfg.BaseCommit = "HEAD"
	a.Attempt++
	if Execute(context.Background(), a, cfg).ExitCode == 0 {
		t.Fatal("unpinned base accepted")
	}
}

func TestWriteCommandsRequireAdminIsolationAndMergePreflightChecksHead(t *testing.T) {
	repo, base := gitFixture(t)
	exe, _ := os.Executable()
	a := Assignment{RunID: "r", NodeID: "n", Attempt: 1, Fence: 1, WorkerID: "w", Type: "command", Command: []string{"edit"}}
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Commands: map[string]CommandSpec{"edit": {Executable: exe}}}
	if r := Execute(context.Background(), a, cfg); r.ExitCode == 0 || !strings.Contains(r.Error, "repository") {
		t.Fatalf("unisolated write allowed: %+v", r)
	}
	cfg.Repository = repo
	cfg.BaseCommit = base
	cfg.Commands["edit"] = CommandSpec{Executable: exe, Args: []string{"-test.run=TestWorkspaceEditHelper", "--"}, Env: map[string]string{"SUMA_WORKSPACE_HELPER": "commit"}, TestEvidence: true}
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode != 0 {
		t.Fatalf("execution: %+v", r)
	}
	request := MergePreflightRequest{AttemptKey: r.WorkspaceEvidence["attempt_key"].(string), ExpectedTargetHEAD: base, ExpectedCandidateHEAD: r.WorkspaceEvidence["head_commit"].(string), ExpectedEvidenceSHA256: r.WorkspaceEvidence["evidence_sha256"].(string), ReviewedBy: "local reviewer"}
	preflight, err := MergePreflight(context.Background(), cfg, request)
	if err != nil || !preflight.Ready || preflight.Merged {
		t.Fatalf("preflight: %+v %v", preflight, err)
	}
	request.ExpectedTargetHEAD = strings.Repeat("0", 40)
	if _, err = MergePreflight(context.Background(), cfg, request); err == nil {
		t.Fatal("stale target accepted")
	}
	request.ExpectedTargetHEAD = base
	request.ExpectedEvidenceSHA256 = strings.Repeat("0", 64)
	if _, err = MergePreflight(context.Background(), cfg, request); err == nil {
		t.Fatal("tampered evidence accepted")
	}
	request.ExpectedEvidenceSHA256 = r.WorkspaceEvidence["evidence_sha256"].(string)
	if err = os.WriteFile(filepath.Join(repo, "file.txt"), []byte("unreviewed target edit\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = MergePreflight(context.Background(), cfg, request); err == nil {
		t.Fatal("dirty target accepted")
	}
	if err = os.WriteFile(filepath.Join(repo, "file.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(repo, ".cache", "runner", "worktrees", request.AttemptKey, "changes.patch"), []byte("changed evidence"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = MergePreflight(context.Background(), cfg, request); err == nil {
		t.Fatal("tampered patch accepted")
	}
	if strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD")) != base {
		t.Fatal("preflight merged")
	}
}

func TestWorktreeCacheCannotTraverseDirectoryLinks(t *testing.T) {
	repo, base := gitFixture(t)
	outside := t.TempDir()
	cache := filepath.Join(repo, ".cache")
	if err := os.Mkdir(cache, 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(cache, "linked")
	if err := os.Symlink(outside, linked); err != nil {
		if runtime.GOOS != "windows" {
			t.Skipf("OS directory symlink unavailable: %v", err)
		}
		if b, junctionErr := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", linked, outside).CombinedOutput(); junctionErr != nil {
			t.Skipf("OS directory link unavailable: %v %s", junctionErr, b)
		}
	}
	t.Cleanup(func() {
		if !within(repo, linked) {
			t.Error("test link escaped fixture")
			return
		}
		if err := os.Remove(linked); err != nil && !os.IsNotExist(err) {
			t.Errorf("unlink fixture directory link: %v", err)
		}
	})
	_, _, err := workspaceConfig(context.Background(), ExecConfig{Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(linked, "attempts")})
	if err == nil {
		t.Fatal("cache link accepted")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("cache escaped into link target")
	}
}

func TestRealGoTestsRunAtPinnedSourceInsteadOfDirtyCheckout(t *testing.T) {
	repo, _ := gitFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module fixture.local/isolated\n\ngo 1.23\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := []byte("package isolated\nimport (\"os\";\"testing\")\nfunc TestPinnedFile(t *testing.T){b,e:=os.ReadFile(\"file.txt\");if e!=nil||string(b)!=\"base\\n\"{t.Fatalf(\"wrong base: %s %v\",b,e)}}\n")
	if err := os.WriteFile(filepath.Join(repo, "check_test.go"), source, 0600); err != nil {
		t.Fatal(err)
	}
	gitTest(t, repo, "add", "go.mod", "check_test.go")
	gitTest(t, repo, "commit", "-m", "fixture real test base")
	base := strings.TrimSpace(gitTest(t, repo, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repo, "check_test.go"), []byte("user draft deliberately not compilable\n"), 0600); err != nil {
		t.Fatal(err)
	}
	exe, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	goCache := os.Getenv("GOCACHE")
	goTemp := os.Getenv("GOTMPDIR")
	if goCache == "" {
		goCache = filepath.Join(repo, ".cache", "verification", "go-build")
	}
	if goTemp == "" {
		goTemp = filepath.Join(repo, ".cache", "verification", "tmp")
	}
	if err = os.MkdirAll(goTemp, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"actual-test": {Executable: exe, Args: []string{"test", "-mod=readonly", "-count=1", "./..."}, TestEvidence: true, Env: map[string]string{"GOCACHE": goCache, "GOTMPDIR": goTemp, "GOPROXY": "off", "GOSUMDB": "off"}}}}
	r := Execute(context.Background(), Assignment{RunID: "real-test", NodeID: "n", WorkerID: "w", Attempt: 1, Fence: 1, Type: "test", Command: []string{"actual-test"}}, cfg)
	if r.ExitCode != 0 || r.Error != "" || !strings.Contains(r.Output, "fixture.local/isolated") || r.WorkspaceEvidence["base_commit"] != base || r.WorkspaceEvidence["test_command"] != true {
		t.Fatalf("real pinned Go tests: %+v", r)
	}
	current, _ := os.ReadFile(filepath.Join(repo, "check_test.go"))
	if string(current) != "user draft deliberately not compilable\n" {
		t.Fatal("real tests changed user source")
	}
}

func TestWorkspaceEditHelper(t *testing.T) {
	mode := os.Getenv("SUMA_WORKSPACE_HELPER")
	if mode == "" {
		return
	}
	b, _ := os.ReadFile("file.txt")
	if string(b) != "base\n" {
		os.Exit(5)
	}
	_ = os.WriteFile("file.txt", []byte("attempt change\n"), 0600)
	_ = os.WriteFile("new.txt", []byte("new\n"), 0600)
	if mode == "commit" {
		cmd := exec.Command("git", "-c", "user.name=Runner Test", "-c", "user.email=runner@example.invalid", "add", "file.txt", "new.txt")
		if cmd.Run() != nil {
			os.Exit(6)
		}
		cmd = exec.Command("git", "-c", "user.name=Runner Test", "-c", "user.email=runner@example.invalid", "commit", "-m", "fixture candidate")
		if cmd.Run() != nil {
			os.Exit(7)
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"status": "edited"})
	os.Exit(0)
}
