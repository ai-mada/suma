package runner

import (
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

	pp "github.com/ai-mada/suma/internal/poolrunnerpairingcontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

const PoolRunnerIdentityProtocol = "suma-pool-runner-identity-v1"
const poolRunnerPurpose = "pool_runner"

// Public metadata is only an exact recovery locator. The Pool secret remains
// in its separate system credential service and the locked handle's memory.
type PoolRunnerIdentity struct {
	Protocol            string `json:"protocol"`
	Purpose             string `json:"purpose"`
	ControlURL          string `json:"control_url"`
	PoolID              string `json:"pool_id"`
	PairingID           string `json:"pairing_id"`
	ProposalOperationID string `json:"proposal_operation_id"`
	DeviceName          string `json:"device_name"`
	CredentialSHA256    string `json:"credential_sha256"`
	OwnerTenantID       string `json:"owner_tenant_id,omitempty"`
	RunnerID            string `json:"runner_id,omitempty"`
	WorkerID            string `json:"worker_id,omitempty"`
	CredentialVersion   int    `json:"credential_version,omitempty"`
	CredentialExpiresAt string `json:"credential_expires_at,omitempty"`
	MaxSlots            int    `json:"max_slots,omitempty"`
}

type PoolRunnerIdentityHandle struct {
	mu             sync.Mutex
	identity       PoolRunnerIdentity
	file           *identityMetadataFile
	account, token string
	original       []byte
	store          RunnerSecretStore
	closed         bool
}

func DefaultPoolRunnerIdentityPath(pair string) (string, error) {
	if !desktopManagedHex(pair, 32) {
		return "", fmt.Errorf("invalid Pool pairing identity")
	}
	base, err := os.UserConfigDir()
	if err != nil || !filepath.IsAbs(base) || !localToolSafeText(base, 4096) {
		return "", fmt.Errorf("user configuration directory unavailable")
	}
	return filepath.Join(base, "suma", "pool-runner", pair, "identity.json"), nil
}

func PreparePoolRunnerPairingIdentity(path, control, pool, pair, device string, store RunnerSecretStore) (*PoolRunnerIdentityHandle, error) {
	origin, err := cloudControlOrigin(control)
	if err != nil || !desktopManagedHex(pool, 32) || !desktopManagedHex(pair, 32) || store == nil || device != "" && (!localToolSafeText(device, 100) || strings.TrimSpace(device) == "") {
		return nil, fmt.Errorf("invalid Pool pairing identity input")
	}
	file, err := openIdentityMetadataFile(path, true)
	if err != nil {
		return nil, err
	}
	h := &PoolRunnerIdentityHandle{file: file, store: store}
	ok := false
	defer func() {
		if !ok {
			_ = h.Close()
		}
	}()
	device = strings.TrimSpace(device)
	value, raw, err := readPoolRunnerIdentity(file.path)
	if err == nil {
		h.identity, h.original = value, raw
		if device == "" {
			device = value.DeviceName
		}
		if value.ControlURL != origin || value.PoolID != pool || value.PairingID != pair || value.DeviceName != device {
			return nil, fmt.Errorf("Pool identity belongs to another original pairing")
		}
		if err = h.loadSecret(); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("Pool identity metadata corrupt, linked or unavailable")
	} else {
		if device == "" {
			device, err = os.Hostname()
			device = strings.TrimSpace(device)
			if err != nil || !localToolSafeText(device, 100) || device == "" {
				return nil, fmt.Errorf("local device name unavailable")
			}
		}
		h.identity = PoolRunnerIdentity{Protocol: PoolRunnerIdentityProtocol, Purpose: poolRunnerPurpose, ControlURL: origin, PoolID: pool, PairingID: pair, DeviceName: device}
		h.account = poolIdentityAccount(h.identity, file.path)
		if _, e := store.Get(PoolRunnerIdentityProtocol, h.account); !errors.Is(e, ErrRunnerSecretNotFound) {
			return nil, fmt.Errorf("Pool metadata absent but system credential account occupied or unavailable; preserve original account")
		}
		secret, operation := make([]byte, 32), make([]byte, 16)
		if _, err = rand.Read(secret); err != nil {
			return nil, fmt.Errorf("secure Pool identity generation failed")
		}
		if _, err = rand.Read(operation); err != nil {
			return nil, fmt.Errorf("secure Pool identity generation failed")
		}
		h.token = hex.EncodeToString(secret)
		h.identity.ProposalOperationID = hex.EncodeToString(operation)
		h.identity.CredentialSHA256 = digest([]byte(h.token))
		if err = store.Set(PoolRunnerIdentityProtocol, h.account, h.token); err != nil {
			return nil, fmt.Errorf("Pool system credential persistence unconfirmed; preserve original account")
		}
		if err = h.loadSecret(); err != nil {
			return nil, err
		}
		if err = h.save(h.identity); err != nil {
			return nil, err
		}
	}
	if err = file.checkParent(); err != nil {
		return nil, err
	}
	ok = true
	return h, nil
}

// Public metadata and requested configuration are checked before the credential
// store is touched. No project identity or environment-token fallback exists.
func LoadPoolRunnerIdentity(path, control, pool, runner string, store RunnerSecretStore) (*PoolRunnerIdentityHandle, error) {
	if store == nil {
		return nil, fmt.Errorf("system credential storage required")
	}
	origin, err := cloudControlOrigin(control)
	if err != nil {
		return nil, fmt.Errorf("Pool control scope invalid")
	}
	file, err := openIdentityMetadataFile(path, false)
	if err != nil {
		return nil, err
	}
	h := &PoolRunnerIdentityHandle{file: file, store: store}
	ok := false
	defer func() {
		if !ok {
			_ = h.Close()
		}
	}()
	h.identity, h.original, err = readPoolRunnerIdentity(file.path)
	if err != nil || h.identity.ControlURL != origin || h.identity.PoolID != pool || h.identity.RunnerID != runner || h.identity.RunnerID == "" {
		return nil, fmt.Errorf("approved Pool identity differs from the selected control and local configuration")
	}
	if err = h.loadSecret(); err != nil {
		return nil, err
	}
	if err = file.checkParent(); err != nil {
		return nil, err
	}
	ok = true
	return h, nil
}

func poolIdentityAccount(value PoolRunnerIdentity, path string) string {
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	raw, _ := json.Marshal([]string{PoolRunnerIdentityProtocol, poolRunnerPurpose, value.ControlURL, value.PoolID, value.PairingID, path})
	return digest(raw)
}
func validPoolRunnerIdentity(v PoolRunnerIdentity) bool {
	origin, err := cloudControlOrigin(v.ControlURL)
	if err != nil || origin != v.ControlURL || v.Protocol != PoolRunnerIdentityProtocol || v.Purpose != poolRunnerPurpose || !desktopManagedHex(v.PoolID, 32) || !desktopManagedHex(v.PairingID, 32) || !desktopManagedHex(v.ProposalOperationID, 32) || !desktopManagedHex(v.CredentialSHA256, 64) || !localToolSafeText(v.DeviceName, 100) || strings.TrimSpace(v.DeviceName) != v.DeviceName || v.DeviceName == "" {
		return false
	}
	if v.RunnerID == "" {
		return v.OwnerTenantID == "" && v.WorkerID == "" && v.CredentialVersion == 0 && v.CredentialExpiresAt == "" && v.MaxSlots == 0
	}
	_, err = time.Parse(time.RFC3339Nano, v.CredentialExpiresAt)
	return desktopManagedHex(v.OwnerTenantID, 32) && desktopManagedHex(v.RunnerID, 32) && v.WorkerID == "pool-"+v.RunnerID && v.CredentialVersion >= 1 && v.MaxSlots >= 1 && v.MaxSlots <= 128 && err == nil
}
func readPoolRunnerIdentity(path string) (PoolRunnerIdentity, []byte, error) {
	var value PoolRunnerIdentity
	raw, err := readIdentityMetadataFile(path)
	if err != nil {
		return value, nil, err
	}
	if strictjson.Decode(raw, &value, runnerIdentityLimit) != nil || !validPoolRunnerIdentity(value) {
		return value, nil, fmt.Errorf("invalid closed Pool identity metadata")
	}
	// The approved optional group is atomic, including explicit zero/null fields.
	fields, _ := decodeToolObject(raw)
	if len(fields) != 8 && len(fields) != 14 {
		return value, nil, fmt.Errorf("partial Pool approval metadata")
	}
	if len(fields) == 14 && value.RunnerID == "" {
		return value, nil, fmt.Errorf("empty Pool approval metadata")
	}
	return value, raw, nil
}
func (h *PoolRunnerIdentityHandle) loadSecret() error {
	if h.account == "" {
		h.account = poolIdentityAccount(h.identity, h.file.path)
	}
	secret, err := h.store.Get(PoolRunnerIdentityProtocol, h.account)
	if err != nil || !desktopManagedHex(secret, 64) || digest([]byte(secret)) != h.identity.CredentialSHA256 {
		return fmt.Errorf("original Pool secret missing, unavailable or differs from metadata")
	}
	h.token = secret
	return nil
}
func (h *PoolRunnerIdentityHandle) save(value PoolRunnerIdentity) error {
	if !validPoolRunnerIdentity(value) {
		return fmt.Errorf("invalid Pool identity metadata")
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil || len(raw)+1 > runnerIdentityLimit {
		return fmt.Errorf("Pool identity metadata exceeds its bound")
	}
	raw = append(raw, '\n')
	if err = h.file.save(h.original, raw); err != nil {
		return err
	}
	h.identity, h.original = value, raw
	return nil
}
func (h *PoolRunnerIdentityHandle) Identity() PoolRunnerIdentity {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.identity
}
func (h *PoolRunnerIdentityHandle) Token() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return ""
	}
	return h.token
}
func (h *PoolRunnerIdentityHandle) Path() string { return h.file.path }
func (h *PoolRunnerIdentityHandle) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return nil
	}
	h.closed = true
	h.token = ""
	return h.file.unlock()
}
func (h *PoolRunnerIdentityHandle) SaveApproved(value pp.PoolRunnerPairing) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || value.State != "approved" || validatePoolPairingObservation(value, h.identity) != nil {
		return fmt.Errorf("invalid approved Pool identity")
	}
	if err := h.loadSecret(); err != nil {
		return err
	}
	approved := h.identity
	approved.OwnerTenantID = value.OwnerTenantID
	approved.RunnerID = value.RunnerID
	approved.WorkerID = value.WorkerID
	approved.CredentialVersion = value.CredentialVersion
	approved.CredentialExpiresAt = value.CredentialExpiresAt
	approved.MaxSlots = value.MaxSlots
	return h.save(approved)
}
