package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

const automationPauseEvidenceTable = "suma_automation_pause_evidence"
const automationPauseEvidenceColumns = "occurrence_id,tenant_id,project_id,rule_id,revision,workflow_id,workflow_version,threshold,observed_at,failed_runs"
const automationPauseEvidenceDDL = `CREATE TABLE IF NOT EXISTS suma_automation_pause_evidence(
	occurrence_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,
	rule_id VARCHAR(64) NOT NULL,revision INTEGER NOT NULL,workflow_id VARCHAR(64) NOT NULL,workflow_version INTEGER NOT NULL,
	threshold INTEGER NOT NULL,observed_at VARCHAR(40) NOT NULL,failed_runs TEXT NOT NULL
)`

type AutomationFailurePauseRun struct {
	TriggerID string `json:"trigger_id"`
	RunID     string `json:"run_id"`
}

type AutomationFailurePauseEvidence struct {
	Threshold  int                         `json:"threshold"`
	ObservedAt string                      `json:"observed_at"`
	FailedRuns []AutomationFailurePauseRun `json:"failed_runs"`
}

// Check an existing table before any initializer writes. An absent independent
// table can be created by normal initialization without synthesizing evidence.
func (s *Service) validateAutomationPauseEvidenceInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: unsupported automation pause evidence dialect", store.ErrInitialSchema)
	}
	var count int
	if err := s.DB.QueryRowContext(ctx, s.q(query), automationPauseEvidenceTable).Scan(&count); err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+automationPauseEvidenceColumns+` FROM `+automationPauseEvidenceTable+` WHERE 1=0`)
	if err != nil {
		return fmt.Errorf("%w: current automation pause evidence columns are required", store.ErrInitialSchema)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	primary, _, err := s.notificationInboxSchemaKeys(ctx, automationPauseEvidenceTable)
	if err != nil {
		return err
	}
	if primary != "occurrence_id" {
		return fmt.Errorf("%w: current automation pause evidence key is required", store.ErrInitialSchema)
	}
	return nil
}

const automationPauseRunSelect = `SELECT o.id,o.run_id,o.tenant_id,o.project_id,o.rule_id,o.revision,o.state,
	COALESCE(r.id,''),COALESCE(r.tenant_id,''),COALESCE(r.project_id,''),COALESCE(r.workflow_id,''),COALESCE(r.workflow_version,0),COALESCE(r.state,'')` +
	automationOccurrenceSource + automationOccurrenceRun + automationOccurrenceScope + ` AND o.revision=? AND o.state='started'`

type automationPauseRunSource struct {
	binding                        AutomationFailurePauseRun
	tenant, project, ruleID, state string
	revision                       int
	runID, runTenant, runProject   string
	workflowID, runState           string
	workflowVersion                int
}

func scanAutomationPauseRunSource(row scanner) (automationPauseRunSource, error) {
	var source automationPauseRunSource
	err := row.Scan(&source.binding.TriggerID, &source.binding.RunID, &source.tenant, &source.project, &source.ruleID, &source.revision, &source.state,
		&source.runID, &source.runTenant, &source.runProject, &source.workflowID, &source.workflowVersion, &source.runState)
	return source, err
}

// Compare exact identifiers even when the database collation is insensitive.
func (source automationPauseRunSource) inScope(ref automationRef, revision int, spec AutomationSpec) bool {
	return source.tenant == ref.tenant && source.project == ref.project && source.ruleID == ref.id &&
		source.revision == revision && source.state == "started" &&
		handoffID(source.binding.TriggerID, false) && handoffID(source.binding.RunID, false) &&
		source.binding.RunID == source.runID && source.runTenant == ref.tenant && source.runProject == ref.project &&
		source.workflowID == spec.WorkflowID && source.workflowVersion == spec.WorkflowVersion
}

func validAutomationFailurePauseRuns(runs []AutomationFailurePauseRun, threshold int, blockedID string) bool {
	if threshold == 0 || !validAutomationFailurePauseThreshold(threshold) || len(runs) != threshold {
		return false
	}
	triggerIDs, runIDs := map[string]bool{}, map[string]bool{}
	for _, run := range runs {
		if !handoffID(run.TriggerID, false) || !handoffID(run.RunID, false) || run.TriggerID == blockedID || triggerIDs[run.TriggerID] || runIDs[run.RunID] {
			return false
		}
		triggerIDs[run.TriggerID], runIDs[run.RunID] = true, true
	}
	return true
}

