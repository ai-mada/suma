//go:build !windows

package repositoryprovider

import (
	"os"
	"path/filepath"
	"syscall"
)

func lockProvider(root string) (func() error, error) {
	if st, err := os.Lstat(filepath.Join(root, ".suma-repository-provider.lock")); err == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
		return nil, syscall.EINVAL
	}
	f, e := os.OpenFile(filepath.Join(root, ".suma-repository-provider.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		f.Close()
		return nil, e
	}
	return func() error { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); return f.Close() }, nil
}

func singleLink(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink == 1
}
