package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/ai-mada/suma/internal/planningcalendar"
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
	if e != nil || date.Year() < 1 || date.Year() > 9999 || date.Format("2006-01-02") != value || date.Weekday() != time.Monday || date.AddDate(0, 0, 6).Year() > 9999 {
		return time.Time{}, invalid("容量周必须是有效的周一日期")
	}
	return date, nil
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
	rows, e := auth.tx.QueryContext(ctx, s.q(`SELECT m.id,m.title,m.kind,m.data,COALESCE(c.version,0),c.schedule FROM suma_members m LEFT JOIN suma_resource_calendars c ON c.member_id=m.id AND c.tenant_id=m.tenant_id AND c.project_id=m.project_id WHERE m.tenant_id=? AND m.project_id=? AND m.status<>'archived' ORDER BY m.title,m.id`), p.TenantID, project)
	if e != nil {
		return nil, e
	}
	items := []CapacityEntry{}
	positions := map[string]int{}
	schedules := map[string]ResourceCalendarSchedule{}
	type candidate struct {
		entry    CapacityEntry
		user     string
		schedule ResourceCalendarSchedule
	}
	candidates := []candidate{}
	hasHumans := false
	for rows.Next() {
		var item CapacityEntry
		var payload sql.NullString
		var metadata string
		if e = rows.Scan(&item.MemberID, &item.Name, &item.Kind, &metadata, &item.CalendarVersion, &payload); e != nil {
			rows.Close()
			return nil, e
		}
		if item.Kind == "" {
			item.Kind = "human"
		}
		item.WeekStart = value
		schedule := unknownResourceCalendar(project, item.MemberID).Schedule
		if payload.Valid {
			if e = json.Unmarshal([]byte(payload.String), &schedule); e != nil {
				rows.Close()
				return nil, e
			}
		}
		item.Days, e = planningcalendar.Week(schedule, value)
		if e != nil {
			rows.Close()
			return nil, e
		}
		available, known := float64(0), true
		for _, day := range item.Days {
			if day.AvailableHours == nil {
				known = false
			} else {
				available += *day.AvailableHours
			}
		}
		if known {
			item.AvailableHours = &available
		}
		row := candidate{entry: item, schedule: schedule}
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
	// Retain resource and calendar records, but only count humans who
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
		schedules[row.entry.MemberID] = row.schedule
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
		if start == "" || due == "" {
			items[i].UnscheduledTasks++
			continue
		}
		// One-sided dates remain valid deadlines or start constraints, but do
		// not identify a full interval for distributing estimated work.
		end := week.AddDate(0, 0, 6).Format("2006-01-02")
		if due < value || start > end {
			continue
		}
		if !h.Valid {
			items[i].UnestimatedTasks++
			continue
		}
		if h.Float64 == 0 {
			continue
		}
		schedule := schedules[owner]
		total, unknown, e := planningcalendar.Weight(schedule, start, due)
		if e != nil {
			return nil, e
		}
		if unknown > 0 || total <= 0 {
			// This is the entire overlapping task estimate, not an allocated
			// weekly share; do not sum this diagnostic across calendar weeks.
			items[i].DistributionUnknownTasks++
			items[i].DistributionUnknownHours += h.Float64
			continue
		}
		first, last := start, due
		if first < value {
			first = value
		}
		if last > end {
			last = end
		}
		weight, _, e := planningcalendar.Weight(schedule, first, last)
		if e != nil {
			return nil, e
		}
		items[i].AllocatedHours += h.Float64 * weight / total
	}
	return items, rows.Err()
}
