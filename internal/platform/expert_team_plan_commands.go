package platform

import (
	"context"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) CommandExpertTeamPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req ExpertTeamPlanCommandRequest) (out ExpertTeamPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || req.ExpectedVersion >= 2147483646 || !planningText(req.Reason, 2000) || !planningKey(req.IdempotencyKey) {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	if req.Action != "pause" && req.Action != "resume" && req.Action != "stop" && req.Action != "revise" {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	if !req.Acknowledge {
		return out, expertError("expert_team_ack_required", expertAck)
	}
	if req.Action == "revise" {
		if !validExpertTeamItems(req.Items) || req.ExpectedConfigurationVersion < 1 {
			return out, expertError("expert_team_invalid", expertInvalid)
		}
	} else if len(req.Items) != 0 || req.ExpectedConfigurationVersion != 0 {
		return out, expertError("expert_team_invalid", expertInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"expert_team_plan_command": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		out, err = s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if err != nil {
			return out, err
		}
		return s.expertTeamPlanProjectionTx(ctx, engine, auth, out)
	}
	out, err = s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if out.Version != req.ExpectedVersion {
		return out, expertError("expert_team_changed", expertPlanChanged)
	}
	if out.State == "stopped" || out.State == "completed" {
		return out, expertError("expert_team_changed", expertStopped)
	}
	previous := out.Version
	out.Version++
	out.ActivationGeneration++
	out.AllowedActions = []string{}
	out.CanStart = false
	switch req.Action {
	case "pause":
		if out.State != "active" {
			return out, expertError("expert_team_changed", expertPlanChanged)
		}
		out.State = "paused"
		err = s.setExpertTeamActivationStateTx(ctx, auth, out, "paused")
	case "resume":
		if out.State != "paused" && out.State != "active" {
			return out, expertError("expert_team_changed", expertPlanChanged)
		}
		if err = s.validateExpertTeamPlanTx(ctx, engine, auth, out, true); err != nil {
			return out, err
		}
		out.State = "active"
		err = s.saveExpertTeamActivationTx(ctx, auth, out)
	case "stop":
		out.State = "stopped"
		for i := range out.Items {
			if out.Items[i].RunID == "" {
				out.Items[i].State = "stopped"
				out.Items[i].Blockers = []string{expertStopped}
			}
		}
		err = s.setExpertTeamActivationStateTx(ctx, auth, out, "stopped")
	case "revise":
		if out.State != "ready" && out.State != "paused" {
			return out, expertError("expert_team_changed", expertRevision)
		}
		if err = s.reviseExpertTeamPlanTx(ctx, engine, auth, &out, req); err != nil {
			return out, err
		}
		if out.State == "paused" {
			err = s.setExpertTeamActivationStateTx(ctx, auth, out, "paused")
		}
	}
	if err != nil {
		return out, err
	}
	if err = s.saveExpertTeamPlanHeadTx(ctx, auth, previous, out); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "expert_team.plan_"+req.Action, id, req.Reason); err != nil {
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

func (s *Service) reviseExpertTeamPlanTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, plan *ExpertTeamPlan, req ExpertTeamPlanCommandRequest) error {
	configuration, err := s.expertTeamHeadTx(ctx, auth, plan.TeamID, 0)
	if err != nil {
		return err
	}
	if configuration.Version != req.ExpectedConfigurationVersion {
		return expertError("expert_team_changed", expertChanged)
	}
	if configuration.State != "published" {
		return expertError("expert_team_ineligible", expertUnpublished)
	}
	if err = s.checkExpertTeamConfigurationTx(ctx, auth, configuration, true); err != nil {
		return err
	}
	requests := map[string]ExpertTeamPlanItemRequest{}
	for _, r := range req.Items {
		requests[r.TaskID] = r
	}
	plan.Revision++
	for i, item := range plan.Items {
		r, changed := requests[item.TaskID]
		if item.RunID != "" {
			if changed {
				return expertError("expert_team_changed", expertRevision)
			}
			continue
		}
		if plan.ConfigurationVersion != configuration.Version && !changed {
			return expertError("expert_team_changed", expertRevision)
		}
		if !changed {
			continue
		}
		fresh, e := s.expertTeamSourceItemTx(ctx, engine, auth, configuration, r, plan.Revision)
		if e != nil {
			return e
		}
		plan.Items[i] = fresh
		delete(requests, item.TaskID)
		if err = s.reserveExpertTeamItemTx(ctx, auth, plan.ID, fresh); err != nil {
			return err
		}
	}
	if len(requests) != 0 {
		return expertError("expert_team_changed", expertRevision)
	}
	plan.ConfigurationVersion, plan.Configuration = configuration.Version, configuration
	if err = s.compileExpertTeamGraphTx(ctx, auth, plan); err != nil {
		return err
	}
	if err = s.validateExpertTeamPlanTx(ctx, engine, auth, *plan, true); err != nil {
		return err
	}
	return s.saveExpertTeamPlanRevisionTx(ctx, auth, *plan)
}
