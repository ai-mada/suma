//go:build windows

package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// Resolve the target through the opened file. EvalSymlinks additionally asks
// FindFirstFile to normalize every ancestor's spelling; a usable installed CLI
// can sit below an ancestor whose directory enumeration is denied.
func resolveExecutableTarget(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("installed executable is not a regular file")
	}
	name := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(windows.Handle(file.Fd()), &name[0], uint32(len(name)), 0)
		if err != nil {
			return "", err
		}
		if n >= uint32(len(name)) {
			if n >= 32768 {
				return "", fmt.Errorf("installed executable path exceeds Windows limit")
			}
			name = make([]uint16, n+1)
			continue
		}
		resolved := windows.UTF16ToString(name[:n])
		switch {
		case strings.HasPrefix(resolved, `\\?\UNC\`):
			resolved = `\\` + strings.TrimPrefix(resolved, `\\?\UNC\`)
		case strings.HasPrefix(resolved, `\\?\`):
			resolved = strings.TrimPrefix(resolved, `\\?\`)
		default:
			return "", fmt.Errorf("installed executable final path is not a DOS path")
		}
		resolved = filepath.Clean(resolved)
		if !filepath.IsAbs(resolved) {
			return "", fmt.Errorf("installed executable final path is not absolute")
		}
		target, err := os.Stat(resolved)
		if err != nil || !os.SameFile(info, target) {
			return "", fmt.Errorf("installed executable final target changed")
		}
		return resolved, nil
	}
}
