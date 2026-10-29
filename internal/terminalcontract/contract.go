// Package terminalcontract defines the single wire contract shared by the
// control plane, authenticated Runner and generated browser client. Raw PTY
// bytes are base64, never HTML, command text, or a claim of task completion.
package terminalcontract

import (
	"encoding/base64"
	"fmt"
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

const Protocol = "suma-pty-v1"
const MaxInputBytes = 8192
const MaxOutputChunkBytes = 32768
const MaxOutputBytes = 2 * 1024 * 1024
const MaxOutputChunks = 4096

type Session struct {
	ID                  string   `json:"id"`
	ProjectID           string   `json:"project_id"`
	TaskID              string   `json:"task_id"`
	TaskVersion         int      `json:"task_version"`
	Version             int      `json:"version"`
	RunID               string   `json:"run_id"`
	RunVersion          int      `json:"run_version"`
	State               string   `json:"state"`
	ToolProfileID       string   `json:"tool_profile_id"`
	ToolName            string   `json:"tool_name"`
	Directory           string   `json:"directory"`
	WorkerID            string   `json:"worker_id"`
	Columns             int      `json:"columns"`
	Rows                int      `json:"rows"`
	ControlEpoch        int64    `json:"control_epoch"`
	NextInputSequence   int64    `json:"next_input_sequence"`
	OutputFirstSequence int64    `json:"output_first_sequence"`
	OutputLastSequence  int64    `json:"output_last_sequence"`
	OutputAckSequence   int64    `json:"output_ack_sequence"`
	ExpiresAt           string   `json:"expires_at"`
	CreatedAt           string   `json:"created_at"`
	AllowedActions      []string `json:"allowed_actions"`
	Reason              string   `json:"reason"`
	// Only a stopped original Runner can report this content manifest. It is
	// source provenance for an explicit candidate plan, never test approval.
	SourceSnapshotSHA256 string                    `json:"source_snapshot_sha256"`
	BaseCommit           string                    `json:"base_commit"`
	AdminBaseCommit      string                    `json:"admin_base_commit"`
	CodeBaseline         *ccmp.CodeBaselineBinding `json:"code_baseline,omitempty"`
}

type CreateRequest struct {
	ProjectID           string `json:"project_id"`
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	ToolProfileID       string `json:"tool_profile_id"`
	ExpectedToolVersion int    `json:"expected_tool_version"`
	Columns             int    `json:"columns"`
	Rows                int    `json:"rows"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
	AcknowledgeShell    bool   `json:"acknowledge_shell"`
	IdempotencyKey      string `json:"idempotency_key"`
}

// Input receipt means bytes were delivered to the original PTY at most once;
// it never asserts that the shell parsed, executed, or completed a command.
type InputReceipt struct {
	InputID      string `json:"input_id"`
	Sequence     int64  `json:"sequence"`
	SHA256       string `json:"sha256"`
	State        string `json:"state"`
	BytesWritten int    `json:"bytes_written"`
}
type Input struct {
	InputID    string `json:"input_id"`
	Sequence   int64  `json:"sequence"`
	SHA256     string `json:"sha256"`
	DataBase64 string `json:"data_base64"`
}
type OutputChunk struct {
	Sequence   int64  `json:"sequence"`
	DataBase64 string `json:"data_base64"`
}
type OutputPage struct {
	SessionID     string        `json:"session_id"`
	ControlEpoch  int64         `json:"control_epoch"`
	Chunks        []OutputChunk `json:"chunks"`
	FirstSequence int64         `json:"first_sequence"`
	LastSequence  int64         `json:"last_sequence"`
	Gap           bool          `json:"gap"`
	State         string        `json:"state"`
}
type CommandRequest struct {
	ProjectID       string `json:"project_id"`
	ExpectedVersion int    `json:"expected_version"`
	ControlEpoch    int64  `json:"control_epoch"`
	Action          string `json:"action"`
	InputID         string `json:"input_id,omitempty"`
	InputSequence   int64  `json:"input_sequence,omitempty"`
	DataBase64      string `json:"data_base64,omitempty"`
	Columns         int    `json:"columns,omitempty"`
	Rows            int    `json:"rows,omitempty"`
	OutputSequence  int64  `json:"output_sequence,omitempty"`
	IdempotencyKey  string `json:"idempotency_key"`
}
type CommandReceipt struct {
	Session Session       `json:"session"`
	Input   *InputReceipt `json:"input"`
}

// Exchanges always bind the original dispatch. A replay may re-deliver the
// same input identity; the Runner's durable write-intent ledger forbids writing
// its bytes again. Outputs are acknowledged only after transactional storage.
type ExchangeRequest struct {
	SessionID            string                    `json:"session_id"`
	RunID                string                    `json:"run_id"`
	NodeID               string                    `json:"node_id"`
	Attempt              int                       `json:"attempt"`
	Fence                int64                     `json:"fence"`
	WorkerID             string                    `json:"worker_id"`
	DispatchID           string                    `json:"dispatch_id"`
	OperationID          string                    `json:"operation_id"`
	OutputChunks         []OutputChunk             `json:"output_chunks"`
	InputReceipts        []InputReceipt            `json:"input_receipts"`
	State                string                    `json:"state"`
	Directory            string                    `json:"directory"`
	ProcessID            int                       `json:"process_id"`
	TreeStopped          bool                      `json:"tree_stopped"`
	ExitCode             int                       `json:"exit_code"`
	Error                string                    `json:"error"`
	SourceSnapshotSHA256 string                    `json:"source_snapshot_sha256"`
	BaseCommit           string                    `json:"base_commit"`
	AdminBaseCommit      string                    `json:"admin_base_commit"`
	CodeBaseline         *ccmp.CodeBaselineBinding `json:"code_baseline,omitempty"`
}
type ExchangeResponse struct {
	// RecoveryOperationID is an explicit reviewed disposition of this exact
	// original dispatch, separate from a final-result acknowledgement.
	RecoveryOperationID string  `json:"recovery_operation_id"`
	SessionID           string  `json:"session_id"`
	ControlEpoch        int64   `json:"control_epoch"`
	Inputs              []Input `json:"inputs"`
	Columns             int     `json:"columns"`
	Rows                int     `json:"rows"`
	StopRequested       bool    `json:"stop_requested"`
	OutputAckSequence   int64   `json:"output_ack_sequence"`
}

func DecodeBytes(encoded string, maximum int) ([]byte, error) {
	if len(encoded) > ((maximum+2)/3)*4 {
		return nil, fmt.Errorf("Terminal frame exceeds its byte limit")
	}
	b, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(b) < 1 || len(b) > maximum {
		return nil, fmt.Errorf("Terminal frame bytes are invalid")
	}
	return b, nil
}
