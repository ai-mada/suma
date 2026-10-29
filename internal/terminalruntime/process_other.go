//go:build !windows && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package terminalruntime

import pty "github.com/aymanbagabas/go-pty"

const BootstrapFlag = "--suma-terminal-pty-child"

func HandleChildProcess(args []string) (bool, int)        { return false, 0 }
func newPTYCommand(p pty.Pty, c Config) (*pty.Cmd, error) { return nil, pty.ErrUnsupported }

func startPTYProcess(c *pty.Cmd) (treeController, error) { return nil, pty.ErrUnsupported }
