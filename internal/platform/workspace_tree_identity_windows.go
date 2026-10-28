//go:build windows

package platform

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// A root-scoped handle exposes the actual volume/file identity through the
// public Windows API. FileInfo.Sys only exposes attributes and is insufficient.
func workspaceTreeFileIdentity(ctx context.Context, root *os.Root, name string, before os.FileInfo) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	file, err := root.Open(name)
	if err != nil {
		return "", workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !workspaceTreeSameMetadata(before, opened) {
		return "", workspaceTreeChangedError()
	}
	var identity windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(file.Fd()), &identity) != nil || identity.FileIndexHigh == 0 && identity.FileIndexLow == 0 {
		return "", workspaceTreeError(422, "unavailable", workspaceTreeUnavailable)
	}
	after, err := file.Stat()
	if err != nil || !workspaceTreeSameMetadata(opened, after) {
		return "", workspaceTreeChangedError()
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x:%08x%08x", identity.VolumeSerialNumber, identity.FileIndexHigh, identity.FileIndexLow), nil
}
