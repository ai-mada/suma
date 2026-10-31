//go:build !windows

package runner

import (
	"errors"
	"os"
	"syscall"
)

func desktopRenewalLockBusy(err error) bool { return errors.Is(err, syscall.EWOULDBLOCK) }

func lockExistingJournal(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}
