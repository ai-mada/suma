package runner

import (
	"encoding/json"
	ccc "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/codexcontract"
	ec "github.com/ai-mada/suma/internal/executioncontract"
	"github.com/ai-mada/suma/internal/releasecontract"
	rc "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
	wc "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

type Assignment struct {
	ExecutionSkills         *cc.ExecutionSkillBundle              `json:"execution_skills,omitempty"`
	WorkBuddyCloud          *wbc.CloudAssignmentBinding           `json:"workbuddy_cloud,omitempty"`
	CandidateConflictRepair *ccc.CandidateConflictRepairExecution `json:"candidate_conflict_repair,omitempty"`
	CandidateComposition    *ccc.CandidateCompositionExecution    `json:"candidate_composition,omitempty"`
	CodeBaseline            *ccc.CodeBaselineBinding              `json:"code_baseline,omitempty"`
	RepositoryIntegration   *rc.RepositoryExecution               `json:"repository_integration,omitempty"`
	TenantID                string                                `json:"tenant_id,omitempty"`
	ProjectID               string                                `json:"project_id,omitempty"`
	PoolID                  string                                `json:"pool_id,omitempty"`
	PoolRunnerID            string                                `json:"pool_runner_id,omitempty"`
	PoolSlotID              string                                `json:"pool_slot_id,omitempty"`
	PoolGrantID             string                                `json:"pool_grant_id,omitempty"`
	PoolGrantVersion        int                                   `json:"pool_grant_version,omitempty"`
	PoolCredentialID        string                                `json:"pool_credential_id,omitempty"`
	PoolCredentialVersion   int                                   `json:"pool_credential_version,omitempty"`
	RunID                   string                                `json:"run_id"`
	NodeID                  string                                `json:"node_id"`
	Attempt                 int                                   `json:"attempt"`
	Fence                   int64                                 `json:"fence"`
	WorkerID                string                                `json:"worker_id"`
	Command                 []string                              `json:"command,omitempty"`
	Type                    string                                `json:"type,omitempty"`
	TimeoutSeconds          int                                   `json:"timeout_seconds,omitempty"`
	Idempotent              bool                                  `json:"idempotent,omitempty"`
	DispatchID              string                                `json:"dispatch_id,omitempty"`
	DispatchState           string                                `json:"dispatch_state,omitempty"`
	LeaseUntil              string                                `json:"lease_until,omitempty"`
	Config                  map[string]any                        `json:"config,omitempty"`
	CandidateCommit         string                                `json:"candidate_commit,omitempty"`
	CandidateFrom           string                                `json:"candidate_from,omitempty"`
	CandidateAttempt        int                                   `json:"candidate_attempt,omitempty"`
	CandidateFence          int64                                 `json:"candidate_fence,omitempty"`
	CandidateEvidenceSHA256 string                                `json:"candidate_evidence_sha256,omitempty"`
	ReleaseExecution        *releasecontract.ReleaseExecution     `json:"release_execution,omitempty"`
}
type Result struct {
	WorkBuddyCloudWait  *wbc.CloudWaitEvidence  `json:"workbuddy_cloud_wait,omitempty"`
	TestReport          *tr.TestReport          `json:"test_report,omitempty"`
	StartupFailure      *ec.StartupFailureProof `json:"startup_failure,omitempty"`
	IntegrationEvidence *rc.RepositoryResult    `json:"integration_evidence,omitempty"`
	Assignment
	ExitCode          int                               `json:"exit_code"`
	Output            string                            `json:"output"`
	Error             string                            `json:"error"`
	FailureClass      string                            `json:"failure_class,omitempty"`
	OperationID       string                            `json:"operation_id"`
	Stopped           bool                              `json:"stopped"`
	OutcomeUnknown    bool                              `json:"outcome_unknown,omitempty"`
	WorkspaceEvidence map[string]any                    `json:"workspace_evidence,omitempty"`
	ArtifactEvidence  *releasecontract.ArtifactEvidence `json:"artifact_evidence,omitempty"`
	ReleaseEvidence   *releasecontract.ReleaseResult    `json:"release_evidence,omitempty"`
	ToolEvidence      *tc.Evidence                      `json:"tool_evidence,omitempty"`
	artifactPending   bool
	candidatePending  bool
}

type Progress struct {
	Assignment    Assignment `json:"assignment"`
	OperationID   string     `json:"operation_id"`
	Message       string     `json:"message"`
	ToolEvent     *tc.Event  `json:"tool_event,omitempty"`
	HostOperation *wc.Event  `json:"host_operation,omitempty"`
	Acked         bool       `json:"acked,omitempty"`
}

// Result carries only completion fields on the wire, independently of the full durable assignment.
func (r Result) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		WorkBuddyCloudWait  *wbc.CloudWaitEvidence            `json:"workbuddy_cloud_wait,omitempty"`
		TestReport          *tr.TestReport                    `json:"test_report,omitempty"`
		StartupFailure      *ec.StartupFailureProof           `json:"startup_failure,omitempty"`
		IntegrationEvidence *rc.RepositoryResult              `json:"integration_evidence,omitempty"`
		RunID               string                            `json:"run_id"`
		NodeID              string                            `json:"node_id"`
		Attempt             int                               `json:"attempt"`
		Fence               int64                             `json:"fence"`
		WorkerID            string                            `json:"worker_id"`
		ExitCode            int                               `json:"exit_code"`
		Output              string                            `json:"output"`
		Error               string                            `json:"error"`
		FailureClass        string                            `json:"failure_class,omitempty"`
		OperationID         string                            `json:"operation_id"`
		Stopped             bool                              `json:"stopped"`
		OutcomeUnknown      bool                              `json:"outcome_unknown"`
		WorkspaceEvidence   map[string]any                    `json:"workspace_evidence,omitempty"`
		ArtifactEvidence    *releasecontract.ArtifactEvidence `json:"artifact_evidence,omitempty"`
		ReleaseEvidence     *releasecontract.ReleaseResult    `json:"release_evidence,omitempty"`
		ToolEvidence        *tc.Evidence                      `json:"tool_evidence,omitempty"`
	}{r.WorkBuddyCloudWait, r.TestReport, r.StartupFailure, r.IntegrationEvidence, r.RunID, r.NodeID, r.Attempt, r.Fence, r.WorkerID, r.ExitCode, r.Output, r.Error, r.FailureClass, r.OperationID, r.Stopped, r.OutcomeUnknown, r.WorkspaceEvidence, r.ArtifactEvidence, r.ReleaseEvidence, r.ToolEvidence})
}
