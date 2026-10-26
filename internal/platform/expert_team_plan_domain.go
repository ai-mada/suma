package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
)

func validExpertTeamItems(items []ExpertTeamPlanItemRequest) bool {
	if len(items) < 1 || len(items) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !handoffID(item.TaskID, false) || item.ExpectedTaskVersion < 1 || !expertRoleKey.MatchString(item.RoleKey) || seen[item.TaskID] {
			return false
		}
		seen[item.TaskID] = true
	}
	return true
}

func (s *Service) CreateExpertTeamPlan(ctx context.Context, engine *dag.Engine, actor Actor, project string, req ExpertTeamPlanCreateRequest) (out ExpertTeamPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(req.TeamID, false) || req.ExpectedConfigurationVersion < 1 || !validExpertTeamItems(req.Items) || !planningKey(req.IdempotencyKey) {
		return out, expertError("expert_team_invalid", expertInvalid)
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
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	out = ExpertTeamPlan{ID: newID(), ProjectID: project, TeamID: req.TeamID, TeamName: configuration.TeamName, Version: 1, Revision: 1, State: "ready", ConfigurationVersion: configuration.Version, Configuration: configuration, Items: []ExpertTeamPlanItem{}, CreatedAt: automationStamp(clock), Valid: true, Blockers: []string{}, AllowedActions: []string{}}
	nodes := 0
	for _, request := range req.Items {
		item, e := s.expertTeamSourceItemTx(ctx, engine, auth, configuration, request, 1)
		if e != nil {
			return out, e
		}
		nodes += len(item.Plan.Workflow.Nodes)
		if nodes > 10000 {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
		out.Items = append(out.Items, item)
	}
	if err = s.compileExpertTeamGraphTx(ctx, auth, &out); err != nil {
		return out, err
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_expert_team_plans(id,tenant_id,project_id,team_id,version,state,expires_at,payload)VALUES(?,?,?,?,?,?,?,?)`), out.ID, auth.target.tenantID, project, out.TeamID, out.Version, out.State, "", stringify(out)); err != nil {
		return out, err
	}
	for _, item := range out.Items {
		if err = s.reserveExpertTeamItemTx(ctx, auth, out.ID, item); err != nil {
			return out, err
		}
	}
	if err = s.saveExpertTeamPlanRevisionTx(ctx, auth, out); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "expert_team.preview", out.ID, out.TeamName); err != nil {
		return out, err
	}
	out, err = s.expertTeamPlanProjectionTx(ctx, engine, auth, out)
	if err != nil {
		return out, err
	}
	if err = auth.tx.Commit(); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) reserveExpertTeamItemTx(ctx context.Context, auth *authorizedCommand, plan string, item ExpertTeamPlanItem) error {
	planID, runID := expertTeamReservedIDs(plan, item)
	_, err := auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_expert_team_plan_items(task_plan_id,tenant_id,project_id,expert_plan_id,run_id)VALUES(?,?,?,?,?)`), planID, auth.target.tenantID, auth.target.projectID, plan, runID)
	return err
}

// All compilation is complete before Core freezes its immutable inputs. A
// second authority transaction in advance compares the complete source again.
func (s *Service) prepareExpertTeamReady(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out ExpertTeamPlan, ready []ExpertTeamPlanItem, err error) {
	ready = []ExpertTeamPlanItem{}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, ready, err
	}
	defer close()
	out, err = s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, ready, err
	}
	if out.State != "ready" && out.State != "active" {
		return out, ready, expertError("expert_team_changed", expertPlanChanged)
	}
	if err = s.validateExpertTeamPlanTx(ctx, engine, auth, out, true); err != nil {
		return out, ready, err
	}
	for _, item := range out.Items {
		if item.RunID != "" {
			continue
		}
		accepted, e := s.expertTeamSelectedDependenciesAcceptedTx(ctx, auth.tx, auth.target.tenantID, project, out, item)
		if e != nil {
			return out, ready, e
		}
		if !accepted {
			continue
		}
		item.Plan.ID, item.Plan.RunID = expertTeamReservedIDs(id, item)
		ready = append(ready, item)
	}
	return out, ready, nil
}

