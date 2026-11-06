package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type AutomationRunOriginResult struct {
	ProjectID string               `json:"project_id"`
	RunID     string               `json:"run_id"`
	Origin    *AutomationRunOrigin `json:"origin"`
}

type AutomationRunOrigin struct {
	RuleID       string `json:"rule_id"`
	RuleName     string `json:"rule_name"`
	OccurrenceID string `json:"occurrence_id"`
	Revision     int    `json:"revision"`
	Source       string `json:"source"`
	ScheduledAt  string `json:"scheduled_at"`
}

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
	rows, err := tx.QueryContext(ctx, s.q(`SELECT r.workflow_id,r.workflow_version,o.id,
		COALESCE(o.rule_id,''),COALESCE(o.revision,0),COALESCE(o.source,''),
		COALESCE(o.scheduled_at,''),COALESCE(o.state,''),v.spec
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
	var workflowID, state string
	var workflowVersion int
	var occurrenceID, revisionSpec sql.NullString
	var origin AutomationRunOrigin
	if err := rows.Scan(&workflowID, &workflowVersion, &occurrenceID, &origin.RuleID, &origin.Revision, &origin.Source, &origin.ScheduledAt, &state, &revisionSpec); err != nil {
		return nil, err
	}
	if rows.Next() {
		return nil, errors.New("multiple automation occurrences are bound to the same run")
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !occurrenceID.Valid {
		return nil, nil
	}
	if occurrenceID.String == "" || origin.RuleID == "" || origin.Revision < 1 || state != "started" || !revisionSpec.Valid {
		return nil, errors.New("invalid automation run binding")
	}
	if origin.Source != "cron" && origin.Source != "manual" && origin.Source != "webhook" {
		return nil, errors.New("invalid automation occurrence source")
	}
	if scheduled, err := time.Parse(time.RFC3339, origin.ScheduledAt); err != nil || automationStamp(scheduled) != origin.ScheduledAt {
		return nil, errors.New("invalid automation occurrence time")
	}
	var spec AutomationSpec
	if err := json.Unmarshal([]byte(revisionSpec.String), &spec); err != nil || strings.TrimSpace(spec.Name) == "" || spec.WorkflowID != workflowID || spec.WorkflowVersion != workflowVersion {
		return nil, errors.New("invalid automation rule revision for run")
	}
	origin.OccurrenceID = occurrenceID.String
	origin.RuleName = spec.Name
	return &origin, nil
}
