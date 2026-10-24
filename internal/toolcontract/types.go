// Package toolcontract defines the bounded, non-secret wire contract shared by
// the control plane, scheduler and execution nodes.
package toolcontract

type ToolExecutionProfile struct {
	CommandID        string `json:"command_id"`
	Adapter          string `json:"adapter"`
	Protocol         string `json:"protocol"`
	ProfileSHA256    string `json:"profile_sha256"`
	ExecutableSHA256 string `json:"executable_sha256"`
	ToolVersion      string `json:"tool_version"`
	ReadOnly         bool   `json:"read_only"`
}

type ToolObservation struct {
	Profile   Profile `json:"profile"`
	Available bool    `json:"available"`
	Reason    string  `json:"reason"`
}

type ToolUsage struct {
	InputTokens           *int64 `json:"input_tokens,omitempty"`
	CachedInputTokens     *int64 `json:"cached_input_tokens,omitempty"`
	CacheWriteInputTokens *int64 `json:"cache_write_input_tokens,omitempty"`
	OutputTokens          *int64 `json:"output_tokens,omitempty"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens,omitempty"`
}

// MaxSafeUsage is the largest exact integer shared by the Go and browser wire.
const MaxSafeUsage int64 = 9007199254740991

const (
	UsageComplete   = "complete"
	UsagePartial    = "partial"
	UsageUnreported = "unreported"
)

type ToolEvent struct {
	SchemaVersion int             `json:"schema_version"`
	CommandID     string          `json:"command_id"`
	ProfileSHA256 string          `json:"profile_sha256"`
	Protocol      string          `json:"protocol"`
	Kind          string          `json:"kind"`
	SessionID     string          `json:"session_id,omitempty"`
	ItemID        string          `json:"item_id,omitempty"`
	Message       string          `json:"message,omitempty"`
	Status        string          `json:"status,omitempty"`
	Usage         *Usage          `json:"usage,omitempty"`
	UsageState    string          `json:"usage_state,omitempty"`
	Diagnostic    *ToolDiagnostic `json:"diagnostic,omitempty"`
}

// ToolDiagnostic is display identity, not authority, terminal evidence or the
// native process's rendered prose. Its source is always ToolEvent.Message.
type ToolDiagnostic struct {
	SchemaVersion int            `json:"schema_version"`
	MessageKey    string         `json:"message_key"`
	MessageParams map[string]any `json:"message_params,omitempty"`
}

type ToolEvidence struct {
	Profile    Profile `json:"profile"`
	SessionID  string  `json:"session_id,omitempty"`
	Terminal   string  `json:"terminal"`
	EventCount int     `json:"event_count"`
	Usage      *Usage  `json:"usage,omitempty"`
	UsageState string  `json:"usage_state"`
}

type Profile = ToolExecutionProfile
type Observation = ToolObservation
type Usage = ToolUsage
type Event = ToolEvent
type Evidence = ToolEvidence
