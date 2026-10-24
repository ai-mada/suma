//go:build !windows

package runner

import (
	"os"
	"syscall"
)

func artifactSingleLink(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
