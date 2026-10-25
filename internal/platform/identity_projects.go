package platform

import "context"

func (s *Service) Projects(ctx context.Context, actor Actor) (out []Record, err error) {
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}
	purpose := purposeOrganizationRead
	if actor.kind == actorProjectAPI {
		target.projectID = actor.credential.projectID
		purpose = purposeProjectRead
	}
	err = s.withCurrentRead(ctx, actor, target, purpose, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.identityProjectsTx(bounded, auth)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// The session envelope reuses this reader inside its own authority transaction;
// it never opens a nested connection while SQLite's only connection is held.
func (s *Service) identityProjectsTx(ctx context.Context, auth *authorizedCommand) ([]Record, error) {
	if auth == nil {
		return nil, forbidden()
	}
	if err := auth.assertTx(auth.tx, auth.target, auth.purpose); err != nil {
		return nil, err
	}
	if auth.purpose != purposeOrganizationRead && auth.purpose != purposeProjectRead {
		return nil, forbidden()
	}
	p := auth.currentPrincipal
	query := `SELECT p.id,p.tenant_id,p.version,p.title,p.description,p.status,p.created_at,p.updated_at FROM suma_projects p WHERE p.tenant_id=?`
	args := []any{p.TenantID}
	if auth.actor.kind == actorProjectAPI {
		query += ` AND p.id=?`
		args = append(args, auth.target.projectID)
	}
	if p.Role != "owner" && p.Role != "admin" {
		query += ` AND EXISTS(SELECT 1 FROM suma_project_members m WHERE m.project_id=p.id AND m.user_id=?)`
		args = append(args, p.ID)
	}
	query += ` ORDER BY p.created_at,p.id`
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		if err = rows.Scan(&r.ID, &r.TenantID, &r.Version, &r.Title, &r.Description, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Name = r.Title
		r.ProjectID = r.ID
		out = append(out, r)
	}
	return out, rows.Err()
}
