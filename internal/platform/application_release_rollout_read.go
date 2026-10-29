package platform

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func (s *Service) applicationRolloutViewTx(ctx context.Context, tx *sql.Tx, p Principal, r ApplicationReleaseRollout, manage bool) (out ApplicationReleaseRolloutView, err error) {
	out = ApplicationReleaseRolloutView{Rollout: r, Items: []ApplicationReleaseRolloutItem{}, Blockers: []ApplicationReleaseSetBlocker{}, AllowedActions: []string{}, BasisCurrent: true}
	out.PlanGroup, err = s.applicationReleaseSetGroupReadTx(ctx, tx, p, r.ProjectID, r.ReleaseSetID, r.PlanGroupID)
	if err != nil {
		return out, err
	}
	if out.PlanGroup.SnapshotSHA256 != r.PlanGroupSHA256 {
		return out, conflict(applicationRolloutChanged)
	}
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, r.ProjectID, r.ParentRunID).Scan(&out.RunVersion, &out.State)
	if err != nil {
		return out, err
	}
	for _, m := range r.Members {
		item := ApplicationReleaseRolloutItem{ApplicationID: m.ApplicationID}
		var reason string
		err = tx.QueryRowContext(ctx, s.q(`SELECT state,reason FROM dag_nodes WHERE run_id=? AND id=?`), r.ParentRunID, m.ParentNodeID).Scan(&item.ParentNodeState, &reason)
		if err != nil {
			return out, err
		}
		if out.State == "paused" && strings.HasPrefix(reason, "rollout_") && out.PauseReason == "" {
			out.PauseReason = reason
		}
		err = tx.QueryRowContext(ctx, s.q(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, r.ProjectID, m.ChildRunID).Scan(&item.ChildRunState)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		if errors.Is(err, sql.ErrNoRows) {
			blockers, e := s.applicationRolloutMemberReadyTx(ctx, tx, p, r, m)
			if e != nil {
				return out, e
			}
			for _, b := range blockers {
				if b.Code != "rollout_dependency_unhealthy" {
					out.Blockers = append(out.Blockers, b)
				}
			}
		} else {
			plan, e := s.currentReleasePlanTx(ctx, tx, p.TenantID, r.ProjectID, m.PlanID)
			if e != nil {
				return out, e
			}
			if plan.SHA256 != m.PlanSHA256 || plan.Revision != m.PlanRevision || plan.ExecutionOperationID == "" {
				out.Blockers = append(out.Blockers, rolloutBlocker("rollout_plan_changed", m.ApplicationID, m.PlanID))
			} else {
				d, e := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, r.ProjectID, plan.ExecutionOperationID)
				if e != nil {
					return out, e
				}
				item.DeploymentOperationID, item.DeploymentState, item.EffectState, item.HealthState = d.OperationID, d.State, d.EffectState, d.HealthState
				if d.RunID != m.ChildRunID || d.NodeID != m.ChildNodeID {
					out.Blockers = append(out.Blockers, rolloutBlocker("rollout_plan_changed", m.ApplicationID, m.PlanID))
				}
				if d.State == "reconciling" {
					// This is the current read projection, not a replacement for
					// the provider's persisted last observation or effect receipt.
					item.DeploymentState, item.EffectState, item.HealthState = "reconciling", "unknown", "unknown"
					out.Blockers = append(out.Blockers, rolloutBlocker("rollout_operation_unknown", m.ApplicationID, d.OperationID))
				}
			}
			if item.ChildRunState == "failed" || item.ChildRunState == "cancelled" {
				out.Blockers = append(out.Blockers, rolloutBlocker("rollout_child_failed", m.ApplicationID, m.PlanID))
			} else if item.ChildRunState == "succeeded" {
				healthy, e := s.applicationRolloutMemberHealthyTx(ctx, tx, p.TenantID, r.ProjectID, m)
				if e != nil {
					return out, e
				}
				if !healthy {
					out.Blockers = append(out.Blockers, rolloutBlocker("rollout_operation_unknown", m.ApplicationID, m.PlanID))
				}
			} else {
				var unknown int
				if e = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND state='reconciling'`), m.ChildRunID).Scan(&unknown); e != nil {
					return out, e
				}
				if unknown > 0 {
					// The last stored deployment may still say queued when both
					// provider ACK and observation delivery were lost. Its old
					// not_started value cannot establish the present effect state.
					item.DeploymentState, item.EffectState, item.HealthState = "reconciling", "unknown", "unknown"
					out.Blockers = append(out.Blockers, rolloutBlocker("rollout_operation_unknown", m.ApplicationID, m.PlanID))
				}
			}
		}
		out.Items = append(out.Items, item)
	}
	if out.State == "paused" && out.PauseReason == "" {
		out.PauseReason = "rollout_paused_by_user"
	}
	out.BasisCurrent = len(out.Blockers) == 0
	if manage {
		if out.State == "running" {
			out.AllowedActions = append(out.AllowedActions, "pause")
		}
		if out.State == "paused" && out.BasisCurrent {
			out.AllowedActions = append(out.AllowedActions, "resume")
		}
		if out.State == "running" || out.State == "paused" {
			out.AllowedActions = append(out.AllowedActions, "cancel")
		}
	}
	return out, nil
}
func (s *Service) GetApplicationReleaseRollout(ctx context.Context, actor Actor, project, setID, id string) (out ApplicationReleaseRolloutView, err error) {
	if !testPlanActor(actor) || !handoffID(setID, false) || !handoffID(id, false) {
		return out, forbidden()
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	r, err := s.applicationRolloutReadTx(ctx, auth.tx, auth.currentPrincipal, project, setID, id)
	if err != nil {
		return out, err
	}
	manage := actor.kind == actorBrowser && s.releaseActorAdminTx(ctx, auth.tx, auth.target.tenantID, project, auth.currentPrincipal.ID) == nil
	return s.applicationRolloutViewTx(ctx, auth.tx, auth.currentPrincipal, r, manage)
}
func (s *Service) ListApplicationReleaseRollouts(ctx context.Context, actor Actor, project, setID, group string, limit int, before string) (out ApplicationReleaseRolloutPage, err error) {
	out.Items = []ApplicationReleaseRolloutView{}
	if !testPlanActor(actor) || !handoffID(setID, false) || group != "" && !handoffID(group, false) || limit < 0 || limit > 100 || before != "" && !handoffID(before, false) {
		return out, invalid(applicationRolloutInvalid)
	}
	if limit == 0 {
		limit = 20
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, setID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	if _, err = s.applicationReleaseSetReadTx(ctx, auth.tx, auth.currentPrincipal, project, setID); err != nil {
		return out, err
	}
	base := ` FROM suma_application_release_rollouts WHERE tenant_id=? AND project_id=? AND set_id=?`
	args := []any{auth.target.tenantID, project, setID}
	if group != "" {
		base += ` AND group_id=?`
		args = append(args, group)
	}
	if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*)`+base), args...).Scan(&out.Total); err != nil {
		return out, err
	}
	query := `SELECT id` + base
	if before != "" {
		var at string
		if err = auth.tx.QueryRowContext(ctx, s.q(`SELECT created_at`+base+` AND id=?`), append(append([]any{}, args...), before)...).Scan(&at); errors.Is(err, sql.ErrNoRows) {
			return out, invalid(applicationRolloutInvalid)
		} else if err != nil {
			return out, err
		}
		query += ` AND (created_at<? OR (created_at=? AND id<?))`
		args = append(args, at, at, before)
	}
	query += ` ORDER BY created_at DESC,id DESC LIMIT ?`
	args = append(args, limit+1)
	rows, err := auth.tx.QueryContext(ctx, s.q(query), args...)
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
		out.NextCursor = ids[limit-1]
		ids = ids[:limit]
	}
	manage := actor.kind == actorBrowser && s.releaseActorAdminTx(ctx, auth.tx, auth.target.tenantID, project, auth.currentPrincipal.ID) == nil
	for _, id := range ids {
		r, e := s.applicationRolloutReadTx(ctx, auth.tx, auth.currentPrincipal, project, setID, id)
		if e != nil {
			return out, e
		}
		v, e := s.applicationRolloutViewTx(ctx, auth.tx, auth.currentPrincipal, r, manage)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	out.CanPreview = manage
	return out, nil
}
