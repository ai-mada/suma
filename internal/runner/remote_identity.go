package runner

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const RunnerIdentityProtocol = "suma-runner-identity-v1"
const runnerIdentityLimit = 16 << 10

// RunnerIdentity is public recovery metadata. The token remains exclusively in
// system credential storage and the locked handle's private memory.
type RunnerIdentity struct {
	Protocol            string `json:"protocol"`
	ControlURL          string `json:"control_url"`
	ProjectID           string `json:"project_id"`
	PairingID           string `json:"pairing_id"`
	ProposalOperationID string `json:"proposal_operation_id"`
	DeviceName          string `json:"device_name"`
	CredentialSHA256    string `json:"credential_sha256"`
	WorkerID            string `json:"worker_id,omitempty"`
	CredentialExpiresAt string `json:"credential_expires_at,omitempty"`
}

type RunnerIdentityHandle struct {
	mu                   sync.Mutex
	identity             RunnerIdentity
	path, account, token string
	store                RunnerSecretStore
	original             []byte
	parentInfo           os.FileInfo
	unlock               func() error
	closed               bool
}

func DefaultRunnerIdentityPath(pair string) (string, error) {
	if !desktopManagedHex(pair, 32) {
		return "", fmt.Errorf("invalid Runner pairing identity")
	}
	base, err := os.UserConfigDir()
	if err != nil || !filepath.IsAbs(base) || !localToolSafeText(base, 4096) {
		return "", fmt.Errorf("user configuration directory is unavailable")
	}
	return filepath.Join(base, "suma", "runner", pair, "identity.json"), nil
}

func PrepareRunnerPairingIdentity(path, control, project, pair, device string, store RunnerSecretStore) (*RunnerIdentityHandle, error) {
	origin, err := cloudControlOrigin(control)
	if err != nil || !desktopManagedHex(project, 32) || !desktopManagedHex(pair, 32) || store == nil || device != "" && (!localToolSafeText(device, 100) || strings.TrimSpace(device) == "") {
		return nil, fmt.Errorf("invalid Runner pairing identity input")
	}
	device = strings.TrimSpace(device)
	h, err := openRunnerIdentity(path, store, true)
	if err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = h.Close()
		}
	}()
	if len(h.original) != 0 {
		value := h.identity
		if device == "" {
			device = value.DeviceName
		}
		if value.ControlURL != origin || value.ProjectID != project || value.PairingID != pair || value.DeviceName != device {
			return nil, fmt.Errorf("Runner identity belongs to a different original pairing request")
		}
		if err = h.loadSecret(); err != nil {
			return nil, err
		}
		if err = h.checkParent(); err != nil {
			return nil, err
		}
	} else {
		if device == "" {
			device, err = os.Hostname()
			if err != nil || !localToolSafeText(device, 100) || strings.TrimSpace(device) == "" {
				return nil, fmt.Errorf("local hostname is unavailable or invalid; select a bounded device name")
			}
			device = strings.TrimSpace(device)
		}
		h.identity = RunnerIdentity{Protocol: RunnerIdentityProtocol, ControlURL: origin, ProjectID: project, PairingID: pair, DeviceName: device}
		h.account = runnerIdentityAccount(h.identity, h.path)
		if _, err = store.Get(RunnerIdentityProtocol, h.account); !errors.Is(err, ErrRunnerSecretNotFound) {
			return nil, fmt.Errorf("identity metadata is absent but its system credential account is occupied or unavailable; preserve the original account")
		}
		token := make([]byte, 32)
		operation := make([]byte, 16)
		if _, err = rand.Read(token); err != nil {
			return nil, fmt.Errorf("secure Runner identity generation failed")
		}
		if _, err = rand.Read(operation); err != nil {
			return nil, fmt.Errorf("secure Runner identity generation failed")
		}
		h.token = hex.EncodeToString(token)
		h.identity.ProposalOperationID = hex.EncodeToString(operation)
		h.identity.CredentialSHA256 = digest([]byte(h.token))
		// An uncertain write or failed readback deliberately leaves the account
		// untouched. Retrying cannot replace an orphan secret with a new token.
		if err = store.Set(RunnerIdentityProtocol, h.account, h.token); err != nil {
			return nil, fmt.Errorf("system credential persistence is unconfirmed; preserve the original account")
		}
		if err = h.loadSecret(); err != nil {
			return nil, err
		}
		if err = h.save(h.identity); err != nil {
			return nil, err
		}
	}
	ok = true
	return h, nil
}

