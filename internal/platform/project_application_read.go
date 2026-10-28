package platform

import (
	"context"
	"database/sql"
	"encoding/json"
)

func (s *Service) applicationRead(ctx context.Context, actor Actor, project, id string) (context.Context, *authorizedCommand, func(), error) {
	if !testPlanActor(actor) {
		return ctx, nil, func() {}, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, true) {
		return ctx, nil, func() {}, invalid(applicationInvalid)
	}
	return s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
}
func (s *Service) GetProjectApplication(ctx context.Context, actor Actor, project, id string) (out ProjectApplicationView, err error) {
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, id)
	if err != nil {
		return out, err
	}
	defer close()
	a, err := s.projectApplicationTx(ctx, auth, project, id)
	if err != nil {
		return out, err
	}
	return s.applicationViewTx(ctx, auth, a)
}
func (s *Service) ListProjectApplications(ctx context.Context, actor Actor, project, state string, limit int, before string) (out ProjectApplicationPage, err error) {
	out.Items = []ProjectApplication{}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 || !handoffID(before, true) || state != "" && state != "active" && state != "archived" {
		return out, invalid(applicationInvalid)
	}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, "")
	if err != nil {
		return out, err
	}
	defer close()
	filter := ` FROM suma_project_applications WHERE tenant_id=? AND project_id=?`
	args := []any{auth.target.tenantID, project}
	if state != "" {
		filter += ` AND state=?`
		args = append(args, state)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+filter), args...).Scan(&out.Total); err != nil {
		return
	}
	query := `SELECT id` + filter
	if before != "" {
		stamp, e := s.planningCursorTx(ctx, auth.tx, auth.target.tenantID, project, "suma_project_applications", before)
		if e != nil {
			return out, e
		}
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, stamp, stamp, before)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	ids, e := applicationQueryIDs(ctx, auth.tx, s.q(query), args...)
	if e != nil {
		return out, e
	}
	if len(ids) > limit {
		ids = ids[:limit]
		out.NextCursor = ids[len(ids)-1]
	}
	for _, id := range ids {
		a, e := s.projectApplicationTx(ctx, auth, project, id)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, a)
	}
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	out.CanCreate = write && actor.kind == actorBrowser
	return out, err
}
func applicationQueryIDs(ctx context.Context, tx *sql.Tx, query string, args ...any) (out []string, err error) {
	out = []string{}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return
		}
		out = append(out, id)
	}
	err = rows.Err()
	return
}
func (s *Service) GetApplicationOptions(ctx context.Context, actor Actor, project string) (out ApplicationOptions, err error) {
	out = ApplicationOptions{ProjectID: project, Teams: []ApplicationTeamOption{}, Targets: []RepositoryIntegrationTarget{}, Applications: []ProjectApplication{}, Complete: true, Limit: 100}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, "")
	if err != nil {
		return out, err
	}
	defer close()
	ids, err := applicationQueryIDs(ctx, auth.tx, s.q(`SELECT id FROM suma_teams WHERE tenant_id=? AND status='active' ORDER BY id LIMIT 101`), auth.target.tenantID)
	if err != nil {
		return out, err
	}
	// TeamTotal is the true qualifying count within the bounded scan. If the
	// organization has more candidates, Complete explicitly remains false.
	out.TeamTotalExact = len(ids) <= 100
	for i, id := range ids {
		if i >= 100 {
			out.Complete = false
			break
		}
		team, e := s.applicationTeamTx(ctx, auth, project, id)
		if e != nil {
			if expertTeamExpectedError(e) {
				continue
			}
			return out, e
		}
		out.Teams = append(out.Teams, team)
	}
	out.TeamTotal = len(out.Teams)
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_repository_targets WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&out.TargetTotal); err != nil {
		return
	}
	ids, err = applicationQueryIDs(ctx, auth.tx, s.q(`SELECT id FROM suma_repository_targets WHERE tenant_id=? AND project_id=? ORDER BY id LIMIT 100`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		t, e := s.repositoryTargetTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		t.CanManage = false
		t.Blockers = []string{}
		out.Targets = append(out.Targets, t)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_project_applications WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&out.ApplicationTotal); err != nil {
		return
	}
	ids, err = applicationQueryIDs(ctx, auth.tx, s.q(`SELECT id FROM suma_project_applications WHERE tenant_id=? AND project_id=? ORDER BY id LIMIT 100`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		a, e := s.projectApplicationTx(ctx, auth, project, id)
		if e != nil {
			return out, e
		}
		out.Applications = append(out.Applications, a)
	}
	out.Complete = out.Complete && len(out.Targets) == out.TargetTotal && len(out.Applications) == out.ApplicationTotal
	return
}
func (s *Service) GetApplicationVersionOptions(ctx context.Context, actor Actor, project, appID string) (out ApplicationVersionOptions, err error) {
	out = ApplicationVersionOptions{ProjectID: project, ApplicationID: appID, Integrations: []ApplicationIntegrationOption{}, Applications: []ProjectApplication{}, Versions: []ApplicationVersionOption{}, Complete: true, Limit: 100}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, appID)
	if err != nil {
		return out, err
	}
	defer close()
	app, err := s.projectApplicationTx(ctx, auth, project, appID)
	if err != nil {
		return out, err
	}
	targetIDs := map[string]bool{}
	for _, scope := range app.CodeScopes {
		targetIDs[scope.TargetID] = true
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT data FROM suma_repository_plans WHERE tenant_id=? AND project_id=? ORDER BY created_at DESC,id DESC LIMIT 1001`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	scanned := 0
	out.IntegrationTotalExact = true
	for rows.Next() {
		var raw string
		var p RepositoryIntegrationPlan
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &p); err != nil {
			break
		}
		scanned++
		if scanned > 1000 {
			out.Complete = false
			out.IntegrationTotalExact = false
			break
		}
		if p.State == "integrated" && targetIDs[p.TargetID] && p.Observation != nil && p.Observation.Receipt != nil {
			out.IntegrationTotal++
			if len(out.Integrations) < 100 {
				out.Integrations = append(out.Integrations, ApplicationIntegrationOption{ID: p.ID, Version: p.Version, TaskID: p.TaskID, TargetID: p.TargetID, Commit: p.Observation.Receipt.ResultCommit, State: p.State})
			}
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	ids, err := applicationQueryIDs(ctx, auth.tx, s.q(`SELECT id FROM suma_project_applications WHERE tenant_id=? AND project_id=? ORDER BY id LIMIT 101`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	if len(ids) > 100 {
		ids = ids[:100]
		out.Complete = false
	}
	for _, id := range ids {
		a, e := s.projectApplicationTx(ctx, auth, project, id)
		if e != nil {
			return out, e
		}
		out.Applications = append(out.Applications, a)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_application_versions WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&out.VersionTotal); err != nil {
		return
	}
	rows, err = auth.tx.QueryContext(ctx, s.q(`SELECT id,application_id,sequence,label,snapshot_sha256 FROM suma_application_versions WHERE tenant_id=? AND project_id=? ORDER BY created_at DESC,id DESC LIMIT 100`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v ApplicationVersionOption
		if err = rows.Scan(&v.ID, &v.ApplicationID, &v.Sequence, &v.Label, &v.SnapshotSHA256); err != nil {
			break
		}
		out.Versions = append(out.Versions, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	out.Complete = out.Complete && len(out.Integrations) == out.IntegrationTotal && len(out.Versions) == out.VersionTotal
	return
}
func (s *Service) GetApplicationVersion(ctx context.Context, actor Actor, project, appID, id string) (out ApplicationVersionView, err error) {
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, appID)
	if err != nil {
		return out, err
	}
	defer close()
	if !handoffID(id, false) {
		return out, invalid(applicationInvalid)
	}
	if _, err = s.projectApplicationTx(ctx, auth, project, appID); err != nil {
		return
	}
	v, err := s.applicationVersionTx(ctx, auth, project, appID, id)
	if err != nil {
		return out, err
	}
	return s.applicationVersionViewTx(ctx, auth, v)
}
func (s *Service) ListApplicationVersions(ctx context.Context, actor Actor, project, appID string, limit int, before string) (out ApplicationVersionPage, err error) {
	out.Items = []ApplicationCodeVersion{}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 || !handoffID(before, true) {
		return out, invalid(applicationInvalid)
	}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, appID)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = s.projectApplicationTx(ctx, auth, project, appID); err != nil {
		return
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_application_versions WHERE tenant_id=? AND project_id=? AND application_id=?`), auth.target.tenantID, project, appID).Scan(&out.Total); err != nil {
		return
	}
	query := `SELECT id FROM suma_application_versions WHERE tenant_id=? AND project_id=? AND application_id=?`
	args := []any{auth.target.tenantID, project, appID}
	if before != "" {
		v, e := s.applicationVersionTx(ctx, auth, project, appID, before)
		if e != nil {
			return out, e
		}
		query += ` AND sequence<?`
		args = append(args, v.Sequence)
	}
	query += ` ORDER BY sequence DESC LIMIT ?`
	args = append(args, limit+1)
	ids, e := applicationQueryIDs(ctx, auth.tx, s.q(query), args...)
	if e != nil {
		return out, e
	}
	if len(ids) > limit {
		ids = ids[:limit]
		out.NextCursor = ids[len(ids)-1]
	}
	for _, id := range ids {
		v, e := s.applicationVersionTx(ctx, auth, project, appID, id)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return
}
