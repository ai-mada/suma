package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

func (s *Service) plansForTx(ctx context.Context, auth *authorizedCommand, project string, tasks []Record) ([]PlannedTask, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	out := make([]PlannedTask, len(tasks))
	if len(tasks) == 0 {
		return out, nil
	}
	args := []any{p.TenantID, project}
	marks := []string{}
	positions := map[string]int{}
	for i, task := range tasks {
		out[i] = PlannedTask{Record: task}
		positions[task.ID] = i
		marks = append(marks, "?")
		args = append(args, task.ID)
	}
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT task_id,version,iteration_id,milestone_id,start_date,due_date,estimated_hours FROM suma_task_plans WHERE tenant_id=? AND project_id=? AND task_id IN (`+strings.Join(marks, ",")+`)`), args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var plan TaskPlan
		var hours sql.NullFloat64
		if e = rows.Scan(&id, &plan.Version, &plan.IterationID, &plan.MilestoneID, &plan.StartDate, &plan.DueDate, &hours); e != nil {
			return nil, e
		}
		if hours.Valid {
			plan.EstimatedHours = &hours.Float64
		}
		if i, ok := positions[id]; ok {
			out[i].Plan = plan
		}
	}
	return out, rows.Err()
}

func (s *Service) planningTx(ctx context.Context, auth *authorizedCommand, project string, limit int, cursor, week, taskID string) (map[string]any, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	if limit <= 0 {
		limit = 200
	}
	tasks, total, next, e := s.listPageTx(ctx, auth, project, "tasks", limit, cursor)
	if e != nil {
		return nil, e
	}
	if taskID != "" {
		task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
		if err != nil {
			return nil, err
		}
		tasks = []Record{task}
		next = ""
	}
	planned, e := s.plansForTx(ctx, auth, project, tasks)
	if e != nil {
		return nil, e
	}
	iterations, e := s.planningPeriodsTx(ctx, auth, project, "iterations")
	if e != nil {
		return nil, e
	}
	milestones, e := s.planningPeriodsTx(ctx, auth, project, "milestones")
	if e != nil {
		return nil, e
	}
	baselines, e := s.baselinesTx(ctx, auth, project)
	if e != nil {
		return nil, e
	}
	capacity, e := s.planningCapacityTx(ctx, auth, project, week)
	if e != nil {
		return nil, e
	}
	var taskCount, accepted, blocked, scheduled, estimated, overdue int
	var hours float64
	today := time.Now().UTC().Format("2006-01-02")
	e = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*),COALESCE(SUM(CASE WHEN t.status='accepted' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN t.status='blocked' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN p.start_date<>'' OR p.due_date<>'' THEN 1 ELSE 0 END),0),COALESCE(SUM(CASE WHEN p.estimated_hours IS NOT NULL THEN 1 ELSE 0 END),0),COALESCE(SUM(p.estimated_hours),0),COALESCE(SUM(CASE WHEN p.due_date<>'' AND p.due_date<? AND t.status<>'accepted' THEN 1 ELSE 0 END),0) FROM suma_tasks t LEFT JOIN suma_task_plans p ON p.task_id=t.id AND p.tenant_id=t.tenant_id AND p.project_id=t.project_id WHERE t.tenant_id=? AND t.project_id=?`), today, p.TenantID, project).Scan(&taskCount, &accepted, &blocked, &scheduled, &estimated, &hours, &overdue)
	if e != nil {
		return nil, e
	}
	var percent *float64
	if taskCount > 0 {
		value := 100 * float64(accepted) / float64(taskCount)
		percent = &value
	}
	return map[string]any{"iterations": iterations, "milestones": milestones, "baselines": baselines, "tasks": planned, "capacity": capacity, "total": total, "next_cursor": next, "coverage": map[string]any{"loaded_tasks": len(tasks), "total_tasks": total, "task_metrics": "whole_project", "timeline": "loaded_tasks"}, "metrics": map[string]any{"total_tasks": taskCount, "accepted_tasks": accepted, "blocked_tasks": blocked, "scheduled_tasks": scheduled, "estimated_tasks": estimated, "estimated_hours": hours, "overdue_tasks": overdue, "acceptance_percent": percent}, "as_of": now()}, nil
}

