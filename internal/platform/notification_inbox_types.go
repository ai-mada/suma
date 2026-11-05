package platform

type NotificationInboxCounts struct {
	Inbox    int `json:"inbox"`
	Unread   int `json:"unread"`
	Archived int `json:"archived"`
}

type NotificationInboxTask struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Title   string `json:"title"`
	Status  string `json:"status"`
}

type NotificationInboxItem struct {
	ID                string                 `json:"id"`
	Version           int                    `json:"version"`
	Sequence          string                 `json:"sequence"`
	Kind              string                 `json:"kind"`
	TaskID            string                 `json:"task_id"`
	SourceTaskVersion int                    `json:"source_task_version"`
	SourceResourceID  string                 `json:"source_resource_id"`
	CreatedAt         string                 `json:"created_at"`
	Read              bool                   `json:"read"`
	Archived          bool                   `json:"archived"`
	ReadAt            string                 `json:"read_at"`
	ArchivedAt        string                 `json:"archived_at"`
	SourceAvailable   bool                   `json:"source_available"`
	AssignmentCurrent bool                   `json:"assignment_current"`
	CurrentTask       *NotificationInboxTask `json:"current_task,omitempty"`
	MessageID         string                 `json:"message_id,omitempty"`
	ReviewRequestID   string                 `json:"review_request_id,omitempty"`
	HandoffID         string                 `json:"handoff_id,omitempty"`
	RunID             string                 `json:"run_id,omitempty"`
}

type NotificationInboxPage struct {
	ProjectID  string                  `json:"project_id"`
	UserID     string                  `json:"user_id"`
	Limit      int                     `json:"limit"`
	Items      []NotificationInboxItem `json:"items"`
	Total      int                     `json:"total"`
	Counts     NotificationInboxCounts `json:"counts"`
	NextCursor string                  `json:"next_cursor"`
	AsOf       string                  `json:"as_of"`
}

type NotificationInboxCommand struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type NotificationInboxCommandReceipt struct {
	IdempotencyKey string `json:"idempotency_key"`
	NotificationID string `json:"notification_id"`
	Version        int    `json:"version"`
	Action         string `json:"action"`
	Read           bool   `json:"read"`
	Archived       bool   `json:"archived"`
	ReadAt         string `json:"read_at"`
	ArchivedAt     string `json:"archived_at"`
	UpdatedAt      string `json:"updated_at"`
}

type NotificationInboxBatchTarget struct {
	ProjectID       string `json:"project_id"`
	NotificationID  string `json:"notification_id"`
	ExpectedVersion int    `json:"expected_version"`
}

type NotificationInboxBatchCommand struct {
	Items          []NotificationInboxBatchTarget `json:"items"`
	Action         string                         `json:"action"`
	IdempotencyKey string                         `json:"idempotency_key"`
}

type NotificationInboxBatchResult struct {
	ProjectID string                          `json:"project_id"`
	Receipt   NotificationInboxCommandReceipt `json:"receipt"`
}

type NotificationInboxBatchReceipt struct {
	IdempotencyKey string                         `json:"idempotency_key"`
	Action         string                         `json:"action"`
	Results        []NotificationInboxBatchResult `json:"results"`
}

// NotificationOrganizationInboxItem associates the same private notification
// projection with the current project needed to open its exact source.
type NotificationOrganizationInboxItem struct {
	ProjectID    string                `json:"project_id"`
	ProjectTitle string                `json:"project_title"`
	Notification NotificationInboxItem `json:"notification"`
}

type NotificationOrganizationInboxPage struct {
	OrganizationID string                              `json:"organization_id"`
	UserID         string                              `json:"user_id"`
	Limit          int                                 `json:"limit"`
	Items          []NotificationOrganizationInboxItem `json:"items"`
	Total          int                                 `json:"total"`
	Counts         NotificationInboxCounts             `json:"counts"`
	NextCursor     string                              `json:"next_cursor"`
	AsOf           string                              `json:"as_of"`
	ScopeRevision  string                              `json:"scope_revision"`
}
