package platform

import "context"

// Use the protected case/run relationship, never editable issues.data, to join
// a failed case back into its task's current delivery and requirement intake.
func (s *Service) appendTaskDeliveryCaseDefectsTx(ctx context.Context, auth *authorizedCommand, task Record, out *TaskDeliveryIssues) error {
	from := ` FROM suma_test_plan_defects d JOIN suma_test_plan_runs x ON x.tenant_id=d.tenant_id AND x.project_id=d.project_id AND x.run_id=d.run_id AND x.plan_id=d.plan_id WHERE d.tenant_id=? AND d.project_id=? AND d.run_id=? AND x.task_id=?`
	args := []any{auth.target.tenantID, task.ProjectID, task.RunID, task.ID}
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
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT d.issue_id`+from+` ORDER BY d.created_at DESC,d.case_key LIMIT ?`), append(args, remaining)...)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		issue, err := s.getRecordTx(ctx, auth, task.ProjectID, "issues", id)
		if err != nil {
			return err
		}
		ref := TaskDeliveryIssue{IssueID: issue.ID, Title: issue.Title, IssueVersion: issue.Version, IssueStatus: issue.Status, Source: "test_case"}
		if requirement, found, e := s.lookupIssueRequirementTx(ctx, auth, task.ProjectID, id); e != nil {
			return e
		} else if found {
			ref.RequirementID, ref.RequirementVersion = requirement.ID, requirement.Version
		}
		out.Items = append(out.Items, ref)
	}
	return nil
}
