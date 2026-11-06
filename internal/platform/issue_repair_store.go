package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

const (
	issueRepairInvalid      = "Invalid issue repair request"
	issueRepairVersion      = "The issue or repair cycle has changed; reload before deciding"
	issueRepairState        = "The current issue repair cycle does not allow this action"
	issueRepairProtected    = "Issue lifecycle and repair evidence require dedicated repair commands"
	issueRepairRegression   = "Bind current published regression cases, including the original failed case and selector"
	issueRepairGate         = "Current accepted repair and complete passing regression evidence are required"
	issueRepairSource       = "The protected original defect source is unavailable or inconsistent"
	issueRepairHistoryLimit = 10
)

func (s *Service) issueRepairLatestTx(ctx context.Context, auth *authorizedCommand, project, issue string) (*IssueRepair, error) {
	return s.issueRepairLatestReadTx(ctx, auth.tx, auth.target.tenantID, project, issue, false)
}

func (s *Service) issueRepairLatestReadTx(ctx context.Context, tx *sql.Tx, tenant, project, issue string, current bool) (*IssueRepair, error) {
	var id, task, closedRun, raw string
	var cycle, version int
	err := tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT id,task_id,cycle,version,closed_run_id,snapshot FROM suma_issue_repairs WHERE tenant_id=? AND project_id=? AND issue_id=? ORDER BY cycle DESC LIMIT 1`, current)), tenant, project, issue).Scan(&id, &task, &cycle, &version, &closedRun, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out IssueRepair
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if out.ID != id || out.TaskID != task || out.Cycle != cycle || out.Version != version || out.TenantID != tenant || out.ProjectID != project || out.IssueID != issue {
		return nil, conflict(issueRepairSource)
	}
	if out.Closure == nil && closedRun != "" || out.Closure != nil && out.Closure.RunID != closedRun {
		return nil, conflict(issueRepairSource)
	}
	return &out, nil
}

func (s *Service) saveIssueRepairTx(ctx context.Context, auth *authorizedCommand, repair IssueRepair, original int) error {
	if err := assertProjectCommand(auth.tx, auth, repair.ProjectID); err != nil {
		return err
	}
	closedRun := ""
	if repair.Closure != nil {
		closedRun = repair.Closure.RunID
	}
	if original == 0 {
		_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_issue_repairs(id,tenant_id,project_id,issue_id,cycle,version,task_id,closed_run_id,snapshot)VALUES(?,?,?,?,?,?,?,?,?)`), repair.ID, repair.TenantID, repair.ProjectID, repair.IssueID, repair.Cycle, repair.Version, repair.TaskID, closedRun, stringify(repair))
		return err
	}
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_issue_repairs SET version=?,closed_run_id=?,snapshot=? WHERE tenant_id=? AND project_id=? AND issue_id=? AND id=? AND version=?`), repair.Version, closedRun, stringify(repair), repair.TenantID, repair.ProjectID, repair.IssueID, repair.ID, original)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return conflict(issueRepairVersion)
	}
	return err
}

func (s *Service) updateIssueRepairStateTx(ctx context.Context, auth *authorizedCommand, issue *Record, status string) error {
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_issues SET status=?,version=version+1,updated_at=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), status, now(), issue.TenantID, issue.ProjectID, issue.ID, issue.Version)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return conflict(issueRepairVersion)
	}
	issue.Status, issue.Version, issue.UpdatedAt = status, issue.Version+1, now()
	return nil
}

// Only the protected plan/run/case relation establishes an originating failed
// test. Issue metadata is intentionally ignored, including a forged origin tag.
func (s *Service) issueRepairOriginalCaseTx(ctx context.Context, auth *authorizedCommand, project, issue string) (*QualityTestCase, error) {
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT plan_id,run_id,case_key FROM suma_test_plan_defects WHERE tenant_id=? AND project_id=? AND issue_id=?`), auth.target.tenantID, project, issue)
	if err != nil {
		return nil, err
	}
	type origin struct{ plan, run, key string }
	origins := []origin{}
	for rows.Next() {
		var item origin
		if err = rows.Scan(&item.plan, &item.run, &item.key); err != nil {
			rows.Close()
			return nil, err
		}
		origins = append(origins, item)
		if len(origins) > 1 {
			rows.Close()
			return nil, conflict(issueRepairSource)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(origins) == 0 {
		return nil, err
	}
	original := origins[0]
	fixed, found, err := s.testPlanRunTx(ctx, auth.tx, auth.target.tenantID, project, original.run)
	if err != nil {
		return nil, err
	}
	if !found || fixed.PlanID != original.plan || fixed.Revision == nil {
		return nil, conflict(issueRepairSource)
	}
	for _, item := range fixed.Revision.Cases {
		if item.Key == original.key {
			return &item, nil
		}
	}
	return nil, conflict(issueRepairSource)
}

func (s *Service) issueRepairRegressionTx(ctx context.Context, auth *authorizedCommand, project string, repair IssueRepair) (testPlanAggregate, TestPlanRevision, error) {
	var empty TestPlanRevision
	if repair.Regression == nil {
		return testPlanAggregate{}, empty, conflict(issueRepairRegression)
	}
	current, err := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, repair.Regression.PlanID)
	if err != nil {
		return current, empty, err
	}
	if current.TaskID != repair.TaskID || current.State == "retired" || current.Published != repair.Regression.PlanRevision {
		return current, empty, conflict(issueRepairRegression)
	}
	revision, err := s.testPlanRevisionTx(ctx, auth.tx, auth.target.tenantID, project, current.ID, current.Published)
	if err != nil {
		return current, revision, err
	}
	if revision.SHA256 != repair.Regression.PlanSHA256 {
		return current, revision, conflict(issueRepairRegression)
	}
	valid, err := s.testPlanSourceCurrentTx(ctx, auth.tx, auth.target.tenantID, project, revision.Source)
	if err != nil {
		return current, revision, err
	}
	if !valid {
		return current, revision, conflict(issueRepairRegression)
	}
	return current, revision, nil
}
