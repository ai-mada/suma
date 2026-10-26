package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) usageProjectionTx(ctx context.Context, auth *authorizedCommand, r usageRecord) (UsageAttempt, error) {
	clock, e := s.authorityNow(ctx, auth.tx)
	if e != nil {
		return r.Attempt, e
	}
	return s.usageProjectionAtTx(ctx, auth, r, clock.Unix())
}
func (s *Service) usageProjectionAtTx(ctx context.Context, auth *authorizedCommand, r usageRecord, clock int64) (UsageAttempt, error) {
	out := r.Attempt
	out.AllowedActions = []string{}
	var state string
	var lease int64
	e := auth.tx.QueryRowContext(ctx, s.q(`SELECT n.state,n.lease_until FROM dag_nodes n JOIN dag_runs d ON d.id=n.run_id WHERE d.tenant_id=? AND d.project_id=? AND n.run_id=? AND n.id=? AND n.attempt=? AND n.fence=?`), auth.target.tenantID, out.ProjectID, out.RunID, out.NodeID, out.Attempt, out.Fence).Scan(&state, &lease)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	active := state == "running" && lease > clock
	if r.Unresolved && (state == "reconciling" || state == "running" && !active) {
		out.State = "reconciling"
	}
	if r.Unresolved && !active && orgAdmin(auth.currentPrincipal) {
		out.AllowedActions = append(out.AllowedActions, "confirm")
	}
	return out, nil
}
func (s *Service) GetUsageDashboard(ctx context.Context, actor Actor, project, month string) (out UsageDashboard, err error) {
	month, err = usageMonth(month)
	if err != nil || !handoffID(project, false) {
		return out, invalid(usageInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	if month == "" {
		month, err = s.usagePeriodTx(ctx, auth.tx)
		if err != nil {
			return out, err
		}
	}
	out = UsageDashboard{ProjectID: project, PeriodStart: month + "-01", AsOf: now(), Balances: []UsageBalance{}, Rates: []UsageRateRevision{}}
	out.CanManageProject, out.CanManageOrganization, err = s.usageManagementTx(ctx, auth, project)
	if err != nil {
		return out, err
	}
	out.CanManageRates = out.CanManageProject && out.CanManageOrganization
	out.ProjectPolicy, err = s.usagePolicyTx(ctx, auth.tx, auth.target.tenantID, project, "project")
	if err != nil {
		return out, err
	}
	if out.CanManageProject {
		out.ProjectPolicy.AllowedActions = []string{"write"}
	}
	org, err := s.usagePolicyTx(ctx, auth.tx, auth.target.tenantID, project, "organization")
	if err != nil {
		return out, err
	}
	out.OrganizationEnforced = org.Mode == "enforce"
	scopes := []UsagePolicy{out.ProjectPolicy}
	if out.CanManageOrganization {
		org.AllowedActions = []string{"write"}
		out.OrganizationPolicy = &org
		scopes = append(scopes, org)
	}
	for _, policy := range scopes {
		tokens, e := s.usageAccountTx(ctx, auth.tx, auth.target.tenantID, policy.Scope, policy.ProjectID, month, "tokens", false)
		if e != nil {
			return out, e
		}
		balance := UsageBalance{Scope: policy.Scope, ProjectID: policy.ProjectID, PeriodStart: month + "-01", ReportedTokens: usagePublic(tokens.spent), ReservedTokens: usagePublic(tokens.reserved), UnresolvedAttempts: tokens.unresolved, Costs: []UsageMoneyBalance{}}
		for _, currency := range []string{"CNY", "USD"} {
			money, e := s.usageAccountTx(ctx, auth.tx, auth.target.tenantID, policy.Scope, policy.ProjectID, month, "cost:"+currency, false)
			if e != nil {
				return out, e
			}
			if money.version != 0 || currency == policy.Currency {
				balance.Costs = append(balance.Costs, UsageMoneyBalance{Currency: currency, SpentMicros: usagePublic(money.spent), ReservedMicros: usagePublic(money.reserved)})
			}
		}
		out.Balances = append(out.Balances, balance)
	}
	current := ` FROM suma_usage_rates r WHERE r.tenant_id=? AND r.project_id=? AND NOT EXISTS(SELECT 1 FROM suma_usage_rates n WHERE n.tenant_id=r.tenant_id AND n.project_id=r.project_id AND n.tool_profile_id=r.tool_profile_id AND n.version>r.version)`
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+current), auth.target.tenantID, project).Scan(&out.RateTotal); err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT r.payload`+current+` ORDER BY r.created_at DESC,r.id DESC LIMIT 100`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var rate UsageRateRevision
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if err = json.Unmarshal([]byte(raw), &rate); err != nil {
			return out, err
		}
		out.Rates = append(out.Rates, rate)
	}
	return out, rows.Err()
}
func (s *Service) ListUsageAttempts(ctx context.Context, actor Actor, project, month, state string, limit int, before string) (page UsageAttemptPage, err error) {
	page.Items = []UsageAttempt{}
	month, err = usageMonth(month)
	if limit == 0 {
		limit = 20
	}
	states := map[string]bool{"": true, "reserved": true, "settled": true, "partial": true, "unreported": true, "reconciling": true, "confirmed": true, "unpriced": true}
	if err != nil || !handoffID(project, false) || !states[state] || limit < 1 || limit > 100 || !handoffID(before, true) {
		return page, invalid(usageInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectRead)
	if err != nil {
		return page, err
	}
	defer close()
	clock, err := s.authorityNow(ctx, auth.tx)
	if err != nil {
		return page, err
	}
	if month == "" {
		month = clock.Format("2006-01")
	}
	// Derive the same state before filtering, counting and keyset pagination.
	// A prior settled attempt never inherits a later attempt's unknown state.
	effective := `CASE WHEN a.unresolved=1 AND (n.state='reconciling' OR (n.state='running' AND n.lease_until<=?)) THEN 'reconciling' ELSE a.state END`
	from := ` FROM suma_usage_attempts a LEFT JOIN dag_nodes n ON n.run_id=a.run_id AND n.id=a.node_id AND n.attempt=a.attempt AND n.fence=a.fence`
	query := from + ` WHERE a.tenant_id=? AND a.project_id=? AND a.period=?`
	args := []any{auth.target.tenantID, project, month}
	if state != "" {
		query += ` AND (` + effective + `)=?`
		args = append(args, clock.Unix(), state)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+query), args...).Scan(&page.Total); err != nil {
		return page, err
	}
	if before != "" {
		var stamp string
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT a.created_at`+query+` AND a.id=?`), append(append([]any{}, args...), before)...).Scan(&stamp)
		if errors.Is(err, sql.ErrNoRows) {
			return page, invalid(usageInvalid)
		}
		if err != nil {
			return page, err
		}
		query += ` AND (a.created_at<? OR (a.created_at=? AND a.id<?))`
		args = append(args, stamp, stamp, before)
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT a.payload`+query+` ORDER BY a.created_at DESC,a.id DESC LIMIT ?`), append(args, limit+1)...)
	if err != nil {
		return page, err
	}
	records := []usageRecord{}
	for rows.Next() {
		var raw string
		var record usageRecord
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
		page.NextCursor = records[len(records)-1].Attempt.ID
	}
	for _, record := range records {
		item, e := s.usageProjectionAtTx(ctx, auth, record, clock.Unix())
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}
