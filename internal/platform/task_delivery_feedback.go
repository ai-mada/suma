package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	issue, err := s.getRecordTx(ctx, auth, project, "issues", original.ID)
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

func (s *Service) taskDeliveryIssuesTx(ctx context.Context, auth *authorizedCommand, task Record, workflow string) (out TaskDeliveryIssues, err error) {
	out = TaskDeliveryIssues{Scope: "current_run", Limit: taskDeliveryReferenceLimit, Complete: true, Items: []TaskDeliveryIssue{}}
	if task.RunID == "" {
		return out, nil
	}
	var stored, raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT o.request_hash,o.response FROM suma_operations o WHERE o.tenant_id=? AND o.project_id=? AND o.operation_id=? AND EXISTS(SELECT 1 FROM dag_events e WHERE e.tenant_id=o.tenant_id AND e.project_id=o.project_id AND e.run_id=? AND e.event_type='run_failed')`), auth.target.tenantID, task.ProjectID, "failure-"+task.RunID, task.RunID).Scan(&stored, &raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if err == nil {
		body := map[string]any{"title": "执行失败：" + workflow, "description": "运行 " + task.RunID + " 失败，请查看节点证据并分诊。", "status": "open", "priority": "high", "idempotency_key": "failure-" + task.RunID, "data": map[string]any{"run_id": task.RunID, "origin": "execution"}}
		expected := digest("system" + stringify(map[string]any{"kind": "issues", "body": body}))
		ref, e := s.taskDeliveryIssueRefTx(ctx, auth, task.ProjectID, stored, raw, expected, "execution_failure", "")
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
