// Package toolinstallationcontract defines a fixed, read-only installation
// scan on one authenticated machine. Installation facts confer no dispatch,
// configuration, model, task or acceptance authority.
package toolinstallationcontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
	"unicode"
	"unicode/utf8"
)

const Protocol = "suma-installed-tool-scan-v1"
const StopProtocol = "suma-installed-tool-stop-v1"
const PendingTTLSeconds = 300
const ClaimLeaseSeconds = 60
const MaxProjectRequests = 10000
const MaxNativeProcesses = 2048

type ToolInstallationCapability struct {
	Protocol   string   `json:"protocol"`
	AdapterIDs []string `json:"adapter_ids"`
}
type ToolInstallationScanReference struct {
	ScanID  string `json:"scan_id"`
	ClaimID string `json:"claim_id"`
	Fence   int64  `json:"fence"`
}
type ToolInstallationItem struct {
	AdapterID        string `json:"adapter_id"`
	Protocol         string `json:"protocol"`
	Installed        bool   `json:"installed"`
	LaunchSupported  bool   `json:"launch_supported"`
	ReasonCode       string `json:"reason_code"`
	Version          string `json:"version,omitempty"`
	ExecutableSHA256 string `json:"executable_sha256,omitempty"`
}

// Counts describe owned process trees, not the number of native descendants.
// All fields, including a genuine zero count, are required on the HTTP wire.
type ToolInstallationStopProof struct {
	Protocol              string                        `json:"protocol"`
	Scan                  ToolInstallationScanReference `json:"scan"`
	SourceSHA256          string                        `json:"source_sha256"`
	ReceiptSHA256         string                        `json:"receipt_sha256"`
	ProbeReturned         bool                          `json:"probe_returned"`
	AllNativeTreesStopped bool                          `json:"all_native_trees_stopped"`
	NativeProcessCount    int                           `json:"native_process_count"`
	StoppedProcessCount   int                           `json:"stopped_process_count"`
	ObservedAt            string                        `json:"observed_at"`
}
type ToolInstallationScanResult struct {
	Items     []ToolInstallationItem     `json:"items"`
	StopProof *ToolInstallationStopProof `json:"stop_proof,omitempty" wire:"nonnull"`
}
type ToolInstallationScan struct {
	ID            string                      `json:"id"`
	ProjectID     string                      `json:"project_id"`
	WorkerID      string                      `json:"worker_id"`
	AdapterIDs    []string                    `json:"adapter_ids"`
	InstanceID    string                      `json:"instance_id"`
	Version       int                         `json:"version"`
	State         string                      `json:"state"`
	RequestedBy   string                      `json:"requested_by"`
	CreatedAt     string                      `json:"created_at"`
	ExpiresAt     string                      `json:"expires_at"`
	SourceSHA256  string                      `json:"source_sha256"`
	Fence         int64                       `json:"fence"`
	ClaimID       string                      `json:"claim_id,omitempty"`
	ClaimedAt     string                      `json:"claimed_at,omitempty"`
	LeaseUntil    string                      `json:"lease_until,omitempty"`
	FinishedAt    string                      `json:"finished_at,omitempty"`
	ResultCode    string                      `json:"result_code,omitempty"`
	Result        *ToolInstallationScanResult `json:"result,omitempty" wire:"nonnull"`
	SourceChanged bool                        `json:"source_changed,omitempty"`
}
type ToolInstallationScanCreateRequest struct {
	ProjectID      string   `json:"project_id"`
	WorkerID       string   `json:"worker_id"`
	AdapterIDs     []string `json:"adapter_ids"`
	IdempotencyKey string   `json:"idempotency_key"`
}
type ToolInstallationScanClaimRequest struct {
	ProjectID string `json:"project_id"`
	WorkerID  string `json:"worker_id"`
	ClaimID   string `json:"claim_id"`
}
type PoolToolInstallationScanClaimRequest struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	RunnerID  string `json:"runner_id"`
	ClaimID   string `json:"claim_id"`
}
type ToolInstallationScanClaim struct {
	Scan *ToolInstallationScan `json:"scan"`
}
type ToolInstallationScanResultRequest struct {
	ProjectID      string                        `json:"project_id"`
	WorkerID       string                        `json:"worker_id"`
	Scan           ToolInstallationScanReference `json:"scan"`
	IdempotencyKey string                        `json:"idempotency_key"`
	Outcome        string                        `json:"outcome"`
	Items          []ToolInstallationItem        `json:"items"`
	FailureCode    string                        `json:"failure_code"`
	StopProof      *ToolInstallationStopProof    `json:"stop_proof,omitempty" wire:"nonnull"`
}
type PoolToolInstallationScanResultRequest struct {
	TenantID       string                        `json:"tenant_id"`
	ProjectID      string                        `json:"project_id"`
	RunnerID       string                        `json:"runner_id"`
	Scan           ToolInstallationScanReference `json:"scan"`
	IdempotencyKey string                        `json:"idempotency_key"`
	Outcome        string                        `json:"outcome"`
	Items          []ToolInstallationItem        `json:"items"`
	FailureCode    string                        `json:"failure_code"`
	StopProof      *ToolInstallationStopProof    `json:"stop_proof,omitempty" wire:"nonnull"`
}
type Capability = ToolInstallationCapability
type Reference = ToolInstallationScanReference
type Item = ToolInstallationItem
type StopProof = ToolInstallationStopProof
type Scan = ToolInstallationScan
type Result = ToolInstallationScanResult
type CreateRequest = ToolInstallationScanCreateRequest
type ClaimRequest = ToolInstallationScanClaimRequest
type PoolClaimRequest = PoolToolInstallationScanClaimRequest
type ClaimReply = ToolInstallationScanClaim
type ResultRequest = ToolInstallationScanResultRequest
type PoolResultRequest = PoolToolInstallationScanResultRequest

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$`)
var operationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,149}$`)
var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var adapterProtocols = map[string]string{"codex-jsonl": "codex-jsonl-v1", "codebuddy-cli": "codebuddy-jsonl-v1", "dsh-multica": "dsh-multica-jsonl-v1", "claude-code-cli": "claude-jsonl-v1", "cursor-cli": "acp-stdio-v1", "qoder-acp": "acp-stdio-v1", "codex-app-server": "codex-app-server-v1", "opencode-sdk": "opencode-sdk-v1", "claude-code-sdk": "claude-sdk-v1"}

