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

	rp "github.com/ai-mada/suma/internal/runnerpairingcontract"
)

const RunnerPairingProtocol = "suma-runner-pairing-v1"

// Pairing prepares an identity only. Reporting inventory and running a Worker
// remain separate explicit operations through their original implementations.
type RunnerPairingOptions struct {
	PairingID, ControlURL, ProjectID, IdentityPath, DeviceName string
	Secrets                                                    RunnerSecretStore
	HTTP                                                       *http.Client
	Observe                                                    func(RunnerPairingResult) error
	pollInterval, waitLimit                                    time.Duration
}

type RunnerPairingResult struct {
	Protocol            string `json:"protocol"`
	State               string `json:"state"`
	Prepared            bool   `json:"prepared"`
	IdentityFile        string `json:"identity_file"`
	ProjectID           string `json:"project_id"`
	PairingID           string `json:"pairing_id"`
	CredentialSHA256    string `json:"credential_sha256"`
	WorkerID            string `json:"worker_id,omitempty"`
	CredentialExpiresAt string `json:"credential_expires_at,omitempty"`
}

func PairRemoteRunner(ctx context.Context, options RunnerPairingOptions) (RunnerPairingResult, error) {
	identity, err := PrepareRunnerPairingIdentity(options.IdentityPath, options.ControlURL, options.ProjectID, options.PairingID, options.DeviceName, options.Secrets)
	if err != nil {
		return RunnerPairingResult{}, err
	}
	defer identity.Close()
	record := identity.Identity()
	result := RunnerPairingResult{Protocol: RunnerPairingProtocol, IdentityFile: identity.Path(), ProjectID: record.ProjectID, PairingID: record.PairingID, CredentialSHA256: record.CredentialSHA256}
	interval := options.pollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	limit := options.waitLimit
	if limit <= 0 || limit > 10*time.Minute {
		limit = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	httpClient := pairingHTTPClient(options.HTTP)
	version := 0
	observe := func(observed rp.RunnerPairing) error {
		if err := validatePairingObservation(observed, record); err != nil {
			return err
		}
		if observed.Version < version {
			return fmt.Errorf("pairing observation version moved backwards")
		}
		version = observed.Version
		changed := result.State != observed.State
		result.State = observed.State
		if observed.State == "approved" {
			if err := identity.SaveApproved(observed.WorkerID, observed.CredentialExpiresAt); err != nil {
				return err
			}
			result.Prepared, result.WorkerID, result.CredentialExpiresAt = true, observed.WorkerID, observed.CredentialExpiresAt
		}
		if changed && options.Observe != nil {
			return options.Observe(result)
		}
		return nil
	}
	// An approved local record still requires a current status check. An unknown
	// proposal replays the exact persisted operation and digest, never a new token.
	if record.WorkerID == "" {
		proposal := rp.RunnerPairingProposalRequest{ProjectID: record.ProjectID, DeviceName: record.DeviceName, CredentialSHA256: record.CredentialSHA256, IdempotencyKey: record.ProposalOperationID}
		response, err := pairingPost(ctx, httpClient, record, identity.Token(), "proposal", proposal)
		if err != nil {
			return result, err
		}
		// Approval is accepted only from status, which rechecks the current Grant.
		if response.State == "approved" {
			if err = validatePairingObservation(response, record); err != nil {
				return result, err
			}
			version = response.Version
		} else {
			if err = observe(response); err != nil {
				return result, err
			}
			if result.State == "rejected" || result.State == "expired" {
				return result, fmt.Errorf("pairing request is %s", result.State)
			}
		}
	}
	request := rp.RunnerPairingStatusRequest{ProjectID: record.ProjectID, IdempotencyKey: record.ProposalOperationID}
	for {
		response, err := pairingPost(ctx, httpClient, record, identity.Token(), "status", request)
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
			return result, fmt.Errorf("pairing request is %s", result.State)
		}
		// The process deadline is monotonic. The server owns ticket/Grant expiry;
		// a machine's wall-clock skew must not manufacture renewed authority.
		if !wait(ctx, interval) {
			return result, fmt.Errorf("pairing wait stopped; preserve the original identity")
		}
	}
}

func pairingHTTPClient(source *http.Client) *http.Client {
	client := http.Client{Timeout: 15 * time.Second}
	if source != nil {
		client = *source
	}
	if client.Timeout <= 0 || client.Timeout > 15*time.Second {
		client.Timeout = 15 * time.Second
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &client
}

func pairingPost(ctx context.Context, client *http.Client, identity RunnerIdentity, token, action string, body any) (rp.RunnerPairing, error) {
	var result rp.RunnerPairing
	raw, err := json.Marshal(body)
	if err != nil {
		return result, fmt.Errorf("pairing request could not be encoded")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, identity.ControlURL+"/api/v1/runner-pairings/"+identity.PairingID+"/"+action, bytes.NewReader(raw))
	if err != nil {
		return result, fmt.Errorf("pairing request is unavailable")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return result, fmt.Errorf("pairing response is unconfirmed; preserve the original identity")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, fmt.Errorf("pairing request rejected: HTTP %d", response.StatusCode)
	}
	raw, err = io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(raw) > 64<<10 {
		return result, fmt.Errorf("pairing response exceeds its bound or is incomplete")
	}
	fields, err := decodeToolObject(raw)
	if err != nil {
		return result, fmt.Errorf("pairing response is not a strict object")
	}
	for _, name := range []string{"id", "project_id", "version", "state", "control_url", "created_at", "expires_at", "can_approve"} {
		if value, present := fields[name]; !present || value == nil {
			return result, fmt.Errorf("pairing response is missing required facts")
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil {
		return rp.RunnerPairing{}, fmt.Errorf("pairing response contains invalid fields")
	}
	return result, nil
}

func validatePairingObservation(value rp.RunnerPairing, identity RunnerIdentity) error {
	invalid := fmt.Errorf("pairing response differs from the original identity")
	if value.ID != identity.PairingID || value.ProjectID != identity.ProjectID || value.ControlURL != identity.ControlURL || value.Version < 1 || value.CanApprove {
		return invalid
	}
	created, err := time.Parse(time.RFC3339Nano, value.CreatedAt)
	if err != nil {
		return invalid
	}
	expires, err := time.Parse(time.RFC3339Nano, value.ExpiresAt)
	if err != nil || !expires.After(created) || expires.Sub(created) > 10*time.Minute {
		return invalid
	}
	switch value.State {
	case "waiting", "proposed", "approved", "rejected", "expired":
	default:
		return invalid
	}
	if value.DeviceName != "" && value.DeviceName != identity.DeviceName || value.CredentialSHA256 != "" && value.CredentialSHA256 != identity.CredentialSHA256 {
		return invalid
	}
	if value.State == "proposed" || value.State == "approved" {
		if value.DeviceName != identity.DeviceName || value.CredentialSHA256 != identity.CredentialSHA256 {
			return invalid
		}
	}
	if value.State == "approved" {
		if !strings.HasPrefix(value.WorkerID, "remote-") || !desktopManagedHex(strings.TrimPrefix(value.WorkerID, "remote-"), 32) {
			return invalid
		}
		if _, err = time.Parse(time.RFC3339Nano, value.CredentialExpiresAt); err != nil {
			return invalid
		}
		if identity.WorkerID != "" && (identity.WorkerID != value.WorkerID || identity.CredentialExpiresAt != value.CredentialExpiresAt) {
			return invalid
		}
	} else if value.WorkerID != "" || value.CredentialExpiresAt != "" {
		return invalid
	}
	return nil
}
