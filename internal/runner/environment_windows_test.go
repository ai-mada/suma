//go:build windows

package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsPowerShellNativeGitHasOrdinaryEnvironmentAndCleanWorktree(t *testing.T) {
	repo, base := gitFixture(t)
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal(err)
	}
	git, err := exec.LookPath("git.exe")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SUMA_PARENT_SECRET", "parent-secret-must-not-be-forwarded")
	script := `$ErrorActionPreference='Stop'
foreach($name in @('SystemDrive','PATHEXT','ComSpec','APPDATA','LOCALAPPDATA','ProgramData')){
  if([string]::IsNullOrWhiteSpace([Environment]::GetEnvironmentVariable($name))){Write-Output ('missing ordinary Windows environment: '+$name);exit 21}
}

if($null -ne [Environment]::GetEnvironmentVariable('SUMA_PARENT_SECRET')){Write-Output 'unexpected parent secret';exit 22}
& $env:SUMA_NATIVE_GIT --version
if($LASTEXITCODE -ne 0){Write-Output 'native Git invocation did not finish';exit 23}
$head=(& $env:SUMA_NATIVE_GIT rev-parse HEAD)
if($LASTEXITCODE -ne 0 -or $head -ne $env:SUMA_EXPECTED_HEAD){Write-Output 'wrong isolated revision';exit 24}
$driveNames=@([Environment]::GetEnvironmentVariables().Keys|Where-Object {$_ -ieq 'SystemDrive'})
if($driveNames.Count -ne 1){Write-Output 'duplicate Windows environment key';exit 25}
Write-Output 'native-git-success'
`
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"powershell-git": {Executable: powershell, Args: []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script}, Env: map[string]string{"SUMA_NATIVE_GIT": git, "SUMA_EXPECTED_HEAD": base, "pAtH": os.Getenv("PATH")}}}}
	a := Assignment{RunID: "windows-context", NodeID: "powershell", WorkerID: "worker", Attempt: 1, Fence: 1, Type: "command", Command: []string{"powershell-git"}}
	r := Execute(context.Background(), a, cfg)
	if r.ExitCode != 0 || r.Error != "" || !strings.Contains(r.Output, "native-git-success") {
		t.Fatalf("PowerShell native Git failed: %+v", r)
	}
	if r.WorkspaceEvidence["clean"] != true || r.WorkspaceEvidence["head_commit"] != base {
		t.Fatalf("minimal Windows environment polluted isolated source: %+v", r.WorkspaceEvidence)
	}
	if err = filepath.WalkDir(repo, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.Contains(strings.ToUpper(path), "%SYSTEMDRIVE%") {
			t.Errorf("unexpanded system path created: %s", path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if current := strings.TrimSpace(gitTest(t, repo, "status", "--porcelain=v1", "--untracked-files=all")); current != "" {
		t.Fatalf("original checkout changed: %q", current)
	}
}

func TestWindowsEnvironmentNamesAreCaseInsensitiveWithoutAmbiguousAliases(t *testing.T) {
	t.Setenv("SUMA_PARENT_SECRET", "must-not-be-inherited")
	env, _, err := commandEnvironment(CommandSpec{Env: map[string]string{"pAtH": "explicit-local-path"}})
	if err != nil {
		t.Fatal(err)
	}
	paths := 0
	for _, entry := range env {
		if strings.HasPrefix(strings.ToUpper(entry), "PATH=") {
			paths++
			if !strings.HasSuffix(entry, "=explicit-local-path") {
				t.Fatal("explicit local environment did not override parent")
			}
		}
		if strings.HasPrefix(entry, "SUMA_PARENT_SECRET=") {
			t.Fatal("unknown parent secret forwarded")
		}
	}
	if paths != 1 {
		t.Fatalf("PATH aliases duplicated: %d", paths)
	}
	if _, _, err = commandEnvironment(CommandSpec{Env: map[string]string{"Path": "first", "PATH": "second"}}); err == nil {
		t.Fatal("ambiguous Windows environment accepted")
	}
}
