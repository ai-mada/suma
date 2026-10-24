package platform

import (
	"context"
	"errors"
	"fmt"
	"github.com/ai-mada/suma/internal/dag"
)

// ProjectFailures consumes durable run_failed facts, including failures from
// Tick, approvals and subworkflows. It does not depend on a runner HTTP request.
// FailureIssue has its own transactional receipt: a crash after issue creation
// but before this ledger acknowledgement safely replays the same issue.
func (s *Service) ProjectFailures(ctx context.Context) error {
	rows, err := s.DB.QueryContext(ctx, `SELECT e.sequence,e.tenant_id,e.project_id,e.run_id,r.workflow_name FROM dag_events e JOIN dag_runs r ON r.id=e.run_id AND r.tenant_id=e.tenant_id AND r.project_id=e.project_id WHERE e.event_type='run_failed' AND NOT EXISTS(SELECT 1 FROM suma_failure_projection p WHERE p.sequence=e.sequence) ORDER BY e.sequence LIMIT 50`)
	if err != nil {
		return err
	}
	type fact struct {
		sequence                   int64
		tenant, project, run, name string
	}
	facts := []fact{}
	for rows.Next() {
		var item fact
		if err = rows.Scan(&item.sequence, &item.tenant, &item.project, &item.run, &item.name); err != nil {
			rows.Close()
			return err
		}
		facts = append(facts, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	failures := []error{}
	for _, item := range facts {
		run := dag.Run{ID: item.run, State: "failed", WorkflowName: item.name}
		if err = s.FailureIssue(ctx, item.tenant, item.project, run); err != nil {
			failures = append(failures, fmt.Errorf("failure event %d: %w", item.sequence, err))
			continue
		}
		query := `INSERT INTO suma_failure_projection(sequence,tenant_id,project_id,run_id,projected_at)VALUES(?,?,?,?,?) ON CONFLICT(sequence) DO NOTHING`
		if s.Dialect == "oceanbase" {
			query = `INSERT IGNORE INTO suma_failure_projection(sequence,tenant_id,project_id,run_id,projected_at)VALUES(?,?,?,?,?)`
		}
		if _, err = s.DB.ExecContext(ctx, s.q(query), item.sequence, item.tenant, item.project, item.run, now()); err != nil {
			failures = append(failures, fmt.Errorf("failure event acknowledgement %d: %w", item.sequence, err))
		}
	}
	return errors.Join(failures...)
}
