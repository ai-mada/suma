package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func applicationReleaseSetDeploymentValid(project string, q ApplicationReleaseSetDeploymentPreviewRequest) bool {
	if q.ProjectID != project || !contentDigest.MatchString(q.ExpectedSetSHA256) || !testPlanText(q.Reason, 2000, true) || q.Environments == nil || len(q.Environments) < 1 || len(q.Environments) > 16 {
		return false
	}
	apps, envs := map[string]bool{}, map[string]bool{}
	for _, e := range q.Environments {
		if !handoffID(e.ApplicationID, false) || !handoffID(e.EnvironmentID, false) || e.ExpectedEnvironmentGeneration < 0 || e.ExpectedEnvironmentGeneration > 9007199254740990 || apps[e.ApplicationID] || envs[e.EnvironmentID] {
			return false
		}
		apps[e.ApplicationID] = true
		envs[e.EnvironmentID] = true
	}
	return true
}
func (s *Service) applicationReleaseSetDeploymentPreviewTx(ctx context.Context, tx *sql.Tx, p Principal, project, id string, q ApplicationReleaseSetDeploymentPreviewRequest) (out ApplicationReleaseSetDeploymentPreview, contents map[string]rc.ReleasePlanContent, err error) {
	contents = map[string]rc.ReleasePlanContent{}
	out = ApplicationReleaseSetDeploymentPreview{ReleaseSetID: id, Items: []ApplicationReleaseSetDeploymentPreviewItem{}, Blockers: []ApplicationReleaseSetBlocker{}, CanPrepare: true}
	set, err := s.applicationReleaseSetReadTx(ctx, tx, p, project, id)
	if err != nil {
		return out, contents, err
	}
	out.ReleaseSetSHA256 = set.SnapshotSHA256
	if set.SnapshotSHA256 != q.ExpectedSetSHA256 {
		return out, contents, conflict(applicationReleaseSetChanged)
	}
	out.Blockers, err = s.applicationReleaseSetBasisTx(ctx, tx, p, set)
	if err != nil {
		return
	}
	if len(out.Blockers) > 0 {
		out.CanPrepare = false
	}
	inputs := map[string]ApplicationReleaseSetEnvironmentInput{}
	for _, e := range q.Environments {
		inputs[e.ApplicationID] = e
	}
	if len(inputs) != len(set.Members) {
		return out, contents, invalid(applicationReleaseSetInvalid)
	}
	sites := map[string]bool{}
	for _, member := range set.Members {
		input, ok := inputs[member.ApplicationID]
		if !ok {
			return out, contents, invalid(applicationReleaseSetInvalid)
		}
		preview, content, e := s.releasePreviewTx(ctx, tx, p, project, ReleasePlanInput{ProjectID: project, CandidateID: member.CandidateID, EnvironmentID: input.EnvironmentID, Action: "deploy", ExpectedEnvironmentGeneration: input.ExpectedEnvironmentGeneration})
		if e != nil {
			if !expertTeamExpectedError(e) {
				return out, contents, e
			}
			out.CanPrepare = false
			out.Blockers = append(out.Blockers, applicationReleaseSetBlock("release_set_environment_changed", member.ApplicationID, input.EnvironmentID))
			continue
		}
		site := content.ProviderID + ":" + content.SiteID
		if sites[site] {
			out.CanPrepare = false
			out.Blockers = append(out.Blockers, applicationReleaseSetBlock("release_set_environment_changed", member.ApplicationID, input.EnvironmentID))
		}
		sites[site] = true
		if content.ArtifactID != member.ArtifactID || content.ArtifactSHA256 != member.ArtifactSHA256 || content.SourceCommit != member.SourceCommit {
			out.CanPrepare = false
			out.Blockers = append(out.Blockers, applicationReleaseSetBlock("release_set_candidate_stale", member.ApplicationID, member.CandidateID))
		}
		content.ApplicationReleaseSet = &rc.ApplicationReleaseSetRef{ReleaseSetID: set.ID, ReleaseSetSHA256: set.SnapshotSHA256, ApplicationID: member.ApplicationID, ApplicationVersionID: member.ApplicationVersionID, ApplicationVersionSHA256: member.ApplicationVersionSHA256}
		preview.PreviewSHA256 = releasePlanHash(content)
		contents[member.ApplicationID] = content
		out.Items = append(out.Items, ApplicationReleaseSetDeploymentPreviewItem{ApplicationID: member.ApplicationID, CandidateID: member.CandidateID, Preview: preview})
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].ApplicationID < out.Items[j].ApplicationID })
	// Identity/time and later runtime status never enter this reviewed preview.
	out.PreviewSHA256 = digest(stringify([]any{"application-release-set-deploy-preview-v1", set.ID, set.SnapshotSHA256, q.Reason, contents}))
	return
}
func (s *Service) PreviewApplicationReleaseSetDeployment(ctx context.Context, actor Actor, project, id string, q ApplicationReleaseSetDeploymentPreviewRequest) (out ApplicationReleaseSetDeploymentPreview, err error) {
	if !handoffID(id, false) || !applicationReleaseSetDeploymentValid(project, q) {
		return out, invalid(applicationReleaseSetInvalid)
	}
	ctx, auth, close, err := s.applicationRead(ctx, actor, project, id)
	if err != nil {
		return out, err
	}
	defer close()
	out, _, err = s.applicationReleaseSetDeploymentPreviewTx(ctx, auth.tx, auth.currentPrincipal, project, id, q)
	return
}
func (s *Service) PrepareApplicationReleaseSetDeployment(ctx context.Context, actor Actor, project, id string, q ApplicationReleaseSetDeploymentPrepareRequest) (out ApplicationReleaseSetPlanGroupView, err error) {
	if actor.kind != actorBrowser {
		return out, forbidden()
	}
	if !handoffID(id, false) || !applicationReleaseSetDeploymentValid(project, q.ApplicationReleaseSetDeploymentPreviewRequest) || !planningKey(q.IdempotencyKey) || !contentDigest.MatchString(q.ExpectedPreviewSHA256) {
		return out, invalid(applicationReleaseSetInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, q.IdempotencyKey, map[string]any{"application_release_set_deployment_prepare": id, "request": q})
	if err != nil {
		return out, err
	}
	if replay {
		var original ApplicationReleaseSetPlanGroupView
		if err = json.Unmarshal([]byte(hash), &original); err != nil {
			return
		}
		g, e := s.applicationReleaseSetGroupReadTx(ctx, auth.tx, auth.currentPrincipal, project, id, original.Group.ID)
		if e != nil {
			return out, e
		}
		if g.SnapshotSHA256 != original.Group.SnapshotSHA256 {
			return out, conflict(applicationReleaseSetChanged)
		}
		return s.applicationReleaseSetGroupViewTx(ctx, auth.tx, auth.currentPrincipal, g)
	}
	preview, contents, err := s.applicationReleaseSetDeploymentPreviewTx(ctx, auth.tx, auth.currentPrincipal, project, id, q.ApplicationReleaseSetDeploymentPreviewRequest)
	if err != nil {
		return out, err
	}
	if !preview.CanPrepare || len(preview.Items) != len(q.Environments) {
		return out, conflict(applicationReleaseSetBlocked)
	}
	if preview.PreviewSHA256 != q.ExpectedPreviewSHA256 {
		return out, conflict(applicationReleaseSetChanged)
	}
	group := ApplicationReleaseSetPlanGroup{ID: newID(), ProjectID: project, ReleaseSetID: id, ReleaseSetSHA256: preview.ReleaseSetSHA256, Bindings: []ApplicationReleaseSetPlanBinding{}, PreparedPreviewSHA256: preview.PreviewSHA256, Reason: q.Reason, CreatedBy: auth.currentPrincipal.ID, CreatedAt: now()}
	// This transaction creates the original pending-approval plans only. It does
	// not reserve environment generations, approve, dispatch or deploy anything.
	for _, item := range preview.Items {
		content := contents[item.ApplicationID]
		plan, e := s.createReleasePlanContentTx(ctx, auth.tx, auth.currentPrincipal, project, content)
		if e != nil {
			return out, e
		}
		ref := content.ApplicationReleaseSet
		group.Bindings = append(group.Bindings, ApplicationReleaseSetPlanBinding{ApplicationID: item.ApplicationID, ApplicationVersionID: ref.ApplicationVersionID, ApplicationVersionSHA256: ref.ApplicationVersionSHA256, CandidateID: content.CandidateID, ArtifactID: content.ArtifactID, ArtifactSHA256: content.ArtifactSHA256, EnvironmentID: content.EnvironmentID, PlanID: plan.ID, PlanSHA256: plan.SHA256})
	}
	group.SnapshotSHA256 = applicationReleaseSetGroupDigest(group)
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_application_release_set_plan_groups(id,tenant_id,project_id,set_id,set_sha256,snapshot_sha256,created_at,payload)VALUES(?,?,?,?,?,?,?,?)`), group.ID, auth.target.tenantID, project, id, group.ReleaseSetSHA256, group.SnapshotSHA256, group.CreatedAt, stringify(group)); err != nil {
		return
	}
	for _, b := range group.Bindings {
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_application_release_set_plan_bindings(plan_id,tenant_id,project_id,group_id,set_id,set_sha256,application_id,application_version_id,application_version_sha256,candidate_id,artifact_id,artifact_sha256,plan_sha256)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`), b.PlanID, auth.target.tenantID, project, group.ID, id, group.ReleaseSetSHA256, b.ApplicationID, b.ApplicationVersionID, b.ApplicationVersionSHA256, b.CandidateID, b.ArtifactID, b.ArtifactSHA256, b.PlanSHA256); err != nil {
			return
		}
	}
	out, err = s.applicationReleaseSetGroupViewTx(ctx, auth.tx, auth.currentPrincipal, group)
	if err != nil {
		return
	}
	if err = s.commandActivity(ctx, auth, "application_release_set.deployment_plans_prepared", group.ID, q.Reason); err != nil {
		return
	}
	if err = s.saveCommandReceipt(ctx, auth, q.IdempotencyKey, hash, out); err != nil {
		return
	}
	err = auth.tx.Commit()
	return
}

