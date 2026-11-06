package platform

import (
	"context"
	"encoding/json"
)

// The slot is created atomically with its authoritative review. Editable issue
// metadata is helpful context, but only this original receipt establishes lineage.
func rejectedReviewIssueBody(reviewID, taskID, runID, reason string) map[string]any {
	return map[string]any{"title": "Quality review rejected", "description": reason, "status": "open", "priority": "high", "idempotency_key": "review-rejected-" + reviewID, "data": map[string]any{"origin": "review_rejected", "review_id": reviewID, "task_id": taskID, "run_id": runID}}
}

func (s *Service) createRejectedReviewIssueTx(ctx context.Context, auth *authorizedCommand, project, reviewID, taskID, runID, reason string) error {
	_, err := s.createCommandRecordTx(ctx, auth.tx, auth, project, "issues", rejectedReviewIssueBody(reviewID, taskID, runID, reason))
	return err
}

func (s *Service) taskDeliveryIssueRefTx(ctx context.Context, auth *authorizedCommand, project, storedHash, raw, expectedHash, source, review string) (TaskDeliveryIssue, error) {
	var out TaskDeliveryIssue
	if storedHash != expectedHash {
		return out, &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
	}
	var original struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &original); err != nil {
		return out, err
	}
	return s.taskDeliveryIssueByIDTx(ctx, auth, project, original.ID, source, review)
}

func (s *Service) taskDeliveryIssueByIDTx(ctx context.Context, auth *authorizedCommand, project, issueID, source, review string) (TaskDeliveryIssue, error) {
	var out TaskDeliveryIssue
	issue, err := s.getRecordTx(ctx, auth, project, "issues", issueID)
	if err != nil {
		return out, err
	}
	out = TaskDeliveryIssue{IssueID: issue.ID, Title: issue.Title, IssueVersion: issue.Version, IssueStatus: issue.Status, Source: source, SourceReviewID: review}
	if requirement, found, err := s.lookupIssueRequirementTx(ctx, auth, project, issue.ID); err != nil {
		return out, err
	} else if found {
		out.RequirementID, out.RequirementVersion = requirement.ID, requirement.Version
	}
	return out, nil
}

func (s *Service) taskDeliveryIssuesTx(ctx context.Context, auth *authorizedCommand, task Record) (out TaskDeliveryIssues, err error) {
	out = TaskDeliveryIssues{Scope: "current_run", Limit: taskDeliveryReferenceLimit, Complete: true, Items: []TaskDeliveryIssue{}}
	if task.RunID == "" {
		return out, nil
	}
	issueID, found, err := s.failureIssueIDTx(ctx, auth.tx, auth.target.tenantID, task.ProjectID, task.RunID)
	if err != nil {
		return out, err
	}
	if found {
		ref, e := s.taskDeliveryIssueByIDTx(ctx, auth, task.ProjectID, issueID, "execution_failure", "")
		if e != nil {
			return out, e
		}
		out.Total++
		out.Items = append(out.Items, ref)
	}
	joinSlot := `('review-rejected-' || r.id)`
	if s.Dialect == "oceanbase" {
		joinSlot = `CONCAT('review-rejected-',r.id)`
	}
	from := ` FROM suma_reviews r JOIN suma_operations o ON o.tenant_id=r.tenant_id AND o.project_id=r.project_id AND o.operation_id=` + joinSlot + ` WHERE r.tenant_id=? AND r.project_id=? AND r.task_id=? AND r.run_id=? AND r.decision='rejected'`
	var reviewCount int
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+from), auth.target.tenantID, task.ProjectID, task.ID, task.RunID).Scan(&reviewCount); err != nil {
		return out, err
	}
	out.Total += reviewCount
	out.Complete = out.Total <= out.Limit
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT r.id,r.reviewer_id,r.reason,o.request_hash,o.response`+from+` ORDER BY r.created_at DESC,r.id LIMIT ?`), auth.target.tenantID, task.ProjectID, task.ID, task.RunID, out.Limit-len(out.Items))
	if err != nil {
		return out, err
	}
	type item struct{ review, reviewer, reason, hash, raw string }
	items := []item{}
	for rows.Next() {
		var row item
		if err = rows.Scan(&row.review, &row.reviewer, &row.reason, &row.hash, &row.raw); err != nil {
			rows.Close()
			return out, err
		}
		items = append(items, row)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, row := range items {
		expected := digest(row.reviewer + stringify(map[string]any{"kind": "issues", "body": rejectedReviewIssueBody(row.review, task.ID, task.RunID, row.reason)}))
		ref, e := s.taskDeliveryIssueRefTx(ctx, auth, task.ProjectID, row.hash, row.raw, expected, "review_rejected", row.review)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, ref)
	}
	return out, nil
}
