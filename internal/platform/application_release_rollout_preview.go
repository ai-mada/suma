package platform

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"strings"
)

func validRolloutPreview(project string, q ApplicationReleaseRolloutPreviewRequest) bool {
	return q.ProjectID == project && handoffID(q.PlanGroupID, false) && contentDigest.MatchString(q.ExpectedSetSHA256) && contentDigest.MatchString(q.ExpectedPlanGroupSHA256) && strings.TrimSpace(q.Reason) == q.Reason && len(q.Reason) > 0 && len(q.Reason) <= 2000
}
func rolloutBlocker(code, app, ref string) ApplicationReleaseSetBlocker {
	return ApplicationReleaseSetBlocker{Code: code, ApplicationID: app, RefID: ref}
}
func (s *Service) applicationRolloutPreviewTx(ctx context.Context, tx *sql.Tx, p Principal, project, setID string, q ApplicationReleaseRolloutPreviewRequest) (out ApplicationReleaseRolloutPreview, err error) {
	out.Blockers = []ApplicationReleaseSetBlocker{}
	set, err := s.applicationReleaseSetReadTx(ctx, tx, p, project, setID)
	if err != nil {
		return out, err
	}
	group, err := s.applicationReleaseSetGroupReadTx(ctx, tx, p, project, setID, q.PlanGroupID)
	if err != nil {
		return out, err
	}
	out.Rollout = ApplicationReleaseRollout{ProjectID: project, ReleaseSetID: setID, ReleaseSetSHA256: set.SnapshotSHA256, PlanGroupID: group.ID, PlanGroupSHA256: group.SnapshotSHA256, Reason: q.Reason, Members: []ApplicationReleaseRolloutMember{}}
	if set.SnapshotSHA256 != q.ExpectedSetSHA256 || group.SnapshotSHA256 != q.ExpectedPlanGroupSHA256 || group.ReleaseSetSHA256 != set.SnapshotSHA256 {
		return out, conflict(applicationRolloutChanged)
	}
	basis, err := s.applicationReleaseSetBasisTx(ctx, tx, p, set)
	if err != nil {
		return out, err
	}
	out.Blockers = append(out.Blockers, basis...)
	if len(set.Cycles) > 0 {
		out.Blockers = append(out.Blockers, rolloutBlocker("rollout_dependency_cycle", "", setID))
	}
	names := map[string]string{}
	for _, m := range set.Members {
		names[m.ApplicationID] = m.ApplicationName
	}
	deps := map[string][]string{}
	for _, d := range set.Dependencies {
		deps[d.ApplicationID] = append(deps[d.ApplicationID], d.TargetApplicationID)
	}
	for _, b := range group.Bindings {
		plan, e := s.currentReleasePlanTx(ctx, tx, p.TenantID, project, b.PlanID)
		if e != nil {
			return out, e
		}
		member := ApplicationReleaseRolloutMember{ApplicationID: b.ApplicationID, ApplicationName: names[b.ApplicationID], PlanID: plan.ID, PlanRevision: plan.Revision, PlanVersion: plan.Version, PlanSHA256: plan.SHA256, ParentNodeID: "app-" + b.ApplicationID, ChildRunID: plan.Content.ReservedExecution.RunID, ChildNodeID: plan.Content.ReservedExecution.NodeID, DependsOn: append([]string{}, deps[b.ApplicationID]...)}
		sort.Strings(member.DependsOn)
		unique := member.DependsOn[:0]
		for _, d := range member.DependsOn {
			if len(unique) == 0 || unique[len(unique)-1] != d {
				unique = append(unique, d)
			}
		}
		member.DependsOn = unique
		out.Rollout.Members = append(out.Rollout.Members, member)
		if plan.SHA256 != b.PlanSHA256 || plan.State != "approved" || plan.Approval == nil || plan.Approval.Decision != "approved" || plan.Approval.PlanSHA256 != plan.SHA256 || plan.ExecutionOperationID != "" {
			out.Blockers = append(out.Blockers, rolloutBlocker("rollout_approval_required", b.ApplicationID, plan.ID))
			continue
		}
		if e = s.releaseActorAdminTx(ctx, tx, p.TenantID, project, plan.Approval.ActorID); e != nil {
			if automationAuthorityFailure(e) {
				out.Blockers = append(out.Blockers, rolloutBlocker("rollout_approval_required", b.ApplicationID, plan.ID))
				continue
			}
			return out, e
		}
		if e = s.currentReleasePlanBasisTx(ctx, tx, p, project, plan, ""); e != nil {
			var api *APIError
			if errors.As(e, &api) && (api.Status == 400 || api.Status == 403 || api.Status == 404 || api.Status == 409) {
				out.Blockers = append(out.Blockers, rolloutBlocker("rollout_plan_changed", b.ApplicationID, plan.ID))
				continue
			}
			return out, e
		}
		var owner string
		e = tx.QueryRowContext(ctx, s.q(`SELECT rollout_id FROM suma_application_release_rollout_members WHERE tenant_id=? AND project_id=? AND plan_id=?`), p.TenantID, project, plan.ID).Scan(&owner)
		if e == nil {
			out.Blockers = append(out.Blockers, rolloutBlocker("rollout_plan_owned", b.ApplicationID, owner))
		} else if !errors.Is(e, sql.ErrNoRows) {
			return out, e
		}
	}
	sort.Slice(out.Rollout.Members, func(i, j int) bool {
		return out.Rollout.Members[i].ApplicationID < out.Rollout.Members[j].ApplicationID
	})
	if len(out.Rollout.Members) != len(set.Members) || len(out.Rollout.Members) == 0 || len(out.Rollout.Members) > 16 {
		out.Blockers = append(out.Blockers, rolloutBlocker("rollout_plan_changed", "", group.ID))
	}
	out.PreviewSHA256 = applicationRolloutPreviewDigest(out.Rollout)
	out.CanStart = len(out.Blockers) == 0
	return out, nil
}
func (s *Service) PreviewApplicationReleaseRollout(ctx context.Context, actor Actor, project, setID string, q ApplicationReleaseRolloutPreviewRequest) (out ApplicationReleaseRolloutPreview, err error) {
	if actor.kind != actorBrowser || !handoffID(setID, false) || !validRolloutPreview(project, q) {
		return out, invalid(applicationRolloutInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, setID, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	if err = s.releaseActorAdminTx(ctx, auth.tx, auth.target.tenantID, project, auth.currentPrincipal.ID); err != nil {
		return out, err
	}
	return s.applicationRolloutPreviewTx(ctx, auth.tx, auth.currentPrincipal, project, setID, q)
}
