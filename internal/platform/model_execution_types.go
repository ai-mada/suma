package platform

import "github.com/ai-mada/suma/internal/modelcontract"

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
	ProjectID          string                 `json:"project_id"`
	ToolProfileID      string                 `json:"tool_profile_id"`
	ToolProfileVersion int                    `json:"tool_profile_version"`
	ToolProfileSHA256  string                 `json:"tool_profile_sha256"`
	ObservedAt         string                 `json:"observed_at"`
	Supported          bool                   `json:"supported"`
	Reason             string                 `json:"reason"`
	Items              []ModelExecutionOption `json:"items"`
}
