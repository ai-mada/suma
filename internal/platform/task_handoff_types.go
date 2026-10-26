package platform

type TaskHandoffCreateRequest struct {
	ProjectID           string `json:"project_id"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	ToResourceID        string `json:"to_resource_id"`
	ToTeamID            string `json:"to_team_id,omitempty"`
	Note                string `json:"note"`
	IdempotencyKey      string `json:"idempotency_key"`
}

type TaskHandoffCommandRequest struct {
	ProjectID               string `json:"project_id"`
	ExpectedVersion         int    `json:"expected_version"`
	Action                  string `json:"action"`
	Reason                  string `json:"reason"`
	AcknowledgeInvalidation bool   `json:"acknowledge_invalidation"`
	IdempotencyKey          string `json:"idempotency_key"`
}

type TaskHandoffBasis struct {
	TaskVersion        int    `json:"task_version"`
	TaskBasisSHA256    string `json:"task_basis_sha256"`
	RequirementID      string `json:"requirement_id"`
	RequirementVersion int    `json:"requirement_version"`
	FromOwnerID        string `json:"from_owner_id"`
	SourceRunID        string `json:"source_run_id"`
	SourceRunVersion   int    `json:"source_run_version"`
	SourceRunState     string `json:"source_run_state"`
	DeliverySHA256     string `json:"delivery_sha256"`
}

type TaskHandoffTeam struct {
	TeamID            string `json:"team_id"`
	Name              string `json:"name"`
	TeamVersion       int    `json:"team_version"`
	MembershipVersion int    `json:"membership_version"`
}

type TaskHandoffRecipient struct {
	ResourceID      string            `json:"resource_id"`
	ResourceVersion int               `json:"resource_version"`
	UserID          string            `json:"user_id"`
	Name            string            `json:"name"`
	Teams           []TaskHandoffTeam `json:"teams"`
}

type TaskHandoff struct {
	ID                  string           `json:"id"`
	ProjectID           string           `json:"project_id"`
	TaskID              string           `json:"task_id"`
	Version             int              `json:"version"`
	State               string           `json:"state"`
	CreatedBy           string           `json:"created_by"`
	CreatedAt           string           `json:"created_at"`
	UpdatedAt           string           `json:"updated_at"`
	ToResourceID        string           `json:"to_resource_id"`
	ToResourceVersion   int              `json:"to_resource_version"`
	ToUserID            string           `json:"to_user_id"`
	ToName              string           `json:"to_name"`
	ToTeamID            string           `json:"to_team_id"`
	ToTeamName          string           `json:"to_team_name"`
	ToTeamVersion       int              `json:"to_team_version"`
	ToMembershipVersion int              `json:"to_membership_version"`
	Note                string           `json:"note"`
	Basis               TaskHandoffBasis `json:"basis"`
	DecisionBy          string           `json:"decision_by"`
	DecisionAt          string           `json:"decision_at"`
	DecisionReason      string           `json:"decision_reason"`
	ResultTaskVersion   int              `json:"result_task_version"`
	BasisCurrent        bool             `json:"basis_current"`
	Blockers            []string         `json:"blockers"`
	AllowedActions      []string         `json:"allowed_actions"`
}

type TaskHandoffPage struct {
	Items          []TaskHandoff          `json:"items"`
	Total          int                    `json:"total"`
	NextCursor     string                 `json:"next_cursor"`
	Recipients     []TaskHandoffRecipient `json:"recipients"`
	CanCreate      bool                   `json:"can_create"`
	CreateBlockers []string               `json:"create_blockers"`
	TaskVersion    int                    `json:"task_version"`
}
