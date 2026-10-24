//go:build !windows

package runner

import (
	"os"
	"path/filepath"
)

func replaceFile(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(dst))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
