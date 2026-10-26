package platform

import (
	"context"
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
