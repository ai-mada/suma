package platform

import "github.com/ai-mada/suma/internal/dag"

// ExecutionPoolPolicyInput configures admission units, never measured hardware.
type ExecutionPoolPolicyInput struct {
	ConcurrencyLimit int64             `json:"concurrency_limit"`
	ResourceLimits   map[string]*int64 `json:"resource_limits"`
	BudgetLimit      *int64            `json:"budget_limit"`
	TenantWeights    map[string]int    `json:"tenant_weights"`
}

type ExecutionPoolPolicy struct {
	ExecutionPoolPolicyInput
	Version     int                    `json:"version"`
	BudgetSpent int64                  `json:"budget_spent"`
	Resources   []dag.ResourceSnapshot `json:"resources"`
	Fairness    SchedulingFairness     `json:"fairness"`
	AsOf        string                 `json:"as_of"`
}

type ExecutionPool struct {
	ID             string              `json:"id"`
	OwnerTenantID  string              `json:"owner_tenant_id"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	Version        int                 `json:"version"`
	Status         string              `json:"status"`
	Policy         ExecutionPoolPolicy `json:"policy"`
	AllowedActions []string            `json:"allowed_actions"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
}

type ExecutionPoolPage struct {
	Items          []ExecutionPool `json:"items"`
	Total          int             `json:"total"`
	NextCursor     string          `json:"next_cursor,omitempty"`
	AllowedActions []string        `json:"allowed_actions"`
}

type ExecutionPoolCreateRequest struct {
	Name           string                   `json:"name"`
	Description    string                   `json:"description"`
	Policy         ExecutionPoolPolicyInput `json:"policy"`
	IdempotencyKey string                   `json:"idempotency_key"`
}

