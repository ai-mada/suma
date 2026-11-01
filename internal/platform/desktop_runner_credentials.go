package platform

import (
	"context"
	"database/sql"
	"time"
)

// Only the desktop domain calls this after current browser administration and
// machine HMAC verification. It creates the original independent 24-hour Grant
// without receiving, retaining or returning the machine's secret.
func (s *Service) issueDesktopRunnerDigestTx(ctx context.Context, tx *sql.Tx, a *authorizedCommand, label, operation, tokenDigest string) (string, error) {
	return s.issueRegisteredRunnerDigestTx(ctx, tx, a, "desktop-runner", label, operation, tokenDigest)
}

// The owning browser domain verifies its specific approval before entering this
// shared issuer. Domain separation preserves the original desktop slot hashes.
func (s *Service) issueRegisteredRunnerDigestTx(ctx context.Context, tx *sql.Tx, a *authorizedCommand, domain, label, operation, tokenDigest string) (string, error) {
	if domain != "desktop-runner" && domain != "remote-runner" {
		return "", forbidden()
	}
	if a == nil {
		return "", forbidden()
	}
	if err := a.assertTx(tx, a.target, a.purpose); err != nil {
		return "", err
	}
	if a.actor.kind != actorBrowser || a.target.projectID == "" || a.target.projectID == organizationScope || !desktopWorkspaceHex(tokenDigest, 64) {
		return "", forbidden()
	}
	switch a.purpose {
	case purposeBrowserProjectWrite, purposeBrowserProjectApprove:
	default:
		return "", forbidden()
	}
	if len(operation) < 8 || len(operation) > 128 || len(label) > 200 {
		return "", desktopRunnerInvalid()
	}
	ref := credentialRef{subjectID: a.currentPrincipal.ID, tenantID: a.currentPrincipal.TenantID, projectID: a.target.projectID, sourceID: a.actor.credential.sourceID, sourceRevision: a.actor.credential.sourceRevision, grantID: newID(), slotID: newID()}
	expiry := a.checkedAt.Add(24 * time.Hour).Format(time.RFC3339Nano)
	slotOperation := digest(domain + ":" + ref.tenantID + ":" + ref.projectID + ":" + operation)
	hash := digest(a.currentPrincipal.ID + stringify(map[string]any{"action": domain + ".issue", "project_id": ref.projectID, "label": label, "operation_id": operation, "credential_sha256": tokenDigest}))
	if err := s.insertIndependentGrantTx(ctx, tx, a, actorProjectRunner, ref, ref.projectID, "runner", slotOperation, hash, expiry); err != nil {
		return "", err
	}
	if err := s.persistCredentialDigestTx(ctx, tx, a.currentPrincipal, actorProjectRunner, ref, label, expiry, a.checkedAt.Format(time.RFC3339Nano), operation, tokenDigest); err != nil {
		return "", err
	}
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_token_scopes(token_hash,kind,project_id) VALUES(?,'runner',?)`), tokenDigest, ref.projectID)
	return expiry, err
}
