package runner

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReadExecutableDigestRejectsSizeChanges(t *testing.T) {
	ctx := context.Background()
	if _, err := readExecutableDigest(ctx, bytes.NewReader([]byte("old")), 4); !errors.Is(err, errExecutableChanged) {
		t.Fatalf("truncated executable error = %v", err)
	}
	grown := bytes.NewReader([]byte("123456789"))
	if _, err := readExecutableDigest(ctx, grown, 4); !errors.Is(err, errExecutableChanged) {
		t.Fatalf("grown executable error = %v", err)
	}
	if grown.Len() != 4 {
		t.Fatalf("read %d bytes from grown executable, want 5", grown.Size()-int64(grown.Len()))
	}
}

func TestReadExecutableDigestStopsAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads := 0
	file := digestTestReader(func(buf []byte) (int, error) {
		reads++
		cancel()
		return copy(buf, "a"), nil
	})
	if _, err := readExecutableDigest(ctx, file, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("digest error = %v, want cancellation", err)
	}
	if reads != 1 {
		t.Fatalf("read %d times, want 1", reads)
	}
}

type digestTestReader func([]byte) (int, error)

func (r digestTestReader) Read(buf []byte) (int, error) { return r(buf) }

func TestExecutableDigestSnapshotRejectsReplacedPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	oldPath := filepath.Join(dir, "original-tool")
	if err := os.WriteFile(path, []byte("abc"), 0o700); err != nil {
		t.Fatal(err)
	}
	snapshot, err := digestExecutable(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if snapshot.sha256 != want || !snapshot.current(path) {
		t.Fatalf("digest = %q, current = %t", snapshot.sha256, snapshot.current(path))
	}
	if err := os.Rename(path, oldPath); err != nil {
		t.Fatal(err)
	}
	t.Run("same size and timestamp", func(t *testing.T) {
		if err := os.WriteFile(path, []byte("xyz"), 0o700); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(path)
		mtime := snapshot.info.ModTime()
		if err := os.Chtimes(path, mtime, mtime); err != nil {
			t.Fatal(err)
		}
		if snapshot.current(path) {
			t.Fatal("replacement matched the executable snapshot")
		}
	})
	t.Run("symlink back to original", func(t *testing.T) {
		if err := os.Symlink(oldPath, path); err != nil {
			t.Skipf("cannot create a symlink: %v", err)
		}
		defer os.Remove(path)
		if snapshot.current(path) {
			t.Fatal("symlink replacement matched the executable snapshot")
		}
	})
	mtime := snapshot.info.ModTime().Add(time.Second)
	if err := os.Chtimes(oldPath, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	if snapshot.current(oldPath) {
		t.Fatal("timestamp change matched the executable snapshot")
	}
}
