package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"
)

type AutomationRunOriginResult struct {
	ProjectID string               `json:"project_id"`
	RunID     string               `json:"run_id"`
	Origin    *AutomationRunOrigin `json:"origin"`
}

type AutomationRunOrigin struct {
	RuleID       string                     `json:"rule_id"`
	RuleName     string                     `json:"rule_name"`
	OccurrenceID string                     `json:"occurrence_id"`
	Revision     int                        `json:"revision"`
	Source       string                     `json:"source"`
	ScheduledAt  string                     `json:"scheduled_at"`
	FailurePause *AutomationRunFailurePause `json:"failure_pause,omitempty" wire:"nonnull"`
}

type AutomationRunFailurePause struct {
	OccurrenceID string `json:"occurrence_id"`
	ObservedAt   string `json:"observed_at"`
	Threshold    int    `json:"threshold"`
}

// An invalid origin is distinguishable from a query failure when an already
// verified run is being enriched with optional automation provenance.
type invalidAutomationRunOrigin string

func (err invalidAutomationRunOrigin) Error() string { return string(err) }

func (s *Service) AutomationRunOrigin(ctx context.Context, actor Actor, project, runID string) (AutomationRunOriginResult, error) {
	var out AutomationRunOriginResult
	if project == "" || project == organizationScope {
		return out, invalid("请选择项目")
	}
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: runID}
	err := s.withCurrentRead(ctx, actor, target, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		origin, err := s.automationRunOriginCurrent(auth.ctx, auth.tx, auth.currentPrincipal.TenantID, project, runID)
		if err != nil {
			return err
		}
		out = AutomationRunOriginResult{ProjectID: project, RunID: runID, Origin: origin}
		return nil
	})
	if err != nil {
		return AutomationRunOriginResult{}, err
	}
	return out, nil
}