func ID(value string) bool                { return idPattern.MatchString(value) }
func Operation(value string) bool         { return operationPattern.MatchString(value) }
func AdapterProtocol(value string) string { return adapterProtocols[value] }
func AdapterIDs() []string {
	return []string{"codex-jsonl", "codebuddy-cli", "dsh-multica", "claude-code-cli", "cursor-cli", "qoder-acp", "codex-app-server", "opencode-sdk", "claude-code-sdk"}
}
func ValidateAdapterIDs(ids []string) error {
	if len(ids) < 1 || len(ids) > 9 {
		return fmt.Errorf("invalid installation scan adapter count")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if adapterProtocols[id] == "" || seen[id] {
			return fmt.Errorf("invalid installation scan adapter")
		}
		seen[id] = true
	}
	return nil
}
func ValidateCapability(v Capability) error {
	if v.Protocol != Protocol {
		return fmt.Errorf("invalid installation scan protocol")
	}
	return ValidateAdapterIDs(v.AdapterIDs)
}
func ValidateReference(v Reference) error {
	if !ID(v.ScanID) || !Operation(v.ClaimID) || v.Fence < 1 {
		return fmt.Errorf("invalid installation scan reference")
	}
	return nil
}
func timestamp(value string) (time.Time, error) {
	if len(value) > 40 {
		return time.Time{}, fmt.Errorf("invalid installation scan time")
	}
	return time.Parse(time.RFC3339Nano, value)
}
func safeText(value string, max int) bool {
	if len(value) > max || !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}
