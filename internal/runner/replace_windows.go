//go:build windows

package runner

import (
	"syscall"
	"unsafe"
)

func replaceFile(src, dst string) error {
	a, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	b, err := syscall.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	r, _, e := syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW").Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)), 0x1|0x8)
	if r == 0 {
		return e
	}
	return nil
}
