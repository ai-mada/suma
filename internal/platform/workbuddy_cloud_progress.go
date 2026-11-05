package platform

import (
	"context"
	"database/sql"
	"errors"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// GetCloudNodeProgress reads the fixed WorkBuddy node and its saved submission
// within one current project authorization. It does not resolve an owner grant.
func (w *WorkBuddyConnections) GetCloudNodeProgress(ctx context.Context, actor Actor, project, run, node string) (wc.CloudNodeProgress, error) {
	if w == nil || w.s == nil || !handoffID(project, false) || !cloudReadID(run, 100) || !cloudReadID(node, 150) {
		return wc.CloudNodeProgress{}, cloudInvalid()
	}
	if !actor.IsBrowser() && actor.kind != actorProjectAPI {
		return wc.CloudNodeProgress{}, forbidden()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, run, purposeProjectRead)
	if err != nil {
		return wc.CloudNodeProgress{}, err
	}
	defer close()

	tenant := auth.target.tenantID
	n, err := w.cloudNodeTx(ctx, auth.tx, tenant, project, run, node)
	if err != nil {
		return wc.CloudNodeProgress{}, err
	}
	if n.spec.ID != node {
		return wc.CloudNodeProgress{}, cloudConflict()
	}
	out := wc.CloudNodeProgress{
		ProjectID: project, RunID: run, NodeID: node, RunVersion: n.runVersion,
		RunState: n.runState, NodeState: n.nodeState, Attempt: n.attempt,
		NodeReason: wc.CloudProgressReason(n.nodeReason), SubmissionState: "not_recorded",
	}
	var submissionID string
	err = auth.tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`)), tenant, project, run, node).Scan(&submissionID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return wc.CloudNodeProgress{}, err
	}
	if err == nil {
		sub, err := w.cloudSubmissionTx(ctx, auth.tx, tenant, project, submissionID, true)
		if err != nil {
			return wc.CloudNodeProgress{}, err
		}
		if sub.ProjectID != project || sub.RunID != run || sub.NodeID != node || sub.RequestSHA256 != n.requestHash {
			return wc.CloudNodeProgress{}, cloudConflict()
		}
		out.SubmissionState = sub.State
		if sub.State == "task_known" {
			out.ProviderStatus = wc.CloudProgressProviderStatus(sub.Task.Status)
			out.ObservedAt = sub.ObservedAt
			out.ObservationFailed = sub.LastError != ""
		}
	}
	if out.Validate() != nil {
		return wc.CloudNodeProgress{}, cloudConflict()
	}
	return out, nil
}
