package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

const poolRunnerColumns = `r.id,r.pool_id,r.worker_id,r.name,r.version,r.status,r.max_slots,r.credential_version,r.last_seen_at,r.created_at,c.expires_at`

func scanPoolRunner(row interface{ Scan(...any) error }) (ExecutionPoolRunner, error) {
	var out ExecutionPoolRunner
	err := row.Scan(&out.ID, &out.PoolID, &out.WorkerID, &out.Name, &out.Version, &out.Status, &out.MaxSlots, &out.CredentialVersion, &out.LastSeenAt, &out.CreatedAt, &out.CredentialExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	out.AllowedActions = []string{}
	if out.Status == "active" {
		out.AllowedActions = []string{"rotate_credential", "revoke"}
	}
	return out, err
}

func (s *Service) poolRunnerTx(ctx context.Context, tx *sql.Tx, pool, runner string) (ExecutionPoolRunner, error) {
	return scanPoolRunner(tx.QueryRowContext(ctx, s.q(`SELECT `+poolRunnerColumns+` FROM suma_execution_pool_runners r JOIN suma_execution_pool_credentials c ON c.runner_id=r.id WHERE r.pool_id=? AND r.id=?`), pool, runner))
}

// RegisterExecutionPoolRunner creates a distinct service identity. Only the
// response to the first successful transaction carries the random secret.
// Persisting the metadata-only receipt makes retries safe without storing a
// credential in business history or accidentally treating it as a user session.
func (s *Service) RegisterExecutionPoolRunner(ctx context.Context, actor Actor, pool string, request ExecutionPoolRunnerCreateRequest) (ExecutionPoolCredentialResponse, error) {
	p := actor.Projection()

	var out ExecutionPoolCredentialResponse
	if err := validateExecutionPoolRunner(request); err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: organizationScope}, purposeOrganizationWrite)
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	if _, err = s.poolAdminTx(ctx, tx, auth, pool); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"register_pool_runner": pool, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		out.Token, out.Replayed, out.RecoveryAction = "", true, "rotate_credential"
		return out, err
	}
	secret := "suma_pool_" + newID() + newID()
	out.Runner, err = s.registerExecutionPoolRunnerDigestTx(ctx, tx, auth, pool, request, digest(secret))
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return ExecutionPoolCredentialResponse{}, err
	}
	out.Token = secret
	return out, nil
}

