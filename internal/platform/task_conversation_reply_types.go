package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
)

// Coverage records the exact window sent to the model. Older turns are never
// silently presented as part of the request or reconstructed from a summary.
type TaskConversationReplyCoverage struct {
	Total         int64 `json:"total"`
	Included      int   `json:"included"`
	FirstSequence int64 `json:"first_sequence"`
	LastSequence  int64 `json:"last_sequence"`
	Complete      bool  `json:"complete"`
}

type TaskConversationReplySource struct {
	SHA256          string                        `json:"sha256"`
	Task            TaskContextRecord             `json:"task"`
	Requirement     *TaskContextRecord            `json:"requirement"`
	TaskRunID       string                        `json:"task_run_id"`
	Messages        []TaskConversationMessage     `json:"messages"`
	Coverage        TaskConversationReplyCoverage `json:"coverage"`
	ReplyToSequence int64                         `json:"reply_to_sequence"`
	ExecutionRoute  dag.ProjectExecutionRoute     `json:"execution_route"`
}

type TaskConversationReply struct {
	ID                 string                      `json:"id"`
	ProjectID          string                      `json:"project_id"`
	TaskID             string                      `json:"task_id"`
	Version            int                         `json:"version"`
	State              string                      `json:"state"`
	Locale             string                      `json:"locale"`
	RunID              string                      `json:"run_id"`
	RunVersion         int                         `json:"run_version"`
	RunState           string                      `json:"run_state"`
	Source             TaskConversationReplySource `json:"source"`
	ToolProfileID      string                      `json:"tool_profile_id"`
	ToolProfileVersion int                         `json:"tool_profile_version"`
	ToolProfileSHA256  string                      `json:"tool_profile_sha256"`
	AssistantName      string                      `json:"assistant_name"`
	ModelExecution     *mc.Binding                 `json:"model_execution" wire:"nonnull"`
	MessageID          string                      `json:"message_id"`
	MessageSequence    int64                       `json:"message_sequence"`
	CreatedBy          string                      `json:"created_by"`
	CreatedAt          string                      `json:"created_at"`
	UpdatedAt          string                      `json:"updated_at"`
	BasisCurrent       bool                        `json:"basis_current"`
	Reason             string                      `json:"reason"`
	AllowedActions     []string                    `json:"allowed_actions"`
}

type TaskConversationReplyCreateRequest struct {
	ProjectID               string        `json:"project_id"`
	TaskID                  string        `json:"task_id"`
	ExpectedTaskVersion     int           `json:"expected_task_version"`
	ExpectedLastSequence    int64         `json:"expected_last_sequence"`
	ReplyToSequence         int64         `json:"reply_to_sequence"`
	ContextMessageLimit     int           `json:"context_message_limit"`
	ToolProfileID           string        `json:"tool_profile_id"`
	ExpectedToolVersion     int           `json:"expected_tool_version"`
	ModelExecutionSelection *mc.Selection `json:"model_execution_selection" wire:"nonnull"`
	Locale                  string        `json:"locale"`
	AcknowledgeModelUse     bool          `json:"acknowledge_model_use"`
	IdempotencyKey          string        `json:"idempotency_key"`
}

type TaskConversationReplyCommandRequest struct {
	ProjectID          string `json:"project_id"`
	ExpectedVersion    int    `json:"expected_version"`
	ExpectedRunVersion int    `json:"expected_run_version"`
	Action             string `json:"action"`
	IdempotencyKey     string `json:"idempotency_key"`
}

type TaskConversationReplyPage struct {
	Items      []TaskConversationReply `json:"items"`
	Total      int                     `json:"total"`
	Limit      int                     `json:"limit"`
	NextCursor string                  `json:"next_cursor"`
}
