package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

// A protected code-producing run uses the DAG lifecycle without acquiring the
// separate managed-release authority. Core still owns state, retry safety,
// descendants, operation replay and the next attempt/fence.
func (s *Service) authorizeCandidateCompositionRunCommandTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, run, action string) (bool, error) {
	if err := assertProjectCommand(tx, auth, project); err != nil {
		return false, err
	}
	if auth.target.resourceID != run || auth.actor.kind != actorBrowser && auth.actor.kind != actorProjectAPI {
		return false, forbidden()
	}
	r, found, err := s.candidateCompositionExecutionRecordTx(ctx, tx, auth.target.tenantID, project, run)
	if err != nil || !found {
		return false, err
	}
	if r.Plan.ProjectID != project || r.Plan.RunID != run || !validCandidateCompositionExecutionRecord(r) {
		return true, compositionError(409, "changed", compositionChanged)
	}
	switch action {
	case "pause", "resume", "cancel", "retry":
		if auth.purpose != purposeProjectWrite && auth.purpose != purposeBrowserProjectWrite {
			return true, forbidden()
		}
	case "reconcile":
		if auth.purpose != purposeProjectApprove && auth.purpose != purposeBrowserProjectApprove {
			return true, forbidden()
		}
	default:
		return true, compositionError(400, "invalid", compositionInvalid)
	}
	// Stopping or recording an original uncertain fact must remain possible
	// after source drift. Only commands that permit new work re-admit inputs.
	if action != "retry" && action != "resume" {
		return true, nil
	}
	if err = s.checkCandidateCompositionExecutionTx(ctx, tx, auth.target.tenantID, project, run, true); err != nil {
		return true, err
	}
	var current dag.Workflow
	if r.Plan.ConflictBasis != nil {
		current, _, err = s.compositionRepairWorkflowTx(ctx, auth, project, r.Plan)
	} else {
		var raw string
		err = tx.QueryRowContext(ctx, s.q(`SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), auth.target.tenantID, project, r.Plan.WorkflowID, r.Plan.WorkflowVersion).Scan(&raw)
		if errors.Is(err, sql.ErrNoRows) {
			return true, missing()
		}
		if err == nil {
			err = json.Unmarshal([]byte(raw), &current)
		}
		if err == nil {
			current, err = buildCandidateCompositionWorkflow(current, r.Plan.ProducerNodeID)
		}
	}
	if err != nil {
		return true, err
	}
	if stringify(current) != stringify(r.Plan.Workflow) {
		return true, compositionError(409, "changed", compositionChanged)
	}
	return true, nil
}
