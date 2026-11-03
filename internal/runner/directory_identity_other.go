//go:build !windows

package runner

import (
	"fmt"
	"os"
)

func directoryIdentity(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil || !cursorPackageOrdinary(info) || !info.IsDir() {
		return nil, fmt.Errorf("installation directory identity is unavailable")
	}
	return info, nil
}
