// Package toolpreparationcontract defines explicit, bounded configuration
// preparation on one authenticated maintenance owner. Configuration acceptance,
// inventory acknowledgement and business execution are separate facts.
package toolpreparationcontract

import (
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

const Protocol = "suma-managed-tool-preparation-v1"
const StopProtocol = "suma-managed-tool-preparation-stop-v1"
const PendingTTLSeconds = 300
const ClaimLeaseSeconds = 60
const MaxProjectRequests = 10000
const MaxNativeProcesses = 2048
const MaxCodexTemplateIDs = 8

// AdapterIDs is the implemented remote factory set, not the full tool catalogue.
func AdapterIDs() []string {
	return []string{"qoder-acp", "cursor-cli", "codex-jsonl", "codebuddy-cli", "dsh-multica", "claude-code-cli", "codex-app-server"}
}

type ManagedToolPreparationCapability struct {
	Protocol         string   `json:"protocol"`
	AdapterIDs       []string `json:"adapter_ids"`
	GenerationSHA256 string   `json:"generation_sha256"`
	CodexTemplateIDs []string `json:"codex_template_ids,omitempty"`
}

// Configuration carries names and the chosen model; never a credential value,
// executable, argument vector or destination path. Factories remain typed.
type ManagedToolPreparationConfiguration struct {
	Model            string `json:"model"`
	CredentialEnvRef string `json:"credential_env_ref"`
	TemplateID       string `json:"template_id,omitempty"`
}
type ManagedToolPreparationReference struct {
	PreparationID string `json:"preparation_id"`
	ClaimID       string `json:"claim_id"`
	Fence         int64  `json:"fence"`
}
type ManagedToolPreparationStopProof struct {
	Protocol              string                          `json:"protocol"`
	Preparation           ManagedToolPreparationReference `json:"preparation"`
	SourceSHA256          string                          `json:"source_sha256"`
	ReceiptSHA256         string                          `json:"receipt_sha256"`
	ProbeReturned         bool                            `json:"probe_returned"`
	AllNativeTreesStopped bool                            `json:"all_native_trees_stopped"`
	NativeProcessCount    int                             `json:"native_process_count"`
	StoppedProcessCount   int                             `json:"stopped_process_count"`
	ObservedAt            string                          `json:"observed_at"`
}

// Artifact bytes, the original complete Profile and the accepted generation
// have independent digests. A matching file hash does not prove acceptance.
type ManagedToolPreparationGenerationReceipt struct {
	ParentGenerationSHA256 string                           `json:"parent_generation_sha256"`
	GenerationSHA256       string                           `json:"generation_sha256"`
	CommandID              string                           `json:"command_id"`
	ArtifactSHA256         string                           `json:"artifact_sha256"`
	LocalReceiptSHA256     string                           `json:"local_receipt_sha256"`
	Profile                tc.Profile                       `json:"profile"`
	StopProof              *ManagedToolPreparationStopProof `json:"stop_proof" wire:"nonnull"`
}
type ManagedToolPreparation struct {
	ID                       string                                   `json:"id"`
	TenantID                 string                                   `json:"tenant_id"`
	ProjectID                string                                   `json:"project_id"`
	WorkerID                 string                                   `json:"worker_id"`
	Version                  int                                      `json:"version"`
	State                    string                                   `json:"state"`
	AdapterID                string                                   `json:"adapter_id"`
	Configuration            ManagedToolPreparationConfiguration      `json:"configuration"`
	InstanceID               string                                   `json:"instance_id"`
	InstallationScanID       string                                   `json:"installation_scan_id"`
	InstallationScanVersion  int                                      `json:"installation_scan_version"`
	InstallationReference    ii.Reference                             `json:"installation_reference"`
	InstallationSourceSHA256 string                                   `json:"installation_source_sha256"`
	InstallationItem         ii.Item                                  `json:"installation_item"`
	InstallationStopProof    *ii.StopProof                            `json:"installation_stop_proof" wire:"nonnull"`
	ExpectedGenerationSHA256 string                                   `json:"expected_generation_sha256"`
	RequestedBy              string                                   `json:"requested_by"`
	CreatedAt                string                                   `json:"created_at"`
	ExpiresAt                string                                   `json:"expires_at"`
	SourceSHA256             string                                   `json:"source_sha256"`
	Fence                    int64                                    `json:"fence"`
	ClaimID                  string                                   `json:"claim_id,omitempty"`
	ClaimedAt                string                                   `json:"claimed_at,omitempty"`
	LeaseUntil               string                                   `json:"lease_until,omitempty"`
	FinishedAt               string                                   `json:"finished_at,omitempty"`
	ResultCode               string                                   `json:"result_code,omitempty"`
	Receipt                  *ManagedToolPreparationGenerationReceipt `json:"receipt,omitempty" wire:"nonnull"`
	StopProof                *ManagedToolPreparationStopProof         `json:"stop_proof,omitempty" wire:"nonnull"`
	InventoryOperationID     string                                   `json:"inventory_operation_id,omitempty"`
	InventoryObservedAt      string                                   `json:"inventory_observed_at,omitempty"`
	SourceChanged            bool                                     `json:"source_changed,omitempty"`
}
type ManagedToolPreparationCreateRequest struct {
	ProjectID                     string                              `json:"project_id"`
	WorkerID                      string                              `json:"worker_id"`
	InstallationScanID            string                              `json:"installation_scan_id"`
	ExpectedScanVersion           int                                 `json:"expected_scan_version"`
	AdapterID                     string                              `json:"adapter_id"`
	Configuration                 ManagedToolPreparationConfiguration `json:"configuration"`
	ExpectedGenerationSHA256      string                              `json:"expected_generation_sha256"`
	AcknowledgeConfigurationWrite bool                                `json:"acknowledge_configuration_write"`
	IdempotencyKey                string                              `json:"idempotency_key"`
}
type ManagedToolPreparationClaimRequest struct {
	ProjectID string `json:"project_id"`
	WorkerID  string `json:"worker_id"`
	ClaimID   string `json:"claim_id"`
}
type PoolManagedToolPreparationClaimRequest struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	RunnerID  string `json:"runner_id"`
	ClaimID   string `json:"claim_id"`
}

