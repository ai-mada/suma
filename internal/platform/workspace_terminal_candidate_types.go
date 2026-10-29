package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

type WorkspaceTerminalCandidatePreviewRequest struct {
	ProjectID                 string `json:"project_id"`
	ExpectedSessionVersion    int    `json:"expected_session_version"`
	ExpectedTaskVersion       int    `json:"expected_task_version"`
	ToolProfileID             string `json:"tool_profile_id"`
	ExpectedToolVersion       int    `json:"expected_tool_version"`
	WorkflowID                string `json:"workflow_id"`
	WorkflowVersion           int    `json:"workflow_version"`
	ProducerNodeID            string `json:"producer_node_id"`
	AcknowledgeStoppedChanges bool   `json:"acknowledge_stopped_changes"`
	IdempotencyKey            string `json:"idempotency_key"`
}

type WorkspaceTerminalCandidatePlan struct {
	ID                 string                       `json:"id"`
	ProjectID          string                       `json:"project_id"`
	Version            int                          `json:"version"`
	State              string                       `json:"state"`
	SessionID          string                       `json:"session_id"`
	SessionVersion     int                          `json:"session_version"`
	TaskID             string                       `json:"task_id"`
	TaskVersion        int                          `json:"task_version"`
	RunID              string                       `json:"run_id"`
	CreatedAt          string                       `json:"created_at"`
	ExpiresAt          string                       `json:"expires_at"`
	ToolProfileID      string                       `json:"tool_profile_id"`
	ToolProfileVersion int                          `json:"tool_profile_version"`
	ToolProfileSHA256  string                       `json:"tool_profile_sha256"`
	WorkerID           string                       `json:"worker_id"`
	WorkflowID         string                       `json:"workflow_id"`
	WorkflowVersion    int                          `json:"workflow_version"`
	ProducerNodeID     string                       `json:"producer_node_id"`
	Context            TaskContext                  `json:"context"`
	Workflow           dag.Workflow                 `json:"workflow"`
	Source             term.TerminalCandidateSource `json:"source"`
	Valid              bool                         `json:"valid"`
	Reason             string                       `json:"reason"`
}

type WorkspaceTerminalCandidatePlanPage struct {
	Items       []WorkspaceTerminalCandidatePlan `json:"items"`
	TaskVersion int                              `json:"task_version"`
}
