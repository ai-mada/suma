// Package workbuddycloudcontract defines the narrow control-plane cloud task
// contract. Validation establishes shape only, never current execution authority.
package workbuddycloudcontract

import (
	"fmt"
	"io"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/workbuddy"
)

const (
	Protocol              = "workbuddy-cloud/v1"
	Plugin                = "workbuddy-cloud"
	NodeType              = "external_workflow"
	MaxVersion            = 2147483647
	MaxDelegationLifetime = 24 * time.Hour
)

// NodeBinding is stored in the immutable node's config.workbuddy_cloud. Machine
// create/refresh commands contain no replacement prompt, provider ID or endpoint.
type NodeBinding struct {
	ConnectionID    string                  `json:"connection_id"`
	GrantGeneration int                     `json:"grant_generation"`
	Request         workbuddy.CreateRequest `json:"request"`
}

func (NodeBinding) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddycloudcontract.NodeBinding{request:redacted}")
}

type CloudDelegateRequest struct {
	ProjectID                 string `json:"project_id"`
	ConnectionID              string `json:"connection_id"`
	ExpectedConnectionVersion int    `json:"expected_connection_version"`
	GrantGeneration           int    `json:"grant_generation"`
	RunID                     string `json:"run_id"`
	NodeID                    string `json:"node_id"`
	ExpectedRunVersion        int    `json:"expected_run_version"`
	NodeSpecSHA256            string `json:"node_spec_sha256"`
	RequestSHA256             string `json:"request_sha256"`
	ExpiresAt                 string `json:"expires_at"`
	MaxCreates                int    `json:"max_creates"`
	IdempotencyKey            string `json:"idempotency_key"`
}

type CloudCreateRequest struct {
	ProjectID       string           `json:"project_id"`
	DelegationID    string           `json:"delegation_id"`
	ExpectedVersion int              `json:"expected_version"`
	Assignment      cc.AssignmentRef `json:"assignment"`
	IdempotencyKey  string           `json:"idempotency_key"`
}

type CloudRefreshRequest struct {
	ProjectID       string           `json:"project_id"`
	ExpectedVersion int              `json:"expected_version"`
	Assignment      cc.AssignmentRef `json:"assignment"`
	IdempotencyKey  string           `json:"idempotency_key"`
}

type CloudRevokeRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// CloudObserveRequest is a current human-owner read of the fixed supplier task.
// It neither accepts nor borrows a Runner assignment and cannot create a task.
type CloudObserveRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type CloudNodeView struct {
	ProjectID      string           `json:"project_id"`
	RunID          string           `json:"run_id"`
	NodeID         string           `json:"node_id"`
	RunVersion     int              `json:"run_version"`
	NodeSpecSHA256 string           `json:"node_spec_sha256"`
	RequestSHA256  string           `json:"request_sha256"`
	Binding        NodeBinding      `json:"binding"`
	Delegation     *CloudDelegation `json:"delegation,omitempty"`
	Submission     *CloudSubmission `json:"submission,omitempty"`
	AllowedActions []string         `json:"allowed_actions"`
}

func (CloudNodeView) Format(s fmt.State, verb rune) {
	_, _ = io.WriteString(s, "workbuddycloudcontract.CloudNodeView{request:redacted}")
}

type CloudDelegation struct {
	ID                       string `json:"id"`
	ProjectID                string `json:"project_id"`
	OwnerID                  string `json:"owner_id"`
	ConnectionID             string `json:"connection_id"`
	Version                  int    `json:"version"`
	RunID                    string `json:"run_id"`
	NodeID                   string `json:"node_id"`
	NodeSpecSHA256           string `json:"node_spec_sha256"`
	ConnectionVersionAtIssue int    `json:"connection_version_at_issue"`
	GrantGeneration          int    `json:"grant_generation"`
	ClientBindingSHA256      string `json:"client_binding_sha256"`
	RequestSHA256            string `json:"request_sha256"`
	ExpiresAt                string `json:"expires_at"`
	MaxCreates               int    `json:"max_creates"`
	CreatesConsumed          int    `json:"creates_consumed"`
	State                    string `json:"state"`
	CreatedAt                string `json:"created_at"`
	UpdatedAt                string `json:"updated_at"`
}

type CloudSubmission struct {
	ID            string           `json:"id"`
	ProjectID     string           `json:"project_id"`
	DelegationID  string           `json:"delegation_id"`
	Version       int              `json:"version"`
	RunID         string           `json:"run_id"`
	NodeID        string           `json:"node_id"`
	Assignment    cc.AssignmentRef `json:"assignment"`
	RequestSHA256 string           `json:"request_sha256"`
	State         string           `json:"state"`
	Task          *workbuddy.Task  `json:"task,omitempty"`
	ObservedAt    string           `json:"observed_at"`
	LastError     string           `json:"last_error"`
	UsageState    string           `json:"usage_state"`
	CreatedAt     string           `json:"created_at"`
	UpdatedAt     string           `json:"updated_at"`
}