// Canonical identity is returned even for a null claim, allowing restart scope
// validation without guessing the tenant from a local environment or browser.
type ManagedToolPreparationClaim struct {
	TenantID    string                  `json:"tenant_id"`
	ProjectID   string                  `json:"project_id"`
	WorkerID    string                  `json:"worker_id"`
	Preparation *ManagedToolPreparation `json:"preparation"`
}
type ManagedToolPreparationResultRequest struct {
	ProjectID      string                                   `json:"project_id"`
	WorkerID       string                                   `json:"worker_id"`
	Preparation    ManagedToolPreparationReference          `json:"preparation"`
	SourceSHA256   string                                   `json:"source_sha256"`
	IdempotencyKey string                                   `json:"idempotency_key"`
	Outcome        string                                   `json:"outcome"`
	FailureCode    string                                   `json:"failure_code"`
	Receipt        *ManagedToolPreparationGenerationReceipt `json:"receipt,omitempty" wire:"nonnull"`
	StopProof      *ManagedToolPreparationStopProof         `json:"stop_proof,omitempty" wire:"nonnull"`
}
type PoolManagedToolPreparationResultRequest struct {
	TenantID       string                                   `json:"tenant_id"`
	ProjectID      string                                   `json:"project_id"`
	RunnerID       string                                   `json:"runner_id"`
	Preparation    ManagedToolPreparationReference          `json:"preparation"`
	SourceSHA256   string                                   `json:"source_sha256"`
	IdempotencyKey string                                   `json:"idempotency_key"`
	Outcome        string                                   `json:"outcome"`
	FailureCode    string                                   `json:"failure_code"`
	Receipt        *ManagedToolPreparationGenerationReceipt `json:"receipt,omitempty" wire:"nonnull"`
	StopProof      *ManagedToolPreparationStopProof         `json:"stop_proof,omitempty" wire:"nonnull"`
}
type ManagedToolPreparationInventoryBinding struct {
	Reference          ManagedToolPreparationReference `json:"reference"`
	SourceSHA256       string                          `json:"source_sha256"`
	GenerationSHA256   string                          `json:"generation_sha256"`
	LocalReceiptSHA256 string                          `json:"local_receipt_sha256"`
}

type Capability = ManagedToolPreparationCapability
type Configuration = ManagedToolPreparationConfiguration
type Reference = ManagedToolPreparationReference
type StopProof = ManagedToolPreparationStopProof
type GenerationReceipt = ManagedToolPreparationGenerationReceipt
type Operation = ManagedToolPreparation
type CreateRequest = ManagedToolPreparationCreateRequest
type ClaimRequest = ManagedToolPreparationClaimRequest
type PoolClaimRequest = PoolManagedToolPreparationClaimRequest
type Claim = ManagedToolPreparationClaim
type ClaimReply = ManagedToolPreparationClaim
type ResultRequest = ManagedToolPreparationResultRequest
type PoolResultRequest = PoolManagedToolPreparationResultRequest
type InventoryBinding = ManagedToolPreparationInventoryBinding
