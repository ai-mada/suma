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
	Tool                  ToolProfile                      `json:"tool"`
	CheckedAt             string                           `json:"checked_at"`
	ExpiresAt             string                           `json:"expires_at"`
	Checks                []ToolProfileDiagnosticCheck     `json:"checks"`
	NextAction            string                           `json:"next_action"`
	Impact                *ToolProfileImpact               `json:"impact,omitempty" wire:"nonnull"`
	MaintenanceResolution ToolProfileMaintenanceResolution `json:"maintenance_resolution"`
}

// ToolProfileMaintenanceResolution is a current read of the original worker's
// maintenance occupancy. It is separate from the four dispatch prerequisites.
// The enclosing report binds this recommendation to the exact current profile
// and display deadline; IDs locate original operations and grant no write right.
type ToolProfileMaintenanceResolution struct {
	ProfileID      string `json:"profile_id"`
	ProfileVersion int    `json:"profile_version"`
	WorkerID       string `json:"worker_id"`
	Action         string `json:"action"`
	Reason         string `json:"reason"`
	InspectionID   string `json:"inspection_id,omitempty"`
	RecoveryID     string `json:"recovery_id,omitempty"`
}

// Impact describes observed references in the current authorized project's
// active runs. It never claims to enumerate external processes or stop them.
type ToolProfileImpact struct {
	Scope                  string                  `json:"scope"`
	ObservedAt             string                  `json:"observed_at"`
	Coverage               string                  `json:"coverage"`
	RunsExamined           int                     `json:"runs_examined"`
	NodesExamined          int                     `json:"nodes_examined"`
	ObservedReferenceCount int                     `json:"observed_reference_count"`
	Items                  []ToolProfileImpactItem `json:"items"`
	ItemsTruncated         bool                    `json:"items_truncated"`
	Reason                 string                  `json:"reason"`
}

type ToolProfileImpactItem struct {
	RunID     string `json:"run_id"`
	NodeID    string `json:"node_id"`
	RunState  string `json:"run_state"`
	NodeState string `json:"node_state"`
	WorkerID  string `json:"worker_id"`
	Attempt   int    `json:"attempt"`
	Fence     int64  `json:"fence"`
}
