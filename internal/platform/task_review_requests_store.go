package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const taskReviewRequestInvalid = "Invalid task review request or cursor"
const taskReviewRequestChanged = "The task review request has changed; reload before deciding"
const taskReviewRequestBinding = "The task or execution binding has changed; reload the current delivery"
const taskReviewRequestRecipient = "Select a current active human with project review authority"
const taskReviewRequestPending = "This task already has a pending review request; cancel it explicitly before requesting again"
const taskReviewRequestPermission = "Only the original requester or a current project administrator may cancel this request"
const taskReviewRequestSource = "The task review request source is invalid"
const taskReviewRequestTimeFormat = "2006-01-02T15:04:05.000000000Z"
const taskReviewRequestColumns = `id,version,project_id,task_id,task_version,run_id,run_version,task_basis_sha256,requester_user_id,requester_name_at_request,reviewer_resource_id,reviewer_user_id,reviewer_name_at_request,note,state,created_at,updated_at,cancelled_by_user_id,cancel_reason,closed_review_id,notification_id`

type taskReviewRequestStored struct {
	Request        TaskReviewRequest
	NotificationID string
}

func validTaskReviewTime(raw string) bool {
	at, err := time.Parse(taskReviewRequestTimeFormat, raw)
	return err == nil && at.Format(taskReviewRequestTimeFormat) == raw
}

func scanTaskReviewRequest(row scanner) (out taskReviewRequestStored, err error) {
	r := &out.Request
	err = row.Scan(&r.ID, &r.Version, &r.ProjectID, &r.TaskID, &r.TaskVersion, &r.RunID, &r.RunVersion, &r.TaskBasisSHA256, &r.RequesterUserID, &r.RequesterNameAtRequest, &r.ReviewerResourceID, &r.ReviewerUserID, &r.ReviewerNameAtRequest, &r.Note, &r.State, &r.CreatedAt, &r.UpdatedAt, &r.CancelledByUserID, &r.CancelReason, &r.ClosedReviewID, &out.NotificationID)
	if err != nil {
		return out, err
	}
	if !handoffID(r.ID, false) || r.Version < 1 || !handoffID(r.ProjectID, false) || !handoffID(r.TaskID, false) || r.TaskVersion < 1 || !diagnosisID(r.RunID, 100) || r.RunVersion < 1 || !contentDigest.MatchString(r.TaskBasisSHA256) || !handoffID(r.RequesterUserID, false) || !handoffID(r.ReviewerResourceID, false) || !handoffID(r.ReviewerUserID, false) || !handoffID(out.NotificationID, false) || !validTaskReviewTime(r.CreatedAt) || !validTaskReviewTime(r.UpdatedAt) || r.State != "pending" && r.State != "cancelled" && r.State != "closed" {
		return out, conflict(taskReviewRequestSource)
	}
	return out, nil
}

func (s *Service) taskReviewRequestTx(ctx context.Context, auth *authorizedCommand, project, id string) (out taskReviewRequestStored, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return out, err
	}
	out, err = scanTaskReviewRequest(auth.tx.QueryRowContext(ctx, s.q(`SELECT `+taskReviewRequestColumns+` FROM suma_task_review_requests WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, id))
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	return out, err
}

func taskReviewRequestEventKey(id, resource, user string) string {
	return digest(stringify(struct {
		Kind, Request, Resource, User string
	}{"task_review_requested", id, resource, user}))
}

// This is direct binding information only. The original Review command retains
// all independent quality, requirement, design, dependency and evidence gates.
func (s *Service) taskReviewBindingCurrentTx(ctx context.Context, auth *authorizedCommand, task Record, request TaskReviewRequest) (bool, error) {
	if task.ID != request.TaskID || task.Version != request.TaskVersion || task.RunID != request.RunID || taskBasis(task) != request.TaskBasisSHA256 || task.Data["run_task_basis"] != request.TaskBasisSHA256 {
		return false, nil
	}
	var version int
	var state string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, task.ProjectID, request.RunID).Scan(&version, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return version == request.RunVersion && state == "succeeded", err
}

func (s *Service) taskReviewRequesterCapabilitiesTx(ctx context.Context, auth *authorizedCommand, project string) (write, admin bool, err error) {
	var orgRole string
	var projectRole sql.NullString
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT om.role,pm.role FROM suma_org_members om LEFT JOIN suma_project_members pm ON pm.project_id=? AND pm.user_id=om.user_id WHERE om.tenant_id=? AND om.user_id=?`), project, auth.target.tenantID, auth.currentPrincipal.ID).Scan(&orgRole, &projectRole)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, forbidden()
	}
	if err != nil {
		return false, false, err
	}
	if !validOrgRole(orgRole) {
		return false, false, forbidden()
	}
	admin = orgRole == "owner" || orgRole == "admin" || projectRole.Valid && projectRole.String == "admin"
	write = admin || projectRole.Valid && (projectRole.String == "reviewer" || projectRole.String == "developer")
	return write, admin, nil
}

