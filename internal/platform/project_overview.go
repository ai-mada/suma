package platform

import (
	"context"
	"time"
)

// OverviewDelivery separates formal acceptance from the state of each task's
// exact linked run. These are overlapping facts, not parts of a progress pie.
type OverviewDelivery struct {
	Scope            string `json:"scope"`
	TotalTasks       int    `json:"total_tasks"`
	ScopeTasks       int    `json:"scope_tasks"`
	AcceptedTasks    int    `json:"accepted_tasks"`
	CancelledTasks   int    `json:"cancelled_tasks"`
	RunningTasks     int    `json:"running_tasks"`
	WaitingTasks     int    `json:"waiting_tasks"`
	ReviewTasks      int    `json:"review_tasks"`
	BlockedTasks     int    `json:"blocked_tasks"`
	ReconcilingTasks int    `json:"reconciling_tasks"`
}

type OverviewAttentionItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	OwnerID   string `json:"owner_id"`
	OwnerName string `json:"owner_name"`
	RunID     string `json:"run_id"`
	RunState  string `json:"run_state"`
	UpdatedAt string `json:"updated_at"`
	DueDate   string `json:"due_date"`
}

type OverviewAttentionPage struct {
	Kind       string                  `json:"kind"`
	Items      []OverviewAttentionItem `json:"items"`
	Total      int                     `json:"total"`
	NextCursor string                  `json:"next_cursor"`
	AsOf       string                  `json:"as_of"`
	DateBasis  string                  `json:"date_basis"`
}

type OverviewMilestone struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	TargetDate string `json:"target_date"`
	Status     string `json:"status"`
}
type OverviewMilestones struct {
	Items []OverviewMilestone `json:"items"`
	Total int                 `json:"total"`
}

const overviewTaskJoins = ` FROM suma_tasks t
 LEFT JOIN dag_runs r ON r.id=t.run_id AND r.tenant_id=t.tenant_id AND r.project_id=t.project_id
 LEFT JOIN suma_task_plans p ON p.task_id=t.id AND p.tenant_id=t.tenant_id AND p.project_id=t.project_id`
const overviewOpenTask = `t.status NOT IN ('accepted','cancelled')`
const overviewBlocked = `(` + overviewOpenTask + ` AND (t.status='blocked' OR r.state='failed'))`

func (s *Service) overviewDeliveryTx(ctx context.Context, auth *authorizedCommand, project string) (OverviewDelivery, error) {
	out := OverviewDelivery{Scope: "whole_project"}
	if err := assertProjectRead(auth, project); err != nil {
		return out, err
	}
	query := `SELECT COUNT(*),
 COALESCE(SUM(CASE WHEN t.status<>'cancelled' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN t.status='accepted' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN t.status='cancelled' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN ` + overviewOpenTask + ` AND r.state='running' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN ` + overviewOpenTask + ` AND r.state IN ('pending','ready','waiting') THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN t.status='in_review' THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN ` + overviewBlocked + ` THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN ` + overviewOpenTask + ` AND r.state='reconciling' THEN 1 ELSE 0 END),0)` + overviewTaskJoins + ` WHERE t.tenant_id=? AND t.project_id=?`
	err := auth.tx.QueryRowContext(ctx, s.q(query), auth.currentPrincipal.TenantID, project).Scan(&out.TotalTasks, &out.ScopeTasks, &out.AcceptedTasks, &out.CancelledTasks, &out.RunningTasks, &out.WaitingTasks, &out.ReviewTasks, &out.BlockedTasks, &out.ReconcilingTasks)
	return out, err
}

func overviewAttentionPredicate(kind string) (string, error) {
	switch kind {
	case "review":
		return `t.status='in_review'`, nil
	case "blocked":
		return overviewBlocked, nil
	case "reconciling":
		return `(` + overviewOpenTask + ` AND r.state='reconciling')`, nil
	case "overdue":
		return `(` + overviewOpenTask + ` AND p.due_date<>'' AND p.due_date<?)`, nil
	default:
		return "", invalid("无效分页游标")
	}
}

// All preview pages share the overview's authorization transaction. Subsequent
// pages reauthorize and read current facts; they are not immutable snapshots.
func (s *Service) overviewAttentionTx(ctx context.Context, auth *authorizedCommand, project, kind string, limit int, cursor, at string) (OverviewAttentionPage, error) {
	out := OverviewAttentionPage{Kind: kind, Items: []OverviewAttentionItem{}, AsOf: at, DateBasis: "UTC"}
	if err := assertProjectRead(auth, project); err != nil {
		return out, err
	}
	predicate, err := overviewAttentionPredicate(kind)
	if err != nil {
		return out, err
	}
	if limit < 1 || limit > 50 {
		return out, invalid("无效分页游标")
	}
	current := auth.currentPrincipal
	// Bind UTC date too: an overdue cursor cannot silently cross a day boundary.
	today := at[:10]
	collection := "overview-attention/" + kind + "/" + today
	c, err := readIngressHistoryCursor(current, project, collection, cursor)
	if err != nil {
		return out, err
	}
	if cursor != "" {
		if !handoffID(c.Last, false) {
			return out, invalid("无效分页游标")
		}
		if _, err = time.Parse(time.RFC3339Nano, c.At); err != nil {
			return out, invalid("无效分页游标")
		}
	}
	where := ` WHERE t.tenant_id=? AND t.project_id=? AND ` + predicate
	args := []any{current.TenantID, project}
	if kind == "overdue" {
		args = append(args, today)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+overviewTaskJoins+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	if cursor != "" {
		where += ` AND (t.updated_at<? OR (t.updated_at=? AND t.id<?))`
		args = append(args, c.At, c.At, c.Last)
	}
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT t.id,t.title,t.status,t.owner_id,COALESCE(m.title,''),t.run_id,COALESCE(r.state,''),t.updated_at,COALESCE(p.due_date,'')`+overviewTaskJoins+` LEFT JOIN suma_members m ON m.id=t.owner_id AND m.tenant_id=t.tenant_id AND m.project_id=t.project_id`+where+` ORDER BY t.updated_at DESC,t.id DESC LIMIT ?`), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item OverviewAttentionItem
		if err = rows.Scan(&item.ID, &item.Title, &item.Status, &item.OwnerID, &item.OwnerName, &item.RunID, &item.RunState, &item.UpdatedAt, &item.DueDate); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		last := out.Items[limit-1]
		out.NextCursor = ingressHistoryCursor(current, project, collection, last.ID, last.UpdatedAt)
	}
	return out, nil
}

func (s *Service) overviewMilestonesTx(ctx context.Context, auth *authorizedCommand, project string) (OverviewMilestones, error) {
	out := OverviewMilestones{Items: []OverviewMilestone{}}
	if err := assertProjectRead(auth, project); err != nil {
		return out, err
	}
	where := ` FROM suma_planning_periods WHERE tenant_id=? AND project_id=? AND kind='milestones' AND status NOT IN ('completed','cancelled') AND target_date<>''`
	args := []any{auth.currentPrincipal.TenantID, project}
	if err := auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+where), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id,title,target_date,status`+where+` ORDER BY target_date,id LIMIT 6`), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item OverviewMilestone
		if err = rows.Scan(&item.ID, &item.Title, &item.TargetDate, &item.Status); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
