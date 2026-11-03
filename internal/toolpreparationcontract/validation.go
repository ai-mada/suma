package toolpreparationcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	ac "github.com/ai-mada/suma/internal/acpcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
	"slices"
	"strings"
	"time"
)

func ID(value string) bool               { return ii.ID(value) }
func ValidOperationID(value string) bool { return ii.Operation(value) }
func ValidDigest(value string) bool      { return tc.ValidDigest(value) }
func Digest(value any) string {
	raw, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Use the complete digest so every accepted operation owns a distinct command
// name without colliding with a preconfigured local-* command.
func CommandID(preparationID string) string {
	if !ID(preparationID) {
		return ""
	}
	return "managed-" + Digest(preparationID)
}
func AdapterRuntime(adapter string) string {
	switch adapter {
	case "qoder-acp":
		return "qoder"
	case "cursor-cli":
		return "cursor"
	case "codex-jsonl":
		return "codex"
	case "codebuddy-cli":
		return "codebuddy"
	case "dsh-multica":
		return "harness"
	case "claude-code-cli":
		return "claude"
	}
	return ""
}
func ValidateAdapterIDs(ids []string) error {
	if len(ids) < 1 || len(ids) > len(AdapterIDs()) {
		return fmt.Errorf("invalid managed preparation adapters")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if AdapterRuntime(id) == "" || seen[id] {
			return fmt.Errorf("invalid managed preparation adapter")
		}
		seen[id] = true
	}
	return nil
}
func ValidateCapability(v Capability) error {
	if v.Protocol != Protocol || !ValidDigest(v.GenerationSHA256) {
		return fmt.Errorf("invalid managed preparation capability")
	}
	return ValidateAdapterIDs(v.AdapterIDs)
}

// ConfigurationMode distinguishes actual factory input semantics. Native
// commands do not silently accept options that their reviewed builder ignores.
func ConfigurationMode(adapter string) string {
	switch adapter {
	case "qoder-acp", "cursor-cli":
		return "model_and_credential"
	case "claude-code-cli":
		return "fixed_claude_model"
	case "codex-jsonl", "codebuddy-cli", "dsh-multica":
		return "existing_local_configuration"
	}
	return ""
}

func ValidateConfiguration(adapter string, v Configuration) error {
	switch ConfigurationMode(adapter) {
	case "model_and_credential":
		if ac.ValidFixedText(v.Model, 256) && (v.CredentialEnvRef == "" || ac.ValidCredentialEnvRef(v.CredentialEnvRef)) && (adapter != "cursor-cli" || v.CredentialEnvRef != "") {
			return nil
		}
	case "fixed_claude_model":
		if tc.ValidClaudeCLIModel(v.Model) && v.CredentialEnvRef == "" {
			return nil
		}
	case "existing_local_configuration":
		if v.Model == "" && v.CredentialEnvRef == "" {
			return nil
		}
	}
	return fmt.Errorf("invalid explicit managed tool preparation configuration")
}

// ACP scans are pinned but intentionally not launch-ready. Reviewed generic
// scans carry an original command and require a workspace; neither is a login.
func CanPrepareItem(v ii.Item) bool {
	if ConfigurationMode(v.AdapterID) == "" || ii.ValidateItem(v) != nil || !v.Installed || !ac.ValidFixedText(v.Version, 256) || !ValidDigest(v.ExecutableSHA256) {
		return false
	}
	if ConfigurationMode(v.AdapterID) == "model_and_credential" {
		return !v.LaunchSupported && v.ReasonCode == "acp_configuration_required"
	}
	return v.LaunchSupported && v.ReasonCode == "workspace_required" && (v.AdapterID != "claude-code-cli" || tc.ValidateClaudeCLIVersion(v.Version) == nil)
}

// ProfileMatchesConfiguration verifies the exact reviewed public tuple and
// capability. Opaque source pins remain bound by the complete profile digest.
func ProfileMatchesConfiguration(adapter string, configuration Configuration, p tc.Profile) bool {
	if ValidateConfiguration(adapter, configuration) != nil || tc.ValidateProfile(p, true) != nil || p.Adapter != AdapterRuntime(adapter) || p.ReadOnly {
		return false
	}
	switch ConfigurationMode(adapter) {
	case "model_and_credential":
		return tc.SupportsACPExecutionProfile(p) && p.ACPExecution.Model == configuration.Model
	case "fixed_claude_model":
		return tc.SupportsClaudeExecutionProfile(p) && p.ClaudeExecution.Model == configuration.Model
	case "existing_local_configuration":
		protocol := map[string]string{"codex-jsonl": "codex-jsonl-v1", "codebuddy-cli": "codebuddy-jsonl-v1", "dsh-multica": "dsh-multica-jsonl-v1"}[adapter]
		plain := tc.Profile{CommandID: p.CommandID, Adapter: p.Adapter, Protocol: protocol, ExecutableSHA256: p.ExecutableSHA256, ToolVersion: p.ToolVersion, ReadOnly: false, ProfileSHA256: p.ProfileSHA256}
		return tc.EqualProfile(p, plain)
	}
	return false
}

func supportedGenerationProfile(p tc.Profile) bool {
	if tc.SupportsACPExecutionProfile(p) || tc.SupportsClaudeExecutionProfile(p) {
		return tc.ValidateProfile(p, true) == nil
	}
	for _, id := range []string{"codex-jsonl", "codebuddy-cli", "dsh-multica"} {
		if ProfileMatchesConfiguration(id, Configuration{}, p) {
			return true
		}
	}
	return false
}

func ValidateCreateRequest(v CreateRequest) error {
	if !ID(v.ProjectID) || !ID(v.WorkerID) || !ID(v.InstallationScanID) || v.ExpectedScanVersion < 1 || v.ExpectedScanVersion > 2147483646 || !ValidDigest(v.ExpectedGenerationSHA256) || !v.AcknowledgeConfigurationWrite || !ValidOperationID(v.IdempotencyKey) {
		return fmt.Errorf("invalid managed preparation request")
	}
	return ValidateConfiguration(v.AdapterID, v.Configuration)
}
func ValidateReference(v Reference) error {
	if !ID(v.PreparationID) || !ValidOperationID(v.ClaimID) || v.Fence < 1 || v.Fence > 9007199254740991 {
		return fmt.Errorf("invalid managed preparation reference")
	}
	return nil
}
func ValidateClaimRequest(v ClaimRequest) error {
	if !ID(v.ProjectID) || !ID(v.WorkerID) || !ValidOperationID(v.ClaimID) {
		return fmt.Errorf("invalid managed preparation claim")
	}
	return nil
}
func ValidatePoolClaimRequest(v PoolClaimRequest) error {
	if !ID(v.TenantID) {
		return fmt.Errorf("invalid pool tenant")
	}
	return ValidateClaimRequest(ClaimRequest{ProjectID: v.ProjectID, WorkerID: v.RunnerID, ClaimID: v.ClaimID})
}
func utc(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && strings.HasSuffix(value, "Z")
}
func ValidateStopProof(v StopProof) error {
	if v.Protocol != StopProtocol || ValidateReference(v.Preparation) != nil || !ValidDigest(v.SourceSHA256) || !ValidDigest(v.ReceiptSHA256) || !v.ProbeReturned || !v.AllNativeTreesStopped || v.NativeProcessCount < 0 || v.NativeProcessCount > MaxNativeProcesses || v.StoppedProcessCount != v.NativeProcessCount || !utc(v.ObservedAt) {
		return fmt.Errorf("invalid managed preparation stop proof")
	}
	return nil
}
func StopProofDigest(v StopProof) string { return Digest(v) }
func ValidateGenerationReceipt(v GenerationReceipt) error {
	if !ValidDigest(v.ParentGenerationSHA256) || !ValidDigest(v.GenerationSHA256) || v.ParentGenerationSHA256 == v.GenerationSHA256 || !tc.ValidCommandID(v.CommandID) || !ValidDigest(v.ArtifactSHA256) || !ValidDigest(v.LocalReceiptSHA256) || !supportedGenerationProfile(v.Profile) || v.Profile.CommandID != v.CommandID || v.StopProof == nil || ValidateStopProof(*v.StopProof) != nil {
		return fmt.Errorf("invalid accepted generation receipt")
	}
	return nil
}
func FailedCodes() []string {
	return []string{"not_started", "source_changed", "rescan_required", "configuration_invalid", "credential_reference_unavailable", "native_source_changed", "generation_changed", "preparation_not_supported", "preparation_failed"}
}
func UnknownCodes() []string {
	return []string{"preparation_timeout", "preparation_interrupted", "worker_restarted", "outcome_unknown", "source_changed"}
}
func ResultCodes() []string {
	out := []string{"request_expired", "prepared", "completed"}
	for _, v := range append(FailedCodes(), UnknownCodes()...) {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
func States() []string {
	return []string{"pending", "claimed", "awaiting_inventory", "completed", "failed", "unknown", "closed_unknown", "expired", "superseded"}
}
func ValidateResultRequest(v ResultRequest) error {
	if !ID(v.ProjectID) || !ID(v.WorkerID) || ValidateReference(v.Preparation) != nil || !ValidDigest(v.SourceSHA256) || !ValidOperationID(v.IdempotencyKey) {
		return fmt.Errorf("invalid managed preparation result")
	}
	if v.StopProof != nil && (ValidateStopProof(*v.StopProof) != nil || v.StopProof.Preparation != v.Preparation || v.StopProof.SourceSHA256 != v.SourceSHA256) {
		return fmt.Errorf("preparation stop proof has another source")
	}
	switch v.Outcome {
	case "prepared":
		if v.FailureCode != "" || v.Receipt == nil || ValidateGenerationReceipt(*v.Receipt) != nil || v.StopProof != nil || v.Receipt.StopProof.Preparation != v.Preparation || v.Receipt.StopProof.SourceSHA256 != v.SourceSHA256 {
			return fmt.Errorf("unproven accepted generation")
		}
	case "failed":
		if !slices.Contains(FailedCodes(), v.FailureCode) || v.Receipt != nil || v.StopProof == nil || v.FailureCode == "not_started" && v.StopProof.NativeProcessCount != 0 {
			return fmt.Errorf("unproven preparation failure")
		}
	case "unknown":
		if !slices.Contains(UnknownCodes(), v.FailureCode) || v.Receipt != nil || v.StopProof != nil {
			return fmt.Errorf("invalid unknown preparation result")
		}
	case "closed_unknown":
		if !slices.Contains(UnknownCodes(), v.FailureCode) || v.Receipt != nil || v.StopProof == nil {
			return fmt.Errorf("unproven preparation closure")
		}
	default:
		return fmt.Errorf("invalid preparation outcome")
	}
	return nil
}
func ValidatePoolResultRequest(v PoolResultRequest) error {
	if !ID(v.TenantID) {
		return fmt.Errorf("invalid pool tenant")
	}
	return ValidateResultRequest(ResultRequest{ProjectID: v.ProjectID, WorkerID: v.RunnerID, Preparation: v.Preparation, SourceSHA256: v.SourceSHA256, IdempotencyKey: v.IdempotencyKey, Outcome: v.Outcome, FailureCode: v.FailureCode, Receipt: v.Receipt, StopProof: v.StopProof})
}
func ValidateInventoryBinding(v InventoryBinding) error {
	if ValidateReference(v.Reference) != nil || !ValidDigest(v.SourceSHA256) || !ValidDigest(v.GenerationSHA256) || !ValidDigest(v.LocalReceiptSHA256) {
		return fmt.Errorf("invalid preparation inventory binding")
	}
	return nil
}

// State, versions, result and current qualification are mutable projections.
// The original source digest remains stable throughout receipt reconciliation.
func SourceSHA256(v Operation) string {
	v.Version = 0
	v.State = ""
	v.SourceSHA256 = ""
	v.FinishedAt = ""
	v.ResultCode = ""
	v.Receipt = nil
	v.StopProof = nil
	v.InventoryOperationID = ""
	v.InventoryObservedAt = ""
	v.SourceChanged = false
	v.Fence = 0
	v.ClaimID = ""
	v.ClaimedAt = ""
	v.LeaseUntil = ""
	return Digest(v)
}
func ValidateOperation(v Operation) error {
	if !ID(v.ID) || !ID(v.TenantID) || !ID(v.ProjectID) || !ID(v.WorkerID) || v.Version < 1 || !slices.Contains(States(), v.State) || ValidateConfiguration(v.AdapterID, v.Configuration) != nil || !ValidOperationID(v.InstanceID) || !ID(v.InstallationScanID) || v.InstallationScanVersion < 1 || ii.ValidateReference(v.InstallationReference) != nil || v.InstallationReference.ScanID != v.InstallationScanID || !ValidDigest(v.InstallationSourceSHA256) || ii.ValidateItem(v.InstallationItem) != nil || v.InstallationItem.AdapterID != v.AdapterID || !CanPrepareItem(v.InstallationItem) || v.InstallationStopProof == nil || ii.ValidateStopProof(*v.InstallationStopProof) != nil || v.InstallationStopProof.Scan != v.InstallationReference || v.InstallationStopProof.SourceSHA256 != v.InstallationSourceSHA256 || !ValidDigest(v.ExpectedGenerationSHA256) || !ID(v.RequestedBy) || !utc(v.CreatedAt) || !utc(v.ExpiresAt) || !ValidDigest(v.SourceSHA256) || v.SourceSHA256 != SourceSHA256(v) {
		return fmt.Errorf("invalid immutable preparation operation")
	}
	if (v.ClaimID != "" || v.Fence != 0 || v.ClaimedAt != "" || v.LeaseUntil != "") && (ValidateReference(Reference{PreparationID: v.ID, ClaimID: v.ClaimID, Fence: v.Fence}) != nil || !utc(v.ClaimedAt) || !utc(v.LeaseUntil)) {
		return fmt.Errorf("invalid preparation claim source")
	}
	if v.ClaimID == "" && !slices.Contains([]string{"pending", "expired", "superseded"}, v.State) {
		return fmt.Errorf("missing preparation claim source")
	}
	if v.Receipt != nil && (ValidateGenerationReceipt(*v.Receipt) != nil || v.Receipt.ParentGenerationSHA256 != v.ExpectedGenerationSHA256 || v.Receipt.CommandID != CommandID(v.ID) || v.Receipt.StopProof.Preparation != (Reference{PreparationID: v.ID, ClaimID: v.ClaimID, Fence: v.Fence}) || v.Receipt.StopProof.SourceSHA256 != v.SourceSHA256) {
		return fmt.Errorf("invalid preparation generation source")
	}
	if v.ResultCode != "" && !slices.Contains(ResultCodes(), v.ResultCode) {
		return fmt.Errorf("invalid preparation result code")
	}
	if v.State == "awaiting_inventory" && v.Receipt == nil || v.State == "completed" && (v.Receipt == nil || !ValidOperationID(v.InventoryOperationID) || !utc(v.InventoryObservedAt)) {
		return fmt.Errorf("missing preparation inventory acknowledgement")
	}
	return nil
}
func ValidateClaim(v Claim) error {
	if !ID(v.TenantID) || !ID(v.ProjectID) || !ID(v.WorkerID) {
		return fmt.Errorf("invalid canonical preparation scope")
	}
	if v.Preparation != nil && (ValidateOperation(*v.Preparation) != nil || v.Preparation.TenantID != v.TenantID || v.Preparation.ProjectID != v.ProjectID || v.Preparation.WorkerID != v.WorkerID) {
		return fmt.Errorf("preparation claim scope changed")
	}
	return nil
}