func (s *Service) advanceExpertTeamPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, expectedVersion, generation int, items []ExpertTeamPlanItem, operationID string, finish func(context.Context, *authorizedCommand, *ExpertTeamPlan) error) (out ExpertTeamPlan, err error) {
	requestCtx := ctx
	if len(items) < 1 || len(items) > 16 {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	prepared := make([]dag.ManagedRunRequest, 0, len(items))
	byRun := map[string]ExpertTeamPlanItem{}
	for _, item := range items {
		if item.Plan.ID == "" || item.Plan.RunID == "" {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
		if _, exists := byRun[item.Plan.RunID]; exists {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
		byRun[item.Plan.RunID] = item
		prepared = append(prepared, dag.ManagedRunRequest{RunID: item.Plan.RunID, Workflow: item.Plan.Workflow})
	}
	authority := s.engineAuthority(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectWrite)
	defer authority.close()
	ctx = authority.ctx
	_, err = engine.CreateManagedRunsBound(ctx, actor.credential.tenantID, project, operationID, prepared, func(tx *sql.Tx) error {
		if e := authority.before(tx); e != nil {
			return e
		}
		auth := authority.proof
		if e := dag.GuardProjectTx(ctx, tx, s.Dialect, auth.target.tenantID, project); e != nil {
			return e
		}
		current, e := s.expertTeamPlanTx(ctx, tx, auth.target.tenantID, project, id)
		if e != nil {
			return e
		}
		if current.Version != expectedVersion {
			return expertError("expert_team_changed", expertPlanChanged)
		}
		if generation == 0 {
			if current.State != "ready" {
				return expertError("expert_team_changed", expertPlanChanged)
			}
		} else {
			if current.State != "active" || current.ActivationGeneration != generation {
				return expertError("expert_team_changed", expertPlanChanged)
			}
			if e = s.checkExpertTeamActivationTx(ctx, tx, auth.target.tenantID, project, id, generation, current.Revision, actor); e != nil {
				return e
			}
		}
		if e = s.validateExpertTeamPlanTx(ctx, engine, auth, current, true); e != nil {
			return e
		}
		for _, candidate := range items {
			found := false
			for _, item := range current.Items {
				if item.TaskID != candidate.TaskID {
					continue
				}
				found = true
				if item.RunID != "" || expertTeamSourceDigest(item) != expertTeamSourceDigest(candidate) {
					return expertError("expert_team_changed", expertSource)
				}
				planID, runID := expertTeamReservedIDs(id, item)
				if planID != candidate.Plan.ID || runID != candidate.Plan.RunID {
					return expertError("expert_team_changed", expertPlanChanged)
				}
				accepted, e := s.expertTeamSelectedDependenciesAcceptedTx(ctx, tx, auth.target.tenantID, project, current, item)
				if e != nil {
					return e
				}
				if !accepted {
					return expertError("expert_team_ineligible", expertWaiting)
				}
			}
			if !found {
				return expertError("expert_team_invalid", expertInvalid)
			}
			copy := candidate.Plan
			if e = s.persistTaskDispatchSnapshotTx(ctx, auth, &copy); e != nil {
				return e
			}
			candidate.Plan = copy
			byRun[copy.RunID] = candidate
		}
		out = current
		return nil
	}, func(tx *sql.Tx, runID string) error {
		item, ok := byRun[runID]
		if !ok {
			return expertError("expert_team_invalid", expertInvalid)
		}
		auth := authority.proof
		if e := s.checkTaskDispatchBoundRouteTx(ctx, tx, auth.currentPrincipal, project, item.Plan, runID); e != nil {
			return e
		}
		if e := s.BindRunTx(ctx, tx, auth, project, item.TaskID, runID, item.TaskVersion); e != nil {
			return e
		}
		if _, e := tx.ExecContext(ctx, s.q(`UPDATE suma_task_dispatch_plans SET version=version+1,state='started' WHERE tenant_id=? AND project_id=? AND id=? AND version=1 AND state='ready'`), auth.target.tenantID, project, item.Plan.ID); e != nil {
			return e
		}
		return s.commandActivity(ctx, auth, "task.dispatched", item.TaskID, item.Plan.Context.Task.Title)
	}, func(tx *sql.Tx, runs []dag.Run) error {
		previous := out.Version
		out.Version++
		out.State = "active"
		out.CanStart = false
		out.AllowedActions = []string{}
		out.Blockers = []string{}
		for _, run := range runs {
			candidate := byRun[run.ID]
			for i := range out.Items {
				if out.Items[i].TaskID == candidate.TaskID {
					candidate.DispatchPlanID, candidate.RunID = candidate.Plan.ID, run.ID
					candidate.Plan.Version, candidate.Plan.State = 2, "started"
					candidate.RunState, candidate.State, candidate.Blockers = run.State, "queued", []string{}
					out.Items[i] = candidate
				}
			}
		}
		if finish != nil {
			if e := finish(ctx, authority.proof, &out); e != nil {
				return e
			}
		}
		return s.saveExpertTeamPlanHeadTx(ctx, authority.proof, previous, out)
	})
	if err != nil {
		return out, err
	}
	authority.close()
	return s.GetExpertTeamPlan(requestCtx, engine, actor, project, id)
}

func (s *Service) StartExpertTeamPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req ExpertTeamPlanStartRequest) (out ExpertTeamPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || !planningKey(req.IdempotencyKey) {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	if !req.AcknowledgeDispatch {
		return out, expertError("expert_team_ack_required", expertAck)
	}
	receiptCtx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(receiptCtx, auth, req.IdempotencyKey, map[string]any{"expert_team_start": id, "request": req})
	if err != nil {
		close()
		return out, err
	}
	if replay {
		out, err = s.expertTeamPlanTx(receiptCtx, auth.tx, auth.target.tenantID, project, id)
		if err == nil {
			out, err = s.expertTeamPlanProjectionTx(receiptCtx, engine, auth, out)
		}
		close()
		return out, err
	}
	close()
	plan, ready, err := s.prepareExpertTeamReady(ctx, engine, actor, project, id)
	if err != nil {
		return out, err
	}
	if plan.State != "ready" || plan.Version != req.ExpectedVersion || len(ready) == 0 {
		return out, expertError("expert_team_changed", expertPlanChanged)
	}
	out, err = s.advanceExpertTeamPlan(ctx, engine, actor, project, id, req.ExpectedVersion, 0, ready, "expert-start-"+digest(actor.credential.subjectID+":"+req.IdempotencyKey), func(ctx context.Context, auth *authorizedCommand, plan *ExpertTeamPlan) error {
		plan.ActivationGeneration++
		if e := s.saveExpertTeamActivationTx(ctx, auth, *plan); e != nil {
			return e
		}
		if e := s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, *plan); e != nil {
			return e
		}
		return s.commandActivity(ctx, auth, "expert_team.started", id, plan.TeamName)
	})
	if err != nil { // A competing identical request may have committed while this
		// request was preparing. Recover only its current-authorized receipt.
		_, retryAuth, retryClose, e := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
		if e != nil {
			return out, err
		}
		defer retryClose()
		_, found, e := s.commandReceipt(ctx, retryAuth, req.IdempotencyKey, map[string]any{"expert_team_start": id, "request": req})
		if e == nil && found {
			current, e := s.expertTeamPlanTx(ctx, retryAuth.tx, retryAuth.target.tenantID, project, id)
			if e != nil {
				return out, e
			}
			return s.expertTeamPlanProjectionTx(ctx, engine, retryAuth, current)
		}
	}
	return out, err
}
