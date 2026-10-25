package platform

import (
	"context"
	"database/sql"
	"errors"
)

// This is the non-locking half of the authority module, used only after the
// complete organization coordination set is held by a human/machine/delegation
// guard. It reads real membership; IDs here never create an Actor or credential.
func (s *Service) currentProjectMembershipLockedTx(ctx context.Context, tx *sql.Tx, tenant, user, project string, write, approve bool) (Principal, error) {
	var p Principal
	err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT u.id,u.name,u.email,m.tenant_id,m.role FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE u.id=? AND m.tenant_id=?`)), user, tenant).Scan(&p.ID, &p.Name, &p.Email, &p.TenantID, &p.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return p, forbidden()
	}
	if err != nil {
		return p, err
	}
	var actualTenant string
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT tenant_id FROM suma_projects WHERE id=?`)), project).Scan(&actualTenant)
	if errors.Is(err, sql.ErrNoRows) {
		return p, missing()
	}
	if err != nil {
		return p, err
	}
	if actualTenant != tenant {
		return p, missing()
	}
	if orgAdmin(p) {
		return p, nil
	}
	var role string
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`)), project, user).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return p, forbidden()
	}
	if err != nil {
		return p, err
	}
	if !validProjectRole(role) || approve && role != "admin" && role != "reviewer" || write && role == "viewer" {
		return p, forbidden()
	}
	return p, nil
}
