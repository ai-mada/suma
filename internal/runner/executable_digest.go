package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxExecutableBytes int64 = 1 << 30

var errExecutableChanged = errors.New("executable changed during fingerprinting")

type executableDigestSnapshot struct {
	sha256 string
	info   os.FileInfo
}

func resolveCommandExecutable(spec CommandSpec) (string, error) {
	exe, err := exec.LookPath(spec.Executable)
	if err != nil {
		return "", errors.New("configured executable is not installed")
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return "", errors.New("executable path unavailable")
	}
	exe, err = resolveExecutableTarget(exe)
	if err != nil {
		return "", errors.New("executable target unavailable")
	}
	switch strings.ToLower(filepath.Ext(exe)) {
	case ".cmd", ".bat":
		return "", errors.New("shell wrapper is not enabled as a native executable")
	}
	return exe, nil
}

// current checks the resolved path. Lstat rejects a new symlink, even if it
// points back to the file used to calculate this snapshot.
func (s executableDigestSnapshot) current(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && sameExecutableFile(s.info, info)
}

func sameExecutableFile(before, after os.FileInfo) bool {
	return before != nil && before.Mode().IsRegular() && before.Mode() == after.Mode() &&
		before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) && os.SameFile(before, after)
}

// digestExecutable fingerprints the resolved target without executing it. The
// open handle and path must describe the same regular file before and after
// reading, so a replacement or an observed write invalidates the snapshot.
func digestExecutable(ctx context.Context, path string) (executableDigestSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return executableDigestSnapshot{}, err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return executableDigestSnapshot{}, errors.New("executable cannot be read")
	}
	if !pathInfo.Mode().IsRegular() || pathInfo.Size() < 0 || pathInfo.Size() > maxExecutableBytes {
		return executableDigestSnapshot{}, errors.New("executable is not a bounded regular file")
	}
	file, err := openExecutableForDigest(path)
	if err != nil {
		return executableDigestSnapshot{}, errors.New("executable cannot be read")
	}
	defer file.Close()
	before, err := file.Stat()
	if err != nil || !sameExecutableFile(before, pathInfo) {
		return executableDigestSnapshot{}, errExecutableChanged
	}
	digest, err := readExecutableDigest(ctx, file, before.Size())
	if err != nil {
		return executableDigestSnapshot{}, err
	}
	after, err := file.Stat()
	result := executableDigestSnapshot{sha256: digest, info: before}
	if err != nil || !sameExecutableFile(before, after) || !result.current(path) {
		return executableDigestSnapshot{}, errExecutableChanged
	}
	if err := ctx.Err(); err != nil {
		return executableDigestSnapshot{}, err
	}
	return result, nil
}

func readExecutableDigest(ctx context.Context, file io.Reader, size int64) (string, error) {
	hash := sha256.New()
	// Read at most one byte beyond the measured size to detect growth. The
	// caller already checked the size against the executable limit.
	read, err := io.Copy(hash, io.LimitReader(executableDigestReader{ctx, file}, size+1))
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err != nil {
		return "", errors.New("executable digest unavailable")
	}
	if read != size {
		return "", errExecutableChanged
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

type executableDigestReader struct {
	ctx    context.Context
	source io.Reader
}

func (r executableDigestReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}
