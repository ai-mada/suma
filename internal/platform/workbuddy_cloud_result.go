package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// cloudResultResolver records the original machine's already incurred effect.
// Its caller has locked current machine/organization/run authority. It does not
// acquire a human grant, decrypt OAuth or call the provider, so an owner's later
// revocation cannot erase a real effect or grant another execution opportunity.
func (w *WorkBuddyConnections) cloudResultResolver(subject candidateSubject) dag.ExternalResultResolver {
	return func(ctx context.Context, tx *sql.Tx, snapshot dag.ExternalAssignment, result dag.Result) (dag.ExternalResultResolution, error) {
		var empty dag.ExternalResultResolution
		if w == nil || w.s == nil || result.WorkBuddyCloudWait == nil {
			return empty, cloudInvalid()
		}
		e := result.WorkBuddyCloudWait
		d, err := w.cloudDelegationTx(ctx, tx, subject.tenant, subject.project, snapshot.Binding.DelegationID, true)
		if err != nil {
			return empty, err
		}
		binding, err := d.AssignmentBinding()
		if err != nil || binding.DelegationSHA256 != snapshot.Binding.DelegationSHA256 || d.RunID != snapshot.RunID || d.NodeID != snapshot.NodeID || d.Version < snapshot.Binding.DelegationVersion {
			return empty, cloudConflict()
		}
		var id string
		err = tx.QueryRowContext(ctx, w.s.q(w.s.currentSQL("SELECT id FROM suma_workbuddy_cloud_submissions WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?")), subject.tenant, subject.project, snapshot.RunID, snapshot.NodeID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			if d.CreatesConsumed != 0 || e.SubmissionID != "" {
				return empty, cloudConflict()
			}
			// The locked run is leaving running before this transaction commits.
			// A late old Create must recheck it and cannot acquire a send permit.
			return dag.ExternalResultResolution{NoExternalEffect: true}, nil
		}
		if err != nil {
			return empty, err
		}
		sub, err := w.cloudSubmissionTx(ctx, tx, subject.tenant, subject.project, id, true)
		if err != nil {
			return empty, err
		}
		if d.CreatesConsumed != 1 || sub.DelegationID != d.ID || sub.RunID != snapshot.RunID || sub.NodeID != snapshot.NodeID || sub.RequestSHA256 != d.RequestSHA256 ||
			sub.workerHash != subject.hash() || sub.assignmentHash != digest(stringify(e.Assignment)) || !wc.SameAssignmentIdentity(snapshot.Assignment, sub.Assignment) {
			return empty, cloudConflict()
		}
		if e.SubmissionID != "" && (e.SubmissionID != sub.ID || e.SubmissionVersion > sub.Version) {
			return empty, cloudConflict()
		}
		// Bind the reported original operation to the receipt written atomically
		// with prepare. Knowing a submission ID alone cannot replace its body/key.
		body := struct {
			Kind    string
			Request wc.CloudCreateRequest
		}{"workbuddy_cloud_create", wc.CloudCreateRequest{ProjectID: subject.project, DelegationID: d.ID, ExpectedVersion: snapshot.Binding.DelegationVersion, Assignment: e.Assignment, IdempotencyKey: e.CreateOperationID}}
		var requestHash, response string
		if err = tx.QueryRowContext(ctx, w.s.q("SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?"), subject.tenant, subject.project, e.CreateOperationID).Scan(&requestHash, &response); err != nil {
			return empty, cloudConflict()
		}
		var receiptID string
		if requestHash != digest(subject.hash()+stringify(body)) || json.Unmarshal([]byte(response), &receiptID) != nil || receiptID != sub.ID {
			return empty, cloudConflict()
		}
		reason := "cloud.observation_unconfirmed"
		switch sub.State {
		case "prepared":
			reason = "cloud.creation_unconfirmed"
		case "unknown":
			reason = "cloud.remote_unknown"
		case "task_known":
			if sub.Task == nil {
				return empty, cloudConflict()
			}
			if sub.Task.Status == "completed" {
				reason = "cloud.delivery_required"
			} else if sub.Task.Status == "waiting_input" {
				reason = "cloud.waiting_input"
			}
		default:
			return empty, cloudConflict()
		}
		return dag.ExternalResultResolution{Reason: reason}, nil
	}
}
