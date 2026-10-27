package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"strings"
	"time"
)

type PlanningPeriod struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	Version     int    `json:"version"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	TargetDate  string `json:"target_date"`
	Goal        string `json:"goal"`
	UpdatedAt   string `json:"updated_at"`
}
type TaskPlan struct {
	Version        int      `json:"version"`
	IterationID    string   `json:"iteration_id"`
	MilestoneID    string   `json:"milestone_id"`
	StartDate      string   `json:"start_date"`
	DueDate        string   `json:"due_date"`
	EstimatedHours *float64 `json:"estimated_hours"`
}
type PlannedTask struct {
	Record
	Plan TaskPlan `json:"plan"`
}
type CapacityEntry struct {
	MemberID         string   `json:"member_id"`
	Name             string   `json:"name"`
	Kind             string   `json:"kind"`
	WeekStart        string   `json:"week_start"`
	Version          int      `json:"version"`
	AvailableHours   *float64 `json:"available_hours"`
	AllocatedHours   float64  `json:"allocated_hours"`
	UnestimatedTasks int      `json:"unestimated_tasks"`
}

func (s *Service) InitializePlanning(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS suma_planning_periods(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,kind VARCHAR(20) NOT NULL,version INTEGER NOT NULL,title VARCHAR(500) NOT NULL,description TEXT NOT NULL,status VARCHAR(30) NOT NULL,start_date VARCHAR(10) NOT NULL,end_date VARCHAR(10) NOT NULL,target_date VARCHAR(10) NOT NULL,goal TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_task_plans(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,iteration_id VARCHAR(64) NOT NULL,milestone_id VARCHAR(64) NOT NULL,start_date VARCHAR(10) NOT NULL,due_date VARCHAR(10) NOT NULL,estimated_hours DOUBLE PRECISION,updated_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,task_id))`,
		`CREATE TABLE IF NOT EXISTS suma_capacity(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,member_id VARCHAR(64) NOT NULL,week_start VARCHAR(10) NOT NULL,version INTEGER NOT NULL,available_hours DOUBLE PRECISION,updated_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id,member_id,week_start))`,
		`CREATE TABLE IF NOT EXISTS suma_baselines(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,title VARCHAR(500) NOT NULL,created_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,task_count BIGINT NOT NULL,accepted_tasks BIGINT NOT NULL,estimated_tasks BIGINT NOT NULL,estimated_hours DOUBLE PRECISION NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_baseline_tasks(baseline_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,task_version INTEGER NOT NULL,title VARCHAR(500) NOT NULL,description TEXT NOT NULL,status VARCHAR(40) NOT NULL,owner_id VARCHAR(64) NOT NULL,requirement_id VARCHAR(64) NOT NULL,depends_on TEXT NOT NULL,acceptance_criteria TEXT NOT NULL,plan_version INTEGER NOT NULL,iteration_id VARCHAR(64) NOT NULL,milestone_id VARCHAR(64) NOT NULL,start_date VARCHAR(10) NOT NULL,due_date VARCHAR(10) NOT NULL,estimated_hours DOUBLE PRECISION,PRIMARY KEY(baseline_id,task_id))`,
	}
	for _, query := range statements {
		if _, err := s.DB.ExecContext(ctx, s.ddl(query)); err != nil {
			return err
		}
	}
	return nil
}
func validDate(value string) bool {
	if value == "" {
		return true
	}
	date, e := time.Parse("2006-01-02", value)
	return e == nil && date.Format("2006-01-02") == value
}
func finiteHours(value any, max float64) (*float64, error) {
	if value == nil {
		return nil, nil
	}
	n, ok := value.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > max {
		return nil, invalid("工时应为有效的非负数，且不能超过限制")
	}
	return &n, nil
}
func exactVersion(m map[string]any, key string, minimum int) error {
	raw, exists := m[key]
	if !exists {
		return invalidDiagnostic("validation.field_required", "缺少"+key, map[string]any{"field": key})
	}
	var v float64
	switch value := raw.(type) {
	case float64:
		v = value
	case int:
		v = float64(value)
	case int64:
		v = float64(value)
	default:
		return invalidDiagnostic("validation.field_integer", key+"必须为整数", map[string]any{"field": key})
	}
	if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v < float64(minimum) || v > math.MaxInt32 {
		return invalidDiagnostic("validation.field_version_integer", key+"必须为有效版本整数", map[string]any{"field": key})
	}
	return nil
}

