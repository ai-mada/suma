package platform

import (
	"context"
	"encoding/json"
)

func (s *Service) repositoryTargetsTx(ctx context.Context, auth *authorizedCommand) (out []RepositoryIntegrationTarget, err error) {
	out = []RepositoryIntegrationTarget{}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT data FROM suma_repository_targets WHERE tenant_id=? AND project_id=? ORDER BY id`), auth.target.tenantID, auth.target.projectID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var raw string
		var target RepositoryIntegrationTarget
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &target); err != nil {
			break
		}
		out = append(out, target)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	_, _, manage, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	for i := range out {
		out[i].CanManage = manage
		out[i].Blockers = []string{}
		if _, e := s.repositoryProvider(auth.target.tenantID, auth.target.projectID, out[i].ProviderID, out[i].TargetAlias); e != nil {
			out[i].Blockers = append(out[i].Blockers, repositoryProvider)
		}
		if out[i].ActiveOperationID != "" {
			out[i].Blockers = append(out[i].Blockers, repositoryBusy)
		}
	}
	return out, nil
}
func (s *Service) ListRepositoryIntegrationTargets(ctx context.Context, actor Actor, project string) (out []RepositoryIntegrationTarget, err error) {
	if !handoffID(project, false) {
		return nil, repositoryError(400, "invalid", repositoryInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return nil, err
	}
	defer close()
	return s.repositoryTargetsTx(ctx, auth)
}
func (s *Service) CreateRepositoryIntegrationTarget(ctx context.Context, actor Actor, project string, req RepositoryIntegrationTargetCreateRequest) (out RepositoryIntegrationTarget, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !planningText(req.Name, 120) || !releaseID(req.ProviderID) || !releaseID(req.TargetAlias) || !planningKey(req.IdempotencyKey) {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	first, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	body := map[string]any{"repository_target_create": req}
	hash, replay, err := s.commandReceipt(first, auth, req.IdempotencyKey, body)
	if err != nil {
		close()
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		close()
		return out, err
	}
	cfg, err := s.repositoryProvider(auth.target.tenantID, project, req.ProviderID, req.TargetAlias)
	close()
	if err != nil {
		return out, err
	}
	observed, err := repositoryProbe(ctx, cfg, req.TargetAlias)
	if err != nil {
		return out, err
	}
	ctx, auth, close, err = s.planningAuthority(ctx, actor, project, "", purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err = s.commandReceipt(ctx, auth, req.IdempotencyKey, body)
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	if _, err = s.repositoryProvider(auth.target.tenantID, project, req.ProviderID, req.TargetAlias); err != nil {
		return out, err
	}
	targets, err := s.repositoryTargetsTx(ctx, auth)
	if err != nil {
		return out, err
	}
	if len(targets) >= 100 {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	for _, target := range targets {
		if target.ProviderID == req.ProviderID && target.TargetAlias == req.TargetAlias {
			return out, repositoryConflict("target", repositoryTarget)
		}
	}
	out = RepositoryIntegrationTarget{ID: newID(), ProjectID: project, Version: 1, Name: req.Name, ProviderID: cfg.ID, TargetAlias: req.TargetAlias, Ref: observed.Ref, ConfigSHA256: observed.ConfigSHA256, Status: "active", Generation: observed.Generation, Observation: &observed, CanManage: true, Blockers: []string{}}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_repository_targets(id,tenant_id,project_id,version,data)VALUES(?,?,?,?,?)`), out.ID, auth.target.tenantID, project, out.Version, stringify(out)); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "repository.target_created", out.ID, out.Name); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
func (s *Service) UpdateRepositoryIntegrationTarget(ctx context.Context, actor Actor, project, id string, req RepositoryIntegrationTargetUpdateRequest) (out RepositoryIntegrationTarget, err error) {
	if req.ProjectID != project || !handoffID(id, false) || req.ExpectedVersion < 1 || !planningText(req.Name, 120) || req.Status != "active" && req.Status != "paused" && req.Status != "retired" || !planningKey(req.IdempotencyKey) {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"repository_target_update": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	out, err = s.repositoryTargetTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if out.Version != req.ExpectedVersion || out.Status == "retired" {
		return out, repositoryConflict("target", repositoryTarget)
	}
	old := out.Version
	out.Version++
	out.Name, out.Status = req.Name, req.Status
	if err = s.saveRepositoryTargetTx(ctx, auth.tx, auth.target.tenantID, old, out); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "repository.target_updated", id, out.Name); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}
func (s *Service) RepositoryIntegrationOptions(ctx context.Context, actor Actor, project, taskID string) (out RepositoryIntegrationOptions, err error) {
	out = RepositoryIntegrationOptions{ProjectID: project, TaskID: taskID, Sources: []RepositoryIntegrationSourceChoice{}, Targets: []RepositoryIntegrationTarget{}, Providers: []RepositoryIntegrationProviderChoice{}, Blockers: []string{}}
	if !handoffID(project, false) || !handoffID(taskID, false) {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	write, _, manage, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	out.CanManage = manage
	out.Targets, err = s.repositoryTargetsTx(ctx, auth)
	if err != nil {
		return out, err
	}
	for _, cfg := range s.RepositoryProviders {
		if cfg.TenantID == auth.target.tenantID && cfg.ProjectID == project {
			out.Providers = append(out.Providers, RepositoryIntegrationProviderChoice{ID: cfg.ID, TargetAliases: append([]string{}, cfg.TargetAliases...)})
		}
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return out, err
	}
	out.TaskVersion = task.Version
	if task.Status != "accepted" {
		out.Blockers = append(out.Blockers, repositoryBasis)
		return out, nil
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT node_id FROM suma_candidate_transfers WHERE tenant_id=? AND project_id=? AND run_id=? ORDER BY node_id`), auth.target.tenantID, project, task.RunID)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		basis, e := s.repositoryBasisTx(ctx, auth, taskID, id)
		if e != nil {
			if !repositoryKnownError(e) {
				return out, e
			}
			out.Blockers = append(out.Blockers, repositoryBasis)
			continue
		}
		out.Sources = append(out.Sources, RepositoryIntegrationSourceChoice{SourceNodeID: id, Source: basis.Source, Eligible: true, Blockers: []string{}})
	}
	out.CanCreate = write && len(out.Sources) > 0 && len(out.Targets) > 0
	return out, nil
}
