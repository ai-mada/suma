package toolinspectioncontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

const MaintenanceProtocol = "suma-tool-maintenance-v1"
const ProbeStopProtocol = "suma-tool-probe-stop-v1"
const RecoveryTTLSeconds = 300
const MaxNativeProcesses = 2048

// A continuous inventory owner declares its protocol, not tool health or proof
// that an earlier native process stopped. One-off report-tools must omit it.
type ToolMaintenanceCapability struct {
	Protocol           string          `json:"protocol"`
	InstanceID         string          `json:"instance_id"`
	InstalledDiscovery *tii.Capability `json:"installed_discovery,omitempty" wire:"nonnull"`
}
type ToolMaintenanceStatus struct {
	State              string          `json:"state"`
	CheckedAt          string          `json:"checked_at"`
	Protocol           string          `json:"protocol,omitempty"`
	InstanceID         string          `json:"instance_id,omitempty"`
	ObservedAt         string          `json:"observed_at,omitempty"`
	ExpiresAt          string          `json:"expires_at,omitempty"`
	InstalledDiscovery *tii.Capability `json:"installed_discovery,omitempty" wire:"nonnull"`
}
type ToolInspectionStopProof struct {
	Protocol              string    `json:"protocol"`
	Inspection            Reference `json:"inspection"`
	SourceSHA256          string    `json:"source_sha256"`
	ReceiptSHA256         string    `json:"receipt_sha256"`
	ProbeReturned         bool      `json:"probe_returned"`
	AllNativeTreesStopped bool      `json:"all_native_trees_stopped"`
	NoPendingInventory    bool      `json:"no_pending_inventory"`
	NativeProcessCount    int       `json:"native_process_count"`
	StoppedProcessCount   int       `json:"stopped_process_count"`
	ObservedAt            string    `json:"observed_at"`
}
type ToolInspectionRecovery struct {
	ID                string                   `json:"id"`
	ProjectID         string                   `json:"project_id"`
	WorkerID          string                   `json:"worker_id"`
	Inspection        Reference                `json:"inspection"`
	InspectionVersion int                      `json:"inspection_version"`
	SourceSHA256      string                   `json:"source_sha256"`
	Version           int                      `json:"version"`
	State             string                   `json:"state"`
	RequestedBy       string                   `json:"requested_by"`
	CreatedAt         string                   `json:"created_at"`
	ExpiresAt         string                   `json:"expires_at"`
	Proof             *ToolInspectionStopProof `json:"proof,omitempty" wire:"nonnull"`
	ProofSHA256       string                   `json:"proof_sha256,omitempty"`
	ProofStoredAt     string                   `json:"proof_stored_at,omitempty"`
	ClosedAt          string                   `json:"closed_at,omitempty"`
}
type ToolInspectionRecoveryCreateRequest struct {
	ProjectID       string    `json:"project_id"`
	ExpectedVersion int       `json:"expected_version"`
	Inspection      Reference `json:"inspection"`
	IdempotencyKey  string    `json:"idempotency_key"`
}
type ToolInspectionRecoveryPollRequest struct {
	ProjectID  string    `json:"project_id"`
	WorkerID   string    `json:"worker_id"`
	Inspection Reference `json:"inspection"`
}
type PoolToolInspectionRecoveryPollRequest struct {
	TenantID   string    `json:"tenant_id"`
	ProjectID  string    `json:"project_id"`
	RunnerID   string    `json:"runner_id"`
	Inspection Reference `json:"inspection"`
}
type ToolInspectionRecoveryPoll struct {
	Recovery *ToolInspectionRecovery `json:"recovery"`
}
type ToolInspectionRecoveryProofRequest struct {
	ProjectID       string                  `json:"project_id"`
	WorkerID        string                  `json:"worker_id"`
	RecoveryID      string                  `json:"recovery_id"`
	ExpectedVersion int                     `json:"expected_version"`
	IdempotencyKey  string                  `json:"idempotency_key"`
	Proof           ToolInspectionStopProof `json:"proof"`
}
type PoolToolInspectionRecoveryProofRequest struct {
	TenantID        string                  `json:"tenant_id"`
	ProjectID       string                  `json:"project_id"`
	RunnerID        string                  `json:"runner_id"`
	RecoveryID      string                  `json:"recovery_id"`
	ExpectedVersion int                     `json:"expected_version"`
	IdempotencyKey  string                  `json:"idempotency_key"`
	Proof           ToolInspectionStopProof `json:"proof"`
}
type ToolInspectionRecoveryAcknowledgementRequest struct {
	ProjectID       string `json:"project_id"`
	WorkerID        string `json:"worker_id"`
	RecoveryID      string `json:"recovery_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
	ProofSHA256     string `json:"proof_sha256"`
}
type PoolToolInspectionRecoveryAcknowledgementRequest struct {
	TenantID        string `json:"tenant_id"`
	ProjectID       string `json:"project_id"`
	RunnerID        string `json:"runner_id"`
	RecoveryID      string `json:"recovery_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
	ProofSHA256     string `json:"proof_sha256"`
}
type ToolInspectionRecoveryList struct {
	Items []ToolInspectionRecovery `json:"items"`
}

