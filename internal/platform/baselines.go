package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
)

type BaselineSummary struct {
	ID             string  `json:"id"`
	Title          string  `json:"title"`
	CreatedAt      string  `json:"created_at"`
	TaskCount      int     `json:"task_count"`
	AcceptedTasks  int     `json:"accepted_tasks"`
	EstimatedTasks int     `json:"estimated_tasks"`
	EstimatedHours float64 `json:"estimated_hours"`
}

const baselineColumns = `id,title,created_at,task_count,accepted_tasks,estimated_tasks,estimated_hours`

func scanBaseline(row interface{ Scan(...any) error }) (BaselineSummary, error) {
	var b BaselineSummary
	e := row.Scan(&b.ID, &b.Title, &b.CreatedAt, &b.TaskCount, &b.AcceptedTasks, &b.EstimatedTasks, &b.EstimatedHours)
	return b, e
}
func (s *Service) baselinesTx(ctx context.Context, auth *authorizedCommand, project string) ([]BaselineSummary, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT `+baselineColumns+` FROM suma_baselines WHERE tenant_id=? AND project_id=? ORDER BY created_at DESC,id`), p.TenantID, project)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	items := []BaselineSummary{}
	for rows.Next() {
		b, e := scanBaseline(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, b)
	}
	return items, rows.Err()
}

// CaptureBaseline copies the complete formal task scope in one INSERT SELECT,
// then derives totals from those immutable rows. It never truncates to UI pages.
func (s *Service) CaptureBaseline(ctx context.Context, actor Actor, project string, m map[string]any) (BaselineSummary, error) {
	title := str(m, "title")
	if e := requireTitle(title); e != nil {
		return BaselineSummary{}, e
	}
	ctx, tx, cancel, e := s.beginAuthorityTx(ctx)
	if e != nil {
		return BaselineSummary{}, e
	}
	defer cancel()
	defer tx.Rollback()
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeProjectWrite)
	if e != nil {
		return BaselineSummary{}, e
	}
	p := auth.currentPrincipal
	key := str(m, "idempotency_key")
	hash, replay, e := s.commandReceipt(ctx, auth, key, map[string]any{"baseline": m})
	if e != nil {
		return BaselineSummary{}, e
	}
	if replay {
		var b BaselineSummary
		e = json.Unmarshal([]byte(hash), &b)
		return b, e
	}
	b := BaselineSummary{ID: newID(), Title: title, CreatedAt: now()}
	_, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_baselines(id,tenant_id,project_id,title,created_by,created_at,task_count,accepted_tasks,estimated_tasks,estimated_hours)VALUES(?,?,?,?,?,?,0,0,0,0)`), b.ID, p.TenantID, project, b.Title, p.ID, b.CreatedAt)
	if e != nil {
		return b, e
	}
	_, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_baseline_tasks(baseline_id,task_id,task_version,title,description,status,owner_id,requirement_id,depends_on,acceptance_criteria,plan_version,iteration_id,milestone_id,start_date,due_date,estimated_hours) SELECT ?,t.id,t.version,t.title,t.description,t.status,t.owner_id,t.requirement_id,t.depends_on,t.acceptance_criteria,COALESCE(p.version,0),COALESCE(p.iteration_id,''),COALESCE(p.milestone_id,''),COALESCE(p.start_date,''),COALESCE(p.due_date,''),p.estimated_hours FROM suma_tasks t LEFT JOIN suma_task_plans p ON p.tenant_id=t.tenant_id AND p.project_id=t.project_id AND p.task_id=t.id WHERE t.tenant_id=? AND t.project_id=?`), b.ID, p.TenantID, project)
	if e != nil {
		return b, e
	}
	if e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*),COALESCE(SUM(CASE WHEN status='accepted' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN estimated_hours IS NOT NULL THEN 1 ELSE 0 END),0),COALESCE(SUM(estimated_hours),0) FROM suma_baseline_tasks WHERE baseline_id=?`), b.ID).Scan(&b.TaskCount, &b.AcceptedTasks, &b.EstimatedTasks, &b.EstimatedHours); e != nil {
		return b, e
	}
	if _, e = tx.ExecContext(ctx, s.q(`UPDATE suma_baselines SET task_count=?,accepted_tasks=?,estimated_tasks=?,estimated_hours=? WHERE id=?`), b.TaskCount, b.AcceptedTasks, b.EstimatedTasks, b.EstimatedHours, b.ID); e != nil {
		return b, e
	}
	if e = s.commandActivity(ctx, auth, "baseline.captured", b.ID, b.Title); e != nil {
		return b, e
	}
	if e = s.saveCommandReceipt(ctx, auth, key, hash, b); e != nil {
		return b, e
	}
	return b, tx.Commit()
}

type baselineCursor struct {
	Tenant   string `json:"tenant"`
	Project  string `json:"project"`
	Baseline string `json:"baseline"`
	ID       string `json:"id"`
}

