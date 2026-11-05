package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	"github.com/ai-mada/suma/internal/strictjson"
)

const taskIterationAssignmentInvalid = "Invalid task iteration assignment request"

type TaskIterationAssignmentSelection struct {
	TaskID              string `json:"task_id"`
	ExpectedTaskVersion int    `json:"expected_task_version"`
	ExpectedPlanVersion int    `json:"expected_plan_version"`
}

type TaskIterationAssignmentRequest struct {
	ProjectID                string                             `json:"project_id"`
	IterationID              string                             `json:"iteration_id"`
	ExpectedIterationVersion int                                `json:"expected_iteration_version"`
	Tasks                    []TaskIterationAssignmentSelection `json:"tasks"`
	IdempotencyKey           string                             `json:"idempotency_key"`
}

type TaskIterationAssignmentReceipt struct {
	TaskID      string `json:"task_id"`
	PlanVersion int    `json:"plan_version"`
	Changed     bool   `json:"changed"`
}

type TaskIterationAssignmentResult struct {
	ProjectID   string                           `json:"project_id"`
	OperationID string                           `json:"operation_id"`
	IterationID string                           `json:"iteration_id"`
	Changes     []TaskIterationAssignmentReceipt `json:"changes"`
}

func decodeTaskIterationAssignment(raw []byte) (TaskIterationAssignmentRequest, error) {
	var req TaskIterationAssignmentRequest
	if err := strictjson.Decode(raw, &req, taskIterationAssignmentBodyLimit); err != nil {
		return TaskIterationAssignmentRequest{}, invalid(taskIterationAssignmentInvalid)
	}
	return req, nil
}

func validateTaskIterationAssignment(project string, req TaskIterationAssignmentRequest) error {
	if project != req.ProjectID || !handoffID(project, false) || !handoffID(req.IterationID, true) ||
		(req.IterationID == "") != (req.ExpectedIterationVersion == 0) || req.ExpectedIterationVersion < 0 || req.ExpectedIterationVersion > math.MaxInt32 ||
		!planningKey(req.IdempotencyKey) || len(req.Tasks) < 1 || len(req.Tasks) > 16 {
		return invalid(taskIterationAssignmentInvalid)
	}
	seen := make(map[string]bool, len(req.Tasks))
	for _, task := range req.Tasks {
		if !handoffID(task.TaskID, false) || seen[task.TaskID] || task.ExpectedTaskVersion < 1 || task.ExpectedTaskVersion > math.MaxInt32 ||
			task.ExpectedPlanVersion < 0 || task.ExpectedPlanVersion > math.MaxInt32 {
			return invalid(taskIterationAssignmentInvalid)
		}
		seen[task.TaskID] = true
	}
	return nil
}

// AssignTaskIterations changes membership through the same schedule writer used
// by the full planning form. Task and plan versions protect the review basis,
// while the writer keeps every other planning field from the current plan.
func (s *Service) AssignTaskIterations(ctx context.Context, actor Actor, project string, req TaskIterationAssignmentRequest) (TaskIterationAssignmentResult, error) {
	var out TaskIterationAssignmentResult
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if err := validateTaskIterationAssignment(project, req); err != nil {
		return out, err
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, project, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()

	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_iteration_assignment": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	tenant, tx := auth.target.tenantID, auth.tx
	if req.IterationID != "" {
		var version int
		var status string
		err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT version,status FROM suma_planning_periods WHERE tenant_id=? AND project_id=? AND kind='iterations' AND id=?`)), tenant, project, req.IterationID).Scan(&version, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return out, conflict("目标迭代不可用，请重新选择")
		}
		if err != nil {
			return out, err
		}
		if version != req.ExpectedIterationVersion || (status != "planned" && status != "active") {
			return out, conflict("目标迭代已变化或已完成，请刷新后重新选择")
		}
	}

	tasks := make([]Record, 0, len(req.Tasks))
	for _, selection := range req.Tasks {
		task := Record{ID: selection.TaskID}
		err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT version FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, task.ID).Scan(&task.Version)
		if errors.Is(err, sql.ErrNoRows) {
			return out, conflict("所选任务已变化，请刷新任务列表")
		}
		if err != nil {
			return out, err
		}
		if task.Version != selection.ExpectedTaskVersion {
			return out, conflict("所选任务已变化，请刷新任务列表")
		}
		tasks = append(tasks, task)
	}
	planned, err := s.plansForTx(ctx, auth, project, tasks)
	if err != nil {
		return out, err
	}

	updates := make([]any, 0, len(planned))
	receipts := make([]TaskIterationAssignmentReceipt, 0, len(planned))
	for i, task := range planned {
		plan := task.Plan
		if plan.Version != req.Tasks[i].ExpectedPlanVersion {
			return out, conflict("任务排期已变化，请保留草稿后刷新")
		}
		receipt := TaskIterationAssignmentReceipt{TaskID: task.ID, PlanVersion: plan.Version, Changed: plan.IterationID != req.IterationID}
		if receipt.Changed {
			if plan.Version == math.MaxInt32 {
				return out, conflict("任务排期版本已达到上限")
			}
			var hours any
			if plan.EstimatedHours != nil {
				hours = *plan.EstimatedHours
			}
			updates = append(updates, map[string]any{
				"task_id": task.ID, "expected_task_version": task.Version, "expected_version": plan.Version,
				"iteration_id": req.IterationID, "milestone_id": plan.MilestoneID, "start_date": plan.StartDate, "due_date": plan.DueDate,
				"estimated_hours": hours,
			})
			receipt.PlanVersion++
		}
		receipts = append(receipts, receipt)
	}
	if len(updates) > 0 {
		if _, err = s.scheduleTasksTx(ctx, tx, auth, project, updates); err != nil {
			return out, err
		}
	}
	out = TaskIterationAssignmentResult{ProjectID: project, OperationID: req.IdempotencyKey, IterationID: req.IterationID, Changes: receipts}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return TaskIterationAssignmentResult{}, err
	}
	if err = tx.Commit(); err != nil {
		return TaskIterationAssignmentResult{}, err
	}
	return out, nil
}
