package platform

import (
	"context"
	"database/sql"
)

// persistedBrowserProjectWriteLockedTx revalidates a persisted browser
// credential inside an execution callback that already holds the exact tenant's
// governance lock. Credential lifecycle and membership writes take that same
// lock. Do not call authorizeTx here: expanding to the creator's other
// organizations after a Runner/Pool lock would reverse the global lock order.
//
// The stored credential is the authority source; expectedSubject is only a
// comparison against its authenticated identity. This factory cannot approve,
// switch organizations or authorize any project other than the given target.
func (s *Service) persistedBrowserProjectWriteLockedTx(ctx context.Context, tx *sql.Tx, hash, expectedSubject string, target commandTarget) (*authorizedCommand, error) {
	if tx == nil || hash == "" || expectedSubject == "" || target.tenantID == "" || target.projectID == "" || target.projectID == organizationScope || target.subjectID != "" {
		return nil, forbidden()
	}
	budget, ok := s.authorityBudgets.Load(tx)
	if !ok {
		return nil, forbidden()
	}
	b := budget.(*authorityTxBudget)
	if err := b.ctx.Err(); err != nil {
		return nil, err
	}
	c, err := s.readCredential(ctx, tx, hash, true)
	if err != nil {
		return nil, err
	}
	at, err := s.authorityNow(ctx, tx)
	if err != nil {
		return nil, err
	}
	if c.kind != actorBrowser || c.ref.subjectID != expectedSubject || c.ref.tenantID != target.tenantID || !c.expires.After(at) {
		return nil, unauthenticated()
	}
	p, err := s.currentProjectMembershipLockedTx(ctx, tx, target.tenantID, c.ref.subjectID, target.projectID, true, false)
	if err != nil {
		return nil, err
	}
	return &authorizedCommand{tx: tx, actor: Actor{kind: c.kind, credential: c.ref}, target: target, purpose: purposeBrowserProjectWrite, currentPrincipal: p, checkedAt: at, deadline: b.deadline, ctx: b.ctx}, nil
}
