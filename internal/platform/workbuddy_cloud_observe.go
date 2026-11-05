package platform

import (
	"context"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// cloudObserver authorizes this current human's explicit observation. It does
// not restore the old machine lease or the consumed/revoked delegation.
func (w *WorkBuddyConnections) cloudObserver(ctx context.Context, auth *authorizedCommand, id string) (cloudSubmissionRow, *workbuddy.Client, error) {
	row, err := w.cloudSubmissionTx(ctx, auth.tx, auth.target.tenantID, auth.target.projectID, id, true)
	if err != nil {
		return row, nil, err
	}
	d, err := w.cloudDelegationTx(ctx, auth.tx, auth.target.tenantID, auth.target.projectID, row.DelegationID, true)
	if err != nil {
		return row, nil, err
	}
	if d.OwnerID != auth.currentPrincipal.ID {
		return row, nil, missing()
	}
	n, err := w.cloudNodeTx(ctx, auth.tx, auth.target.tenantID, auth.target.projectID, row.RunID, row.NodeID)
	if err != nil {
		return row, nil, err
	}
	if d.RunID != row.RunID || d.NodeID != row.NodeID || n.specHash != d.NodeSpecSHA256 || n.requestHash != d.RequestSHA256 || row.RequestSHA256 != d.RequestSHA256 || n.binding.ConnectionID != d.ConnectionID || n.binding.GrantGeneration != d.GrantGeneration || row.State != "task_known" || row.Task == nil {
		return row, nil, cloudConflict()
	}
	_, client, err := w.cloudGrantTx(ctx, auth.tx, auth.target.tenantID, auth.target.projectID, d.OwnerID, d.ConnectionID, d.GrantGeneration, d.ClientBindingSHA256)
	return row, client, err
}

func (w *WorkBuddyConnections) ObserveCloudTask(ctx context.Context, actor Actor, project, id string, req wc.CloudObserveRequest) (wc.CloudSubmission, error) {
	if w == nil || w.s == nil || project != req.ProjectID || !handoffID(id, false) || req.Validate() != nil {
		return wc.CloudSubmission{}, cloudInvalid()
	}
	body := struct {
		Kind, ID string
		Request  wc.CloudObserveRequest
	}{"workbuddy_cloud_observe", id, req}
	var original cloudSubmissionRow
	var client *workbuddy.Client
	replayed := false
	var requestHash string
	err := func() error {
		bounded, auth, close, e := w.cloudTurnBrowser(ctx, actor, project, id, false, true)
		if e != nil {
			return e
		}
		defer close()
		original, client, e = w.cloudObserver(bounded, auth, id)
		if e != nil {
			return e
		}
		requestHash, replayed, e = w.s.commandReceipt(bounded, auth, req.IdempotencyKey, body)
		if e != nil {
			return e
		}
		if replayed {
			var receiptID string
			if json.Unmarshal([]byte(requestHash), &receiptID) != nil || receiptID != id {
				return cloudConflict()
			}
			return nil
		}
		if original.Version != req.ExpectedVersion || original.Version >= wc.MaxVersion {
			return cloudConflict()
		}
		return nil
	}()
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	if replayed {
		return original.CloudSubmission, nil
	}
	task, providerErr := client.GetTask(ctx, original.Task.ID)
	// Reauthorize and lock the original execution route after provider I/O.
	bounded, auth, close, err := w.cloudTurnBrowser(ctx, actor, project, id, false, true)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	defer close()
	row, _, err := w.cloudObserver(bounded, auth, id)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	hash, replay, err := w.s.commandReceipt(bounded, auth, req.IdempotencyKey, body)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	if replay {
		return row.CloudSubmission, nil
	}
	if hash != requestHash || row.Version != original.Version || row.Task.ID != original.Task.ID || row.assignmentHash != original.assignmentHash {
		return wc.CloudSubmission{}, cloudConflict()
	}
	clock, err := w.s.authorityNow(bounded, auth.tx)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	before := row.Version
	row.Version++
	row.UpdatedAt = automationStamp(clock)
	if providerErr != nil {
		row.LastError = "workbuddy_cloud_query_failed"
	} else {
		row.Task = &task
		row.ObservedAt = row.UpdatedAt
		row.LastError = ""
	}
	if err = w.saveCloudSubmissionTx(bounded, auth.tx, row, before); err != nil {
		return wc.CloudSubmission{}, err
	}
	if err = w.s.saveCommandReceipt(bounded, auth, req.IdempotencyKey, hash, id); err != nil {
		return wc.CloudSubmission{}, err
	}
	if err = w.s.commandActivity(bounded, auth, "workbuddy_cloud_observed", id, "WorkBuddy cloud task observed"); err != nil {
		return wc.CloudSubmission{}, err
	}
	if providerErr == nil && task.Status == "completed" {
		if _, err = dag.RefineWorkBuddyDeliveryWaitTx(bounded, auth.tx, w.s.Dialect, auth.target.tenantID, project, row.CloudSubmission); err != nil {
			return wc.CloudSubmission{}, err
		}
	}
	if err = auth.tx.Commit(); err != nil {
		return wc.CloudSubmission{}, err
	}
	if providerErr != nil {
		return wc.CloudSubmission{}, &APIError{502, "workbuddy_cloud_query_failed", "workbuddy_cloud_query_failed", map[string]any{"submission_id": id}}
	}
	return row.CloudSubmission, nil
}