func (s *Service) baselineTx(ctx context.Context, auth *authorizedCommand, project, id, cursor string, limit int) (map[string]any, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	tx := auth.tx
	if limit <= 0 {
		limit = 200
	}
	if limit > 200 {
		return nil, invalid("基线页最多200条")
	}
	c := baselineCursor{}
	if cursor != "" {
		raw, e := base64.RawURLEncoding.DecodeString(cursor)
		if e != nil || len(raw) > 4096 || json.Unmarshal(raw, &c) != nil || c.Tenant != p.TenantID || c.Project != project || c.Baseline != id || c.ID == "" {
			return nil, invalid("基线分页游标无效")
		}
	}
	b, e := scanBaseline(tx.QueryRowContext(ctx, s.q(`SELECT `+baselineColumns+` FROM suma_baselines WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id))
	if e != nil {
		return nil, missing()
	}
	args := []any{id}
	query := `SELECT task_id,task_version,title,description,status,owner_id,requirement_id,depends_on,acceptance_criteria,plan_version,iteration_id,milestone_id,start_date,due_date,estimated_hours FROM suma_baseline_tasks WHERE baseline_id=?`
	if cursor != "" {
		query += ` AND task_id>?`
		args = append(args, c.ID)
	}
	query += ` ORDER BY task_id LIMIT ?`
	args = append(args, limit+1)
	rows, e := tx.QueryContext(ctx, s.q(query), args...)
	if e != nil {
		return nil, e
	}
	tasks := []PlannedTask{}
	for rows.Next() {
		var item PlannedTask
		var deps, criteria string
		var hours sql.NullFloat64
		if e = rows.Scan(&item.ID, &item.Version, &item.Title, &item.Description, &item.Status, &item.OwnerID, &item.RequirementID, &deps, &criteria, &item.Plan.Version, &item.Plan.IterationID, &item.Plan.MilestoneID, &item.Plan.StartDate, &item.Plan.DueDate, &hours); e != nil {
			rows.Close()
			return nil, e
		}
		item.TenantID = p.TenantID
		item.ProjectID = project
		item.Data = map[string]any{}
		if e = json.Unmarshal([]byte(deps), &item.DependsOn); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal([]byte(criteria), &item.AcceptanceCriteria); e != nil {
			rows.Close()
			return nil, e
		}
		if hours.Valid {
			item.Plan.EstimatedHours = &hours.Float64
		}
		tasks = append(tasks, item)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	next := ""
	if len(tasks) > limit {
		tasks = tasks[:limit]
		last := tasks[len(tasks)-1]
		raw, _ := json.Marshal(baselineCursor{Tenant: p.TenantID, Project: project, Baseline: id, ID: last.ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	var added, removed, changed, planChanged, total, accepted int
	if e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*),COALESCE(SUM(CASE WHEN t.status='accepted' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN b.task_id IS NULL THEN 1 ELSE 0 END),0) FROM suma_tasks t LEFT JOIN suma_baseline_tasks b ON b.baseline_id=? AND b.task_id=t.id WHERE t.tenant_id=? AND t.project_id=?`), id, p.TenantID, project).Scan(&total, &accepted, &added); e != nil {
		return nil, e
	}
	if e = tx.QueryRowContext(ctx, s.q(`SELECT COALESCE(SUM(CASE WHEN t.id IS NULL THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN t.id IS NOT NULL AND (t.title<>b.title OR t.description<>b.description OR t.owner_id<>b.owner_id OR t.requirement_id<>b.requirement_id OR t.depends_on<>b.depends_on OR t.acceptance_criteria<>b.acceptance_criteria) THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN t.id IS NOT NULL AND (COALESCE(p.iteration_id,'')<>b.iteration_id OR COALESCE(p.milestone_id,'')<>b.milestone_id OR COALESCE(p.start_date,'')<>b.start_date OR COALESCE(p.due_date,'')<>b.due_date OR p.estimated_hours<>b.estimated_hours OR (p.estimated_hours IS NULL AND b.estimated_hours IS NOT NULL) OR (p.estimated_hours IS NOT NULL AND b.estimated_hours IS NULL)) THEN 1 ELSE 0 END),0) FROM suma_baseline_tasks b LEFT JOIN suma_tasks t ON t.id=b.task_id AND t.tenant_id=? AND t.project_id=? LEFT JOIN suma_task_plans p ON p.task_id=t.id AND p.tenant_id=t.tenant_id AND p.project_id=t.project_id WHERE b.baseline_id=?`), p.TenantID, project, id).Scan(&removed, &changed, &planChanged); e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(b)
	var out map[string]any
	json.Unmarshal(raw, &out)
	out["snapshot"] = map[string]any{"tasks": tasks, "total": b.TaskCount, "next_cursor": next}
	out["comparison"] = map[string]any{"added_tasks": added, "removed_tasks": removed, "definition_changed_tasks": changed, "plan_changed_tasks": planChanged, "accepted_tasks": accepted, "total_tasks": total}
	out["as_of"] = now()
	return out, nil
}
