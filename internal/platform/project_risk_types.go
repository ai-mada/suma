package platform

// Risk references are domain-owned links to real records in this project.
// Editable metadata in tasks, requirements or issues never creates a risk link.
type ProjectRiskReference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type ProjectRiskFields struct {
	Title       string                 `json:"title"`
	Detail      string                 `json:"detail"`
	OwnerID     string                 `json:"owner_id"`
	ReviewDate  string                 `json:"review_date"`
	Strategy    string                 `json:"strategy"`
	Plan        string                 `json:"plan"`
	Probability *int                   `json:"probability"`
	Impact      *int                   `json:"impact"`
	Links       []ProjectRiskReference `json:"links"`
}

type ProjectRiskSource struct {
	AlertID              string `json:"alert_id"`
	AlertVersion         int    `json:"alert_version"`
	AnalysisID           string `json:"analysis_id"`
	RunID                string `json:"run_id"`
	RiskKey              string `json:"risk_key"`
	SourceSHA256         string `json:"source_sha256"`
	OriginalOutputSHA256 string `json:"original_output_sha256"`
	AdoptedBy            string `json:"adopted_by"`
	AdoptedAt            string `json:"adopted_at"`
}

type ProjectRisk struct {
	ID                 string `json:"id"`
	TenantID           string `json:"tenant_id"`
	ProjectID          string `json:"project_id"`
	Version            int    `json:"version"`
	Cycle              int    `json:"cycle"`
	AssessmentRevision int    `json:"assessment_revision"`
	State              string `json:"state"`
	ProjectRiskFields
	Score           *int               `json:"score"`
	Source          *ProjectRiskSource `json:"source,omitempty"`
	CurrentReviewID string             `json:"current_review_id,omitempty"`
	CreatedBy       string             `json:"created_by"`
	CreatedAt       string             `json:"created_at"`
	UpdatedAt       string             `json:"updated_at"`
}

type ProjectRiskLinkBasis struct {
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	Version       int    `json:"version"`
	VerifiedRunID string `json:"verified_run_id,omitempty"`
	ReviewID      string `json:"review_id,omitempty"`
}

type ProjectRiskReviewInput struct {
	Disposition         string `json:"disposition"`
	ResidualProbability *int   `json:"residual_probability"`
	ResidualImpact      *int   `json:"residual_impact"`
	Rationale           string `json:"rationale"`
	Evidence            string `json:"evidence"`
}

// A review freezes its assessment, current human owner and exact linked record
// versions. Human acceptance of exposure is distinct from verified mitigation.
type ProjectRiskReview struct {
	ID                 string                 `json:"id"`
	Cycle              int                    `json:"cycle"`
	AssessmentRevision int                    `json:"assessment_revision"`
	AssessmentSHA256   string                 `json:"assessment_sha256"`
	OwnerID            string                 `json:"owner_id"`
	OwnerVersion       int                    `json:"owner_version"`
	OwnerUserID        string                 `json:"owner_user_id"`
	Links              []ProjectRiskLinkBasis `json:"links"`
	ProjectRiskReviewInput
	ResidualScore  *int   `json:"residual_score"`
	SubmittedBy    string `json:"submitted_by"`
	SubmittedAt    string `json:"submitted_at"`
	Decision       string `json:"decision"`
	DecidedBy      string `json:"decided_by,omitempty"`
	DecidedAt      string `json:"decided_at,omitempty"`
	DecisionReason string `json:"decision_reason,omitempty"`
}

type ProjectRiskBlocker struct {
	Code  string `json:"code"`
	RefID string `json:"ref_id,omitempty"`
}

type ProjectRiskReadiness struct {
	AssessmentReady bool                 `json:"assessment_ready"`
	MitigationReady bool                 `json:"mitigation_ready"`
	CloseReady      bool                 `json:"close_ready"`
	Blockers        []ProjectRiskBlocker `json:"blockers"`
}

type ProjectRiskView struct {
	Risk            ProjectRisk          `json:"risk"`
	CurrentReview   *ProjectRiskReview   `json:"current_review,omitempty"`
	History         []ProjectRiskReview  `json:"history"`
	HistoryTotal    int                  `json:"history_total"`
	HistoryComplete bool                 `json:"history_complete"`
	AllowedActions  []string             `json:"allowed_actions"`
	Readiness       ProjectRiskReadiness `json:"readiness"`
	Created         *bool                `json:"created,omitempty" wire:"nonnull"`
}

type ProjectRiskPage struct {
	Items      []ProjectRisk `json:"items"`
	Total      int           `json:"total"`
	NextCursor string        `json:"next_cursor"`
	CanCreate  bool          `json:"can_create"`
}

type ProjectRiskMatrixCell struct {
	Probability int `json:"probability"`
	Impact      int `json:"impact"`
	Count       int `json:"count"`
}

type ProjectRiskSummary struct {
	ProjectID       string                  `json:"project_id"`
	AsOf            string                  `json:"as_of"`
	Total           int                     `json:"total"`
	ActiveCount     int                     `json:"active_count"`
	OpenCount       int                     `json:"open_count"`
	MitigatingCount int                     `json:"mitigating_count"`
	InReviewCount   int                     `json:"in_review_count"`
	ClosedCount     int                     `json:"closed_count"`
	UnassessedCount int                     `json:"unassessed_count"`
	OverdueCount    int                     `json:"overdue_count"`
	Matrix          []ProjectRiskMatrixCell `json:"matrix"`
	MatrixScope     string                  `json:"matrix_scope"`
}

type ProjectRiskCreateRequest struct {
	ProjectID string `json:"project_id"`
	ProjectRiskFields
	SourceAlertID        string `json:"source_alert_id,omitempty"`
	ExpectedAlertVersion int    `json:"expected_alert_version,omitempty"`
	Reason               string `json:"reason"`
	IdempotencyKey       string `json:"idempotency_key"`
}

type ProjectRiskSaveRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	ProjectRiskFields
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type ProjectRiskCommandRequest struct {
	ProjectID        string                  `json:"project_id"`
	ExpectedVersion  int                     `json:"expected_version"`
	Action           string                  `json:"action"`
	Review           *ProjectRiskReviewInput `json:"review,omitempty" wire:"nonnull"`
	ExpectedReviewID string                  `json:"expected_review_id,omitempty"`
	Reason           string                  `json:"reason"`
	IdempotencyKey   string                  `json:"idempotency_key"`
}
