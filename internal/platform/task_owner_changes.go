package platform

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

const taskOwnerChangeInvalid = "Invalid task owner change request"
const taskOwnerTargetInvalid = "Invalid task owner target query or cursor"
const taskOwnerChangeChanged = "Selected tasks changed; keep the selection and preview again"
const taskOwnerTargetUnavailable = "The selected canonical human owner is no longer available"
const taskOwnerTargetChanged = "The selected owner resource changed; reload the people directory"
const taskOwnerPreviewChanged = "The task owner change preview changed; preview again before confirming"
const taskOwnerChangeUnchanged = "The selected tasks already have this owner"
const taskOwnerVersionExhausted = "Task version limit reached; review the task before changing its owner"

func validateTaskOwnerSelection(req TaskOwnerChangePreviewRequest) error {
	if !handoffID(req.ProjectID, false) || !handoffID(req.OwnerID, false) || req.ExpectedOwnerVersion < 1 || req.ExpectedOwnerVersion > 2147483647 || len(req.Tasks) < 1 || len(req.Tasks) > 16 {
		return invalid(taskOwnerChangeInvalid)
	}
	seen := map[string]bool{}
	for _, task := range req.Tasks {
		if !handoffID(task.TaskID, false) || task.ExpectedVersion < 1 || task.ExpectedVersion > 2147483647 || seen[task.TaskID] {
			return invalid(taskOwnerChangeInvalid)
		}
		seen[task.TaskID] = true
	}
	return nil
}

func (s *Service) PreviewTaskOwnerChanges(ctx context.Context, actor Actor, project string, req TaskOwnerChangePreviewRequest) (out TaskOwnerChangePreview, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if project != req.ProjectID {
		return out, invalid(taskOwnerChangeInvalid)
	}
	if err = validateTaskOwnerSelection(req); err != nil {
		return out, err
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, project, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	out, _, err = s.taskOwnerChangePreviewTx(ctx, auth, project, req)
	if err == nil {
		err = auth.assertTx(auth.tx, auth.target, purposeProjectWrite)
	}
	return out, err
}

func (s *Service) ChangeTaskOwners(ctx context.Context, actor Actor, project string, req TaskOwnerChangeRequest) (out TaskOwnerChangeResult, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if project == "" || req.ProjectID != project {
		return out, invalid(taskOwnerChangeInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, project, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	// Current caller authority precedes replay; legitimate later task/owner
	// changes must not prevent recovery of this original immutable receipt.
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_owner_changes": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	previewRequest := TaskOwnerChangePreviewRequest{ProjectID: req.ProjectID, OwnerID: req.OwnerID, ExpectedOwnerVersion: req.ExpectedOwnerVersion, Tasks: req.Tasks}
	if err = validateTaskOwnerSelection(previewRequest); err != nil {
		return out, err
	}
	sha, decodeErr := hex.DecodeString(req.PreviewSHA256)
	if !req.AcknowledgeInvalidation || decodeErr != nil || len(sha) != 32 || hex.EncodeToString(sha) != req.PreviewSHA256 {
		return out, invalid(taskOwnerChangeInvalid)
	}
	preview, selected, err := s.taskOwnerChangePreviewTx(ctx, auth, project, previewRequest)
	if err != nil {
		return out, err
	}
	if preview.PreviewSHA256 != req.PreviewSHA256 {
		return out, conflict(taskOwnerPreviewChanged)
	}
	if !preview.CanApply {
		return out, &APIError{http.StatusUnprocessableEntity, "invalid_input", taskOwnerChangeUnchanged, nil}
	}
	roots := make([]string, 0, preview.ChangedCount)
	for _, task := range selected {
		if task.OwnerID == req.OwnerID {
			continue
		}
		_, err = s.updateCommandRecordTx(ctx, auth, project, "tasks", task.ID, map[string]any{"expected_version": task.Version, "owner_id": req.OwnerID})
		if err != nil {
			return out, err
		}
		roots = append(roots, task.ID)
	}
	if err = s.InvalidateDependentsTx(ctx, auth.tx, auth, project, roots, false); err != nil {
		return out, err
	}
	out = TaskOwnerChangeResult{ProjectID: project, OperationID: req.IdempotencyKey, Changes: []TaskOwnerChangeReceipt{}}
	for _, before := range selected {
		// An unchanged-owner selected descendant may have lost acceptance, so
		// receipts reflect its final committed version rather than an early skip.
		after, e := scanRecord(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, before.ID))
		if e != nil {
			return out, e
		}
		ownerChanged := before.OwnerID != req.OwnerID
		if ownerChanged {
			if e = s.commandActivity(ctx, auth, "tasks.updated", after.ID, after.Title); e != nil {
				return out, e
			}
		}
		out.Changes = append(out.Changes, TaskOwnerChangeReceipt{TaskID: before.ID, PreviousVersion: before.Version, Version: after.Version, PreviousOwnerID: before.OwnerID, OwnerID: after.OwnerID, Status: after.Status, OwnerChanged: ownerChanged})
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
