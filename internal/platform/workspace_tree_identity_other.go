//go:build linux || darwin

package platform

import (
	"context"
	"fmt"
	"os"
	"syscall"
)

// Linux and macOS are the current non-Windows runtime targets. Stat_t exposes
// device/inode identity without opening a file or reading its contents.
func workspaceTreeFileIdentity(ctx context.Context, _ *os.Root, _ string, info os.FileInfo) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	identity, ok := info.Sys().(*syscall.Stat_t)
	if !ok || identity.Ino == 0 {
		return "", workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
	}
	return fmt.Sprintf("%d:%d", identity.Dev, identity.Ino), nil
}
