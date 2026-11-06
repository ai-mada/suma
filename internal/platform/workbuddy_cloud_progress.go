package platform

import (
	"context"
	"database/sql"
	"errors"
	"regexp"

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

	return w.cloudNodeProgressTx(ctx, auth.tx, auth.target.tenantID, project, run, node, "")
}

const (
	cloudProgressDefaultLimit = 10
	cloudProgressMaxLimit     = 20
)

var cloudProgressSubmissionID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// ListCloudNodeProgress reads project-visible progress from the submissions
// index. It has no dependency on the provider, owner account, or permissions to
// view a private submission. Scope is checked again for every continuation.
func (w *WorkBuddyConnections) ListCloudNodeProgress(ctx context.Context, actor Actor, project string, limit int, before string) (wc.CloudNodeProgressPage, error) {
	if w == nil || w.s == nil || !handoffID(project, false) || limit < 1 || limit > cloudProgressMaxLimit {
		return wc.CloudNodeProgressPage{}, cloudInvalid()
	}
	if !actor.IsBrowser() && actor.kind != actorProjectAPI {
		return wc.CloudNodeProgressPage{}, forbidden()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return wc.CloudNodeProgressPage{}, err
	}
	defer close()

	tenant, principal := auth.target.tenantID, auth.currentPrincipal
	collection := "workbuddy_cloud_progress:" + digest(principal.ID)
	last, err := automationDecodeCursor(principal, project, collection, before)
	if err != nil {
		return wc.CloudNodeProgressPage{}, err
	}
	rows, err := auth.tx.QueryContext(ctx, w.s.q(`SELECT id,run_id,node_id FROM suma_workbuddy_cloud_submissions
		WHERE tenant_id=? AND project_id=? AND id>? ORDER BY id LIMIT ?`), tenant, project, last, limit+1)
	if err != nil {
		return wc.CloudNodeProgressPage{}, err
	}
	defer rows.Close()

	type reference struct{ id, run, node string }
	refs := make([]reference, 0, limit+1)
	for rows.Next() {
		var item reference
		if err = rows.Scan(&item.id, &item.run, &item.node); err != nil {
			return wc.CloudNodeProgressPage{}, err
		}
		refs = append(refs, item)
	}
	if err = rows.Err(); err != nil {
		return wc.CloudNodeProgressPage{}, err
	}
	if err = rows.Close(); err != nil {
		return wc.CloudNodeProgressPage{}, err
	}

	out := wc.CloudNodeProgressPage{ProjectID: project, Items: make([]wc.CloudNodeProgress, 0, limit)}
	if len(refs) > limit {
		refs = refs[:limit]
		out.NextCursor = automationEncodeCursor(principal, project, collection, refs[limit-1].id)
	}
	for _, item := range refs {
		if !cloudProgressSubmissionID.MatchString(item.id) {
			return wc.CloudNodeProgressPage{}, cloudConflict()
		}
		progress, err := w.cloudNodeProgressTx(ctx, auth.tx, tenant, project, item.run, item.node, item.id)
		if err != nil {
			var api *APIError
			if errors.As(err, &api) && (api.Status == 400 || api.Status == 404) {
				return wc.CloudNodeProgressPage{}, cloudConflict()
			}
			return wc.CloudNodeProgressPage{}, err
		}
		out.Items = append(out.Items, progress)
	}
	return out, nil
}

func (w *WorkBuddyConnections) cloudNodeProgressTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node, submissionID string) (wc.CloudNodeProgress, error) {
	n, err := w.cloudNodeTx(ctx, tx, tenant, project, run, node)
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
	if submissionID == "" {
		err = tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`)), tenant, project, run, node).Scan(&submissionID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return wc.CloudNodeProgress{}, err
		}
		if err == nil && submissionID == "" {
			return wc.CloudNodeProgress{}, cloudConflict()
		}
	}
	if submissionID != "" {
		sub, err := w.cloudSubmissionTx(ctx, tx, tenant, project, submissionID, true)
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
