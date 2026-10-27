package platform

import (
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/modelcontract"
)

// Revision requests select current published execution inputs. The task and
// original sources are resolved from the protected reservation, never supplied.
type CandidateCompositionRepairRevisionRequest struct {
	ModelExecutionSelection *modelcontract.Selection `json:"model_execution_selection,omitempty" wire:"nonnull"`
	ProjectID               string                   `json:"project_id"`
	ExpectedVersion         int                      `json:"expected_version"`
	ExpectedTaskVersion     int                      `json:"expected_task_version"`
	ExpectedConflictSHA256  string                   `json:"expected_conflict_sha256"`
	WorkflowID              string                   `json:"workflow_id"`
	WorkflowVersion         int                      `json:"workflow_version"`
	ProducerNodeID          string                   `json:"producer_node_id"`
	ToolProfileID           string                   `json:"tool_profile_id"`
	ExpectedToolVersion     int                      `json:"expected_tool_version"`
	ExpectedRecoverySHA256  string                   `json:"expected_recovery_sha256,omitempty"`
	Reason                  string                   `json:"reason"`
	Acknowledge             bool                     `json:"acknowledge"`
	IdempotencyKey          string                   `json:"idempotency_key"`
}

type CandidateCompositionRepairRecoveryAttempt struct {
	Assignment           cc.AssignmentRef `json:"assignment"`
	ResultSHA256         string           `json:"result_sha256"`
	StartupFailureSHA256 string           `json:"startup_failure_sha256"`
}

type CandidateCompositionRepairRecoveryBasis struct {
	PlanID      string                                      `json:"plan_id"`
	PlanVersion int                                         `json:"plan_version"`
	TaskID      string                                      `json:"task_id"`
	TaskVersion int                                         `json:"task_version"`
	RunID       string                                      `json:"run_id"`
	RunVersion  int                                         `json:"run_version"`
	Attempts    []CandidateCompositionRepairRecoveryAttempt `json:"attempts"`
	SHA256      string                                      `json:"sha256"`
}

type CandidateCompositionRepairRevisionContext struct {
	ProjectID       string                                   `json:"project_id"`
	PlanID          string                                   `json:"plan_id"`
	PlanVersion     int                                      `json:"plan_version"`
	TaskVersion     int                                      `json:"task_version"`
	ConflictBasis   *CandidateCompositionConflictBasis       `json:"conflict_basis,omitempty"`
	RecoveryBasis   *CandidateCompositionRepairRecoveryBasis `json:"recovery_basis,omitempty"`
	EligibilityMode string                                   `json:"eligibility_mode"`
	CanRevise       bool                                     `json:"can_revise"`
	Blockers        []string                                 `json:"blockers"`
}

// Only this safe projection is public. The initiating credential reference and
// token digest remain inside immutable execution records.
type CandidateCompositionRepairRevisionLineage struct {
	RootPlanID         string `json:"root_plan_id"`
	ParentPlanID       string `json:"parent_plan_id"`
	Ordinal            int    `json:"ordinal"`
	CurrentHeadPlanID  string `json:"current_head_plan_id"`
	IsCurrent          bool   `json:"is_current"`
	Reason             string `json:"reason"`
	CreatedAt          string `json:"created_at"`
	DefinitionSHA256   string `json:"definition_sha256"`
	CreatorKind        string `json:"creator_kind"`
	CreatorPrincipalID string `json:"creator_principal_id"`
	Observed           bool   `json:"observed"`
}

// ConflictBasis preserves one known failed attempt. It never represents sealed
// code or a successful composition; a repair produces its own independent run.
type CandidateCompositionConflictBasis struct {
	CompositionID      string                            `json:"composition_id"`
	CompositionVersion int                               `json:"composition_version"`
	TaskID             string                            `json:"task_id"`
	TaskVersion        int                               `json:"task_version"`
	RunID              string                            `json:"run_id"`
	RunVersion         int                               `json:"run_version"`
	NodeID             string                            `json:"node_id"`
	Attempt            int                               `json:"attempt"`
	Fence              int64                             `json:"fence"`
	WorkerID           string                            `json:"worker_id"`
	Assignment         cc.AssignmentRef                  `json:"assignment"`
	ResultSHA256       string                            `json:"result_sha256"`
	EvidenceSHA256     string                            `json:"evidence_sha256"`
	Evidence           ccmp.CandidateCompositionEvidence `json:"evidence"`
	SHA256             string                            `json:"sha256"`
}

type CandidateCompositionConflictContext struct {
	ProjectID          string                             `json:"project_id"`
	CompositionID      string                             `json:"composition_id"`
	CompositionVersion int                                `json:"composition_version"`
	Basis              *CandidateCompositionConflictBasis `json:"basis,omitempty"`
	RepairPlanID       string                             `json:"repair_plan_id"`
	CanCreate          bool                               `json:"can_create"`
	Blockers           []string                           `json:"blockers"`
}

type CandidateCompositionRepairDraft struct {
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	Priority           string   `json:"priority,omitempty"`
	OwnerID            string   `json:"owner_id,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type CandidateCompositionRepairCreateRequest struct {
	ModelExecutionSelection    *modelcontract.Selection        `json:"model_execution_selection,omitempty" wire:"nonnull"`
	ProjectID                  string                          `json:"project_id"`
	ExpectedCompositionVersion int                             `json:"expected_composition_version"`
	ExpectedConflictSHA256     string                          `json:"expected_conflict_sha256"`
	Repair                     CandidateCompositionRepairDraft `json:"repair"`
	WorkflowID                 string                          `json:"workflow_id"`
	WorkflowVersion            int                             `json:"workflow_version"`
	ProducerNodeID             string                          `json:"producer_node_id"`
	ToolProfileID              string                          `json:"tool_profile_id"`
	ExpectedToolVersion        int                             `json:"expected_tool_version"`
	IdempotencyKey             string                          `json:"idempotency_key"`
}
