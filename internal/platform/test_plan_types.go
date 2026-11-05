package platform

import tr "github.com/ai-mada/suma/internal/testreportcontract"

// Keep model/task context compact. Full cases stay behind the quality read API;
// this immutable reference avoids spending model context on a complete report.
type TaskTestPlanContext struct {
	Revision     int    `json:"revision"`
	SHA256       string `json:"sha256"`
	SourceSHA256 string `json:"source_sha256"`
	CaseCount    int    `json:"case_count"`
	Title        string `json:"title"`
}

// Test cases describe intended checks. Only a captured, exact-attempt report
// supplies execution facts; editing a case can never create a passing result.
type QualityTestCase struct {
	Key            string              `json:"key"`
	Title          string              `json:"title"`
	Description    string              `json:"description"`
	Preconditions  string              `json:"preconditions"`
	Steps          []string            `json:"steps"`
	ExpectedResult string              `json:"expected_result"`
	Priority       string              `json:"priority"`
	Covers         []int               `json:"covers"`
	Selector       tr.TestCaseSelector `json:"selector"`
}

type TestPlanSource struct {
	Task            TaskContextRecord  `json:"task"`
	TaskBasisSHA256 string             `json:"task_basis_sha256"`
	Requirement     *TaskContextRecord `json:"requirement,omitempty"`
	SHA256          string             `json:"sha256"`
}

type TestPlanRevision struct {
	Revision    int               `json:"revision"`
	State       string            `json:"state"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Source      TestPlanSource    `json:"source"`
	Cases       []QualityTestCase `json:"cases"`
	SHA256      string            `json:"sha256"`
	CreatedBy   string            `json:"created_by"`
	CreatedAt   string            `json:"created_at"`
}

type TestPlanView struct {
	ID                string            `json:"id"`
	ProjectID         string            `json:"project_id"`
	TaskID            string            `json:"task_id"`
	Version           int               `json:"version"`
	State             string            `json:"state"`
	LatestRevision    int               `json:"latest_revision"`
	PublishedRevision int               `json:"published_revision"`
	HasDraft          bool              `json:"has_draft"`
	BasisCurrent      bool              `json:"basis_current"`
	Eligibility       string            `json:"eligibility"`
	Latest            *TestPlanRevision `json:"latest,omitempty"`
	Published         *TestPlanRevision `json:"published,omitempty"`
	AllowedActions    []string          `json:"allowed_actions"`
	CreatedAt         string            `json:"created_at"`
	UpdatedAt         string            `json:"updated_at"`
}

type TestPlanList struct {
	Items      []TestPlanView `json:"items"`
	Total      int            `json:"total"`
	NextCursor string         `json:"next_cursor"`
}

type TestPlanCreateRequest struct {
	ProjectID                  string            `json:"project_id"`
	TaskID                     string            `json:"task_id"`
	ExpectedTaskVersion        int               `json:"expected_task_version"`
	ExpectedRequirementVersion int               `json:"expected_requirement_version"`
	Title                      string            `json:"title"`
	Description                string            `json:"description"`
	Cases                      []QualityTestCase `json:"cases"`
	IdempotencyKey             string            `json:"idempotency_key"`
}

type TestPlanSaveRequest struct {
	ProjectID                  string            `json:"project_id"`
	ExpectedVersion            int               `json:"expected_version"`
	ExpectedTaskVersion        int               `json:"expected_task_version"`
	ExpectedRequirementVersion int               `json:"expected_requirement_version"`
	Title                      string            `json:"title"`
	Description                string            `json:"description"`
	Cases                      []QualityTestCase `json:"cases"`
	IdempotencyKey             string            `json:"idempotency_key"`
}

type TestPlanCommandRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotency_key"`
}

// Ordinary run lists carry no report XML. Read original reports explicitly
// through the authorized quality domain, not by copying them into every node.
type TestPlanReportRef struct {
	ID              string `json:"id"`
	RunID           string `json:"run_id"`
	NodeID          string `json:"node_id"`
	Attempt         int    `json:"attempt"`
	Fence           int64  `json:"fence"`
	WorkerID        string `json:"worker_id"`
	SourceCommit    string `json:"source_commit"`
	SHA256          string `json:"sha256"`
	Protocol        string `json:"protocol"`
	Eligibility     string `json:"eligibility"`
	ProcessExitCode int    `json:"process_exit_code"`
	OutcomeUnknown  bool   `json:"outcome_unknown"`
	ReportedAt      string `json:"reported_at"`
	Total           int    `json:"total"`
	Passed          int    `json:"passed"`
	Failed          int    `json:"failed"`
	Errors          int    `json:"errors"`
	Skipped         int    `json:"skipped"`
	Unmapped        int    `json:"unmapped"`
}

type TestPlanReport struct {
	TestPlanReportRef
	XML   string              `json:"xml"`
	Cases []tr.TestReportCase `json:"cases"`
}

type TestCaseEvidence struct {
	ReportID     string `json:"report_id"`
	NodeID       string `json:"node_id"`
	Attempt      int    `json:"attempt"`
	Fence        int64  `json:"fence"`
	WorkerID     string `json:"worker_id"`
	SourceCommit string `json:"source_commit"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	DurationMS   int64  `json:"duration_ms"`
}

type TestCaseResult struct {
	CaseKey  string             `json:"case_key"`
	Title    string             `json:"title"`
	Status   string             `json:"status"`
	Covers   []int              `json:"covers"`
	Evidence []TestCaseEvidence `json:"evidence"`
	IssueID  string             `json:"issue_id"`
}

// TestPlanPackageEvidence is the bounded gate projection for a sealed package.
// It is independent of JUnit case counts. Artifact fields require a verified
// current proof for the exact package attempt and its build ancestor.
type TestPlanPackageEvidence struct {
	NodeID         string `json:"node_id"`
	SourceNodeID   string `json:"source_node_id"`
	State          string `json:"state"`
	Attempt        int    `json:"attempt,omitempty"`
	WorkerID       string `json:"worker_id,omitempty"`
	SourceWorkerID string `json:"source_worker_id,omitempty"`
	ArtifactID     string `json:"artifact_id,omitempty"`
	ManifestSHA256 string `json:"manifest_sha256,omitempty"`
	SourceCommit   string `json:"source_commit,omitempty"`
}

type TestPlanExecution struct {
	PlanID          string                    `json:"plan_id"`
	TaskID          string                    `json:"task_id"`
	Revision        int                       `json:"revision"`
	PlanSHA256      string                    `json:"plan_sha256"`
	RunID           string                    `json:"run_id"`
	RunState        string                    `json:"run_state"`
	CreatedAt       string                    `json:"created_at"`
	State           string                    `json:"state"`
	BasisCurrent    bool                      `json:"basis_current"`
	SourceCommits   []string                  `json:"source_commits"`
	Total           int                       `json:"total"`
	Passed          int                       `json:"passed"`
	Failed          int                       `json:"failed"`
	Errors          int                       `json:"errors"`
	Skipped         int                       `json:"skipped"`
	NotRun          int                       `json:"not_run"`
	Cases           []TestCaseResult          `json:"cases"`
	Reports         []TestPlanReportRef       `json:"reports"`
	PackageEvidence []TestPlanPackageEvidence `json:"package_evidence"`
}

type TestPlanExecutionList struct {
	Items      []TestPlanExecution `json:"items"`
	Total      int                 `json:"total"`
	NextCursor string              `json:"next_cursor"`
}

type TestPlanDefectRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	CaseKey         string `json:"case_key"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotency_key"`
}
