// Package toolinspectioncontract describes one explicitly requested inspection
// of a registered worker's existing tool configuration. It grants no task,
// installation, configuration mutation or model execution authority.
package toolinspectioncontract

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const PendingTTLSeconds = 300
const ClaimLeaseSeconds = 60
const MaxProjectRequests = 10000

type ToolInspectionReference struct {
	InspectionID string `json:"inspection_id"`
	ClaimID      string `json:"claim_id"`
	Fence        int64  `json:"fence"`
}

type ToolInspection struct {
	ID                   string `json:"id"`
	ProjectID            string `json:"project_id"`
	WorkerID             string `json:"worker_id"`
	ToolProfileID        string `json:"tool_profile_id"`
	ToolProfileVersion   int    `json:"tool_profile_version"`
	ProfileSHA256        string `json:"profile_sha256"`
	CommandID            string `json:"command_id"`
	Version              int    `json:"version"`
	Fence                int64  `json:"fence"`
	State                string `json:"state"`
	RequestedBy          string `json:"requested_by"`
	CreatedAt            string `json:"created_at"`
	ExpiresAt            string `json:"expires_at"`
	ClaimID              string `json:"claim_id,omitempty"`
	ClaimedAt            string `json:"claimed_at,omitempty"`
	LeaseUntil           string `json:"lease_until,omitempty"`
	ObservedAt           string `json:"observed_at,omitempty"`
	InventoryOperationID string `json:"inventory_operation_id,omitempty"`
	ResultCode           string `json:"result_code,omitempty"`
	RecoveryID           string `json:"recovery_id,omitempty"`
	ClosedAt             string `json:"closed_at,omitempty"`
	SourceChanged        bool   `json:"source_changed,omitempty"`
}

type ToolInspectionCreateRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type ToolInspectionClaimRequest struct {
	ProjectID string `json:"project_id"`
	WorkerID  string `json:"worker_id"`
	ClaimID   string `json:"claim_id"`
}

type PoolToolInspectionClaimRequest struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	RunnerID  string `json:"runner_id"`
	ClaimID   string `json:"claim_id"`
}

type ToolInspectionClaim struct {
	Inspection *ToolInspection `json:"inspection"`
}

type ToolInspectionFailureRequest struct {
	ProjectID      string                  `json:"project_id"`
	WorkerID       string                  `json:"worker_id"`
	Inspection     ToolInspectionReference `json:"inspection"`
	IdempotencyKey string                  `json:"idempotency_key"`
	Outcome        string                  `json:"outcome"`
	FailureCode    string                  `json:"failure_code"`
}

type PoolToolInspectionFailureRequest struct {
	TenantID       string                  `json:"tenant_id"`
	ProjectID      string                  `json:"project_id"`
	RunnerID       string                  `json:"runner_id"`
	Inspection     ToolInspectionReference `json:"inspection"`
	IdempotencyKey string                  `json:"idempotency_key"`
	Outcome        string                  `json:"outcome"`
	FailureCode    string                  `json:"failure_code"`
}

type ToolInspectionList struct {
	Items []ToolInspection `json:"items"`
}

