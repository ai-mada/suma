package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

func (s *Service) issueRepairSelectedCasesTx(ctx context.Context, auth *authorizedCommand, project, issue string, repair IssueRepair, revision TestPlanRevision) error {
	if repair.Regression == nil || len(repair.Regression.CaseKeys) == 0 {
		return conflict(issueRepairRegression)
	}
	original, err := s.issueRepairOriginalCaseTx(ctx, auth, project, issue)
	if err != nil {
		return err
	}
	definitions := map[string]QualityTestCase{}
	for _, item := range revision.Cases {
		definitions[item.Key] = item
	}
	containsOriginal := original == nil
	seen := map[string]bool{}
	for _, key := range repair.Regression.CaseKeys {
		item, found := definitions[key]
		if !found || seen[key] {
			return conflict(issueRepairRegression)
		}
		seen[key] = true
		if original != nil && key == original.CaseKey && item.Selector == original.selector {
			containsOriginal = true
		}
	}
	if !containsOriginal {
		return conflict(issueRepairRegression)
	}
	return nil
}

// Closure readiness uses the existing evidence authority, never a supplied
// "passed" flag. The caller holds current identity and project coordination.
func (s *Service) issueRepairClosureTx(ctx context.Context, auth *authorizedCommand, project string, repair IssueRepair, runID string, packageOrigin *issueRepairPackageOrigin) (*IssueRepairClosure, []IssueRepairBlocker, error) {
	blockers := []IssueRepairBlocker{}
	add := func(code, ref string) { blockers = append(blockers, IssueRepairBlocker{Code: code, RefID: ref}) }
	task, err := s.getRecordTx(ctx, auth, project, "tasks", repair.TaskID)
	if err != nil {
		return nil, blockers, err
	}
	if task.Status != "accepted" {
		add("repair_task_not_accepted", task.ID)
	}
	if runID == "" || task.RunID != runID || task.Data["verified_run_id"] != runID {
		add("repair_run_changed", runID)
	}
	if task.RequirementID != repair.RequirementID {
		add("repair_evidence_stale", task.ID)
	}
	// A new cycle always has a new task. Also reject reuse of a prior closure's
	// run explicitly, even if later task metadata were corrupted independently.
	if runID != "" {
		var prior int
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_issue_repairs WHERE tenant_id=? AND project_id=? AND issue_id=? AND cycle<? AND closed_run_id=?`), auth.target.tenantID, project, repair.IssueID, repair.Cycle, runID).Scan(&prior)
		if err != nil {
			return nil, blockers, err
		}
		if prior != 0 {
			add("repair_run_changed", runID)
		}
	}
	if task.Status == "accepted" && task.RunID == runID && runID != "" {
		if err = s.checkCurrentTaskEvidenceTx(ctx, auth.tx, auth.currentPrincipal, project, task.ID, map[string]bool{}, map[string]bool{}); err != nil {
			var api *APIError
			if !errors.As(err, &api) {
				return nil, blockers, err
			}
			add("repair_evidence_stale", task.ID)
		}
		if err = s.checkTaskRequirementDesignRunTx(ctx, auth, project, task.ID, runID, true); err != nil {
			var api *APIError
			if !errors.As(err, &api) {
				return nil, blockers, err
			}
			add("repair_evidence_stale", task.ID)
		}
		if err = s.checkCandidateCompositionAcceptanceTx(ctx, auth.tx, auth.currentPrincipal, project, runID); err != nil {
			var api *APIError
			if !errors.As(err, &api) {
				return nil, blockers, err
			}
			add("repair_evidence_stale", task.ID)
		}
	}
	var review string
	if task.Version > 1 && runID != "" {
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? AND task_version=? AND decision='accepted' ORDER BY created_at DESC,id DESC LIMIT 1`), auth.target.tenantID, project, task.ID, runID, task.Version-1).Scan(&review)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, blockers, err
		}
	}
	if review == "" {
		add("formal_review_missing", task.ID)
	}
	if repair.Regression == nil {
		add("regression_not_bound", repair.ID)
		return nil, blockers, nil
	}
	plan, revision, err := s.issueRepairRegressionTx(ctx, auth, project, repair)
	if err != nil {
		var api *APIError
		if !errors.As(err, &api) {
			return nil, blockers, err
		}
		add("regression_plan_changed", repair.Regression.PlanID)
		return nil, blockers, nil
	}
	if err = s.issueRepairSelectedCasesTx(ctx, auth, project, repair.IssueID, repair, revision); err != nil {
		var api *APIError
		if !errors.As(err, &api) {
			return nil, blockers, err
		}
		add("original_case_required", repair.IssueID)
		return nil, blockers, nil
	}
	if task.RunID == "" || runID != task.RunID {
		return nil, blockers, nil
	}
	fixed, found, err := s.testPlanRunTx(ctx, auth.tx, auth.target.tenantID, project, runID)
	if err != nil {
		return nil, blockers, err
	}
	if !found || fixed.TaskID != task.ID || fixed.PlanID != plan.ID || fixed.Revision == nil || fixed.Revision.Revision != revision.Revision || fixed.Revision.SHA256 != revision.SHA256 {
		add("regression_plan_changed", plan.ID)
		return nil, blockers, nil
	}
	execution, err := s.testPlanExecutionTx(ctx, auth.tx, auth.target.tenantID, project, plan, fixed, runID)
	if err != nil {
		return nil, blockers, err
	}
	if !execution.BasisCurrent || execution.State != "passed" {
		add("repair_evidence_stale", runID)
	}
	cases := map[string]TestCaseResult{}
	for _, item := range execution.Cases {
		cases[item.CaseKey] = item
	}
	for _, key := range repair.Regression.CaseKeys {
		item, ok := cases[key]
		if !ok || item.Status != "passed" || len(item.Evidence) == 0 {
			add("regression_case_failed", key)
			continue
		}
		for _, evidence := range item.Evidence {
			valid, checkErr := s.codeCandidateIndependentTestTx(ctx, auth, project, runID, evidence)
			if checkErr != nil {
				return nil, blockers, checkErr
			}
			if !valid {
				add("repair_evidence_stale", evidence.NodeID)
			}
		}
	}
	var packageRegression *IssueRepairPackageProof
	if packageOrigin != nil {
		var blocker string
		packageRegression, blocker, err = s.issueRepairPackageRegressionTx(ctx, auth, project, packageOrigin, fixed, execution)
		if err != nil {
			return nil, blockers, err
		}
		if packageRegression == nil {
			add(blocker, runID)
		}
	}
	if len(blockers) != 0 {
		return nil, blockers, nil
	}
	return &IssueRepairClosure{RunID: runID, TaskVersion: task.Version, ReviewID: review, PlanID: plan.ID, PlanRevision: revision.Revision, PlanSHA256: revision.SHA256, CaseKeys: append([]string{}, repair.Regression.CaseKeys...), SourceCommits: append([]string{}, execution.SourceCommits...), PackageRegression: packageRegression}, blockers, nil
}

