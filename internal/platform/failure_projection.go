package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

// This capability is derived only from a persisted failure event and binds one
// transaction and exact scope. It cannot authorize ordinary application writes.
type failureProjectionProof struct {
	tx                             *sql.Tx
	tenant, project, run, workflow string
	sequence                       int64
	occurredAt                     string
	deadline                       time.Time
}

func (s *Service) failureSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, runID string) (*failureProjectionProof, error) {
	if tx == nil {
		return nil, forbidden()
	}
	if err := s.authorityWriter(ctx, tx); err != nil {
		return nil, err
	}
	proof := &failureProjectionProof{tx: tx, deadline: s.txAuthorityBudget(ctx, tx).deadline}
	query := `SELECT e.sequence,r.tenant_id,r.project_id,r.id,r.workflow_name FROM dag_events e JOIN dag_runs r ON r.id=e.run_id AND r.tenant_id=e.tenant_id AND r.project_id=e.project_id JOIN suma_projects p ON p.id=r.project_id AND p.tenant_id=r.tenant_id WHERE e.event_type='run_failed' AND r.id=? AND r.tenant_id=? AND r.project_id=? ORDER BY e.sequence LIMIT 1`
	err := tx.QueryRowContext(ctx, s.q(query), runID, tenant, project).Scan(&proof.sequence, &proof.tenant, &proof.project, &proof.run, &proof.workflow)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, forbidden()
	}
	if err != nil {
		return nil, err
	}
	if err := s.lockOrganization(ctx, tx, proof.tenant); err != nil {
		return nil, err
	}
	if err := dag.GuardProjectTx(ctx, tx, s.Dialect, proof.tenant, proof.project); err != nil {
		return nil, err
	}
	var sequence int64
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT sequence,at_time FROM dag_events WHERE sequence=? AND tenant_id=? AND project_id=? AND run_id=? AND event_type='run_failed'`)), proof.sequence, proof.tenant, proof.project, proof.run).Scan(&sequence, &proof.occurredAt)
	if err != nil {
		return nil, err
	}
	return proof, nil
}

func (p *failureProjectionProof) assertTx(tx *sql.Tx) error {
	if p == nil || tx == nil || p.tx != tx || p.sequence <= 0 || p.tenant == "" || p.project == "" || p.run == "" {
		return forbidden()
	}
	if !time.Now().Before(p.deadline) {
		return context.DeadlineExceeded
	}
	return nil
}

func (s *Service) projectFailureIssue(ctx context.Context, tenant, project, runID string) error {
	parentCtx := ctx
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer tx.Rollback()
	proof, err := s.failureSourceTx(ctx, tx, tenant, project, runID)
	if err != nil {
		return err
	}
	var managed int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_release_deployments WHERE tenant_id=? AND project_id=? AND run_id=?`), proof.tenant, proof.project, proof.run).Scan(&managed); err != nil {
		return err
	}
	if managed > 0 {
		tx.Rollback()
		cancel()
		_, err = s.projectManagedReleaseFailure(parentCtx, proof.tenant, proof.project, proof.run)
		return err
	}
	if err := s.insertFailureIssueTx(ctx, tx, proof); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Service) insertFailureIssueTx(ctx context.Context, tx *sql.Tx, proof *failureProjectionProof) error {
	if err := proof.assertTx(tx); err != nil {
		return err
	}
	key := failureIssueKey(proof.run)
	m := failureIssueBody(proof.run, proof.workflow)
	hash := failureIssueHash(m)
	var previous, response string
	err := tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), proof.tenant, proof.project, key).Scan(&previous, &response)
	if err == nil {
		_, err = failureIssueReceiptID(response, previous, hash, proof.tenant, proof.project, proof.run)
		return err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	r := Record{ID: newID(), TenantID: proof.tenant, ProjectID: proof.project, Version: 1, Title: str(m, "title"), Description: str(m, "description"), Status: "open", Priority: "high", DependsOn: stringsAt(m, "depends_on"), AcceptanceCriteria: stringsAt(m, "acceptance_criteria"), CreatedAt: now(), UpdatedAt: now(), Data: m["data"].(map[string]any)}
	if err := requireTitle(r.Title); err != nil {
		return err
	}
	if err := s.insertRecord(ctx, tx, r, "issues"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_activity(id,tenant_id,project_id,actor_id,kind,object_id,title,created_at) VALUES(?,?,?,?,?,?,?,?)`), newID(), proof.tenant, proof.project, "system", "issues.created", r.ID, r.Title, now()); err != nil {
		return err
	}
	if err = s.projectTaskRunFailureInboxTx(ctx, tx, proof); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response) VALUES(?,?,?,?,?)`), proof.tenant, proof.project, key, hash, stringify(r))
	return err
}

