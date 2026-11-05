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

func validateTaskPriorityChange(req TaskPriorityChangeRequest) error {
	switch req.Priority {
	case "urgent", "high", "medium", "low":
	default:
		return invalid(taskPriorityChangeInvalid)
	}
	if !handoffID(req.ProjectID, false) || !planningKey(req.IdempotencyKey) || len(req.Tasks) < 1 || len(req.Tasks) > 16 {
		return invalid(taskPriorityChangeInvalid)
	}
	seen := make(map[string]bool, len(req.Tasks))
	for _, task := range req.Tasks {
		if !handoffID(task.TaskID, false) || seen[task.TaskID] || task.ExpectedVersion < 1 || task.ExpectedVersion > math.MaxInt32 {
			return invalid(taskPriorityChangeInvalid)
		}
		seen[task.TaskID] = true
	}
	return nil
}

// ChangeTaskPriorities changes planning metadata while keeping the original
// task basis and formal evidence intact, including on an accepted task.
func (s *Service) ChangeTaskPriorities(ctx context.Context, actor Actor, req TaskPriorityChangeRequest) (TaskPriorityChangeResult, error) {
	var out TaskPriorityChangeResult
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	if err := validateTaskPriorityChange(req); err != nil {
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

	tenant, tx := auth.target.tenantID, auth.tx
	receipts := make([]TaskPriorityChangeReceipt, 0, len(req.Tasks))
	titles := make([]string, 0, len(req.Tasks))
	for _, selected := range req.Tasks {
		receipt := TaskPriorityChangeReceipt{TaskID: selected.TaskID, Priority: req.Priority}
		var status, title string
		err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT version,priority,status,title FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`)),
			tenant, project, selected.TaskID).Scan(&receipt.PreviousVersion, &receipt.PreviousPriority, &status, &title)
		if errors.Is(err, sql.ErrNoRows) {
			return out, conflict(taskPrioritySelectionChanged)
		}
		if err != nil {
			return out, err
		}
		if receipt.PreviousVersion != selected.ExpectedVersion || status == "archived" || status == "deleted" {
			return out, conflict(taskPrioritySelectionChanged)
		}
		receipt.Version = receipt.PreviousVersion
		receipt.Changed = receipt.PreviousPriority != req.Priority
		if receipt.Changed {
			if receipt.Version == math.MaxInt32 {
				return out, conflict(recordVersionExhausted)
			}
			receipt.Version++
		}
		receipts = append(receipts, receipt)
		titles = append(titles, title)
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
		if err = s.commandActivity(ctx, auth, "tasks.updated", receipt.TaskID, titles[i]); err != nil {
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
	if !actor.IsBrowser() {
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
	var req TaskPriorityChangeRequest
	if err != nil || strictjson.Decode(raw, &req, taskPriorityChangeBodyLimit) != nil {
		writeError(w, invalid(taskPriorityChangeInvalid))
		return
	}
	out, err := s.Service.ChangeTaskPriorities(r.Context(), actor, req)
	respond(w, http.StatusOK, out, err)
}
