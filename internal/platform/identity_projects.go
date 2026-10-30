package platform

import "context"

// sessionProject is a read projection of current authority, not a stored project
// property or a grant supplied by the browser. Domain mutations still authorize
// the current caller independently in their own transaction.
type sessionProject struct {
	Record
	Role string `json:"role"`
}

func (s *Service) identitySessionProjectsTx(ctx context.Context, auth *authorizedCommand) ([]sessionProject, error) {
	projects, err := s.identityProjectsTx(ctx, auth)
	if err != nil {
		return nil, err
	}
	roles := map[string]string{}
	if auth.currentPrincipal.Role != "owner" && auth.currentPrincipal.Role != "admin" {
		// Read all memberships once, after identityProjectsTx closes its rows.
		// This also works with SQLite's single connection and avoids N+1 reads.
		rows, queryErr := auth.tx.QueryContext(ctx, s.q(`SELECT m.project_id,m.role FROM suma_project_members m JOIN suma_projects p ON p.id=m.project_id WHERE p.tenant_id=? AND m.user_id=?`), auth.target.tenantID, auth.currentPrincipal.ID)
		if queryErr != nil {
			return nil, queryErr
		}
		defer rows.Close()
		for rows.Next() {
			var project, role string
			if err = rows.Scan(&project, &role); err != nil {
				return nil, err
			}
			roles[project] = role
		}
		if err = rows.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]sessionProject, 0, len(projects))
	for _, project := range projects {
		role := roles[project.ID]
		if auth.currentPrincipal.Role == "owner" || auth.currentPrincipal.Role == "admin" {
			role = "admin" // Effective project role inherited from the organization.
		}
		switch role {
		case "admin", "developer", "reviewer", "viewer":
		default:
			return nil, forbidden() // Never infer write access from missing authority.
		}
		out = append(out, sessionProject{Record: project, Role: role})
	}
	return out, nil
}

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
