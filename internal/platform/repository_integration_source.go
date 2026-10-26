package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) repositoryBasisTx(ctx context.Context, auth *authorizedCommand, taskID, nodeID string) (out RepositoryIntegrationBasis, err error) {
	if err = assertProjectRead(auth, auth.target.projectID); err != nil {
		return out, err
	}
	return s.repositoryBasisReadTx(ctx, auth.tx, auth.currentPrincipal, auth.target.projectID, taskID, nodeID)
}

// Called only after the caller has proven a current human project credential.
// This resolves immutable data without inventing a browser Actor for callbacks.
func (s *Service) repositoryBasisReadTx(ctx context.Context, tx *sql.Tx, p Principal, project, taskID, nodeID string) (out RepositoryIntegrationBasis, err error) {
	ctx, _ = compositionAcceptanceContext(ctx)
	task, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, taskID))
	if err != nil {
		return out, err
	}
	accepted, err := s.currentReleaseTaskBasisTx(ctx, tx, p, project, taskID, task.RunID)
	if err != nil {
		return out, repositoryConflict("basis", repositoryBasis)
	}
	if err = s.checkCandidateCompositionAcceptanceTx(ctx, tx, p, project, task.RunID); err != nil {
		return out, err
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT c.binding FROM suma_candidate_transfers c JOIN dag_nodes n ON n.run_id=c.run_id AND n.id=c.node_id AND n.attempt=c.attempt AND n.fence=c.fence AND n.worker_id=c.worker_id WHERE c.tenant_id=? AND c.project_id=? AND c.run_id=? AND c.node_id=?`), p.TenantID, project, task.RunID, nodeID).Scan(&raw)
	if err != nil {
		return out, repositoryConflict("basis", repositoryBasis)
	}
	var source cc.CandidateBinding
	if json.Unmarshal([]byte(raw), &source) != nil || !source.ValidSealed() {
		return out, repositoryConflict("basis", repositoryBasis)
	}
	if err = s.candidateSourceTx(ctx, tx, candidateSubject{tenant: p.TenantID, project: project}, source); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT n.id,n.state,n.spec,a.result FROM dag_nodes n LEFT JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt AND a.fence=n.fence AND a.worker_id=n.worker_id WHERE n.run_id=? ORDER BY n.id`), task.RunID)
	if err != nil {
		return out, err
	}
	run := dag.Run{ID: task.RunID, State: "succeeded", Nodes: []dag.NodeRun{}}
	proofs := []any{}
	for rows.Next() {
		var id, state, specRaw string
		var resultRaw *string
		if err = rows.Scan(&id, &state, &specRaw, &resultRaw); err != nil {
			break
		}
		var spec dag.Node
		var result dag.Result
		if json.Unmarshal([]byte(specRaw), &spec) != nil {
			err = repositoryConflict("basis", repositoryBasis)
			break
		}
		if resultRaw != nil && json.Unmarshal([]byte(*resultRaw), &result) != nil {
			err = repositoryConflict("basis", repositoryBasis)
			break
		}
		run.Nodes = append(run.Nodes, dag.NodeRun{ID: id, Type: spec.Type, State: state, Attempt: result.Attempt, Fence: result.Fence, WorkerID: result.WorkerID, WorkspaceEvidence: result.WorkspaceEvidence, ToolEvidence: result.ToolEvidence})
		if spec.Type == "test" && state != "skipped" {
			proofs = append(proofs, []any{id, state, result.Attempt, result.Fence, result.WorkerID, digest(stringify(result))})
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(proofs) == 0 || validateCodeTestEvidence(run) != nil {
		return out, repositoryConflict("basis", repositoryBasis)
	}
	out = RepositoryIntegrationBasis{TaskID: task.ID, TaskVersion: task.Version, RunID: task.RunID, SourceNodeID: nodeID, Source: source, AcceptedTaskBasis: accepted}
	out.BasisSHA256 = digest(stringify([]any{out, proofs}))
	return out, nil
}
func (s *Service) checkRepositoryBasisTx(ctx context.Context, auth *authorizedCommand, basis RepositoryIntegrationBasis) error {
	current, err := s.repositoryBasisTx(ctx, auth, basis.TaskID, basis.SourceNodeID)
	if err != nil {
		return err
	}
	if current != basis {
		return repositoryConflict("basis", repositoryBasis)
	}
	return nil
}
func (s *Service) repositoryCapabilitiesTx(ctx context.Context, auth *authorizedCommand) (write, approve, manage bool, err error) {
	if auth.actor.kind != actorBrowser && auth.actor.kind != actorProjectAPI {
		return false, false, false, nil
	}
	_, e := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, auth.target.projectID, true, false)
	if e == nil {
		write = true
	} else if !expertTeamExpectedError(e) {
		return false, false, false, e
	}
	_, e = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, auth.target.projectID, false, true)
	if e == nil {
		approve = true
	} else if !expertTeamExpectedError(e) {
		return false, false, false, e
	}
	manage = approve && auth.actor.kind == actorBrowser
	return
}
func repositoryKnownError(err error) bool { var api *APIError; return errors.As(err, &api) }
