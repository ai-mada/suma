package platform

import "github.com/ai-mada/suma/internal/toolcontract"

// ToolProfileMatchRequest locates the exact immutable configuration on one
// current project worker. A name or digest alone is not its full identity.
// This is a read request: it neither creates a receipt nor grants dispatch.
type ToolProfileMatchRequest struct {
	ProjectID string               `json:"project_id"`
	WorkerID  string               `json:"worker_id"`
	Profile   toolcontract.Profile `json:"profile"`
}

// ToolProfileMatchList is a complete, non-paginated match set under the existing
// project limit. Revoked items remain history; only paused/enabled may be reused.
// Availability remains an independent observation, never inferred from a match.
type ToolProfileMatchList struct {
	ProjectID string               `json:"project_id"`
	WorkerID  string               `json:"worker_id"`
	Profile   toolcontract.Profile `json:"profile"`
	Items     []ToolProfile        `json:"items"`
	Total     int                  `json:"total"`
}