// Limit in trigger order before checking state. An unfinished or missing run
// interrupts the streak even when older dispatches all failed.
func (s *Service) automationFailurePauseRuns(ctx context.Context, tx *sql.Tx, ref automationRef, rule AutomationRule) ([]AutomationFailurePauseRun, error) {
	threshold := rule.FailurePauseThreshold
	if threshold == 0 {
		return nil, nil
	}
	if !validAutomationFailurePauseThreshold(threshold) {
		return nil, invalid(automationFailurePauseThresholdInvalid)
	}
	query := automationPauseRunSelect + ` ORDER BY o.created_at DESC,o.id DESC LIMIT ?`
	rows, err := tx.QueryContext(ctx, s.q(query), ref.tenant, ref.project, ref.id, rule.Revision, threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := make([]AutomationFailurePauseRun, 0, threshold)
	for rows.Next() {
		source, err := scanAutomationPauseRunSource(rows)
		if err != nil {
			return nil, err
		}
		if !source.inScope(ref, rule.Revision, rule.AutomationSpec) || source.runState != "failed" {
			return nil, nil
		}
		runs = append(runs, source.binding)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !validAutomationFailurePauseRuns(runs, threshold, "") {
		return nil, nil
	}
	return runs, nil
}

func (s *Service) insertAutomationFailurePauseEvidenceTx(ctx context.Context, tx *sql.Tx, ref automationRef, rule AutomationRule, id, at string, runs []AutomationFailurePauseRun) error {
	payload, err := json.Marshal(runs)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO `+automationPauseEvidenceTable+`(`+automationPauseEvidenceColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?)`),
		id, ref.tenant, ref.project, ref.id, rule.Revision, rule.WorkflowID, rule.WorkflowVersion, rule.FailurePauseThreshold, at, string(payload))
	return err
}

func (s *Service) automationFailurePauseEvidenceTx(ctx context.Context, tx *sql.Tx, ref automationRef, occurrence AutomationOccurrence) (*AutomationFailurePauseEvidence, error) {
	var evidence AutomationFailurePauseEvidence
	var id, tenant, project, ruleID, workflowID, raw, specRaw, revisionRuleID string
	var revision, workflowVersion int
	err := tx.QueryRowContext(ctx, s.q(`SELECT e.occurrence_id,e.tenant_id,e.project_id,e.rule_id,e.revision,e.workflow_id,e.workflow_version,e.threshold,e.observed_at,
		CASE WHEN LENGTH(e.failed_runs)<=8192 THEN e.failed_runs ELSE '' END,COALESCE(v.rule_id,''),COALESCE(v.spec,'')
		FROM `+automationPauseEvidenceTable+` e LEFT JOIN suma_automation_revisions v ON v.rule_id=e.rule_id AND v.revision=e.revision
		WHERE e.occurrence_id=? AND e.tenant_id=? AND e.project_id=? AND e.rule_id=?`), occurrence.ID, ref.tenant, ref.project, ref.id).
		Scan(&id, &tenant, &project, &ruleID, &revision, &workflowID, &workflowVersion, &evidence.Threshold, &evidence.ObservedAt, &raw, &revisionRuleID, &specRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if id != occurrence.ID || tenant != ref.tenant || project != ref.project || ruleID != ref.id || revisionRuleID != ruleID ||
		revision != occurrence.Revision || evidence.ObservedAt != occurrence.UpdatedAt || len(raw) > 8192 {
		return nil, nil
	}
	var spec AutomationSpec
	if json.Unmarshal([]byte(specRaw), &spec) != nil || spec.WorkflowID != workflowID || spec.WorkflowVersion != workflowVersion || spec.FailurePauseThreshold != evidence.Threshold ||
		json.Unmarshal([]byte(raw), &evidence.FailedRuns) != nil || !validAutomationFailurePauseRuns(evidence.FailedRuns, evidence.Threshold, occurrence.ID) {
		return nil, nil
	}
	if canonical, err := json.Marshal(evidence.FailedRuns); err != nil || string(canonical) != raw {
		return nil, nil
	}
	// Retain the original observed failures. Only recheck current scope and
	// linkage before offering navigation, even if a run's state has changed.
	bound, err := s.automationPauseEvidenceBindingsVisible(ctx, tx, ref, revision, spec, evidence.FailedRuns)
	if err != nil || !bound {
		return nil, err
	}
	return &evidence, nil
}

func (s *Service) automationPauseEvidenceBindingsVisible(ctx context.Context, tx *sql.Tx, ref automationRef, revision int, spec AutomationSpec, runs []AutomationFailurePauseRun) (bool, error) {
	bindings := make(map[string]string, len(runs))
	args := []any{ref.tenant, ref.project, ref.id, revision}
	for _, run := range runs {
		bindings[run.TriggerID] = run.RunID
		args = append(args, run.TriggerID)
	}
	query := automationPauseRunSelect + ` AND o.id IN (` + strings.Repeat("?,", len(runs)-1) + `?)`
	rows, err := tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		source, err := scanAutomationPauseRunSource(rows)
		if err != nil {
			return false, err
		}
		if !source.inScope(ref, revision, spec) || bindings[source.binding.TriggerID] != source.binding.RunID {
			return false, nil
		}
		delete(bindings, source.binding.TriggerID)
	}
	return len(bindings) == 0, rows.Err()
}
