package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"

	"github.com/ai-mada/suma/internal/strictjson"
)

const (
	taskPlanningStatusChangeAPIPath    = "/api/v1/task-planning-status-changes"
	taskPlanningStatusPreviewAPIPath   = taskPlanningStatusChangeAPIPath + "/preview"
	taskPlanningStatusBodyLimit        = 16 << 10
	taskPlanningStatusRunLimit         = 256
	taskPlanningStatusInvalid          = "Invalid task planning status request"
	taskPlanningStatusSelectionChanged = "所选任务已变化，请刷新任务列表"
)

type TaskPlanningStatusSelection struct {
	TaskID          string `json:"task_id"`
	ExpectedVersion int    `json:"expected_version"`
}

type TaskPlanningStatusPreviewRequest struct {
	ProjectID string                        `json:"project_id"`
	Tasks     []TaskPlanningStatusSelection `json:"tasks"`
}

type TaskPlanningStatusPreviewItem struct {
	TaskID      string `json:"task_id"`
	Version     int    `json:"version"`
	Status      string `json:"status"`
	Eligibility string `json:"eligibility"`
}

type TaskPlanningStatusPreview struct {
	ProjectID string                          `json:"project_id"`
	Items     []TaskPlanningStatusPreviewItem `json:"items"`
}

type TaskPlanningStatusChangeRequest struct {
	ProjectID      string                        `json:"project_id"`
	Status         string                        `json:"status"`
	Tasks          []TaskPlanningStatusSelection `json:"tasks"`
	IdempotencyKey string                        `json:"idempotency_key"`
}

type TaskPlanningStatusChangeReceipt struct {
	TaskID          string `json:"task_id"`
	PreviousVersion int    `json:"previous_version"`
	Version         int    `json:"version"`
	PreviousStatus  string `json:"previous_status"`
	Status          string `json:"status"`
	Changed         bool   `json:"changed"`
}

type TaskPlanningStatusChangeResult struct {
	ProjectID   string                            `json:"project_id"`
	OperationID string                            `json:"operation_id"`
	Changes     []TaskPlanningStatusChangeReceipt `json:"changes"`
}

func manualTaskPlanningStatus(status string) bool {
	switch status {
	case "draft", "todo", "in_progress", "blocked", "cancelled":
		return true
	}
	return false
}

func validateTaskPlanningStatusSelection(project string, selected []TaskPlanningStatusSelection) error {
	if !handoffID(project, false) || len(selected) < 1 || len(selected) > 16 {
		return invalid(taskPlanningStatusInvalid)
	}
	seen := make(map[string]bool, len(selected))
	for _, task := range selected {
		if !handoffID(task.TaskID, false) || seen[task.TaskID] || task.ExpectedVersion < 1 || task.ExpectedVersion > math.MaxInt32 {
			return invalid(taskPlanningStatusInvalid)
		}
		seen[task.TaskID] = true
	}
	return nil
}

// Protect workflow-owned records even if another authorized editor changed the
// displayed status but the canonical verification evidence is still present.
func protectedTaskPlanningStatus(status, data string) bool {
	if !manualTaskPlanningStatus(status) {
		return true
	}
	var fields map[string]json.RawMessage
	raw := []byte(data)
	if strictjson.Decode(raw, &fields, len(raw)) != nil || fields == nil {
		return true
	}
	value, present := fields["verified_run_id"]
	if !present {
		return false
	}
	var run *string
	return json.Unmarshal(value, &run) != nil || run == nil || *run != ""
}

type taskPlanningStatusRow struct {
	item  TaskPlanningStatusPreviewItem
	title string
}

