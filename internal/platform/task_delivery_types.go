package platform

// TaskDelivery is a read-only projection of one current task and its exact
// bound run. It never grants review/dispatch authority or creates work on read.
type TaskDelivery struct {
	TestPlanID       string                  `json:"test_plan_id,omitempty"`
	ProjectID        string                  `json:"project_id"`
	TaskID           string                  `json:"task_id"`
	TaskVersion      int                     `json:"task_version"`
	TaskStatus       string                  `json:"task_status"`
	RunBasisState    string                  `json:"run_basis_state"`
	FormalAcceptance bool                    `json:"formal_acceptance"`
	Run              *TaskDeliveryRun        `json:"run"`
	ExpertPlans      TaskDeliveryExpertPlans `json:"expert_plans"`
	Issues           TaskDeliveryIssues      `json:"issues"`
}

type TaskDeliveryRun struct {
	RunID   string                  `json:"run_id"`
	Version int                     `json:"version"`
	State   string                  `json:"state"`
	Nodes   TaskDeliveryNodeSummary `json:"nodes"`
	Tests   TaskDeliveryNodeSummary `json:"tests"`
}

// Counts cover every node in the bound run; Items is an explicitly capped
// drill-down list. Test counts are execution facts, never approval eligibility.
type TaskDeliveryNodeSummary struct {
	Total       int                `json:"total"`
	Waiting     int                `json:"waiting"`
	Running     int                `json:"running"`
	Succeeded   int                `json:"succeeded"`
	Failed      int                `json:"failed"`
	Skipped     int                `json:"skipped"`
	Reconciling int                `json:"reconciling"`
	Other       int                `json:"other"`
	Limit       int                `json:"limit"`
	Complete    bool               `json:"complete"`
	Items       []TaskDeliveryNode `json:"items"`
}

type TaskDeliveryNode struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	State   string `json:"state"`
	Attempt int    `json:"attempt"`
}

// Scope is always current_run. Pending plans without that actual run binding
// are deliberately outside this projection rather than guessed from JSON data.
type TaskDeliveryExpertPlans struct {
	Scope    string                   `json:"scope"`
	Total    int                      `json:"total"`
	Limit    int                      `json:"limit"`
	Complete bool                     `json:"complete"`
	Items    []TaskDeliveryExpertPlan `json:"items"`
}

type TaskDeliveryExpertPlan struct {
	PlanID    string `json:"plan_id"`
	Version   int    `json:"version"`
	Revision  int    `json:"revision"`
	TeamID    string `json:"team_id"`
	TeamName  string `json:"team_name"`
	State     string `json:"state"`
	ItemState string `json:"item_state"`
}

// Issue references come from canonical failure/rejected-review operation slots.
// Editable issue Data is not evidence that an issue belongs to a delivery.
type TaskDeliveryIssues struct {
	Scope    string              `json:"scope"`
	Total    int                 `json:"total"`
	Limit    int                 `json:"limit"`
	Complete bool                `json:"complete"`
	Items    []TaskDeliveryIssue `json:"items"`
}

type TaskDeliveryIssue struct {
	IssueID            string `json:"issue_id"`
	Title              string `json:"title"`
	IssueVersion       int    `json:"issue_version"`
	IssueStatus        string `json:"issue_status"`
	Source             string `json:"source"`
	SourceReviewID     string `json:"source_review_id"`
	RequirementID      string `json:"requirement_id"`
	RequirementVersion int    `json:"requirement_version"`
}
