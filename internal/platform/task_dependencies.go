package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strings"
)

const taskDependencyGraphLimit = 1000
const taskDependencyChangeLimit = 100

const (
	taskDependencyInvalid          = "Invalid task dependency change request"
	taskDependencyQueryInvalid     = "Invalid task dependency graph query"
	taskDependencyDuplicateTask    = "Task dependency changes contain a duplicate task"
	taskDependencyDuplicateEdge    = "Task dependencies contain a duplicate task"
	taskDependencySelf             = "Task cannot depend on itself"
	taskDependencyReference        = "Predecessor task does not exist in the current project"
	taskDependencyCycle            = "Task dependencies contain a cycle"
	taskDependencyChanged          = "Task dependencies changed; keep the draft and reload"
	taskDependencyVersionExhausted = "Task version limit reached; review the task before changing dependencies"
	taskDependencyUnchanged        = "Task dependency changes must modify each selected task"
)

// TaskDependencyGraph is a complete business-task graph, separate from a run's
// immutable execution DAG. An oversized graph has no partial task projection.
type TaskDependencyGraph struct {
	ProjectID string               `json:"project_id"`
	Total     int                  `json:"total"`
	Limit     int                  `json:"limit"`
	Complete  bool                 `json:"complete"`
	Tasks     []TaskDependencyNode `json:"tasks"`
}

type TaskDependencyNode struct {
	TaskID    string   `json:"task_id"`
	Version   int      `json:"version"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	DependsOn []string `json:"depends_on"`
}

// TaskDependencyChangeRequest fixes each dirty task's original version. The
// whole final project graph is validated and committed in one authority transaction.
type TaskDependencyChangeRequest struct {
	ProjectID      string                 `json:"project_id"`
	Changes        []TaskDependencyChange `json:"changes"`
	IdempotencyKey string                 `json:"idempotency_key"`
}

type TaskDependencyChange struct {
	TaskID          string   `json:"task_id"`
	ExpectedVersion int      `json:"expected_version"`
	DependsOn       []string `json:"depends_on"`
}

// TaskDependencyChangeResult is the immutable original command acknowledgement,
// including on replay. Clients must read the graph again for current versions.
type TaskDependencyChangeResult struct {
	ProjectID   string                        `json:"project_id"`
	OperationID string                        `json:"operation_id"`
	Changes     []TaskDependencyChangeReceipt `json:"changes"`
}

type TaskDependencyChangeReceipt struct {
	TaskID          string   `json:"task_id"`
	PreviousVersion int      `json:"previous_version"`
	Version         int      `json:"version"`
	DependsOn       []string `json:"depends_on"`
}

type taskDependencyConflict struct {
	TaskID          string   `json:"task_id"`
	ExpectedVersion int      `json:"expected_version"`
	CurrentVersion  int      `json:"current_version"`
	DependsOn       []string `json:"depends_on"`
}

func taskDependencyStrings(values []string) []string {
	return append([]string{}, values...)
}

// TaskDependencies reads count and nodes from the same authorized transaction.
// The explicit completeness flag prevents a paginated list becoming a false DAG.
func (s *Service) TaskDependencies(ctx context.Context, actor Actor, project string) (out TaskDependencyGraph, err error) {
	out = TaskDependencyGraph{ProjectID: project, Limit: taskDependencyGraphLimit, Tasks: []TaskDependencyNode{}}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		readCtx := auth.ctx
		out.Tasks = []TaskDependencyNode{}
		if e := assertProjectRead(auth, project); e != nil {
			return e
		}
		if e := auth.tx.QueryRowContext(readCtx, s.q(`SELECT COUNT(*) FROM suma_tasks WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&out.Total); e != nil {
			return e
		}
		out.Complete = out.Total <= taskDependencyGraphLimit
		if !out.Complete {
			return nil
		}
		rows, e := auth.tx.QueryContext(readCtx, s.q(`SELECT id,version,title,status,depends_on FROM suma_tasks WHERE tenant_id=? AND project_id=? ORDER BY id`), auth.target.tenantID, project)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var item TaskDependencyNode
			var deps string
			if e = rows.Scan(&item.TaskID, &item.Version, &item.Title, &item.Status, &deps); e != nil {
				return e
			}
			if e = json.Unmarshal([]byte(deps), &item.DependsOn); e != nil {
				return e
			}
			item.DependsOn = taskDependencyStrings(item.DependsOn)
			out.Tasks = append(out.Tasks, item)
		}
		return rows.Err()
	})
	return out, err
}

