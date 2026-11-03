//go:build windows

package runner

import (
	"fmt"
	"os"
	"syscall"
)

// Capture the real FileID from a shared, no-follow directory handle. Windows
// Lstat FileInfo can load its ID lazily through another path open; a false
// SameFile result alone cannot distinguish drift from a failed lazy reopen.
// The observed C-user-directory rejection did not expose that reopen's error.
func directoryIdentity(path string) (os.FileInfo, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, fmt.Errorf("installation directory identity is unavailable")
	}
	// Zero desired access is sufficient for GetFileInformationByHandle and
	// does not require reading ancestor contents or FILE_READ_ATTRIBUTES. A
	// traversable profile ancestor may legitimately deny the latter permission.
	handle, err := syscall.CreateFile(name, 0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_OPEN_REPARSE_POINT|syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return nil, fmt.Errorf("installation directory identity could not be opened: %w", err)
	}
	var attributes syscall.ByHandleFileInformation
	if err = syscall.GetFileInformationByHandle(handle, &attributes); err != nil || attributes.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY == 0 || attributes.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		_ = syscall.CloseHandle(handle)
		return nil, fmt.Errorf("installation directory is linked or unavailable")
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = syscall.CloseHandle(handle)
		return nil, fmt.Errorf("installation directory identity is unavailable")
	}
	info, statErr := file.Stat() // eager volume and FileID, retained after Close.
	closeErr := file.Close()
	if statErr != nil || closeErr != nil || !cursorPackageOrdinary(info) || !info.IsDir() {
		return nil, fmt.Errorf("installation directory identity could not be read")
	}
	return info, nil
}
