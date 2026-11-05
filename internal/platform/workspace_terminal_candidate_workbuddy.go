package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

// The caller owns the transaction and any scheduler locks. The original cloud
// attempt establishes provenance only. The local terminal remains the file source.
func (s *Service) workspaceTerminalWorkBuddyOriginTx(ctx context.Context, tx *sql.Tx, tenant, project, taskID, runID string) (*term.TerminalCandidateWorkBuddyOrigin, error) {
	if runID == "" {
		return nil, nil
	}
	changed := conflict(workspaceTerminalCandidateChanged)
	readError := func(err error) error {
		var api *APIError
		if errors.Is(err, sql.ErrNoRows) || errors.Is(err, dag.ErrNotFound) || errors.Is(err, dag.ErrConflict) ||
			errors.As(err, &api) && (api.Status == 404 || api.Status == 409) {
			return changed
		}
		return err
	}
	plan, found, err := s.workBuddyTaskDispatchRunSourceTx(ctx, tx, tenant, project, runID)
	if err != nil || !found {
		return nil, readError(err)
	}
	if plan.State != "started" || plan.TaskID != taskID || plan.RunID != runID {
		return nil, changed
	}
	var taskRun, taskState string
	var taskVersion int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT run_id,status,version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, taskID).Scan(&taskRun, &taskState, &taskVersion); err != nil {
		return nil, readError(err)
	}
	if taskRun != runID || taskState == "accepted" || taskVersion < plan.TaskVersion+1 {
		return nil, changed
	}
	boundVersion, err := s.continuationProvenanceTx(ctx, tx, tenant, project, taskID, runID)
	if err != nil {
		return nil, readError(err)
	}
	if boundVersion != plan.TaskVersion+1 {
		return nil, changed
	}

	origin := term.TerminalCandidateWorkBuddyOrigin{RunID: runID, NodeID: plan.NodeID}
	var runState, nodeState, reason, worker, rawSpec string
	err = tx.QueryRowContext(ctx, s.q(`SELECT r.version,r.state,n.state,n.reason,n.attempt,n.fence,n.worker_id,n.spec
		FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id
		WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.id=?`), tenant, project, runID, plan.NodeID).
		Scan(&origin.RunVersion, &runState, &nodeState, &reason, &origin.Attempt, &origin.Fence, &worker, &rawSpec)
	if err != nil {
		return nil, readError(err)
	}
	var spec dag.Node
	if origin.RunVersion < 1 || runState != "running" || nodeState != "waiting" || reason != "cloud.delivery_required" ||
		origin.Attempt < 1 || origin.Fence < 1 || json.Unmarshal([]byte(rawSpec), &spec) != nil ||
		!dag.WorkBuddyCloudNode(spec) || digest(stringify(spec)) != plan.NodeSpecSHA256 {
		return nil, changed
	}
	assignment, err := dag.ReadExternalAssignmentTx(ctx, tx, s.Dialect, tenant, project, runID, origin.NodeID, origin.Attempt)
	if err != nil {
		return nil, readError(err)
	}
	if assignment.Fence != origin.Fence || assignment.WorkerID != worker || assignment.Binding.DelegationID != plan.DelegationID {
		return nil, changed
	}
	origin.AssignmentSHA256 = assignment.AssignmentSHA256
	return &origin, nil
}