func LoadRunnerIdentity(path string, store RunnerSecretStore) (*RunnerIdentityHandle, error) {
	if store == nil {
		return nil, fmt.Errorf("system credential storage is required")
	}
	h, err := openRunnerIdentity(path, store, false)
	if err != nil {
		return nil, err
	}
	if err = h.loadSecret(); err != nil {
		_ = h.Close()
		return nil, err
	}
	if err = h.checkParent(); err != nil {
		_ = h.Close()
		return nil, err
	}
	return h, nil
}

func (h *RunnerIdentityHandle) Identity() RunnerIdentity {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.identity
}
func (h *RunnerIdentityHandle) Token() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ""
	}
	return h.token
}
func (h *RunnerIdentityHandle) Path() string { return h.path }
func (h *RunnerIdentityHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	h.token = ""
	return h.unlock()
}

func (h *RunnerIdentityHandle) SaveApproved(worker, expires string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || !strings.HasPrefix(worker, "remote-") || !desktopManagedHex(strings.TrimPrefix(worker, "remote-"), 32) {
		return fmt.Errorf("invalid approved Runner identity")
	}
	if _, err := time.Parse(time.RFC3339, expires); err != nil || !localToolSafeText(expires, 64) {
		return fmt.Errorf("invalid Runner credential expiry")
	}
	if err := h.loadSecret(); err != nil {
		return err
	}
	if h.identity.WorkerID != "" && (h.identity.WorkerID != worker || h.identity.CredentialExpiresAt != expires) {
		return fmt.Errorf("approved Runner identity differs from the original receipt")
	}
	approved := h.identity
	approved.WorkerID, approved.CredentialExpiresAt = worker, expires
	return h.save(approved)
}

func runnerIdentityAccount(identity RunnerIdentity, path string) string {
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	raw, _ := json.Marshal(struct{ ControlURL, ProjectID, PairingID, IdentityPath string }{identity.ControlURL, identity.ProjectID, identity.PairingID, path})
	return digest(raw)
}

func (h *RunnerIdentityHandle) loadSecret() error {
	if h.account == "" {
		h.account = runnerIdentityAccount(h.identity, h.path)
	}
	token, err := h.store.Get(RunnerIdentityProtocol, h.account)
	if err != nil || !desktopManagedHex(token, 64) || digest([]byte(token)) != h.identity.CredentialSHA256 {
		return fmt.Errorf("original Runner secret is missing, unavailable or differs from identity metadata")
	}
	h.token = token
	return nil
}

func openRunnerIdentity(path string, store RunnerSecretStore, create bool) (*RunnerIdentityHandle, error) {
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
	h := &RunnerIdentityHandle{path: abs, store: store, unlock: unlock, parentInfo: parentInfo}
	if err = h.checkParent(); err != nil {
		_ = h.Close()
		return nil, err
	}
	value, raw, err := readRunnerIdentity(abs)
	if err != nil {
		if create && os.IsNotExist(err) {
			return h, nil
		}
		_ = h.Close()
		return nil, fmt.Errorf("Runner identity metadata is corrupt, linked or unavailable")
	}
	h.identity, h.original = value, raw
	if err = h.checkParent(); err != nil {
		_ = h.Close()
		return nil, err
	}
	return h, nil
}

