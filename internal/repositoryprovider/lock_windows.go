//go:build windows

package repositoryprovider

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

func lockProvider(root string) (func() error, error) {
	if st, err := os.Lstat(filepath.Join(root, ".suma-repository-provider.lock")); err == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
		return nil, syscall.EINVAL
	}
	f, e := os.OpenFile(filepath.Join(root, ".suma-repository-provider.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	var o syscall.Overlapped
	k := syscall.NewLazyDLL("kernel32.dll")
	ok, _, err := k.NewProc("LockFileEx").Call(f.Fd(), 3, 0, 1, 0, uintptr(unsafe.Pointer(&o)))
	if ok == 0 {
		f.Close()
		return nil, err
	}
	return func() error {
		k.NewProc("UnlockFileEx").Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&o)))
		return f.Close()
	}, nil
}

func singleLink(f *os.File) bool {
	var info syscall.ByHandleFileInformation
	return syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info) == nil && info.NumberOfLinks == 1 && info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0
}
