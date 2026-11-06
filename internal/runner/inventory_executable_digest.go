package runner

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var errInventoryExecutableChanged = errors.New("configured executable changed during inventory")

// inventoryExecutableDigests lives for one DescribeTools call. A shared
// interpreter only shares its byte digest, while each profile still resolves
// its own command and probes its own protocol and configuration.
type inventoryExecutableDigests struct {
	mu     sync.Mutex
	byPath map[string]*inventoryExecutableDigest
}

type inventoryExecutableDigest struct {
	ready    chan struct{}
	snapshot executableDigestSnapshot
	err      error
	changed  atomic.Bool
}

func (digests *inventoryExecutableDigests) get(ctx context.Context, spec CommandSpec) (string, executableDigestSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return "", executableDigestSnapshot{}, err
	}
	path, err := resolveCommandExecutable(spec)
	if err != nil {
		return "", executableDigestSnapshot{}, err
	}
	digests.mu.Lock()
	if digests.byPath == nil {
		digests.byPath = make(map[string]*inventoryExecutableDigest)
	}
	entry, exists := digests.byPath[path]
	if !exists {
		entry = &inventoryExecutableDigest{ready: make(chan struct{})}
		digests.byPath[path] = entry
	}
	digests.mu.Unlock()

	if !exists {
		entry.snapshot, entry.err = digestExecutable(ctx, path)
		close(entry.ready)
	} else {
		select {
		case <-entry.ready:
		case <-ctx.Done():
			return "", executableDigestSnapshot{}, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return "", executableDigestSnapshot{}, err
	}
	if err := entry.recheck(path); err != nil {
		return "", executableDigestSnapshot{}, err
	}
	return path, entry.snapshot, nil
}

// current checks both the pinned file and the configured command's current
// resolution after all probes. A changed PATH or link cannot reuse the old
// bytes just because its previous target still exists.
func (digests *inventoryExecutableDigests) current(spec CommandSpec, path string) bool {
	digests.mu.Lock()
	entry := digests.byPath[path]
	digests.mu.Unlock()
	if entry == nil {
		return false
	}
	select {
	case <-entry.ready:
	default:
		return false
	}
	if entry.recheck(path) != nil {
		return false
	}
	resolved, err := resolveCommandExecutable(spec)
	return err == nil && resolved == path
}

func (entry *inventoryExecutableDigest) recheck(path string) error {
	if entry.err != nil {
		return entry.err
	}
	if !entry.changed.Load() && !entry.snapshot.current(path) {
		entry.changed.Store(true)
	}
	if entry.changed.Load() {
		return errInventoryExecutableChanged
	}
	return nil
}