func (s *Service) taskPlanningStatusRowsTx(ctx context.Context, auth *authorizedCommand, project string, selected []TaskPlanningStatusSelection) ([]taskPlanningStatusRow, error) {
	tenant, tx := auth.target.tenantID, auth.tx
	out := make([]taskPlanningStatusRow, 0, len(selected))
	for _, task := range selected {
		row := taskPlanningStatusRow{item: TaskPlanningStatusPreviewItem{TaskID: task.TaskID, Eligibility: "editable"}}
		var run, data string
		err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT version,status,title,run_id,data FROM suma_tasks
			WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, task.TaskID).
			Scan(&row.item.Version, &row.item.Status, &row.title, &run, &data)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, conflict(taskPlanningStatusSelectionChanged)
		}
		if err != nil {
			return nil, err
		}
		if row.item.Version != task.ExpectedVersion {
			return nil, conflict(taskPlanningStatusSelectionChanged)
		}
		if protectedTaskPlanningStatus(row.item.Status, data) {
			row.item.Eligibility = "workflow_status"
		} else {
			row.item.Eligibility, err = s.taskPlanningStatusRunEligibility(ctx, tx, tenant, project, task.TaskID, run)
			if err != nil {
				return nil, err
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// The same project guard that serializes scheduler transitions is held here.
// A fresh task binding can leave its previous run live. Search the original
// scoped provenance for unresolved roots and for runs with linked effects.
// Terminal standalone history needs no individual round-trip or review.
func (s *Service) taskPlanningStatusRunEligibility(ctx context.Context, tx *sql.Tx, tenant, project, task, currentRun string) (string, error) {
	if currentRun != "" {
		eligibility, err := s.taskPlanningStatusRunFamilyEligibility(ctx, tx, tenant, project, currentRun, []string{currentRun})
		if err != nil || eligibility != "editable" {
			return eligibility, err
		}
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT p.run_id, r.state FROM suma_task_run_provenance p
		LEFT JOIN dag_runs r ON r.id=p.run_id AND r.tenant_id=p.tenant_id AND r.project_id=p.project_id
		WHERE p.tenant_id=? AND p.project_id=? AND p.task_id=? AND p.run_id<>?
		AND (r.id IS NULL OR r.state NOT IN ('succeeded','failed','cancelled')
			OR EXISTS (SELECT 1 FROM dag_subruns x WHERE x.parent_run_id=p.run_id)
			OR EXISTS (SELECT 1 FROM dag_compensations c WHERE c.original_run_id=p.run_id))
		ORDER BY p.run_id LIMIT ?`), tenant, project, task, currentRun, taskPlanningStatusRunLimit+1)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var history []string
	for rows.Next() {
		var run string
		var state sql.NullString
		if err = rows.Scan(&run, &state); err != nil {
			return "", err
		}
		switch state.String {
		case "running", "paused", "cancelling":
			return "active_run", nil
		case "succeeded", "failed", "cancelled":
		default:
			return "unverified_run", nil
		}
		if len(history) == taskPlanningStatusRunLimit {
			return "unverified_run", nil
		}
		history = append(history, run)
	}
	if err = rows.Err(); err != nil {
		return "", err
	}
	if err = rows.Close(); err != nil {
		return "", err
	}
	return s.taskPlanningStatusRunFamilyEligibility(ctx, tx, tenant, project, "", history)
}

// Terminal DAG runs are derived from node outcomes. A subrun may independently
// acquire a compensation run, so check the bounded family before allowing edits.
func (s *Service) taskPlanningStatusRunFamilyEligibility(ctx context.Context, tx *sql.Tx, tenant, project, reviewRun string, family []string) (string, error) {
	seen := make(map[string]bool, len(family))
	for _, id := range family {
		seen[id] = true
	}
	for head := 0; head < len(family); head++ {
		id := family[head]
		var state string
		err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, id).Scan(&state)
		if errors.Is(err, sql.ErrNoRows) {
			return "unverified_run", nil
		}
		if err != nil {
			return "", err
		}
		switch state {
		case "running", "paused", "cancelling":
			return "active_run", nil
		case "succeeded":
			if id == reviewRun {
				return "review_required", nil
			}
		case "failed", "cancelled":
		default:
			return "unverified_run", nil
		}
		rows, err := tx.QueryContext(ctx, s.q(`SELECT child_run_id FROM dag_subruns WHERE parent_run_id=?
			UNION ALL SELECT compensation_run_id FROM dag_compensations WHERE original_run_id=? ORDER BY 1 LIMIT ?`), id, id, taskPlanningStatusRunLimit+1)
		if err != nil {
			return "", err
		}
		for rows.Next() {
			var child string
			if err := rows.Scan(&child); err != nil {
				rows.Close()
				return "", err
			}
			if seen[child] || len(family) == taskPlanningStatusRunLimit {
				rows.Close()
				return "unverified_run", nil
			}
			seen[child] = true
			family = append(family, child)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
	}
	return "editable", nil
}

func (s *Service) PreviewTaskPlanningStatuses(ctx context.Context, actor Actor, req TaskPlanningStatusPreviewRequest) (TaskPlanningStatusPreview, error) {
	var out TaskPlanningStatusPreview
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if err := validateTaskPlanningStatusSelection(req.ProjectID, req.Tasks); err != nil {
		return out, err
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, req.ProjectID, req.ProjectID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	rows, err := s.taskPlanningStatusRowsTx(ctx, auth, req.ProjectID, req.Tasks)
	if err != nil {
		return out, err
	}
	out = TaskPlanningStatusPreview{ProjectID: req.ProjectID, Items: make([]TaskPlanningStatusPreviewItem, 0, len(rows))}
	for _, row := range rows {
		out.Items = append(out.Items, row.item)
	}
	return out, nil
}

func (s *Service) ChangeTaskPlanningStatuses(ctx context.Context, actor Actor, req TaskPlanningStatusChangeRequest) (TaskPlanningStatusChangeResult, error) {
	var out TaskPlanningStatusChangeResult
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !manualTaskPlanningStatus(req.Status) || !planningKey(req.IdempotencyKey) {
		return out, invalid(taskPlanningStatusInvalid)
	}
	if err := validateTaskPlanningStatusSelection(req.ProjectID, req.Tasks); err != nil {
		return out, err
	}
	project := req.ProjectID
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, project, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()

	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_planning_status_changes": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	rows, err := s.taskPlanningStatusRowsTx(ctx, auth, project, req.Tasks)
	if err != nil {
		return out, err
	}
	receipts := make([]TaskPlanningStatusChangeReceipt, 0, len(rows))
	for _, row := range rows {
		if row.item.Eligibility != "editable" {
			return out, &APIError{http.StatusConflict, "task_planning_status_locked", "任务需要在原运行或验收流程中处理，请重新预览", nil}
		}
		item := row.item
		receipt := TaskPlanningStatusChangeReceipt{TaskID: item.TaskID, PreviousVersion: item.Version, Version: item.Version,
			PreviousStatus: item.Status, Status: req.Status, Changed: item.Status != req.Status}
		if receipt.Changed {
			if receipt.Version == math.MaxInt32 {
				return out, conflict(recordVersionExhausted)
			}
			receipt.Version++
		}
		receipts = append(receipts, receipt)
	}

	at := now()
	for i, receipt := range receipts {
		if !receipt.Changed {
			continue
		}
		update, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET status=?,version=?,updated_at=?
			WHERE tenant_id=? AND project_id=? AND id=? AND version=?`),
			req.Status, receipt.Version, at, auth.target.tenantID, project, receipt.TaskID, receipt.PreviousVersion)
		if err != nil {
			return out, err
		}
		count, err := update.RowsAffected()
		if err != nil {
			return out, err
		}
		if count != 1 {
			return out, conflict(taskPlanningStatusSelectionChanged)
		}
		if err = s.commandActivity(ctx, auth, "tasks.updated", receipt.TaskID, rows[i].title); err != nil {
			return out, err
		}
	}
	out = TaskPlanningStatusChangeResult{ProjectID: project, OperationID: req.IdempotencyKey, Changes: receipts}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return TaskPlanningStatusChangeResult{}, err
	}
	if err = auth.tx.Commit(); err != nil {
		return TaskPlanningStatusChangeResult{}, err
	}
	return out, nil
}

