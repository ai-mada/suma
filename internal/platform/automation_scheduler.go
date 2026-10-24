package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

const automationRuleBatch = 32
const automationDispatchBatch = 100

type automationRef struct{ tenant, project, id string }

// TickAutomations performs bounded durable materialization followed by guarded
// dispatch. Multiple processes serialize through the existing organization lock.
func (s *Service) TickAutomations(ctx context.Context, e *dag.Engine) error {
	clock, err := s.automationClock(ctx, s.DB)
	if err != nil {
		return err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT tenant_id,project_id,id FROM suma_automations WHERE state='enabled' AND next_at<>'' AND next_at<=? ORDER BY next_at,id LIMIT ?`), automationStamp(clock), automationRuleBatch)
	if err != nil {
		return err
	}
	refs := []automationRef{}
	for rows.Next() {
		var r automationRef
		if err = rows.Scan(&r.tenant, &r.project, &r.id); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	var failures []error
	for _, r := range refs {
		if err = s.materializeAutomation(ctx, r); err != nil {
			failures = append(failures, err)
		}
	}
	// Admit only the head occurrence of each rule per tick. A large queue behind
	// an active rule cannot consume the entire dispatch batch. Waiting heads move
	// behind untouched heads through their persisted updated_at timestamp.
	rows, err = s.DB.QueryContext(ctx, s.q(`SELECT o.tenant_id,o.project_id,o.rule_id,o.id FROM suma_automation_occurrences o JOIN suma_automations a ON a.id=o.rule_id WHERE o.state='queued' AND a.state='enabled' AND NOT EXISTS(SELECT 1 FROM suma_automation_occurrences earlier WHERE earlier.rule_id=o.rule_id AND earlier.state='queued' AND (earlier.scheduled_at<o.scheduled_at OR (earlier.scheduled_at=o.scheduled_at AND earlier.created_at<o.created_at) OR (earlier.scheduled_at=o.scheduled_at AND earlier.created_at=o.created_at AND earlier.id<o.id))) ORDER BY o.updated_at,o.created_at,o.id LIMIT ?`), automationDispatchBatch)
	if err != nil {
		return err
	}
	type candidate struct {
		ref automationRef
		id  string
	}
	pending := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.ref.tenant, &c.ref.project, &c.ref.id, &c.id); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, c := range pending {
		if err = s.dispatchAutomation(ctx, e, c.ref, c.id); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
func automationActor(ref automationRef, id string) Principal {
	return Principal{ID: id, TenantID: ref.tenant}
}
func automationAuthorityFailure(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && (ae.Status == 403 || ae.Status == 404)
}
func (s *Service) materializeAutomation(ctx context.Context, ref automationRef) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, ref.tenant); err != nil {
		return err
	}
	p := automationActor(ref, "")
	a, err := s.automationRuleTx(ctx, tx, p, ref.project, ref.id)
	if err != nil {
		return err
	}
	if a.State != "enabled" || a.NextAt == "" {
		return nil
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	next, err := time.Parse(time.RFC3339, a.NextAt)
	if err != nil {
		return err
	}
	if next.After(clock) {
		return nil
	}
	c, err := validateAutomationSpec(a.AutomationSpec)
	if err != nil {
		return err
	}
	latest, err := c.PreviousContext(ctx, clock)
	if err != nil {
		return err
	}
	if latest.Before(next) {
		return fmt.Errorf("automation cursor is not a matching cron minute")
	}
	future, err := c.NextContext(ctx, clock)
	if err != nil {
		return err
	}
	a.NextAt = automationStamp(future)
	a.LastError = ""
	actor := automationActor(ref, a.ActivatedBy)
	if err = s.authorizeProjectTx(ctx, tx, actor, ref.project, true, true); err == nil {
		err = s.automationWorkflow(ctx, tx, actor, ref.project, a.AutomationSpec)
	}
	if err != nil {
		var ae *APIError
		if !automationAuthorityFailure(err) && !(errors.As(err, &ae) && ae.Status == 400) {
			return err
		}
		a.LastError = "activation_authority_or_workflow_unavailable"
		if _, err = s.insertAutomationOccurrence(ctx, tx, p, a, "cron", automationStamp(next), "blocked", a.LastError+";window_end="+automationStamp(latest), automationStamp(next)); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_automation_occurrences SET state='blocked',reason=?,updated_at=? WHERE tenant_id=? AND project_id=? AND rule_id=? AND state='queued'`), a.LastError, automationStamp(clock), ref.tenant, ref.project, ref.id); err != nil {
			return err
		}
	} else {
		missed := next.Before(clock.Truncate(time.Minute))
		insert := func(at time.Time, state, reason string) error {
			_, e := s.insertAutomationOccurrence(ctx, tx, p, a, "cron", automationStamp(at), state, reason, automationStamp(at))
			return e
		}
		switch {
		case !missed:
			err = insert(next, "queued", "")
		case a.Misfire == "skip":
			windowEnd := latest
			if latest.Equal(clock.Truncate(time.Minute)) {
				windowEnd, err = c.PreviousContext(ctx, latest.Add(-time.Minute))
				if err != nil {
					return err
				}
			}
			err = insert(next, "skipped", "misfire_skip;window_end="+automationStamp(windowEnd))
			if err == nil && latest.Equal(clock.Truncate(time.Minute)) {
				err = insert(latest, "queued", "")
			}
		case a.Misfire == "latest":
			err = insert(latest, "queued", "misfire_latest;window_start="+automationStamp(next))
		case a.Misfire == "catch_up":
			at := next
			for n := 0; n < a.CatchUpLimit && !at.After(clock); n++ {
				if err = insert(at, "queued", "misfire_catch_up"); err != nil {
					break
				}
				at, err = c.NextContext(ctx, at)
				if err != nil {
					break
				}
			}
			if err == nil && !at.After(clock) {
				err = insert(at, "skipped", "misfire_catch_up_limit;window_end="+automationStamp(latest))
			}
		}
		if err != nil {
			return err
		}
	}
	// Scheduler cursor/error updates deliberately do not consume user CAS versions.
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_automations SET next_at=?,last_error=? WHERE tenant_id=? AND project_id=? AND id=?`), a.NextAt, a.LastError, ref.tenant, ref.project, ref.id); err != nil {
		return err
	}
	return tx.Commit()
}

type automationDeferred struct{ state, reason string }

func (e *automationDeferred) Error() string { return e.reason }

func (s *Service) automationAdmission(ctx context.Context, tx *sql.Tx, ref automationRef, id string) (AutomationRule, AutomationOccurrence, error) {
	var o AutomationOccurrence
	if err := s.lockOrganization(ctx, tx, ref.tenant); err != nil {
		return AutomationRule{}, o, err
	}
	a, err := s.automationRuleTx(ctx, tx, automationActor(ref, ""), ref.project, ref.id)
	if err != nil {
		return a, o, err
	}
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,rule_id,revision,source,scheduled_at,state,reason,run_id,created_at,updated_at FROM suma_automation_occurrences WHERE tenant_id=? AND project_id=? AND rule_id=? AND id=?`), ref.tenant, ref.project, ref.id, id).Scan(&o.ID, &o.RuleID, &o.Revision, &o.Source, &o.ScheduledAt, &o.State, &o.Reason, &o.RunID, &o.CreatedAt, &o.UpdatedAt)
	if err != nil {
		return a, o, err
	}
	if o.State != "queued" {
		return a, o, &automationDeferred{"", "already_resolved"}
	}
	if a.State != "enabled" || a.Revision != o.Revision {
		return a, o, &automationDeferred{"blocked", "rule_inactive_or_revised"}
	}
	p := automationActor(ref, a.ActivatedBy)
	if err = s.authorizeProjectTx(ctx, tx, p, ref.project, true, true); err != nil {
		if automationAuthorityFailure(err) {
			return a, o, &automationDeferred{"blocked", "activation_authority_revoked"}
		}
		return a, o, err
	}
	if err = s.automationWorkflow(ctx, tx, p, ref.project, a.AutomationSpec); err != nil {
		var ae *APIError
		if errors.As(err, &ae) {
			return a, o, &automationDeferred{"blocked", "workflow_unavailable"}
		}
		return a, o, err
	}
	if a.Overlap != "allow" {
		var active int
		// Paused, reconciling and approval/waiting runs remain active. The DAG
		// exposes exactly these four terminal states for ordinary workflows.
		if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_automation_occurrences o JOIN dag_runs r ON r.id=o.run_id AND r.tenant_id=o.tenant_id AND r.project_id=o.project_id WHERE o.tenant_id=? AND o.project_id=? AND o.rule_id=? AND r.state NOT IN ('succeeded','failed','skipped','cancelled')`), ref.tenant, ref.project, ref.id).Scan(&active); err != nil {
			return a, o, err
		}
		if active > 0 {
			if a.Overlap == "forbid" {
				return a, o, &automationDeferred{"skipped", "overlap_forbid"}
			}
			return a, o, &automationDeferred{"queued", "overlap_wait"}
		}
		if a.Overlap == "queue" {
			var ahead int
			if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_automation_occurrences WHERE tenant_id=? AND project_id=? AND rule_id=? AND state='queued' AND (scheduled_at<? OR (scheduled_at=? AND created_at<?) OR (scheduled_at=? AND created_at=? AND id<?))`), ref.tenant, ref.project, ref.id, o.ScheduledAt, o.ScheduledAt, o.CreatedAt, o.ScheduledAt, o.CreatedAt, o.ID).Scan(&ahead); err != nil {
				return a, o, err
			}
			if ahead > 0 {
				return a, o, &automationDeferred{"queued", "queue_wait"}
			}
		}
	}
	return a, o, nil
}
func (s *Service) dispatchAutomation(ctx context.Context, e *dag.Engine, ref automationRef, id string) error {
	a, err := s.automationRuleTx(ctx, s.DB, automationActor(ref, ""), ref.project, ref.id)
	if err != nil {
		return err
	}
	_, err = e.CreateRunBoundAuthorized(ctx, ref.tenant, ref.project, a.WorkflowID, a.WorkflowVersion, "automation:"+id, "automation:"+id,
		func(tx *sql.Tx) error {
			current, _, err := s.automationAdmission(ctx, tx, ref, id)
			if err != nil {
				return err
			}
			if current.Revision != a.Revision || current.WorkflowID != a.WorkflowID || current.WorkflowVersion != a.WorkflowVersion {
				return &automationDeferred{"blocked", "rule_revised"}
			}
			return nil
		},
		func(tx *sql.Tx, run string) error {
			clock, err := s.automationClock(ctx, tx)
			if err != nil {
				return err
			}
			result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_automation_occurrences SET state='started',run_id=?,reason='',updated_at=? WHERE tenant_id=? AND project_id=? AND rule_id=? AND id=? AND state='queued'`), run, automationStamp(clock), ref.tenant, ref.project, ref.id, id)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return conflict("自动化触发已被处理")
			}
			_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_automations SET last_error='' WHERE tenant_id=? AND project_id=? AND id=?`), ref.tenant, ref.project, ref.id)
			return err
		})
	var deferred *automationDeferred
	if !errors.As(err, &deferred) {
		return err
	}
	// A declined admission rolls back the DAG transaction. Re-evaluate under the
	// same organization lock before persisting a skip/block/wait, so no stale
	// permission or overlap decision is written after another dispatcher wins.
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, _, err = s.automationAdmission(ctx, tx, ref, id)
	if !errors.As(err, &deferred) {
		return err
	}
	if deferred.state == "" {
		return nil
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_automation_occurrences SET state=?,reason=?,updated_at=? WHERE tenant_id=? AND project_id=? AND rule_id=? AND id=? AND state='queued'`), deferred.state, deferred.reason, automationStamp(clock), ref.tenant, ref.project, ref.id, id); err != nil {
		return err
	}
	if deferred.state == "blocked" {
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_automations SET last_error=? WHERE tenant_id=? AND project_id=? AND id=?`), deferred.reason, ref.tenant, ref.project, ref.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
