package platform

import (
	"context"
	"database/sql"
	"encoding/json"
)

func (s *Service) applicationReleaseSetViewTx(ctx context.Context, auth *authorizedCommand, set ApplicationReleaseSet) (out ApplicationReleaseSetView, err error) {
	out = ApplicationReleaseSetView{Set: set, Blockers: []ApplicationReleaseSetBlocker{}, AllowedActions: []string{}, PlanGroups: []ApplicationReleaseSetPlanGroupView{}, PlanGroupsComplete: true}
	out.Blockers, err = s.applicationReleaseSetBasisTx(ctx, auth.tx, auth.currentPrincipal, set)
	if err != nil {
		return
	}
	out.BasisCurrent = len(out.Blockers) == 0
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	if out.BasisCurrent && write && auth.actor.kind == actorBrowser {
		out.AllowedActions = []string{"prepare_deployment"}
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_application_release_set_plan_groups WHERE tenant_id=? AND project_id=? AND set_id=?`), auth.target.tenantID, set.ProjectID, set.ID).Scan(&out.PlanGroupTotal); err != nil {
		return
	}
	ids, e := applicationQueryIDs(ctx, auth.tx, s.q(`SELECT id FROM suma_application_release_set_plan_groups WHERE tenant_id=? AND project_id=? AND set_id=? ORDER BY created_at DESC,id DESC LIMIT 10`), auth.target.tenantID, set.ProjectID, set.ID)
	if e != nil {
		return out, e
	}
	for _, id := range ids {
		group, e := s.applicationReleaseSetGroupReadTx(ctx, auth.tx, auth.currentPrincipal, set.ProjectID, set.ID, id)
		if e != nil {
			return out, e
		}
		view, e := s.applicationReleaseSetGroupViewTx(ctx, auth.tx, auth.currentPrincipal, group)
		if e != nil {
			return out, e
		}
		out.PlanGroups = append(out.PlanGroups, view)
	}
	out.PlanGroupsComplete = len(out.PlanGroups) == out.PlanGroupTotal
	return
}
func (s *Service) applicationReleaseSetGroupViewTx(ctx context.Context, tx *sql.Tx, p Principal, group ApplicationReleaseSetPlanGroup) (out ApplicationReleaseSetPlanGroupView, err error) {
	out = ApplicationReleaseSetPlanGroupView{Group: group, Items: []ApplicationReleaseSetRolloutItem{}}
	for _, binding := range group.Bindings {
		plan, e := s.currentReleasePlanTx(ctx, tx, p.TenantID, group.ProjectID, binding.PlanID)
		if e != nil {
			return out, e
		}
		if plan.SHA256 != binding.PlanSHA256 || releasePlanHash(plan.Content) != plan.SHA256 || plan.Content.ApplicationReleaseSet == nil || plan.Content.ApplicationReleaseSet.ReleaseSetID != group.ReleaseSetID || plan.Content.ApplicationReleaseSet.ReleaseSetSHA256 != group.ReleaseSetSHA256 || plan.Content.ApplicationReleaseSet.ApplicationID != binding.ApplicationID || plan.Content.ApplicationReleaseSet.ApplicationVersionID != binding.ApplicationVersionID || plan.Content.ApplicationReleaseSet.ApplicationVersionSHA256 != binding.ApplicationVersionSHA256 || plan.Content.ArtifactID != binding.ArtifactID || plan.Content.ArtifactSHA256 != binding.ArtifactSHA256 {
			return out, conflict(applicationReleaseSetChanged)
		}
		item := ApplicationReleaseSetRolloutItem{Binding: binding, PlanVersion: plan.Version, PlanState: plan.State, DeploymentOperationID: plan.ExecutionOperationID}
		if plan.ExecutionOperationID != "" {
			d, e := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, group.ProjectID, plan.ExecutionOperationID)
			if e != nil {
				return out, e
			}
			if d.PlanID != plan.ID || d.PlanSHA256 != plan.SHA256 {
				return out, conflict(applicationReleaseSetChanged)
			}
			item.DeploymentState = d.State
			item.EffectState = d.EffectState
			item.HealthState = d.HealthState
		}
		out.Items = append(out.Items, item)
	}
	return
}
func (s *Service) GetApplicationReleaseSet(ctx context.Context, actor Actor, project, id string) (out ApplicationReleaseSetView, err error) {
	if !handoffID(id, false) {
		return out, invalid(applicationReleaseSetInvalid)
	}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, id)
	if err != nil {
		return out, err
	}
	defer close()
	set, err := s.applicationReleaseSetReadTx(ctx, auth.tx, auth.currentPrincipal, project, id)
	if err != nil {
		return out, err
	}
	return s.applicationReleaseSetViewTx(ctx, auth, set)
}
func (s *Service) ListApplicationReleaseSets(ctx context.Context, actor Actor, project string, limit int, before string) (out ApplicationReleaseSetPage, err error) {
	out.Items = []ApplicationReleaseSetSummary{}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 || !handoffID(before, true) {
		return out, invalid(applicationReleaseSetInvalid)
	}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, "")
	if err != nil {
		return out, err
	}
	defer close()
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_application_release_sets WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&out.Total); err != nil {
		return
	}
	query := `SELECT id,label,snapshot_sha256,member_count,created_by,created_at FROM suma_application_release_sets WHERE tenant_id=? AND project_id=?`
	args := []any{auth.target.tenantID, project}
	if before != "" {
		stamp, e := s.planningCursorTx(ctx, auth.tx, auth.target.tenantID, project, "suma_application_release_sets", before)
		if e != nil {
			return out, e
		}
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, stamp, stamp, before)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		v := ApplicationReleaseSetSummary{ProjectID: project}
		if err = rows.Scan(&v.ID, &v.Label, &v.SnapshotSHA256, &v.MemberCount, &v.CreatedBy, &v.CreatedAt); err != nil {
			break
		}
		out.Items = append(out.Items, v)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[len(out.Items)-1].ID
	}
	write, _, _, err := s.repositoryCapabilitiesTx(ctx, auth)
	out.CanCreate = write && actor.kind == actorBrowser
	return out, err
}
func (s *Service) GetApplicationReleaseSetOptions(ctx context.Context, actor Actor, project string) (out ApplicationReleaseSetOptions, err error) {
	out = ApplicationReleaseSetOptions{ProjectID: project, Applications: []ApplicationReleaseSetApplicationOption{}, Versions: []ApplicationVersionOption{}, Candidates: []ApplicationReleaseSetCandidateOption{}, Environments: []ReleaseEnvironment{}, Complete: true, Limit: 100}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, "")
	if err != nil {
		return out, err
	}
	defer close()
	for _, item := range []struct {
		table string
		total *int
	}{{"suma_project_applications", &out.ApplicationTotal}, {"suma_application_versions", &out.VersionTotal}, {"suma_release_candidates", &out.CandidateTotal}, {"suma_release_environments", &out.EnvironmentTotal}} {
		if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM `+item.table+` WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(item.total); err != nil {
			return
		}
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id,name,state,current_version_id FROM suma_project_applications WHERE tenant_id=? AND project_id=? ORDER BY id LIMIT 100`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var a ApplicationReleaseSetApplicationOption
		if err = rows.Scan(&a.ID, &a.Name, &a.State, &a.CurrentRegisteredVersionID); err != nil {
			break
		}
		out.Applications = append(out.Applications, a)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
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
	if err != nil {
		return
	}
	rows, err = auth.tx.QueryContext(ctx, s.q(`SELECT payload FROM suma_release_candidates WHERE tenant_id=? AND project_id=? ORDER BY created_at DESC,id DESC LIMIT 100`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var raw string
		var c ReleaseCandidate
		if err = rows.Scan(&raw); err != nil {
			break
		}
		if err = json.Unmarshal([]byte(raw), &c); err != nil {
			break
		}
		if c.ProjectID != project {
			err = conflict(applicationReleaseSetChanged)
			break
		}
		out.Candidates = append(out.Candidates, ApplicationReleaseSetCandidateOption{ID: c.ID, Name: c.Name, SourceCommit: c.SourceCommit, ArtifactID: c.ArtifactID, ArtifactSHA256: c.ArtifactSHA256, BuildWorkerID: c.BuildProof.Assignment.WorkerID, PackageTestWorkerID: c.PackageTestProof.Assignment.WorkerID})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return
	}
	ids, err := applicationQueryIDs(ctx, auth.tx, s.q(`SELECT id FROM suma_release_environments WHERE tenant_id=? AND project_id=? ORDER BY id LIMIT 100`), auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		e, x := s.releaseEnvironmentTx(ctx, auth.tx, auth.target.tenantID, project, id, true)
		if x != nil {
			return out, x
		}
		e.AllowedActions = []string{}
		out.Environments = append(out.Environments, e)
	}
	out.Complete = len(out.Applications) == out.ApplicationTotal && len(out.Versions) == out.VersionTotal && len(out.Candidates) == out.CandidateTotal && len(out.Environments) == out.EnvironmentTotal
	return
}
