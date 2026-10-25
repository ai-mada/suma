package platform

import (
	"context"
	"database/sql"
)

// engineCommandAuthority joins platform authority to the Engine's transaction.
// The Engine owns commit/rollback; this adapter owns the bounded context and
// releases the authority timer even when its before callback refuses a replay.
type engineCommandAuthority struct {
	service *Service
	ctx     context.Context
	cancel  context.CancelFunc
	actor   Actor
	target  commandTarget
	purpose commandPurpose
	tx      *sql.Tx
	proof   *authorizedCommand
}

func (s *Service) engineAuthority(ctx context.Context, actor Actor, target commandTarget, purpose commandPurpose) *engineCommandAuthority {
	bounded, cancel := context.WithTimeout(ctx, authorityTransactionBudget)
	return &engineCommandAuthority{service: s, ctx: bounded, cancel: cancel, actor: actor, target: target, purpose: purpose}
}

func (a *engineCommandAuthority) before(tx *sql.Tx) error {
	a.tx = tx
	var err error
	a.proof, err = a.service.authorizeTx(a.ctx, tx, a.actor, a.target, a.purpose)
	return err
}

// Execution callbacks establish the credential proof once, before scheduler
// access. Claim filters, tool events and artifact/result checks then consume it
// instead of reacquiring organization locks after execution locks.
func (a *engineCommandAuthority) beforeRunner(tx *sql.Tx, worker string) error {
	a.tx = tx
	var err error
	a.proof, err = a.service.runnerIdentityTx(a.ctx, tx, a.actor, a.target.projectID, worker)
	return err
}

func (a *engineCommandAuthority) close() {
	a.cancel()
	if a.tx != nil {
		a.service.endAuthorityTx(a.tx)
	}
}
