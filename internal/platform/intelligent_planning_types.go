package platform

type PlanningScheduleAction struct {
	TaskID         string   `json:"task_id"`
	IterationID    string   `json:"iteration_id"`
	MilestoneID    string   `json:"milestone_id"`
	StartDate      string   `json:"start_date"`
	DueDate        string   `json:"due_date"`
	EstimatedHours *float64 `json:"estimated_hours"`
}

type PlanningRisk struct {
	Key      string   `json:"key"`
	Severity string   `json:"severity"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	TaskIDs  []string `json:"task_ids"`
}

type PlanningOutput struct {
	SchemaVersion int                      `json:"schema_version"`
	Summary       string                   `json:"summary"`
	Rationale     string                   `json:"rationale"`
	Actions       []PlanningScheduleAction `json:"actions"`
	Risks         []PlanningRisk           `json:"risks"`
}

type PlanningSourceTask struct {
	Task      TaskContextRecord `json:"task"`
	OwnerID   string            `json:"owner_id"`
	Status    string            `json:"status"`
	Priority  string            `json:"priority"`
	DependsOn []string          `json:"depends_on"`
	Plan      TaskPlan          `json:"plan"`
}

type PlanningSource struct {
	SHA256       string               `json:"sha256"`
	WeekStart    string               `json:"week_start"`
	Tasks        []PlanningSourceTask `json:"tasks"`
	Requirements []TaskContextRecord  `json:"requirements"`
	Capacity     []CapacityEntry      `json:"capacity"`
}

type PlanningAnalysis struct {
	ID                   string          `json:"id"`
	ProjectID            string          `json:"project_id"`
	Version              int             `json:"version"`
	State                string          `json:"state"`
	Goal                 string          `json:"goal"`
	Locale               string          `json:"locale"`
	CreatedBy            string          `json:"created_by"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	ToolProfileID        string          `json:"tool_profile_id"`
	ToolProfileVersion   int             `json:"tool_profile_version"`
	ToolProfileSHA256    string          `json:"tool_profile_sha256"`
	RunID                string          `json:"run_id"`
	MaxSteps             int             `json:"max_steps"`
	MaxOutputTokens      int             `json:"max_output_tokens"`
	Source               PlanningSource  `json:"source"`
	Proposal             *PlanningOutput `json:"proposal,omitempty"`
	OriginalOutputSHA256 string          `json:"original_output_sha256"`
	OriginalOutput       string          `json:"original_output"`
	Reason               string          `json:"reason"`
	AllowedActions       []string        `json:"allowed_actions"`
	BasisCurrent         bool            `json:"basis_current"`
}

type PlanningAnalysisCreateRequest struct {
	ProjectID           string   `json:"project_id"`
	Goal                string   `json:"goal"`
	Locale              string   `json:"locale"`
	TaskIDs             []string `json:"task_ids"`
	ToolProfileID       string   `json:"tool_profile_id"`
	ExpectedToolVersion int      `json:"expected_tool_version"`
	MaxSteps            int      `json:"max_steps"`
	MaxOutputTokens     int      `json:"max_output_tokens"`
	AcknowledgeModelUse bool     `json:"acknowledge_model_use"`
	IdempotencyKey      string   `json:"idempotency_key"`
}

type PlanningAnalysisCommandRequest struct {
	ProjectID                  string          `json:"project_id"`
	ExpectedVersion            int             `json:"expected_version"`
	Action                     string          `json:"action"`
	Proposal                   *PlanningOutput `json:"proposal,omitempty"`
	Reason                     string          `json:"reason"`
	AcknowledgeScheduleChanges bool            `json:"acknowledge_schedule_changes"`
	IdempotencyKey             string          `json:"idempotency_key"`
}

type PlanningAnalysisPage struct {
	Items      []PlanningAnalysis `json:"items"`
	Total      int                `json:"total"`
	NextCursor string             `json:"next_cursor"`
}

type PlanningAlert struct {
	ID             string   `json:"id"`
	ProjectID      string   `json:"project_id"`
	Version        int      `json:"version"`
	AnalysisID     string   `json:"analysis_id"`
	RunID          string   `json:"run_id"`
	RiskKey        string   `json:"risk_key"`
	Severity       string   `json:"severity"`
	Title          string   `json:"title"`
	Detail         string   `json:"detail"`
	TaskIDs        []string `json:"task_ids"`
	State          string   `json:"state"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	DecisionBy     string   `json:"decision_by"`
	DecisionAt     string   `json:"decision_at"`
	DecisionReason string   `json:"decision_reason"`
}

type PlanningAlertPage struct {
	Items         []PlanningAlert `json:"items"`
	Total         int             `json:"total"`
	NextCursor    string          `json:"next_cursor"`
	OpenCount     int             `json:"open_count"`
	CriticalCount int             `json:"critical_count"`
}

type PlanningAlertCommandRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotency_key"`
}
