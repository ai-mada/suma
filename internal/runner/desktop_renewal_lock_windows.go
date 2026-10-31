//go:build windows

package runner

import (
	"errors"
	"os"
	"syscall"
)

func desktopRenewalLockBusy(err error) bool {
	return errors.Is(err, syscall.Errno(32)) || errors.Is(err, syscall.Errno(33))
}

// Match the Worker's exclusive journal owner without OPEN_ALWAYS or write
// access. A read-only check may never create/truncate the original lock file.
func lockExistingJournal(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Permit independent metadata reads of our own handle, while excluding the
	// original Worker's required write access and every delete/replace handle.
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
