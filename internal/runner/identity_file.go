package runner

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// File mechanics are purpose-neutral. Domain decoders and credential accounts
// remain separate; this helper has no identity, token or authorization fields.
type identityMetadataFile struct {
	path       string
	parentInfo os.FileInfo
	unlock     func() error
}

func openIdentityMetadataFile(path string, create bool) (*identityMetadataFile, error) {
	if !localToolSafeText(path, 4096) {
		return nil, fmt.Errorf("invalid Runner identity path")
	}
	if runtime.GOOS == "windows" && strings.TrimRight(filepath.Base(path), ". ") != filepath.Base(path) {
		// filepath.Abs uses Win32 full-path normalization and can discard the
		// original trailing dot/space before the destination exists.
		return nil, fmt.Errorf("Runner identity path contains a Windows filename alias")
	}
	abs, err := filepath.Abs(path)
	if err != nil || !localToolSafeText(abs, 4096) {
		return nil, fmt.Errorf("Runner identity path is unavailable")
	}
	if runtime.GOOS == "windows" {
		// Win32 aliases and alternate streams cannot identify an uncreated file
		// canonically. Refuse them before assigning a system credential account.
		if strings.HasPrefix(abs, `\\?\`) || strings.HasPrefix(abs, `\\.\`) || strings.Contains(abs[len(filepath.VolumeName(abs)):], ":") {
			return nil, fmt.Errorf("Runner identity needs an unambiguous ordinary file path")
		}
		for _, part := range strings.Split(abs[len(filepath.VolumeName(abs)):], string(filepath.Separator)) {
			if strings.TrimRight(part, ". ") != part {
				return nil, fmt.Errorf("Runner identity path contains a Windows filename alias")
			}
		}
	}
	parent := filepath.Dir(abs)
	if create {
		if err = safeMkdir(parent); err != nil {
			return nil, fmt.Errorf("Runner identity directory is unavailable or linked")
		}
	}
	for current := parent; ; current = filepath.Dir(current) {
		info, statErr := os.Lstat(current)
		if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("Runner identity directory is unavailable or linked")
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return nil, fmt.Errorf("Runner identity directory is unavailable")
	}
	abs = filepath.Join(parent, filepath.Base(abs))
	info, err := os.Lstat(abs)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("Runner identity is not a regular public metadata file")
		}
		abs, err = filepath.EvalSymlinks(abs)
		if err != nil {
			return nil, fmt.Errorf("Runner identity path is unavailable")
		}
	} else if !os.IsNotExist(err) || !create {
		return nil, fmt.Errorf("Runner identity metadata is unavailable")
	}
	lock := abs + ".lock"
	if info, err = os.Lstat(lock); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != 0 {
			return nil, fmt.Errorf("Runner identity lock is not a regular owned lock file")
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("Runner identity lock is unavailable")
	}
	unlock, err := lockJournal(lock)
	if err != nil {
		return nil, fmt.Errorf("Runner identity is already owned or its lock is unavailable")
	}
	parentInfo, err := os.Lstat(filepath.Dir(abs))
	if err != nil {
		_ = unlock()
		return nil, fmt.Errorf("Runner identity directory changed before opening")
	}
	h := &identityMetadataFile{path: abs, parentInfo: parentInfo, unlock: unlock}
	if err = h.checkParent(); err != nil {
		_ = unlock()
		return nil, err
	}
	return h, nil
}
func (h *identityMetadataFile) checkParent() error {
	parent := filepath.Dir(h.path)
	for directory := parent; ; directory = filepath.Dir(directory) {
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || directory == parent && !os.SameFile(info, h.parentInfo) {
			return fmt.Errorf("Runner identity directory changed or now traverses a link")
		}
		if filepath.Dir(directory) == directory {
			return nil
		}
	}
}

func readIdentityMetadataFile(path string) ([]byte, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() > runnerIdentityLimit {
		return nil, fmt.Errorf("invalid Runner identity metadata file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("Runner identity changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(f, runnerIdentityLimit+1))
	if err != nil || len(raw) > runnerIdentityLimit {
		return nil, fmt.Errorf("Runner identity exceeds its metadata bound")
	}
	after, err := os.Lstat(path)
	if err != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, after) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
		return nil, fmt.Errorf("Runner identity changed while reading")
	}
	return raw, nil
}

// Compare the original bytes under the original directory identity before
// persisting any updated public metadata. Never replace a new competing file.
func (h *identityMetadataFile) save(original, raw []byte) error {
	if err := h.checkParent(); err != nil {
		return err
	}
	if len(original) != 0 {
		current, err := readIdentityMetadataFile(h.path)
		if err != nil || !bytes.Equal(current, original) {
			return fmt.Errorf("Runner identity metadata changed; preserve the original request")
		}
	} else {
		f, err := os.OpenFile(h.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("Runner identity metadata destination already exists or is unavailable")
		}
		if err = f.Close(); err != nil {
			return fmt.Errorf("Runner identity metadata destination could not be closed")
		}
	}
	if err := h.checkParent(); err != nil {
		return err
	}
	if err := writeEvidence(h.path, raw); err != nil {
		return fmt.Errorf("Runner identity metadata could not be persisted; preserve its original system credential")
	}
	return nil
}
