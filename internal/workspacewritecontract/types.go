// Package workspacewritecontract defines human-requested, compare-and-swap
// saves in one live native attempt. It grants no arbitrary filesystem access.
package workspacewritecontract

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

const Protocol = "workspace-write-v1"
const MaxContentBytes = 48 << 10
const RequestTTLSeconds = 120
const LeaseSeconds = 10

// Scope is the exact already-dispatched execution, never a host path.
type Scope = woc.Scope

type WorkspaceWriteCreateRequest struct {
	ProjectID             string `json:"project_id"`
	Scope                 Scope  `json:"scope"`
	WorkspacePolicySHA256 string `json:"workspace_policy_sha256"`
	Path                  string `json:"path"`
	ExpectedSHA256        string `json:"expected_sha256"`
	Content               string `json:"content"`
	ContentSHA256         string `json:"content_sha256"`
	IdempotencyKey        string `json:"idempotency_key"`
}

// Command contains durable metadata only. Source text is a separate bounded
// delivery payload and is never returned by browser status/receipt reads.
type WorkspaceWriteCommand struct {
	ID                    string                `json:"id"`
	ProjectID             string                `json:"project_id"`
	Version               int                   `json:"version"`
	State                 string                `json:"state"`
	Scope                 Scope                 `json:"scope"`
	WorkspacePolicySHA256 string                `json:"workspace_policy_sha256"`
	Path                  string                `json:"path"`
	ExpectedSHA256        string                `json:"expected_sha256"`
	ContentSHA256         string                `json:"content_sha256"`
	RequestSHA256         string                `json:"request_sha256"`
	ExecutionFence        int64                 `json:"execution_fence"`
	ExecuteClaimID        string                `json:"execute_claim_id,omitempty"`
	ExecuteBefore         string                `json:"execute_before,omitempty"`
	InspectionID          string                `json:"inspection_id,omitempty"`
	InspectionState       string                `json:"inspection_state,omitempty"`
	CreatedAt             string                `json:"created_at"`
	ExpiresAt             string                `json:"expires_at"`
	Result                *WorkspaceWriteEffect `json:"result,omitempty"`
	ErrorCode             string                `json:"error_code,omitempty"`
}

type WorkspaceWriteEffect struct {
	NativeOperationID string `json:"native_operation_id"`
	Path              string `json:"path"`
	PreviousSHA256    string `json:"previous_sha256"`
	SHA256            string `json:"sha256"`
	Generation        int    `json:"generation"`
}

type WorkspaceWritePage struct {
	ProjectID string                  `json:"project_id"`
	Items     []WorkspaceWriteCommand `json:"items"`
	Total     int                     `json:"total"`
	Limit     int                     `json:"limit"`
	Truncated bool                    `json:"truncated"`
}

type WorkspaceWriteInspectionRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type WorkspaceWriteClaimRequest struct {
	ProjectID             string `json:"project_id"`
	WorkerID              string `json:"worker_id"`
	Scope                 Scope  `json:"scope"`
	WorkspacePolicySHA256 string `json:"workspace_policy_sha256"`
	OperationID           string `json:"operation_id"`
}

// An execute lease is issued once. Inspect leases can only examine the same
// original operation and never authorize another write or historical reopen.
type WorkspaceWriteLease struct {
	Command    WorkspaceWriteCommand `json:"command"`
	ClaimID    string                `json:"claim_id"`
	Mode       string                `json:"mode"`
	LeaseUntil string                `json:"lease_until"`
	Content    *string               `json:"content,omitempty"`
}

type WorkspaceWriteResultRequest struct {
	ProjectID             string                `json:"project_id"`
	WorkerID              string                `json:"worker_id"`
	CommandID             string                `json:"command_id"`
	Scope                 Scope                 `json:"scope"`
	WorkspacePolicySHA256 string                `json:"workspace_policy_sha256"`
	RequestSHA256         string                `json:"request_sha256"`
	ExecutionFence        int64                 `json:"execution_fence"`
	ClaimID               string                `json:"claim_id"`
	Mode                  string                `json:"mode"`
	Outcome               string                `json:"outcome"`
	Effect                *WorkspaceWriteEffect `json:"effect,omitempty"`
	ErrorCode             string                `json:"error_code,omitempty"`
	ObservedAt            string                `json:"observed_at"`
	ResultSHA256          string                `json:"result_sha256"`
	OperationID           string                `json:"operation_id"`
}

type WorkspaceWriteReceipt struct {
	CommandID    string `json:"command_id"`
	ClaimID      string `json:"claim_id"`
	ResultSHA256 string `json:"result_sha256"`
	Acknowledged bool   `json:"acknowledged"`
}

type CreateRequest = WorkspaceWriteCreateRequest
type Command = WorkspaceWriteCommand
type Effect = WorkspaceWriteEffect
type InspectionRequest = WorkspaceWriteInspectionRequest
type ClaimRequest = WorkspaceWriteClaimRequest
type Lease = WorkspaceWriteLease
type ResultRequest = WorkspaceWriteResultRequest
type Receipt = WorkspaceWriteReceipt
type Page = WorkspaceWritePage

func ContentDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// Stable across delivery attempts; excludes UI idempotency and raw content.
func RequestDigest(req CreateRequest) string {
	return woc.Digest(struct {
		Protocol                        string
		ProjectID                       string
		Scope                           Scope
		Policy, Path, Expected, Content string
	}{Protocol, req.ProjectID, req.Scope, req.WorkspacePolicySHA256, req.Path, req.ExpectedSHA256, req.ContentSHA256})
}

func NativeOperationID(id string) string { return "remote-write:" + id }

func ValidateCreate(req CreateRequest) error {
	if req.ProjectID == "" || woc.ValidateScope(req.Scope) != nil || !woc.ValidDigest(req.WorkspacePolicySHA256) || !woc.ValidPath(req.Path, false) || !woc.ValidDigest(req.ExpectedSHA256) || !woc.ValidDigest(req.ContentSHA256) || !utf8.ValidString(req.Content) || strings.ContainsRune(req.Content, 0) || len(req.Content) > MaxContentBytes || ContentDigest(req.Content) != req.ContentSHA256 {
		return fmt.Errorf("invalid workspace save request")
	}
	return nil
}

func ValidateCommand(c Command) error {
	if c.ID == "" || c.ProjectID == "" || c.Version < 1 || woc.ValidateScope(c.Scope) != nil || !woc.ValidDigest(c.WorkspacePolicySHA256) || !woc.ValidPath(c.Path, false) || !woc.ValidDigest(c.ExpectedSHA256) || !woc.ValidDigest(c.ContentSHA256) || !woc.ValidDigest(c.RequestSHA256) || c.ExecutionFence < 0 {
		return fmt.Errorf("invalid workspace save command")
	}
	req := CreateRequest{ProjectID: c.ProjectID, Scope: c.Scope, WorkspacePolicySHA256: c.WorkspacePolicySHA256, Path: c.Path, ExpectedSHA256: c.ExpectedSHA256, ContentSHA256: c.ContentSHA256}
	if RequestDigest(req) != c.RequestSHA256 {
		return fmt.Errorf("workspace save request digest changed")
	}
	switch c.State {
	case "requested", "leased", "applied", "not_applied", "reconciling", "expired":
	default:
		return fmt.Errorf("invalid workspace save state")
	}
	if !ValidError(c.ErrorCode) {
		return fmt.Errorf("invalid workspace save diagnostic")
	}
	if c.State == "applied" {
		e := c.Result
		if e == nil || c.ExecutionFence < 1 || c.ErrorCode != "" || e.NativeOperationID != NativeOperationID(c.ID) || e.Path != c.Path || e.PreviousSHA256 != c.ExpectedSHA256 || e.SHA256 != c.ContentSHA256 || e.Generation < 1 {
			return fmt.Errorf("invalid applied workspace save command")
		}
	} else if c.Result != nil {
		return fmt.Errorf("unexpected workspace save effect")
	}
	return nil
}

func ValidError(code string) bool {
	switch code {
	case "", "not_started", "file_changed", "file_missing", "path_not_allowed", "sensitive_content", "workspace_closed", "workspace_unavailable", "request_invalid", "permit_expired", "operation_unknown", "operation_conflict", "output_limit", "text_required":
		return true
	}
	return false
}

func ResultDigest(outcome string, effect *Effect, code string) string {
	return woc.Digest(struct {
		Outcome   string  `json:"outcome"`
		Effect    *Effect `json:"effect"`
		ErrorCode string  `json:"error_code"`
	}{outcome, effect, code})
}

// An applied receipt must prove the exact desired file transition. A matching
// current file by itself is never proof that the original operation happened.
func ValidateResult(c Command, req ResultRequest) error {
	if ValidateCommand(c) != nil || req.ProjectID != c.ProjectID || req.CommandID != c.ID || req.Scope != c.Scope || req.WorkerID != c.Scope.WorkerID || req.WorkspacePolicySHA256 != c.WorkspacePolicySHA256 || req.RequestSHA256 != c.RequestSHA256 || req.ExecutionFence != c.ExecutionFence || c.ExecutionFence < 1 || req.ClaimID == "" || (req.Mode != "execute" && req.Mode != "inspect") || !ValidError(req.ErrorCode) || req.ResultSHA256 != ResultDigest(req.Outcome, req.Effect, req.ErrorCode) {
		return fmt.Errorf("workspace save receipt differs from original command")
	}
	switch req.Outcome {
	case "applied":
		e := req.Effect
		if e == nil || req.ErrorCode != "" || e.NativeOperationID != NativeOperationID(c.ID) || e.Path != c.Path || e.PreviousSHA256 != c.ExpectedSHA256 || e.SHA256 != c.ContentSHA256 || e.Generation < 1 {
			return fmt.Errorf("invalid applied workspace save receipt")
		}
	case "not_applied":
		if req.Effect != nil || req.ErrorCode == "" || req.ErrorCode == "operation_unknown" {
			return fmt.Errorf("invalid workspace save no-effect receipt")
		}
	case "unknown":
		if req.Effect != nil || req.ErrorCode != "operation_unknown" {
			return fmt.Errorf("invalid unknown workspace save receipt")
		}
	default:
		return fmt.Errorf("invalid workspace save outcome")
	}
	return nil
}

// Decode reuses the bounded, duplicate/Unicode/case-sensitive decoder shared
// with workspace observations; the destination DTO defines this write surface.
func Decode(raw []byte, out any) error { return woc.Decode(raw, out) }
