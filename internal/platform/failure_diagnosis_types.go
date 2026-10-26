package platform

import "github.com/ai-mada/suma/internal/localization"

type FailureRepairDraft struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Priority           string   `json:"priority"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type FailureDiagnosisObservation struct {
	Statement       string `json:"statement"`
	EvidenceIndices []int  `json:"evidence_indices"`
}
type FailureDiagnosisHypothesis struct {
	Statement       string `json:"statement"`
	EvidenceIndices []int  `json:"evidence_indices"`
	Confidence      string `json:"confidence"`
}
type FailureDiagnosisOutput struct {
	SchemaVersion      int                           `json:"schema_version"`
	Summary            string                        `json:"summary"`
	Observations       []FailureDiagnosisObservation `json:"observations"`
	Hypotheses         []FailureDiagnosisHypothesis  `json:"hypotheses"`
	RecommendedActions []string                      `json:"recommended_actions"`
	TestPlan           []string                      `json:"test_plan"`
	Repair             FailureRepairDraft            `json:"repair"`
}
type FailureDiagnosisEvidence struct {
	Kind          string `json:"kind"`
	Text          string `json:"text"`
	SHA256        string `json:"sha256"`
	TotalBytes    int    `json:"total_bytes"`
	SelectedBytes int    `json:"selected_bytes"`
	Selection     string `json:"selection"`
}
type FailureDiagnosisSource struct {
	SHA256             string                     `json:"sha256"`
	IssueID            string                     `json:"issue_id"`
	IssueVersion       int                        `json:"issue_version"`
	RunID              string                     `json:"run_id"`
	RunVersion         int                        `json:"run_version"`
	RunState           string                     `json:"run_state"`
	NodeID             string                     `json:"node_id"`
	NodeState          string                     `json:"node_state"`
	Attempt            int                        `json:"attempt"`
	Fence              int64                      `json:"fence"`
	WorkerID           string                     `json:"worker_id"`
	TaskID             string                     `json:"task_id,omitempty"`
	TaskVersion        int                        `json:"task_version,omitempty"`
	TaskTitle          string                     `json:"task_title,omitempty"`
	RequirementID      string                     `json:"requirement_id,omitempty"`
	RequirementVersion int                        `json:"requirement_version,omitempty"`
	Evidence           []FailureDiagnosisEvidence `json:"evidence"`
}
type FailureDiagnosisContext struct {
	// Read-only display metadata; Blockers retain their canonical source bytes.
	BlockerDiagnostics []*localization.Rendered `json:"blocker_diagnostics,omitempty"`
	ProjectID          string                   `json:"project_id"`
	RunID              string                   `json:"run_id"`
	NodeID             string                   `json:"node_id"`
	Attempt            int                      `json:"attempt"`
	Source             *FailureDiagnosisSource  `json:"source,omitempty"`
	Issue              *Record                  `json:"issue,omitempty"`
	Requirement        *Record                  `json:"requirement,omitempty"`
	Eligible           bool                     `json:"eligible"`
	CanCreate          bool                     `json:"can_create"`
	Blockers           []string                 `json:"blockers"`
}
type FailureDiagnosisAnalysis struct {
	// Present only for platform-owned warnings, never a human decision reason.
	ReasonDiagnostic     *localization.Rendered  `json:"reason_diagnostic,omitempty"`
	ID                   string                  `json:"id"`
	ProjectID            string                  `json:"project_id"`
	Version              int                     `json:"version"`
	State                string                  `json:"state"`
	Goal                 string                  `json:"goal"`
	Locale               string                  `json:"locale"`
	CreatedBy            string                  `json:"created_by"`
	CreatedAt            string                  `json:"created_at"`
	UpdatedAt            string                  `json:"updated_at"`
	ToolProfileID        string                  `json:"tool_profile_id"`
	ToolProfileVersion   int                     `json:"tool_profile_version"`
	ToolProfileSHA256    string                  `json:"tool_profile_sha256"`
	RunID                string                  `json:"run_id"`
	MaxSteps             int                     `json:"max_steps"`
	MaxOutputTokens      int                     `json:"max_output_tokens"`
	Source               FailureDiagnosisSource  `json:"source"`
	Proposal             *FailureDiagnosisOutput `json:"proposal,omitempty"`
	Repair               *FailureRepairDraft     `json:"repair,omitempty"`
	OriginalOutput       string                  `json:"original_output"`
	OriginalOutputSHA256 string                  `json:"original_output_sha256"`
	AdoptedRequirementID string                  `json:"adopted_requirement_id"`
	Reason               string                  `json:"reason"`
	AllowedActions       []string                `json:"allowed_actions"`
	BasisCurrent         bool                    `json:"basis_current"`
}
type FailureDiagnosisCreateRequest struct {
	ProjectID            string `json:"project_id"`
	IssueID              string `json:"issue_id"`
	ExpectedIssueVersion int    `json:"expected_issue_version"`
	RunID                string `json:"run_id"`
	NodeID               string `json:"node_id"`
	Attempt              int    `json:"attempt"`
	ExpectedSourceSHA256 string `json:"expected_source_sha256"`
	Goal                 string `json:"goal"`
	Locale               string `json:"locale"`
	ToolProfileID        string `json:"tool_profile_id"`
	ExpectedToolVersion  int    `json:"expected_tool_version"`
	MaxSteps             int    `json:"max_steps,omitempty"`
	MaxOutputTokens      int    `json:"max_output_tokens,omitempty"`
	AcknowledgeModelUse  bool   `json:"acknowledge_model_use"`
	IdempotencyKey       string `json:"idempotency_key"`
}
type FailureDiagnosisCommandRequest struct {
	ProjectID        string              `json:"project_id"`
	ExpectedVersion  int                 `json:"expected_version"`
	Action           string              `json:"action"`
	Repair           *FailureRepairDraft `json:"repair,omitempty"`
	Reason           string              `json:"reason"`
	AcknowledgeDraft bool                `json:"acknowledge_draft"`
	IdempotencyKey   string              `json:"idempotency_key"`
}
type FailureDiagnosisPage struct {
	Items      []FailureDiagnosisAnalysis `json:"items"`
	Total      int                        `json:"total"`
	NextCursor string                     `json:"next_cursor"`
}
type FailureDiagnosisAdoption struct {
	AnalysisID           string `json:"analysis_id"`
	AnalysisVersion      int    `json:"analysis_version"`
	DiagnosisRunID       string `json:"diagnosis_run_id"`
	SourceRunID          string `json:"source_run_id"`
	SourceNodeID         string `json:"source_node_id"`
	SourceAttempt        int    `json:"source_attempt"`
	SourceFence          int64  `json:"source_fence"`
	IssueID              string `json:"issue_id"`
	ToolProfileSHA256    string `json:"tool_profile_sha256"`
	SourceSHA256         string `json:"source_sha256"`
	OriginalOutputSHA256 string `json:"original_output_sha256"`
	RepairSHA256         string `json:"repair_sha256"`
	RequirementID        string `json:"requirement_id"`
	RequirementVersion   int    `json:"requirement_version"`
	AdoptedBy            string `json:"adopted_by"`
	AdoptedAt            string `json:"adopted_at"`
	Reason               string `json:"reason"`
}