func validateTaskDependencyChanges(req TaskDependencyChangeRequest) error {
	if req.ProjectID == "" || req.Changes == nil || len(req.Changes) < 1 || len(req.Changes) > taskDependencyChangeLimit || len(req.IdempotencyKey) < 8 || len(req.IdempotencyKey) > 128 {
		return invalid(taskDependencyInvalid)
	}
	seen := map[string]bool{}
	for _, change := range req.Changes {
		if change.TaskID == "" || strings.TrimSpace(change.TaskID) != change.TaskID || change.ExpectedVersion < 1 || change.ExpectedVersion > math.MaxInt32 || change.DependsOn == nil {
			return invalid(taskDependencyInvalid)
		}
		if seen[change.TaskID] {
			return invalid(taskDependencyDuplicateTask)
		}
		seen[change.TaskID] = true
		deps := map[string]bool{}
		for _, id := range change.DependsOn {
			if id == "" || strings.TrimSpace(id) != id {
				return invalid(taskDependencyInvalid)
			}
			if id == change.TaskID {
				return invalid(taskDependencySelf)
			}
			if deps[id] {
				return invalid(taskDependencyDuplicateEdge)
			}
			deps[id] = true
		}
	}
	return nil
}

func taskDependencySameSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	set := map[string]bool{}
	for _, id := range left {
		set[id] = true
	}
	for _, id := range right {
		if !set[id] {
			return false
		}
	}
	return true
}

// Validate once after all requested edits. Validating intermediate records would
// reject legal atomic rewiring when an intermediate graph temporarily has a cycle.
// Kahn's algorithm avoids recursion for long chains in large projects.
func validateTaskDependencyGraph(graph map[string][]string) error {
	remaining := make(map[string]int, len(graph))
	children := make(map[string][]string, len(graph))
	queue := make([]string, 0, len(graph))
	for id, dependencies := range graph {
		remaining[id] = len(dependencies)
		for _, predecessor := range dependencies {
			if _, exists := graph[predecessor]; !exists {
				return invalid(taskDependencyReference)
			}
			if predecessor == id {
				return invalid(taskDependencySelf)
			}
			children[predecessor] = append(children[predecessor], id)
		}
		if len(dependencies) == 0 {
			queue = append(queue, id)
		}
	}
	for pos := 0; pos < len(queue); pos++ {
		for _, id := range children[queue[pos]] {
			remaining[id]--
			if remaining[id] == 0 {
				queue = append(queue, id)
			}
		}
	}
	if len(queue) != len(graph) {
		return invalid(taskDependencyCycle)
	}
	return nil
}

