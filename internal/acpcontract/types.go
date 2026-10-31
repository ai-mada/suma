// Package acpcontract contains non-secret facts shared by local ACP adapters,
// the control plane and clients. Configuration acknowledgement, execution,
// process stopping and independent quality evidence remain separate facts.
package acpcontract

const (
	Protocol   = "acp-stdio-v1"
	SDKVersion = "0.13.5"
)

// Capability is a fixed administrator profile, not a provider health claim.
// Paths, argv, login files and credential values never cross this boundary.
type ACPExecutionCapability struct {
	Runtime             string `json:"runtime"`
	ProtocolVersion     int    `json:"protocol_version"`
	SDKVersion          string `json:"sdk_version"`
	ConfigurationSHA256 string `json:"configuration_sha256"`
	Model               string `json:"model"`
	ModelSource         string `json:"model_source"`
	BudgetMode          string `json:"budget_mode"`
	IdentityEvidence    string `json:"identity_evidence"`
	AuthStrategy        string `json:"auth_strategy"`
	PermissionMode      string `json:"permission_mode"`
}

// Evidence records facts observed on one original process/session. A selected
// configuration is not proof of the provider's underlying model or its bill.
type ACPExecutionEvidence struct {
	ConfigurationSHA256        string `json:"configuration_sha256"`
	RequestedModel             string `json:"requested_model"`
	SelectedModel              string `json:"selected_model,omitempty"`
	ModelSelectionAcknowledged bool   `json:"model_selection_acknowledged"`
	SessionID                  string `json:"session_id,omitempty"`
	ProtocolVersion            int    `json:"protocol_version,omitempty"`
	AgentName                  string `json:"agent_name,omitempty"`
	AgentVersion               string `json:"agent_version,omitempty"`
	PromptStarted              bool   `json:"prompt_started"`
	PromptCompleted            bool   `json:"prompt_completed"`
	StopReason                 string `json:"stop_reason,omitempty"`
	ToolsComplete              bool   `json:"tools_complete"`
	PermissionsRequested       int    `json:"permissions_requested"`
	PermissionsDenied          int    `json:"permissions_denied"`
	CancelRequested            bool   `json:"cancel_requested"`
	HostProcessStopped         bool   `json:"host_process_stopped"`
	HostProcessTreeStopped     bool   `json:"host_process_tree_stopped"`
}

// PermissionOption preserves the agent's exact opaque identifier and label.
// Only allow_once/reject_once may be selected by the platform.
type ToolPermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// PermissionRequest is sanitized before both the local journal and network.
// Incomplete or redacted details cannot authorize an allow_once decision.
type ToolPermissionRequest struct {
	IdempotencyKey  string             `json:"idempotency_key"`
	SessionID       string             `json:"session_id"`
	ToolCallID      string             `json:"tool_call_id"`
	Title           string             `json:"title"`
	Kind            string             `json:"kind"`
	Details         string             `json:"details"`
	DetailsComplete bool               `json:"details_complete"`
	Options         []PermissionOption `json:"options"`
}

// PermissionDecision is the current human decision; it grants no persistent
// permission or approval for another request, process, attempt or task.
type ToolPermissionDecision struct {
	OptionID  string `json:"option_id"`
	Kind      string `json:"kind"`
	ActorID   string `json:"actor_id"`
	DecidedAt string `json:"decided_at"`
}

// Permission binds a durable request to one exact platform execution source.
// AllowedActions is derived from current authority and is never a stored grant.
type ToolPermission struct {
	ID             string              `json:"id"`
	ProjectID      string              `json:"project_id"`
	RunID          string              `json:"run_id"`
	NodeID         string              `json:"node_id"`
	Attempt        int                 `json:"attempt"`
	Fence          int64               `json:"fence"`
	DispatchID     string              `json:"dispatch_id"`
	WorkerID       string              `json:"worker_id"`
	ProfileSHA256  string              `json:"profile_sha256"`
	Version        int64               `json:"version"`
	State          string              `json:"state"`
	Request        PermissionRequest   `json:"request"`
	Decision       *PermissionDecision `json:"decision,omitempty"`
	CreatedAt      string              `json:"created_at"`
	ExpiresAt      string              `json:"expires_at"`
	AllowedActions []string            `json:"allowed_actions"`
}

type ToolPermissionList struct {
	Items []Permission `json:"items"`
}

type ToolPermissionDecisionRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int64  `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
	OptionID        string `json:"option_id"`
}

// Short Go names follow the existing toolcontract Profile/Evidence convention;
// generated public schemas keep the unambiguous domain-qualified names above.
type Capability = ACPExecutionCapability
type Evidence = ACPExecutionEvidence
type PermissionOption = ToolPermissionOption
type PermissionRequest = ToolPermissionRequest
type PermissionDecision = ToolPermissionDecision
type Permission = ToolPermission
type PermissionList = ToolPermissionList
type DecisionRequest = ToolPermissionDecisionRequest
