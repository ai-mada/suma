package platform

import (
	"context"
	"database/sql"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

// The accepted task and explicit producer, rather than a browser-supplied
// commit or an arbitrary transfer, identify this third baseline origin.
// repositoryBasisReadTx retains the original Review, current task/dependency
// evidence, sealed transfer and independent same-candidate test authority.
func (s *Service) acceptedTaskCandidateBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project, task string, version int, node string) (out RepositoryIntegrationBasis, err error) {
	if !handoffID(task, false) || version < 1 || !diagnosisID(node, 150) {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	out, err = s.repositoryBasisReadTx(ctx, tx, p, project, task, node)
	if err != nil {
		return out, err
	}
	if out.TaskID != task || out.TaskVersion != version || out.SourceNodeID != node {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	ordinary, err := s.acceptedTaskCandidateNodeTx(ctx, tx, p, project, out)
	if err != nil {
		return out, err
	}
	if !ordinary {
		return out, compositionError(409, "baseline_changed", compositionBaseline)
	}
	return out, nil
}

// Options can reuse the already resolved formal basis. Keep command-based
// compositions and terminal candidates on their existing distinct origins.
// The exact current spec is read with the complete original candidate tuple;
// a similarly named producer or a former attempt cannot satisfy this check.
func (s *Service) acceptedTaskCandidateNodeTx(ctx context.Context, tx *sql.Tx, p Principal, project string, basis RepositoryIntegrationBasis) (bool, error) {
	if basis.Source.ProjectID != project || basis.Source.Source.RunID != basis.RunID || basis.Source.Source.NodeID != basis.SourceNodeID {
		return false, nil
	}
	producer, found, err := s.codeCandidateNodeResultReadTx(ctx, tx, p, project, basis.RunID, basis.SourceNodeID)
	if err != nil || !found {
		return false, err
	}
	return producer.Spec.Type == "agent" && codeCandidateResultIdentity(producer, basis.RunID) &&
		producer.Attempt == basis.Source.Source.Attempt && producer.Fence == basis.Source.Source.Fence && producer.Worker == basis.Source.Source.WorkerID, nil
}

// A task can expose several successful producers. Include the complete source
// in the memo identity so one producer's acceptance cannot stand in for a
// sibling, a retry or different sealed bytes under the same accepted task.
func acceptedTaskBaselineMemoKey(bindingTask string, version int, source cc.Binding) string {
	return "task:" + digest(stringify([]any{bindingTask, version, source}))
}
