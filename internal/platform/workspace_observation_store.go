package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

const (
	workspaceObservationInvalid     = "Invalid workspace observation request"
	workspaceObservationUnavailable = "The exact native workspace attempt is no longer available"
	workspaceObservationStale       = "Workspace observation lease has changed or expired"
	workspaceObservationCaller      = "Workspace observation belongs to its original current caller"
)

type PoolWorkspaceObservationClaimRequest struct {
	woc.ClaimRequest
	TenantID   string         `json:"tenant_id"`
	RunnerID   string         `json:"runner_id"`
	Assignment dag.Assignment `json:"assignment"`
}
type PoolWorkspaceObservationResultRequest struct {
	woc.ResultRequest
	TenantID   string         `json:"tenant_id"`
	RunnerID   string         `json:"runner_id"`
	Assignment dag.Assignment `json:"assignment"`
}

type workspaceObservationRecord struct {
	Observation                                   woc.Observation               `json:"observation"`
	Caller                                        expertTeamCredentialReference `json:"caller"`
	Assignment                                    dag.Assignment                `json:"assignment"`
	PoolOwner                                     string                        `json:"pool_owner,omitempty"`
	LeaseFence, LeaseUntil, LeaseStarted, Expires int64
	LeaseOwner, ResultSHA                         string
}

func (s *Service) InitializeWorkspaceObservations(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_workspace_observations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,caller_hash VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,assignment_fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,state VARCHAR(20) NOT NULL,version INTEGER NOT NULL,lease_fence BIGINT NOT NULL,lease_until BIGINT NOT NULL,lease_owner VARCHAR(64) NOT NULL,expires_at BIGINT NOT NULL,payload TEXT NOT NULL,result TEXT NOT NULL,result_sha256 VARCHAR(64) NOT NULL,UNIQUE(tenant_id,project_id,caller_hash,operation_id))`))
	if err != nil {
		return err
	}
	if err = store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_workspace_observation_claim", "suma_workspace_observations", "tenant_id", "project_id", "worker_id", "run_id", "node_id", "state", "expires_at", "id"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_workspace_observation_expiry", "suma_workspace_observations", "expires_at", "state", "id")
}
func (s *Service) workspaceObservationTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (workspaceObservationRecord, error) {
	var out workspaceObservationRecord
	var raw, result, state string
	var version int
	err := tx.QueryRowContext(ctx, s.q(`SELECT payload,result,state,version,lease_fence,lease_until,lease_owner,expires_at,result_sha256 FROM suma_workspace_observations WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw, &result, &state, &version, &out.LeaseFence, &out.LeaseUntil, &out.LeaseOwner, &out.Expires, &out.ResultSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	// Lease columns are independent mutable state; only immutable metadata is
	// decoded from payload, and result bytes remain separately expirable.
	var payload workspaceObservationRecord
	if err = json.Unmarshal([]byte(raw), &payload); err != nil {
		return out, err
	}
	out.Observation, out.Caller, out.Assignment, out.PoolOwner = payload.Observation, payload.Caller, payload.Assignment, payload.PoolOwner
	out.LeaseStarted = payload.LeaseStarted
	out.Observation.State, out.Observation.Version = state, version
	if result != "" {
		var r woc.Result
		if err = json.Unmarshal([]byte(result), &r); err != nil {
			return out, err
		}
		out.Observation.Result = &r
	}
	return out, nil
}
func (s *Service) saveWorkspaceObservationTx(ctx context.Context, tx *sql.Tx, tenant, project string, r *workspaceObservationRecord, old int) error {
	result := ""
	if r.Observation.Result != nil {
		result = stringify(r.Observation.Result)
	}
	snapshot := *r
	snapshot.Observation.Result = nil
	res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_workspace_observations SET state=?,version=?,lease_fence=?,lease_until=?,lease_owner=?,expires_at=?,payload=?,result=?,result_sha256=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), r.Observation.State, r.Observation.Version, r.LeaseFence, r.LeaseUntil, r.LeaseOwner, r.Expires, stringify(snapshot), result, r.ResultSHA, tenant, project, r.Observation.ID, old)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return conflict(workspaceObservationStale)
	}
	return nil
}
func workspaceObservationTime(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(time.RFC3339)
}

// Expiry discards content even for completed observations. It never touches a
// DAG node, lease, task, candidate, model allowance or execution result.
func (s *Service) ExpireWorkspaceObservations(ctx context.Context) (int, error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return 0, err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return 0, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_workspace_observations WHERE expires_at<=? AND state<>'expired' ORDER BY expires_at,id LIMIT 100`), clock)
	if err != nil {
		return 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, id := range ids {
		res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_workspace_observations SET state='expired',version=version+1,lease_until=0,lease_owner='',result='',result_sha256='' WHERE id=? AND expires_at<=? AND state<>'expired'`), id, clock)
		if e != nil {
			return 0, e
		}
		n, e := res.RowsAffected()
		if e != nil {
			return 0, e
		}
		count += int(n)
	}
	return count, tx.Commit()
}
