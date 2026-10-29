package platform

import (
	"context"
	"database/sql"

	term "github.com/ai-mada/suma/internal/terminalcontract"
)

// A terminal plan is a distinct accepted-source kind. It never impersonates a
// composition, and the same current formal Review/candidate gates remain the
// authority for adopting its code into an unstarted follow-up task.
func (s *Service) terminalCandidateAcceptedBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, version int) (out RepositoryIntegrationBasis, err error) {
	source, err := s.workspaceTerminalCandidateSourceTx(ctx, tx, p.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if source == nil {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	plan, err := s.taskDispatchPlanTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	if plan.State != "started" || plan.Version != version || plan.RunID == "" || source.PlanID != id {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	var producerFound bool
	for _, node := range plan.Workflow.Nodes {
		if node.ID != plan.AgentNodeID {
			continue
		}
		frozen, e := term.DecodeCandidateSource(node.Config[term.CandidateConfigKey])
		if e != nil || frozen == nil || stringify(frozen) != stringify(source) || node.Type != "command" {
			return out, compositionError(409, "baseline_changed", compositionBaseline)
		}
		producerFound = true
	}
	if !producerFound {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	out, err = s.repositoryBasisReadTx(ctx, tx, p, project, plan.TaskID, plan.AgentNodeID)
	if err != nil {
		return out, err
	}
	if out.RunID != plan.RunID {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	producer, found, err := s.codeCandidateNodeResultReadTx(ctx, tx, p, project, plan.RunID, plan.AgentNodeID)
	if err != nil {
		return out, err
	}
	if !found || !codeCandidateResultIdentity(producer, plan.RunID) || validateTerminalCandidateEvidence(producer.Result.WorkspaceEvidence, producer.Result.ToolEvidence, source) != nil {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	return out, nil
}
