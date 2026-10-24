package dag

type PoolPolicyInput struct {
	ConcurrencyLimit int64             `json:"concurrency_limit"`
	ResourceLimits   map[string]*int64 `json:"resource_limits"`
	BudgetLimit      *int64            `json:"budget_limit"`
	TenantWeights    map[string]int    `json:"tenant_weights"`
}
type PoolPolicy struct {
	PoolID  string `json:"pool_id"`
	Version int    `json:"version"`
	PoolPolicyInput
	BudgetSpent int64              `json:"budget_spent"`
	Resources   []ResourceSnapshot `json:"resources"`
	AsOf        string             `json:"as_of"`
}
type PoolScope struct {
	PoolID        string `json:"pool_id"`
	TenantID      string `json:"tenant_id"`
	ProjectID     string `json:"project_id"`
	GrantID       string `json:"grant_id"`
	GrantVersion  int    `json:"grant_version"`
	ProjectWeight int    `json:"project_weight"`
	Status        string `json:"status"`
}
type ProjectExecutionRouteInput struct {
	PoolID       string `json:"pool_id"`
	GrantID      string `json:"grant_id"`
	GrantVersion int    `json:"grant_version"`
}
type ProjectExecutionRoute struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	Version   int    `json:"version"`
	ProjectExecutionRouteInput
}
type PoolCapabilityScope struct {
	TenantID     string   `json:"tenant_id"`
	ProjectID    string   `json:"project_id"`
	Capabilities []string `json:"capabilities"`
}
type PoolClaimRequest struct {
	PoolID      string                `json:"pool_id"`
	RunnerID    string                `json:"runner_id"`
	WorkerID    string                `json:"worker_id"`
	SlotID      string                `json:"slot_id"`
	OperationID string                `json:"operation_id"`
	Scopes      []PoolCapabilityScope `json:"scopes"`
}
type PoolClaimAuthority struct {
	CredentialID      string      `json:"credential_id"`
	CredentialVersion int         `json:"credential_version"`
	MaxSlots          int         `json:"max_slots"`
	PoolActive        bool        `json:"pool_active"`
	Scopes            []PoolScope `json:"scopes"`
}
