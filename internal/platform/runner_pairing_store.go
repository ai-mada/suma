package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

func (s *Service) InitializeRunnerPairings(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_runner_pairings(id VARCHAR(32) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,expires_at VARCHAR(40) NOT NULL,proposal_operation_id VARCHAR(128) NOT NULL,proposal_sha256 VARCHAR(64) NOT NULL,payload TEXT NOT NULL)`))
	return err
}

type runnerPairingRecord struct {
	RunnerPairing
	tenant, proposalOperation, proposalSHA string
}

func (s *Service) runnerPairingTx(ctx context.Context, q authorityQuery, id, project, tenant string, current bool) (out runnerPairingRecord, err error) {
	query := `SELECT tenant_id,version,state,expires_at,proposal_operation_id,proposal_sha256,payload FROM suma_runner_pairings WHERE id=? AND project_id=?`
	args := []any{id, project}
	if tenant != "" {
		query += ` AND tenant_id=?`
		args = append(args, tenant)
	}
	if current {
		query = s.currentSQL(query)
	}
	var raw, state, expires string
	var version int
	err = q.QueryRowContext(ctx, s.q(query), args...).Scan(&out.tenant, &version, &state, &expires, &out.proposalOperation, &out.proposalSHA, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(raw), &out.RunnerPairing) != nil || out.ID != id || out.ProjectID != project || out.Version != version || out.State != state || out.ExpiresAt != expires || out.CanApprove {
		return runnerPairingRecord{}, runnerPairingError(409, "changed")
	}
	return out, nil
}

func (s *Service) saveRunnerPairingTx(ctx context.Context, tx *sql.Tx, record runnerPairingRecord, before int) error {
	p := record.RunnerPairing
	p.CanApprove = false
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_runner_pairings SET version=?,state=?,proposal_operation_id=?,proposal_sha256=?,payload=? WHERE id=? AND tenant_id=? AND project_id=? AND version=?`), p.Version, p.State, record.proposalOperation, record.proposalSHA, stringify(p), p.ID, record.tenant, p.ProjectID, before)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count != 1 {
		return runnerPairingError(409, "changed")
	}
	return err
}

func runnerPairingView(record runnerPairingRecord, clock time.Time, browser bool, control string) (RunnerPairing, error) {
	p := record.RunnerPairing
	expires, err := time.Parse(time.RFC3339Nano, p.ExpiresAt)
	if err != nil {
		return RunnerPairing{}, runnerPairingError(409, "changed")
	}
	if (p.State == "waiting" || p.State == "proposed") && expires.Unix() <= clock.Unix() {
		p.State = "expired"
	}
	p.CanApprove = browser && p.State == "proposed" && runnerPairingSameControl(p.ControlURL, control)
	return p, nil
}
