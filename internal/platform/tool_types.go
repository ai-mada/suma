package platform

import (
	"github.com/ai-mada/suma/internal/displaycontract"
	"github.com/ai-mada/suma/internal/toolcontract"
)

type ToolProfile struct {
	ID               string                             `json:"id"`
	ProjectID        string                             `json:"project_id"`
	WorkerID         string                             `json:"worker_id"`
	Name             string                             `json:"name"`
	Version          int                                `json:"version"`
	State            string                             `json:"state"`
	Profile          toolcontract.Profile               `json:"profile"`
	EnabledBy        string                             `json:"enabled_by"`
	CreatedAt        string                             `json:"created_at"`
	UpdatedAt        string                             `json:"updated_at"`
	Observation      *toolcontract.Observation          `json:"observation,omitempty"`
	LastSeenAt       string                             `json:"last_seen_at,omitempty"`
	Available        bool                               `json:"available"`
	Reason           string                             `json:"reason"`
	ReasonDiagnostic *displaycontract.DisplayDiagnostic `json:"reason_diagnostic,omitempty"`
}
type ToolProfileCreateRequest struct {
	ProjectID      string               `json:"project_id"`
	WorkerID       string               `json:"worker_id"`
	Name           string               `json:"name"`
	Profile        toolcontract.Profile `json:"profile"`
	IdempotencyKey string               `json:"idempotency_key"`
}
type ToolProfileCommandRequest struct {
	ProjectID       string `json:"project_id"`
	Action          string `json:"action"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type ToolProfileList struct {
	Items      []ToolProfile `json:"items"`
	Total      int           `json:"total"`
	NextCursor string        `json:"next_cursor,omitempty"`
}
type ToolInventoryRequest struct {
	ProjectID   string                     `json:"project_id"`
	WorkerID    string                     `json:"worker_id"`
	OperationID string                     `json:"operation_id"`
	Items       []toolcontract.Observation `json:"items"`
}
type ToolInventoryReceipt struct {
	Acknowledged bool   `json:"acknowledged"`
	ObservedAt   string `json:"observed_at"`
}
