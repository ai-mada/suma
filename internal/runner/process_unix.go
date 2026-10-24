//go:build !windows

package runner

import (
	"fmt"
	"os/exec"
	"syscall"
)

type processController struct{ cmd *exec.Cmd }

func startProcess(c *exec.Cmd) (*processController, error) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		return nil, err
	}
	return &processController{c}, nil
}
func (p *processController) Stop() error {
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return err
	}
	return fmt.Errorf("process group killed; detached descendant containment unavailable")
}
func (p *processController) Close() { _ = p.Stop() }

func (p *processController) Finish() error {
	if err := syscall.Kill(-p.cmd.Process.Pid, 0); err == syscall.ESRCH {
		return nil
	}
	return p.Stop()
}
