package platform

import (
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

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
