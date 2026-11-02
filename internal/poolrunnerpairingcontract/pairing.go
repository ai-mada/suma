// Package poolrunnerpairingcontract defines the single wire contract for
// organization-owned Pool machine pairing. Project Runner identities are a
// separate purpose and cannot be exchanged for a Pool credential.
package poolrunnerpairingcontract

type PoolRunnerPairing struct {
	ID                  string `json:"id"`
	Purpose             string `json:"purpose"`
	OwnerTenantID       string `json:"owner_tenant_id"`
	PoolID              string `json:"pool_id"`
	PoolVersion         int    `json:"pool_version"`
	Version             int    `json:"version"`
	State               string `json:"state"`
	ControlURL          string `json:"control_url"`
	Name                string `json:"name"`
	MaxSlots            int    `json:"max_slots"`
	TTLSeconds          int    `json:"ttl_seconds"`
	DeviceName          string `json:"device_name,omitempty"`
	CredentialSHA256    string `json:"credential_sha256,omitempty"`
	RunnerID            string `json:"runner_id,omitempty"`
	WorkerID            string `json:"worker_id,omitempty"`
	CredentialVersion   int    `json:"credential_version,omitempty"`
	CredentialExpiresAt string `json:"credential_expires_at,omitempty"`
	CreatedAt           string `json:"created_at"`
	ExpiresAt           string `json:"expires_at"`
	CanApprove          bool   `json:"can_approve"`
	ReasonCode          string `json:"reason_code,omitempty"`
}

type PoolRunnerPairingPage struct {
	OwnerTenantID string              `json:"owner_tenant_id"`
	PoolID        string              `json:"pool_id"`
	AsOf          string              `json:"as_of"`
	Items         []PoolRunnerPairing `json:"items"`
	Total         int                 `json:"total"`
	NextCursor    string              `json:"next_cursor,omitempty"`
}

type PoolRunnerPairingCreateRequest struct {
	PoolID         string `json:"pool_id"`
	Name           string `json:"name"`
	MaxSlots       int    `json:"max_slots"`
	TTLSeconds     int    `json:"ttl_seconds"`
	IdempotencyKey string `json:"idempotency_key"`
}

type PoolRunnerPairingCommandRequest struct {
	PoolID          string `json:"pool_id"`
	Action          string `json:"action"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type PoolRunnerPairingProposalRequest struct {
	PoolID           string `json:"pool_id"`
	DeviceName       string `json:"device_name"`
	CredentialSHA256 string `json:"credential_sha256"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type PoolRunnerPairingStatusRequest struct {
	PoolID         string `json:"pool_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