func (s *Server) taskPlanningStatusChangesHTTP(w http.ResponseWriter, r *http.Request, actor Actor) {
	w.Header().Set("Cache-Control", "no-store")
	if !myWorkHuman(actor) {
		writeError(w, forbidden())
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "Method not allowed"})
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery {
		writeError(w, invalid(taskPlanningStatusInvalid))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, taskPlanningStatusBodyLimit+1))
	if err != nil {
		writeError(w, invalid(taskPlanningStatusInvalid))
		return
	}
	if r.URL.Path == taskPlanningStatusPreviewAPIPath {
		var req TaskPlanningStatusPreviewRequest
		if strictjson.Decode(raw, &req, taskPlanningStatusBodyLimit) != nil {
			writeError(w, invalid(taskPlanningStatusInvalid))
			return
		}
		out, err := s.Service.PreviewTaskPlanningStatuses(r.Context(), actor, req)
		respond(w, http.StatusOK, out, err)
		return
	}
	var req TaskPlanningStatusChangeRequest
	if strictjson.Decode(raw, &req, taskPlanningStatusBodyLimit) != nil {
		writeError(w, invalid(taskPlanningStatusInvalid))
		return
	}
	out, err := s.Service.ChangeTaskPlanningStatuses(r.Context(), actor, req)
	respond(w, http.StatusOK, out, err)
}
