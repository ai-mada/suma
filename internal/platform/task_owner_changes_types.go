package platform

type TaskOwnerTarget struct {
	ResourceID      string `json:"resource_id"`
	ResourceVersion int    `json:"resource_version"`
	UserID          string `json:"user_id"`
	Name            string `json:"name"`
}

type TaskOwnerTargetQuery struct {
	Q      string `json:"q,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type TaskOwnerTargetPage struct {
	ProjectID  string            `json:"project_id"`
	Items      []TaskOwnerTarget `json:"items"`
	Total      int               `json:"total"`
	Limit      int               `json:"limit"`
	NextCursor string            `json:"next_cursor"`
}

type TaskOwnerSelection struct {
	TaskID          string `json:"task_id"`
	ExpectedVersion int    `json:"expected_version"`
}

type TaskOwnerChangePreviewRequest struct {
	ProjectID            string               `json:"project_id"`
	OwnerID              string               `json:"owner_id"`
	ExpectedOwnerVersion int                  `json:"expected_owner_version"`
	Tasks                []TaskOwnerSelection `json:"tasks"`
}

type TaskOwnerChangePreviewItem struct {
	TaskID     string `json:"task_id"`
	Version    int    `json:"version"`
	Title      string `json:"title"`
	Status     string `json:"status"`
	OwnerID    string `json:"owner_id"`
	OwnerName  string `json:"owner_name"`
	RunID      string `json:"run_id"`
	WillChange bool   `json:"will_change"`
}

type TaskOwnerChangePreview struct {
	ProjectID      string                       `json:"project_id"`
	Owner          TaskOwnerTarget              `json:"owner"`
	Tasks          []TaskOwnerChangePreviewItem `json:"tasks"`
	ChangedCount   int                          `json:"changed_count"`
	UnchangedCount int                          `json:"unchanged_count"`
	PreviewSHA256  string                       `json:"preview_sha256"`
	CanApply       bool                         `json:"can_apply"`
}

type TaskOwnerChangeRequest struct {
	ProjectID               string               `json:"project_id"`
	OwnerID                 string               `json:"owner_id"`
	ExpectedOwnerVersion    int                  `json:"expected_owner_version"`
	Tasks                   []TaskOwnerSelection `json:"tasks"`
	PreviewSHA256           string               `json:"preview_sha256"`
	AcknowledgeInvalidation bool                 `json:"acknowledge_invalidation"`
	IdempotencyKey          string               `json:"idempotency_key"`
}

type TaskOwnerChangeReceipt struct {
	TaskID          string `json:"task_id"`
	PreviousVersion int    `json:"previous_version"`
	Version         int    `json:"version"`
	PreviousOwnerID string `json:"previous_owner_id"`
	OwnerID         string `json:"owner_id"`
	Status          string `json:"status"`
	OwnerChanged    bool   `json:"owner_changed"`
}

type TaskOwnerChangeResult struct {
	ProjectID   string                   `json:"project_id"`
	OperationID string                   `json:"operation_id"`
	Changes     []TaskOwnerChangeReceipt `json:"changes"`
}
