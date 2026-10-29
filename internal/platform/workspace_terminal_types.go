package platform

import (
	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

type WorkspaceTerminalSession term.Session
type WorkspaceTerminalCreateRequest term.CreateRequest
type WorkspaceTerminalCommandRequest term.CommandRequest
type WorkspaceTerminalInputReceipt term.InputReceipt
type WorkspaceTerminalOutputChunk term.OutputChunk
type WorkspaceTerminalCommandReceipt struct {
	Session WorkspaceTerminalSession       `json:"session"`
	Input   *WorkspaceTerminalInputReceipt `json:"input"`
}
type WorkspaceTerminalOutputPage struct {
	SessionID     string                         `json:"session_id"`
	ControlEpoch  int64                          `json:"control_epoch"`
	Chunks        []WorkspaceTerminalOutputChunk `json:"chunks"`
	FirstSequence int64                          `json:"first_sequence"`
	LastSequence  int64                          `json:"last_sequence"`
	Gap           bool                           `json:"gap"`
	State         string                         `json:"state"`
}

type WorkspaceTerminalPage struct {
	Items            []WorkspaceTerminalSession `json:"items"`
	Profiles         []ToolProfile              `json:"profiles"`
	TaskVersion      int                        `json:"task_version"`
	CanCreate        bool                       `json:"can_create"`
	ProfilesComplete bool                       `json:"profiles_complete"`
}

type PoolWorkspaceTerminalExchangeRequest struct {
	term.ExchangeRequest
	TenantID   string         `json:"tenant_id"`
	ProjectID  string         `json:"project_id"`
	RunnerID   string         `json:"runner_id"`
	Assignment dag.Assignment `json:"assignment"`
}

// Private metadata never grants the browser or an agent a machine credential.
// A session owns its original isolated attempt, not the editor's shared tree.
type workspaceTerminalRecord struct {
	Session               term.Session              `json:"session"`
	Task                  TaskContextRecord         `json:"task"`
	TaskRunID             string                    `json:"task_run_id"`
	Requirement           *TaskContextRecord        `json:"requirement"`
	ToolProfileVersion    int                       `json:"tool_profile_version"`
	ToolProfileSHA256     string                    `json:"tool_profile_sha256"`
	Creator               string                    `json:"creator"`
	CreatorCredentialHash string                    `json:"creator_credential_hash"`
	Route                 dag.ProjectExecutionRoute `json:"route"`
	ProcessID             int                       `json:"process_id"`
	ExitCode              int                       `json:"exit_code"`
	TreeStopped           bool                      `json:"tree_stopped"`
	Attempt               int                       `json:"attempt"`
	Fence                 int64                     `json:"fence"`
	DispatchID            string                    `json:"dispatch_id"`
}
