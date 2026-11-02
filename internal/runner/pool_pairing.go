package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	pp "github.com/ai-mada/suma/internal/poolrunnerpairingcontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

const PoolRunnerPairingProtocol = "suma-pool-runner-pairing-v1"

type PoolRunnerPairingOptions struct {
	PairingID, ControlURL, PoolID, IdentityPath, DeviceName string
	Secrets                                                 RunnerSecretStore
	HTTP                                                    *http.Client
	Observe                                                 func(PoolRunnerPairingResult) error
	pollInterval, waitLimit                                 time.Duration
}
type PoolRunnerPairingResult struct {
	Protocol            string `json:"protocol"`
	Purpose             string `json:"purpose"`
	State               string `json:"state"`
	Prepared            bool   `json:"prepared"`
	IdentityFile        string `json:"identity_file"`
	PoolID              string `json:"pool_id"`
	PairingID           string `json:"pairing_id"`
	CredentialSHA256    string `json:"credential_sha256"`
	RunnerID            string `json:"runner_id,omitempty"`
	WorkerID            string `json:"worker_id,omitempty"`
	MaxSlots            int    `json:"max_slots,omitempty"`
	CredentialVersion   int    `json:"credential_version,omitempty"`
	CredentialExpiresAt string `json:"credential_expires_at,omitempty"`
}

// PairPoolRunner persists the original secret and proposal before its first
// network operation. Approval is accepted only after a current status response;
// this function never creates a Pool Worker or grants a project capability.
func PairPoolRunner(parent context.Context, options PoolRunnerPairingOptions) (PoolRunnerPairingResult, error) {
	h, err := PreparePoolRunnerPairingIdentity(options.IdentityPath, options.ControlURL, options.PoolID, options.PairingID, options.DeviceName, options.Secrets)
	if err != nil {
		return PoolRunnerPairingResult{}, err
	}
	defer h.Close()
	record := h.Identity()
	result := PoolRunnerPairingResult{Protocol: PoolRunnerPairingProtocol, Purpose: poolRunnerPurpose, IdentityFile: h.Path(), PoolID: record.PoolID, PairingID: record.PairingID, CredentialSHA256: record.CredentialSHA256}
	interval := options.pollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	limit := options.waitLimit
	if limit <= 0 || limit > 10*time.Minute {
		limit = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, limit)
	defer cancel()
	client := pairingHTTPClient(options.HTTP)
	client.Jar = nil
	version := 0
	var basis *pp.PoolRunnerPairing
	verify := func(value pp.PoolRunnerPairing) error {
		if validatePoolPairingObservation(value, record) != nil || value.Version < version {
			return fmt.Errorf("Pool pairing response differs from its original identity or version")
		}
		if basis != nil && !samePoolPairingBasis(*basis, value) {
			return fmt.Errorf("Pool pairing immutable request changed")
		}
		if basis == nil {
			copy := value
			basis = &copy
		}
		version = value.Version
		return nil
	}
	observe := func(value pp.PoolRunnerPairing) error {
		if err := verify(value); err != nil {
			return err
		}
		changed := result.State != value.State
		result.State = value.State
		if value.State == "approved" {
			if err := h.SaveApproved(value); err != nil {
				return err
			}
			result.Prepared, result.RunnerID, result.WorkerID, result.MaxSlots, result.CredentialVersion, result.CredentialExpiresAt = true, value.RunnerID, value.WorkerID, value.MaxSlots, value.CredentialVersion, value.CredentialExpiresAt
		}
		if changed && options.Observe != nil {
			return options.Observe(result)
		}
		return nil
	}
	if record.RunnerID == "" {
		request := pp.PoolRunnerPairingProposalRequest{PoolID: record.PoolID, DeviceName: record.DeviceName, CredentialSHA256: record.CredentialSHA256, IdempotencyKey: record.ProposalOperationID}
		response, err := poolPairingPost(ctx, client, record, h.Token(), "proposal", request)
		if err != nil {
			return result, err
		}
		if response.State == "approved" {
			if err = verify(response); err != nil {
				return result, err
			}
		} else {
			if err = observe(response); err != nil {
				return result, err
			}
			if result.State == "rejected" || result.State == "expired" {
				return result, fmt.Errorf("Pool pairing is not approved")
			}
		}
	}
	request := pp.PoolRunnerPairingStatusRequest{PoolID: record.PoolID, IdempotencyKey: record.ProposalOperationID}
	for {
		response, err := poolPairingPost(ctx, client, record, h.Token(), "status", request)
		if err != nil {
			return result, err
		}
		if err = observe(response); err != nil {
			return result, err
		}
		switch result.State {
		case "approved":
			return result, nil
		case "rejected", "expired":
			return result, fmt.Errorf("Pool pairing is not approved")
		}
		if !wait(ctx, interval) {
			return result, fmt.Errorf("Pool pairing wait stopped; preserve original identity")
		}
	}
}