// registerExecutionPoolRunnerDigestTx is the single Pool registration effect.
// Manual registration and pairing approval supply the same current human
// authority; only the machine retains a pairing's plaintext credential.
func (s *Service) registerExecutionPoolRunnerDigestTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, pool string, request ExecutionPoolRunnerCreateRequest, tokenSHA string) (ExecutionPoolRunner, error) {
	var out ExecutionPoolRunner
	if auth == nil {
		return out, forbidden()
	}
	if err := auth.assertTx(tx, commandTarget{tenantID: auth.currentPrincipal.TenantID, projectID: organizationScope}, purposeOrganizationWrite); err != nil {
		return out, err
	}
	if err := validateExecutionPoolRunner(request); err != nil {
		return out, err
	}
	if !desktopWorkspaceHex(tokenSHA, 64) {
		return out, forbidden()
	}
	if _, err := s.poolAdminTx(ctx, tx, auth, pool); err != nil {
		return out, err
	}
	var total int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_execution_pool_runners WHERE pool_id=? AND status='active'`), pool).Scan(&total); err != nil {
		return out, err
	}
	if total >= 1000 {
		return out, conflict("资源池已达到 1000 个有效执行节点上限")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	id := newID()
	out = ExecutionPoolRunner{ID: id, PoolID: pool, WorkerID: "pool-" + id, Name: request.Name, Version: 1, Status: "active", MaxSlots: request.MaxSlots, CredentialVersion: 1, CredentialExpiresAt: automationStamp(clock.Add(time.Duration(request.TTLSeconds) * time.Second)), CreatedAt: automationStamp(clock), AllowedActions: []string{"rotate_credential", "revoke"}}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_execution_pool_runners(id,pool_id,worker_id,name,version,status,max_slots,credential_version,last_seen_at,created_by,created_at)VALUES(?,?,?,?,?,?,?,?,?,?,?)`), out.ID, pool, out.WorkerID, out.Name, 1, "active", out.MaxSlots, 1, "", auth.currentPrincipal.ID, out.CreatedAt); err != nil {
		return ExecutionPoolRunner{}, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_execution_pool_credentials(id,runner_id,version,token_hash,status,expires_at)VALUES(?,?,?,?,?,?)`), newID(), id, 1, tokenSHA, "active", out.CredentialExpiresAt); err != nil {
		return ExecutionPoolRunner{}, err
	}
	if err = s.commandActivity(ctx, auth, "execution_pool.runner_registered", id, out.Name); err != nil {
		return ExecutionPoolRunner{}, err
	}
	return out, nil
}

// Rotation requires a drained identity. This avoids invalidating a running
// dispatch's frozen credential generation and preserves unknown-effect slots
// until they have been reconciled. Revocation remains available immediately.
func (s *Service) RotateExecutionPoolCredential(ctx context.Context, actor Actor, pool, runner string, request ExecutionPoolCredentialRequest) (ExecutionPoolCredentialResponse, error) {
	p := actor.Projection()

	var out ExecutionPoolCredentialResponse
	if request.ExpectedVersion < 1 {
		return out, invalid("请提供当前执行节点版本")
	}
	if err := validatePoolOperation(request.IdempotencyKey); err != nil {
		return out, err
	}
	if err := validatePoolTTL(request.TTLSeconds); err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: organizationScope}, purposeOrganizationWrite)
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	if _, err = s.poolAdminTx(ctx, tx, auth, pool); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"rotate_pool_credential": runner, "pool": pool, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		out.Token, out.Replayed, out.RecoveryAction = "", true, "rotate_credential"
		return out, err
	}
	r, err := s.poolRunnerTx(ctx, tx, pool, runner)
	if err != nil {
		return out, err
	}
	if r.Status != "active" || r.Version != request.ExpectedVersion {
		return out, conflict("执行节点版本或状态已变化，请刷新")
	}
	var active int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes WHERE worker_id=? AND state IN ('running','reconciling')`), r.WorkerID).Scan(&active); err != nil {
		return out, err
	}
	if active != 0 {
		return out, conflict("请先排空正在执行的任务并核对失联结果，再轮换凭证")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	secret := "suma_pool_" + newID() + newID()
	r.Version++
	r.CredentialVersion++
	r.CredentialExpiresAt = automationStamp(clock.Add(time.Duration(request.TTLSeconds) * time.Second))
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_execution_pool_runners SET version=?,credential_version=? WHERE id=? AND pool_id=? AND version=? AND status='active'`), r.Version, r.CredentialVersion, runner, pool, request.ExpectedVersion)
	if err != nil {
		return out, err
	}
	if changed, e := result.RowsAffected(); e != nil || changed != 1 {
		return out, conflict("执行节点已被其他操作修改")
	}
	result, err = tx.ExecContext(ctx, s.q(`UPDATE suma_execution_pool_credentials SET version=?,token_hash=?,expires_at=? WHERE runner_id=? AND version=? AND status='active'`), r.CredentialVersion, digest(secret), r.CredentialExpiresAt, runner, r.CredentialVersion-1)
	if err != nil {
		return out, err
	}
	if changed, e := result.RowsAffected(); e != nil || changed != 1 {
		return out, conflict("执行凭证已变化，请刷新")
	}
	out.Runner = r
	if err = s.commandActivity(ctx, auth, "execution_pool.credential_rotated", runner, r.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = tx.Commit(); err != nil {
		return ExecutionPoolCredentialResponse{}, err
	}
	out.Token = secret
	return out, nil
}

func (s *Service) RevokeExecutionPoolRunner(ctx context.Context, actor Actor, pool, runner string, request ExecutionPoolCommandRequest) (ExecutionPoolRunner, error) {
	p := actor.Projection()

	var out ExecutionPoolRunner
	if request.Action != "revoke" || request.ExpectedVersion < 1 {
		return out, invalid("执行节点仅支持按明确版本撤销")
	}
	if err := validatePoolOperation(request.IdempotencyKey); err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: organizationScope}, purposeOrganizationWrite)
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	if _, err = s.poolAdminTx(ctx, tx, auth, pool); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"revoke_pool_runner": runner, "pool": pool, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	out, err = s.poolRunnerTx(ctx, tx, pool, runner)
	if err != nil {
		return out, err
	}
	if out.Status != "active" || out.Version != request.ExpectedVersion {
		return out, conflict("执行节点版本或状态已变化，请刷新")
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_execution_pool_runners SET status='revoked',version=version+1 WHERE id=? AND pool_id=? AND version=? AND status='active'`), runner, pool, request.ExpectedVersion)
	if err != nil {
		return out, err
	}
	if changed, e := result.RowsAffected(); e != nil || changed != 1 {
		return out, conflict("执行节点已变化")
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_execution_pool_credentials SET status='revoked' WHERE runner_id=?`), runner); err != nil {
		return out, err
	}
	out.Status, out.Version, out.AllowedActions = "revoked", out.Version+1, []string{}
	if err = s.commandActivity(ctx, auth, "execution_pool.runner_revoked", runner, out.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
