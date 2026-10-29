package platform

// SharedTaskViewPreferences stores only declarative task selection and layout.
// It never carries task contents, authority, execution actions or paging state.
type SharedTaskViewPreferences struct {
	Q             string `json:"q"`
	Status        string `json:"status"`
	StatusUnset   bool   `json:"status_unset,omitempty"`
	OwnerID       string `json:"owner_id"`
	Priority      string `json:"priority"`
	RequirementID string `json:"requirement_id,omitempty"`
	Unassigned    bool   `json:"unassigned,omitempty"`
	PriorityUnset bool   `json:"priority_unset,omitempty"`
	Sort          string `json:"sort"`
	Layout        string `json:"layout"`
	GroupBy       string `json:"group_by"`
}
type SharedTaskView struct {
	ID          string                    `json:"id"`
	ProjectID   string                    `json:"project_id"`
	OwnerID     string                    `json:"owner_id"`
	Name        string                    `json:"name"`
	Visibility  string                    `json:"visibility"`
	Version     int                       `json:"version"`
	State       string                    `json:"state"`
	Preferences SharedTaskViewPreferences `json:"preferences"`
	CreatedAt   string                    `json:"created_at"`
	UpdatedAt   string                    `json:"updated_at"`
	CanEdit     bool                      `json:"can_edit"`
	CanDelete   bool                      `json:"can_delete"`
}
type SharedTaskViewCreateRequest struct {
	ProjectID      string                    `json:"project_id"`
	Name           string                    `json:"name"`
	Visibility     string                    `json:"visibility"`
	Preferences    SharedTaskViewPreferences `json:"preferences"`
	IdempotencyKey string                    `json:"idempotency_key"`
}
type SharedTaskViewUpdateRequest struct {
	ProjectID       string                    `json:"project_id"`
	Name            string                    `json:"name"`
	Visibility      string                    `json:"visibility"`
	Preferences     SharedTaskViewPreferences `json:"preferences"`
	ExpectedVersion int                       `json:"expected_version"`
	IdempotencyKey  string                    `json:"idempotency_key"`
}
type SharedTaskViewArchiveRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type SharedTaskViewPage struct {
	ProjectID  string           `json:"project_id"`
	Items      []SharedTaskView `json:"items"`
	Total      int              `json:"total"`
	Limit      int              `json:"limit"`
	NextCursor string           `json:"next_cursor"`
	AsOf       string           `json:"as_of"`
}
