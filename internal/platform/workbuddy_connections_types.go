package platform

import (
	"fmt"
	"io"

	"github.com/ai-mada/suma/internal/workbuddy"
)

// WorkBuddyConnections owns provider grants, never task execution or platform
// identity. All records remain in the control plane's authoritative database.
type WorkBuddyConnections struct {
	s          *Service
	app        workbuddy.OAuthConfig
	key        []byte
	appDigest  string
	configured bool
}

func (WorkBuddyConnections) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "platform.WorkBuddyConnections{credentials:redacted}")
}

// WorkBuddyConnection is a current-authorized, owner-scoped public projection.
// Connected means a provider grant was saved, not that execution is available.
type WorkBuddyConnection struct {
	ID              string   `json:"id"`
	ProjectID       string   `json:"project_id"`
	Version         int      `json:"version"`
	OwnerID         string   `json:"owner_id"`
	ClientID        string   `json:"client_id"`
	OpenID          string   `json:"open_id"`
	Scopes          []string `json:"scopes"`
	State           string   `json:"state"`
	GrantGeneration int      `json:"grant_generation"`
	TokenGeneration int      `json:"token_generation"`
	ExpiresAt       string   `json:"expires_at"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	LastError       string   `json:"last_error"`
	AllowedActions  []string `json:"allowed_actions"`
}

type WorkBuddyConnectionPage struct {
	ProjectID       string                `json:"project_id"`
	Configured      bool                  `json:"configured"`
	AvailableScopes []string              `json:"available_scopes"`
	CanAuthorize    bool                  `json:"can_authorize"`
	Items           []WorkBuddyConnection `json:"items"`
	Limit           int                   `json:"limit"`
	NextCursor      string                `json:"next_cursor"`
}

type WorkBuddyAuthorizationRequest struct {
	ProjectID       string   `json:"project_id"`
	ConnectionID    string   `json:"connection_id,omitempty"`
	ExpectedVersion int      `json:"expected_version,omitempty"`
	Scopes          []string `json:"scopes"`
	IdempotencyKey  string   `json:"idempotency_key"`
}

type WorkBuddyAuthorization struct {
	ID               string              `json:"id"`
	Version          int                 `json:"version"`
	Connection       WorkBuddyConnection `json:"connection"`
	AuthorizationURL string              `json:"authorization_url,omitempty"`
	ExpiresAt        string              `json:"expires_at"`
	State            string              `json:"state"`
}

// Complete is a specific OAuth protocol command. Code and state are consumed
// only in memory; receipt hashes and persisted attempts contain their digests.
type WorkBuddyAuthorizationComplete struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	State           string `json:"state"`
	Code            string `json:"code"`
	IdempotencyKey  string `json:"idempotency_key"`
}

func (WorkBuddyAuthorizationComplete) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "platform.WorkBuddyAuthorizationComplete{credentials:redacted}")
}

type WorkBuddyAuthorizationCancel struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	State           string `json:"state"`
	IdempotencyKey  string `json:"idempotency_key"`
}

func (WorkBuddyAuthorizationCancel) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "platform.WorkBuddyAuthorizationCancel{credentials:redacted}")
}

type WorkBuddyRefreshRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type WorkBuddyDisconnectRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type WorkBuddyCallbackResult struct {
	ProjectID    string `json:"project_id"`
	ConnectionID string `json:"connection_id"`
	State        string `json:"state"`
	RedirectURL  string `json:"redirect_url"`
}

// Only the isolated callback bootstrap sends this ephemeral request. These
// fields must never be included in generic write-action browser persistence.
type WorkBuddyCallbackRequest struct {
	State string `json:"state"`
	Code  string `json:"code,omitempty"`
	Error string `json:"error,omitempty"`
}

func (WorkBuddyCallbackRequest) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "platform.WorkBuddyCallbackRequest{credentials:redacted}")
}

type workBuddyConnectionRow struct {
	WorkBuddyConnection
	TenantID, AppDigest, Ciphertext, ActiveOperationID string
}

type workBuddyOAuthOperation struct {
	ID, TenantID, ProjectID, ConnectionID, OwnerID string
	Kind, State, RequestHash, CredentialDigest     string
	StateHash, CodeHash, AppDigest, ScopesJSON     string
	Version, ConnectionVersion, GrantGeneration    int
	TokenGeneration                                int
	ExpiresAt, CreatedAt, UpdatedAt, LastError     string
}

// These are safe diagnostic identifiers, never provider response text.
const (
	workBuddyRecordMaxVersion = 2147483647
	workBuddyInvalid          = "workbuddy_connection_invalid"
	workBuddyUnavailable      = "workbuddy_connection_unavailable"
	workBuddyVersion          = "workbuddy_connection_version"
	workBuddyReauthorize      = "workbuddy_reauthorization_required"
	workBuddyEffectUnknown    = "workbuddy_effect_unknown"
)

// Commands must reserve every version transition before a provider effect or
// before invalidating a usable grant. The same rule drives public actions.
func workBuddyVersionRoom(version, transitions int) bool {
	return version >= 1 && transitions >= 1 && transitions <= 3 && version <= workBuddyRecordMaxVersion-transitions
}
