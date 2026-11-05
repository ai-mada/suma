package platform

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/ai-mada/suma/internal/planningcalendar"
)

const (
	projectPortfolioCapacityRowLimit = 2000
	projectPortfolioCapacityInvalid  = "Project portfolio capacity requires a canonical Monday date"
)

type ProjectPortfolioCapacityPage struct {
	WeekStart string                           `json:"week_start"`
	AsOf      string                           `json:"as_of"`
	Items     []ProjectPortfolioCapacityPerson `json:"items"`
}

type ProjectPortfolioCapacityPerson struct {
	UserID           string                         `json:"user_id"`
	Name             string                         `json:"name"`
	CalendarVersion  int                            `json:"calendar_version"`
	AvailableHours   *float64                       `json:"available_hours"`
	AllocatedHours   float64                        `json:"allocated_hours"`
	UnestimatedTasks int                            `json:"unestimated_tasks"`
	UnallocatedTasks int                            `json:"unallocated_tasks"`
	State            string                         `json:"state"`
	Days             []ProjectPortfolioCapacityDay  `json:"days"`
	Tasks            []ProjectPortfolioCapacityTask `json:"tasks"`
}

type ProjectPortfolioCapacityDay struct {
	Date           string   `json:"date"`
	AvailableHours *float64 `json:"available_hours"`
	AllocatedHours float64  `json:"allocated_hours"`
}

type ProjectPortfolioCapacityTask struct {
	ProjectID      string   `json:"project_id"`
	ProjectTitle   string   `json:"project_title"`
	TaskID         string   `json:"task_id"`
	TaskTitle      string   `json:"task_title"`
	StartDate      string   `json:"start_date"`
	DueDate        string   `json:"due_date"`
	EstimatedHours *float64 `json:"estimated_hours"`
	AllocatedHours *float64 `json:"allocated_hours"`
}

func (s *Service) ProjectPortfolioCapacity(ctx context.Context, actor Actor, weekValue string) (ProjectPortfolioCapacityPage, error) {
	week, err := weekMonday(weekValue)
	if weekValue == "" || err != nil {
		return ProjectPortfolioCapacityPage{}, invalid(projectPortfolioCapacityInvalid)
	}
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}
	var page ProjectPortfolioCapacityPage
	err = s.withCurrentRead(ctx, actor, target, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		items, err := s.projectPortfolioCapacityTx(bounded, auth, weekValue, week.AddDate(0, 0, 6).Format(time.DateOnly))
		if err != nil {
			return err
		}
		page = ProjectPortfolioCapacityPage{
			WeekStart: weekValue, AsOf: auth.checkedAt.UTC().Format(time.RFC3339), Items: items,
		}
		return nil
	})
	if err != nil {
		return ProjectPortfolioCapacityPage{}, err
	}
	return page, nil
}

