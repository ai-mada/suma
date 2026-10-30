package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	taskWorkspaceInvalid       = "Task workspace query is invalid"
	taskWorkspaceCursorInvalid = "Task workspace cursor does not match the current query"
	taskWorkspacePriorityOrder = `CASE priority WHEN 'urgent' THEN 4 WHEN 'high' THEN 3 WHEN 'medium' THEN 2 WHEN 'low' THEN 1 ELSE 0 END`
)

type TaskWorkspaceQuery struct {
	Q              string `json:"q,omitempty"`
	Status         string `json:"status,omitempty"`
	StatusUnset    bool   `json:"status_unset,omitempty"`
	OwnerID        string `json:"owner_id,omitempty"`
	Priority       string `json:"priority,omitempty"`
	RequirementID  string `json:"requirement_id,omitempty"`
	IterationID    string `json:"iteration_id,omitempty"`
	IterationUnset bool   `json:"iteration_unset,omitempty"`
	Unassigned     bool   `json:"unassigned,omitempty"`
	PriorityUnset  bool   `json:"priority_unset,omitempty"`
	Sort           string `json:"sort,omitempty"`
	Limit          int    `json:"limit,omitempty"`
	Cursor         string `json:"cursor,omitempty"`
}

// Each page describes current authorized facts. AsOf is the observation time,
// not a frozen cross-request snapshot: later edits can change totals or order.
type TaskWorkspacePage struct {
	ProjectID  string                 `json:"project_id"`
	Limit      int                    `json:"limit"`
	Items      []Record               `json:"items"`
	TaskPlans  []TaskWorkspacePlanRef `json:"task_plans"`
	Total      int                    `json:"total"`
	NextCursor string                 `json:"next_cursor"`
	AsOf       string                 `json:"as_of"`
}

// TaskWorkspacePlanRef comes from the planning domain, never task payload data.
// A zero plan version means there is no plan; an existing plan can have no iteration.
type TaskWorkspacePlanRef struct {
	TaskID      string `json:"task_id"`
	IterationID string `json:"iteration_id"`
	PlanVersion int    `json:"plan_version"`
}

