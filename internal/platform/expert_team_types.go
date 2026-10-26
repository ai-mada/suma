package platform

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
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	RoleKey             string `json:"role_key"`
}

type ExpertTeamPlanCreateRequest struct {
	ProjectID                    string                      `json:"project_id"`
	TeamID                       string                      `json:"team_id"`
	ExpectedConfigurationVersion int                         `json:"expected_configuration_version"`
	Items                        []ExpertTeamPlanItemRequest `json:"items"`
	IdempotencyKey               string                      `json:"idempotency_key"`
}

type ExpertTeamPlanItem struct {
	RoleKey      string           `json:"role_key"`
	RoleTitle    string           `json:"role_title"`
	ResourceID   string           `json:"resource_id"`
	ResourceName string           `json:"resource_name"`
	Plan         TaskDispatchPlan `json:"plan"`
	RunState     string           `json:"run_state"`
}

type ExpertTeamPlan struct {
	ID                   string                  `json:"id"`
	ProjectID            string                  `json:"project_id"`
	TeamID               string                  `json:"team_id"`
	TeamName             string                  `json:"team_name"`
	Version              int                     `json:"version"`
	State                string                  `json:"state"`
	ConfigurationVersion int                     `json:"configuration_version"`
	Configuration        ExpertTeamConfiguration `json:"configuration"`
	Items                []ExpertTeamPlanItem    `json:"items"`
	CreatedAt            string                  `json:"created_at"`
	ExpiresAt            string                  `json:"expires_at"`
	Valid                bool                    `json:"valid"`
	Blockers             []string                `json:"blockers"`
	CanStart             bool                    `json:"can_start"`
}

type ExpertTeamPlanStartRequest struct {
	ProjectID           string `json:"project_id"`
	ExpectedVersion     int    `json:"expected_version"`
	AcknowledgeDispatch bool   `json:"acknowledge_dispatch"`
	IdempotencyKey      string `json:"idempotency_key"`
}
