package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) SaveExpertTeam(ctx context.Context, engine *dag.Engine, actor Actor, project, team string, req ExpertTeamSaveRequest) (out ExpertTeamConfiguration, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(team, false) || req.ExpectedVersion < 0 || req.ExpectedVersion >= 2147483646 || len(req.Roles) < 1 || len(req.Roles) > 8 || !planningKey(req.IdempotencyKey) {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	seen := map[string]bool{}
	for _, r := range req.Roles {
		if err = validateExpertRole(r); err != nil {
			return out, err
		}
		if seen[r.Key] {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
		seen[r.Key] = true
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, team, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"expert_team_save": team, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return s.expertTeamProjectionTx(ctx, auth, out)
	}
	old, err := s.expertTeamHeadTx(ctx, auth, team, 0)
	if err != nil {
		return out, err
	}
	if old.Version != req.ExpectedVersion {
		return out, expertError("expert_team_changed", expertChanged)
	}
	if old.State == "retired" {
		return out, expertError("expert_team_retired", expertRetired)
	}
	name, version, err := s.expertTeamIdentityTx(ctx, auth, team)
	if err != nil {
		return out, err
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	out = ExpertTeamConfiguration{ProjectID: project, TeamID: team, TeamName: name, TeamVersion: version, Version: old.Version + 1, State: "draft", Roles: []ExpertTeamRole{}, CreatedAt: old.CreatedAt, UpdatedAt: automationStamp(clock), Valid: true, Blockers: []string{}}
	if out.CreatedAt == "" {
		out.CreatedAt = out.UpdatedAt
	}
	for _, r := range req.Roles {
		role, e := s.expertTeamResolveRoleTx(ctx, auth, team, r, true)
		if e != nil {
			return out, e
		}
		out.Roles = append(out.Roles, role)
	}
	if err = s.saveExpertTeamTx(ctx, auth, old, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "expert_team.saved", team, name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	out, err = s.expertTeamProjectionTx(ctx, auth, out)
	if err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) CommandExpertTeam(ctx context.Context, engine *dag.Engine, actor Actor, project, team string, req ExpertTeamCommandRequest) (out ExpertTeamConfiguration, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(team, false) || req.ExpectedVersion < 1 || req.ExpectedVersion >= 2147483646 || req.Action != "publish" && req.Action != "retire" || !planningText(req.Reason, 2000) || !planningKey(req.IdempotencyKey) {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, team, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"expert_team_command": team, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return s.expertTeamProjectionTx(ctx, auth, out)
	}
	old, err := s.expertTeamHeadTx(ctx, auth, team, 0)
	if err != nil {
		return out, err
	}
	if old.Version != req.ExpectedVersion {
		return out, expertError("expert_team_changed", expertChanged)
	}
	if old.State == "retired" {
		return out, expertError("expert_team_retired", expertRetired)
	}
	out = old
	if req.Action == "publish" {
		if old.State != "draft" {
			return out, expertError("expert_team_changed", expertChanged)
		}
		if err = s.checkExpertTeamConfigurationTx(ctx, auth, old, true); err != nil {
			return out, err
		}
		out.State = "published"
	} else {
		out.State = "retired"
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	out.Version++
	out.UpdatedAt = automationStamp(clock)
	out.CanEdit = false
	out.CanPublish = false
	out.CanRetire = false
	if err = s.saveExpertTeamTx(ctx, auth, old, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "expert_team."+req.Action, team, req.Reason); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	out, err = s.expertTeamProjectionTx(ctx, auth, out)
	if err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) CreateExpertTeamPlan(ctx context.Context, engine *dag.Engine, actor Actor, project string, req ExpertTeamPlanCreateRequest) (out ExpertTeamPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(req.TeamID, false) || req.ExpectedConfigurationVersion < 1 || len(req.Items) < 1 || len(req.Items) > 16 || !planningKey(req.IdempotencyKey) {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	seen := map[string]bool{}
	for _, item := range req.Items {
		if !handoffID(item.TaskID, false) || item.ExpectedTaskVersion < 1 || !expertRoleKey.MatchString(item.RoleKey) || seen[item.TaskID] {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
		seen[item.TaskID] = true
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, req.TeamID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"expert_team_plan": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		out, err = s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, project, out.ID)
		if err != nil {
			return out, err
		}
		return s.expertTeamPlanProjectionTx(ctx, engine, auth, out)
	}
	configuration, err := s.expertTeamHeadTx(ctx, auth, req.TeamID, 0)
	if err != nil {
		return out, err
	}
	if configuration.Version != req.ExpectedConfigurationVersion {
		return out, expertError("expert_team_changed", expertChanged)
	}
	if configuration.State != "published" {
		return out, expertError("expert_team_ineligible", expertUnpublished)
	}
	if err = s.checkExpertTeamConfigurationTx(ctx, auth, configuration, true); err != nil {
		return out, err
	}
	roles := map[string]ExpertTeamRole{}
	for _, r := range configuration.Roles {
		roles[r.Key] = r
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	out = ExpertTeamPlan{ID: newID(), ProjectID: project, TeamID: req.TeamID, TeamName: configuration.TeamName, Version: 1, State: "ready", ConfigurationVersion: configuration.Version, Configuration: configuration, Items: []ExpertTeamPlanItem{}, CreatedAt: automationStamp(clock), Valid: true, Blockers: []string{}}
	totalNodes := 0
	for _, item := range req.Items {
		role, ok := roles[item.RoleKey]
		if !ok {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
		if err = s.checkExpertTeamTaskTx(ctx, auth, item.TaskID); err != nil {
			return out, err
		}
		plan, e := s.createTaskDispatchPlanTx(ctx, engine, auth, project, TaskDispatchPreviewRequest{ProjectID: project, TaskID: item.TaskID, ExpectedTaskVersion: item.ExpectedTaskVersion, WorkflowID: role.WorkflowID, WorkflowVersion: role.WorkflowVersion, AgentNodeID: role.AgentNodeID, ToolProfileID: role.ToolProfileID, ExpectedToolVersion: role.ToolProfileVersion, Knowledge: role.Knowledge})
		if e != nil {
			return out, e
		}
		totalNodes += len(plan.Workflow.Nodes)
		if totalNodes > 10000 {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
		if out.ExpiresAt == "" || plan.ExpiresAt < out.ExpiresAt {
			out.ExpiresAt = plan.ExpiresAt
		}
		out.Items = append(out.Items, ExpertTeamPlanItem{RoleKey: role.Key, RoleTitle: role.Title, ResourceID: role.ResourceID, ResourceName: role.ResourceName, Plan: plan})
		if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_expert_team_plan_items(task_plan_id,tenant_id,project_id,expert_plan_id,run_id)VALUES(?,?,?,?,?)`), plan.ID, auth.target.tenantID, project, out.ID, plan.RunID); err != nil {
			return out, err
		}
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_expert_team_plans(id,tenant_id,project_id,team_id,version,state,expires_at,payload)VALUES(?,?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, project, out.TeamID, out.Version, out.State, out.ExpiresAt, stringify(out)); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "expert_team.preview", out.ID, out.TeamName); err != nil {
		return out, err
	}
	out.CanStart = true
	return out, auth.tx.Commit()
}

func (s *Service) validateExpertTeamPlanTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, plan ExpertTeamPlan, write bool) error {
	if plan.State != "ready" {
		return expertError("expert_team_changed", expertPlanChanged)
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= plan.ExpiresAt {
		return expertError("expert_team_ineligible", expertExpired)
	}
	head, err := s.expertTeamHeadTx(ctx, auth, plan.TeamID, 0)
	if err != nil {
		return err
	}
	if head.Version != plan.ConfigurationVersion {
		return expertError("expert_team_changed", expertChanged)
	}
	if head.State != "published" {
		return expertError("expert_team_ineligible", expertUnpublished)
	}
	if err = s.checkExpertTeamConfigurationTx(ctx, auth, head, write); err != nil {
		return err
	}
	for _, item := range plan.Items {
		current, err := s.taskDispatchPlanTx(ctx, auth.tx, auth.currentPrincipal, plan.ProjectID, item.Plan.ID)
		if err != nil {
			return err
		}
		if current.Version != item.Plan.Version || current.RunID != item.Plan.RunID {
			return expertError("expert_team_changed", expertPlanChanged)
		}
		if err = s.checkExpertTeamTaskTx(ctx, auth, current.TaskID); err != nil {
			return err
		}
		if write {
			if err = s.validateTaskDispatchPlanTx(ctx, auth.tx, engine, auth, plan.ProjectID, current); err != nil {
				return err
			}
		} else if err = s.checkExpertTeamPreviewReadTx(ctx, auth, current); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) StartExpertTeamPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req ExpertTeamPlanStartRequest) (out ExpertTeamPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || !planningKey(req.IdempotencyKey) {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	if !req.AcknowledgeDispatch {
		return out, expertError("expert_team_ack_required", expertAck)
	}
	plan, err := s.GetExpertTeamPlan(ctx, engine, actor, project, id)
	if err != nil {
		return out, err
	}
	items := make([]dag.ManagedRunRequest, len(plan.Items))
	byRun := map[string]ExpertTeamPlanItem{}
	for i, item := range plan.Items {
		items[i] = dag.ManagedRunRequest{RunID: item.Plan.RunID, Workflow: item.Plan.Workflow}
		byRun[item.Plan.RunID] = item
	}
	authority := s.engineAuthority(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectWrite)
	defer authority.close()
	ctx = authority.ctx
	var hash string
	_, err = engine.CreateManagedRunsBound(ctx, actor.credential.tenantID, project, "expert-"+digest(actor.Projection().ID+":"+req.IdempotencyKey), items, func(tx *sql.Tx) error {
		if e := authority.before(tx); e != nil {
			return e
		}
		if e := dag.GuardProjectTx(ctx, tx, s.Dialect, authority.proof.target.tenantID, project); e != nil {
			return e
		}
		var replay bool
		hash, replay, err = s.commandReceipt(ctx, authority.proof, req.IdempotencyKey, map[string]any{"expert_team_start": id, "request": req})
		if err != nil || replay {
			return err
		}
		current, e := s.expertTeamPlanTx(ctx, tx, authority.proof.target.tenantID, project, id)
		if e != nil {
			return e
		}
		if current.Version != req.ExpectedVersion {
			return expertError("expert_team_changed", expertPlanChanged)
		}
		return s.validateExpertTeamPlanTx(ctx, engine, authority.proof, current, true)
	}, func(tx *sql.Tx, runID string) error {
		item, ok := byRun[runID]
		if !ok {
			return expertError("expert_team_invalid", expertInvalid)
		}
		auth := authority.proof
		if e := s.checkTaskDispatchBoundRouteTx(ctx, tx, auth.currentPrincipal, project, item.Plan, runID); e != nil {
			return e
		}
		if e := s.BindRunTx(ctx, tx, auth, project, item.Plan.TaskID, runID, item.Plan.TaskVersion); e != nil {
			return e
		}
		res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_task_dispatch_plans SET version=version+1,state='started' WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND state='ready'`), auth.target.tenantID, project, item.Plan.ID, item.Plan.Version)
		if e != nil {
			return e
		}
		if n, e := res.RowsAffected(); e != nil {
			return e
		} else if n != 1 {
			return expertError("expert_team_changed", expertPlanChanged)
		}
		return s.commandActivity(ctx, auth, "task.dispatched", item.Plan.TaskID, item.Plan.Context.Task.Title)
	}, func(tx *sql.Tx, runs []dag.Run) error {
		out = plan
		out.State = "started"
		out.Version++
		out.Valid = true
		out.CanStart = false
		out.Blockers = []string{}
		for i := range out.Items {
			out.Items[i].Plan.State = "started"
			out.Items[i].Plan.Version++
			out.Items[i].RunState = runs[i].State
		}
		res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_expert_team_plans SET version=?,state='started',payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND state='ready'`), out.Version, stringify(out), authority.proof.target.tenantID, project, id, req.ExpectedVersion)
		if e != nil {
			return e
		}
		if n, e := res.RowsAffected(); e != nil {
			return e
		} else if n != 1 {
			return expertError("expert_team_changed", expertPlanChanged)
		}
		if e = s.saveCommandReceipt(ctx, authority.proof, req.IdempotencyKey, hash, out); e != nil {
			return e
		}
		return s.commandActivity(ctx, authority.proof, "expert_team.started", id, plan.TeamName)
	})
	if err != nil {
		return out, err
	}
	return s.GetExpertTeamPlan(ctx, engine, actor, project, id)
}