func (s *Service) automationRunOriginCurrent(ctx context.Context, tx *sql.Tx, tenant, project, runID string) (*AutomationRunOrigin, error) {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT r.id,r.tenant_id,r.project_id,r.workflow_id,r.workflow_version,o.id,
		COALESCE(o.rule_id,''),COALESCE(o.revision,0),COALESCE(o.source,''),
		COALESCE(o.scheduled_at,''),COALESCE(o.state,''),v.spec,
		COALESCE(o.tenant_id,''),COALESCE(o.project_id,''),COALESCE(o.run_id,''),
		COALESCE(a.id,''),COALESCE(a.tenant_id,''),COALESCE(a.project_id,''),COALESCE(v.rule_id,'')
		FROM dag_runs r
		LEFT JOIN suma_automation_occurrences o ON o.tenant_id=r.tenant_id AND o.project_id=r.project_id AND o.run_id=r.id
		LEFT JOIN suma_automations a ON a.tenant_id=o.tenant_id AND a.project_id=o.project_id AND a.id=o.rule_id
		LEFT JOIN suma_automation_revisions v ON v.rule_id=a.id AND v.revision=o.revision
		WHERE r.tenant_id=? AND r.project_id=? AND r.id=? LIMIT 2`), tenant, project, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, missing()
	}
	var storedRunID, storedTenant, storedProject, workflowID, state string
	var occurrenceTenant, occurrenceProject, occurrenceRunID, ruleID, ruleTenant, ruleProject, revisionRuleID string
	var workflowVersion int
	var occurrenceID, revisionSpec sql.NullString
	var origin AutomationRunOrigin
	if err := rows.Scan(&storedRunID, &storedTenant, &storedProject, &workflowID, &workflowVersion, &occurrenceID,
		&origin.RuleID, &origin.Revision, &origin.Source, &origin.ScheduledAt, &state, &revisionSpec,
		&occurrenceTenant, &occurrenceProject, &occurrenceRunID, &ruleID, &ruleTenant, &ruleProject, &revisionRuleID); err != nil {
		return nil, err
	}
	if rows.Next() {
		return nil, invalidAutomationRunOrigin("multiple automation occurrences are bound to the same run")
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	// Scope comparisons are exact even under a case-insensitive collation.
	if storedRunID != runID || storedTenant != tenant || storedProject != project {
		return nil, missing()
	}
	if !occurrenceID.Valid {
		return nil, nil
	}
	if !handoffID(occurrenceID.String, false) || !handoffID(origin.RuleID, false) || origin.Revision < 1 || state != "started" || !revisionSpec.Valid ||
		occurrenceTenant != tenant || occurrenceProject != project || occurrenceRunID != runID ||
		ruleID != origin.RuleID || ruleTenant != tenant || ruleProject != project || revisionRuleID != origin.RuleID {
		return nil, invalidAutomationRunOrigin("invalid automation run binding")
	}
	if origin.Source != "cron" && origin.Source != "manual" && origin.Source != "webhook" {
		return nil, invalidAutomationRunOrigin("invalid automation occurrence source")
	}
	if scheduled, err := time.Parse(time.RFC3339, origin.ScheduledAt); err != nil || automationStamp(scheduled) != origin.ScheduledAt {
		return nil, invalidAutomationRunOrigin("invalid automation occurrence time")
	}
	var spec AutomationSpec
	if err := json.Unmarshal([]byte(revisionSpec.String), &spec); err != nil || strings.TrimSpace(spec.Name) == "" || spec.WorkflowID != workflowID || spec.WorkflowVersion != workflowVersion {
		return nil, invalidAutomationRunOrigin("invalid automation rule revision for run")
	}
	origin.OccurrenceID = occurrenceID.String
	origin.RuleName = spec.Name
	if spec.FailurePauseThreshold != 0 && validAutomationFailurePauseThreshold(spec.FailurePauseThreshold) {
		origin.FailurePause, err = s.automationRunFailurePauseTx(ctx, tx, automationRef{tenant: tenant, project: project, id: ruleID}, runID, origin)
		if err != nil {
			return nil, err
		}
	}
	return &origin, nil
}

// Resuming a rule after its failure pause requires a new revision. The index
// finds the one expected pause for this revision, and a second row indicates
// ambiguous history. The blocked occurrence and its evidence are then verified.
func (s *Service) automationRunFailurePauseTx(ctx context.Context, tx *sql.Tx, ref automationRef, runID string, origin AutomationRunOrigin) (*AutomationRunFailurePause, error) {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT e.occurrence_id,COALESCE(o.id,''),COALESCE(o.tenant_id,''),COALESCE(o.project_id,''),
		COALESCE(o.rule_id,''),COALESCE(o.revision,0),COALESCE(o.state,''),COALESCE(o.reason,''),COALESCE(o.run_id,''),COALESCE(o.updated_at,'')
		FROM `+automationPauseEvidenceTable+` e
		LEFT JOIN suma_automation_occurrences o ON o.id=e.occurrence_id AND o.tenant_id=e.tenant_id AND o.project_id=e.project_id
			AND o.rule_id=e.rule_id AND o.revision=e.revision
		WHERE e.tenant_id=? AND e.project_id=? AND e.rule_id=? AND e.revision=? LIMIT 2`), ref.tenant, ref.project, ref.id, origin.Revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	var id, tenant, project string
	var blocked AutomationOccurrence
	if err := rows.Scan(&id, &blocked.ID, &tenant, &project, &blocked.RuleID, &blocked.Revision, &blocked.State, &blocked.Reason, &blocked.RunID, &blocked.UpdatedAt); err != nil {
		return nil, err
	}
	if rows.Next() {
		return nil, nil
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if !handoffID(id, false) || blocked.ID != id || tenant != ref.tenant || project != ref.project || blocked.RuleID != ref.id || blocked.Revision != origin.Revision ||
		blocked.State != "blocked" || blocked.Reason != automationFailurePauseReason || blocked.RunID != "" {
		return nil, nil
	}
	if observed, err := time.Parse(time.RFC3339, blocked.UpdatedAt); err != nil || automationStamp(observed) != blocked.UpdatedAt {
		return nil, nil
	}
	evidence, err := s.automationFailurePauseEvidenceTx(ctx, tx, ref, blocked)
	if err != nil || evidence == nil {
		return nil, err
	}
	for _, failed := range evidence.FailedRuns {
		if failed.RunID == runID && failed.TriggerID == origin.OccurrenceID {
			return &AutomationRunFailurePause{OccurrenceID: blocked.ID, ObservedAt: evidence.ObservedAt, Threshold: evidence.Threshold}, nil
		}
	}
	return nil, nil
}
