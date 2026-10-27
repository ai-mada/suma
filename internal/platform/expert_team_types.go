package platform

import "github.com/ai-mada/suma/internal/modelcontract"

type ExpertTeamRoleRequest struct {
	Key                     string                      `json:"key"`
	Title                   string                      `json:"title"`
	ResourceID              string                      `json:"resource_id"`
	ExpectedResourceVersion int                         `json:"expected_resource_version"`
	WorkflowID              string                      `json:"workflow_id"`
	WorkflowVersion         int                         `json:"workflow_version"`
	AgentNodeID             string                      `json:"agent_node_id"`
	ToolProfileID           string                      `json:"tool_profile_id"`
	ExpectedToolVersion     int                         `json:"expected_tool_version"`
	Knowledge               []ProjectKnowledgeReference `json:"knowledge"`
}

type ExpertTeamRole struct {
	Key                string                      `json:"key"`
	Title              string                      `json:"title"`
	ResourceID         string                      `json:"resource_id"`
	ResourceVersion    int                         `json:"resource_version"`
	ResourceKind       string                      `json:"resource_kind"`
	ResourceName       string                      `json:"resource_name"`
	MembershipID       string                      `json:"membership_id"`
	MembershipVersion  int                         `json:"membership_version"`
	WorkflowID         string                      `json:"workflow_id"`
	WorkflowVersion    int                         `json:"workflow_version"`
	AgentNodeID        string                      `json:"agent_node_id"`
	ToolProfileID      string                      `json:"tool_profile_id"`
	ToolProfileVersion int                         `json:"tool_profile_version"`
	ToolProfileSHA256  string                      `json:"tool_profile_sha256"`
	ToolName           string                      `json:"tool_name"`
	Knowledge          []ProjectKnowledgeReference `json:"knowledge"`
}

type ExpertTeamConfiguration struct {
	ProjectID   string           `json:"project_id"`
	TeamID      string           `json:"team_id"`
	TeamName    string           `json:"team_name"`
	TeamVersion int              `json:"team_version"`
	Version     int              `json:"version"`
	State       string           `json:"state"`
	Roles       []ExpertTeamRole `json:"roles"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
	Valid       bool             `json:"valid"`
	Blockers    []string         `json:"blockers"`
	CanEdit     bool             `json:"can_edit"`
	CanPublish  bool             `json:"can_publish"`
	CanRetire   bool             `json:"can_retire"`
}

type ExpertTeamSaveRequest struct {
	ProjectID       string                  `json:"project_id"`
	ExpectedVersion int                     `json:"expected_version"`
	Roles           []ExpertTeamRoleRequest `json:"roles"`
	IdempotencyKey  string                  `json:"idempotency_key"`
}

type ExpertTeamCommandRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotency_key"`
}

type ExpertTeamConfigurationPage struct {
	Items      []ExpertTeamConfiguration `json:"items"`
	Total      int                       `json:"total"`
	NextCursor string                    `json:"next_cursor,omitempty"`
}

type ExpertTeamPlanItemRequest struct {
	TaskID                  string                   `json:"task_id"`
	ExpectedTaskVersion     int                      `json:"expected_task_version"`
	RoleKey                 string                   `json:"role_key"`
	ModelExecutionSelection *modelcontract.Selection `json:"model_execution_selection,omitempty"`
}

type ExpertTeamPlanCreateRequest struct {
	ProjectID                    string                      `json:"project_id"`
	TeamID                       string                      `json:"team_id"`
	ExpectedConfigurationVersion int                         `json:"expected_configuration_version"`
	Items                        []ExpertTeamPlanItemRequest `json:"items"`
	IdempotencyKey               string                      `json:"idempotency_key"`
}

type ExpertTeamPlanItem struct {
	TaskID          string                     `json:"task_id"`
	TaskVersion     int                        `json:"task_version"`
	TaskBasisSHA256 string                     `json:"task_basis_sha256"`
	SourceRevision  int                        `json:"source_revision"`
	DependsOn       []string                   `json:"depends_on"`
	Dependencies    []ExpertTeamDependencyFact `json:"dependencies"`
	State           string                     `json:"state"`
	Blockers        []string                   `json:"blockers"`
	DispatchPlanID  string                     `json:"dispatch_plan_id,omitempty"`
	RunID           string                     `json:"run_id,omitempty"`
	RoleKey         string                     `json:"role_key"`
	RoleTitle       string                     `json:"role_title"`
	ResourceID      string                     `json:"resource_id"`
	ResourceName    string                     `json:"resource_name"`
	Plan            TaskDispatchPlan           `json:"plan"`
	RunState        string                     `json:"run_state"`
}

type ExpertTeamPlan struct {
	ID                   string                  `json:"id"`
	ProjectID            string                  `json:"project_id"`
	TeamID               string                  `json:"team_id"`
	TeamName             string                  `json:"team_name"`
	Version              int                     `json:"version"`
	Revision             int                     `json:"revision"`
	GraphSHA256          string                  `json:"graph_sha256"`
	ActivationGeneration int                     `json:"activation_generation"`
	State                string                  `json:"state"`
	ConfigurationVersion int                     `json:"configuration_version"`
	Configuration        ExpertTeamConfiguration `json:"configuration"`
	Items                []ExpertTeamPlanItem    `json:"items"`
	CreatedAt            string                  `json:"created_at"`
	ExpiresAt            string                  `json:"expires_at"`
	Valid                bool                    `json:"valid"`
	Blockers             []string                `json:"blockers"`
	CanStart             bool                    `json:"can_start"`
	AllowedActions       []string                `json:"allowed_actions"`
	Progress             ExpertTeamPlanProgress  `json:"progress"`
}

type ExpertTeamDependencyFact struct {
	TaskID      string `json:"task_id"`
	Selected    bool   `json:"selected"`
	Accepted    bool   `json:"accepted"`
	TaskVersion int    `json:"task_version"`
	RunID       string `json:"run_id,omitempty"`
}

type ExpertTeamPlanProgress struct {
	Total          int `json:"total"`
	Waiting        int `json:"waiting"`
	Ready          int `json:"ready"`
	Executing      int `json:"executing"`
	AwaitingReview int `json:"awaiting_review"`
	Accepted       int `json:"accepted"`
	Blocked        int `json:"blocked"`
	Stopped        int `json:"stopped"`
}

type ExpertTeamPlanCommandRequest struct {
	ProjectID                    string                      `json:"project_id"`
	ExpectedVersion              int                         `json:"expected_version"`
	Action                       string                      `json:"action"`
	Reason                       string                      `json:"reason"`
	Acknowledge                  bool                        `json:"acknowledge"`
	ExpectedConfigurationVersion int                         `json:"expected_configuration_version,omitempty"`
	Items                        []ExpertTeamPlanItemRequest `json:"items,omitempty"`
	IdempotencyKey               string                      `json:"idempotency_key"`
}

type ExpertTeamPlanPage struct {
	Items      []ExpertTeamPlan `json:"items"`
	Total      int              `json:"total"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

type ExpertTeamPlanStartRequest struct {
	ProjectID           string `json:"project_id"`
	ExpectedVersion     int    `json:"expected_version"`
	AcknowledgeDispatch bool   `json:"acknowledge_dispatch"`
	IdempotencyKey      string `json:"idempotency_key"`
}