// Pairing may wait for minutes. Recheck the original directory identity before
// adopting or changing metadata; a copied file in a substituted parent is not
// the original identity, even when its public bytes are unchanged.
func (h *RunnerIdentityHandle) checkParent() error {
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

func readRunnerIdentity(path string) (RunnerIdentity, []byte, error) {
	var identity RunnerIdentity
	before, err := os.Lstat(path)
	if err != nil {
		return identity, nil, err
	}
	if !before.Mode().IsRegular() || before.Mode()&os.ModeSymlink != 0 || before.Size() > runnerIdentityLimit {
		return identity, nil, fmt.Errorf("invalid Runner identity metadata file")
	}
	f, err := os.Open(path)
	if err != nil {
		return identity, nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return identity, nil, fmt.Errorf("Runner identity changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(f, runnerIdentityLimit+1))
	if err != nil || len(raw) > runnerIdentityLimit {
		return identity, nil, fmt.Errorf("Runner identity exceeds its metadata bound")
	}
	after, err := os.Lstat(path)
	if err != nil || after.Mode()&os.ModeSymlink != 0 || !os.SameFile(opened, after) || opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) {
		return identity, nil, fmt.Errorf("Runner identity changed while reading")
	}
	fields, err := decodeToolObject(raw)
	if err != nil || len(fields) != 7 && len(fields) != 9 {
		return identity, nil, fmt.Errorf("invalid Runner identity metadata object")
	}
	for _, name := range []string{"protocol", "control_url", "project_id", "pairing_id", "proposal_operation_id", "device_name", "credential_sha256"} {
		if _, ok := fields[name].(string); !ok {
			return identity, nil, fmt.Errorf("invalid Runner identity metadata field")
		}
	}
	if len(fields) == 9 {
		for _, name := range []string{"worker_id", "credential_expires_at"} {
			if value, ok := fields[name].(string); !ok || value == "" {
				return identity, nil, fmt.Errorf("invalid approved Runner identity metadata")
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&identity); err != nil {
		return identity, nil, fmt.Errorf("unknown Runner identity metadata fields")
	}
	origin, err := cloudControlOrigin(identity.ControlURL)
	if err != nil || origin != identity.ControlURL || identity.Protocol != RunnerIdentityProtocol || !desktopManagedHex(identity.ProjectID, 32) || !desktopManagedHex(identity.PairingID, 32) || !desktopManagedHex(identity.ProposalOperationID, 32) || !desktopManagedHex(identity.CredentialSHA256, 64) || !localToolSafeText(identity.DeviceName, 100) || strings.TrimSpace(identity.DeviceName) != identity.DeviceName {
		return RunnerIdentity{}, nil, fmt.Errorf("invalid Runner identity metadata values")
	}
	if identity.WorkerID != "" {
		if !strings.HasPrefix(identity.WorkerID, "remote-") || !desktopManagedHex(strings.TrimPrefix(identity.WorkerID, "remote-"), 32) {
			return RunnerIdentity{}, nil, fmt.Errorf("invalid approved Runner worker")
		}
		if _, err = time.Parse(time.RFC3339, identity.CredentialExpiresAt); err != nil || !localToolSafeText(identity.CredentialExpiresAt, 64) {
			return RunnerIdentity{}, nil, fmt.Errorf("invalid Runner credential expiry")
		}
	}
	return identity, raw, nil
}

func (h *RunnerIdentityHandle) save(value RunnerIdentity) error {
	if err := h.checkParent(); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil || len(raw)+1 > runnerIdentityLimit {
		return fmt.Errorf("Runner identity metadata could not be encoded within its bound")
	}
	raw = append(raw, '\n')
	if len(h.original) != 0 {
		_, raw, err := readRunnerIdentity(h.path)
		if err != nil || !bytes.Equal(raw, h.original) {
			return fmt.Errorf("Runner identity metadata changed; preserve the original request")
		}
	} else {
		// Reserve a new destination exclusively before using the mature atomic
		// writer. Never replace a file that appeared after secret persistence.
		file, err := os.OpenFile(h.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("Runner identity metadata destination already exists or is unavailable")
		}
		if err = file.Close(); err != nil {
			return fmt.Errorf("Runner identity metadata destination could not be closed")
		}
	}
	if err = h.checkParent(); err != nil {
		return err
	}
	if err = writeEvidence(h.path, raw); err != nil {
		return fmt.Errorf("Runner identity metadata could not be persisted; preserve its original system credential")
	}
	h.identity, h.original = value, raw
	return nil
}