func scanPeriod(row interface{ Scan(...any) error }) (PlanningPeriod, error) {
	var item PlanningPeriod
	err := row.Scan(&item.ID, &item.ProjectID, &item.Version, &item.Title, &item.Description, &item.Status, &item.StartDate, &item.EndDate, &item.TargetDate, &item.Goal, &item.UpdatedAt)
	return item, err
}

const periodColumns = `id,project_id,version,title,description,status,start_date,end_date,target_date,goal,updated_at`

func (s *Service) planningPeriodsTx(ctx context.Context, auth *authorizedCommand, project, kind string) ([]PlanningPeriod, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT `+periodColumns+` FROM suma_planning_periods WHERE tenant_id=? AND project_id=? AND kind=? ORDER BY created_at DESC,id`), p.TenantID, project, kind)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []PlanningPeriod{}
	for rows.Next() {
		item, e := scanPeriod(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (s *Service) SavePlanningPeriod(ctx context.Context, actor Actor, project, kind, id string, m map[string]any) (PlanningPeriod, error) {
	if kind != "iterations" && kind != "milestones" {
		return PlanningPeriod{}, missing()
	}
	ctx, tx, cancel, e := s.beginAuthorityTx(ctx)
	if e != nil {
		return PlanningPeriod{}, e
	}
	defer cancel()
	defer tx.Rollback()
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectWrite)
	if e != nil {
		return PlanningPeriod{}, e
	}
	p := auth.currentPrincipal
	key := str(m, "idempotency_key")
	hash, replay, e := s.commandReceipt(ctx, auth, key, map[string]any{"period_kind": kind, "id": id, "body": m})
	if e != nil {
		return PlanningPeriod{}, e
	}
	if replay {
		var out PlanningPeriod
		e = json.Unmarshal([]byte(hash), &out)
		return out, e
	}
	item := PlanningPeriod{ID: id, ProjectID: project, Version: 1, Status: "planned"}
	creating := id == ""
	if creating {
		item.ID = newID()
	} else {
		if e = exactVersion(m, "expected_version", 1); e != nil {
			return item, e
		}
		item, e = scanPeriod(tx.QueryRowContext(ctx, s.q(`SELECT `+periodColumns+` FROM suma_planning_periods WHERE tenant_id=? AND project_id=? AND kind=? AND id=?`), p.TenantID, project, kind, id))
		if e != nil {
			return item, missing()
		}
		if item.Version != number(m, "expected_version") {
			return item, conflict("计划已变化，请保留草稿后刷新")
		}
		item.Version++
	}
	for _, field := range []string{"title", "description", "status", "start_date", "end_date", "target_date", "goal"} {
		if value, exists := m[field]; exists {
			if _, ok := value.(string); !ok {
				return item, invalidDiagnostic("validation.field_string", field+"必须为字符串", map[string]any{"field": field})
			}
		}
	}
	for name, value := range m {
		v, _ := value.(string)
		switch name {
		case "title":
			item.Title = strings.TrimSpace(v)
		case "description":
			item.Description = v
		case "status":
			item.Status = v
		case "start_date":
			item.StartDate = v
		case "end_date":
			item.EndDate = v
		case "target_date":
			item.TargetDate = v
		case "goal":
			item.Goal = v
		}
	}
	if e = requireTitle(item.Title); e != nil {
		return item, e
	}
	if len(item.Description) > 200000 || len(item.Goal) > 20000 {
		return item, invalid("计划说明超出限制")
	}
	if item.Status != "planned" && item.Status != "active" && item.Status != "completed" {
		return item, invalid("无效计划状态")
	}
	if !validDate(item.StartDate) || !validDate(item.EndDate) || !validDate(item.TargetDate) {
		return item, invalid("日期必须使用有效的YYYY-MM-DD格式")
	}
	if kind == "iterations" && (item.StartDate == "" || item.EndDate == "" || item.StartDate > item.EndDate) {
		return item, invalid("迭代必须有先后正确的开始与结束日期")
	}
	if kind == "milestones" && item.TargetDate == "" {
		return item, invalid("里程碑必须指定目标日期")
	}
	if item.Status == "completed" {
		column := "iteration_id"
		if kind == "milestones" {
			column = "milestone_id"
		}
		query := `SELECT t.status FROM suma_task_plans p JOIN suma_tasks t ON t.id=p.task_id AND t.project_id=p.project_id AND t.tenant_id=p.tenant_id WHERE p.tenant_id=? AND p.project_id=? AND p.` + column + `=? ORDER BY t.id`
		if s.Dialect != "sqlite" {
			query += ` FOR UPDATE`
		}
		rows, er := tx.QueryContext(ctx, s.q(query), p.TenantID, project, item.ID)
		if er != nil {
			return item, er
		}
		unfinished := false
		for rows.Next() {
			var state string
			if er = rows.Scan(&state); er != nil {
				rows.Close()
				return item, er
			}
			if state != "accepted" {
				unfinished = true
			}
		}
		er = rows.Err()
		rows.Close()
		if er != nil {
			return item, er
		}
		if unfinished {
			return item, conflict("仍有关联任务尚未通过验收，不能完成计划")
		}
	}
	item.UpdatedAt = now()
	if creating {
		_, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_planning_periods(id,tenant_id,project_id,kind,version,title,description,status,start_date,end_date,target_date,goal,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), item.ID, p.TenantID, project, kind, item.Version, item.Title, item.Description, item.Status, item.StartDate, item.EndDate, item.TargetDate, item.Goal, item.UpdatedAt, item.UpdatedAt)
	} else {
		res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_planning_periods SET version=?,title=?,description=?,status=?,start_date=?,end_date=?,target_date=?,goal=?,updated_at=? WHERE id=? AND tenant_id=? AND project_id=? AND version=?`), item.Version, item.Title, item.Description, item.Status, item.StartDate, item.EndDate, item.TargetDate, item.Goal, item.UpdatedAt, item.ID, p.TenantID, project, item.Version-1)
		e = err
		if e == nil {
			n, _ := res.RowsAffected()
			if n != 1 {
				e = conflict("计划保存冲突")
			}
		}
	}
	if e != nil {
		return item, e
	}
	if e = s.commandActivity(ctx, auth, kind+".saved", item.ID, item.Title); e != nil {
		return item, e
	}
	if e = s.saveCommandReceipt(ctx, auth, key, hash, item); e != nil {
		return item, e
	}
	return item, tx.Commit()
}

