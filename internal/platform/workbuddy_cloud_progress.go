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

	progress, _, err := w.cloudNodeProgressTx(ctx, auth.tx, auth.target.tenantID, project, run, node, "")
	return progress, err
}

const (
	cloudProgressDefaultLimit      = 10
	cloudProgressMaxLimit          = 20
	cloudProgressFilteredScanLimit = 40
)

var cloudProgressSubmissionID = regexp.MustCompile(`^[a-f0-9]{32}$`)

// ListCloudNodeProgress reads project-visible progress from the submissions
// index. It has no dependency on the provider, owner account, or permissions to
// view a private submission. Scope is checked again for every continuation.
func (w *WorkBuddyConnections) ListCloudNodeProgress(ctx context.Context, actor Actor, project string, limit int, before, filter, taskID string) (wc.CloudNodeProgressPage, error) {
	if w == nil || w.s == nil || !handoffID(project, false) || limit < 1 || limit > cloudProgressMaxLimit ||
		filter != "" && !wc.ValidCloudProgressFilter(filter) || taskID != "" && !wc.ValidCloudProgressTaskID(taskID) {
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
	scanLimit := limit
	if filter != "" {
		collection += ":" + filter
	}
	if taskID != "" {
		collection += ":task:" + taskID
	}
	if filter != "" || taskID != "" {
		scanLimit = cloudProgressFilteredScanLimit
	}
	last, err := automationDecodeCursor(principal, project, collection, before)
	if err != nil {
		return wc.CloudNodeProgressPage{}, err
	}
	if taskID != "" {
		var present int
		err = auth.tx.QueryRowContext(ctx, w.s.q(`SELECT 1 FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, taskID).Scan(&present)
		if errors.Is(err, sql.ErrNoRows) {
			return wc.CloudNodeProgressPage{}, missing()
		}
		if err != nil {
			return wc.CloudNodeProgressPage{}, err
		}
	}
	rows, err := auth.tx.QueryContext(ctx, w.s.q(`SELECT id,run_id,node_id FROM suma_workbuddy_cloud_submissions
		WHERE tenant_id=? AND project_id=? AND id>? ORDER BY id LIMIT ?`), tenant, project, last, scanLimit+1)
	if err != nil {
		return wc.CloudNodeProgressPage{}, err
	}
	defer rows.Close()

	type reference struct{ id, run, node string }
	refs := make([]reference, 0, scanLimit+1)
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

	out := wc.CloudNodeProgressPage{ProjectID: project, Filter: filter, TaskID: taskID, Items: make([]wc.CloudNodeProgress, 0, limit)}
	inspected := 0
	for _, item := range refs[:min(len(refs), scanLimit)] {
		if !cloudProgressSubmissionID.MatchString(item.id) {
			return wc.CloudNodeProgressPage{}, cloudConflict()
		}
		progress, n, err := w.cloudNodeProgressTx(ctx, auth.tx, tenant, project, item.run, item.node, item.id)
		if err != nil {
			var api *APIError
			if errors.As(err, &api) && (api.Status == 400 || api.Status == 404) {
				return wc.CloudNodeProgressPage{}, cloudConflict()
			}
			return wc.CloudNodeProgressPage{}, err
		}
		inspected++
		if !progress.MatchesFilter(filter) {
			continue
		}
		delivery, title, err := w.cloudTaskDeliveryLinkTx(ctx, auth, project, item.run, item.node, n)
		if err != nil {
			return wc.CloudNodeProgressPage{}, err
		}
		if taskID != "" && (delivery == nil || delivery.TaskID != taskID) ||
			filter == wc.CloudProgressDeliveryFilter && (delivery == nil || delivery.RunRelation != "current") {
			continue
		}
		if delivery != nil {
			link := wc.CloudProgressTaskLink{
				RunID: item.run, NodeID: item.node, TaskID: delivery.TaskID,
				TaskVersion: delivery.TaskVersion, RunRelation: delivery.RunRelation, TaskTitle: title,
			}
			if link.Validate() != nil {
				return wc.CloudNodeProgressPage{}, cloudConflict()
			}
			out.TaskLinks = append(out.TaskLinks, link)
		}
		out.Items = append(out.Items, progress)
		if len(out.Items) == limit {
			break
		}
	}
	if inspected < len(refs) {
		// A filtered page can be empty. Advance past the last inspected
		// record, preserving every unread record for the continuation.
		out.NextCursor = automationEncodeCursor(principal, project, collection, refs[inspected-1].id)
	}
	return out, nil
}

func (w *WorkBuddyConnections) cloudNodeProgressTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node, submissionID string) (wc.CloudNodeProgress, cloudNode, error) {
	n, err := w.cloudNodeTx(ctx, tx, tenant, project, run, node)
	if err != nil {
		return wc.CloudNodeProgress{}, cloudNode{}, err
	}
	if n.spec.ID != node {
		return wc.CloudNodeProgress{}, cloudNode{}, cloudConflict()
	}
	out := wc.CloudNodeProgress{
		ProjectID: project, RunID: run, NodeID: node, RunVersion: n.runVersion,
		RunState: n.runState, NodeState: n.nodeState, Attempt: n.attempt,
		NodeReason: wc.CloudProgressReason(n.nodeReason), SubmissionState: "not_recorded",
	}
	if submissionID == "" {
		err = tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL(`SELECT id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`)), tenant, project, run, node).Scan(&submissionID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return wc.CloudNodeProgress{}, cloudNode{}, err
		}
		if err == nil && submissionID == "" {
			return wc.CloudNodeProgress{}, cloudNode{}, cloudConflict()
		}
	}
	if submissionID != "" {
		sub, err := w.cloudSubmissionTx(ctx, tx, tenant, project, submissionID, true)
		if err != nil {
			return wc.CloudNodeProgress{}, cloudNode{}, err
		}
		if sub.ProjectID != project || sub.RunID != run || sub.NodeID != node || sub.RequestSHA256 != n.requestHash {
			return wc.CloudNodeProgress{}, cloudNode{}, cloudConflict()
		}
		out.SubmissionState = sub.State
		if sub.State == "task_known" {
			out.ProviderStatus = wc.CloudProgressProviderStatus(sub.Task.Status)
			out.ObservedAt = sub.ObservedAt
			out.ObservationFailed = sub.LastError != ""
		}
	}
	if out.Validate() != nil {
		return wc.CloudNodeProgress{}, cloudNode{}, cloudConflict()
	}
	return out, n, nil
}