const (
	failureProjectionBatchSize  = 50
	failureProjectionRetrySlots = 10
)

type failureProjectionFact struct {
	sequence, attempts   int64
	tenant, project, run string
}

func (s *Service) failureProjectionBatch(ctx context.Context, retries bool, databaseNow int64) ([]failureProjectionFact, error) {
	const runJoin = ` JOIN dag_runs r ON r.id=e.run_id AND r.tenant_id=e.tenant_id AND r.project_id=e.project_id`
	const unacknowledged = ` e.event_type='run_failed' AND NOT EXISTS(SELECT 1 FROM suma_failure_projection p WHERE p.sequence=e.sequence)`
	query := `SELECT e.sequence,0,e.tenant_id,e.project_id,e.run_id FROM dag_events e` + runJoin + ` WHERE` + unacknowledged +
		` AND NOT EXISTS(SELECT 1 FROM suma_failure_projection_retries retry WHERE retry.sequence=e.sequence) ORDER BY e.sequence LIMIT ?`
	args := []any{failureProjectionBatchSize}
	if retries {
		query = `SELECT e.sequence,retry.attempts,e.tenant_id,e.project_id,e.run_id FROM suma_failure_projection_retries retry JOIN dag_events e ON e.sequence=retry.sequence` + runJoin +
			` WHERE` + unacknowledged + ` AND retry.next_attempt_at<=? ORDER BY retry.next_attempt_at,e.sequence LIMIT ?`
		args = []any{databaseNow, failureProjectionBatchSize}
	}
	rows, err := s.DB.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	facts := make([]failureProjectionFact, 0, failureProjectionBatchSize)
	for rows.Next() {
		var item failureProjectionFact
		if err = rows.Scan(&item.sequence, &item.attempts, &item.tenant, &item.project, &item.run); err != nil {
			return nil, err
		}
		facts = append(facts, item)
	}
	return facts, rows.Err()
}

// ProjectFailures consumes durable run_failed facts without relying on a
// runner callback. Reserve space for due retries and fresh work in each batch;
// a continuous backlog in either queue cannot occupy all slots.
func (s *Service) ProjectFailures(ctx context.Context) error {
	databaseNow, err := s.failureProjectionNow(ctx)
	if err != nil {
		return err
	}
	fresh, err := s.failureProjectionBatch(ctx, false, databaseNow)
	if err != nil {
		return err
	}
	due, err := s.failureProjectionBatch(ctx, true, databaseNow)
	if err != nil {
		return err
	}
	freshCount := min(len(fresh), failureProjectionBatchSize-min(len(due), failureProjectionRetrySlots))
	retryCount := min(len(due), failureProjectionBatchSize-freshCount)
	var failures []error
	for _, item := range append(fresh[:freshCount], due[:retryCount]...) {
		if ctx.Err() != nil {
			return errors.Join(errors.Join(failures...), ctx.Err())
		}
		err = s.projectFailureIssue(ctx, item.tenant, item.project, item.run)
		if err == nil {
			// If acknowledgement fails after issue commit, the original
			// projection receipt makes the next attempt idempotent.
			err = s.ackFailureProjection(ctx, item.sequence, item.tenant, item.project, item.run)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("failure event %d: %w", item.sequence, err))
			failedAt, retryErr := s.failureProjectionNow(ctx)
			if retryErr == nil {
				retryErr = s.deferFailureProjection(ctx, item.sequence, item.attempts, failedAt)
			}
			if retryErr != nil {
				failures = append(failures, fmt.Errorf("schedule failure event %d retry: %w", item.sequence, retryErr))
			}
		}
	}
	return errors.Join(failures...)
}
