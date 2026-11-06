package platform

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

const failureProjectionRetryMaxSeconds int64 = 300

func (s *Service) failureProjectionNow(ctx context.Context) (int64, error) {
	clock, err := s.authorityNow(ctx, s.DB)
	return clock.Unix(), err
}

// Retry and acknowledgement serialize on the authoritative event, which also
// covers a first failure without a retry row to lock. Neither changes the event.
func (s *Service) lockFailureProjectionEventTx(ctx context.Context, tx *sql.Tx, sequence int64) (tenant, project, run string, err error) {
	if err = s.authorityWriter(ctx, tx); err != nil {
		return
	}
	query := s.currentSQL(`SELECT tenant_id,project_id,run_id FROM dag_events WHERE sequence=? AND event_type='run_failed'`)
	err = tx.QueryRowContext(ctx, s.q(query), sequence).Scan(&tenant, &project, &run)
	return
}

// deferFailureProjection advances the selected retry generation once. A stale
// worker cannot postpone another worker's retry or revive an acknowledged event.
func (s *Service) deferFailureProjection(ctx context.Context, sequence, previousAttempts, databaseNow int64) error {
	if previousAttempts < 0 || previousAttempts == math.MaxInt64 || databaseNow < 0 || databaseNow > math.MaxInt64-failureProjectionRetryMaxSeconds {
		return fmt.Errorf("invalid failure projection retry state")
	}
	delay := min(failureProjectionRetryMaxSeconds, int64(1)<<min(previousAttempts, 9))
	ctx, tx, close, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer close()
	if _, _, _, err = s.lockFailureProjectionEventTx(ctx, tx, sequence); err != nil {
		return err
	}
	var acknowledged bool
	if err = tx.QueryRowContext(ctx, s.q(`SELECT EXISTS(SELECT 1 FROM suma_failure_projection WHERE sequence=?)`), sequence).Scan(&acknowledged); err != nil || acknowledged {
		return err
	}
	if previousAttempts == 0 {
		query := `INSERT INTO suma_failure_projection_retries(sequence,attempts,next_attempt_at) VALUES(?,?,?) ON CONFLICT(sequence) DO NOTHING`
		if s.Dialect == "oceanbase" {
			query = `INSERT INTO suma_failure_projection_retries(sequence,attempts,next_attempt_at) VALUES(?,?,?) ON DUPLICATE KEY UPDATE sequence=sequence`
		}
		_, err = tx.ExecContext(ctx, s.q(query), sequence, 1, databaseNow+delay)
	} else {
		_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_failure_projection_retries SET attempts=?,next_attempt_at=? WHERE sequence=? AND attempts=?`), previousAttempts+1, databaseNow+delay, sequence, previousAttempts)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ackFailureProjection runs only after the issue projection succeeds. Retry
// retirement and its ledger acknowledgement must commit or roll back together.
func (s *Service) ackFailureProjection(ctx context.Context, sequence int64, tenant, project, run string) error {
	ctx, tx, close, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer close()
	storedTenant, storedProject, storedRun, err := s.lockFailureProjectionEventTx(ctx, tx, sequence)
	if err != nil {
		return err
	}
	if storedTenant != tenant || storedProject != project || storedRun != run {
		return fmt.Errorf("failure projection event scope changed")
	}
	query := `INSERT INTO suma_failure_projection(sequence,tenant_id,project_id,run_id,projected_at) VALUES(?,?,?,?,?) ON CONFLICT(sequence) DO NOTHING`
	if s.Dialect == "oceanbase" {
		query = `INSERT INTO suma_failure_projection(sequence,tenant_id,project_id,run_id,projected_at) VALUES(?,?,?,?,?) ON DUPLICATE KEY UPDATE sequence=sequence`
	}
	if _, err = tx.ExecContext(ctx, s.q(query), sequence, tenant, project, run, now()); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, s.q(`DELETE FROM suma_failure_projection_retries WHERE sequence=?`), sequence); err != nil {
		return err
	}
	return tx.Commit()
}
