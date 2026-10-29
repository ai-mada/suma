//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package terminalruntime

import (
	"errors"
	"syscall"

	pty "github.com/aymanbagabas/go-pty"
)

type unixTree struct{ cmd *pty.Cmd }

const BootstrapFlag = "--suma-terminal-pty-child"

func HandleChildProcess(args []string) (bool, int) { return false, 0 }
func newPTYCommand(p pty.Pty, c Config) (*pty.Cmd, error) {
	return p.Command(c.Executable, c.Args...), nil
}

func startPTYProcess(c *pty.Cmd) (treeController, error) {
	// go-pty sets setsid and the controlling terminal itself. Setpgid from the
	// ordinary command runner must not be combined with those PTY semantics.
	if err := c.Start(); err != nil {
		return nil, err
	}
	return &unixTree{cmd: c}, nil
}
func (t *unixTree) Stop() error {
	err := syscall.Kill(-t.cmd.Process.Pid, syscall.SIGKILL)
	if err != nil && err != syscall.ESRCH {
		return errors.Join(ErrTreeUnknown, err)
	}
	// A process group alone cannot prove containment of setsid/double-fork
	// descendants. Until the caller supplies a verified cgroup/container adapter,
	// preserve reconciliation rather than falsely releasing the writer lease.
	return ErrTreeUnknown
}
func (t *unixTree) Finish() error                 { return t.Stop() }
func (t *unixTree) Close()                        {}
func (t *unixTree) ActiveProcesses() (int, error) { return 0, ErrTreeUnknown }
