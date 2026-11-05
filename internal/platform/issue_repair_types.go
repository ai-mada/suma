package platform

// IssueRepair is the domain-owned repair cycle. Editable Record.Data never
// establishes repair lineage, regression scope, or authority to close an issue.
type IssueRepair struct {
	ID            string                  `json:"id"`
	TenantID      string                  `json:"tenant_id"`
	ProjectID     string                  `json:"project_id"`
	IssueID       string                  `json:"issue_id"`
	Cycle         int                     `json:"cycle"`
	Version       int                     `json:"version"`
	RequirementID string                  `json:"requirement_id"`
	TaskID        string                  `json:"task_id"`
	PreparedBy    string                  `json:"prepared_by"`
	CreatedAt     string                  `json:"created_at"`
	UpdatedAt     string                  `json:"updated_at"`
	Regression    *IssueRegressionBinding `json:"regression,omitempty"`
	Closure       *IssueRepairClosure     `json:"closure,omitempty"`
	ReopenedAt    string                  `json:"reopened_at,omitempty"`
	ReopenedBy    string                  `json:"reopened_by,omitempty"`
	ReopenReason  string                  `json:"reopen_reason,omitempty"`
}

type IssueRegressionBinding struct {
	PlanID       string   `json:"plan_id"`
	PlanRevision int      `json:"plan_revision"`
	PlanSHA256   string   `json:"plan_sha256"`
	CaseKeys     []string `json:"case_keys"`
	BoundBy      string   `json:"bound_by"`
	BoundAt      string   `json:"bound_at"`
}

// Closure freezes the exact accepted delivery and regression evidence. Reopening
// preserves this snapshot and starts a fresh cycle rather than rewriting history.
type IssueRepairClosure struct {
	RunID             string                   `json:"run_id"`
	TaskVersion       int                      `json:"task_version"`
	ReviewID          string                   `json:"review_id"`
	PlanID            string                   `json:"plan_id"`
	PlanRevision      int                      `json:"plan_revision"`
	PlanSHA256        string                   `json:"plan_sha256"`
	CaseKeys          []string                 `json:"case_keys"`
	SourceCommits     []string                 `json:"source_commits"`
	ClosedBy          string                   `json:"closed_by"`
	ClosedAt          string                   `json:"closed_at"`
	Reason            string                   `json:"reason"`
	PackageRegression *IssueRepairPackageProof `json:"package_regression,omitempty"`
}

// IssueRepairPackageOrigin is read from the protected defect and original
// build evidence. It remains historical when the application advances.
type IssueRepairPackageOrigin struct {
	RunID          string `json:"run_id"`
	NodeID         string `json:"node_id"`
	Attempt        int    `json:"attempt"`
	Fence          int64  `json:"fence"`
	ArtifactID     string `json:"artifact_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	SourceCommit   string `json:"source_commit"`
	ApplicationID  string `json:"application_id,omitempty"`
	ScopeID        string `json:"scope_id,omitempty"`
	ScopeRoot      string `json:"scope_root,omitempty"`
	RepositoryID   string `json:"repository_id,omitempty"`
}

// IssueRepairPackageProof retains the verified package attempt in the closed
// cycle, whose RunID supplies the parent run for this proof.
type IssueRepairPackageProof struct {
	NodeID         string `json:"node_id"`
	Attempt        int    `json:"attempt"`
	Fence          int64  `json:"fence"`
	ArtifactID     string `json:"artifact_id"`
	ManifestSHA256 string `json:"manifest_sha256"`
	SourceCommit   string `json:"source_commit"`
	ProofID        string `json:"proof_id"`
}

type IssueRepairBlocker struct {
	Code  string `json:"code"`
	RefID string `json:"ref_id,omitempty"`
}

type IssueRepairView struct {
	Issue           Record                    `json:"issue"`
	Cycle           int                       `json:"cycle"`
	Current         *IssueRepair              `json:"current,omitempty"`
	Task            *Record                   `json:"task,omitempty"`
	History         []IssueRepair             `json:"history"`
	HistoryTotal    int                       `json:"history_total"`
	HistoryComplete bool                      `json:"history_complete"`
	CanPrepare      bool                      `json:"can_prepare"`
	CanBind         bool                      `json:"can_bind"`
	CanClose        bool                      `json:"can_close"`
	CanReopen       bool                      `json:"can_reopen"`
	Blockers        []IssueRepairBlocker      `json:"blockers"`
	OriginalPackage *IssueRepairPackageOrigin `json:"original_package,omitempty"`
}

type IssueRepairPrepareRequest struct {
	ProjectID            string   `json:"project_id"`
	ExpectedIssueVersion int      `json:"expected_issue_version"`
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	OwnerID              string   `json:"owner_id"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	Reason               string   `json:"reason"`
	IdempotencyKey       string   `json:"idempotency_key"`
}

type IssueRepairBindRequest struct {
	ProjectID             string   `json:"project_id"`
	ExpectedIssueVersion  int      `json:"expected_issue_version"`
	ExpectedRepairVersion int      `json:"expected_repair_version"`
	ExpectedPlanVersion   int      `json:"expected_plan_version"`
	PlanID                string   `json:"plan_id"`
	CaseKeys              []string `json:"case_keys"`
	Reason                string   `json:"reason"`
	IdempotencyKey        string   `json:"idempotency_key"`
}

type IssueRepairCloseRequest struct {
	ProjectID             string `json:"project_id"`
	ExpectedIssueVersion  int    `json:"expected_issue_version"`
	ExpectedRepairVersion int    `json:"expected_repair_version"`
	RunID                 string `json:"run_id"`
	Reason                string `json:"reason"`
	IdempotencyKey        string `json:"idempotency_key"`
}

type IssueRepairReopenRequest struct {
	ProjectID             string `json:"project_id"`
	ExpectedIssueVersion  int    `json:"expected_issue_version"`
	ExpectedRepairVersion int    `json:"expected_repair_version"`
	Reason                string `json:"reason"`
	IdempotencyKey        string `json:"idempotency_key"`
}
