//go:build !unix

package runner

import "os"

func openExecutableForDigest(path string) (*os.File, error) {
	return os.Open(path)
}
