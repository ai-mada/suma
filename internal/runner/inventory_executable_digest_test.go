package runner

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInventoryExecutableDigestSharesSnapshotAndRejectsDrift(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tool.exe")
	modifiedAt := time.Unix(1700000000, 0)
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
			t.Fatal(err)
		}
	}
	write("initial")
	spec := CommandSpec{Executable: path}
	var digests inventoryExecutableDigests
	const profiles = 4
	type result struct {
		path     string
		snapshot executableDigestSnapshot
		err      error
	}
	results := make(chan result, profiles)
	for range profiles {
		go func() {
			resolved, snapshot, err := digests.get(ctx, spec)
			results <- result{resolved, snapshot, err}
		}()
	}
	first := <-results
	if first.err != nil || first.snapshot.sha256 != fmt.Sprintf("%x", sha256.Sum256([]byte("initial"))) {
		t.Fatalf("first inventory digest = %q, error = %v", first.snapshot.sha256, first.err)
	}
	for range profiles - 1 {
		profile := <-results
		if profile.err != nil || profile.path != first.path || profile.snapshot != first.snapshot {
			t.Fatalf("inventory profile did not reuse the same snapshot: %v", profile.err)
		}
	}
	if !digests.current(spec, first.path) {
		t.Fatal("unchanged executable was rejected")
	}

	write("changed executable")
	if digests.current(spec, first.path) {
		t.Fatal("changed executable was accepted after the probes")
	}
	if _, _, err := digests.get(ctx, spec); !errors.Is(err, errInventoryExecutableChanged) {
		t.Fatalf("same inventory after drift returned %v", err)
	}
	var nextScan inventoryExecutableDigests
	_, updated, err := nextScan.get(ctx, spec)
	if err != nil || updated.sha256 != fmt.Sprintf("%x", sha256.Sum256([]byte("changed executable"))) {
		t.Fatalf("new inventory digest = %q, error = %v", updated.sha256, err)
	}

	write("initial")
	if !first.snapshot.current(first.path) {
		t.Fatal("restored file must match the initial metadata for the sticky drift check")
	}
	if _, _, err := digests.get(ctx, spec); !errors.Is(err, errInventoryExecutableChanged) {
		t.Fatalf("same inventory accepted a restored executable after drift: %v", err)
	}
}
