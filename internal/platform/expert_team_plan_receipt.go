package platform

import (
	"context"
	"encoding/json"

	"github.com/ai-mada/suma/internal/dag"
)

// Recover the original effect, not the latest reviewed source. Current facts
// may update its existing run references, but a superseded receipt grants no
// current plan actions. Every caller has reauthorized before reading the key.
func (s *Service) expertTeamPlanReceiptTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, raw, planID, teamID string) (out ExpertTeamPlan, err error) {
	if auth == nil {
		return out, forbidden()
	}
	if err = auth.assertTx(auth.tx, auth.target, purposeProjectWrite); err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if !handoffID(out.ID, false) || out.ProjectID != auth.target.projectID || out.Version < 1 || out.Revision < 1 || out.Configuration.ProjectID != out.ProjectID || out.Configuration.TeamID != out.TeamID || out.Configuration.Version != out.ConfigurationVersion || planID != "" && (out.ID != planID || auth.target.resourceID != planID) || teamID != "" && (out.TeamID != teamID || auth.target.resourceID != teamID) {
		return out, expertError("expert_team_changed", expertPlanChanged)
	}
	head, err := s.expertTeamPlanTx(ctx, auth.tx, auth.target.tenantID, out.ProjectID, out.ID)
	if err != nil {
		return out, err
	}
	if head.TeamID != out.TeamID {
		return out, expertError("expert_team_changed", expertPlanChanged)
	}
	out, err = s.expertTeamPlanProjectionTx(ctx, engine, auth, out)
	if err == nil && head.Version != out.Version {
		out.CanStart = false
		out.AllowedActions = []string{}
		out.Valid = false
		out.Blockers = expertAppendReason(out.Blockers, expertPlanChanged)
	}
	return out, err
}
