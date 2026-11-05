package platform

import "context"

// The protected case and package-attempt relationships keep quality issues in
// current task delivery even when a user edits their ordinary issue metadata.
func (s *Service) appendTaskDeliveryQualityDefectsTx(ctx context.Context, auth *authorizedCommand, task Record, out *TaskDeliveryIssues) error {
	scope := ` d JOIN suma_test_plan_runs x ON x.tenant_id=d.tenant_id AND x.project_id=d.project_id AND x.run_id=d.run_id AND x.plan_id=d.plan_id WHERE d.tenant_id=? AND d.project_id=? AND d.run_id=? AND x.task_id=?`
	from := ` FROM (SELECT d.issue_id,d.created_at,'test_case' AS source,d.case_key AS sort_key FROM suma_test_plan_defects` + scope +
		` UNION ALL SELECT d.issue_id,d.created_at,'package_test' AS source,d.node_id AS sort_key FROM suma_test_plan_package_defects` + scope + `) defects`
	tenant, project, run, id := auth.target.tenantID, task.ProjectID, task.RunID, task.ID
	args := []any{tenant, project, run, id, tenant, project, run, id}
	var count int
	if err := auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+from), args...).Scan(&count); err != nil {
		return err
	}
	out.Total += count
	out.Complete = out.Total <= out.Limit
	remaining := out.Limit - len(out.Items)
	if remaining <= 0 || count == 0 {
		return nil
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT issue_id,source`+from+` ORDER BY created_at DESC,source,sort_key,issue_id LIMIT ?`), append(args, remaining)...)
	if err != nil {
		return err
	}
	type qualityIssue struct{ id, source string }
	issues := []qualityIssue{}
	for rows.Next() {
		var item qualityIssue
		if err = rows.Scan(&item.id, &item.source); err != nil {
			rows.Close()
			return err
		}
		issues = append(issues, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range issues {
		issue, err := s.getRecordTx(ctx, auth, task.ProjectID, "issues", item.id)
		if err != nil {
			return err
		}
		ref := TaskDeliveryIssue{IssueID: issue.ID, Title: issue.Title, IssueVersion: issue.Version, IssueStatus: issue.Status, Source: item.source}
		if requirement, found, e := s.lookupIssueRequirementTx(ctx, auth, task.ProjectID, item.id); e != nil {
			return e
		} else if found {
			ref.RequirementID, ref.RequirementVersion = requirement.ID, requirement.Version
		}
		out.Items = append(out.Items, ref)
	}
	return nil
}