func (s *Service) taskReviewReviewerAvailableTx(ctx context.Context, auth *authorizedCommand, request TaskReviewRequest) (bool, error) {
	where, args := s.canonicalHumanTargetsWhere(auth.target.tenantID, request.ProjectID, true)
	args = append(args, request.ReviewerResourceID, request.ReviewerUserID)
	var count int
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+where+` AND m.id=? AND h.user_id=?`), args...).Scan(&count)
	return count == 1, err
}

func (s *Service) hydrateTaskReviewRequestTx(ctx context.Context, auth *authorizedCommand, task Record, stored taskReviewRequestStored, write, admin bool) (out TaskReviewRequest, err error) {
	out = stored.Request
	out.BindingCurrent, err = s.taskReviewBindingCurrentTx(ctx, auth, task, out)
	if err != nil {
		return out, err
	}
	out.ReviewerAvailable, err = s.taskReviewReviewerAvailableTx(ctx, auth, out)
	if err != nil {
		return out, err
	}
	out.IsCurrentAssignee = out.State == "pending" && out.ReviewerAvailable && out.ReviewerUserID == auth.currentPrincipal.ID
	out.CanCancel = out.State == "pending" && write && (admin || out.RequesterUserID == auth.currentPrincipal.ID)
	out.SourceAvailable = task.ID == out.TaskID && task.ProjectID == out.ProjectID
	if out.State == "closed" {
		var decision TaskReviewDecision
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id,reviewer_id,decision,reason,created_at FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND task_version=? AND run_id=? AND id=?`), auth.target.tenantID, out.ProjectID, out.TaskID, out.TaskVersion, out.RunID, out.ClosedReviewID).Scan(&decision.ReviewID, &decision.ReviewerUserID, &decision.Decision, &decision.Reason, &decision.CreatedAt)
		if errors.Is(err, sql.ErrNoRows) {
			out.SourceAvailable = false
			err = nil
		} else if err != nil {
			return out, err
		} else if !handoffID(decision.ReviewID, false) || !handoffID(decision.ReviewerUserID, false) || decision.Decision != "accepted" && decision.Decision != "rejected" {
			out.SourceAvailable = false
		} else {
			out.Decision = &decision
		}
	}
	return out, nil
}

// Invoked only after the original Review INSERT, before changing its task.
// Missing/nonmatching requests never block the original role-authorized Review.
func (s *Service) completeTaskReviewRequestTx(ctx context.Context, auth *authorizedCommand, task Record, reviewID string) error {
	if auth == nil {
		return forbidden()
	}
	if err := assertProjectCommand(auth.tx, auth, task.ProjectID); err != nil {
		return err
	}
	var requestID string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT request_id FROM suma_task_review_pending WHERE tenant_id=? AND project_id=? AND task_id=?`), auth.target.tenantID, task.ProjectID, task.ID).Scan(&requestID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	stored, err := s.taskReviewRequestTx(ctx, auth, task.ProjectID, requestID)
	if err != nil {
		return err
	}
	r := stored.Request
	if r.State != "pending" || r.TaskID != task.ID {
		return conflict(taskReviewRequestSource)
	}
	current, err := s.taskReviewBindingCurrentTx(ctx, auth, task, r)
	if err != nil || !current {
		return err
	}
	var found string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND task_version=? AND run_id=? AND id=? AND reviewer_id=? AND decision IN ('accepted','rejected')`), auth.target.tenantID, task.ProjectID, task.ID, r.TaskVersion, r.RunID, reviewID, auth.currentPrincipal.ID).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return conflict(taskReviewRequestSource)
	}
	if err != nil {
		return err
	}
	at := auth.checkedAt.UTC().Format(taskReviewRequestTimeFormat)
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_task_review_requests SET state='closed',version=version+1,updated_at=?,closed_review_id=? WHERE tenant_id=? AND project_id=? AND id=? AND state='pending' AND version=? AND version<2147483647`), at, reviewID, auth.target.tenantID, task.ProjectID, r.ID, r.Version)
	if err != nil {
		return err
	}
	if count, e := result.RowsAffected(); e != nil || count != 1 {
		if e != nil {
			return e
		}
		return conflict(taskReviewRequestChanged)
	}
	result, err = auth.tx.ExecContext(ctx, s.q(`DELETE FROM suma_task_review_pending WHERE tenant_id=? AND project_id=? AND task_id=? AND request_id=?`), auth.target.tenantID, task.ProjectID, task.ID, r.ID)
	if err != nil {
		return err
	}
	if count, e := result.RowsAffected(); e != nil || count != 1 {
		if e != nil {
			return e
		}
		return conflict(taskReviewRequestChanged)
	}
	return nil
}
