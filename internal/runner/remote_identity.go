package runner

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	return loadRunnerIdentity(path, "", store)
}

// LoadPairedRunnerIdentity selects an existing default identity only. Validate
// its complete metadata and original pairing before accessing the secret store.
func LoadPairedRunnerIdentity(pair string, store RunnerSecretStore) (*RunnerIdentityHandle, error) {
	path, err := DefaultRunnerIdentityPath(pair)
	if err != nil {
		return nil, err
	}
	return loadRunnerIdentity(path, pair, store)
}

func loadRunnerIdentity(path, expectedPair string, store RunnerSecretStore) (*RunnerIdentityHandle, error) {
	if store == nil {
		return nil, fmt.Errorf("system credential storage is required")
	}
	h, err := openRunnerIdentity(path, store, false)
	if err != nil {
		return nil, err
	}
	if expectedPair != "" && h.identity.PairingID != expectedPair {
		_ = h.Close()
		return nil, fmt.Errorf("Runner identity belongs to a different pairing selector")
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
	file, err := openIdentityMetadataFile(path, create)
	if err != nil {
		return nil, err
	}
	h := &RunnerIdentityHandle{path: file.path, store: store, unlock: file.unlock, parentInfo: file.parentInfo}
	value, raw, err := readRunnerIdentity(h.path)
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

func (h *RunnerIdentityHandle) checkParent() error {
	return (&identityMetadataFile{path: h.path, parentInfo: h.parentInfo}).checkParent()
}
func readRunnerIdentity(path string) (RunnerIdentity, []byte, error) {
	var identity RunnerIdentity
	raw, err := readIdentityMetadataFile(path)
	if err != nil {
		return identity, nil, err
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
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil || len(raw)+1 > runnerIdentityLimit {
		return fmt.Errorf("Runner identity metadata could not be encoded within its bound")
	}
	raw = append(raw, '\n')
	file := identityMetadataFile{path: h.path, parentInfo: h.parentInfo}
	if err = file.save(h.original, raw); err != nil {
		return err
	}
	h.identity, h.original = value, raw
	return nil
}
