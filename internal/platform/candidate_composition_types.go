package platform

import (
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/modelcontract"
)

type CandidateCompositionSourceRequest struct {
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	SourceNodeID        string `json:"source_node_id"`
}
type CandidateCompositionCreateRequest struct {
	ProjectID           string                              `json:"project_id"`
	TaskID              string                              `json:"task_id"`
	ExpectedTaskVersion int                                 `json:"expected_task_version"`
	WorkflowID          string                              `json:"workflow_id"`
	WorkflowVersion     int                                 `json:"workflow_version"`
	ProducerNodeID      string                              `json:"producer_node_id"`
	Sources             []CandidateCompositionSourceRequest `json:"sources"`
	IdempotencyKey      string                              `json:"idempotency_key"`
}
type CandidateCompositionStartRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Acknowledge     bool   `json:"acknowledge"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type CandidateCompositionCommandRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type CandidateCompositionPlan struct {
	ModelExecution     *modelcontract.Binding                     `json:"model_execution,omitempty"`
	RepairRevision     *CandidateCompositionRepairRevisionLineage `json:"repair_revision,omitempty"`
	ConflictBasis      *CandidateCompositionConflictBasis         `json:"conflict_basis,omitempty"`
	ToolProfileID      string                                     `json:"tool_profile_id,omitempty"`
	ToolProfileVersion int                                        `json:"tool_profile_version,omitempty"`
	ToolProfileSHA256  string                                     `json:"tool_profile_sha256,omitempty"`
	ID                 string                                     `json:"id"`
	ProjectID          string                                     `json:"project_id"`
	Version            int                                        `json:"version"`
	State              string                                     `json:"state"`
	TaskID             string                                     `json:"task_id"`
	TaskVersion        int                                        `json:"task_version"`
	TaskBasisSHA256    string                                     `json:"task_basis_sha256"`
	WorkflowID         string                                     `json:"workflow_id"`
	WorkflowVersion    int                                        `json:"workflow_version"`
	ProducerNodeID     string                                     `json:"producer_node_id"`
	Strategy           string                                     `json:"strategy"`
	AdminBaseCommit    string                                     `json:"admin_base_commit"`
	SourcesSHA256      string                                     `json:"sources_sha256"`
	Sources            []RepositoryIntegrationBasis               `json:"sources"`
	Context            TaskContext                                `json:"context"`
	Workflow           dag.Workflow                               `json:"workflow"`
	ExecutionRoute     dag.ProjectExecutionRoute                  `json:"execution_route"`
	RunID              string                                     `json:"run_id"`
	RunState           string                                     `json:"run_state"`
	TaskState          string                                     `json:"task_state"`
	Candidate          *cc.Binding                                `json:"candidate,omitempty"`
	Conflicts          []string                                   `json:"conflicts"`
	CreatedAt          string                                     `json:"created_at"`
	ExpiresAt          string                                     `json:"expires_at"`
	FixedTimestamp     string                                     `json:"fixed_timestamp"`
	OperationID        string                                     `json:"operation_id"`
	Valid              bool                                       `json:"valid"`
	Blockers           []string                                   `json:"blockers"`
	AllowedActions     []string                                   `json:"allowed_actions"`
}
type CandidateCompositionPage struct {
	Items      []CandidateCompositionPlan `json:"items"`
	NextCursor string                     `json:"next_cursor"`
	Total      int                        `json:"total"`
}
type CandidateCompositionSourceChoice struct {
	TaskID          string     `json:"task_id"`
	TaskVersion     int        `json:"task_version"`
	Title           string     `json:"title"`
	SourceNodeID    string     `json:"source_node_id"`
	Source          cc.Binding `json:"source"`
	Eligible        bool       `json:"eligible"`
	CanBindBaseline bool       `json:"can_bind_baseline"`
	Blockers        []string   `json:"blockers"`
}
type CandidateCompositionOptions struct {
	ProjectID   string                             `json:"project_id"`
	TaskID      string                             `json:"task_id"`
	TaskVersion int                                `json:"task_version"`
	Sources     []CandidateCompositionSourceChoice `json:"sources"`
	CanCreate   bool                               `json:"can_create"`
	Blockers    []string                           `json:"blockers"`
}
type TaskCodeBaselineBindRequest struct {
	ProjectID                            string `json:"project_id"`
	ExpectedTaskVersion                  int    `json:"expected_task_version"`
	ExpectedBindingVersion               int    `json:"expected_binding_version"`
	CompositionID                        string `json:"composition_id,omitempty"`
	ExpectedCompositionVersion           int    `json:"expected_composition_version,omitempty"`
	TerminalCandidatePlanID              string `json:"terminal_candidate_plan_id,omitempty"`
	ExpectedTerminalCandidatePlanVersion int    `json:"expected_terminal_candidate_plan_version,omitempty"`
	SourceTaskID                         string `json:"source_task_id,omitempty"`
	ExpectedSourceTaskVersion            int    `json:"expected_source_task_version,omitempty"`
	SourceNodeID                         string `json:"source_node_id,omitempty"`
	Acknowledge                          bool   `json:"acknowledge"`
	Reason                               string `json:"reason"`
	IdempotencyKey                       string `json:"idempotency_key"`
}
type CandidateCompositionBaseline struct {
	ProjectID string                    `json:"project_id"`
	TaskID    string                    `json:"task_id"`
	Version   int                       `json:"version"`
	Binding   *ccmp.CodeBaselineBinding `json:"binding,omitempty"`
	CanBind   bool                      `json:"can_bind"`
	Blockers  []string                  `json:"blockers"`
}
