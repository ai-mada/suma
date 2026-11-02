// Package poolconnectioncontract defines the current Pool connection facts.
// A local check, a historical contact and current inventory provenance remain
// distinct; none of these types grants task execution or tool enablement.
package poolconnectioncontract

const PoolConnectionCheckProtocol = "suma-pool-connection-check-v1"

// PoolConnectionScope compares one explicitly configured scope with the
// current machine session. Other server grants are never added to local input.
type PoolConnectionScope struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	Granted   bool   `json:"granted"`
}

// PoolConnectionCheck is a one-shot authenticated observation. Its file digest
// covers original file bytes, not the resolved configuration used by a Journal.
// Checking never starts a Worker, writes a Journal or consumes an execution slot.
type PoolConnectionCheck struct {
	Protocol                string                `json:"protocol"`
	PoolID                  string                `json:"pool_id"`
	RunnerID                string                `json:"runner_id"`
	WorkerID                string                `json:"worker_id"`
	CredentialVersion       int                   `json:"credential_version"`
	PoolActive              bool                  `json:"pool_active"`
	ConfiguredSlots         int                   `json:"configured_slots"`
	CurrentMaxSlots         int                   `json:"current_max_slots"`
	ConfigurationFileSHA256 string                `json:"configuration_file_sha256"`
	CheckedAt               string                `json:"checked_at"`
	Scopes                  []PoolConnectionScope `json:"scopes"`
}

// PoolRunnerReadinessRunner shows current registration/credential metadata.
// HistoricalLastSeenAt is retained through rotations and is never online proof.
type PoolRunnerReadinessRunner struct {
	Status               string `json:"status"`
	Version              int    `json:"version"`
	CredentialVersion    int    `json:"credential_version"`
	CredentialState      string `json:"credential_state"`
	CredentialExpiresAt  string `json:"credential_expires_at"`
	HistoricalLastSeenAt string `json:"historical_last_seen_at"`
}

// PoolRunnerReadinessScope contains public Grant facts and an authorized,
// as-of projection of the original maintenance/inventory provenance checks.
// Project reads require a separate current project proof; Pool ownership alone
// only yields restricted route/source fields and no inventory timestamps/count.
type PoolRunnerReadinessScope struct {
	TenantID            string `json:"tenant_id"`
	ProjectID           string `json:"project_id"`
	GrantID             string `json:"grant_id"`
	GrantVersion        int    `json:"grant_version"`
	GrantStatus         string `json:"grant_status"`
	SharedProjectName   string `json:"shared_project_name"`
	CanOpenProject      bool   `json:"can_open_project"`
	RouteState          string `json:"route_state"`
	SourceCurrent       bool   `json:"source_current"`
	SourceReason        string `json:"source_reason"`
	InventoryObservedAt string `json:"inventory_observed_at"`
	InventoryExpiresAt  string `json:"inventory_expires_at"`
	InventoryItemCount  *int   `json:"inventory_item_count,omitempty"`
}

// PoolRunnerReadiness is a read-only snapshot at server CheckedAt. SourceCurrent
// means matching current authenticated maintenance/inventory reports, including
// an explicitly empty report. Tool capability, profile enablement, task gates
// and prepared-file identity are evaluated elsewhere through original domains.
type PoolRunnerReadiness struct {
	PoolID     string                    `json:"pool_id"`
	RunnerID   string                    `json:"runner_id"`
	WorkerID   string                    `json:"worker_id"`
	PoolStatus string                    `json:"pool_status"`
	CheckedAt  string                    `json:"checked_at"`
	Runner     PoolRunnerReadinessRunner `json:"runner"`
	Scope      PoolRunnerReadinessScope  `json:"scope"`
}
