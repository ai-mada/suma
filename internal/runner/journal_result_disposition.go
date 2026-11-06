package runner

import (
	rc "github.com/ai-mada/suma/internal/releasecontract"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

// ResultRejection is local delivery history. It never becomes a business
// completion receipt or a Runner result field on the wire.
type ResultRejection string

const ResultRejectedClosedAttempt ResultRejection = "closed_attempt"

func (e entry) resultDeliverySettled() bool {
	return e.Acked || e.ResultRejection == ResultRejectedClosedAttempt
}

func (e entry) validResultDisposition() bool {
	if e.ResultRejection == "" {
		return !e.Acked || e.Result != nil
	}
	return !e.Acked && e.ResultRejection == ResultRejectedClosedAttempt && e.canRejectClosedResult()
}

func (e entry) canRejectClosedResult() bool {
	a, r := e.Assignment, e.Result
	if r == nil || r.OperationID == "" || a.RunID == "" || a.NodeID == "" || a.WorkerID == "" || a.Attempt < 1 || a.Fence < 1 ||
		r.RunID != a.RunID || r.NodeID != a.NodeID || r.WorkerID != a.WorkerID || r.Attempt != a.Attempt || r.Fence != a.Fence {
		return false
	}
	switch a.Type {
	case "task", "script", "command", "agent", "test":
	default:
		return false
	}
	_, appBuild := a.Config[rc.ApplicationBuildConfigKey]
	_, repositorySource := a.Config[ri.RepositorySourceExecutionKey]
	if appBuild || repositorySource || cloudReserved(a) || hasCandidate(a) || a.WorkBuddyCloud != nil || a.ReleaseExecution != nil ||
		a.RepositoryIntegration != nil || a.CodeBaseline != nil || a.DevelopmentContinuation != nil || a.CandidateComposition != nil ||
		a.CandidateConflictRepair != nil || a.ExecutionSkills != nil {
		return false
	}
	if r.OutcomeUnknown || r.Stopped || r.WorkBuddyCloudWait != nil || r.WorkspaceEvidence != nil || r.TestReport != nil ||
		r.StartupFailure != nil || r.IntegrationEvidence != nil || r.ArtifactEvidence != nil || r.ReleaseEvidence != nil || r.ToolEvidence != nil {
		return false
	}
	if e.ArtifactPending != nil || e.CandidatePending != nil || e.ModelGateway != nil || e.WorkspaceTrace != nil || e.Terminal != nil ||
		e.Cloud != nil || e.ToolPermissions != nil || e.ToolInputs != nil || e.Steering != nil {
		return false
	}
	for _, p := range e.Progress {
		if p.HostOperation != nil {
			return false
		}
	}
	return true
}
