package platform

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ai-mada/suma/internal/strictjson"
	"time"
)

func (s *Service) poolPairingAdminTx(ctx context.Context, tx *sql.Tx, actor Actor, pool string) (*authorizedCommand, ExecutionPool, error) {
	auth, e := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.Projection().TenantID, projectID: organizationScope}, purposeOrganizationWrite)
	if e != nil {
		return nil, ExecutionPool{}, e
	}
	p, e := s.poolAdminTx(ctx, tx, auth, pool)
	return auth, p, e
}
func poolPairingReceipt(raw string) (out PoolRunnerPairing, err error) {
	if strictjson.Decode([]byte(raw), &out, 8192) != nil || !validPoolPairing(out) || out.CanApprove || out.ReasonCode != "" {
		return out, poolPairingError(409, "changed")
	}
	return out, nil
}

// CreatePoolRunnerPairing fixes registration inputs before the machine ever
// proposes a digest. A retried create restores its original durable snapshot.
func (s *Service) CreatePoolRunnerPairing(parent context.Context, actor Actor, req PoolRunnerPairingCreateRequest, control string) (out PoolRunnerPairing, err error) {
	if !executionPoolID.MatchString(req.PoolID) || validateExecutionPoolRunner(ExecutionPoolRunnerCreateRequest{Name: req.Name, MaxSlots: req.MaxSlots, TTLSeconds: req.TTLSeconds, IdempotencyKey: req.IdempotencyKey}) != nil {
		return out, poolPairingError(400, "invalid")
	}
	ctx, tx, done, e := s.beginAuthorityTx(parent)
	if e != nil {
		return out, e
	}
	defer done()
	defer tx.Rollback()
	auth, pool, e := s.poolPairingAdminTx(ctx, tx, actor, req.PoolID)
	if e != nil {
		return out, e
	}
	hash, replay, e := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"pool_runner_pairing_create": req})
	if e != nil {
		return out, e
	}
	if replay {
		return poolPairingReceipt(hash)
	}
	if pool.Status != "active" {
		return out, poolPairingError(409, "changed")
	}
	canonical, e := runnerPairingControlURL(control)
	if e != nil {
		return out, poolPairingError(503, "unavailable")
	}
	clock, e := s.authorityNow(ctx, tx)
	if e != nil {
		return out, e
	}
	var total int
	if e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_pool_runner_pairings WHERE owner_tenant_id=? AND pool_id=? AND state IN ('waiting','proposed') AND expires_at>?`), pool.OwnerTenantID, pool.ID, poolPairingStamp(clock)).Scan(&total); e != nil {
		return out, e
	}
	if total >= 8 {
		return out, poolPairingError(409, "limit")
	}
	out = PoolRunnerPairing{ID: newID(), Purpose: "pool_runner", OwnerTenantID: pool.OwnerTenantID, PoolID: pool.ID, PoolVersion: pool.Version, Version: 1, State: "waiting", ControlURL: canonical, Name: req.Name, MaxSlots: req.MaxSlots, TTLSeconds: req.TTLSeconds, CreatedAt: poolPairingStamp(clock), ExpiresAt: poolPairingStamp(clock.Add(10 * time.Minute))}
	if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_pool_runner_pairings(id,owner_tenant_id,pool_id,version,state,created_at,expires_at,proposal_operation_id,proposal_sha256,payload)VALUES(?,?,?,1,'waiting',?,?,'','',?)`), out.ID, out.OwnerTenantID, out.PoolID, out.CreatedAt, out.ExpiresAt, stringify(out)); e != nil {
		return PoolRunnerPairing{}, e
	}
	if e = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); e != nil {
		return PoolRunnerPairing{}, e
	}
	if e = s.commandActivity(ctx, auth, "pool_runner_pairing.created", out.ID, out.Name); e != nil {
		return PoolRunnerPairing{}, e
	}
	return out, tx.Commit()
}

