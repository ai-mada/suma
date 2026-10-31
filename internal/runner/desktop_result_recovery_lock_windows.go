//go:build windows

package runner

import (
	"os"
	"syscall"
)

// Write access excludes another apply as well as the original Worker. The
// existing lock bytes are never changed, and a missing lock is never created.
func lockExistingJournalForRecovery(path string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
