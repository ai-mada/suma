//go:build !windows

package deployer

import (
	"os"
	"path/filepath"
	"syscall"
)

func lockProvider(root string) (func() error, error) {
	f, err := os.OpenFile(filepath.Join(root, "provider.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() error { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); return f.Close() }, nil
}
func singleLink(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Nlink == 1
}
func replaceProviderFile(from, to string) error { return os.Rename(from, to) }
