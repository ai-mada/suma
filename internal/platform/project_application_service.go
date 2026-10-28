package platform

import (
	"context"
	"encoding/json"
)

func (s *Service) applicationViewTx(ctx context.Context, auth *authorizedCommand, a ProjectApplication) (out ProjectApplicationView, err error) {
	out = ProjectApplicationView{Application: a, AllowedActions: []string{}, Blockers: []string{}}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_application_versions WHERE tenant_id=? AND project_id=? AND application_id=?`), auth.target.tenantID, a.ProjectID, a.ID).Scan(&out.RegisteredVersionCount); err != nil {
		return
	}
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	if write && auth.actor.kind == actorBrowser {
		out.AllowedActions = append(out.AllowedActions, "save")
		if a.State == "active" {
			out.AllowedActions = append(out.AllowedActions, "register_version")
		}
	}
	if a.State != "active" {
		out.Blockers = append(out.Blockers, "application_archived")
	}
	return
}
func (s *Service) applicationWrite(ctx context.Context, actor Actor, project, id string, q ApplicationWriteRequest) (out ProjectApplicationView, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if project != q.ProjectID || !handoffID(id, true) || !applicationWriteValid(q) || !planningKey(q.IdempotencyKey) || id == "" && q.ExpectedVersion != 0 || id != "" && q.ExpectedVersion < 1 {
		return out, invalid(applicationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, q.IdempotencyKey, map[string]any{"application_write": id, "request": q})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return
	}
	var a ProjectApplication
	if id != "" {
		a, err = s.projectApplicationTx(ctx, auth, project, id)
		if err != nil {
			return
		}
		if a.Version != q.ExpectedVersion {
			return out, conflict(applicationChanged)
		}
	}
	scopes, err := s.applicationFieldsTx(ctx, auth, project, a, q)
	if err != nil {
		return out, err
	}
	var duplicate int
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_applications WHERE tenant_id=? AND project_id=? AND app_key=? AND id<>?`), auth.target.tenantID, project, q.Key, id).Scan(&duplicate); err != nil {
		return
	}
	if duplicate != 0 {
		return out, conflict(applicationChanged)
	}
	previous := a.Version
	if id == "" {
		a.ID = newID()
		a.ProjectID = project
		a.CreatedBy = auth.currentPrincipal.ID
		a.CreatedAt = now()
	}
	a.Version++
	a.Key = q.Key
	a.Name = q.Name
	a.Description = q.Description
	a.Kind = q.Kind
	a.ParentApplicationID = q.ParentApplicationID
	a.OwnerTeamID = q.OwnerTeamID
	a.State = q.State
	a.CodeScopes = scopes
	a.UpdatedAt = now()
	if err = s.saveProjectApplicationTx(ctx, auth, a, previous); err != nil {
		return
	}
	if err = s.commandActivity(ctx, auth, "application.saved", a.ID, q.Reason); err != nil {
		return
	}
	if out, err = s.applicationViewTx(ctx, auth, a); err != nil {
		return
	}
	if err = s.saveCommandReceipt(ctx, auth, q.IdempotencyKey, hash, out); err != nil {
		return
	}
	err = auth.tx.Commit()
	return
}
func (s *Service) CreateProjectApplication(ctx context.Context, actor Actor, project string, q ApplicationWriteRequest) (ProjectApplicationView, error) {
	return s.applicationWrite(ctx, actor, project, "", q)
}
func (s *Service) SaveProjectApplication(ctx context.Context, actor Actor, project, id string, q ApplicationWriteRequest) (ProjectApplicationView, error) {
	return s.applicationWrite(ctx, actor, project, id, q)
}