var maintenanceProtocol = regexp.MustCompile(`^suma-tool-maintenance-v[1-9][0-9]{0,3}$`)

func ValidateMaintenanceCapability(v ToolMaintenanceCapability) error {
	if !maintenanceProtocol.MatchString(v.Protocol) || !Operation(v.InstanceID) || v.InstalledDiscovery != nil && (v.Protocol != MaintenanceProtocol || tii.ValidateCapability(*v.InstalledDiscovery) != nil) {
		return fmt.Errorf("invalid maintenance capability")
	}
	return nil
}

// Immutable original source is stable across unknown/closure/version updates
// and continuous-process restarts; an instance declaration is not old evidence.
func InspectionSourceSHA256(v Inspection) string {
	raw, _ := json.Marshal(struct {
		ID, ProjectID, WorkerID, ToolProfileID                                                      string
		ToolProfileVersion                                                                          int
		ProfileSHA256, CommandID, RequestedBy, CreatedAt, ExpiresAt, ClaimID, ClaimedAt, LeaseUntil string
		Fence                                                                                       int64
	}{v.ID, v.ProjectID, v.WorkerID, v.ToolProfileID, v.ToolProfileVersion, v.ProfileSHA256, v.CommandID, v.RequestedBy, v.CreatedAt, v.ExpiresAt, v.ClaimID, v.ClaimedAt, v.LeaseUntil, v.Fence})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func StopProofDigest(v ToolInspectionStopProof) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func ValidateStopProof(v ToolInspectionStopProof) error {
	if v.Protocol != ProbeStopProtocol || ValidateReference(v.Inspection) != nil || !digest.MatchString(v.SourceSHA256) || !digest.MatchString(v.ReceiptSHA256) || !v.ProbeReturned || !v.AllNativeTreesStopped || !v.NoPendingInventory || v.NativeProcessCount < 0 || v.NativeProcessCount > MaxNativeProcesses || v.StoppedProcessCount != v.NativeProcessCount {
		return fmt.Errorf("incomplete original native stop proof")
	}
	if _, err := time.Parse(time.RFC3339Nano, v.ObservedAt); err != nil {
		return fmt.Errorf("invalid stop proof time")
	}
	return nil
}
func ValidateRecovery(v ToolInspectionRecovery) error {
	if !ID(v.ID) || !ID(v.ProjectID) || !ID(v.WorkerID) || !ID(v.RequestedBy) || ValidateReference(v.Inspection) != nil || v.InspectionVersion < 1 || v.Version < 1 || !digest.MatchString(v.SourceSHA256) {
		return fmt.Errorf("invalid recovery source")
	}
	created, e := time.Parse(time.RFC3339Nano, v.CreatedAt)
	expires, x := time.Parse(time.RFC3339Nano, v.ExpiresAt)
	if e != nil || x != nil || !expires.After(created) {
		return fmt.Errorf("invalid recovery deadline")
	}
	if v.Proof != nil {
		if ValidateStopProof(*v.Proof) != nil || v.Proof.Inspection != v.Inspection || v.Proof.SourceSHA256 != v.SourceSHA256 || StopProofDigest(*v.Proof) != v.ProofSHA256 || v.ProofStoredAt == "" {
			return fmt.Errorf("invalid recovery proof")
		}
		if _, e := time.Parse(time.RFC3339Nano, v.ProofStoredAt); e != nil {
			return fmt.Errorf("invalid proof receipt time")
		}
	} else if v.ProofSHA256 != "" || v.ProofStoredAt != "" {
		return fmt.Errorf("proof digest without proof")
	}
	switch v.State {
	case "requested":
		if v.Proof != nil || v.ClosedAt != "" {
			return fmt.Errorf("requested recovery has result facts")
		}
	case "proof_stored":
		if v.Proof == nil || v.ClosedAt != "" {
			return fmt.Errorf("proof receipt is incomplete")
		}
	case "closed_unknown":
		if v.Proof == nil || v.ClosedAt == "" {
			return fmt.Errorf("closed recovery requires exact proof receipt")
		}
		if _, e := time.Parse(time.RFC3339Nano, v.ClosedAt); e != nil {
			return fmt.Errorf("invalid closure time")
		}
	case "expired":
		if v.ClosedAt != "" {
			return fmt.Errorf("expired recovery cannot close")
		}
	default:
		return fmt.Errorf("invalid recovery state")
	}
	return nil
}
