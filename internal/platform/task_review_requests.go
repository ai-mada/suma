package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

func taskReviewRequestText(value string) bool {
	return utf8.ValidString(value) && len(value) <= 8000 && utf8.RuneCountInString(value) <= 2000 && strings.TrimSpace(value) != "" && !strings.ContainsRune(value, 0)
}

func taskReviewRequestKey(value string) bool {
	return planningKey(value) && strings.TrimSpace(value) != ""
}

func validTaskReviewRequestCreate(project string, req TaskReviewRequestCreate) bool {
	return req.ProjectID == project && handoffID(project, false) && handoffID(req.TaskID, false) && req.ExpectedTaskVersion >= 1 && req.ExpectedTaskVersion <= 2147483646 && diagnosisID(req.RunID, 100) && req.ExpectedRunVersion >= 1 && req.ExpectedRunVersion <= 2147483646 && handoffID(req.ReviewerResourceID, false) && taskReviewRequestText(req.Note) && taskReviewRequestKey(req.IdempotencyKey)
}

func validTaskReviewRequestCommand(project, id string, req TaskReviewRequestCommand) bool {
	return req.ProjectID == project && handoffID(project, false) && handoffID(id, false) && req.ExpectedVersion >= 1 && req.ExpectedVersion <= 2147483646 && req.Action == "cancel" && taskReviewRequestText(req.Reason) && taskReviewRequestKey(req.IdempotencyKey)
}

