package platform

// ApplicationQualityExecutionInput selects one already persisted task/run.
// Project and tenant come only from the authenticated application transport.
// An omitted limit is 10; explicit limits are 1..20. AfterCase is an actual
// key in the frozen revision's case order, never an offset into a new plan.
type ApplicationQualityExecutionInput struct {
	TaskID    string `json:"task_id"`
	RunID     string `json:"run_id"`
	Limit     int    `json:"limit,omitempty"`
	AfterCase string `json:"after_case,omitempty"`
}

// ApplicationQualityEvidenceReference intentionally excludes report XML,
// diagnostic messages, file paths, process output and command configuration.
type ApplicationQualityEvidenceReference struct {
	ReportID     string `json:"report_id"`
	SourceCommit string `json:"source_commit"`
	NodeID       string `json:"node_id"`
	Attempt      int    `json:"attempt"`
	Fence        int64  `json:"fence"`
	WorkerID     string `json:"worker_id"`
	Status       string `json:"status"`
}

// ApplicationQualityCase is a compact observation, not a mutation or grant.
// EvidenceTotal counts all matching facts; at most three references are sent.
type ApplicationQualityCase struct {
	CaseKey          string                                `json:"case_key"`
	Title            string                                `json:"title"`
	Status           string                                `json:"status"`
	Covers           []int                                 `json:"covers"`
	IssueID          string                                `json:"issue_id"`
	Evidence         []ApplicationQualityEvidenceReference `json:"evidence"`
	EvidenceTotal    int                                   `json:"evidence_total"`
	EvidenceComplete bool                                  `json:"evidence_complete"`
}

// ApplicationQualityExecutionPage preserves the quality domain's current
// verdict and immutable source, while paging only its ordered case projection.
// Complete refers to the case page; it never means business acceptance.
type ApplicationQualityExecutionPage struct {
	ProjectID             string                   `json:"project_id"`
	TaskID                string                   `json:"task_id"`
	PlanID                string                   `json:"plan_id"`
	RunID                 string                   `json:"run_id"`
	Revision              int                      `json:"revision"`
	PlanSHA256            string                   `json:"plan_sha256"`
	SourceSHA256          string                   `json:"source_sha256"`
	SourceCommits         []string                 `json:"source_commits"`
	SourceCommitTotal     int                      `json:"source_commit_total"`
	SourceCommitsComplete bool                     `json:"source_commits_complete"`
	RunState              string                   `json:"run_state"`
	State                 string                   `json:"state"`
	BasisCurrent          bool                     `json:"basis_current"`
	Total                 int                      `json:"total"`
	Passed                int                      `json:"passed"`
	Failed                int                      `json:"failed"`
	Errors                int                      `json:"errors"`
	Skipped               int                      `json:"skipped"`
	NotRun                int                      `json:"not_run"`
	Limit                 int                      `json:"limit"`
	Complete              bool                     `json:"complete"`
	NextCase              string                   `json:"next_case,omitempty"`
	Cases                 []ApplicationQualityCase `json:"cases"`
}
