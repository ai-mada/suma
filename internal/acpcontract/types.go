// Package acpcontract contains non-secret facts shared by local ACP adapters,
// the control plane and clients. Configuration acknowledgement, execution,
// process stopping and independent quality evidence remain separate facts.
package acpcontract

import cu "github.com/ai-mada/suma/internal/cursorcontract"

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
	ConfigurationSHA256        string                   `json:"configuration_sha256"`
	RequestedModel             string                   `json:"requested_model"`
	SelectedModel              string                   `json:"selected_model,omitempty"`
	ModelSelectionAcknowledged bool                     `json:"model_selection_acknowledged"`
	SessionID                  string                   `json:"session_id,omitempty"`
	ProtocolVersion            int                      `json:"protocol_version,omitempty"`
	AgentName                  string                   `json:"agent_name,omitempty"`
	AgentVersion               string                   `json:"agent_version,omitempty"`
	PromptStarted              bool                     `json:"prompt_started"`
	PromptCompleted            bool                     `json:"prompt_completed"`
	StopReason                 string                   `json:"stop_reason,omitempty"`
	ToolsComplete              bool                     `json:"tools_complete"`
	PermissionsRequested       int                      `json:"permissions_requested"`
	PermissionsDenied          int                      `json:"permissions_denied"`
	CancelRequested            bool                     `json:"cancel_requested"`
	HostProcessStopped         bool                     `json:"host_process_stopped"`
	HostProcessTreeStopped     bool                     `json:"host_process_tree_stopped"`
	UserInputs                 []cu.CursorInputEvidence `json:"user_inputs,omitempty"`
}

// PermissionOption preserves the agent's exact opaque identifier and label.
// Only allow_once/reject_once may be selected by the platform.
type ToolPermissionOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

// ACPToolPermissionSource retains the native session, tool and opaque options.
// The platform's idempotency key, lifecycle and human identity live in the
// separate toolpermissioncontract envelope, shared with other protocols.
type ACPToolPermissionSource struct {
	SessionID       string             `json:"session_id"`
	ToolCallID      string             `json:"tool_call_id"`
	Title           string             `json:"title"`
	Kind            string             `json:"kind"`
	Details         string             `json:"details"`
	DetailsComplete bool               `json:"details_complete"`
	Options         []PermissionOption `json:"options"`
}

type ACPToolPermissionChoice struct {
	OptionID string `json:"option_id"`
	Kind     string `json:"kind"`
}

type Capability = ACPExecutionCapability
type Evidence = ACPExecutionEvidence
type PermissionOption = ToolPermissionOption
type NativeSource = ACPToolPermissionSource
type Choice = ACPToolPermissionChoice
