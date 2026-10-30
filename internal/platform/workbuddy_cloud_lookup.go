package platform

import (
	"context"
	"database/sql"
	"errors"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// LookupCloudSubmission reads the original machine's persisted submission. It
// never restores the in-memory permit held by a successful prepare call stack.
func (w *WorkBuddyConnections) LookupCloudSubmission(ctx context.Context, actor Actor, project string, req wc.CloudLookupRequest) (wc.CloudLookupResult, error) {
	if project != req.ProjectID {
		return wc.CloudLookupResult{}, cloudInvalid()
	}
	return w.lookupCloudSubmission(ctx, projectCandidateSubject(actor, project), req)
}

func (w *WorkBuddyConnections) LookupPoolCloudSubmission(ctx context.Context, token string, req wc.CloudLookupRequest) (wc.CloudLookupResult, error) {
	return w.lookupCloudSubmission(ctx, candidateSubject{pool: true, token: token, tenant: req.Assignment.TenantID, project: req.ProjectID}, req)
}

func (w *WorkBuddyConnections) lookupCloudSubmission(ctx context.Context, subject candidateSubject, req wc.CloudLookupRequest) (wc.CloudLookupResult, error) {
	var empty wc.CloudLookupResult
	if w == nil || w.s == nil || ctx == nil || req.Validate() != nil {
		return empty, cloudInvalid()
	}
	bounded, tx, done, err := w.s.beginAuthorityTx(ctx)
	if err != nil {
		return empty, err
	}
	defer done()
	defer tx.Rollback()
	d, err := w.cloudLookupAuthorityTx(bounded, tx, subject, req)
	if err != nil {
		return empty, err
	}
	out := wc.CloudLookupResult{ProjectID: req.ProjectID, DelegationID: req.DelegationID, Assignment: req.Assignment, State: "not_found"}
	var id string
	err = tx.QueryRowContext(bounded, w.s.q(w.s.currentSQL(`SELECT id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND delegation_id=?`)), subject.tenant, subject.project, req.DelegationID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		if d.CreatesConsumed != 0 {
			return empty, cloudConflict()
		}
		// Absence is a fact at this transaction's read point. Another original
		// create can prepare later; no caller may derive a new permit from it.
		return out, nil
	}
	if err != nil {
		return empty, err
	}
	sub, err := w.cloudSubmissionTx(bounded, tx, subject.tenant, subject.project, id, true)
	if err != nil {
		return empty, err
	}
	if sub.workerHash != subject.hash() || sub.assignmentHash != digest(stringify(req.Assignment)) {
		return empty, forbidden()
	}
	if d.CreatesConsumed != 1 || sub.DelegationID != d.ID || sub.RunID != d.RunID || sub.NodeID != d.NodeID || sub.RequestSHA256 != d.RequestSHA256 {
		return empty, cloudConflict()
	}
	out.State, out.Submission = sub.State, &sub.CloudSubmission
	if out.Validate() != nil {
		return empty, cloudConflict()
	}
	// No operation receipt, provider call, lease renewal, activity or business
	// update is committed. Rollback releases the ordered authority locks.
	return out, nil
}