func (s *Service) issueRepairViewTx(ctx context.Context, auth *authorizedCommand, project, issueID string) (out IssueRepairView, err error) {
	out = IssueRepairView{Cycle: 1, History: []IssueRepair{}, HistoryComplete: true, Blockers: []IssueRepairBlocker{}}
	out.Issue, err = s.getRecordTx(ctx, auth, project, "issues", issueID)
	if err != nil {
		return out, err
	}
	caseOrigin, err := s.issueRepairOriginalCaseTx(ctx, auth, project, issueID)
	if err != nil {
		return out, err
	}
	if caseOrigin != nil {
		out.OriginalCase = &caseOrigin.IssueRepairCaseOrigin
	}
	packageOrigin, err := s.issueRepairOriginalPackageTx(ctx, auth, project, issueID)
	if err != nil {
		return out, err
	}
	if packageOrigin != nil {
		out.OriginalPackage = &packageOrigin.IssueRepairPackageOrigin
	}
	latest, err := s.issueRepairLatestTx(ctx, auth, project, issueID)
	if err != nil {
		return out, err
	}
	if latest != nil {
		out.Cycle = latest.Cycle
		if latest.ReopenedAt == "" {
			out.Current = latest
		} else {
			out.Cycle++
		}
	}
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_issue_repairs WHERE tenant_id=? AND project_id=? AND issue_id=?`), auth.target.tenantID, project, issueID).Scan(&out.HistoryTotal)
	if err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT snapshot FROM suma_issue_repairs WHERE tenant_id=? AND project_id=? AND issue_id=? ORDER BY cycle DESC LIMIT ?`), auth.target.tenantID, project, issueID, issueRepairHistoryLimit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var raw string
		var item IssueRepair
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return out, err
		}
		if err = json.Unmarshal([]byte(raw), &item); err != nil {
			rows.Close()
			return out, err
		}
		if item.TenantID != auth.target.tenantID || item.ProjectID != project || item.IssueID != issueID {
			rows.Close()
			return out, conflict(issueRepairSource)
		}
		if out.Current != nil && item.ID == out.Current.ID {
			continue
		}
		if len(out.History) < issueRepairHistoryLimit {
			out.History = append(out.History, item)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if out.Current != nil {
		out.HistoryTotal--
	}
	out.HistoryComplete = len(out.History) == out.HistoryTotal
	write, approve := false, false
	if _, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, false); err == nil {
		write = true
	} else {
		var api *APIError
		if !errors.As(err, &api) {
			return out, err
		}
	}
	if auth.actor.kind == actorBrowser {
		if _, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, project, true, true); err == nil {
			approve = true
		} else {
			var api *APIError
			if !errors.As(err, &api) {
				return out, err
			}
		}
	}
	out.CanPrepare = write && out.Issue.Status == "open" && out.Current == nil
	if out.Current == nil {
		out.Blockers = append(out.Blockers, IssueRepairBlocker{Code: "repair_not_prepared"})
		return out, nil
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", out.Current.TaskID)
	if err != nil {
		return out, err
	}
	out.Task = &task
	if out.Issue.Status == "closed" {
		out.CanReopen = approve && out.Current.Closure != nil
		out.Blockers = append(out.Blockers, IssueRepairBlocker{Code: "issue_closed", RefID: issueID})
		if write && out.Current.Closure != nil && out.Current.Closure.PackageRegression != nil {
			out.ReleaseHandoff, err = s.issueRepairReleaseHandoffTx(ctx, auth.tx, auth.currentPrincipal, project, issueID, true)
			var api *APIError
			if err != nil && (!errors.As(err, &api) || api.Status != 404 && api.Status != 409) {
				return out, err
			}
		}
		return out, nil
	}
	out.CanBind = write && out.Issue.Status == "in_progress" && out.Current.Closure == nil
	_, blockers, err := s.issueRepairClosureTx(ctx, auth, project, *out.Current, task.RunID, packageOrigin)
	if err != nil {
		return out, err
	}
	out.Blockers = append(out.Blockers, blockers...)
	out.CanClose = approve && out.Issue.Status == "in_progress" && out.Current.Closure == nil && len(out.Blockers) == 0
	return out, nil
}
