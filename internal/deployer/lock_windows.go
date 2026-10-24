//go:build windows

package deployer

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func lockProvider(root string) (func() error, error) {
	f, err := os.OpenFile(filepath.Join(root, "provider.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	var overlap syscall.Overlapped
	kernel := syscall.NewLazyDLL("kernel32.dll")
	locked, _, callErr := kernel.NewProc("LockFileEx").Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&overlap)))
	if locked == 0 {
		_ = f.Close()
		return nil, callErr
	}
	return func() error {
		_, _, _ = kernel.NewProc("UnlockFileEx").Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlap)))
		return f.Close()
	}, nil
}
func singleLink(f *os.File) bool {
	var info syscall.ByHandleFileInformation
	return syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info) == nil && info.NumberOfLinks == 1 && info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0
}
func replaceProviderFile(from, to string) error {
	source, e := syscall.UTF16PtrFromString(from)
	if e != nil {
		return e
	}
	target, e := syscall.UTF16PtrFromString(to)
	if e != nil {
		return e
	}
	ok, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW").Call(uintptr(unsafe.Pointer(source)), uintptr(unsafe.Pointer(target)), 0x1|0x8)
	if ok == 0 {
		return err
	}
	return nil
}
