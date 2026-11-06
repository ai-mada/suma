package platform

import (
	"context"
	"database/sql"
)

const (
	automationFailurePauseReason           = "failure_pause_threshold_reached"
	automationFailurePauseThresholdInvalid = "failure_pause_threshold 必须为 0 或 2 到 10 的整数"
)

func validAutomationFailurePauseThreshold(value int) bool {
	return value == 0 || value >= 2 && value <= 10
}

// Check the most recent dispatches in trigger order, including unfinished and
// missing runs. Filtering for failures before applying the limit would turn
// old failures separated by a healthy run into a false consecutive streak.
func (s *Service) automationFailurePauseReached(ctx context.Context, tx *sql.Tx, ref automationRef, rule AutomationRule) (bool, error) {
	threshold := rule.FailurePauseThreshold
	if threshold == 0 {
		return false, nil
	}
	if !validAutomationFailurePauseThreshold(threshold) {
		return false, invalid(automationFailurePauseThresholdInvalid)
	}
	var dispatched, failed int
	err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*),COUNT(CASE WHEN recent.state='failed' THEN 1 END) FROM (
		SELECT r.state FROM suma_automation_occurrences o
		LEFT JOIN dag_runs r ON r.id=o.run_id AND r.tenant_id=o.tenant_id AND r.project_id=o.project_id AND r.workflow_id=? AND r.workflow_version=?
		WHERE o.tenant_id=? AND o.project_id=? AND o.rule_id=? AND o.revision=? AND o.state='started'
		ORDER BY o.created_at DESC,o.id DESC LIMIT ?
	) recent`), rule.WorkflowID, rule.WorkflowVersion, ref.tenant, ref.project, ref.id, rule.Revision, threshold).Scan(&dispatched, &failed)
	return dispatched == threshold && failed == threshold, err
}

// Admission has already rechecked the streak in this evaluation transaction.
// Commit the rule state and all queued occurrence reasons together. The narrow
// evaluation proof cannot start, stop, or modify an existing DAG run.
func (s *Service) pauseAutomationAfterFailures(ctx context.Context, auth *authorizedCommand, ref automationRef, rule AutomationRule) error {
	if auth == nil || auth.activationDenied || auth.actor.kind != actorDurableAutomation || auth.actor.delegation.recordID != ref.id {
		return forbidden()
	}
	target := commandTarget{tenantID: ref.tenant, projectID: ref.project, resourceID: ref.id}
	if err := auth.assertTx(auth.tx, target, purposeAutomationEvaluate); err != nil {
		return err
	}
	at := automationStamp(auth.checkedAt)
	result, err := auth.tx.ExecContext(ctx, s.q(`UPDATE suma_automations SET state='paused',version=version+1,next_at='',last_error=?,updated_at=?
		WHERE tenant_id=? AND project_id=? AND id=? AND state='enabled' AND revision=? AND version=? AND activated_by=?`),
		automationFailurePauseReason, at, ref.tenant, ref.project, ref.id, rule.Revision, rule.Version, rule.ActivatedBy)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return conflict("自动化规则版本已变化")
	}
	_, err = auth.tx.ExecContext(ctx, s.q(`UPDATE suma_automation_occurrences SET state='blocked',reason=?,updated_at=?
		WHERE tenant_id=? AND project_id=? AND rule_id=? AND state='queued'`), automationFailurePauseReason, at, ref.tenant, ref.project, ref.id)
	return err
}
