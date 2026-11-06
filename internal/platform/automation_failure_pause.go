package platform

import (
	"context"
	"encoding/json"
	"strings"
)

const (
	automationFailurePauseReason           = "failure_pause_threshold_reached"
	automationFailurePauseThresholdInvalid = "failure_pause_threshold 必须为 0 或 2 到 10 的整数"
	automationFailurePausedInboxKind       = "automation_failure_paused"
	automationPauseInboxEventPrefix        = "automation-pause:"
)

// The bounded inbox page locates the historical occurrence by its event key
// and reads the title from that occurrence's pinned rule revision.
const automationPauseInboxJoins = ` LEFT JOIN suma_automation_occurrences apo
	ON n.kind='` + automationFailurePausedInboxKind + `' AND apo.tenant_id=n.tenant_id AND apo.project_id=n.project_id
	AND apo.rule_id=n.source_resource_id AND apo.id=SUBSTR(n.source_event_key,LENGTH('` + automationPauseInboxEventPrefix + `')+1)
	AND apo.state='blocked' AND apo.reason='` + automationFailurePauseReason + `' AND apo.run_id=''
	LEFT JOIN suma_automations apr ON apr.tenant_id=n.tenant_id AND apr.project_id=n.project_id AND apr.id=apo.rule_id
	LEFT JOIN suma_automation_revisions apv ON apv.rule_id=apr.id AND apv.revision=apo.revision`

const automationPauseInboxColumns = `,COALESCE(apo.tenant_id,''),COALESCE(apo.project_id,''),COALESCE(apo.id,''),COALESCE(apo.rule_id,''),COALESCE(apo.revision,0),
	COALESCE(apo.state,''),COALESCE(apo.reason,''),COALESCE(apo.run_id,''),COALESCE(apo.updated_at,''),
	COALESCE(apr.tenant_id,''),COALESCE(apr.project_id,''),COALESCE(apr.id,''),COALESCE(apv.rule_id,''),COALESCE(apv.spec,'')`

type automationPauseInboxSource struct {
	tenant, project                 string
	ruleTenant, ruleProject, ruleID string
	occurrence                      AutomationOccurrence
	revisionRuleID, spec            string
}

func (source automationPauseInboxSource) target(item NotificationInboxItem, tenant, project, eventKey string) *NotificationInboxAutomationTarget {
	o := source.occurrence
	// Confirm exact identities after the scoped join, including on databases
	// whose collation compares identifiers and event prefixes case-insensitively.
	if source.tenant != tenant || source.project != project || source.ruleTenant != tenant || source.ruleProject != project ||
		source.ruleID != item.SourceResourceID || o.RuleID != source.ruleID || source.revisionRuleID != source.ruleID || !handoffID(o.ID, false) || o.Revision < 1 ||
		o.State != "blocked" || o.Reason != automationFailurePauseReason || o.RunID != "" || o.UpdatedAt != item.CreatedAt ||
		eventKey != automationPauseInboxEventPrefix+o.ID {
		return nil
	}
	var spec AutomationSpec
	if json.Unmarshal([]byte(source.spec), &spec) != nil || spec.Name == "" || spec.Name != strings.TrimSpace(spec.Name) || len(spec.Name) > 200 ||
		spec.FailurePauseThreshold == 0 || !validAutomationFailurePauseThreshold(spec.FailurePauseThreshold) {
		return nil
	}
	return &NotificationInboxAutomationTarget{RuleID: source.ruleID, Revision: o.Revision, TriggerID: o.ID, Title: spec.Name}
}

func validAutomationFailurePauseThreshold(value int) bool {
	return value == 0 || value >= 2 && value <= 10
}

// Admission has already rechecked the streak in this evaluation transaction.
// Commit the rule state, queued occurrence reasons, evidence and private
// notification together. The evaluation proof grants no DAG writes.
func (s *Service) pauseAutomationAfterFailures(ctx context.Context, auth *authorizedCommand, ref automationRef, rule AutomationRule, occurrence AutomationOccurrence, failedRuns []AutomationFailurePauseRun) error {
	if auth == nil || auth.activationDenied || auth.actor.kind != actorDurableAutomation || auth.actor.delegation.recordID != ref.id {
		return forbidden()
	}
	target := commandTarget{tenantID: ref.tenant, projectID: ref.project, resourceID: ref.id}
	if err := auth.assertTx(auth.tx, target, purposeAutomationEvaluate); err != nil {
		return err
	}
	if auth.currentPrincipal.ID == "" || auth.currentPrincipal.ID != rule.ActivatedBy {
		return forbidden()
	}
	if rule.ID != ref.id || occurrence.RuleID != ref.id || occurrence.Revision != rule.Revision || occurrence.State != "queued" || occurrence.RunID != "" || !handoffID(occurrence.ID, false) {
		return conflict("自动化触发已被处理")
	}
	if !validAutomationFailurePauseRuns(failedRuns, rule.FailurePauseThreshold, occurrence.ID) {
		return conflict("自动化触发已被处理")
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
	if err != nil {
		return err
	}
	// Only the exact occurrence admitted in this transaction may be the
	// source, even though all pending occurrences were blocked together.
	var blockedID string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_automation_occurrences
		WHERE tenant_id=? AND project_id=? AND rule_id=? AND id=? AND revision=? AND state='blocked' AND reason=? AND updated_at=? AND run_id=''`),
		ref.tenant, ref.project, ref.id, occurrence.ID, rule.Revision, automationFailurePauseReason, at).Scan(&blockedID)
	if err != nil {
		return err
	}
	if blockedID != occurrence.ID {
		return conflict("自动化触发已被处理")
	}
	if err = s.insertAutomationFailurePauseEvidenceTx(ctx, auth.tx, ref, rule, occurrence.ID, at, failedRuns); err != nil {
		return err
	}
	_, err = s.insertNotificationInboxSourceTx(ctx, auth.tx, ref.tenant, ref.project, at, notificationInboxSource{
		Kind: automationFailurePausedInboxKind, ResourceID: ref.id,
		EventKey: automationPauseInboxEventPrefix + occurrence.ID, RecipientUserID: rule.ActivatedBy,
	})
	return err
}
