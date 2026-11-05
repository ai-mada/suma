package platform

import (
	"cmp"
	"context"
	"slices"
	"time"
)

const (
	projectScheduleOverlapRowLimit = 2000
	projectScheduleOverlapInvalid  = "Schedule overlap requires a canonical Monday date"
)

type ProjectScheduleOverlapPage struct {
	WeekStart string                         `json:"week_start"`
	AsOf      string                         `json:"as_of"`
	Items     []ProjectScheduleOverlapPerson `json:"items"`
}

type ProjectScheduleOverlapPerson struct {
	UserID      string                             `json:"user_id"`
	Name        string                             `json:"name"`
	Days        []string                           `json:"days"`
	Assignments []ProjectScheduleOverlapAssignment `json:"assignments"`
}

type ProjectScheduleOverlapAssignment struct {
	ProjectID    string `json:"project_id"`
	ProjectTitle string `json:"project_title"`
	TaskID       string `json:"task_id"`
	TaskTitle    string `json:"task_title"`
	StartDate    string `json:"start_date"`
	DueDate      string `json:"due_date"`
}

func (s *Service) ProjectScheduleOverlaps(ctx context.Context, actor Actor, weekValue string) (ProjectScheduleOverlapPage, error) {
	week, err := weekMonday(weekValue)
	if weekValue == "" || err != nil {
		return ProjectScheduleOverlapPage{}, invalid(projectScheduleOverlapInvalid)
	}
	var days [7]string
	for i := range days {
		days[i] = week.AddDate(0, 0, i).Format(time.DateOnly)
	}

	target := commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}
	var page ProjectScheduleOverlapPage
	err = s.withCurrentRead(ctx, actor, target, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		items, err := s.projectScheduleOverlapTx(bounded, auth, days)
		if err != nil {
			return err
		}
		page = ProjectScheduleOverlapPage{
			WeekStart: days[0], AsOf: auth.checkedAt.UTC().Format(time.RFC3339), Items: items,
		}
		return nil
	})
	if err != nil {
		return ProjectScheduleOverlapPage{}, err
	}
	return page, nil
}

func (s *Service) projectScheduleOverlapTx(ctx context.Context, auth *authorizedCommand, days [7]string) ([]ProjectScheduleOverlapPerson, error) {
	if err := assertOrganizationRead(auth); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal

	// Use exact role comparisons on OceanBase's case-insensitive collations.
	orgRole, assigneeRole, callerRole := "om.role", "assignee.role", "caller.role"
	if s.Dialect == "oceanbase" {
		orgRole, assigneeRole, callerRole = "BINARY "+orgRole, "BINARY "+assigneeRole, "BINARY "+callerRole
	}
	query := `SELECT u.id,u.name,p.id,p.title,t.id,t.title,sp.start_date,sp.due_date
		FROM suma_task_plans sp
		JOIN suma_projects p ON p.id=sp.project_id AND p.tenant_id=sp.tenant_id
		JOIN suma_tasks t ON t.id=sp.task_id AND t.tenant_id=sp.tenant_id AND t.project_id=sp.project_id
		JOIN suma_human_resources hr ON hr.resource_id=t.owner_id AND hr.tenant_id=sp.tenant_id AND hr.project_id=sp.project_id
		JOIN suma_members m ON m.id=hr.resource_id AND m.tenant_id=hr.tenant_id AND m.project_id=hr.project_id
		JOIN suma_org_members om ON om.tenant_id=hr.tenant_id AND om.user_id=hr.user_id
		JOIN suma_users u ON u.id=hr.user_id
		LEFT JOIN suma_project_members assignee ON assignee.project_id=sp.project_id AND assignee.user_id=hr.user_id
		WHERE p.tenant_id=? AND m.kind='human' AND m.status NOT IN ('archived','deleted')
		AND (` + orgRole + ` IN ('owner','admin') OR ` + assigneeRole + ` IN ('admin','developer','reviewer','viewer'))
		AND t.status NOT IN ('accepted','archived','deleted','canceled','cancelled')
		AND sp.start_date<>'' AND sp.due_date<>'' AND sp.start_date<=? AND sp.due_date>=?`
	args := []any{p.TenantID, days[6], days[0]}
	// Authorize the caller's project scope before counting rows or comparing
	// dates. An unreadable project cannot affect overlaps or the row limit.
	if !orgAdmin(p) {
		query += ` AND EXISTS(SELECT 1 FROM suma_project_members caller
			WHERE caller.project_id=p.id AND caller.user_id=? AND ` + callerRole + ` IN ('admin','developer','reviewer','viewer'))`
		args = append(args, p.ID)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(query+" LIMIT ?"), append(args, projectScheduleOverlapRowLimit+1)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	people := make(map[string]*ProjectScheduleOverlapPerson)
	rowCount := 0
	for rows.Next() {
		rowCount++
		if rowCount > projectScheduleOverlapRowLimit {
			return nil, &APIError{
				Status: 409, Code: "schedule_overlap_scope_too_large",
				Message: "Too many assignments to calculate a complete schedule overlap",
			}
		}
		var userID, name string
		var assignment ProjectScheduleOverlapAssignment
		if err := rows.Scan(&userID, &name, &assignment.ProjectID, &assignment.ProjectTitle, &assignment.TaskID, &assignment.TaskTitle, &assignment.StartDate, &assignment.DueDate); err != nil {
			return nil, err
		}
		if !validDate(assignment.StartDate) || !validDate(assignment.DueDate) || assignment.StartDate > assignment.DueDate {
			return nil, conflict("Scheduled assignment has an invalid date interval")
		}
		person := people[userID]
		if person == nil {
			person = &ProjectScheduleOverlapPerson{UserID: userID, Name: name}
			people[userID] = person
		}
		person.Assignments = append(person.Assignments, assignment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	items := make([]ProjectScheduleOverlapPerson, 0)
	for _, person := range people {
		if person.retainScheduleOverlaps(days) {
			items = append(items, *person)
		}
	}
	slices.SortFunc(items, func(a, b ProjectScheduleOverlapPerson) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.UserID, b.UserID))
	})
	return items, nil
}

func (p *ProjectScheduleOverlapPerson) retainScheduleOverlaps(days [7]string) bool {
	var firstProject [7]string
	var overlaps [7]bool
	for _, assignment := range p.Assignments {
		for i, day := range days {
			if day < assignment.StartDate || day > assignment.DueDate {
				continue
			}
			if firstProject[i] == "" {
				firstProject[i] = assignment.ProjectID
			} else if firstProject[i] != assignment.ProjectID {
				overlaps[i] = true
			}
		}
	}
	for i, day := range days {
		if overlaps[i] {
			p.Days = append(p.Days, day)
		}
	}
	if len(p.Days) == 0 {
		return false
	}
	p.Assignments = slices.DeleteFunc(p.Assignments, func(a ProjectScheduleOverlapAssignment) bool {
		for _, day := range p.Days {
			if a.StartDate <= day && day <= a.DueDate {
				return false
			}
		}
		return true
	})
	slices.SortFunc(p.Assignments, func(a, b ProjectScheduleOverlapAssignment) int {
		return cmp.Or(cmp.Compare(a.ProjectTitle, b.ProjectTitle), cmp.Compare(a.ProjectID, b.ProjectID),
			cmp.Compare(a.StartDate, b.StartDate), cmp.Compare(a.DueDate, b.DueDate), cmp.Compare(a.TaskID, b.TaskID))
	})
	return true
}