type Reference = ToolInspectionReference
type Inspection = ToolInspection
type CreateRequest = ToolInspectionCreateRequest
type ClaimRequest = ToolInspectionClaimRequest
type ClaimReply = ToolInspectionClaim
type FailureRequest = ToolInspectionFailureRequest

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@+\-]{0,149}$`)
var digest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func ID(s string) bool { return identifier.MatchString(s) }
func Operation(s string) bool {
	return len(s) >= 8 && len(s) <= 128 && utf8.ValidString(s) && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0
}
func ValidateReference(v Reference) error {
	if !ID(v.InspectionID) || !Operation(v.ClaimID) || v.Fence < 1 || v.Fence > 9007199254740991 {
		return fmt.Errorf("invalid tool inspection reference")
	}
	return nil
}
func ValidateClaimRequest(v ClaimRequest) error {
	if !ID(v.ProjectID) || !ID(v.WorkerID) || !Operation(v.ClaimID) {
		return fmt.Errorf("invalid tool inspection claim")
	}
	return nil
}
func ValidateFailureRequest(v FailureRequest) error {
	if !ID(v.ProjectID) || !ID(v.WorkerID) || ValidateReference(v.Inspection) != nil || !Operation(v.IdempotencyKey) {
		return fmt.Errorf("invalid tool inspection failure")
	}
	if v.Outcome == "failed" && v.FailureCode == "not_started" {
		return nil
	}
	if v.Outcome == "unknown" && (v.FailureCode == "probe_interrupted" || v.FailureCode == "probe_timeout" || v.FailureCode == "probe_failed") {
		return nil
	}
	return fmt.Errorf("invalid tool inspection failure outcome")
}
func ValidateInspection(v Inspection) error {
	if !ID(v.ID) || !ID(v.ProjectID) || !ID(v.WorkerID) || !ID(v.ToolProfileID) || !ID(v.CommandID) || !ID(v.RequestedBy) || v.ToolProfileVersion < 1 || v.Version < 1 || v.Fence < 0 || v.Fence > 9007199254740991 || !digest.MatchString(v.ProfileSHA256) {
		return fmt.Errorf("invalid tool inspection source")
	}
	switch v.State {
	case "pending", "claimed", "completed", "failed", "unknown", "expired", "superseded", "closed_unknown":
	default:
		return fmt.Errorf("invalid tool inspection state")
	}
	created, e := time.Parse(time.RFC3339Nano, v.CreatedAt)
	expires, x := time.Parse(time.RFC3339Nano, v.ExpiresAt)
	if e != nil || x != nil || !expires.After(created) {
		return fmt.Errorf("invalid tool inspection deadline")
	}
	if v.Fence > 0 && (ValidateReference(Reference{v.ID, v.ClaimID, v.Fence}) != nil || v.ClaimedAt == "" || v.LeaseUntil == "") {
		return fmt.Errorf("invalid tool inspection claim source")
	}
	for _, s := range []string{v.ClaimedAt, v.LeaseUntil, v.ObservedAt} {
		if s != "" {
			if _, e := time.Parse(time.RFC3339Nano, s); e != nil {
				return fmt.Errorf("invalid tool inspection time")
			}
		}
	}
	claimed := v.Fence > 0
	if !claimed && (v.ClaimID != "" || v.ClaimedAt != "" || v.LeaseUntil != "") {
		return fmt.Errorf("unclaimed inspection contains claim facts")
	}
	if claimed {
		at, _ := time.Parse(time.RFC3339Nano, v.ClaimedAt)
		until, _ := time.Parse(time.RFC3339Nano, v.LeaseUntil)
		if at.Before(created) || !until.After(at) {
			return fmt.Errorf("invalid tool inspection lease")
		}
	}
	if v.State != "completed" && (v.ObservedAt != "" || v.InventoryOperationID != "") {
		return fmt.Errorf("only a completed inspection has inventory facts")
	}
	if v.State != "closed_unknown" && (v.RecoveryID != "" || v.ClosedAt != "") {
		return fmt.Errorf("only reconciled unknown has recovery facts")
	}
	switch v.State {
	case "pending":
		if claimed || v.ResultCode != "" {
			return fmt.Errorf("pending inspection contains execution facts")
		}
	case "claimed":
		if !claimed || v.ResultCode != "" {
			return fmt.Errorf("claimed inspection requires a live claim")
		}
	case "completed":
		if !claimed || v.ObservedAt == "" || !Operation(v.InventoryOperationID) {
			return fmt.Errorf("completed inspection requires its original inventory")
		}
		switch v.ResultCode {
		case "reported_available", "reported_unavailable", "profile_changed", "command_missing":
		default:
			return fmt.Errorf("invalid inventory result")
		}
	case "failed":
		if !claimed || v.ResultCode != "not_started" {
			return fmt.Errorf("failed inspection requires a not-started receipt")
		}
	case "unknown":
		if !claimed || (v.ResultCode != "probe_interrupted" && v.ResultCode != "probe_timeout" && v.ResultCode != "probe_failed") {
			return fmt.Errorf("unknown inspection requires its original claim")
		}
	case "closed_unknown":
		if !claimed || !ID(v.RecoveryID) || (v.ResultCode != "probe_interrupted" && v.ResultCode != "probe_timeout" && v.ResultCode != "probe_failed") {
			return fmt.Errorf("closed unknown requires original unknown and recovery")
		}
		if _, err := time.Parse(time.RFC3339Nano, v.ClosedAt); err != nil {
			return fmt.Errorf("closed unknown requires durable closure")
		}
	case "expired":
		if claimed || v.ResultCode != "request_expired" {
			return fmt.Errorf("expired inspection cannot have executed")
		}
	case "superseded":
		if claimed || v.ResultCode != "source_changed" {
			return fmt.Errorf("superseded inspection requires source change")
		}
	}
	return nil
}
