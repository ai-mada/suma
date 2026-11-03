package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type privateProbeScopeKey struct{}

// Only the helpers that create an owned directory may bind this context. Env
// fields supplied by an administrator are never evidence of an isolated HOME.
type privateProbeScope struct {
	mu       sync.Mutex
	root     string
	identity os.FileInfo
	remove   func()
	parent   *privateProbeScope
	trusted  bool
	active   int
	unknown  bool
}

func privateProbeScopeFrom(ctx context.Context) *privateProbeScope {
	s, _ := ctx.Value(privateProbeScopeKey{}).(*privateProbeScope)
	return s
}

func bindPrivateProbeDirectory(ctx context.Context, root string, remove func()) (context.Context, func(), error) {
	if old := privateProbeScopeFrom(ctx); old != nil && old.contains(root) {
		// The existing owner removes the parent only after all nested probes.
		return ctx, func() {}, nil
	}
	info, err := directoryIdentity(root)
	if err != nil || !filepath.IsAbs(root) || filepath.Clean(root) != root || filepath.Dir(root) == root {
		if remove != nil {
			remove()
		}
		return ctx, nil, newProbeFailure("source", "source_unavailable", err)
	}
	s := &privateProbeScope{root: root, identity: info, remove: remove, parent: privateProbeScopeFrom(ctx), trusted: true}
	return context.WithValue(ctx, privateProbeScopeKey{}, s), s.close, nil
}

func newPrivateProbeContext(ctx context.Context) (context.Context, func(), error) {
	root, remove, err := newACPProbeDirectory()
	if err != nil {
		return ctx, nil, newProbeFailure("source", "source_unavailable", err)
	}
	next, closeScope, err := bindPrivateProbeDirectory(ctx, root, remove)
	if err == nil {
		privateProbeScopeFrom(next).trusted = false
	}
	return next, closeScope, err
}

func (s *privateProbeScope) contains(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	rel, err := filepath.Rel(s.root, filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func (s *privateProbeScope) directory(path string) error {
	if !s.contains(path) {
		return fmt.Errorf("private probe directory is outside its owner")
	}
	current, err := directoryIdentity(s.root)
	if err != nil || !os.SameFile(s.identity, current) {
		return fmt.Errorf("private probe root changed")
	}
	rel, _ := filepath.Rel(s.root, filepath.Clean(path))
	root, err := os.OpenRoot(s.root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err = root.MkdirAll(rel, 0700); err != nil {
		return err
	}
	part := s.root
	if rel != "." {
		for _, name := range strings.Split(rel, string(filepath.Separator)) {
			part = filepath.Join(part, name)
			info, e := os.Lstat(part)
			if e != nil {
				return e
			}
			if !info.IsDir() || !cursorPackageOrdinary(info) {
				return fmt.Errorf("private probe directory is linked")
			}
		}
	}
	return nil
}

func (s *privateProbeScope) apply(cmd *exec.Cmd) error {
	home, work, temp := filepath.Join(s.root, "home"), filepath.Join(s.root, "working"), filepath.Join(s.root, "temp")
	for _, p := range []string{home, work, temp} {
		if err := s.directory(p); err != nil {
			return err
		}
	}
	if !s.trusted || !s.contains(cmd.Dir) {
		cmd.Dir = work
	}
	if err := s.directory(cmd.Dir); err != nil {
		return err
	}
	defaults := []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME", "TEMP", "TMP", "TMPDIR", "CODEX_HOME", "CLAUDE_CONFIG_DIR"}
	values := map[string]string{}
	for _, name := range defaults {
		values[name] = home
	}
	for _, name := range []string{"TEMP", "TMP", "TMPDIR"} {
		values[name] = temp
	}
	values["CODEX_HOME"] = filepath.Join(s.root, "codex")
	values["CLAUDE_CONFIG_DIR"] = filepath.Join(s.root, "claude")
	clean := make([]string, 0, len(cmd.Env)+len(defaults))
	for _, entry := range cmd.Env {
		name, value, _ := strings.Cut(entry, "=")
		key := strings.ToUpper(name)
		if _, private := values[key]; private {
			if s.trusted && s.contains(value) {
				values[key] = value
			}
			continue
		}
		if key != "HOMEDRIVE" && key != "HOMEPATH" {
			clean = append(clean, entry)
		}
	}
	for _, name := range defaults {
		if err := s.directory(values[name]); err != nil {
			return err
		}
		clean = append(clean, name+"="+values[name])
	}
	if runtime.GOOS == "windows" {
		volume := filepath.VolumeName(values["USERPROFILE"])
		clean = append(clean, "HOMEDRIVE="+volume, "HOMEPATH="+strings.TrimPrefix(values["USERPROFILE"], volume))
	}
	cmd.Env = clean
	return nil
}

func (s *privateProbeScope) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.unknown {
		return newProbeFailure("protocol", "stop_unverified", nil)
	}
	s.active++
	return nil
}
func (s *privateProbeScope) retain() {
	s.mu.Lock()
	s.unknown = true
	s.mu.Unlock()
	if s.parent != nil {
		s.parent.retain()
	}
}
func (s *privateProbeScope) end(stopped bool) {
	s.mu.Lock()
	if s.active > 0 {
		s.active--
	}
	if !stopped {
		s.unknown = true
	}
	s.mu.Unlock()
	if !stopped && s.parent != nil {
		s.parent.retain()
	}
}
func (s *privateProbeScope) close() {
	s.mu.Lock()
	safe := s.active == 0 && !s.unknown
	remove := s.remove
	if safe {
		s.remove = nil
	}
	s.mu.Unlock()
	// Unknown means the original reconciliation owns the state. Never infer
	// absence of descendants from a closed handle, elapsed deadline or PID.
	if safe && remove != nil {
		current, err := directoryIdentity(s.root)
		if err == nil && os.SameFile(s.identity, current) {
			remove()
		}
	}
}

func prepareProbeScope(ctx context.Context, cmd *exec.Cmd) (*privateProbeScope, func(), error) {
	s := privateProbeScopeFrom(ctx)
	closeScope := func() {}
	if s == nil {
		if collector := toolProbeCollectorFrom(ctx); collector != nil {
			// These directories belong to the original durable maintenance owner.
			root, err := os.MkdirTemp(collector.directory, "probe-home-")
			if err != nil {
				return nil, nil, err
			}
			next, _, err := bindPrivateProbeDirectory(ctx, root, nil)
			if err != nil {
				return nil, nil, err
			}
			s = privateProbeScopeFrom(next)
			s.trusted = false
		} else {
			next, close, err := newPrivateProbeContext(ctx)
			if err != nil {
				return nil, nil, err
			}
			s = privateProbeScopeFrom(next)
			closeScope = close
		}
	}
	if err := s.apply(cmd); err != nil {
		closeScope()
		return nil, nil, err
	}
	return s, closeScope, nil
}