func (s *Service) CreateTaskReviewRequest(ctx context.Context, actor Actor, project string, req TaskReviewRequestCreate) (out TaskReviewRequest, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !validTaskReviewRequestCreate(project, req) {
		return out, invalid(taskReviewRequestInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, req.TaskID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_review_request_create": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return out, auth.assertTx(auth.tx, auth.target, purposeProjectWrite)
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", req.TaskID)
	if err != nil {
		return out, err
	}
	if task.Version != req.ExpectedTaskVersion || task.RunID != req.RunID || task.Data["run_task_basis"] != taskBasis(task) {
		return out, conflict(taskReviewRequestBinding)
	}
	var runVersion int
	var runState string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT version,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, req.RunID).Scan(&runVersion, &runState)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (runVersion != req.ExpectedRunVersion || runState != "succeeded") {
		return out, conflict(taskReviewRequestBinding)
	}
	if err != nil {
		return out, err
	}
	var pending string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT request_id FROM suma_task_review_pending WHERE tenant_id=? AND project_id=? AND task_id=?`), auth.target.tenantID, project, task.ID).Scan(&pending)
	if err == nil {
		return out, conflict(taskReviewRequestPending)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	where, args := s.canonicalHumanTargetsWhere(auth.target.tenantID, project, true)
	args = append(args, req.ReviewerResourceID)
	var target TaskReviewTarget
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT m.id,h.user_id,m.title,m.version`+where+` AND m.id=?`), args...).Scan(&target.ResourceID, &target.UserID, &target.Name, &target.ResourceVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return out, invalid(taskReviewRequestRecipient)
	}
	if err != nil {
		return out, err
	}
	if !handoffID(target.UserID, false) || target.ResourceID != req.ReviewerResourceID || target.ResourceVersion < 1 {
		return out, conflict(taskReviewRequestSource)
	}
	at := auth.checkedAt.UTC().Format(taskReviewRequestTimeFormat)
	out = TaskReviewRequest{ID: newID(), Version: 1, ProjectID: project, TaskID: task.ID, TaskVersion: task.Version, RunID: req.RunID, RunVersion: runVersion, TaskBasisSHA256: taskBasis(task), RequesterUserID: auth.currentPrincipal.ID, RequesterNameAtRequest: auth.currentPrincipal.Name, ReviewerResourceID: target.ResourceID, ReviewerUserID: target.UserID, ReviewerNameAtRequest: target.Name, Note: req.Note, State: "pending", CreatedAt: at, UpdatedAt: at, BindingCurrent: true, ReviewerAvailable: true, IsCurrentAssignee: target.UserID == auth.currentPrincipal.ID, CanCancel: true, SourceAvailable: true}
	notificationID, err := s.insertNotificationInboxTx(ctx, auth, project, notificationInboxSource{Kind: "task_review_requested", TaskID: task.ID, TaskVersion: task.Version, ResourceID: target.ResourceID, EventKey: taskReviewRequestEventKey(out.ID, target.ResourceID, target.UserID), RecipientUserID: target.UserID})
	if err != nil {
		return TaskReviewRequest{}, err
	}
	_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_review_requests(tenant_id,`+taskReviewRequestColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), auth.target.tenantID, out.ID, out.Version, out.ProjectID, out.TaskID, out.TaskVersion, out.RunID, out.RunVersion, out.TaskBasisSHA256, out.RequesterUserID, out.RequesterNameAtRequest, out.ReviewerResourceID, out.ReviewerUserID, out.ReviewerNameAtRequest, out.Note, out.State, out.CreatedAt, out.UpdatedAt, "", "", "", notificationID)
	if err != nil {
		return TaskReviewRequest{}, err
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_review_pending(tenant_id,project_id,task_id,request_id) VALUES(?,?,?,?)`), auth.target.tenantID, project, task.ID, out.ID); err != nil {
		return TaskReviewRequest{}, err
	}
	if err = s.commandActivity(ctx, auth, "task.review_requested", task.ID, "Task delivery review requested"); err != nil {
		return TaskReviewRequest{}, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return TaskReviewRequest{}, err
	}
	if err = auth.assertTx(auth.tx, auth.target, purposeProjectWrite); err != nil {
		return TaskReviewRequest{}, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) CommandTaskReviewRequest(ctx context.Context, actor Actor, project, id string, req TaskReviewRequestCommand) (out TaskReviewRequest, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !validTaskReviewRequestCommand(project, id, req) {
		return out, invalid(taskReviewRequestInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_review_request_command": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return out, auth.assertTx(auth.tx, auth.target, purposeProjectWrite)
	}
	stored, err := s.taskReviewRequestTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	write, admin, err := s.taskReviewRequesterCapabilitiesTx(ctx, auth, project)
	if err != nil {
		return out, err
	}
	if !write || !admin && stored.Request.RequesterUserID != auth.currentPrincipal.ID {
		return out, &APIError{403, "forbidden", taskReviewRequestPermission, nil}
	}
	if stored.Request.Version != req.ExpectedVersion || stored.Request.State != "pending" {
		return out, conflict(taskReviewRequestChanged)
	}
	at := auth.checkedAt.UTC().Format(taskReviewRequestTimeFormat)
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_task_review_requests SET state='cancelled',version=version+1,updated_at=?,cancelled_by_user_id=?,cancel_reason=? WHERE tenant_id=? AND project_id=? AND id=? AND state='pending' AND version=?`), at, auth.currentPrincipal.ID, req.Reason, auth.target.tenantID, project, id, req.ExpectedVersion)
	if err != nil {
		return out, err
	}
	if count, e := result.RowsAffected(); e != nil || count != 1 {
		if e != nil {
			return out, e
		}
		return out, conflict(taskReviewRequestChanged)
	}
	result, err = auth.tx.ExecContext(ctx, s.q(`DELETE FROM suma_task_review_pending WHERE tenant_id=? AND project_id=? AND task_id=? AND request_id=?`), auth.target.tenantID, project, stored.Request.TaskID, id)
	if err != nil {
		return out, err
	}
	if count, e := result.RowsAffected(); e != nil || count != 1 {
		if e != nil {
			return out, e
		}
		return out, conflict(taskReviewRequestChanged)
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", stored.Request.TaskID)
	if err != nil {
		return out, err
	}
	stored.Request.State, stored.Request.Version, stored.Request.UpdatedAt = "cancelled", req.ExpectedVersion+1, at
	stored.Request.CancelledByUserID, stored.Request.CancelReason = auth.currentPrincipal.ID, req.Reason
	out, err = s.hydrateTaskReviewRequestTx(ctx, auth, task, stored, write, admin)
	if err != nil {
		return TaskReviewRequest{}, err
	}
	if err = s.commandActivity(ctx, auth, "task.review_request_cancelled", task.ID, "Task delivery review request cancelled"); err != nil {
		return TaskReviewRequest{}, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return TaskReviewRequest{}, err
	}
	if err = auth.assertTx(auth.tx, auth.target, purposeProjectWrite); err != nil {
		return TaskReviewRequest{}, err
	}
	return out, auth.tx.Commit()
}

type taskReviewRequestCursor struct {
	Version int    `json:"version"`
	Tenant  string `json:"tenant"`
	User    string `json:"user"`
	Project string `json:"project"`
	Task    string `json:"task"`
	Limit   int    `json:"limit"`
	Created string `json:"created"`
	ID      string `json:"id"`
}

func (s *Service) ListTaskReviewRequests(ctx context.Context, actor Actor, project, taskID string, limit int, cursor string) (out TaskReviewRequestPage, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || !handoffID(taskID, false) || limit < 1 || limit > 50 || len(cursor) > 4096 {
		return out, invalid(taskReviewRequestInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return out, err
	}
	write, admin, err := s.taskReviewRequesterCapabilitiesTx(ctx, auth, project)
	if err != nil {
		return out, err
	}
	out = TaskReviewRequestPage{ProjectID: project, TaskID: taskID, TaskVersion: task.Version, CanRequest: write, Limit: limit, Items: []TaskReviewRequest{}}
	scope := taskReviewRequestCursor{Version: 1, Tenant: auth.target.tenantID, User: auth.currentPrincipal.ID, Project: project, Task: taskID, Limit: limit}
	var position taskReviewRequestCursor
	if cursor != "" {
		raw, decodeErr := base64.RawURLEncoding.Strict().DecodeString(cursor)
		if decodeErr != nil || json.Unmarshal(raw, &position) != nil || base64.RawURLEncoding.EncodeToString([]byte(stringify(position))) != cursor {
			return out, invalid(taskReviewRequestInvalid)
		}
		identity := position
		identity.Created, identity.ID = "", ""
		if identity != scope || !validTaskReviewTime(position.Created) || !handoffID(position.ID, false) {
			return out, invalid(taskReviewRequestInvalid)
		}
	}
	where := ` FROM suma_task_review_requests WHERE tenant_id=? AND project_id=? AND task_id=?`
	args := []any{auth.target.tenantID, project, taskID}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT p.request_id FROM suma_task_review_pending p JOIN suma_task_review_requests r ON r.tenant_id=p.tenant_id AND r.project_id=p.project_id AND r.task_id=p.task_id AND r.id=p.request_id AND r.state='pending' WHERE p.tenant_id=? AND p.project_id=? AND p.task_id=?`), auth.target.tenantID, project, taskID).Scan(&out.PendingRequestID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if cursor != "" {
		where += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, position.Created, position.Created, position.ID)
	}
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT `+taskReviewRequestColumns+where+` ORDER BY created_at DESC,id DESC LIMIT ?`), args...)
	if err != nil {
		return out, err
	}
	stored := []taskReviewRequestStored{}
	for rows.Next() {
		item, scanErr := scanTaskReviewRequest(rows)
		if scanErr != nil {
			rows.Close()
			return TaskReviewRequestPage{}, scanErr
		}
		stored = append(stored, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return TaskReviewRequestPage{}, err
	}
	if len(stored) > limit {
		stored = stored[:limit]
		scope.Created, scope.ID = stored[limit-1].Request.CreatedAt, stored[limit-1].Request.ID
		out.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	for _, item := range stored {
		hydrated, hydrateErr := s.hydrateTaskReviewRequestTx(ctx, auth, task, item, write, admin)
		if hydrateErr != nil {
			return TaskReviewRequestPage{}, hydrateErr
		}
		out.Items = append(out.Items, hydrated)
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}

func (s *Service) GetTaskReviewRequest(ctx context.Context, actor Actor, project, id string) (out TaskReviewRequestDetail, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(taskReviewRequestInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	stored, err := s.taskReviewRequestTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", stored.Request.TaskID)
	if err != nil {
		return out, err
	}
	write, admin, err := s.taskReviewRequesterCapabilitiesTx(ctx, auth, project)
	if err != nil {
		return out, err
	}
	request, err := s.hydrateTaskReviewRequestTx(ctx, auth, task, stored, write, admin)
	if err != nil {
		return out, err
	}
	out = TaskReviewRequestDetail{ProjectID: project, TaskID: task.ID, TaskTitle: task.Title, TaskVersion: task.Version, CanRequest: write, Request: request}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}