const taskWorkspaceIterationKey = `COALESCE((SELECT p.iteration_id FROM suma_task_plans p WHERE p.tenant_id=suma_tasks.tenant_id AND p.project_id=suma_tasks.project_id AND p.task_id=suma_tasks.id),'')`

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
	if !handoffID(q.IterationID, true) || q.IterationUnset && q.IterationID != "" {
		return q, invalid(taskWorkspaceInvalid)
	}
	// Search and exact values remain literal. Whitespace cannot silently select
	// a different task set; empty owner/priority are selected by explicit flags.
	if !taskWorkspaceText(q.Q) || utf8.RuneCountInString(q.Q) > 200 || len(q.Cursor) > 4096 || !handoffID(q.OwnerID, true) || !handoffID(q.RequirementID, true) || q.Unassigned && q.OwnerID != "" || q.PriorityUnset && q.Priority != "" || q.StatusUnset && q.Status != "" {
		return q, invalid(taskWorkspaceInvalid)
	}
	for _, value := range []string{q.Status, q.Priority} {
		if utf8.RuneCountInString(value) > 200 || !taskWorkspaceText(value) || value != "" && strings.TrimSpace(value) == "" {
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
		if !validRecordOrderKey(c.Value) {
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
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
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
	if !myWorkHuman(auth.actor) {
		return TaskWorkspacePage{}, forbidden()
	}
	q, err := normalizeTaskWorkspaceQuery(input)
	if err != nil || !handoffID(project, false) {
		return TaskWorkspacePage{}, invalid(taskWorkspaceInvalid)
	}
	p := auth.currentPrincipal
	fingerprint := q
	fingerprint.Cursor = ""
	position := taskWorkspaceCursor{Version: 1, Subject: p.ID, Tenant: p.TenantID, Project: project, Filter: digest(stringify(fingerprint))}
	if q.Cursor != "" {
		position, err = taskWorkspaceCursorDecode(q.Cursor, position, q.Sort)
		if err != nil {
			return TaskWorkspacePage{}, err
		}
	}
	out := TaskWorkspacePage{ProjectID: project, Limit: q.Limit, Items: []Record{}, AsOf: auth.checkedAt.UTC().Format(time.RFC3339Nano)}
	where, args := taskWorkspaceWhere(p.TenantID, project, q)
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
	if q.Sort == "updated_desc" || q.Sort == "created_desc" {
		column, err = recordOrderKeySQL(s.Dialect, column)
		if err != nil {
			return TaskWorkspacePage{}, err
		}
		column = "(" + column + ")"
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
		if q.Sort == "updated_desc" || q.Sort == "created_desc" {
			source := item.UpdatedAt
			if q.Sort == "created_desc" {
				source = item.CreatedAt
			}
			if _, keyErr := recordOrderKey(source); keyErr != nil {
				return TaskWorkspacePage{}, keyErr
			}
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return TaskWorkspacePage{}, err
	}
	if err = rows.Close(); err != nil {
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
		if q.Sort == "updated_desc" || q.Sort == "created_desc" {
			position.Value, err = recordOrderKey(position.Value)
			if err != nil {
				return TaskWorkspacePage{}, err
			}
		}
		out.NextCursor = taskWorkspaceCursorEncode(position)
	}
	out.TaskPlans, err = s.taskWorkspacePlanRefs(ctx, auth, project, out.Items)
	if err != nil {
		return TaskWorkspacePage{}, err
	}
	return out, nil
}

// Read only this bounded page's plan relations in the same authorized transaction.
func (s *Service) taskWorkspacePlanRefs(ctx context.Context, auth *authorizedCommand, project string, items []Record) ([]TaskWorkspacePlanRef, error) {
	out := make([]TaskWorkspacePlanRef, len(items))
	if len(items) == 0 {
		return out, nil
	}
	positions := make(map[string]int, len(items))
	args := []any{auth.target.tenantID, project}
	marks := make([]string, len(items))
	for i, item := range items {
		out[i].TaskID = item.ID
		positions[item.ID] = i
		args = append(args, item.ID)
		marks[i] = "?"
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT task_id,iteration_id,version FROM suma_task_plans WHERE tenant_id=? AND project_id=? AND task_id IN (`+strings.Join(marks, ",")+`)`), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var plan TaskWorkspacePlanRef
		if err = rows.Scan(&plan.TaskID, &plan.IterationID, &plan.PlanVersion); err != nil {
			return nil, err
		}
		if i, ok := positions[plan.TaskID]; ok {
			out[i] = plan
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// One parameterized compiler serves both complete group counts and canonical
// item reads. No business status, including archived or literal "all", is hidden.
func taskWorkspaceWhere(tenant, project string, q TaskWorkspaceQuery) (string, []any) {
	where := ` FROM suma_tasks WHERE tenant_id=? AND project_id=?`
	args := []any{tenant, project}
	if q.Q != "" {
		like := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(q.Q) + "%"
		where += ` AND (LOWER(title) LIKE LOWER(?) ESCAPE '!' OR LOWER(description) LIKE LOWER(?) ESCAPE '!' OR LOWER(id) LIKE LOWER(?) ESCAPE '!')`
		args = append(args, like, like, like)
	}
	if q.Status != "" {
		where += ` AND status=?`
		args = append(args, q.Status)
	}
	if q.StatusUnset {
		where += ` AND status=''`
	}
	if q.Priority != "" {
		where += ` AND priority=?`
		args = append(args, q.Priority)
	}
	if q.PriorityUnset {
		where += ` AND priority=''`
	}
	if q.OwnerID != "" {
		where += ` AND owner_id=?`
		args = append(args, q.OwnerID)
	}
	if q.Unassigned {
		where += ` AND owner_id=''`
	}
	if q.RequirementID != "" {
		where += ` AND requirement_id=?`
		args = append(args, q.RequirementID)
	}
	if q.IterationID != "" {
		where += ` AND ` + taskWorkspaceIterationKey + `=?`
		args = append(args, q.IterationID)
	}
	if q.IterationUnset {
		where += ` AND ` + taskWorkspaceIterationKey + `=''`
	}
	return where, args
}
