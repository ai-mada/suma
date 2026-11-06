//go:build unix

package runner

import (
	"os"
	"syscall"
)

func openExecutableForDigest(path string) (*os.File, error) {
	// The final path component might have become a symlink or FIFO since
	// Lstat. Refuse symlinks and ensure opening a FIFO cannot wait for a peer.
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
}
