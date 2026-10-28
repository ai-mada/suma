// Package workspaceoperationcontract defines safe Go-host workspace facts.
// SDK activity is a separate contract and cannot manufacture this envelope.
package workspaceoperationcontract

const (
	Protocol      = "suma-workspace-operation-v1"
	Source        = "go_workspace_host"
	MaxCalls      = 2048
	MaxRecords    = MaxCalls * 2
	MaxEventBytes = 4096
	MaxGeneration = 2048
)

// Identity is supplied by the original authenticated assignment, never by an
// SDK tool argument. It also scopes deterministic transport/event identities.
type Identity struct {
	TenantID  string `json:"tenant_id"`
	ProjectID string `json:"project_id"`
	RunID     string `json:"run_id"`
	NodeID    string `json:"node_id"`
	WorkerID  string `json:"worker_id"`
	Attempt   int    `json:"attempt"`
	Fence     int64  `json:"fence"`
}

// Event projects one durable invocation transition. No source text, command,
// output, environment, model payload, URL, token or MCP arguments are allowed.
// A completed check is an observed completion; its exit code is separate.
type WorkspaceOperationEvent struct {
	Protocol          string `json:"protocol"`
	Source            string `json:"source"`
	SchemaVersion     int    `json:"schema_version"`
	EventID           string `json:"event_id"`
	Sequence          int    `json:"sequence"`
	OperationID       string `json:"operation_id"`
	EffectOperationID string `json:"effect_operation_id,omitempty"`
	Revision          int    `json:"revision"`
	Action            string `json:"action"`
	Phase             string `json:"phase"`
	GenerationBefore  int    `json:"generation_before"`
	GenerationAfter   int    `json:"generation_after"`
	StartedAt         string `json:"started_at"`
	RecordedAt        string `json:"recorded_at"`
	CompletedAt       string `json:"completed_at,omitempty"`
	RequestSHA256     string `json:"request_sha256"`
	PolicySHA256      string `json:"policy_sha256"`
	ProfileSHA256     string `json:"profile_sha256"`
	Path              string `json:"path,omitempty"`
	CheckID           string `json:"check_id,omitempty"`
	ResultSHA256      string `json:"result_sha256,omitempty"`
	ExitCode          *int   `json:"exit_code,omitempty" wire:"nonnull"`
	Stopped           *bool  `json:"stopped,omitempty" wire:"nonnull"`
	ErrorCode         string `json:"error_code,omitempty"`
	Replayed          bool   `json:"replayed,omitempty"`
}

// Coverage describes local capture, not platform acknowledgement or the loaded
// browser page. PendingCount counts invocations without a captured terminal.
type WorkspaceOperationCoverage struct {
	InvocationCount int    `json:"invocation_count"`
	RecordCount     int    `json:"record_count"`
	PendingCount    int    `json:"pending_count"`
	MaxInvocations  int    `json:"max_invocations"`
	MaxRecords      int    `json:"max_records"`
	State           string `json:"state"`
	Reason          string `json:"reason,omitempty"`
}

type Event = WorkspaceOperationEvent
type Coverage = WorkspaceOperationCoverage
