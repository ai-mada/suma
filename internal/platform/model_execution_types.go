package platform

import (
	ac "github.com/ai-mada/suma/internal/acpcontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type ModelExecutionOptionsRequest struct {
	ProjectID           string `json:"project_id"`
	ToolProfileID       string `json:"tool_profile_id"`
	ExpectedToolVersion int    `json:"expected_tool_version"`
}
type ModelExecutionOption struct {
	Deployment             modelcontract.Deployment `json:"deployment"`
	Rate                   *UsageRateRevision       `json:"rate,omitempty"`
	CostState              string                   `json:"cost_state"`
	DefaultParameters      modelcontract.Parameters `json:"default_parameters"`
	InputTokenReservation  int64                    `json:"input_token_reservation"`
	OutputTokenReservation int64                    `json:"output_token_reservation"`
}
type ModelExecutionOptions struct {
	FixedACP           *ac.Capability                  `json:"fixed_acp,omitempty"`
	ParameterLimits    *modelcontract.Parameters       `json:"parameter_limits,omitempty"`
	FixedOpenCode      *tc.OpenCodeExecutionCapability `json:"fixed_opencode,omitempty"`
	FixedClaude        *tc.ClaudeExecutionCapability   `json:"fixed_claude,omitempty"`
	CodexSupported     bool                            `json:"codex_supported"`
	CodexCatalog       *cc.Catalog                     `json:"codex_catalog,omitempty"`
	CodexReason        string                          `json:"codex_reason"`
	ProjectID          string                          `json:"project_id"`
	ToolProfileID      string                          `json:"tool_profile_id"`
	ToolProfileVersion int                             `json:"tool_profile_version"`
	ToolProfileSHA256  string                          `json:"tool_profile_sha256"`
	ObservedAt         string                          `json:"observed_at"`
	Supported          bool                            `json:"supported"`
	Reason             string                          `json:"reason"`
	Items              []ModelExecutionOption          `json:"items"`
}
