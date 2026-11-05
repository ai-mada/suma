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
	taskPriorityChangeAPIPath    = "/api/v1/task-priority-changes"
	taskPriorityPreviewAPIPath   = taskPriorityChangeAPIPath + "/preview"
	taskPriorityChangeBodyLimit  = 16 << 10
	taskPriorityChangeInvalid    = "Invalid task priority change request"
	taskPrioritySelectionChanged = "所选任务已变化或不可编辑，请刷新任务列表"
)

type TaskPriorityChangeSelection struct {
	TaskID          string `json:"task_id"`
	ExpectedVersion int    `json:"expected_version"`
}

type TaskPriorityChangeRequest struct {
	ProjectID      string                        `json:"project_id"`
	Priority       string                        `json:"priority"`
	Tasks          []TaskPriorityChangeSelection `json:"tasks"`
	IdempotencyKey string                        `json:"idempotency_key"`
}

type TaskPriorityChangePreviewRequest struct {
	ProjectID string                        `json:"project_id"`
	Priority  string                        `json:"priority"`
	Tasks     []TaskPriorityChangeSelection `json:"tasks"`
}

type TaskPriorityChangePreviewItem struct {
	TaskID           string `json:"task_id"`
	Version          int    `json:"version"`
	PreviousPriority string `json:"previous_priority"`
	Priority         string `json:"priority"`
	Changed          bool   `json:"changed"`
}

type TaskPriorityChangePreview struct {
	ProjectID string                          `json:"project_id"`
	Items     []TaskPriorityChangePreviewItem `json:"items"`
}

type TaskPriorityChangeReceipt struct {
	TaskID           string `json:"task_id"`
	PreviousVersion  int    `json:"previous_version"`
	Version          int    `json:"version"`
	PreviousPriority string `json:"previous_priority"`
	Priority         string `json:"priority"`
	Changed          bool   `json:"changed"`
}

type TaskPriorityChangeResult struct {
	ProjectID   string                      `json:"project_id"`
	OperationID string                      `json:"operation_id"`
	Changes     []TaskPriorityChangeReceipt `json:"changes"`
}

func validateTaskPrioritySelection(project, priority string, tasks []TaskPriorityChangeSelection) error {
	switch priority {
	case "urgent", "high", "medium", "low":
	default:
		return invalid(taskPriorityChangeInvalid)
	}
	if !handoffID(project, false) || len(tasks) < 1 || len(tasks) > 16 {
		return invalid(taskPriorityChangeInvalid)
	}
	seen := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		if !handoffID(task.TaskID, false) || seen[task.TaskID] || task.ExpectedVersion < 1 || task.ExpectedVersion > math.MaxInt32 {
			return invalid(taskPriorityChangeInvalid)
		}
		seen[task.TaskID] = true
	}
	return nil
}

type taskPriorityChangeRow struct {
	item  TaskPriorityChangePreviewItem
	title string
}

