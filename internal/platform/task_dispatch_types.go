package platform

import (
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type TaskContextRecord struct {
	ID                 string   `json:"id"`
	Version            int      `json:"version"`
	Title              string   `json:"title"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type TaskContext struct {
	// TestPlan is the current published, protected quality contract. It is
	// compared again before dispatch and frozen in the same run transaction.
	TestPlan *TaskTestPlanContext `json:"test_plan,omitempty"`
	// CodeBaseline is resolved only from the protected task binding.
	CodeBaseline *ccmp.CodeBaselineBinding `json:"code_baseline,omitempty"`
	Task         TaskContextRecord         `json:"task"`
	Requirement  *TaskContextRecord        `json:"requirement,omitempty"`
	// Knowledge is resolved server-side from explicitly selected reviewed revisions.
	Knowledge []ProjectKnowledgeContextItem `json:"knowledge,omitempty"`
	// Design is resolved from the protected task binding, never editable task data.
	Design *RequirementDesignContext `json:"design,omitempty"`
}

type TaskDispatchPlan struct {
	OctopExecution     *tc.OctopExecutionCapability    `json:"octop_execution,omitempty"`
	OpenCodeExecution  *tc.OpenCodeExecutionCapability `json:"opencode_execution,omitempty"`
	CodexExecution     *cc.Binding                     `json:"codex_execution,omitempty"`
	ModelExecution     *modelcontract.Binding          `json:"model_execution,omitempty"`
	CodeBaseline       *ccmp.CodeBaselineBinding       `json:"code_baseline,omitempty"`
	ID                 string                          `json:"id"`
	ProjectID          string                          `json:"project_id"`
	Version            int                             `json:"version"`
	State              string                          `json:"state"`
	TaskID             string                          `json:"task_id"`
	TaskVersion        int                             `json:"task_version"`
	RequirementID      string                          `json:"requirement_id,omitempty"`
	RequirementVersion int                             `json:"requirement_version,omitempty"`
	WorkflowID         string                          `json:"workflow_id"`
	WorkflowVersion    int                             `json:"workflow_version"`
	AgentNodeID        string                          `json:"agent_node_id"`
	ToolProfileID      string                          `json:"tool_profile_id"`
	ToolProfileVersion int                             `json:"tool_profile_version"`
	ToolProfileSHA256  string                          `json:"tool_profile_sha256"`
	RunID              string                          `json:"run_id"`
	CreatedAt          string                          `json:"created_at"`
	ExpiresAt          string                          `json:"expires_at"`
	Context            TaskContext                     `json:"context"`
	Workflow           dag.Workflow                    `json:"workflow"`
	// ExecutionRoute is the original reviewed display snapshot, never input
	// authority. Nil means older history did not record a public route view.
	ExecutionRoute *dag.ProjectExecutionRoute `json:"execution_route,omitempty"`
	Valid          bool                       `json:"valid"`
	Reason         string                     `json:"reason"`
}

type TaskDispatchPreviewRequest struct {
	CodexExecutionSelection *cc.Selection               `json:"codex_execution_selection,omitempty" wire:"nonnull"`
	ModelExecutionSelection *modelcontract.Selection    `json:"model_execution_selection,omitempty" wire:"nonnull"`
	ProjectID               string                      `json:"project_id"`
	TaskID                  string                      `json:"task_id"`
	ExpectedTaskVersion     int                         `json:"expected_task_version"`
	WorkflowID              string                      `json:"workflow_id"`
	WorkflowVersion         int                         `json:"workflow_version"`
	AgentNodeID             string                      `json:"agent_node_id"`
	ToolProfileID           string                      `json:"tool_profile_id"`
	ExpectedToolVersion     int                         `json:"expected_tool_version"`
	IdempotencyKey          string                      `json:"idempotency_key"`
	Knowledge               []ProjectKnowledgeReference `json:"knowledge,omitempty"`
}

type TaskDispatchStartRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	IdempotencyKey  string `json:"idempotency_key"`
}
