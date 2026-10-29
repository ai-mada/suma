package platform

import mc "github.com/ai-mada/suma/internal/modelcontract"

// A reviewed discussion plan is an explicit development handoff. It reuses
// TaskDispatchPlan, its fixed model budget, and the existing quality workflow.
// The editable goal and criteria supplement, rather than rewrite, the task's
// formal scope or any published test/acceptance contract.
type TaskDiscussionDevelopmentCreateRequest struct {
	ProjectID               string        `json:"project_id"`
	TaskID                  string        `json:"task_id"`
	ExpectedTaskVersion     int           `json:"expected_task_version"`
	ReplyID                 string        `json:"reply_id"`
	ExpectedReplyVersion    int           `json:"expected_reply_version"`
	WorkflowID              string        `json:"workflow_id"`
	WorkflowVersion         int           `json:"workflow_version"`
	AgentNodeID             string        `json:"agent_node_id"`
	ToolProfileID           string        `json:"tool_profile_id"`
	ExpectedToolVersion     int           `json:"expected_tool_version"`
	ModelExecutionSelection *mc.Selection `json:"model_execution_selection" wire:"nonnull"`
	ReviewedGoal            string        `json:"reviewed_goal"`
	AcceptanceCriteria      []string      `json:"acceptance_criteria"`
	AcknowledgeExecution    bool          `json:"acknowledge_execution"`
	IdempotencyKey          string        `json:"idempotency_key"`
}

type TaskDiscussionDevelopmentPlan struct {
	ID                 string                  `json:"id"`
	ProjectID          string                  `json:"project_id"`
	TaskID             string                  `json:"task_id"`
	Version            int                     `json:"version"`
	ReplyID            string                  `json:"reply_id"`
	ReplyVersion       int                     `json:"reply_version"`
	SourceSHA256       string                  `json:"source_sha256"`
	Message            TaskConversationMessage `json:"message"`
	ReviewedGoal       string                  `json:"reviewed_goal"`
	AcceptanceCriteria []string                `json:"acceptance_criteria"`
	CreatedBy          string                  `json:"created_by"`
	CreatedAt          string                  `json:"created_at"`
	DispatchPlan       TaskDispatchPlan        `json:"dispatch_plan"`
}