type ExecutionPoolCommandRequest struct {
	Action          string `json:"action"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// Grant metadata is deliberately limited to the project-approved shared label.
type ExecutionPoolGrant struct {
	ID                string   `json:"id"`
	PoolID            string   `json:"pool_id"`
	PoolName          string   `json:"pool_name"`
	OwnerTenantID     string   `json:"owner_tenant_id"`
	TenantID          string   `json:"tenant_id"`
	ProjectID         string   `json:"project_id"`
	SharedProjectName string   `json:"shared_project_name"`
	Version           int      `json:"version"`
	Status            string   `json:"status"`
	ProjectWeight     int      `json:"project_weight"`
	RequestSHA256     string   `json:"request_sha256"`
	RequestedBy       string   `json:"requested_by"`
	DecidedBy         string   `json:"decided_by,omitempty"`
	AllowedActions    []string `json:"allowed_actions"`
	CanOpenProject    bool     `json:"can_open_project"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type ExecutionPoolGrantPage struct {
	Items          []ExecutionPoolGrant `json:"items"`
	Total          int                  `json:"total"`
	NextCursor     string               `json:"next_cursor,omitempty"`
	AllowedActions []string             `json:"allowed_actions"`
}

type ExecutionPoolGrantRequest struct {
	ProjectID      string `json:"project_id"`
	PoolID         string `json:"pool_id"`
	ProjectWeight  int    `json:"project_weight"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ExecutionPoolGrantCommand struct {
	ProjectID       string `json:"project_id,omitempty"`
	Action          string `json:"action"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type ExecutionPoolRunner struct {
	ID                  string                            `json:"id"`
	PoolID              string                            `json:"pool_id"`
	WorkerID            string                            `json:"worker_id"`
	Name                string                            `json:"name"`
	Version             int                               `json:"version"`
	Status              string                            `json:"status"`
	MaxSlots            int                               `json:"max_slots"`
	CredentialVersion   int                               `json:"credential_version"`
	CredentialExpiresAt string                            `json:"credential_expires_at"`
	LastSeenAt          string                            `json:"last_seen_at,omitempty"`
	AllowedActions      []string                          `json:"allowed_actions"`
	CreatedAt           string                            `json:"created_at"`
	SlotOccupancy       *ExecutionPoolRunnerSlotOccupancy `json:"slot_occupancy,omitempty"`
}

// ExecutionPoolRunnerSlotOccupancy is present on the authorized pool runner
// page only. These persisted counts do not establish machine liveness.
type ExecutionPoolRunnerSlotOccupancy struct {
	ObservedAt  string `json:"observed_at"`
	Held        int    `json:"held"`
	Running     int    `json:"running"`
	Reconciling int    `json:"reconciling"`
	Unknown     int    `json:"unknown"`
}

type ExecutionPoolRunnerPage struct {
	Items          []ExecutionPoolRunner `json:"items"`
	Total          int                   `json:"total"`
	NextCursor     string                `json:"next_cursor,omitempty"`
	AllowedActions []string              `json:"allowed_actions"`
}

type ExecutionPoolRunnerCreateRequest struct {
	Name           string `json:"name"`
	MaxSlots       int    `json:"max_slots"`
	TTLSeconds     int    `json:"ttl_seconds"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ExecutionPoolCredentialRequest struct {
	ExpectedVersion int    `json:"expected_version"`
	TTLSeconds      int    `json:"ttl_seconds"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// Token is returned once, excluded from every persisted operation receipt.
type ExecutionPoolCredentialResponse struct {
	Runner         ExecutionPoolRunner `json:"runner"`
	Token          string              `json:"token,omitempty"`
	Replayed       bool                `json:"replayed"`
	RecoveryAction string              `json:"recovery_action,omitempty"`
}

type ExecutionPoolRunnerSession struct {
	Runner     ExecutionPoolRunner `json:"runner"`
	PoolActive bool                `json:"pool_active"`
	Scopes     []dag.PoolScope     `json:"scopes"`
}

// Project members may choose a currently authorized execution identity without
// gaining access to the owning organization's credential administration.
type ProjectPoolRunner struct {
	ID                  string `json:"id"`
	WorkerID            string `json:"worker_id"`
	PoolID              string `json:"pool_id"`
	Name                string `json:"name"`
	Status              string `json:"status"`
	CredentialExpiresAt string `json:"credential_expires_at"`
	LastSeenAt          string `json:"last_seen_at,omitempty"`
}
type ProjectPoolRunnerPage struct {
	Items []ProjectPoolRunner `json:"items"`
	AsOf  string              `json:"as_of"`
}

type ExecutionPoolPolicyPreviewRequest struct {
	ExpectedVersion int                      `json:"expected_version"`
	Policy          ExecutionPoolPolicyInput `json:"policy"`
	IdempotencyKey  string                   `json:"idempotency_key"`
}

type ExecutionPoolWeightChange struct {
	TenantID string `json:"tenant_id"`
	Before   int    `json:"before"`
	After    int    `json:"after"`
}

type ExecutionPoolPreviewImpact struct {
	ReadyNodes     int    `json:"ready_nodes"`
	ActiveAttempts int    `json:"active_attempts"`
	AsOf           string `json:"as_of"`
}

// Preview distinguishes metadata, authorization and policy generations. The
// changing occupancy is an observed impact, not an execution reservation.
type ExecutionPoolPolicyPreview struct {
	ID                   string                      `json:"id"`
	PoolID               string                      `json:"pool_id"`
	Version              int                         `json:"version"`
	State                string                      `json:"state"`
	PolicyVersion        int                         `json:"policy_version"`
	PoolVersion          int                         `json:"pool_version"`
	PayloadSHA256        string                      `json:"payload_sha256"`
	Before               ExecutionPoolPolicy         `json:"before"`
	Policy               ExecutionPoolPolicyInput    `json:"policy"`
	Changes              []SchedulingPolicyChange    `json:"changes"`
	WeightChanges        []ExecutionPoolWeightChange `json:"weight_changes"`
	Impact               ExecutionPoolPreviewImpact  `json:"impact"`
	CreatedBy            string                      `json:"created_by"`
	CreatedAt            string                      `json:"created_at"`
	ExpiresAt            string                      `json:"expires_at"`
	AppliedPolicyVersion int                         `json:"applied_policy_version"`
	AllowedActions       []string                    `json:"allowed_actions"`
	Valid                bool                        `json:"valid"`
	Reason               string                      `json:"reason,omitempty"`
}

type ExecutionPreviewCommitRequest struct {
	ProjectID       string `json:"project_id,omitempty"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type ExecutionRouteInput struct {
	Mode         string `json:"mode"`
	PoolID       string `json:"pool_id,omitempty"`
	GrantID      string `json:"grant_id,omitempty"`
	GrantVersion int    `json:"grant_version,omitempty"`
}

type ExecutionRouteView struct {
	dag.ProjectExecutionRoute
	Mode           string   `json:"mode"`
	PoolName       string   `json:"pool_name,omitempty"`
	GrantState     string   `json:"grant_state,omitempty"`
	AllowedActions []string `json:"allowed_actions"`
	AsOf           string   `json:"as_of"`
}

type ExecutionRoutePreviewRequest struct {
	ProjectID       string              `json:"project_id"`
	ExpectedVersion int                 `json:"expected_version"`
	Route           ExecutionRouteInput `json:"route"`
	IdempotencyKey  string              `json:"idempotency_key"`
}

type ExecutionRoutePreviewImpact struct {
	ExistingRuns int    `json:"existing_runs"`
	AsOf         string `json:"as_of"`
}

type ExecutionRoutePreview struct {
	ID                  string                      `json:"id"`
	ProjectID           string                      `json:"project_id"`
	Version             int                         `json:"version"`
	State               string                      `json:"state"`
	RouteVersion        int                         `json:"route_version"`
	PayloadSHA256       string                      `json:"payload_sha256"`
	Before              ExecutionRouteView          `json:"before"`
	Route               ExecutionRouteInput         `json:"route"`
	PoolName            string                      `json:"pool_name,omitempty"`
	PoolStatus          string                      `json:"pool_status,omitempty"`
	PoolVersion         int                         `json:"pool_version"`
	PoolPolicyVersion   int                         `json:"pool_policy_version"`
	Impact              ExecutionRoutePreviewImpact `json:"impact"`
	CreatedBy           string                      `json:"created_by"`
	CreatedAt           string                      `json:"created_at"`
	ExpiresAt           string                      `json:"expires_at"`
	AppliedRouteVersion int                         `json:"applied_route_version"`
	AllowedActions      []string                    `json:"allowed_actions"`
	Valid               bool                        `json:"valid"`
	Reason              string                      `json:"reason,omitempty"`
}
