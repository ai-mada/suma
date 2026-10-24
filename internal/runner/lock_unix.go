//go:build !windows

package runner

import (
	"os"
	"syscall"
)

func lockJournal(path string) (func() error, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return func() error { return f.Close() }, nil
}