func (s *Service) ScheduleTasks(ctx context.Context, actor Actor, project string, m map[string]any) ([]PlannedTask, error) {
	changes, ok := m["changes"].([]any)
	if !ok || len(changes) == 0 || len(changes) > 200 {
		return nil, invalid("请选择1–200条排期变更")
	}
	ctx, tx, cancel, e := s.beginAuthorityTx(ctx)
	if e != nil {
		return nil, e
	}
	defer cancel()
	defer tx.Rollback()
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeProjectWrite)
	if e != nil {
		return nil, e
	}
	key := str(m, "idempotency_key")
	hash, replay, e := s.commandReceipt(ctx, auth, key, map[string]any{"schedule": m})
	if e != nil {
		return nil, e
	}
	if replay {
		var out []PlannedTask
		e = json.Unmarshal([]byte(hash), &out)
		return out, e
	}
	items, e := s.scheduleTasksTx(ctx, tx, auth, project, changes)
	if e != nil {
		return nil, e
	}
	if e = s.saveCommandReceipt(ctx, auth, key, hash, items); e != nil {
		return nil, e
	}
	return items, tx.Commit()
}

// scheduleTasksTx shares the complete normal schedule validation and writes
// with explicit model-proposal adoption, in the caller's original transaction.
func (s *Service) scheduleTasksTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project string, changes []any) ([]PlannedTask, error) {
	if e := assertProjectCommand(tx, auth, project); e != nil {
		return nil, e
	}
	if len(changes) == 0 || len(changes) > 200 {
		return nil, invalid("请选择1–200条排期变更")
	}
	p := auth.currentPrincipal
	var e error
	items := []PlannedTask{}
	seen := map[string]bool{}
	for _, raw := range changes {
		change, ok := raw.(map[string]any)
		if !ok {
			return nil, invalid("排期格式无效")
		}
		if e = exactVersion(change, "expected_task_version", 1); e != nil {
			return nil, e
		}
		if e = exactVersion(change, "expected_version", 0); e != nil {
			return nil, e
		}
		for _, field := range []string{"iteration_id", "milestone_id", "start_date", "due_date"} {
			if _, ok := change[field].(string); !ok {
				return nil, invalidDiagnostic("validation.schedule_field_required", "排期必须包含完整字段："+field, map[string]any{"field": field})
			}
		}
		if _, ok := change["estimated_hours"]; !ok {
			return nil, invalid("排期必须包含estimated_hours，可为null")
		}
		id := str(change, "task_id")
		if seen[id] {
			return nil, invalid("批次包含重复任务")
		}
		seen[id] = true
		readTask := `SELECT ` + recordColumns + ` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`
		if s.Dialect != "sqlite" {
			readTask += ` FOR UPDATE`
		}
		task, e := scanRecord(tx.QueryRowContext(ctx, s.q(readTask), p.TenantID, project, id))
		if e != nil {
			return nil, missing()
		}
		if task.Version != number(change, "expected_task_version") {
			return nil, conflict("任务已变化，请重新核对排期")
		}
		var old int
		var previousIteration, previousMilestone string
		e = tx.QueryRowContext(ctx, s.q(`SELECT version,iteration_id,milestone_id FROM suma_task_plans WHERE tenant_id=? AND project_id=? AND task_id=?`), p.TenantID, project, id).Scan(&old, &previousIteration, &previousMilestone)
		if e != nil && e != sql.ErrNoRows {
			return nil, e
		}
		if number(change, "expected_version") != old {
			return nil, conflict("任务排期已变化，请保留草稿后刷新")
		}
		for _, previous := range []string{previousIteration, previousMilestone} {
			if previous != "" {
				var status string
				if e = tx.QueryRowContext(ctx, s.q(`SELECT status FROM suma_planning_periods WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, previous).Scan(&status); e != nil {
					return nil, e
				}
				if status == "completed" {
					return nil, conflict("请先重新打开已完成计划，再调整其任务排期")
				}
			}
		}
		hours, e := finiteHours(change["estimated_hours"], 100000)
		if e != nil {
			return nil, e
		}
		plan := TaskPlan{Version: old + 1, IterationID: str(change, "iteration_id"), MilestoneID: str(change, "milestone_id"), StartDate: str(change, "start_date"), DueDate: str(change, "due_date"), EstimatedHours: hours}
		if !validDate(plan.StartDate) || !validDate(plan.DueDate) || (plan.StartDate != "" && plan.DueDate != "" && plan.StartDate > plan.DueDate) {
			return nil, invalid("任务排期日期无效")
		}
		for kind, ref := range map[string]string{"iterations": plan.IterationID, "milestones": plan.MilestoneID} {
			if ref == "" {
				continue
			}
			var status string
			if e = tx.QueryRowContext(ctx, s.q(`SELECT status FROM suma_planning_periods WHERE tenant_id=? AND project_id=? AND kind=? AND id=?`), p.TenantID, project, kind, ref).Scan(&status); e != nil {
				return nil, invalid("关联迭代或里程碑不存在于当前项目")
			}
			if status == "completed" {
				return nil, conflict("请先重新打开计划，再调整任务范围")
			}
		}
		if old == 0 {
			_, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_plans(tenant_id,project_id,task_id,version,iteration_id,milestone_id,start_date,due_date,estimated_hours,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?)`), p.TenantID, project, id, plan.Version, plan.IterationID, plan.MilestoneID, plan.StartDate, plan.DueDate, plan.EstimatedHours, now())
		} else {
			res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_task_plans SET version=?,iteration_id=?,milestone_id=?,start_date=?,due_date=?,estimated_hours=?,updated_at=? WHERE tenant_id=? AND project_id=? AND task_id=? AND version=?`), plan.Version, plan.IterationID, plan.MilestoneID, plan.StartDate, plan.DueDate, plan.EstimatedHours, now(), p.TenantID, project, id, old)
			e = err
			if e == nil {
				n, _ := res.RowsAffected()
				if n != 1 {
					e = conflict("排期并发冲突")
				}
			}
		}
		if e != nil {
			return nil, e
		}
		items = append(items, PlannedTask{Record: task, Plan: plan})
		if e = s.commandActivity(ctx, auth, "task.scheduled", id, task.Title); e != nil {
			return nil, e
		}
	}
	return items, nil
}
