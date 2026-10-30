package platform

import (
	"context"
	"encoding/json"

	"github.com/ai-mada/suma/internal/workbuddy"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func (w *WorkBuddyConnections) CreateCloudTask(ctx context.Context, actor Actor, project string, req wc.CloudCreateRequest) (wc.CloudSubmission, error) {
	if project != req.ProjectID {
		return wc.CloudSubmission{}, cloudInvalid()
	}
	return w.createCloudTask(ctx, projectCandidateSubject(actor, project), req)
}
func (w *WorkBuddyConnections) CreatePoolCloudTask(ctx context.Context, token string, req wc.CloudCreateRequest) (wc.CloudSubmission, error) {
	return w.createCloudTask(ctx, candidateSubject{pool: true, token: token, tenant: req.Assignment.TenantID, project: req.ProjectID}, req)
}
func (w *WorkBuddyConnections) createCloudTask(ctx context.Context, subject candidateSubject, req wc.CloudCreateRequest) (wc.CloudSubmission, error) {
	if w == nil || w.s == nil || req.Validate() != nil {
		return wc.CloudSubmission{}, cloudInvalid()
	}
	var prepared cloudSubmissionRow
	var client *workbuddy.Client
	var request workbuddy.CreateRequest
	send := false
	err := func() error {
		bounded, tx, done, err := w.s.beginAuthorityTx(ctx)
		if err != nil {
			return err
		}
		defer done()
		defer tx.Rollback()
		proof, err := w.cloudMachineAuthorityTx(bounded, tx, subject, req.DelegationID, req.Assignment, true)
		if err != nil {
			return err
		}
		hash, replay, err := w.s.candidateReceipt(bounded, proof.candidate, req.IdempotencyKey, struct {
			Kind    string
			Request wc.CloudCreateRequest
		}{"workbuddy_cloud_create", req})
		if err != nil {
			return err
		}
		if replay {
			var id string
			if json.Unmarshal([]byte(hash), &id) != nil {
				return cloudConflict()
			}
			prepared, err = w.cloudSubmissionTx(bounded, tx, subject.tenant, subject.project, id, true)
			if err != nil {
				return err
			}
			if prepared.workerHash != subject.hash() || prepared.assignmentHash != digest(stringify(req.Assignment)) {
				return forbidden()
			}
			if prepared.State != "task_known" {
				return cloudPending(prepared.ID, prepared.State)
			}
			return nil
		}
		d := proof.delegation
		if d.Version != req.ExpectedVersion || d.Version >= wc.MaxVersion || d.CreatesConsumed != 0 {
			return cloudConflict()
		}
		var count int
		if err = tx.QueryRowContext(bounded, w.s.q(`SELECT COUNT(*) FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), subject.tenant, subject.project, req.Assignment.RunID, req.Assignment.NodeID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return cloudConflict()
		}
		clock, err := w.s.authorityNow(bounded, tx)
		if err != nil {
			return err
		}
		stamp := automationStamp(clock)
		prepared = cloudSubmissionRow{CloudSubmission: wc.CloudSubmission{ID: newID(), ProjectID: subject.project, DelegationID: d.ID, Version: 1, RunID: d.RunID, NodeID: d.NodeID, Assignment: req.Assignment, RequestSHA256: d.RequestSHA256, State: "prepared", UsageState: "unreported", CreatedAt: stamp, UpdatedAt: stamp}, tenant: subject.tenant, assignmentHash: digest(stringify(req.Assignment)), workerHash: subject.hash(), tokenGeneration: proof.connection.TokenGeneration, nonce: digest(newID() + newID())}
		before := d.Version
		d.Version++
		d.CreatesConsumed = 1
		d.UpdatedAt = stamp
		if err = proof.candidate.assert(); err != nil {
			return err
		}
		if err = w.saveCloudDelegationTx(bounded, tx, d, before); err != nil {
			return err
		}
		if err = w.saveCloudSubmissionTx(bounded, tx, prepared, 0); err != nil {
			return err
		}
		if err = w.s.saveCandidateReceipt(bounded, proof.candidate, req.IdempotencyKey, hash, prepared.ID); err != nil {
			return err
		}
		if err = w.cloudMachineActivity(bounded, proof, "workbuddy_cloud_prepared", prepared.ID); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		client = proof.client
		request = proof.node.binding.Request
		send = true
		return nil
	}()
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	if !send {
		return prepared.CloudSubmission, nil
	}
	// Only this successful prepare call stack has the send permit. Neither the
	// database record nor a repeated operation can reconstruct it after a crash.
	task, providerErr := client.CreateTask(ctx, request)
	saved, err := w.finishCloudCreate(context.WithoutCancel(ctx), prepared, task, providerErr)
	if err != nil {
		return wc.CloudSubmission{}, cloudPending(prepared.ID, "prepared")
	}
	// Late provider facts are persisted even after revocation, but returning them
	// still requires the original machine and owner to be authorized now.
	bounded, tx, done, err := w.s.beginAuthorityTx(ctx)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	defer done()
	defer tx.Rollback()
	if _, err = w.cloudMachineAuthorityTx(bounded, tx, subject, req.DelegationID, req.Assignment, false); err != nil {
		return wc.CloudSubmission{}, err
	}
	if saved.State != "task_known" {
		return wc.CloudSubmission{}, cloudPending(saved.ID, saved.State)
	}
	return saved.CloudSubmission, nil
}

// This seam accepts only the in-memory original prepare record. It records an
// already observed fact; it grants no additional provider call or user access.
func (w *WorkBuddyConnections) finishCloudCreate(ctx context.Context, permit cloudSubmissionRow, task workbuddy.Task, providerErr error) (cloudSubmissionRow, error) {
	bounded, tx, done, err := w.s.beginAuthorityTx(ctx)
	if err != nil {
		return cloudSubmissionRow{}, err
	}
	defer done()
	defer tx.Rollback()
	if err = w.s.authorityWriter(bounded, tx); err != nil {
		return cloudSubmissionRow{}, err
	}
	if err = w.s.lockOrganization(bounded, tx, permit.tenant); err != nil {
		return cloudSubmissionRow{}, err
	}
	row, err := w.cloudSubmissionTx(bounded, tx, permit.tenant, permit.ProjectID, permit.ID, true)
	if err != nil {
		return row, err
	}
	if permit.Version != 1 || row.Version != permit.Version || row.State != "prepared" || row.nonce != permit.nonce || row.assignmentHash != permit.assignmentHash || row.RequestSHA256 != permit.RequestSHA256 || row.workerHash != permit.workerHash {
		return row, cloudConflict()
	}
	clock, err := w.s.authorityNow(bounded, tx)
	if err != nil {
		return row, err
	}
	row.Version++
	row.UpdatedAt = automationStamp(clock)
	row.State = "unknown"
	row.LastError = "workbuddy_cloud_effect_unknown"
	if providerErr == nil && task.ID != "" {
		row.State = "task_known"
		row.Task = &task
		row.LastError = ""
		row.ObservedAt = row.UpdatedAt
	}
	if err = w.saveCloudSubmissionTx(bounded, tx, row, permit.Version); err != nil {
		return row, err
	}
	return row, tx.Commit()
}

func (w *WorkBuddyConnections) RefreshCloudTask(ctx context.Context, actor Actor, project, id string, req wc.CloudRefreshRequest) (wc.CloudSubmission, error) {
	if project != req.ProjectID {
		return wc.CloudSubmission{}, cloudInvalid()
	}
	return w.refreshCloudTask(ctx, projectCandidateSubject(actor, project), id, req)
}
func (w *WorkBuddyConnections) RefreshPoolCloudTask(ctx context.Context, token, id string, req wc.CloudRefreshRequest) (wc.CloudSubmission, error) {
	return w.refreshCloudTask(ctx, candidateSubject{pool: true, token: token, tenant: req.Assignment.TenantID, project: req.ProjectID}, id, req)
}
func (w *WorkBuddyConnections) refreshCloudTask(ctx context.Context, subject candidateSubject, id string, req wc.CloudRefreshRequest) (wc.CloudSubmission, error) {
	if w == nil || w.s == nil || req.Validate() != nil || !handoffID(id, false) {
		return wc.CloudSubmission{}, cloudInvalid()
	}
	var original cloudSubmissionRow
	var client *workbuddy.Client
	var requestHash string
	replayed := false
	body := struct {
		Kind, ID string
		Request  wc.CloudRefreshRequest
	}{"workbuddy_cloud_refresh", id, req}
	err := func() error {
		bounded, tx, done, err := w.s.beginAuthorityTx(ctx)
		if err != nil {
			return err
		}
		defer done()
		defer tx.Rollback()
		if err = w.s.authorityWriter(bounded, tx); err != nil {
			return err
		}
		// Discovery is a nonlocking read; the complete owner/machine lock set is
		// taken by cloudMachineAuthorityTx before this record is read as authority.
		discovered, err := w.cloudSubmissionTx(bounded, tx, subject.tenant, subject.project, id, false)
		if err != nil {
			return err
		}
		proof, err := w.cloudMachineAuthorityTx(bounded, tx, subject, discovered.DelegationID, req.Assignment, true)
		if err != nil {
			return err
		}
		original, err = w.cloudSubmissionTx(bounded, tx, subject.tenant, subject.project, id, true)
		if err != nil {
			return err
		}
		if original.assignmentHash != digest(stringify(req.Assignment)) || original.workerHash != subject.hash() || original.DelegationID != proof.delegation.ID {
			return forbidden()
		}
		requestHash, replayed, err = w.s.candidateReceipt(bounded, proof.candidate, req.IdempotencyKey, body)
		if err != nil {
			return err
		}
		if replayed {
			return nil
		}
		if original.Version != req.ExpectedVersion || original.Version >= wc.MaxVersion || original.State != "task_known" || original.Task == nil {
			return cloudConflict()
		}
		client = proof.client
		return nil
	}()
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	if replayed {
		return original.CloudSubmission, nil
	}
	task, providerErr := client.GetTask(ctx, original.Task.ID)
	bounded, tx, done, err := w.s.beginAuthorityTx(ctx)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	defer done()
	defer tx.Rollback()
	proof, err := w.cloudMachineAuthorityTx(bounded, tx, subject, original.DelegationID, req.Assignment, false)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	row, err := w.cloudSubmissionTx(bounded, tx, subject.tenant, subject.project, id, true)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	if row.Version != original.Version || row.Task == nil || row.Task.ID != original.Task.ID || row.assignmentHash != original.assignmentHash || row.workerHash != original.workerHash {
		return wc.CloudSubmission{}, cloudConflict()
	}
	// Recheck the key under the same authority transaction; another command may
	// have used it during the GET. Read retries cannot overwrite a newer snapshot.
	hash, replay, err := w.s.candidateReceipt(bounded, proof.candidate, req.IdempotencyKey, body)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	if replay {
		return row.CloudSubmission, nil
	}
	if hash != requestHash {
		return wc.CloudSubmission{}, cloudConflict()
	}
	before := row.Version
	row.Version++
	clock, err := w.s.authorityNow(bounded, tx)
	if err != nil {
		return wc.CloudSubmission{}, err
	}
	row.UpdatedAt = automationStamp(clock)
	if providerErr != nil {
		row.LastError = "workbuddy_cloud_query_failed"
	} else {
		row.Task = &task
		row.ObservedAt = row.UpdatedAt
		row.LastError = ""
	}
	if err = w.saveCloudSubmissionTx(bounded, tx, row, before); err != nil {
		return wc.CloudSubmission{}, err
	}
	if err = w.s.saveCandidateReceipt(bounded, proof.candidate, req.IdempotencyKey, hash, row.ID); err != nil {
		return wc.CloudSubmission{}, err
	}
	if err = w.cloudMachineActivity(bounded, proof, "workbuddy_cloud_observed", row.ID); err != nil {
		return wc.CloudSubmission{}, err
	}
	if err = tx.Commit(); err != nil {
		return wc.CloudSubmission{}, err
	}
	if providerErr != nil {
		return wc.CloudSubmission{}, &APIError{502, "workbuddy_cloud_query_failed", "workbuddy_cloud_query_failed", map[string]any{"submission_id": id}}
	}
	return row.CloudSubmission, nil
}
