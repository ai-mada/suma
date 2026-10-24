package platform

import (
	"context"
	"database/sql"
)

// Governance and the privileged write share one organization lock. The caller
// takes this lock before the scheduler lock, then checks current SQL authority.
func (s *Service) authorizeProjectTx(ctx context.Context, tx *sql.Tx, p Principal, project string, write, approve bool) error {
	if err := s.lockOrganization(ctx, tx, p.TenantID); err != nil {
		return err
	}
	var tenant string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT tenant_id FROM suma_projects WHERE id=?`), project).Scan(&tenant); err != nil || tenant != p.TenantID {
		return missing()
	}
	// Only the package-private system principal can project execution failures.
	// It still takes the organization lock and verifies the exact project scope.
	if p.internal && p.ID == "system" {
		return nil
	}
	live, err := s.liveOrganization(ctx, tx, p)
	if err != nil {
		return err
	}
	if orgAdmin(live) {
		return nil
	}
	var role string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`), project, p.ID).Scan(&role); err != nil {
		return forbidden()
	}
	if !validProjectRole(role) || approve && role != "reviewer" && role != "admin" || write && role == "viewer" {
		return forbidden()
	}
	return nil
}

func (s *Service) reviewReceipt(ctx context.Context, p Principal, project, key string, request any) (string, bool, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	if err = s.authorizeProjectTx(ctx, tx, p, project, true, true); err != nil {
		return "", false, err
	}
	return s.receipt(ctx, tx, p, project, key, request)
}