func (s *Service) taskPriorityChangeRowsTx(ctx context.Context, auth *authorizedCommand, project, priority string, tasks []TaskPriorityChangeSelection) ([]taskPriorityChangeRow, error) {
	rows := make([]taskPriorityChangeRow, 0, len(tasks))
	for _, selected := range tasks {
		row := taskPriorityChangeRow{item: TaskPriorityChangePreviewItem{TaskID: selected.TaskID, Priority: priority}}
		var status string
		err := auth.tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT version,priority,status,title FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`)),
			auth.target.tenantID, project, selected.TaskID).Scan(&row.item.Version, &row.item.PreviousPriority, &status, &row.title)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, conflict(taskPrioritySelectionChanged)
		}
		if err != nil {
			return nil, err
		}
		if row.item.Version != selected.ExpectedVersion || status == "archived" || status == "deleted" {
			return nil, conflict(taskPrioritySelectionChanged)
		}
		row.item.Changed = row.item.PreviousPriority != priority
		if row.item.Changed && row.item.Version == math.MaxInt32 {
			return nil, conflict(recordVersionExhausted)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// PreviewTaskPriorities returns the current authorized values without recording
// an operation. Apply will recheck the same explicit versions and eligibility.
func (s *Service) PreviewTaskPriorities(ctx context.Context, actor Actor, req TaskPriorityChangePreviewRequest) (TaskPriorityChangePreview, error) {
	var out TaskPriorityChangePreview
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if err := validateTaskPrioritySelection(req.ProjectID, req.Priority, req.Tasks); err != nil {
		return out, err
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, req.ProjectID, req.ProjectID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	rows, err := s.taskPriorityChangeRowsTx(ctx, auth, req.ProjectID, req.Priority, req.Tasks)
	if err != nil {
		return out, err
	}
	out = TaskPriorityChangePreview{ProjectID: req.ProjectID, Items: make([]TaskPriorityChangePreviewItem, 0, len(rows))}
	for _, row := range rows {
		out.Items = append(out.Items, row.item)
	}
	return out, nil
}

// ChangeTaskPriorities changes planning metadata while keeping the original
// task basis and formal evidence intact, including on an accepted task.
func (s *Service) ChangeTaskPriorities(ctx context.Context, actor Actor, req TaskPriorityChangeRequest) (TaskPriorityChangeResult, error) {
	var out TaskPriorityChangeResult
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !planningKey(req.IdempotencyKey) {
		return out, invalid(taskPriorityChangeInvalid)
	}
	if err := validateTaskPrioritySelection(req.ProjectID, req.Priority, req.Tasks); err != nil {
		return out, err
	}
	project := req.ProjectID
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, project, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()

	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_priority_changes": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}

	rows, err := s.taskPriorityChangeRowsTx(ctx, auth, project, req.Priority, req.Tasks)
	if err != nil {
		return out, err
	}
	tenant, tx := auth.target.tenantID, auth.tx
	receipts := make([]TaskPriorityChangeReceipt, 0, len(rows))
	for _, row := range rows {
		item := row.item
		receipt := TaskPriorityChangeReceipt{TaskID: item.TaskID, PreviousVersion: item.Version, Version: item.Version,
			PreviousPriority: item.PreviousPriority, Priority: item.Priority, Changed: item.Changed}
		if receipt.Changed {
			receipt.Version++
		}
		receipts = append(receipts, receipt)
	}

	at := now()
	for i, receipt := range receipts {
		if !receipt.Changed {
			continue
		}
		// The generic task editor may withdraw acceptance. Here priority is
		// the only business field being changed and is outside taskBasis.
		update, err := tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET priority=?,version=?,updated_at=?
			WHERE tenant_id=? AND project_id=? AND id=? AND version=?`),
			req.Priority, receipt.Version, at, tenant, project, receipt.TaskID, receipt.PreviousVersion)
		if err != nil {
			return out, err
		}
		count, err := update.RowsAffected()
		if err != nil {
			return out, err
		}
		if count != 1 {
			return out, conflict(taskPrioritySelectionChanged)
		}
		if err = s.commandActivity(ctx, auth, "tasks.updated", receipt.TaskID, rows[i].title); err != nil {
			return out, err
		}
	}

	out = TaskPriorityChangeResult{ProjectID: project, OperationID: req.IdempotencyKey, Changes: receipts}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return TaskPriorityChangeResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return TaskPriorityChangeResult{}, err
	}
	return out, nil
}

func (s *Server) taskPriorityChangesHTTP(w http.ResponseWriter, r *http.Request, actor Actor) {
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
		writeError(w, invalid(taskPriorityChangeInvalid))
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, taskPriorityChangeBodyLimit+1))
	if err != nil {
		writeError(w, invalid(taskPriorityChangeInvalid))
		return
	}
	if r.URL.Path == taskPriorityPreviewAPIPath {
		var req TaskPriorityChangePreviewRequest
		if strictjson.Decode(raw, &req, taskPriorityChangeBodyLimit) != nil {
			writeError(w, invalid(taskPriorityChangeInvalid))
			return
		}
		out, err := s.Service.PreviewTaskPriorities(r.Context(), actor, req)
		respond(w, http.StatusOK, out, err)
		return
	}
	var req TaskPriorityChangeRequest
	if strictjson.Decode(raw, &req, taskPriorityChangeBodyLimit) != nil {
		writeError(w, invalid(taskPriorityChangeInvalid))
		return
	}
	out, err := s.Service.ChangeTaskPriorities(r.Context(), actor, req)
	respond(w, http.StatusOK, out, err)
}
