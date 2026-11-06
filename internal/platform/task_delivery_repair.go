package platform

import (
	"context"
	"database/sql"
	"errors"
)

// The task key is unique within a tenant and project, including when an older
// repair cycle has been reopened. Never replace it with the issue's latest cycle.
func (s *Service) taskDeliveryRepairTx(ctx context.Context, auth *authorizedCommand, taskID, testPlanID string) (*TaskDeliveryRepair, error) {
	tenant, project := auth.target.tenantID, auth.target.projectID
	var key issueRepairSnapshotKey
	var raw, issueID, issueTenant, issueProject string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT r.id,r.tenant_id,r.project_id,r.issue_id,r.task_id,r.cycle,r.version,r.closed_run_id,r.snapshot,
		i.id,i.tenant_id,i.project_id FROM suma_issue_repairs r
		JOIN suma_issues i ON i.tenant_id=r.tenant_id AND i.project_id=r.project_id AND i.id=r.issue_id
		WHERE r.tenant_id=? AND r.project_id=? AND r.task_id=? AND i.status NOT IN ('archived','deleted')`), tenant, project, taskID).
		Scan(&key.id, &key.tenant, &key.project, &key.issue, &key.task, &key.cycle, &key.version, &key.closedRun, &raw, &issueID, &issueTenant, &issueProject)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Exact comparisons also protect against aliasing under a case-insensitive collation.
	if key.tenant != tenant || key.project != project || key.task != taskID ||
		issueID != key.issue || issueTenant != tenant || issueProject != project ||
		!handoffID(key.issue, false) || !handoffID(key.id, false) || key.cycle < 1 || key.version < 1 {
		return nil, nil
	}
	repair, err := key.decode(raw)
	if err != nil || repair.Closure != nil && key.closedRun == "" || repair.ReopenedAt != "" && repair.Closure == nil {
		return nil, nil
	}
	state := "active"
	switch {
	case repair.ReopenedAt != "":
		state = "reopened"
	case repair.Closure != nil:
		state = "closed"
	}
	out := &TaskDeliveryRepair{IssueID: issueID, Cycle: key.cycle, State: state}
	if auth.actor.IsBrowser() {
		out.AutomationSource = repair.AutomationSource
	}
	if repair.Regression != nil && testPlanID != "" && repair.Regression.PlanID == testPlanID {
		out.Regression = repair.Regression
	}
	return out, nil
}
