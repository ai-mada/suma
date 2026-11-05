package platform

import (
	"context"
	"database/sql"
	"encoding/json"
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
	key := "failure-" + proof.run
	m := map[string]any{"title": "执行失败：" + proof.workflow, "description": "运行 " + proof.run + " 失败，请查看节点证据并分诊。", "status": "open", "priority": "high", "idempotency_key": key, "data": map[string]any{"run_id": proof.run, "origin": "execution"}}
	hash := digest("system" + stringify(map[string]any{"kind": "issues", "body": m}))
	var previous, response string
	err := tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), proof.tenant, proof.project, key).Scan(&previous, &response)
	if err == nil {
		if previous != hash {
			return &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
		}
		var existing Record
		return json.Unmarshal([]byte(response), &existing)
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

// ProjectFailures consumes durable run_failed facts, including failures from
// Tick, approvals and subworkflows. It does not depend on a runner HTTP request.
// FailureIssue has its own transactional receipt. A crash before ledger
// acknowledgement replays the same issue without recalculating recipients.
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
