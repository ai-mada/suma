package platform

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	projectPortfolioInvalid = "Invalid project portfolio query or cursor"
	// Project timestamps are canonical UTC RFC3339Nano. Removing the Z suffix
	// keeps a whole second before its fractional instants in lexical order.
	projectPortfolioTimeOrder = "REPLACE(p.created_at,'Z','')"
)

type ProjectPortfolioPage struct {
	Items      []ProjectPortfolioItem `json:"items"`
	AsOf       string                 `json:"as_of"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}

type ProjectPortfolioItem struct {
	ProjectID     string                     `json:"project_id"`
	Title         string                     `json:"title"`
	Description   string                     `json:"description"`
	Status        string                     `json:"status"`
	TaskTotal     int                        `json:"task_total"`
	AcceptedTasks int                        `json:"accepted_tasks"`
	BlockedTasks  int                        `json:"blocked_tasks"`
	ActiveRisks   int                        `json:"active_risks"`
	NextMilestone *ProjectPortfolioMilestone `json:"next_milestone,omitempty"`
}

type ProjectPortfolioMilestone struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TargetDate string `json:"target_date"`
}

type projectPortfolioCursor struct {
	Organization string `json:"organization"`
	User         string `json:"user"`
	SearchSHA256 string `json:"search_sha256"`
	Limit        int    `json:"limit"`
	ProjectID    string `json:"project_id"`
}

func (s *Service) ProjectPortfolio(ctx context.Context, actor Actor, search string, limit int, after string) (ProjectPortfolioPage, error) {
	search = strings.TrimSpace(search)
	if !utf8.ValidString(search) || len(search) > 120 || strings.ContainsRune(search, 0) || limit < 1 || limit > 50 || len(after) > 1024 {
		return ProjectPortfolioPage{}, invalid(projectPortfolioInvalid)
	}
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}
	var page ProjectPortfolioPage
	err := s.withCurrentRead(ctx, actor, target, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var err error
		page, err = s.projectPortfolioPageTx(bounded, auth, search, limit, after)
		return err
	})
	if err != nil {
		return ProjectPortfolioPage{}, err
	}
	return page, nil
}

func projectPortfolioCursorAnchor(after string, scope projectPortfolioCursor) (string, error) {
	var supplied projectPortfolioCursor
	raw, err := base64.RawURLEncoding.Strict().DecodeString(after)
	if err != nil || json.Unmarshal(raw, &supplied) != nil || base64.RawURLEncoding.EncodeToString([]byte(stringify(supplied))) != after {
		return "", invalid(projectPortfolioInvalid)
	}
	id := supplied.ProjectID
	supplied.ProjectID = ""
	if supplied != scope || !handoffID(id, false) {
		return "", invalid(projectPortfolioInvalid)
	}
	return id, nil
}

func (s *Service) projectPortfolioPageTx(ctx context.Context, auth *authorizedCommand, search string, limit int, after string) (ProjectPortfolioPage, error) {
	// Project grant changes take the same organization authority lock. The
	// selection, cursor proof, and all aggregates use this one transaction.
	p := auth.currentPrincipal
	where := " FROM suma_projects p WHERE p.tenant_id=?"
	args := []any{p.TenantID}
	if !orgAdmin(p) {
		role := "m.role"
		if s.Dialect == "oceanbase" {
			role = "BINARY m.role"
		}
		where += " AND EXISTS (SELECT 1 FROM suma_project_members m WHERE m.project_id=p.id AND m.user_id=? AND " + role + " IN ('admin','developer','reviewer','viewer'))"
		args = append(args, p.ID)
	}
	if search != "" {
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(search) + "%"
		where += " AND (LOWER(p.title) LIKE LOWER(?) ESCAPE '!' OR LOWER(p.description) LIKE LOWER(?) ESCAPE '!')"
		args = append(args, pattern, pattern)
	}

	scope := projectPortfolioCursor{Organization: p.TenantID, User: p.ID, SearchSHA256: digest(search), Limit: limit}
	if after != "" {
		id, err := projectPortfolioCursorAnchor(after, scope)
		if err != nil {
			return ProjectPortfolioPage{}, err
		}
		var instant string
		err = auth.tx.QueryRowContext(ctx, s.q("SELECT "+projectPortfolioTimeOrder+where+" AND p.id=?"), append(args, id)...).Scan(&instant)
		if errors.Is(err, sql.ErrNoRows) {
			return ProjectPortfolioPage{}, invalid(projectPortfolioInvalid)
		}
		if err != nil {
			return ProjectPortfolioPage{}, err
		}
		where += " AND (" + projectPortfolioTimeOrder + "<? OR (" + projectPortfolioTimeOrder + "=? AND p.id<?))"
		args = append(args, instant, instant, id)
	}

	page := ProjectPortfolioPage{Items: make([]ProjectPortfolioItem, 0, limit), AsOf: auth.checkedAt.UTC().Format(time.RFC3339)}
	rows, err := auth.tx.QueryContext(ctx, s.q("SELECT p.id,p.title,p.description,p.status"+where+" ORDER BY "+projectPortfolioTimeOrder+" DESC,p.id DESC LIMIT ?"), append(args, limit+1)...)
	if err != nil {
		return ProjectPortfolioPage{}, err
	}
	for rows.Next() {
		var item ProjectPortfolioItem
		if err = rows.Scan(&item.ProjectID, &item.Title, &item.Description, &item.Status); err != nil {
			rows.Close()
			return ProjectPortfolioPage{}, err
		}
		page.Items = append(page.Items, item)
	}
	err = rows.Err()
	rows.Close() // Close before the next statement on the same transaction.
	if err != nil {
		return ProjectPortfolioPage{}, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		scope.ProjectID = page.Items[limit-1].ProjectID
		page.NextCursor = base64.RawURLEncoding.EncodeToString([]byte(stringify(scope)))
	}
	if len(page.Items) > 0 {
		err = s.projectPortfolioFactsTx(ctx, auth, page.Items)
	}
	return page, err
}

func (s *Service) projectPortfolioFactsTx(ctx context.Context, auth *authorizedCommand, items []ProjectPortfolioItem) error {
	byProject := make(map[string]*ProjectPortfolioItem, len(items))
	args := make([]any, 1, len(items)+1)
	args[0] = auth.currentPrincipal.TenantID
	for i := range items {
		byProject[items[i].ProjectID] = &items[i]
		args = append(args, items[i].ProjectID)
	}
	scope := " WHERE tenant_id=? AND project_id IN (" + strings.TrimSuffix(strings.Repeat("?,", len(items)), ",") + ")"

	// Each fact uses its own source so multiple risks or milestones cannot
	// multiply task counts. Only displayed projects enter these statements.
	taskQuery := `SELECT project_id,COUNT(*),
		SUM(CASE WHEN status='accepted' THEN 1 ELSE 0 END),
		SUM(CASE WHEN status='blocked' THEN 1 ELSE 0 END)
		FROM suma_tasks` + scope + " GROUP BY project_id"
	if err := s.scanProjectPortfolioRows(ctx, auth, taskQuery, args, func(rows *sql.Rows) error {
		var project string
		var total, accepted, blocked int
		if err := rows.Scan(&project, &total, &accepted, &blocked); err != nil {
			return err
		}
		item := byProject[project]
		item.TaskTotal, item.AcceptedTasks, item.BlockedTasks = total, accepted, blocked
		return nil
	}); err != nil {
		return err
	}
	riskQuery := "SELECT project_id,COUNT(*) FROM suma_risks" + scope + " AND state IN ('open','mitigating','in_review') GROUP BY project_id"
	if err := s.scanProjectPortfolioRows(ctx, auth, riskQuery, args, func(rows *sql.Rows) error {
		var project string
		var active int
		if err := rows.Scan(&project, &active); err != nil {
			return err
		}
		byProject[project].ActiveRisks = active
		return nil
	}); err != nil {
		return err
	}
	milestoneQuery := `SELECT project_id,id,title,target_date FROM (
		SELECT project_id,id,title,target_date,
		ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY target_date ASC,id ASC) AS date_rank
		FROM suma_planning_periods` + scope +
		" AND kind='milestones' AND status IN ('planned','active') AND target_date<>'') dated WHERE date_rank=1"
	return s.scanProjectPortfolioRows(ctx, auth, milestoneQuery, args, func(rows *sql.Rows) error {
		var project string
		var milestone ProjectPortfolioMilestone
		if err := rows.Scan(&project, &milestone.ID, &milestone.Title, &milestone.TargetDate); err != nil {
			return err
		}
		if !validDate(milestone.TargetDate) {
			return conflict("Invalid project milestone date")
		}
		byProject[project].NextMilestone = &milestone
		return nil
	})
}

func (s *Service) scanProjectPortfolioRows(ctx context.Context, auth *authorizedCommand, query string, args []any, scan func(*sql.Rows) error) error {
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}
