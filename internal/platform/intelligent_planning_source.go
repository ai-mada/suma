package platform

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

func planningTaskIDs(source PlanningSource) []string {
	ids := make([]string, 0, len(source.Tasks))
	for _, task := range source.Tasks {
		ids = append(ids, task.Task.ID)
	}
	return ids
}

// This collector has no authorization power. Public callers first authorize;
// admission callers first validate the persisted original credential and scope.
func (s *Service) planningFormalDigestTx(ctx context.Context, tx *sql.Tx, tenant, project string, ids []string, week string) (string, error) {
	ids = append([]string{}, ids...)
	sort.Strings(ids)
	h := sha256.New()
	fmt.Fprintln(h, stringify([]any{week, ids}))
	marks := make([]string, len(ids))
	args := []any{tenant, project}
	for i, id := range ids {
		marks[i] = "?"
		args = append(args, id)
	}
	queries := []struct {
		sql  string
		args []any
	}{
		{`SELECT ` + recordColumns + ` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id IN (` + strings.Join(marks, ",") + `) ORDER BY id`, args},
		{`SELECT task_id,version,iteration_id,milestone_id,start_date,due_date,estimated_hours FROM suma_task_plans WHERE tenant_id=? AND project_id=? AND task_id IN (` + strings.Join(marks, ",") + `) ORDER BY task_id`, args},
		{`SELECT r.id,r.version,r.title,r.description,r.status,r.acceptance_criteria,r.data FROM suma_requirements r WHERE r.tenant_id=? AND r.project_id=? AND EXISTS(SELECT 1 FROM suma_tasks t WHERE t.tenant_id=r.tenant_id AND t.project_id=r.project_id AND t.requirement_id=r.id AND t.id IN (` + strings.Join(marks, ",") + `)) ORDER BY r.id`, args},
		{`SELECT m.id,m.version,m.title,m.status,m.kind,m.data,c.version,c.schedule FROM suma_members m LEFT JOIN suma_resource_calendars c ON c.tenant_id=m.tenant_id AND c.project_id=m.project_id AND c.member_id=m.id WHERE m.tenant_id=? AND m.project_id=? ORDER BY m.id`, []any{tenant, project}},
		{`SELECT t.id,t.owner_id,p.version,p.start_date,p.due_date,p.estimated_hours FROM suma_tasks t LEFT JOIN suma_task_plans p ON p.tenant_id=t.tenant_id AND p.project_id=t.project_id AND p.task_id=t.id WHERE t.tenant_id=? AND t.project_id=? ORDER BY t.id`, []any{tenant, project}},
		{`SELECT om.user_id,om.role,pm.role FROM suma_org_members om JOIN suma_users u ON u.id=om.user_id LEFT JOIN suma_project_members pm ON pm.user_id=om.user_id AND pm.project_id=? WHERE om.tenant_id=? ORDER BY om.user_id`, []any{project, tenant}},
		{`SELECT id,version,kind,status,start_date,end_date,target_date FROM suma_planning_periods WHERE tenant_id=? AND project_id=? ORDER BY id`, []any{tenant, project}},
	}
	for _, query := range queries {
		rows, err := tx.QueryContext(ctx, s.q(query.sql), query.args...)
		if err != nil {
			return "", err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		for rows.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range values {
				targets[i] = &values[i]
			}
			if err = rows.Scan(targets...); err != nil {
				rows.Close()
				return "", err
			}
			fmt.Fprintln(h, stringify(values))
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return "", err
		}
		fmt.Fprintln(h, ";")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Service) planningSourceTx(ctx context.Context, auth *authorizedCommand, project string, ids []string, week string) (source PlanningSource, err error) {
	if err = assertProjectRead(auth, project); err != nil {
		return source, err
	}
	monday, err := weekMonday(week)
	if err != nil {
		return source, err
	}
	source = PlanningSource{WeekStart: monday.Format("2006-01-02"), Tasks: []PlanningSourceTask{}, Requirements: []TaskContextRecord{}, Capacity: []CapacityEntry{}}
	ids = append([]string{}, ids...)
	sort.Strings(ids)
	records := make([]Record, 0, len(ids))
	requirements := map[string]bool{}
	for _, id := range ids {
		task, e := s.getRecordTx(ctx, auth, project, "tasks", id)
		if e != nil {
			return source, e
		}
		if task.Status == "archived" || task.Status == "deleted" {
			return source, invalid(planningSourceChanged)
		}
		records = append(records, task)
		if task.RequirementID != "" {
			requirements[task.RequirementID] = true
		}
	}
	planned, err := s.plansForTx(ctx, auth, project, records)
	if err != nil {
		return source, err
	}
	for _, task := range planned {
		source.Tasks = append(source.Tasks, PlanningSourceTask{Task: taskContextRecord(task.Record), OwnerID: task.OwnerID, Status: task.Status, Priority: task.Priority, DependsOn: append([]string{}, task.DependsOn...), Plan: task.Plan})
	}
	requirementIDs := []string{}
	for id := range requirements {
		requirementIDs = append(requirementIDs, id)
	}
	sort.Strings(requirementIDs)
	for _, id := range requirementIDs {
		requirement, e := s.getRecordTx(ctx, auth, project, "requirements", id)
		if e != nil {
			return source, e
		}
		source.Requirements = append(source.Requirements, taskContextRecord(requirement))
	}
	source.Capacity, err = s.planningCapacityTx(ctx, auth, project, source.WeekStart)
	if err != nil {
		return source, err
	}
	source.SHA256, err = s.planningFormalDigestTx(ctx, auth.tx, auth.target.tenantID, project, ids, source.WeekStart)
	return source, err
}
