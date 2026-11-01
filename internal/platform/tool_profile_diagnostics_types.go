package platform

import "github.com/ai-mada/suma/internal/displaycontract"

// ToolProfileDiagnosticCheck describes one current prerequisite, not a new
// permission. Codes and actions are bounded by the public domain contract.
type ToolProfileDiagnosticCheck struct {
	Code             string                             `json:"code"`
	Status           string                             `json:"status"`
	Reason           string                             `json:"reason"`
	ReasonDiagnostic *displaycontract.DisplayDiagnostic `json:"reason_diagnostic,omitempty"`
	NextAction       string                             `json:"next_action"`
}

// ToolProfileDiagnostics shares the original availability evaluation. Its short
// display deadline grants no right to execute, renew, repair or enable a tool.
type ToolProfileDiagnostics struct {
	Tool       ToolProfile                  `json:"tool"`
	CheckedAt  string                       `json:"checked_at"`
	ExpiresAt  string                       `json:"expires_at"`
	Checks     []ToolProfileDiagnosticCheck `json:"checks"`
	NextAction string                       `json:"next_action"`
}
