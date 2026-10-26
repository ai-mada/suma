package runner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type nativeGitScope struct{}

// This environment is also used before checkout: rejecting filters only after
// a worktree exists is too late to prevent a configured smudge program running.
func nativeGitCommand(ctx context.Context, dir string, args ...string) *exec.Cmd {
	flags := []string{"-c", "core.quotePath=false", "-c", "core.longpaths=true", "-c", "core.hooksPath=" + os.DevNull, "-c", "core.autocrlf=false", "-c", "core.eol=lf", "-c", "core.fsmonitor=false", "-c", "core.attributesFile=" + os.DevNull, "-c", "submodule.recurse=false"}
	flags = append(flags, "-c", "gc.auto=0", "-c", "maintenance.auto=false", "-c", "commit.gpgsign=false", "-c", "protocol.allow=never")
	if len(args) > 0 && args[0] == "diff" {
		args = append([]string{"diff", "--no-ext-diff", "--no-textconv"}, args[1:]...)
	}
	cmd := exec.CommandContext(ctx, "git", append(flags, args...)...)
	cmd.Dir = dir
	for _, name := range []string{"PATH", "SystemRoot", "WINDIR", "SystemDrive", "ComSpec", "PATHEXT", "TEMP", "TMP", "TMPDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			cmd.Env = append(cmd.Env, name+"="+value)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1")
	return cmd
}

func nativeBaseAttributes(ctx context.Context, repo, base string) error {
	paths, err := gitRun(ctx, repo, "ls-tree", "-r", "--name-only", "-z", base)
	if err != nil {
		return fmt.Errorf("native source tree cannot be inspected before checkout")
	}
	cmd := nativeGitCommand(ctx, repo, "check-attr", "--source="+base, "-z", "filter", "working-tree-encoding", "--stdin")
	cmd.Stdin = bytes.NewReader(paths)
	result := newOutput(4<<20, nil)
	cmd.Stdout = result
	cmd.Stderr = result
	if cmd.Run() != nil || result.truncated {
		return fmt.Errorf("native source attributes require a supported bounded Git inspection")
	}
	fields := strings.Split(strings.TrimSuffix(result.b.String(), "\x00"), "\x00")
	if len(fields) == 1 && fields[0] == "" {
		return nil
	}
	if len(fields)%3 != 0 {
		return fmt.Errorf("native source attribute evidence is invalid")
	}
	for i := 0; i < len(fields); i += 3 {
		value := fields[i+2]
		if value == "unspecified" || value == "unset" {
			continue
		}
		if fields[i+1] == "working-tree-encoding" && (strings.EqualFold(value, "UTF-8") || strings.EqualFold(value, "UTF8")) {
			continue
		}
		return fmt.Errorf("native source uses an external filter or unsupported text encoding")
	}
	return nil
}