func (s *Service) ChangeTaskDependencies(ctx context.Context, actor Actor, project string, req TaskDependencyChangeRequest) (out TaskDependencyChangeResult, err error) {
	if project == "" || req.ProjectID != project {
		return out, invalid(taskDependencyInvalid)
	}
	ctx, auth, cleanup, err := s.planningAuthority(ctx, actor, project, project, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer cleanup()
	// Authorization and project coordination precede even a receipt replay. A
	// replay does not re-evaluate versions that legitimately changed afterward.
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"task_dependencies": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if err = validateTaskDependencyChanges(req); err != nil {
		return out, err
	}
	graph := map[string][]string{}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id,depends_on FROM suma_tasks WHERE tenant_id=? AND project_id=? ORDER BY id`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, raw string
		if err = rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return out, err
		}
		var deps []string
		if err = json.Unmarshal([]byte(raw), &deps); err != nil {
			rows.Close()
			return out, err
		}
		graph[id] = deps
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	tasks := make([]Record, 0, len(req.Changes))
	conflicts := []taskDependencyConflict{}
	for _, change := range req.Changes {
		task, e := s.getRecordTx(ctx, auth, project, "tasks", change.TaskID)
		if e != nil {
			return out, e
		}
		if task.Version != change.ExpectedVersion {
			conflicts = append(conflicts, taskDependencyConflict{TaskID: task.ID, ExpectedVersion: change.ExpectedVersion, CurrentVersion: task.Version, DependsOn: taskDependencyStrings(task.DependsOn)})
		}
		tasks = append(tasks, task)
	}
	if len(conflicts) > 0 {
		return out, &APIError{http.StatusConflict, "version_conflict", taskDependencyChanged, map[string]any{"project_id": project, "conflicts": conflicts}}
	}
	for i, change := range req.Changes {
		if tasks[i].Version == math.MaxInt32 {
			return out, conflict(taskDependencyVersionExhausted)
		}
		if taskDependencySameSet(tasks[i].DependsOn, change.DependsOn) {
			return out, invalid(taskDependencyUnchanged)
		}
		graph[change.TaskID] = change.DependsOn
	}
	if err = validateTaskDependencyGraph(graph); err != nil {
		return out, err
	}
	out = TaskDependencyChangeResult{ProjectID: project, OperationID: req.IdempotencyKey, Changes: []TaskDependencyChangeReceipt{}}
	roots := make([]string, 0, len(tasks))
	for i, task := range tasks {
		before := task.Version
		oldBasis := taskBasis(task)
		task.DependsOn = taskDependencyStrings(req.Changes[i].DependsOn)
		if taskBasis(task) != oldBasis {
			task.Status = "in_progress"
			delete(task.Data, "verified_run_id")
		}
		task.Version++
		task.UpdatedAt = now()
		// Only dependency-related columns change. Other task fields and the run's
		// original input snapshot remain intact, including during active execution.
		res, e := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_tasks SET version=?,depends_on=?,status=?,updated_at=?,data=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), task.Version, stringify(task.DependsOn), task.Status, task.UpdatedAt, stringify(task.Data), auth.target.tenantID, project, task.ID, before)
		if e != nil {
			return out, e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return out, e
		}
		if n != 1 {
			return out, conflict(taskDependencyChanged)
		}
		if e = s.commandActivity(ctx, auth, "tasks.dependencies_updated", task.ID, task.Title); e != nil {
			return out, e
		}
		roots = append(roots, task.ID)
		out.Changes = append(out.Changes, TaskDependencyChangeReceipt{TaskID: task.ID, PreviousVersion: before, Version: task.Version, DependsOn: taskDependencyStrings(task.DependsOn)})
	}
	if err = s.InvalidateDependentsTx(ctx, auth.tx, auth, project, roots, false); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func taskDependencyQuery(r *http.Request, project string) error {
	if len(r.URL.RawQuery) > 8192 {
		return invalid(taskDependencyQueryInvalid)
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return invalid(taskDependencyQueryInvalid)
	}
	for key, items := range values {
		if key != "project_id" || len(items) != 1 || items[0] == "" || items[0] != project {
			return invalid(taskDependencyQueryInvalid)
		}
	}
	if r.Method == http.MethodGet && values.Get("project_id") == "" {
		return invalid(taskDependencyQueryInvalid)
	}
	return nil
}

// Closed decoding validates nested field presence before DTO decoding, so null
// versions cannot quietly become zero and a missing depends_on cannot mean clear.
func decodeTaskDependencyChanges(m map[string]any) (TaskDependencyChangeRequest, error) {
	var out TaskDependencyChangeRequest
	if len(m) != 3 || m["project_id"] == nil || m["changes"] == nil || m["idempotency_key"] == nil {
		return out, invalid(taskDependencyInvalid)
	}
	items, ok := m["changes"].([]any)
	if !ok || len(items) < 1 || len(items) > taskDependencyChangeLimit {
		return out, invalid(taskDependencyInvalid)
	}
	for _, item := range items {
		change, ok := item.(map[string]any)
		if !ok || len(change) != 3 || change["task_id"] == nil || change["expected_version"] == nil || change["depends_on"] == nil {
			return out, invalid(taskDependencyInvalid)
		}
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return out, invalid(taskDependencyInvalid)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&out); err != nil {
		return out, invalid(taskDependencyInvalid)
	}
	return out, nil
}

func (s *Server) taskDependenciesHTTP(w http.ResponseWriter, r *http.Request, actor Actor, scope, project string, parts []string, m map[string]any) bool {
	if len(parts) == 0 || parts[0] != "task-dependencies" {
		return false
	}
	if scope != "" && scope != "api" {
		writeError(w, forbidden())
		return true
	}
	if len(parts) != 1 {
		writeError(w, missing())
		return true
	}
	if err := taskDependencyQuery(r, project); err != nil {
		writeError(w, err)
		return true
	}
	switch r.Method {
	case http.MethodGet:
		out, err := s.Service.TaskDependencies(r.Context(), actor, project)
		respond(w, http.StatusOK, out, err)
	case http.MethodPost:
		req, err := decodeTaskDependencyChanges(m)
		if err != nil {
			writeError(w, err)
			return true
		}
		out, err := s.Service.ChangeTaskDependencies(r.Context(), actor, project, req)
		respond(w, http.StatusOK, out, err)
	default:
		writeError(w, missing())
	}
	return true
}