// Original approval, execution, authorization and effect gates call this through
// their existing current plan-basis check. A set reference grants no authority.
func (s *Service) applicationReleaseSetPlanBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project string, plan ReleasePlan) error {
	ref := plan.Content.ApplicationReleaseSet
	if ref == nil {
		return nil
	}
	if rc.ValidateApplicationReleaseSetRef(ref) != nil {
		return conflict(applicationReleaseSetChanged)
	}
	var groupID, setID, setSHA, appID, versionID, versionSHA, candidate, artifact, artifactSHA, planSHA string
	err := tx.QueryRowContext(ctx, s.q(`SELECT group_id,set_id,set_sha256,application_id,application_version_id,application_version_sha256,candidate_id,artifact_id,artifact_sha256,plan_sha256 FROM suma_application_release_set_plan_bindings WHERE tenant_id=? AND project_id=? AND plan_id=?`), p.TenantID, project, plan.ID).Scan(&groupID, &setID, &setSHA, &appID, &versionID, &versionSHA, &candidate, &artifact, &artifactSHA, &planSHA)
	if errors.Is(err, sql.ErrNoRows) {
		return conflict(applicationReleaseSetChanged)
	}
	if err != nil {
		return err
	}
	if setID != ref.ReleaseSetID || setSHA != ref.ReleaseSetSHA256 || appID != ref.ApplicationID || versionID != ref.ApplicationVersionID || versionSHA != ref.ApplicationVersionSHA256 || candidate != plan.Content.CandidateID || artifact != plan.Content.ArtifactID || artifactSHA != plan.Content.ArtifactSHA256 || planSHA != plan.SHA256 {
		return conflict(applicationReleaseSetChanged)
	}
	group, err := s.applicationReleaseSetGroupReadTx(ctx, tx, p, project, setID, groupID)
	if err != nil {
		return err
	}
	found := false
	for _, b := range group.Bindings {
		if b.PlanID == plan.ID && b.PlanSHA256 == plan.SHA256 && b.ApplicationID == appID {
			found = true
		}
	}
	if !found {
		return conflict(applicationReleaseSetChanged)
	}
	set, err := s.applicationReleaseSetReadTx(ctx, tx, p, project, setID)
	if err != nil {
		return err
	}
	if set.SnapshotSHA256 != setSHA {
		return conflict(applicationReleaseSetChanged)
	}
	memberFound := false
	for _, m := range set.Members {
		if m.ApplicationID == appID && m.ApplicationVersionID == versionID && m.ApplicationVersionSHA256 == versionSHA && m.CandidateID == candidate && m.ArtifactID == artifact && m.ArtifactSHA256 == artifactSHA {
			memberFound = true
		}
	}
	if !memberFound {
		return conflict(applicationReleaseSetChanged)
	}
	blockers, err := s.applicationReleaseSetBasisTx(ctx, tx, p, set)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return conflict(applicationReleaseSetBlocked)
	}
	return nil
}