func ValidateItem(v Item) error {
	if adapterProtocols[v.AdapterID] != v.Protocol || v.Protocol == "" || !safeText(v.Version, 256) || (v.ExecutableSHA256 != "" && !digestPattern.MatchString(v.ExecutableSHA256)) {
		return fmt.Errorf("invalid installation observation")
	}
	if v.LaunchSupported && (!v.Installed || v.ReasonCode != "workspace_required" || v.Version == "") {
		return fmt.Errorf("installation observation is not launch-ready")
	}
	switch v.ReasonCode {
	case "not_installed", "cursor_editor_only", "cursor_scan_incomplete":
		if v.Installed || v.LaunchSupported || v.Version != "" || v.ExecutableSHA256 != "" {
			return fmt.Errorf("missing installation has source facts")
		}
	case "unsupported_launcher", "invalid_package", "node_unavailable", "probe_failed", "cursor_agent_unverified", "cursor_launcher_unsupported":
		if v.LaunchSupported {
			return fmt.Errorf("unverified installation is launch-ready")
		}
	case "workspace_required":
		if !v.Installed || !v.LaunchSupported {
			return fmt.Errorf("missing workspace-ready installation")
		}
	case "configuration_required", "version_unsupported", "acp_configuration_required":
		if !v.Installed || v.LaunchSupported || v.Version == "" || v.ExecutableSHA256 == "" {
			return fmt.Errorf("incomplete configured installation source")
		}
	default:
		return fmt.Errorf("unknown installation reason")
	}
	return nil
}
func ValidateStopProof(v StopProof) error {
	if v.Protocol != StopProtocol || ValidateReference(v.Scan) != nil || !digestPattern.MatchString(v.SourceSHA256) || !digestPattern.MatchString(v.ReceiptSHA256) || !v.ProbeReturned || !v.AllNativeTreesStopped || v.NativeProcessCount < 0 || v.NativeProcessCount > MaxNativeProcesses || v.StoppedProcessCount != v.NativeProcessCount {
		return fmt.Errorf("incomplete original installation stop proof")
	}
	_, err := timestamp(v.ObservedAt)
	return err
}

