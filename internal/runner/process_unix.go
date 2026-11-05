//go:build !windows

package runner

import (
	"fmt"
	"os/exec"
	"syscall"
)

type processController struct{ unixProcessScope }

type unixProcessScope interface {
	Stop() error
	Finish() error
	Close()
	containment() string
}

type processGroup struct{ cmd *exec.Cmd }

func startProcessGroup(c *exec.Cmd) (*processController, error) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := c.Start(); err != nil {
		return nil, err
	}
	return &processController{&processGroup{c}}, nil
}
func (p *processGroup) containment() string { return unixGroupContainment }

func (p *processGroup) Stop() error {
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		return err
	}
	return fmt.Errorf("process group killed; detached descendant containment unavailable")
}
func (p *processGroup) Close() { _ = p.Stop() }

func (p *processGroup) Finish() error {
	if err := syscall.Kill(-p.cmd.Process.Pid, 0); err == syscall.ESRCH {
		return nil
	}
	return p.Stop()
}
