//go:build windows

package runner

import (
	"os"
	"syscall"
)

func artifactSingleLink(f *os.File) bool {
	var info syscall.ByHandleFileInformation
	return syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info) == nil && info.NumberOfLinks == 1 && info.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0
}
