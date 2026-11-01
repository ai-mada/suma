package platform

import "github.com/ai-mada/suma/internal/toolcontract"

// ToolProfileReplacementReuse fixes the exact existing target selected by the
// operator. A match never silently chooses one profile or authorizes activation.
type ToolProfileReplacementReuse struct {
	ProfileID       string `json:"profile_id"`
	ExpectedVersion int    `json:"expected_version"`
}

// ToolProfileReplacementRequest prepares an immutable configuration handoff.
// Name is required only for a new paused target; Reuse instead fixes an existing
// target and version. The old profile and its existing attempts remain intact.
type ToolProfileReplacementRequest struct {
	ProjectID       string                       `json:"project_id"`
	ExpectedVersion int                          `json:"expected_version"`
	WorkerID        string                       `json:"worker_id"`
	Profile         toolcontract.Profile         `json:"profile"`
	PauseOriginal   bool                         `json:"pause_original"`
	IdempotencyKey  string                       `json:"idempotency_key"`
	Name            string                       `json:"name,omitempty"`
	Reuse           *ToolProfileReplacementReuse `json:"reuse,omitempty" wire:"nonnull"`
}

// ToolProfileReplacement preserves the committed preparation snapshots. It is
// a relationship and receipt, not a second profile authority or a live health
// check. Replaying it must not recompute observations or repeat prior effects.
type ToolProfileReplacement struct {
	ID            string      `json:"id"`
	ProjectID     string      `json:"project_id"`
	Original      ToolProfile `json:"original"`
	Replacement   ToolProfile `json:"replacement"`
	Disposition   string      `json:"disposition"`
	PauseOriginal bool        `json:"pause_original"`
	CreatedAt     string      `json:"created_at"`
	CreatedBy     string      `json:"created_by"`
}

type ToolProfileReplacementList struct {
	ProjectID  string                   `json:"project_id"`
	ProfileID  string                   `json:"profile_id"`
	Direction  string                   `json:"direction"`
	Items      []ToolProfileReplacement `json:"items"`
	NextCursor string                   `json:"next_cursor,omitempty"`
}

type ToolProfileReplacementListQuery struct {
	Direction string `json:"direction,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Cursor    string `json:"cursor,omitempty"`
}
