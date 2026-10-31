// Package toolpermissioncontract owns platform permission lifecycle and typed
// native request branches. ACP and Codex retain their distinct wire identities.
package toolpermissioncontract

import (
	ac "github.com/ai-mada/suma/internal/acpcontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
)

type ToolPermissionRequest struct {
	IdempotencyKey string                      `json:"idempotency_key"`
	Protocol       string                      `json:"protocol"`
	ACP            *ac.ACPToolPermissionSource `json:"acp,omitempty" wire:"nonnull"`
	Codex          *cc.CodexApprovalRequest    `json:"codex,omitempty" wire:"nonnull"`
}

type ToolPermissionChoice struct {
	Protocol string                      `json:"protocol"`
	ACP      *ac.ACPToolPermissionChoice `json:"acp,omitempty" wire:"nonnull"`
	Codex    *cc.CodexApprovalChoice     `json:"codex,omitempty" wire:"nonnull"`
}

type ToolPermissionDecision struct {
	Choice
	ActorID   string `json:"actor_id"`
	DecidedAt string `json:"decided_at"`
}

type ToolPermission struct {
	ID             string                       `json:"id"`
	ProjectID      string                       `json:"project_id"`
	RunID          string                       `json:"run_id"`
	NodeID         string                       `json:"node_id"`
	Attempt        int                          `json:"attempt"`
	Fence          int64                        `json:"fence"`
	DispatchID     string                       `json:"dispatch_id"`
	WorkerID       string                       `json:"worker_id"`
	ProfileSHA256  string                       `json:"profile_sha256"`
	Version        int64                        `json:"version"`
	State          string                       `json:"state"`
	Request        Request                      `json:"request"`
	Decision       *Decision                    `json:"decision,omitempty" wire:"nonnull"`
	Observation    *cc.CodexApprovalObservation `json:"observation,omitempty" wire:"nonnull"`
	CreatedAt      string                       `json:"created_at"`
	ExpiresAt      string                       `json:"expires_at"`
	AllowedActions []string                     `json:"allowed_actions"`
}

type ToolPermissionList struct {
	Items []Permission `json:"items"`
}

type ToolPermissionDecisionRequest struct {
	Choice
	ProjectID       string `json:"project_id"`
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type Request = ToolPermissionRequest
type Choice = ToolPermissionChoice
type Decision = ToolPermissionDecision
type Permission = ToolPermission
type List = ToolPermissionList
type DecisionRequest = ToolPermissionDecisionRequest
