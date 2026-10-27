package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/store"
	wcc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

const (
	workspaceWriteInvalid     = "Invalid workspace save request"
	workspaceWriteChanged     = "Workspace save command has changed"
	workspaceWriteTaskChanged = "Workspace task execution basis has changed"
)

type PoolWorkspaceWriteClaimRequest struct {
	wcc.ClaimRequest
	TenantID   string         `json:"tenant_id"`
	RunnerID   string         `json:"runner_id"`
	Assignment dag.Assignment `json:"assignment"`
}
type PoolWorkspaceWriteResultRequest struct {
	wcc.ResultRequest
	TenantID   string         `json:"tenant_id"`
	RunnerID   string         `json:"runner_id"`
	Assignment dag.Assignment `json:"assignment"`
}
type workspaceWriteClaim struct {
	Mode, Owner, ResultSHA string
	IssuedAt, LeaseUntil   int64
}
type workspaceWriteRecord struct {
	Command                                              wcc.Command                    `json:"command"`
	Caller                                               expertTeamCredentialReference  `json:"caller"`
	Assignment                                           dag.Assignment                 `json:"assignment"`
	PoolOwner                                            string                         `json:"pool_owner,omitempty"`
	Claims                                               map[string]workspaceWriteClaim `json:"claims"`
	Content                                              string                         `json:"-"`
	Expires, ExecuteBefore, InspectionBefore, NextExpiry int64
}

func (s *Service) InitializeWorkspaceWrites(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_workspace_writes(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,caller_hash VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,assignment_fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,policy_sha256 VARCHAR(64) NOT NULL,state VARCHAR(20) NOT NULL,inspection_state VARCHAR(20) NOT NULL,version INTEGER NOT NULL,created_at BIGINT NOT NULL,expires_at BIGINT NOT NULL,execute_before BIGINT NOT NULL,next_expiry BIGINT NOT NULL,payload TEXT NOT NULL,content TEXT NOT NULL,UNIQUE(tenant_id,project_id,caller_hash,operation_id))`))
	if err != nil {
		return err
	}
	if err = store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_workspace_write_caller", "suma_workspace_writes", "tenant_id", "project_id", "caller_hash", "created_at", "id"); err != nil {
		return err
	}
	if err = store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_workspace_write_claim", "suma_workspace_writes", "tenant_id", "project_id", "worker_id", "run_id", "node_id", "state", "inspection_state", "id"); err != nil {
		return err
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_workspace_write_expiry", "suma_workspace_writes", "next_expiry", "id")
}
func (s *Service) workspaceWriteTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (workspaceWriteRecord, error) {
	var r workspaceWriteRecord
	var raw, state, inspection string
	var version int
	err := tx.QueryRowContext(ctx, s.q(`SELECT payload,content,state,inspection_state,version,expires_at,execute_before,next_expiry FROM suma_workspace_writes WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw, &r.Content, &state, &inspection, &version, &r.Expires, &r.ExecuteBefore, &r.NextExpiry)
	if errors.Is(err, sql.ErrNoRows) {
		return r, missing()
	}
	if err != nil {
		return r, err
	}
	var saved workspaceWriteRecord
	if err = json.Unmarshal([]byte(raw), &saved); err != nil {
		return r, err
	}
	r.Command, r.Caller, r.Assignment, r.PoolOwner, r.Claims, r.InspectionBefore = saved.Command, saved.Caller, saved.Assignment, saved.PoolOwner, saved.Claims, saved.InspectionBefore
	r.Command.State, r.Command.InspectionState, r.Command.Version = state, inspection, version
	if r.Claims == nil || wcc.ValidateCommand(r.Command) != nil {
		return r, invalid(workspaceWriteInvalid)
	}
	return r, nil
}
func (s *Service) saveWorkspaceWriteTx(ctx context.Context, tx *sql.Tx, tenant, project string, r *workspaceWriteRecord, old int) error {
	res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_workspace_writes SET state=?,inspection_state=?,version=?,expires_at=?,execute_before=?,next_expiry=?,payload=?,content=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), r.Command.State, r.Command.InspectionState, r.Command.Version, r.Expires, r.ExecuteBefore, r.NextExpiry, stringify(r), r.Content, tenant, project, r.Command.ID, old)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return conflict(workspaceWriteChanged)
	}
	return nil
}
func workspaceWriteNextExpiry(r *workspaceWriteRecord) {
	r.NextExpiry = 0
	if r.Command.State == "requested" {
		r.NextExpiry = r.Expires
	} else if r.Command.State == "leased" {
		r.NextExpiry = r.ExecuteBefore
	}
	if r.Command.InspectionState == "requested" || r.Command.InspectionState == "leased" {
		if r.NextExpiry == 0 || r.InspectionBefore < r.NextExpiry {
			r.NextExpiry = r.InspectionBefore
		}
	}
}
func (s *Service) refreshWorkspaceWriteTx(ctx context.Context, tx *sql.Tx, tenant, project string, r *workspaceWriteRecord, clock int64) error {
	old := r.Command.Version
	changed := false
	if r.Command.State == "requested" && r.Expires <= clock {
		r.Command.State = "expired"
		r.Command.ErrorCode = "permit_expired"
		r.Content = ""
		changed = true
	}
	if r.Command.State == "leased" && r.ExecuteBefore <= clock {
		r.Command.State = "reconciling"
		r.Command.ErrorCode = "operation_unknown"
		r.Content = ""
		changed = true
	}
	if (r.Command.InspectionState == "requested" || r.Command.InspectionState == "leased") && r.InspectionBefore <= clock {
		r.Command.InspectionState = "expired"
		changed = true
	}
	workspaceWriteNextExpiry(r)
	if changed {
		r.Command.Version++
		return s.saveWorkspaceWriteTx(ctx, tx, tenant, project, r, old)
	}
	return nil
}
func (s *Service) ExpireWorkspaceWrites(ctx context.Context) (int, error) {
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
	rows, err := tx.QueryContext(ctx, s.q(`SELECT tenant_id,project_id,id FROM suma_workspace_writes WHERE next_expiry>0 AND next_expiry<=? ORDER BY next_expiry,id LIMIT 100`), clock)
	if err != nil {
		return 0, err
	}
	type key struct{ tenant, project, id string }
	keys := []key{}
	for rows.Next() {
		var k key
		if err = rows.Scan(&k.tenant, &k.project, &k.id); err != nil {
			rows.Close()
			return 0, err
		}
		keys = append(keys, k)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, k := range keys {
		r, e := s.workspaceWriteTx(ctx, tx, k.tenant, k.project, k.id)
		if e != nil {
			return 0, e
		}
		if e = s.refreshWorkspaceWriteTx(ctx, tx, k.tenant, k.project, &r, clock); e != nil {
			return 0, e
		}
	}
	return len(keys), tx.Commit()
}
