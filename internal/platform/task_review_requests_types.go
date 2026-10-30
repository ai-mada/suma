package platform

type TaskReviewTarget struct {
	ResourceID      string `json:"resource_id"`
	UserID          string `json:"user_id"`
	Name            string `json:"name"`
	ResourceVersion int    `json:"resource_version"`
}

type TaskReviewTargetQuery struct {
	Q           string   `json:"q,omitempty"`
	Limit       int      `json:"limit,omitempty"`
	Cursor      string   `json:"cursor,omitempty"`
	ResourceIDs []string `json:"resource_ids,omitempty"`
}

type TaskReviewTargetPage struct {
	ProjectID  string             `json:"project_id"`
	TaskID     string             `json:"task_id"`
	Limit      int                `json:"limit"`
	Items      []TaskReviewTarget `json:"items"`
	Total      int                `json:"total"`
	NextCursor string             `json:"next_cursor"`
}

type TaskReviewDecision struct {
	ReviewID       string `json:"review_id"`
	ReviewerUserID string `json:"reviewer_user_id"`
	Decision       string `json:"decision"`
	Reason         string `json:"reason"`
	CreatedAt      string `json:"created_at"`
}

type TaskReviewRequest struct {
	ID                     string              `json:"id"`
	Version                int                 `json:"version"`
	ProjectID              string              `json:"project_id"`
	TaskID                 string              `json:"task_id"`
	TaskVersion            int                 `json:"task_version"`
	RunID                  string              `json:"run_id"`
	RunVersion             int                 `json:"run_version"`
	TaskBasisSHA256        string              `json:"task_basis_sha256"`
	RequesterUserID        string              `json:"requester_user_id"`
	RequesterNameAtRequest string              `json:"requester_name_at_request"`
	ReviewerResourceID     string              `json:"reviewer_resource_id"`
	ReviewerUserID         string              `json:"reviewer_user_id"`
	ReviewerNameAtRequest  string              `json:"reviewer_name_at_request"`
	Note                   string              `json:"note"`
	State                  string              `json:"state"`
	CreatedAt              string              `json:"created_at"`
	UpdatedAt              string              `json:"updated_at"`
	CancelledByUserID      string              `json:"cancelled_by_user_id"`
	CancelReason           string              `json:"cancel_reason"`
	ClosedReviewID         string              `json:"closed_review_id"`
	BindingCurrent         bool                `json:"binding_current"`
	ReviewerAvailable      bool                `json:"reviewer_available"`
	IsCurrentAssignee      bool                `json:"is_current_assignee"`
	CanCancel              bool                `json:"can_cancel"`
	SourceAvailable        bool                `json:"source_available"`
	Decision               *TaskReviewDecision `json:"decision,omitempty"`
}

type TaskReviewRequestPage struct {
	ProjectID        string              `json:"project_id"`
	TaskID           string              `json:"task_id"`
	TaskVersion      int                 `json:"task_version"`
	CanRequest       bool                `json:"can_request"`
	Limit            int                 `json:"limit"`
	Items            []TaskReviewRequest `json:"items"`
	Total            int                 `json:"total"`
	NextCursor       string              `json:"next_cursor"`
	PendingRequestID string              `json:"pending_request_id"`
}

type TaskReviewRequestDetail struct {
	ProjectID   string            `json:"project_id"`
	TaskID      string            `json:"task_id"`
	TaskTitle   string            `json:"task_title"`
	TaskVersion int               `json:"task_version"`
	CanRequest  bool              `json:"can_request"`
	Request     TaskReviewRequest `json:"request"`
}

type TaskReviewRequestCreate struct {
	ProjectID           string `json:"project_id"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	RunID               string `json:"run_id"`
	ExpectedRunVersion  int    `json:"expected_run_version"`
	ReviewerResourceID  string `json:"reviewer_resource_id"`
	Note                string `json:"note"`
	IdempotencyKey      string `json:"idempotency_key"`
}

type TaskReviewRequestCommand struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotency_key"`
}
