package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) planningCursorTx(ctx context.Context, tx *sql.Tx, tenant, project, table, id string) (string, error) {
	if !handoffID(id, false) {
		return "", invalid(planningInvalidRequest)
	}
	var stamp string
	err := tx.QueryRowContext(ctx, s.q(`SELECT created_at FROM `+table+` WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&stamp)
	if errors.Is(err, sql.ErrNoRows) {
		return "", missing()
	}
	return stamp, err
}

func (s *Service) ListPlanningAnalyses(ctx context.Context, actor Actor, project string, limit int, before string) (page PlanningAnalysisPage, err error) {
	page.Items = []PlanningAnalysis{}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || limit < 1 || limit > 100 || !handoffID(before, true) {
		return page, invalid(planningInvalidRequest)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectRead)
	if err != nil {
		return page, err
	}
	defer close()
	tenant := auth.target.tenantID
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_planning_analyses WHERE tenant_id=? AND project_id=?`), tenant, project).Scan(&page.Total); err != nil {
		return page, err
	}
	query := `SELECT payload FROM suma_planning_analyses WHERE tenant_id=? AND project_id=?`
	args := []any{tenant, project}
	if before != "" {
		stamp, e := s.planningCursorTx(ctx, auth.tx, tenant, project, "suma_planning_analyses", before)
		if e != nil {
			return page, e
		}
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, stamp, stamp, before)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return page, err
	}
	records := []planningRecord{}
	for rows.Next() {
		var raw string
		var record planningRecord
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &record); err != nil {
			break
		}
		records = append(records, record)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return page, err
	}
	if len(records) > limit {
		records = records[:limit]
		page.NextCursor = records[len(records)-1].Analysis.ID
	}
	for _, record := range records {
		item, e := s.planningProjectionTx(ctx, auth, record.Analysis)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

func (s *Service) ListPlanningAlerts(ctx context.Context, actor Actor, project, state string, limit int, before string) (page PlanningAlertPage, err error) {
	page.Items = []PlanningAlert{}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || limit < 1 || limit > 100 || !handoffID(before, true) || state != "" && state != "open" && state != "acknowledged" && state != "resolved" {
		return page, invalid(planningInvalidRequest)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectRead)
	if err != nil {
		return page, err
	}
	defer close()
	tenant := auth.target.tenantID
	for _, count := range []struct {
		query string
		args  []any
		out   *int
	}{
		{`SELECT COUNT(*) FROM suma_planning_alerts WHERE tenant_id=? AND project_id=? AND state='open'`, []any{tenant, project}, &page.OpenCount},
		{`SELECT COUNT(*) FROM suma_planning_alerts WHERE tenant_id=? AND project_id=? AND state='open' AND severity='critical'`, []any{tenant, project}, &page.CriticalCount},
	} {
		if err = auth.tx.QueryRowContext(ctx, s.q(count.query), count.args...).Scan(count.out); err != nil {
			return page, err
		}
	}
	query := ` FROM suma_planning_alerts WHERE tenant_id=? AND project_id=?`
	args := []any{tenant, project}
	if state != "" {
		query += ` AND state=?`
		args = append(args, state)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+query), args...).Scan(&page.Total); err != nil {
		return page, err
	}
	if before != "" {
		stamp, e := s.planningCursorTx(ctx, auth.tx, tenant, project, "suma_planning_alerts", before)
		if e != nil {
			return page, e
		}
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, stamp, stamp, before)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT payload`+query+` ORDER BY created_at DESC,id DESC LIMIT ?`), append(args, limit+1)...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var item PlanningAlert
		if err = rows.Scan(&raw); err != nil {
			return page, err
		}
		if err = json.Unmarshal([]byte(raw), &item); err != nil {
			return page, err
		}
		if item.TaskIDs == nil {
			item.TaskIDs = []string{}
		}
		page.Items = append(page.Items, item)
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
		page.NextCursor = page.Items[len(page.Items)-1].ID
	}
	return page, rows.Err()
}

func (s *Service) CommandPlanningAlert(ctx context.Context, actor Actor, project, id string, req PlanningAlertCommandRequest) (out PlanningAlert, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || !planningKey(req.IdempotencyKey) || !planningText(req.Reason, 4000) || req.Action != "acknowledge" && req.Action != "resolve" {
		return out, invalid(planningAlertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"planning_alert": id, "body": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	out, err = s.planningAlertTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if out.Version != req.ExpectedVersion || out.State == "resolved" || req.Action == "acknowledge" && out.State != "open" {
		return out, conflict(planningAlertInvalid)
	}
	old := out.Version
	out.Version++
	out.UpdatedAt = now()
	out.DecisionAt = out.UpdatedAt
	out.DecisionBy = auth.currentPrincipal.ID
	out.DecisionReason = req.Reason
	out.State = "acknowledged"
	if req.Action == "resolve" {
		out.State = "resolved"
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_planning_alerts SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), out.Version, out.State, stringify(out), auth.target.tenantID, project, id, old)
	if err != nil {
		return out, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return out, err
	}
	if n != 1 {
		return out, conflict(planningAlertInvalid)
	}
	if err = s.commandActivity(ctx, auth, "planning.alert_"+req.Action, id, req.Reason); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
