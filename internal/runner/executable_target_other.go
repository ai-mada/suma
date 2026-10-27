//go:build !windows

package runner

import "path/filepath"

func resolveExecutableTarget(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
