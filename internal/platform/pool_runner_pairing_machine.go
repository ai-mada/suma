package platform

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
)

// A proposal token proves only this exact pairing. It is never converted to an
// Actor and therefore cannot enter organization or project command authority.
func poolPairingProof(row poolPairingRecord, operation, token string) bool {
	return row.proposalOperation == operation && subtle.ConstantTimeCompare([]byte(row.CredentialSHA256), []byte(digest(token))) == 1
}
func (s *Service) poolPairingMachinePrecheck(ctx context.Context, id, pool, key, token string, proposal *PoolRunnerPairingProposalRequest) (poolPairingRecord, error) {
	row, e := s.poolPairingTx(ctx, s.DB, id, pool, "", false)
	if e != nil {
		var api *APIError
		if errors.As(e, &api) && api.Status == 404 {
			return row, forbidden()
		}
		return row, e
	}
	if proposal == nil {
		if !poolPairingProof(row, key, token) {
			return poolPairingRecord{}, forbidden()
		}
	} else if row.State != "waiting" && (!poolPairingProof(row, key, token) || row.proposalSHA != digest(stringify(*proposal))) {
		return poolPairingRecord{}, poolPairingError(409, "changed")
	}
	return row, nil
}
func (s *Service) poolPairingMachineLocked(ctx context.Context, tx *sql.Tx, pre poolPairingRecord) (poolPairingRecord, ExecutionPool, error) {
	if e := s.authorityWriter(ctx, tx); e != nil {
		return poolPairingRecord{}, ExecutionPool{}, e
	}
	if e := s.poolOrderedOrganizationsTx(ctx, tx, []string{pre.OwnerTenantID}); e != nil {
		return poolPairingRecord{}, ExecutionPool{}, e
	}
	pool, e := s.currentExecutionPoolMetadataTx(ctx, tx, pre.PoolID)
	if e != nil {
		return poolPairingRecord{}, pool, e
	}
	if pool.OwnerTenantID != pre.OwnerTenantID {
		return poolPairingRecord{}, pool, forbidden()
	}
	row, e := s.poolPairingTx(ctx, tx, pre.ID, pre.PoolID, pre.OwnerTenantID, true)
	return row, pool, e
}
func (s *Service) poolPairingMachineViewTx(ctx context.Context, tx *sql.Tx, row poolPairingRecord, pool ExecutionPool, key, token, control string) (PoolRunnerPairing, error) {
	if !poolPairingProof(row, key, token) {
		return PoolRunnerPairing{}, forbidden()
	}
	if row.State == "approved" {
		identity, e := s.poolMachineIdentityByHashTx(ctx, tx, row.CredentialSHA256, row.RunnerID, true)
		if e != nil {
			return PoolRunnerPairing{}, e
		}
		r := identity.runner
		if identity.owner != row.OwnerTenantID || identity.poolStatus != "active" || r.PoolID != row.PoolID || r.WorkerID != row.WorkerID || r.CredentialVersion != row.CredentialVersion || r.CredentialExpiresAt != row.CredentialExpiresAt {
			return PoolRunnerPairing{}, forbidden()
		}
	}
	if !runnerPairingSameControl(row.ControlURL, control) {
		return PoolRunnerPairing{}, poolPairingError(409, "changed")
	}
	clock, e := s.authorityNow(ctx, tx)
	if e != nil {
		return PoolRunnerPairing{}, e
	}
	out, e := s.poolPairingViewTx(ctx, tx, row, pool, clock, false, control)
	if e != nil {
		return out, e
	}
	if e = poolPairingTerminal(out); e != nil {
		return PoolRunnerPairing{}, e
	}
	return out, nil
}
func (s *Service) ProposePoolRunnerPairing(parent context.Context, id string, req PoolRunnerPairingProposalRequest, token, control string) (out PoolRunnerPairing, err error) {
	if !poolPairingMachineInput(id, req.PoolID, req.IdempotencyKey, token) || !poolPairingText(req.DeviceName, 100) || !desktopWorkspaceHex(req.CredentialSHA256, 64) {
		return out, poolPairingError(400, "invalid")
	}
	if subtle.ConstantTimeCompare([]byte(digest(token)), []byte(req.CredentialSHA256)) != 1 {
		return out, forbidden()
	}
	prectx, cancel := context.WithTimeout(parent, authorityTransactionBudget)
	defer cancel()
	pre, e := s.poolPairingMachinePrecheck(prectx, id, req.PoolID, req.IdempotencyKey, token, &req)
	if e != nil {
		return out, e
	}
	ctx, tx, done, e := s.beginAuthorityTx(prectx)
	if e != nil {
		return out, e
	}
	defer done()
	defer tx.Rollback()
	row, pool, e := s.poolPairingMachineLocked(ctx, tx, pre)
	if e != nil {
		return out, e
	}
	if row.State != "waiting" {
		if row.proposalSHA != digest(stringify(req)) {
			return out, poolPairingError(409, "changed")
		}
		return s.poolPairingMachineViewTx(ctx, tx, row, pool, req.IdempotencyKey, token, control)
	}
	if !runnerPairingSameControl(row.ControlURL, control) {
		return out, poolPairingError(409, "changed")
	}
	clock, e := s.authorityNow(ctx, tx)
	if e != nil {
		return out, e
	}
	view, e := s.poolPairingViewTx(ctx, tx, row, pool, clock, false, control)
	if e != nil {
		return out, e
	}
	if e = poolPairingTerminal(view); e != nil {
		return out, e
	}
	var existing int
	if e = tx.QueryRowContext(ctx, s.q(`SELECT (SELECT COUNT(*) FROM suma_execution_pool_credentials WHERE token_hash=?)+(SELECT COUNT(*) FROM suma_credential_authority WHERE token_hash=?)`), req.CredentialSHA256, req.CredentialSHA256).Scan(&existing); e != nil {
		return out, e
	}
	if existing != 0 {
		return out, poolPairingError(409, "changed")
	}
	before := row.Version
	if row.Version >= 2147483647 {
		return out, poolPairingError(409, "changed")
	}
	row.Version++
	row.State = "proposed"
	row.DeviceName = req.DeviceName
	row.CredentialSHA256 = req.CredentialSHA256
	row.proposalOperation = req.IdempotencyKey
	row.proposalSHA = digest(stringify(req))
	if e = s.savePoolPairingTx(ctx, tx, row, before); e != nil {
		return out, e
	}
	return row.PoolRunnerPairing, tx.Commit()
}
func (s *Service) PoolRunnerPairingStatus(parent context.Context, id string, req PoolRunnerPairingStatusRequest, token, control string) (out PoolRunnerPairing, err error) {
	if !poolPairingMachineInput(id, req.PoolID, req.IdempotencyKey, token) {
		return out, poolPairingError(400, "invalid")
	}
	prectx, cancel := context.WithTimeout(parent, authorityTransactionBudget)
	defer cancel()
	pre, e := s.poolPairingMachinePrecheck(prectx, id, req.PoolID, req.IdempotencyKey, token, nil)
	if e != nil {
		return out, e
	}
	ctx, tx, done, e := s.beginAuthorityTx(prectx)
	if e != nil {
		return out, e
	}
	defer done()
	defer tx.Rollback()
	row, pool, e := s.poolPairingMachineLocked(ctx, tx, pre)
	if e != nil {
		return out, e
	}
	return s.poolPairingMachineViewTx(ctx, tx, row, pool, req.IdempotencyKey, token, control)
}
