package runner

import (
	"encoding/json"
	"github.com/ai-mada/suma/internal/releasecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type Assignment struct {
	RunID                   string                            `json:"run_id"`
	NodeID                  string                            `json:"node_id"`
	Attempt                 int                               `json:"attempt"`
	Fence                   int64                             `json:"fence"`
	WorkerID                string                            `json:"worker_id"`
	Command                 []string                          `json:"command,omitempty"`
	Type                    string                            `json:"type,omitempty"`
	TimeoutSeconds          int                               `json:"timeout_seconds,omitempty"`
	Idempotent              bool                              `json:"idempotent,omitempty"`
	Config                  map[string]any                    `json:"config,omitempty"`
	CandidateCommit         string                            `json:"candidate_commit,omitempty"`
	CandidateFrom           string                            `json:"candidate_from,omitempty"`
	CandidateAttempt        int                               `json:"candidate_attempt,omitempty"`
	CandidateFence          int64                             `json:"candidate_fence,omitempty"`
	CandidateEvidenceSHA256 string                            `json:"candidate_evidence_sha256,omitempty"`
	ReleaseExecution        *releasecontract.ReleaseExecution `json:"release_execution,omitempty"`
}
type Result struct {
	Assignment
	ExitCode          int                               `json:"exit_code"`
	Output            string                            `json:"output"`
	Error             string                            `json:"error"`
	OperationID       string                            `json:"operation_id"`
	Stopped           bool                              `json:"stopped"`
	OutcomeUnknown    bool                              `json:"outcome_unknown,omitempty"`
	WorkspaceEvidence map[string]any                    `json:"workspace_evidence,omitempty"`
	ArtifactEvidence  *releasecontract.ArtifactEvidence `json:"artifact_evidence,omitempty"`
	ReleaseEvidence   *releasecontract.ReleaseResult    `json:"release_evidence,omitempty"`
	ToolEvidence      *tc.Evidence                      `json:"tool_evidence,omitempty"`
	artifactPending   bool
}

type Progress struct {
	Assignment  Assignment `json:"assignment"`
	OperationID string     `json:"operation_id"`
	Message     string     `json:"message"`
	ToolEvent   *tc.Event  `json:"tool_event,omitempty"`
	Acked       bool       `json:"acked,omitempty"`
}

// Result carries only completion fields on the wire, independently of the full durable assignment.
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
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
		WorkspaceEvidence map[string]any                    `json:"workspace_evidence,omitempty"`
		ArtifactEvidence  *releasecontract.ArtifactEvidence `json:"artifact_evidence,omitempty"`
		ReleaseEvidence   *releasecontract.ReleaseResult    `json:"release_evidence,omitempty"`
		ToolEvidence      *tc.Evidence                      `json:"tool_evidence,omitempty"`
	}{r.RunID, r.NodeID, r.Attempt, r.Fence, r.WorkerID, r.ExitCode, r.Output, r.Error, r.OperationID, r.Stopped, r.OutcomeUnknown, r.WorkspaceEvidence, r.ArtifactEvidence, r.ReleaseEvidence, r.ToolEvidence})
}
