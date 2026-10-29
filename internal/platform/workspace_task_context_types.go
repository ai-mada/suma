package platform

// WorkspaceTaskContext is a current, read-only task view. Selecting a task in
// the IDE does not bind a local directory to its code execution or authorize a
// save, dispatch, test, review, terminal, or background process.
type WorkspaceTaskContext struct {
	ProjectID   string               `json:"project_id"`
	ObservedAt  string               `json:"observed_at"`
	Task        WorkspaceTask        `json:"task"`
	Requirement *TaskContextRecord   `json:"requirement"`
	Owner       *WorkspaceTaskOwner  `json:"owner"`
	Design      WorkspaceTaskDesign  `json:"design"`
	Delivery    TaskDelivery         `json:"delivery"`
	Quality     WorkspaceTaskQuality `json:"quality"`
}
type WorkspaceTask struct {
	ID                 string   `json:"id"`
	Version            int      `json:"version"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	Status             string   `json:"status"`
	Priority           string   `json:"priority"`
	OwnerID            string   `json:"owner_id"`
	RequirementID      string   `json:"requirement_id"`
	RunID              string   `json:"run_id"`
}
type WorkspaceTaskOwner struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
}
type WorkspaceTaskDesign struct {
	State   string                    `json:"state"`
	Context *RequirementDesignContext `json:"context"`
}

// State and counts reuse the original published-plan projection. They do not
// assert independent code testing or enlarge formal acceptance eligibility.
type WorkspaceTaskQuality struct {
	PlanID       string `json:"plan_id"`
	Revision     int    `json:"revision"`
	PlanSHA256   string `json:"plan_sha256"`
	RunID        string `json:"run_id"`
	RunState     string `json:"run_state"`
	State        string `json:"state"`
	BasisCurrent bool   `json:"basis_current"`
	Total        int    `json:"total"`
	Passed       int    `json:"passed"`
	Failed       int    `json:"failed"`
	Errors       int    `json:"errors"`
	Skipped      int    `json:"skipped"`
	NotRun       int    `json:"not_run"`
}
type WorkspaceTaskOption struct {
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Title    string `json:"title"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}
type WorkspaceTaskPage struct {
	ProjectID  string                `json:"project_id"`
	Search     string                `json:"search"`
	Items      []WorkspaceTaskOption `json:"items"`
	Total      int                   `json:"total"`
	Limit      int                   `json:"limit"`
	NextCursor string                `json:"next_cursor"`
}
