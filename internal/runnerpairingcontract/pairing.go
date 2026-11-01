// Package runnerpairingcontract defines the one remote enrollment wire contract.
// A pairing locator permits proposing a device, never issuing credentials or
// running tasks. Only current browser administration approves the fixed digest.
package runnerpairingcontract

type RunnerPairing struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Version    int    `json:"version"`
	State      string `json:"state"`
	ControlURL string `json:"control_url"`
	CreatedAt  string `json:"created_at"`
	ExpiresAt  string `json:"expires_at"`
	CanApprove bool   `json:"can_approve"`
	// ReasonCode describes this browser read, never stored approval authority.
	// Machine responses omit it; an approved identity is not an online worker.
	ReasonCode          string `json:"reason_code,omitempty"`
	DeviceName          string `json:"device_name,omitempty"`
	CredentialSHA256    string `json:"credential_sha256,omitempty"`
	WorkerID            string `json:"worker_id,omitempty"`
	CredentialExpiresAt string `json:"credential_expires_at,omitempty"`
}

// RunnerPairingPage is a live, currently authorized project collection. A cursor
// seeks after its last item; AsOf describes this read, not a frozen snapshot.
type RunnerPairingPage struct {
	ProjectID  string          `json:"project_id"`
	Status     string          `json:"status"`
	Q          string          `json:"q"`
	Items      []RunnerPairing `json:"items"`
	Total      int64           `json:"total"`
	Limit      int             `json:"limit"`
	NextCursor string          `json:"next_cursor,omitempty"`
	AsOf       string          `json:"as_of"`
}

type RunnerPairingCreateRequest struct {
	ProjectID      string `json:"project_id"`
	IdempotencyKey string `json:"idempotency_key"`
}

type RunnerPairingCommandRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
	Action          string `json:"action"`
}

// Proposal and status use the machine-held token in the Authorization header.
// The service stores only its digest; no browser or HTTP response gets a token.
type RunnerPairingProposalRequest struct {
	ProjectID        string `json:"project_id"`
	DeviceName       string `json:"device_name"`
	CredentialSHA256 string `json:"credential_sha256"`
	IdempotencyKey   string `json:"idempotency_key"`
}

type RunnerPairingStatusRequest struct {
	ProjectID      string `json:"project_id"`
	IdempotencyKey string `json:"idempotency_key"`
}
