package dag

import (
	"errors"
	"github.com/ai-mada/suma/internal/releasecontract"
	"github.com/ai-mada/suma/internal/toolcontract"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrConflict    = errors.New("conflict")
	ErrInvalid     = errors.New("invalid")
	ErrStale       = errors.New("stale assignment")
	ErrUnsupported = errors.New("unsupported")
	ErrCancelled   = errors.New("cancellation requested")
)

type Workflow struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version int    `json:"version"`
	Nodes   []Node `json:"nodes"`
	Edges   []Edge `json:"edges"`
}
type Node struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Type           string            `json:"type"`
	Command        []string          `json:"command"`
	DependsOn      []string          `json:"depends_on"`
	Join           string            `json:"join"`
	Quorum         int               `json:"quorum"`
	Condition      string            `json:"condition,omitempty"`
	Branches       map[string]string `json:"branches,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds"`
	MaxRetries     int               `json:"max_retries"`
	Idempotent     bool              `json:"idempotent"`
	Resources      map[string]int64  `json:"resources,omitempty"`
	Config         map[string]any    `json:"config,omitempty"`
}
type Edge struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Condition string `json:"condition,omitempty"`
}
type Run struct {
	ID                string    `json:"id"`
	TenantID          string    `json:"tenant_id"`
	ProjectID         string    `json:"project_id"`
	WorkflowID        string    `json:"workflow_id"`
	WorkflowName      string    `json:"workflow_name"`
	WorkflowVersion   int       `json:"workflow_version"`
	Version           int       `json:"version"`
	State             string    `json:"state"`
	CreatedAt         string    `json:"created_at"`
	UpdatedAt         string    `json:"updated_at"`
	Nodes             []NodeRun `json:"nodes"`
	CompensationRunID string    `json:"compensation_run_id,omitempty"`
	CompensationState string    `json:"compensation_state,omitempty"`
}
type NodeRun struct {
	ID                     string                            `json:"id"`
	Name                   string                            `json:"name"`
	Type                   string                            `json:"type"`
	State                  string                            `json:"state"`
	Reason                 string                            `json:"reason"`
	Attempt                int                               `json:"attempt"`
	Fence                  int64                             `json:"fence"`
	WorkerID               string                            `json:"worker_id"`
	Command                []string                          `json:"command"`
	Output                 string                            `json:"output"`
	Error                  string                            `json:"error"`
	StartedAt              string                            `json:"started_at"`
	CompletedAt            string                            `json:"completed_at"`
	ChildRunID             string                            `json:"child_run_id,omitempty"`
	Idempotent             bool                              `json:"idempotent"`
	Compensatable          bool                              `json:"compensatable"`
	WorkspaceEvidence      map[string]any                    `json:"workspace_evidence,omitempty"`
	ArtifactEvidence       *releasecontract.ArtifactEvidence `json:"artifact_evidence,omitempty"`
	ToolEvidence           *toolcontract.Evidence            `json:"tool_evidence,omitempty"`
	ToolProfileID          string                            `json:"tool_profile_id,omitempty"`
	ToolProfileSHA256      string                            `json:"tool_profile_sha256,omitempty"`
	WinnerNodeIDs          []string                          `json:"winner_node_ids,omitempty"`
	ReadyAt                string                            `json:"ready_at,omitempty"`
	AdmissionPolicyVersion int                               `json:"admission_policy_version,omitempty"`
}
type Assignment struct {
	RunID                   string                            `json:"run_id"`
	NodeID                  string                            `json:"node_id"`
	Attempt                 int                               `json:"attempt"`
	Fence                   int64                             `json:"fence"`
	WorkerID                string                            `json:"worker_id"`
	Command                 []string                          `json:"command"`
	Type                    string                            `json:"type"`
	TimeoutSeconds          int                               `json:"timeout_seconds"`
	Idempotent              bool                              `json:"idempotent"`
	Config                  map[string]any                    `json:"config"`
	CandidateCommit         string                            `json:"candidate_commit,omitempty"`
	CandidateFrom           string                            `json:"candidate_from,omitempty"`
	CandidateAttempt        int                               `json:"candidate_attempt,omitempty"`
	CandidateFence          int64                             `json:"candidate_fence,omitempty"`
	CandidateEvidenceSHA256 string                            `json:"candidate_evidence_sha256,omitempty"`
	ReleaseExecution        *releasecontract.ReleaseExecution `json:"release_execution,omitempty"`
	DispatchID              string                            `json:"dispatch_id,omitempty"`
	DispatchState           string                            `json:"dispatch_state,omitempty"`
	LeaseUntil              string                            `json:"lease_until,omitempty"`
}
type Result struct {
	RunID             string                            `json:"run_id"`
	NodeID            string                            `json:"node_id"`
	Attempt           int                               `json:"attempt"`
	Fence             int64                             `json:"fence"`
	WorkerID          string                            `json:"worker_id"`
	ExitCode          int                               `json:"exit_code"`
	Output            string                            `json:"output"`
	Error             string                            `json:"error"`
	OperationID       string                            `json:"operation_id"`
	Stopped           bool                              `json:"stopped"`
	OutcomeUnknown    bool                              `json:"outcome_unknown"`
	FailureClass      string                            `json:"failure_class,omitempty"`
	WorkspaceEvidence map[string]any                    `json:"workspace_evidence,omitempty"`
	ArtifactEvidence  *releasecontract.ArtifactEvidence `json:"artifact_evidence,omitempty"`
	ReleaseEvidence   *releasecontract.ReleaseResult    `json:"release_evidence,omitempty"`
	ToolEvidence      *toolcontract.Evidence            `json:"tool_evidence,omitempty"`
}
type Event struct {
	Sequence int64          `json:"sequence"`
	RunID    string         `json:"run_id"`
	NodeID   string         `json:"node_id"`
	Type     string         `json:"type"`
	At       string         `json:"at"`
	Data     map[string]any `json:"data"`
}