// Request identity is immutable across claim, expiry and closure. Claim/fence
// additionally bind every proof; a claim never changes this source digest.
func ScanSourceSHA256(v Scan) string {
	raw, _ := json.Marshal(struct {
		ID, ProjectID, WorkerID                       string
		AdapterIDs                                    []string
		InstanceID, RequestedBy, CreatedAt, ExpiresAt string
	}{v.ID, v.ProjectID, v.WorkerID, v.AdapterIDs, v.InstanceID, v.RequestedBy, v.CreatedAt, v.ExpiresAt})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func StopProofDigest(v StopProof) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func unknownCode(value string) bool {
	return value == "probe_timeout" || value == "probe_interrupted" || value == "worker_restarted"
}
func ValidateCreateRequest(v CreateRequest) error {
	if !ID(v.ProjectID) || !ID(v.WorkerID) || !Operation(v.IdempotencyKey) {
		return fmt.Errorf("invalid installation scan request")
	}
	return ValidateAdapterIDs(v.AdapterIDs)
}
func ValidateClaimRequest(v ClaimRequest) error {
	if !ID(v.ProjectID) || !ID(v.WorkerID) || !Operation(v.ClaimID) {
		return fmt.Errorf("invalid installation scan claim")
	}
	return nil
}
func ValidateResultRequest(v ResultRequest) error {
	if !ID(v.ProjectID) || !ID(v.WorkerID) || !Operation(v.IdempotencyKey) || ValidateReference(v.Scan) != nil || v.Items == nil || len(v.Items) > 9 {
		return fmt.Errorf("invalid installation scan result")
	}
	seen := map[string]bool{}
	for _, item := range v.Items {
		if seen[item.AdapterID] || ValidateItem(item) != nil {
			return fmt.Errorf("invalid installation scan result item")
		}
		seen[item.AdapterID] = true
	}
	if v.StopProof != nil && (ValidateStopProof(*v.StopProof) != nil || v.StopProof.Scan != v.Scan) {
		return fmt.Errorf("installation stop proof source differs")
	}
	switch v.Outcome {
	case "completed":
		if len(v.Items) == 0 || v.FailureCode != "" || v.StopProof == nil {
			return fmt.Errorf("incomplete successful installation scan")
		}
	case "failed":
		if len(v.Items) != 0 || v.FailureCode != "not_started" || v.StopProof == nil || v.StopProof.NativeProcessCount != 0 {
			return fmt.Errorf("unproven installation scan not-started result")
		}
	case "unknown":
		if len(v.Items) != 0 || !unknownCode(v.FailureCode) || v.StopProof != nil {
			return fmt.Errorf("invalid unknown installation scan result")
		}
	case "closed_unknown":
		if len(v.Items) != 0 || !unknownCode(v.FailureCode) || v.StopProof == nil {
			return fmt.Errorf("installation scan closure needs original stop proof")
		}
	default:
		return fmt.Errorf("unknown installation scan outcome")
	}
	return nil
}
func ValidateScan(v Scan) error {
	if !ID(v.ID) || !ID(v.ProjectID) || !ID(v.WorkerID) || !ID(v.RequestedBy) || !Operation(v.InstanceID) || v.Version < 1 || v.Version > 2147483647 || ValidateAdapterIDs(v.AdapterIDs) != nil || v.SourceSHA256 != ScanSourceSHA256(v) || v.Fence < 0 {
		return fmt.Errorf("invalid installation scan source")
	}
	created, e := timestamp(v.CreatedAt)
	expires, x := timestamp(v.ExpiresAt)
	if e != nil || x != nil || expires.Sub(created) != PendingTTLSeconds*time.Second {
		return fmt.Errorf("invalid installation scan deadline")
	}
	claimed := v.Fence > 0
	if claimed {
		at, e := timestamp(v.ClaimedAt)
		until, x := timestamp(v.LeaseUntil)
		if !Operation(v.ClaimID) || e != nil || x != nil || until.Sub(at) != ClaimLeaseSeconds*time.Second {
			return fmt.Errorf("invalid installation scan claim facts")
		}
	} else if v.ClaimID != "" || v.ClaimedAt != "" || v.LeaseUntil != "" {
		return fmt.Errorf("unclaimed installation scan has claim facts")
	}
	finished := v.FinishedAt != ""
	if finished {
		if _, e := timestamp(v.FinishedAt); e != nil {
			return e
		}
	}
	switch v.State {
	case "pending":
		if claimed || finished || v.Result != nil || v.ResultCode != "" {
			return fmt.Errorf("pending installation scan has execution facts")
		}
	case "claimed":
		if !claimed || finished || v.Result != nil || v.ResultCode != "" {
			return fmt.Errorf("incomplete installation scan claim")
		}
	case "unknown":
		if !claimed || finished || v.Result != nil || !unknownCode(v.ResultCode) {
			return fmt.Errorf("invalid unknown installation scan")
		}
	case "expired", "superseded":
		code := "request_expired"
		if v.State == "superseded" {
			code = "source_changed"
		}
		if claimed || finished || v.Result != nil || v.ResultCode != code {
			return fmt.Errorf("invalid unstarted installation scan terminal")
		}
	case "completed", "failed", "closed_unknown":
		if !claimed || !finished || v.Result == nil {
			return fmt.Errorf("installation scan terminal lacks result facts")
		}
		failure := v.ResultCode
		if v.State == "completed" {
			if v.ResultCode != "scan_completed" {
				return fmt.Errorf("invalid installation completion code")
			}
			failure = ""
		}
		req := ResultRequest{ProjectID: v.ProjectID, WorkerID: v.WorkerID, Scan: Reference{v.ID, v.ClaimID, v.Fence}, IdempotencyKey: "validation", Outcome: v.State, Items: v.Result.Items, FailureCode: failure, StopProof: v.Result.StopProof}
		if ValidateResultRequest(req) != nil || v.Result.StopProof.SourceSHA256 != v.SourceSHA256 {
			return fmt.Errorf("invalid installation scan terminal proof")
		}
		if v.State == "completed" {
			if len(v.Result.Items) != len(v.AdapterIDs) {
				return fmt.Errorf("incomplete installation scan coverage")
			}
			for i, id := range v.AdapterIDs {
				if v.Result.Items[i].AdapterID != id {
					return fmt.Errorf("installation result order differs")
				}
			}
		}
	default:
		return fmt.Errorf("invalid installation scan state")
	}
	return nil
}