func poolPairingPost(ctx context.Context, client *http.Client, identity PoolRunnerIdentity, token, action string, body any) (pp.PoolRunnerPairing, error) {
	var value pp.PoolRunnerPairing
	raw, err := json.Marshal(body)
	if err != nil {
		return value, fmt.Errorf("Pool pairing request could not be encoded")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, identity.ControlURL+"/api/v1/pool-runner-pairings/"+identity.PairingID+"/"+action, bytes.NewReader(raw))
	if err != nil {
		return value, fmt.Errorf("Pool pairing request unavailable")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(req)
	if err != nil {
		return value, fmt.Errorf("Pool pairing response unconfirmed; preserve original identity")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return value, fmt.Errorf("Pool pairing request rejected: HTTP %d", response.StatusCode)
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || strictjson.Decode(raw, &value, 64<<10) != nil {
		return value, fmt.Errorf("Pool pairing response incomplete or invalid")
	}
	return value, nil
}
func validatePoolPairingObservation(v pp.PoolRunnerPairing, identity PoolRunnerIdentity) error {
	invalid := fmt.Errorf("Pool pairing response differs from original identity")
	if v.ID != identity.PairingID || v.Purpose != poolRunnerPurpose || v.PoolID != identity.PoolID || v.ControlURL != identity.ControlURL || !desktopManagedHex(v.OwnerTenantID, 32) || v.PoolVersion < 1 || v.Version < 1 || v.CanApprove || v.MaxSlots < 1 || v.MaxSlots > 128 || v.TTLSeconds < 3600 || v.TTLSeconds > 30*24*3600 || !localToolSafeText(v.Name, 200) || strings.TrimSpace(v.Name) != v.Name || v.Name == "" || v.ReasonCode != "" {
		return invalid
	}
	created, e := time.Parse(time.RFC3339Nano, v.CreatedAt)
	expires, x := time.Parse(time.RFC3339Nano, v.ExpiresAt)
	if e != nil || x != nil || !expires.After(created) || expires.Sub(created) > 10*time.Minute {
		return invalid
	}
	switch v.State {
	case "waiting", "proposed", "approved", "rejected", "expired":
	default:
		return invalid
	}
	if v.DeviceName != "" && v.DeviceName != identity.DeviceName || v.CredentialSHA256 != "" && v.CredentialSHA256 != identity.CredentialSHA256 {
		return invalid
	}
	if (v.State == "proposed" || v.State == "approved") && (v.DeviceName != identity.DeviceName || v.CredentialSHA256 != identity.CredentialSHA256) {
		return invalid
	}
	if v.State == "approved" {
		if !desktopManagedHex(v.RunnerID, 32) || v.WorkerID != "pool-"+v.RunnerID || v.CredentialVersion < 1 {
			return invalid
		}
		if _, err := time.Parse(time.RFC3339Nano, v.CredentialExpiresAt); err != nil {
			return invalid
		}
		if identity.RunnerID != "" && (identity.OwnerTenantID != v.OwnerTenantID || identity.RunnerID != v.RunnerID || identity.WorkerID != v.WorkerID || identity.CredentialVersion != v.CredentialVersion || identity.CredentialExpiresAt != v.CredentialExpiresAt || identity.MaxSlots != v.MaxSlots) {
			return invalid
		}
	} else if v.RunnerID != "" || v.WorkerID != "" || v.CredentialVersion != 0 || v.CredentialExpiresAt != "" {
		return invalid
	}
	return nil
}
func samePoolPairingBasis(a, b pp.PoolRunnerPairing) bool {
	return a.ID == b.ID && a.Purpose == b.Purpose && a.OwnerTenantID == b.OwnerTenantID && a.PoolID == b.PoolID && a.PoolVersion == b.PoolVersion && a.ControlURL == b.ControlURL && a.Name == b.Name && a.MaxSlots == b.MaxSlots && a.TTLSeconds == b.TTLSeconds && a.CreatedAt == b.CreatedAt && a.ExpiresAt == b.ExpiresAt
}
