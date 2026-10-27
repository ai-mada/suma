package platform

import (
	"context"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) expertTeamCapabilitiesTx(ctx context.Context, auth *authorizedCommand) (write, approve bool, err error) {
	if auth.actor.kind != actorBrowser && auth.actor.kind != actorProjectAPI {
		return false, false, nil
	}
	_, e := s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, auth.target.projectID, true, false)
	if e == nil {
		write = true
	} else if !expertTeamExpectedError(e) {
		return false, false, e
	}
	if auth.actor.kind == actorBrowser {
		_, e = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, auth.target.projectID, false, true)
		if e == nil {
			approve = true
		} else if !expertTeamExpectedError(e) {
			return false, false, e
		}
	}
	return
}

func clearExpertTeamActions(out *ExpertTeamConfiguration) {
	out.CanEdit = false
	out.CanPublish = false
	out.CanRetire = false
	if out.Roles == nil {
		out.Roles = []ExpertTeamRole{}
	}
	if out.Blockers == nil {
		out.Blockers = []string{}
	}
	for i := range out.Roles {
		if out.Roles[i].Knowledge == nil {
			out.Roles[i].Knowledge = []ProjectKnowledgeReference{}
		}
	}
}

func (s *Service) expertTeamProjectionTx(ctx context.Context, auth *authorizedCommand, out ExpertTeamConfiguration) (ExpertTeamConfiguration, error) {
	clearExpertTeamActions(&out)
	out.Blockers = []string{}
	out.Valid = false
	head, err := s.expertTeamHeadTx(ctx, auth, out.TeamID, 0)
	if err != nil {
		return out, err
	}
	if head.Version != out.Version {
		out.Blockers = append(out.Blockers, expertChanged)
		return out, nil
	}
	write, approve, err := s.expertTeamCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	if out.State == "retired" {
		out.Blockers = append(out.Blockers, expertRetired)
		return out, nil
	}
	_, _, err = s.expertTeamIdentityTx(ctx, auth, out.TeamID)
	if err != nil {
		if !expertTeamExpectedError(err) {
			return out, err
		}
		out.Blockers = append(out.Blockers, err.Error())
		out.CanRetire = approve && out.Version > 0
		return out, nil
	}
	out.CanEdit = write
	out.CanRetire = approve && out.Version > 0
	if out.Version == 0 {
		return out, nil
	}
	if err = s.checkExpertTeamConfigurationTx(ctx, auth, out, false); err != nil {
		if !expertTeamExpectedError(err) {
			return out, err
		}
		out.Blockers = append(out.Blockers, err.Error())
		return out, nil
	}
	out.Valid = true
	out.CanPublish = approve && out.State == "draft"
	return out, nil
}

func (s *Service) GetExpertTeam(ctx context.Context, actor Actor, project, team string, revision int) (out ExpertTeamConfiguration, err error) {
	if !handoffID(project, false) || !handoffID(team, false) || revision < 0 {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, team, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	out, err = s.expertTeamHeadTx(ctx, auth, team, revision)
	if err != nil {
		return out, err
	}
	if revision > 0 {
		clearExpertTeamActions(&out)
		return out, nil
	}
	return s.expertTeamProjectionTx(ctx, auth, out)
}

func (s *Service) ListExpertTeams(ctx context.Context, actor Actor, project string, limit int, before string) (out ExpertTeamConfigurationPage, err error) {
	out.Items = []ExpertTeamConfiguration{}
	if limit == 0 {
		limit = 20
	}
	if !handoffID(project, false) || limit < 1 || limit > 100 {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, "", purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	position, err := automationDecodeCursor(auth.currentPrincipal, project, "expert_teams", before)
	if err != nil {
		return out, err
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_expert_team_configurations WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT team_id FROM suma_expert_team_configurations WHERE tenant_id=? AND project_id=? AND team_id>? ORDER BY team_id LIMIT ?`), auth.target.tenantID, project, position, limit+1)
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
	if len(ids) > limit {
		ids = ids[:limit]
		out.NextCursor = automationEncodeCursor(auth.currentPrincipal, project, "expert_teams", ids[len(ids)-1])
	}
	for _, id := range ids {
		c, e := s.expertTeamHeadTx(ctx, auth, id, 0)
		if e != nil {
			return out, e
		}
		c, e = s.expertTeamProjectionTx(ctx, auth, c)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, c)
	}
	return out, nil
}

func (s *Service) GetExpertTeamPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out ExpertTeamPlan, err error) {
	if engine == nil || !handoffID(project, false) || !handoffID(id, false) {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	out, err = s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	return s.expertTeamPlanProjectionTx(ctx, engine, auth, out)
}

func (s *Service) checkExpertTeamPreviewReadTx(ctx context.Context, auth *authorizedCommand, plan TaskDispatchPlan) error {
	if plan.State != "ready" {
		return expertError("expert_team_changed", expertPlanChanged)
	}
	current, err := s.taskContextForDispatchTx(ctx, auth, plan.ProjectID, plan.TaskID, knowledgeReferences(plan.Context.Knowledge))
	if err != nil {
		return err
	}
	if digest(stringify(current)) != digest(stringify(plan.Context)) {
		return expertError("expert_team_changed", expertPlanChanged)
	}
	route, err := dag.ReadProjectExecutionRouteTx(ctx, auth.tx, s.Dialect, auth.target.tenantID, plan.ProjectID)
	if err != nil {
		return err
	}
	if plan.ExecutionRoute == nil || *plan.ExecutionRoute != route {
		return expertError("expert_team_changed", expertSource)
	}
	profile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, plan.ProjectID, plan.ToolProfileID)
	if err != nil {
		return err
	}
	if err = s.checkDispatchProfileRouteTx(ctx, auth.tx, auth.target.tenantID, plan.ProjectID, profile, route); err != nil {
		return err
	}
	return s.validateModelExecutionPlanTx(ctx, auth.tx, auth.target.tenantID, plan.ProjectID, profile, plan)
}
