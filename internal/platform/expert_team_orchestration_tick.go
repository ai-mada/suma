package platform

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/ai-mada/suma/internal/dag"
)

const expertTeamTickPlans = 16
const expertTeamTickRuns = 16
const expertTeamTickWave = 2

// TickExpertTeamPlans advances only bounded business readiness. The existing
// DAG remains the sole dispatcher and charges resources only when claimed.
func (s *Service) TickExpertTeamPlans(ctx context.Context, engine *dag.Engine) error {
	if engine == nil {
		return expertError("expert_team_invalid", expertInvalid)
	}
	clock, err := s.authorityNow(ctx, s.DB)
	if err != nil {
		return err
	}
	rows, err := s.DB.QueryContext(ctx, s.q(`SELECT a.tenant_id,a.project_id,a.plan_id FROM suma_expert_team_plan_activations a JOIN suma_expert_team_plans p ON p.id=a.plan_id AND p.tenant_id=a.tenant_id AND p.project_id=a.project_id WHERE a.state IN ('active','paused','stopped') AND a.next_check_at<=? ORDER BY a.next_check_at,a.last_checked_at,a.plan_id LIMIT ?`), automationStamp(clock), expertTeamTickPlans)
	if err != nil {
		return err
	}
	type ref struct{ tenant, project, id string }
	refs := []ref{}
	for rows.Next() {
		var r ref
		if err = rows.Scan(&r.tenant, &r.project, &r.id); err != nil {
			rows.Close()
			return err
		}
		refs = append(refs, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	failures := []error{}
	created := 0
	for _, r := range refs {
		if ctx.Err() != nil {
			failures = append(failures, ctx.Err())
			break
		}
		activation, e := s.expertTeamActivationTx(ctx, s.DB, r.tenant, r.project, r.id, false)
		if e != nil {
			if !expertTeamExpectedError(e) {
				failures = append(failures, e)
			}
			if activation.PlanID != "" {
				if cursorErr := s.expertTeamScheduleCheck(ctx, activation, expertAuthority); cursorErr != nil {
					failures = append(failures, cursorErr)
				}
			}
			continue
		}
		plan, e := s.projectExpertTeamPlanFacts(ctx, r.tenant, r.project, r.id)
		if e != nil {
			failures = append(failures, e)
			if cursorErr := s.expertTeamScheduleCheck(ctx, activation, expertSource); cursorErr != nil {
				failures = append(failures, cursorErr)
			}
			continue
		}
		reason := ""
		if plan.State == "active" && created < expertTeamTickRuns {
			actor, current, e := s.restoreExpertTeamActor(ctx, r.tenant, r.project, r.id)
			if e == nil && (current.Generation != plan.ActivationGeneration || current.SourceRevision != plan.Revision) {
				e = expertError("expert_team_changed", expertPlanChanged)
			}
			if e == nil {
				var ready []ExpertTeamPlanItem
				plan, ready, e = s.prepareExpertTeamReady(ctx, engine, actor, r.project, r.id)
				if e == nil && len(ready) > 0 {
					limit := expertTeamTickWave
					if left := expertTeamTickRuns - created; left < limit {
						limit = left
					}
					if len(ready) > limit {
						ready = ready[:limit]
					}
					ids := make([]string, len(ready))
					for i, item := range ready {
						ids[i] = item.TaskID + ":" + item.Plan.RunID
					}
					sort.Strings(ids)
					op := "expert-tick-" + digest(plan.ID+":"+stringify(plan.ActivationGeneration)+":"+strings.Join(ids, "|"))
					created += len(ready) // Budget submissions even if the response is lost after commit.
					_, e = s.advanceExpertTeamPlan(ctx, engine, actor, r.project, r.id, plan.Version, plan.ActivationGeneration, ready, op, nil)
				}
			}
			if e != nil {
				if expertTeamExpectedError(e) {
					reason = expertAuthority
					var api *APIError
					if errors.As(e, &api) && api.Status == 409 && api.Code != "expert_team_activation_unavailable" {
						if api.Message != expertPlanChanged {
							reason = api.Message
						} else {
							reason = ""
						}
					}
				} else {
					failures = append(failures, e)
				}
			}
		}
		if e = s.expertTeamScheduleCheck(ctx, activation, reason); e != nil {
			failures = append(failures, e)
		}
	}
	return errors.Join(failures...)
}