func weekMonday(value string) (time.Time, error) {
	if value == "" {
		now := time.Now().UTC()
		days := (int(now.Weekday()) + 6) % 7
		value = now.AddDate(0, 0, -days).Format("2006-01-02")
	}
	date, e := time.Parse("2006-01-02", value)
	if e != nil || date.Weekday() != time.Monday {
		return time.Time{}, invalid("容量周必须是有效的周一日期")
	}
	return date, nil
}
func plannedWeekShare(start, due string, week time.Time) (float64, bool) {
	if start == "" && due == "" {
		return 0, false
	}
	if start == "" {
		start = due
	}
	if due == "" {
		due = start
	}
	a, e := time.Parse("2006-01-02", start)
	if e != nil {
		return 0, false
	}
	b, e := time.Parse("2006-01-02", due)
	if e != nil || b.Before(a) {
		return 0, false
	}
	days := int(b.Sub(a).Hours()/24) + 1
	first := a
	if week.After(first) {
		first = week
	}
	last := b
	end := week.AddDate(0, 0, 6)
	if end.Before(last) {
		last = end
	}
	if last.Before(first) {
		return 0, true
	}
	return float64(int(last.Sub(first).Hours()/24)+1) / float64(days), true
}
func (s *Service) planningCapacityTx(ctx context.Context, auth *authorizedCommand, project, weekValue string) ([]CapacityEntry, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	week, e := weekMonday(weekValue)
	if e != nil {
		return nil, e
	}
	value := week.Format("2006-01-02")
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT m.id,m.title,m.kind,m.data,COALESCE(c.version,0),c.available_hours FROM suma_members m LEFT JOIN suma_capacity c ON c.member_id=m.id AND c.tenant_id=m.tenant_id AND c.project_id=m.project_id AND c.week_start=? WHERE m.tenant_id=? AND m.project_id=? AND m.status<>'archived' ORDER BY m.title,m.id`), value, p.TenantID, project)
	if e != nil {
		return nil, e
	}
	items := []CapacityEntry{}
	positions := map[string]int{}
	type candidate struct {
		entry CapacityEntry
		user  string
	}
	candidates := []candidate{}
	hasHumans := false
	for rows.Next() {
		var item CapacityEntry
		var h sql.NullFloat64
		var metadata string
		if e = rows.Scan(&item.MemberID, &item.Name, &item.Kind, &metadata, &item.Version, &h); e != nil {
			rows.Close()
			return nil, e
		}
		if item.Kind == "" {
			item.Kind = "human"
		}
		if h.Valid {
			item.AvailableHours = &h.Float64
		}
		item.WeekStart = value
		row := candidate{entry: item}
		if item.Kind == "human" {
			hasHumans = true
			var data map[string]any
			if e = json.Unmarshal([]byte(metadata), &data); e != nil {
				rows.Close()
				return nil, e
			}
			row.user, _ = data["user_id"].(string)
		}
		candidates = append(candidates, row)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	// Retain historical resources and capacity rows, but only count humans who
	// still have their real organization membership and current project access.
	allowedHumans := map[string]bool{}
	if hasHumans {
		rows, e = auth.tx.QueryContext(ctx, s.q(`SELECT om.user_id FROM suma_org_members om JOIN suma_users u ON u.id=om.user_id JOIN suma_projects p ON p.tenant_id=om.tenant_id AND p.id=? LEFT JOIN suma_project_members pm ON pm.project_id=p.id AND pm.user_id=om.user_id WHERE om.tenant_id=? AND (om.role IN ('owner','admin') OR pm.role IN ('admin','reviewer','developer','viewer'))`), project, p.TenantID)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var user string
			if e = rows.Scan(&user); e != nil {
				rows.Close()
				return nil, e
			}
			allowedHumans[user] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	for _, row := range candidates {
		if row.entry.Kind == "human" && !allowedHumans[row.user] {
			continue
		}
		positions[row.entry.MemberID] = len(items)
		items = append(items, row.entry)
	}
	rows, e = auth.tx.QueryContext(ctx, s.q(`SELECT t.owner_id,COALESCE(p.start_date,''),COALESCE(p.due_date,''),p.estimated_hours FROM suma_tasks t LEFT JOIN suma_task_plans p ON p.tenant_id=t.tenant_id AND p.project_id=t.project_id AND p.task_id=t.id WHERE t.tenant_id=? AND t.project_id=? AND t.owner_id<>''`), p.TenantID, project)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var owner, start, due string
		var h sql.NullFloat64
		if e = rows.Scan(&owner, &start, &due, &h); e != nil {
			return nil, e
		}
		i, ok := positions[owner]
		if !ok {
			continue
		}
		share, scheduled := plannedWeekShare(start, due, week)
		if scheduled && share == 0 {
			continue
		}
		if !h.Valid || !scheduled {
			items[i].UnestimatedTasks++
		} else {
			items[i].AllocatedHours += h.Float64 * share
		}
	}
	return items, rows.Err()
}
func (s *Service) SaveCapacity(ctx context.Context, actor Actor, project string, m map[string]any) (CapacityEntry, error) {
	if e := exactVersion(m, "expected_version", 0); e != nil {
		return CapacityEntry{}, e
	}
	if str(m, "week_start") == "" {
		return CapacityEntry{}, invalid("请明确指定容量周")
	}
	if _, exists := m["available_hours"]; !exists {
		return CapacityEntry{}, invalid("请明确指定available_hours；未知时使用null")
	}
	week, e := weekMonday(str(m, "week_start"))
	if e != nil {
		return CapacityEntry{}, e
	}
	h, e := finiteHours(m["available_hours"], 168)
	if e != nil {
		return CapacityEntry{}, e
	}
	ctx, tx, cancel, e := s.beginAuthorityTx(ctx)
	if e != nil {
		return CapacityEntry{}, e
	}
	defer cancel()
	defer tx.Rollback()
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: str(m, "member_id")}, purposeProjectWrite)
	if e != nil {
		return CapacityEntry{}, e
	}
	p := auth.currentPrincipal
	key := str(m, "idempotency_key")
	hash, replay, e := s.commandReceipt(ctx, auth, key, map[string]any{"capacity": m})
	if e != nil {
		return CapacityEntry{}, e
	}
	if replay {
		var out CapacityEntry
		e = json.Unmarshal([]byte(hash), &out)
		return out, e
	}
	item := CapacityEntry{MemberID: str(m, "member_id"), WeekStart: week.Format("2006-01-02"), AvailableHours: h}
	var metadata string
	if e = tx.QueryRowContext(ctx, s.q(`SELECT title,kind,data FROM suma_members WHERE tenant_id=? AND project_id=? AND id=? AND status<>'archived'`), p.TenantID, project, item.MemberID).Scan(&item.Name, &item.Kind, &metadata); e != nil {
		return item, missing()
	}
	if item.Kind == "human" || item.Kind == "" {
		var data map[string]any
		if e = json.Unmarshal([]byte(metadata), &data); e != nil {
			return item, e
		}
		user, _ := data["user_id"].(string)
		if e = s.BindHumanResourceTx(ctx, tx, auth, project, user); e != nil {
			return item, e
		}
	}
	var old int
	e = tx.QueryRowContext(ctx, s.q(`SELECT version FROM suma_capacity WHERE tenant_id=? AND project_id=? AND member_id=? AND week_start=?`), p.TenantID, project, item.MemberID, item.WeekStart).Scan(&old)
	if e != nil && e != sql.ErrNoRows {
		return item, e
	}
	if old != number(m, "expected_version") {
		return item, conflict("容量已变化，请刷新后重新调整")
	}
	item.Version = old + 1
	if old == 0 {
		_, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_capacity(tenant_id,project_id,member_id,week_start,version,available_hours,updated_at)VALUES(?,?,?,?,?,?,?)`), p.TenantID, project, item.MemberID, item.WeekStart, item.Version, h, now())
	} else {
		_, e = tx.ExecContext(ctx, s.q(`UPDATE suma_capacity SET version=?,available_hours=?,updated_at=? WHERE tenant_id=? AND project_id=? AND member_id=? AND week_start=? AND version=?`), item.Version, h, now(), p.TenantID, project, item.MemberID, item.WeekStart, old)
	}
	if e != nil {
		return item, e
	}
	if e = s.commandActivity(ctx, auth, "capacity.updated", item.MemberID, item.Name); e != nil {
		return item, e
	}
	if e = s.saveCommandReceipt(ctx, auth, key, hash, item); e != nil {
		return item, e
	}
	return item, tx.Commit()
}
