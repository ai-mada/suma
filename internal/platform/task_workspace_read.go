package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	taskWorkspaceInvalid       = "Task workspace query is invalid"
	taskWorkspaceCursorInvalid = "Task workspace cursor does not match the current query"
	taskWorkspacePriorityOrder = `CASE priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END`
)

type TaskWorkspaceQuery struct {
	Q        string `json:"q,omitempty"`
	Status   string `json:"status,omitempty"`
	OwnerID  string `json:"owner_id,omitempty"`
	Priority string `json:"priority,omitempty"`
	Sort     string `json:"sort,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	Cursor   string `json:"cursor,omitempty"`
}

// Each page describes current authorized facts. AsOf is the observation time,
// not a frozen cross-request snapshot: later edits can change totals or order.
type TaskWorkspacePage struct {
	Items      []Record `json:"items"`
	Total      int      `json:"total"`
	NextCursor string   `json:"next_cursor"`
	AsOf       string   `json:"as_of"`
}

type taskWorkspaceCursor struct {
	Version int    `json:"version"`
	Subject string `json:"subject"`
	Tenant  string `json:"tenant"`
	Project string `json:"project"`
	Filter  string `json:"filter"`
	Value   string `json:"value"`
	ID      string `json:"id"`
}

func taskWorkspaceText(value string) bool {
	if !utf8.ValidString(value) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func normalizeTaskWorkspaceQuery(q TaskWorkspaceQuery) (TaskWorkspaceQuery, error) {
	// Only the free-text query is normalized. Exact filter values must already
	// be trimmed, so a malformed value cannot silently select a different set.
	q.Q = strings.TrimSpace(q.Q)
	if !taskWorkspaceText(q.Q) || utf8.RuneCountInString(q.Q) > 200 || len(q.Cursor) > 4096 || !handoffID(q.OwnerID, true) || !taskWorkspaceText(q.OwnerID) {
		return q, invalid(taskWorkspaceInvalid)
	}
	for _, value := range []string{q.Status, q.Priority} {
		if len(value) > 64 || !taskWorkspaceText(value) || strings.TrimSpace(value) != value {
			return q, invalid(taskWorkspaceInvalid)
		}
	}
	if q.Sort == "" {
		q.Sort = "updated_desc"
	}
	switch q.Sort {
	case "updated_desc", "created_desc", "title_asc", "priority_desc":
	default:
		return q, invalid(taskWorkspaceInvalid)
	}
	if q.Limit == 0 {
		q.Limit = 100
	}
	if q.Limit < 1 || q.Limit > 100 {
		return q, invalid(taskWorkspaceInvalid)
	}
	return q, nil
}

func taskWorkspaceCursorEncode(c taskWorkspaceCursor) string {
	// Encode the sort value separately so even a valid 500-byte title made of
	// JSON-escaped characters stays inside the bounded outer cursor.
	c.Value = base64.RawURLEncoding.EncodeToString([]byte(c.Value))
	return base64.RawURLEncoding.EncodeToString([]byte(stringify(c)))
}

func taskWorkspaceCursorDecode(encoded string, expected taskWorkspaceCursor, mode string) (taskWorkspaceCursor, error) {
	var c taskWorkspaceCursor
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || json.Unmarshal(raw, &c) != nil {
		return c, invalid(taskWorkspaceCursorInvalid)
	}
	value, err := base64.RawURLEncoding.Strict().DecodeString(c.Value)
	c.Value = string(value)
	if err != nil || taskWorkspaceCursorEncode(c) != encoded || c.Version != expected.Version || c.Subject != expected.Subject || c.Tenant != expected.Tenant || c.Project != expected.Project || c.Filter != expected.Filter || !handoffID(c.ID, false) {
		return c, invalid(taskWorkspaceCursorInvalid)
	}
	switch mode {
	case "updated_desc", "created_desc":
		if !validReleaseTime(c.Value) {
			return c, invalid(taskWorkspaceCursorInvalid)
		}
	case "title_asc":
		if len(c.Value) > 500 || c.Value == "" || !utf8.ValidString(c.Value) {
			return c, invalid(taskWorkspaceCursorInvalid)
		}
	case "priority_desc":
		if len(c.Value) != 1 || c.Value < "0" || c.Value > "4" {
			return c, invalid(taskWorkspaceCursorInvalid)
		}
	}
	return c, nil
}

func taskWorkspacePriorityRank(priority string) int {
	switch priority {
	case "urgent":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

// ListTaskWorkspace deliberately uses the ordinary project-read authority.
// An agent/runner credential is not an alternative task workspace identity.
func (s *Service) ListTaskWorkspace(ctx context.Context, actor Actor, project string, q TaskWorkspaceQuery) (out TaskWorkspacePage, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.taskWorkspaceTx(bounded, auth, project, q)
		return err
	})
	if err != nil {
		return TaskWorkspacePage{}, err
	}
	return out, nil
}

func (s *Service) taskWorkspaceTx(ctx context.Context, auth *authorizedCommand, project string, input TaskWorkspaceQuery) (TaskWorkspacePage, error) {
	if err := assertProjectRead(auth, project); err != nil {
		return TaskWorkspacePage{}, err
	}
	q, err := normalizeTaskWorkspaceQuery(input)
	if err != nil || !handoffID(project, false) {
		return TaskWorkspacePage{}, invalid(taskWorkspaceInvalid)
	}
	p := auth.currentPrincipal
	position := taskWorkspaceCursor{Version: 1, Subject: p.ID, Tenant: p.TenantID, Project: project, Filter: digest(stringify([]string{q.Q, q.Status, q.OwnerID, q.Priority, q.Sort}))}
	if q.Cursor != "" {
		position, err = taskWorkspaceCursorDecode(q.Cursor, position, q.Sort)
		if err != nil {
			return TaskWorkspacePage{}, err
		}
	}
	out := TaskWorkspacePage{Items: []Record{}, AsOf: now()}
	where := ` FROM suma_tasks WHERE tenant_id=? AND project_id=?`
	args := []any{p.TenantID, project}
	if q.Q != "" {
		// Explicit folding and the portable ! escape preserve literal %, _ and !
		// across the supported SQL dialects. Search never truncates before WHERE.
		like := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q.Q) + "%"
		where += ` AND (LOWER(title) LIKE LOWER(?) ESCAPE '!' OR LOWER(description) LIKE LOWER(?) ESCAPE '!' OR LOWER(id) LIKE LOWER(?) ESCAPE '!')`
		args = append(args, like, like, like)
	}
	if q.Status != "" {
		where += ` AND status=?`
		args = append(args, q.Status)
	}
	if q.Priority != "" {
		where += ` AND priority=?`
		args = append(args, q.Priority)
	}
	if q.OwnerID != "" {
		owner := q.OwnerID
		if owner == "__unassigned__" {
			owner = ""
		}
		where += ` AND owner_id=?`
		args = append(args, owner)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+where), args...).Scan(&out.Total); err != nil {
		return TaskWorkspacePage{}, err
	}
	column, direction, operator := "updated_at", "DESC", "<"
	switch q.Sort {
	case "created_desc":
		column = "created_at"
	case "title_asc":
		column, direction, operator = "title", "ASC", ">"
	case "priority_desc":
		column = taskWorkspacePriorityOrder
	}
	query := `SELECT ` + recordColumns + where
	if q.Cursor != "" {
		var value any = position.Value
		if q.Sort == "priority_desc" {
			value, _ = strconv.Atoi(position.Value)
		}
		query += ` AND (` + column + operator + `? OR (` + column + `=? AND id>?))`
		args = append(args, value, value, position.ID)
	}
	query += ` ORDER BY ` + column + ` ` + direction + `,id ASC LIMIT ?`
	args = append(args, q.Limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return TaskWorkspacePage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		item, err := scanRecord(rows)
		if err != nil {
			return TaskWorkspacePage{}, err
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return TaskWorkspacePage{}, err
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		last := out.Items[q.Limit-1]
		position.ID, position.Value = last.ID, last.UpdatedAt
		switch q.Sort {
		case "created_desc":
			position.Value = last.CreatedAt
		case "title_asc":
			position.Value = last.Title
		case "priority_desc":
			position.Value = strconv.Itoa(taskWorkspacePriorityRank(last.Priority))
		}
		out.NextCursor = taskWorkspaceCursorEncode(position)
	}
	return out, nil
}