func (s *Service) poolPairingViewTx(ctx context.Context, tx *sql.Tx, row poolPairingRecord, pool ExecutionPool, clock time.Time, browser bool, control string) (PoolRunnerPairing, error) {
	out := row.PoolRunnerPairing
	out.CanApprove = false
	out.ReasonCode = ""
	expiry, e := time.Parse(time.RFC3339Nano, out.ExpiresAt)
	if e != nil {
		return PoolRunnerPairing{}, poolPairingError(409, "changed")
	}
	if (out.State == "waiting" || out.State == "proposed") && !clock.Before(expiry) {
		out.State = "expired"
	}
	if !browser {
		return out, nil
	}
	switch out.State {
	case "expired":
		out.ReasonCode = "expired"
	case "rejected":
		out.ReasonCode = "rejected"
	}
	if out.State == "waiting" || out.State == "proposed" {
		switch {
		case !runnerPairingSameControl(out.ControlURL, control):
			out.ReasonCode = "control_changed"
		case pool.Status != "active":
			out.ReasonCode = "pool_inactive"
		case pool.Version != out.PoolVersion:
			out.ReasonCode = "pool_changed"
		default:
			out.CanApprove = out.State == "proposed"
		}
	}
	if out.State == "approved" {
		identity, e := s.poolMachineIdentityByHashTx(ctx, tx, out.CredentialSHA256, out.RunnerID, false)
		if e != nil {
			var api *APIError
			if !errors.As(e, &api) || api.Status != 403 {
				return PoolRunnerPairing{}, e
			}
			out.ReasonCode = "credential_unavailable"
		} else if identity.owner != out.OwnerTenantID || identity.runner.PoolID != out.PoolID || identity.runner.WorkerID != out.WorkerID || identity.runner.CredentialVersion != out.CredentialVersion || identity.runner.CredentialExpiresAt != out.CredentialExpiresAt {
			out.ReasonCode = "credential_unavailable"
		} else if identity.poolStatus != "active" {
			out.ReasonCode = "pool_inactive"
		} else if !runnerPairingSameControl(out.ControlURL, control) {
			out.ReasonCode = "control_changed"
		}
	}
	return out, nil
}

func (s *Service) GetPoolRunnerPairing(parent context.Context, actor Actor, poolID, id, control string) (out PoolRunnerPairing, err error) {
	if !executionPoolID.MatchString(poolID) || !executionPoolID.MatchString(id) {
		return out, poolPairingError(400, "invalid")
	}
	ctx, tx, done, e := s.beginAuthorityTx(parent)
	if e != nil {
		return out, e
	}
	defer done()
	defer tx.Rollback()
	auth, pool, e := s.poolPairingAdminTx(ctx, tx, actor, poolID)
	if e != nil {
		return out, e
	}
	row, e := s.poolPairingTx(ctx, tx, id, poolID, auth.currentPrincipal.TenantID, true)
	if e != nil {
		return out, e
	}
	clock, e := s.authorityNow(ctx, tx)
	if e != nil {
		return out, e
	}
	return s.poolPairingViewTx(ctx, tx, row, pool, clock, true, control)
}

func (s *Service) CommandPoolRunnerPairing(parent context.Context, actor Actor, id string, req PoolRunnerPairingCommandRequest, control string) (out PoolRunnerPairing, err error) {
	if !executionPoolID.MatchString(id) || !executionPoolID.MatchString(req.PoolID) || !ingressVersion(req.ExpectedVersion) || validatePoolOperation(req.IdempotencyKey) != nil || (req.Action != "approve" && req.Action != "reject") {
		return out, poolPairingError(400, "invalid")
	}
	ctx, tx, done, e := s.beginAuthorityTx(parent)
	if e != nil {
		return out, e
	}
	defer done()
	defer tx.Rollback()
	auth, pool, e := s.poolPairingAdminTx(ctx, tx, actor, req.PoolID)
	if e != nil {
		return out, e
	}
	hash, replay, e := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"pool_runner_pairing_command": id, "request": req})
	if e != nil {
		return out, e
	}
	if replay {
		return poolPairingReceipt(hash)
	}
	row, e := s.poolPairingTx(ctx, tx, id, req.PoolID, pool.OwnerTenantID, true)
	if e != nil {
		return out, e
	}
	clock, e := s.authorityNow(ctx, tx)
	if e != nil {
		return out, e
	}
	view, e := s.poolPairingViewTx(ctx, tx, row, pool, clock, true, control)
	if e != nil {
		return out, e
	}
	if e = poolPairingTerminal(view); e != nil {
		return out, e
	}
	if row.Version != req.ExpectedVersion || row.Version >= 2147483647 || row.State == "approved" || (req.Action == "approve" && !view.CanApprove) {
		return out, poolPairingError(409, "changed")
	}
	before := row.Version
	row.Version++
	if req.Action == "reject" {
		row.State = "rejected"
	} else {
		runner, e := s.registerExecutionPoolRunnerDigestTx(ctx, tx, auth, req.PoolID, ExecutionPoolRunnerCreateRequest{Name: row.Name, MaxSlots: row.MaxSlots, TTLSeconds: row.TTLSeconds, IdempotencyKey: req.IdempotencyKey}, row.CredentialSHA256)
		if e != nil {
			return out, e
		}
		row.State = "approved"
		row.RunnerID = runner.ID
		row.WorkerID = runner.WorkerID
		row.CredentialVersion = runner.CredentialVersion
		row.CredentialExpiresAt = runner.CredentialExpiresAt
	}
	if e = s.savePoolPairingTx(ctx, tx, row, before); e != nil {
		return out, e
	}
	out = row.PoolRunnerPairing
	if e = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); e != nil {
		return PoolRunnerPairing{}, e
	}
	if e = s.commandActivity(ctx, auth, "pool_runner_pairing."+out.State, id, out.Name); e != nil {
		return PoolRunnerPairing{}, e
	}
	return out, tx.Commit()
}
