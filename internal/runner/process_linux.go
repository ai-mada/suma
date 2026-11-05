package runner

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const linuxProcessStopWait = 5 * time.Second

type linuxPIDNamespace struct {
	mu        sync.Mutex
	pidFD     int
	quiescent bool
}

func startNativeProcess(c *exec.Cmd, requireContainment bool) (*processController, error) {
	if !nativeProbeContainmentConfigured() {
		if requireContainment {
			return nil, fmt.Errorf("native probe process containment requires %s=1", linuxPIDNamespaceEnv)
		}
		return startProcessGroup(c)
	}

	// Keep the worker's effective identities. In particular, an unprivileged
	// worker's executable must still see its non-root UID and GID.
	uid, gid := os.Geteuid(), os.Getegid()
	pidFD := -1
	c.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags:  syscall.CLONE_NEWUSER | syscall.CLONE_NEWPID,
		UidMappings: []syscall.SysProcIDMap{{ContainerID: uid, HostID: uid, Size: 1}},
		GidMappings: []syscall.SysProcIDMap{{ContainerID: gid, HostID: gid, Size: 1}},
		PidFD:       &pidFD,
	}
	if err := c.Start(); err != nil {
		return nil, fmt.Errorf("cannot launch in a Linux PID namespace: %w", err)
	}
	if pidFD < 0 {
		// Go requests CLONE_PIDFD at launch. Keep this unexpected failure
		// unverified and reap the owned child even if it takes longer to exit.
		_ = c.Process.Kill()
		done := make(chan struct{})
		go func() {
			_ = c.Wait()
			close(done)
		}()
		timer := time.NewTimer(linuxProcessStopWait)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
		}
		return nil, fmt.Errorf("Linux PID namespace init handle unavailable")
	}
	return &processController{&linuxPIDNamespace{pidFD: pidFD}}, nil
}

func (p *linuxPIDNamespace) containment() string { return linuxPIDNamespaceContainment }

func (p *linuxPIDNamespace) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stop()
}

func (p *linuxPIDNamespace) Finish() error { return p.Stop() }

func (p *linuxPIDNamespace) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pidFD >= 0 {
		_ = p.stop()
		_ = unix.Close(p.pidFD)
		p.pidFD = -1
	}
}

func (p *linuxPIDNamespace) stop() error {
	if p.quiescent {
		return nil
	}
	if p.pidFD < 0 {
		return fmt.Errorf("Linux PID namespace init handle is closed")
	}
	stopped, err := p.waitForExit(0)
	if err != nil || stopped {
		return err
	}
	if err := unix.PidfdSendSignal(p.pidFD, unix.SIGKILL, nil, 0); err != nil && err != unix.ESRCH {
		return fmt.Errorf("stop Linux PID namespace init: %w", err)
	}
	stopped, err = p.waitForExit(linuxProcessStopWait)
	if err == nil && !stopped {
		return fmt.Errorf("Linux PID namespace shutdown timed out")
	}
	return err
}

func (p *linuxPIDNamespace) waitForExit(timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	fds := []unix.PollFd{{Fd: int32(p.pidFD), Events: unix.POLLIN}}
	for {
		remaining := max(time.Until(deadline), 0)
		milliseconds := int((remaining + time.Millisecond - 1) / time.Millisecond)
		_, err := unix.Poll(fds, milliseconds)
		if err == unix.EINTR && time.Now().Before(deadline) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("wait for Linux PID namespace init: %w", err)
		}
		if fds[0].Revents&(unix.POLLERR|unix.POLLNVAL) != 0 {
			return false, fmt.Errorf("Linux PID namespace init handle is invalid")
		}
		// The kernel kills and reaps the namespace's descendants during
		// namespace-init exit, before it exposes init exit to this pidfd.
		p.quiescent = fds[0].Revents&(unix.POLLIN|unix.POLLHUP) != 0
		return p.quiescent, nil
	}
}