func (s *Service) projectPortfolioCapacityTx(ctx context.Context, auth *authorizedCommand, start, end string) ([]ProjectPortfolioCapacityPerson, error) {
	if err := assertOrganizationRead(auth); err != nil {
		return nil, err
	}
	p := auth.currentPrincipal
	scope, args := s.visibleHumanScheduleScope(p, start, end)
	// Bound the eligible task set before ranking it. Attach the calendar to
	// just one row per user so large exception lists are not copied per task.
	query := `WITH selected_assignments AS (
		SELECT u.id AS user_id,u.name AS user_name,p.id AS project_id,p.title AS project_title,
			t.id AS task_id,t.title AS task_title,sp.start_date,sp.due_date,sp.estimated_hours
		` + scope + ` LIMIT ?
	), ranked_assignments AS (
		SELECT selected_assignments.*,COUNT(*) OVER () AS assignment_count,
			ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY task_id) AS assignee_row
		FROM selected_assignments
	)
	SELECT a.user_id,a.user_name,a.project_id,a.project_title,a.task_id,a.task_title,a.start_date,a.due_date,
		a.estimated_hours,COALESCE(c.version,0),c.schedule,a.assignment_count
	FROM ranked_assignments a
	LEFT JOIN suma_organization_work_calendars c ON a.assignee_row=1 AND c.tenant_id=? AND c.user_id=a.user_id
	ORDER BY a.user_id,a.assignee_row`
	rows, err := auth.tx.QueryContext(ctx, s.q(query), append(args, projectPortfolioCapacityRowLimit+1, p.TenantID)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]ProjectPortfolioCapacityPerson, 0)
	var person *ProjectPortfolioCapacityPerson
	var schedule ResourceCalendarSchedule
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var userID, name string
		var version, assignmentCount int
		var rawSchedule sql.NullString
		var task ProjectPortfolioCapacityTask
		if err := rows.Scan(&userID, &name, &task.ProjectID, &task.ProjectTitle, &task.TaskID, &task.TaskTitle,
			&task.StartDate, &task.DueDate, &task.EstimatedHours, &version, &rawSchedule, &assignmentCount); err != nil {
			return nil, err
		}
		if assignmentCount > projectPortfolioCapacityRowLimit {
			return nil, &APIError{
				Status: http.StatusConflict, Code: "project_capacity_scope_too_large",
				Message: "Too many assignments to calculate complete project portfolio capacity",
			}
		}
		if person == nil || person.UserID != userID {
			schedule, err = portfolioCapacityCalendar(version, rawSchedule)
			if err != nil {
				return nil, err
			}
			entry, err := newPortfolioCapacityPerson(userID, name, version, start, schedule)
			if err != nil {
				return nil, err
			}
			items = append(items, entry)
			person = &items[len(items)-1]
		}
		if err := person.allocatePortfolioTask(schedule, &task); err != nil {
			return nil, err
		}
		person.Tasks = append(person.Tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range items {
		person := &items[i]
		switch {
		case person.AvailableHours != nil && person.AllocatedHours > *person.AvailableHours+1e-9:
			person.State = "over_capacity"
		case person.AvailableHours == nil || person.UnestimatedTasks > 0 || person.UnallocatedTasks > 0:
			person.State = "basis_incomplete"
		default:
			person.State = "within_visible_capacity"
		}
		slices.SortFunc(person.Tasks, func(a, b ProjectPortfolioCapacityTask) int {
			return cmp.Or(cmp.Compare(a.ProjectTitle, b.ProjectTitle), cmp.Compare(a.ProjectID, b.ProjectID),
				cmp.Compare(a.StartDate, b.StartDate), cmp.Compare(a.DueDate, b.DueDate), cmp.Compare(a.TaskID, b.TaskID))
		})
	}
	slices.SortFunc(items, func(a, b ProjectPortfolioCapacityPerson) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), cmp.Compare(a.UserID, b.UserID))
	})
	return items, nil
}

func portfolioCapacityCalendar(version int, raw sql.NullString) (ResourceCalendarSchedule, error) {
	var schedule ResourceCalendarSchedule
	if !raw.Valid && version == 0 {
		return schedule, nil
	}
	if err := json.Unmarshal([]byte(raw.String), &schedule); err != nil {
		return schedule, err
	}
	if version < 1 || len(schedule.Weekdays) != 7 || schedule.Exceptions == nil {
		return schedule, errors.New("invalid stored organization work calendar")
	}
	return schedule, nil
}

func newPortfolioCapacityPerson(userID, name string, version int, week string, schedule ResourceCalendarSchedule) (ProjectPortfolioCapacityPerson, error) {
	days, err := planningcalendar.Week(schedule, week)
	if err != nil {
		return ProjectPortfolioCapacityPerson{}, err
	}
	person := ProjectPortfolioCapacityPerson{
		UserID: userID, Name: name, CalendarVersion: version,
		Days: make([]ProjectPortfolioCapacityDay, len(days)),
	}
	known, total := true, 0.0
	for i, day := range days {
		person.Days[i] = ProjectPortfolioCapacityDay{Date: day.Date, AvailableHours: day.AvailableHours}
		if day.AvailableHours == nil {
			known = false
		} else {
			total += *day.AvailableHours
		}
	}
	if known {
		person.AvailableHours = &total
	}
	return person, nil
}

func (p *ProjectPortfolioCapacityPerson) allocatePortfolioTask(schedule ResourceCalendarSchedule, task *ProjectPortfolioCapacityTask) error {
	if !validDate(task.StartDate) || !validDate(task.DueDate) || task.StartDate > task.DueDate {
		return conflict("Scheduled assignment has an invalid date interval")
	}
	if task.EstimatedHours == nil {
		p.UnestimatedTasks++
		return nil
	}
	if _, err := finiteHours(*task.EstimatedHours, 100000); err != nil {
		return conflict("Scheduled assignment has an invalid estimate")
	}
	var allocated float64
	if *task.EstimatedHours > 0 {
		weight, unknown, err := planningcalendar.Weight(schedule, task.StartDate, task.DueDate)
		if err != nil {
			return err
		}
		if unknown > 0 || weight == 0 {
			p.UnallocatedTasks++
			return nil
		}
		for i := range p.Days {
			day := &p.Days[i]
			if day.Date < task.StartDate || day.Date > task.DueDate {
				continue
			}
			// A complete interval weight proves that each overlapping date has
			// a known value, including any explicit zero-hour date.
			hours := *task.EstimatedHours * (*day.AvailableHours / weight)
			day.AllocatedHours += hours
			allocated += hours
		}
	}
	task.AllocatedHours = &allocated
	p.AllocatedHours += allocated
	return nil
}
