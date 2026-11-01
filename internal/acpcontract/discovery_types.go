package acpcontract

const ModelDiscoveryProtocol = "suma-acp-model-discovery-v1"

// Choices preserve the exact agent value. The host adds ConfigurationSHA256
// only after combining it with the actual reviewed local installation.
type ACPModelChoice struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	ConfigurationSHA256 string `json:"configuration_sha256,omitempty"`
}

// Catalogue is a snapshot from one explicit NewSession response, not a model
// health check, provider identity, price list or independently granted access.
type ACPModelCatalogue struct {
	Source          string           `json:"source"`
	ProtocolVersion int              `json:"protocol_version"`
	ConfigID        string           `json:"config_id"`
	CurrentValue    string           `json:"current_value"`
	Models          []ACPModelChoice `json:"models"`
}

// Transport requests and real host lifecycle are recorded independently.
// Zero Prompt requests does not prove the trusted CLI made no internal calls.
type ACPModelDiscoveryEvidence struct {
	SessionCreated         bool `json:"session_created"`
	SessionCancelRequested bool `json:"session_cancel_requested"`
	SessionCancelWritten   bool `json:"session_cancel_written"`
	PromptRequests         int  `json:"prompt_requests"`
	ConfigurationWrites    int  `json:"configuration_writes"`
	PermissionRequests     int  `json:"permission_requests"`
	UnexpectedActivity     int  `json:"unexpected_activity"`
	HostProcessStopped     bool `json:"host_process_stopped"`
	HostProcessTreeStopped bool `json:"host_process_tree_stopped"`
	HostProcessExitCode    *int `json:"host_process_exit_code,omitempty"`
	HostStoppedAfterRPC    bool `json:"host_stopped_after_rpc"`
}

// Safe result for the fixed local stdin command. It contains no executable
// path, session identifier, raw configuration, argv, login files or secrets.
type ACPModelDiscoveryResult struct {
	Protocol         string                    `json:"protocol"`
	Runtime          string                    `json:"runtime"`
	State            string                    `json:"state"`
	ExecutableSHA256 string                    `json:"executable_sha256"`
	ToolVersion      string                    `json:"tool_version"`
	AgentName        string                    `json:"agent_name,omitempty"`
	AgentVersion     string                    `json:"agent_version,omitempty"`
	Catalogue        *ACPModelCatalogue        `json:"catalogue,omitempty" wire:"nonnull"`
	Evidence         ACPModelDiscoveryEvidence `json:"evidence"`
	ErrorCode        string                    `json:"error_code,omitempty"`
}
