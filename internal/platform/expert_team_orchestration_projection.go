package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

// Formal evidence reads below carry only an exact persisted tenant/task/run
// scope. They cannot construct an Actor or authorize dispatch or acceptance.
func (s *Service) expertTeamFormalAcceptanceTx(ctx context.Context, tx *sql.Tx, tenant, project, taskID, exactRun string) (bool, Record, error) {
	task, err := scanRecord(tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`)), tenant, project, taskID))
	if err != nil {
		return false, task, err
	}
	verified, _ := task.Data["verified_run_id"].(string)
	if task.Status != "accepted" || task.RunID == "" || verified != task.RunID || exactRun != "" && task.RunID != exactRun {
		return false, task, nil
	}
	var reviews int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? AND decision='accepted'`), tenant, project, taskID, task.RunID).Scan(&reviews); err != nil {
		return false, task, err
	}
	if reviews == 0 {
		return false, task, nil
	}
	if err = s.checkCurrentTaskEvidenceTx(ctx, tx, Principal{TenantID: tenant}, project, taskID, map[string]bool{}, map[string]bool{}); err != nil {
		if expertTeamExpectedError(err) {
			return false, task, nil
		}
		return false, task, err
	}
	return true, task, nil
}
func (s *Service) expertTeamSelectedDependenciesAcceptedTx(ctx context.Context, tx *sql.Tx, tenant, project string, plan ExpertTeamPlan, item ExpertTeamPlanItem) (bool, error) {
	selected := map[string]ExpertTeamPlanItem{}
	for _, candidate := range plan.Items {
		selected[candidate.TaskID] = candidate
	}
	for _, id := range item.DependsOn {
		expected := ""
		if predecessor, ok := selected[id]; ok {
			if predecessor.RunID == "" {
				return false, nil
			}
			expected = predecessor.RunID
		}
		accepted, _, err := s.expertTeamFormalAcceptanceTx(ctx, tx, tenant, project, id, expected)
		if err != nil {
			return false, err
		}
		if !accepted {
			return false, nil
		}
	}
	return true, nil
}
func expertAppendReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
func (s *Service) expertTeamProjectFactsTx(ctx context.Context, tx *sql.Tx, tenant string, plan ExpertTeamPlan) (ExpertTeamPlan, error) {
	selected := map[string]ExpertTeamPlanItem{}
	for _, item := range plan.Items {
		selected[item.TaskID] = item
	}
	plan.Progress = ExpertTeamPlanProgress{Total: len(plan.Items)}
	plan.Blockers = []string{}
	for i := range plan.Items {
		item := &plan.Items[i]
		item.Blockers = []string{}
		item.Dependencies = []ExpertTeamDependencyFact{}
		waiting, wrong := false, false
		for _, id := range item.DependsOn {
			predecessor, isSelected := selected[id]
			exact := ""
			if isSelected {
				exact = predecessor.RunID
			}
			accepted, task, err := s.expertTeamFormalAcceptanceTx(ctx, tx, tenant, plan.ProjectID, id, exact)
			if err != nil {
				return plan, err
			}
			if isSelected && exact == "" {
				accepted = false
			}
			if isSelected && task.Status == "accepted" && (exact == "" || task.RunID != exact) {
				wrong = true
			}
			if !accepted {
				waiting = true
			}
			item.Dependencies = append(item.Dependencies, ExpertTeamDependencyFact{TaskID: id, Selected: isSelected, Accepted: accepted, TaskVersion: task.Version, RunID: task.RunID})
		}
		if item.RunID == "" {
			item.RunState = ""
			item.DispatchPlanID = ""
			item.Plan.ID = ""
			item.Plan.RunID = ""
			item.Plan.Valid = false
			switch {
			case plan.State == "stopped":
				item.State = "stopped"
				item.Blockers = append(item.Blockers, expertStopped)
			case wrong:
				item.State = "blocked"
				item.Blockers = append(item.Blockers, expertWrongRun)
			case waiting:
				item.State = "waiting_dependencies"
				item.Blockers = append(item.Blockers, expertWaiting)
			default:
				item.State = "ready"
			}
		} else {
			var state string
			var reconciling int
			if err := tx.QueryRowContext(ctx, s.q(`SELECT r.state, (SELECT COUNT(*) FROM dag_nodes n WHERE n.run_id=r.id AND n.state='reconciling') FROM dag_runs r WHERE r.tenant_id=? AND r.project_id=? AND r.id=?`), tenant, plan.ProjectID, item.RunID).Scan(&state, &reconciling); err != nil {
				return plan, err
			}
			item.RunState = state
			accepted, task, err := s.expertTeamFormalAcceptanceTx(ctx, tx, tenant, plan.ProjectID, item.TaskID, item.RunID)
			if err != nil {
				return plan, err
			}
			switch {
			case reconciling > 0:
				item.State = "blocked"
				item.Blockers = append(item.Blockers, expertRunBlocked)
			case accepted:
				item.State = "accepted"
			case task.Status == "accepted" && task.RunID != item.RunID:
				item.State = "blocked"
				item.Blockers = append(item.Blockers, expertWrongRun)
			case task.Status == "accepted":
				item.State = "blocked"
				item.Blockers = append(item.Blockers, expertRunBlocked)
			case state == "succeeded" && task.Status != "blocked":
				item.State = "awaiting_review"
			case state == "failed" || state == "cancelled" || state == "skipped" || state == "reconciling" || state == "paused" || state == "cancelling" || state == "succeeded":
				item.State = "blocked"
				item.Blockers = append(item.Blockers, expertRunBlocked)
			default:
				var attempts int
				if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND attempt>0`), item.RunID).Scan(&attempts); err != nil {
					return plan, err
				}
				if attempts == 0 {
					item.State = "queued"
				} else {
					item.State = "executing"
				}
			}
		}
		switch item.State {
		case "waiting_dependencies":
			plan.Progress.Waiting++
		case "ready":
			plan.Progress.Ready++
		case "queued", "executing":
			plan.Progress.Executing++
		case "awaiting_review":
			plan.Progress.AwaitingReview++
		case "accepted":
			plan.Progress.Accepted++
		case "stopped":
			plan.Progress.Stopped++
		default:
			plan.Progress.Blocked++
		}
	}
	if len(plan.Items) > 0 && plan.Progress.Accepted == len(plan.Items) && (plan.State == "active" || plan.State == "paused") {
		plan.State = "completed"
	}
	return plan, nil
}

func (s *Service) expertTeamPlanProjectionTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, out ExpertTeamPlan) (ExpertTeamPlan, error) {
	out.CanStart = false
	out.AllowedActions = []string{}
	out.Valid = true
	clearExpertTeamActions(&out.Configuration)
	var err error
	out, err = s.expertTeamProjectFactsTx(ctx, auth.tx, auth.target.tenantID, out)
	if err != nil {
		return out, err
	}
	for i := range out.Items {
		item := &out.Items[i]
		if item.RunID != "" || out.State == "stopped" {
			continue
		}
		if err = s.checkExpertTeamPendingSourceTx(ctx, engine, auth, out, *item, false); err != nil {
			if !expertTeamExpectedError(err) {
				return out, err
			}
			item.State = "blocked"
			item.Blockers = expertAppendReason(item.Blockers, err.Error())
			out.Blockers = expertAppendReason(out.Blockers, err.Error())
			out.Valid = false
		}
	}
	// Recount after current source eligibility, preserving fixed source/run data.
	out.Progress = ExpertTeamPlanProgress{Total: len(out.Items)}
	for _, item := range out.Items {
		switch item.State {
		case "waiting_dependencies":
			out.Progress.Waiting++
		case "ready":
			out.Progress.Ready++
		case "queued", "executing":
			out.Progress.Executing++
		case "awaiting_review":
			out.Progress.AwaitingReview++
		case "accepted":
			out.Progress.Accepted++
		case "stopped":
			out.Progress.Stopped++
		default:
			out.Progress.Blocked++
		}
	}
	write, _, err := s.expertTeamCapabilitiesTx(ctx, auth)
	if err != nil {
		return out, err
	}
	if write {
		switch out.State {
		case "ready":
			out.CanStart = out.Valid
			out.AllowedActions = []string{"start", "revise", "stop"}
		case "active":
			out.AllowedActions = []string{"pause", "stop"}
		case "paused":
			out.AllowedActions = []string{"resume", "stop", "revise"}
		}
	}
	var a expertTeamActivation
	a, err = s.expertTeamActivationTx(ctx, auth.tx, auth.target.tenantID, out.ProjectID, out.ID, false)
	if err == nil && a.LastError != "" {
		out.Blockers = expertAppendReason(out.Blockers, a.LastError)
		if write && out.State == "active" && a.LastError == expertAuthority {
			out.AllowedActions = expertAppendReason(out.AllowedActions, "resume")
		}
	} else if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	return out, nil
}

// Projection has no dispatch authority. It updates only facts derived from this
// head's existing immutable task/run references; no Actor or task writes occur.
func (s *Service) projectExpertTeamPlanFacts(ctx context.Context, tenant, project, id string) (ExpertTeamPlan, error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return ExpertTeamPlan{}, err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return ExpertTeamPlan{}, err
	}
	if err = s.lockOrganization(ctx, tx, tenant); err != nil {
		return ExpertTeamPlan{}, err
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, tenant, project); err != nil {
		return ExpertTeamPlan{}, err
	}
	old, err := s.expertTeamPlanTx(ctx, tx, tenant, project, id)
	if err != nil {
		return old, err
	}
	var independent ExpertTeamPlan
	if err = json.Unmarshal([]byte(stringify(old)), &independent); err != nil {
		return old, err
	}
	out, err := s.expertTeamProjectFactsTx(ctx, tx, tenant, independent)
	if err != nil {
		return old, err
	}
	// User read eligibility never becomes a persisted system action.
	old.Valid, out.Valid = false, false
	old.CanStart, out.CanStart = false, false
	old.AllowedActions, out.AllowedActions = nil, nil
	old.Blockers, out.Blockers = nil, nil
	if stringify(old) != stringify(out) {
		out.Version++
		res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_expert_team_plans SET version=?,state=?,payload=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), out.Version, out.State, stringify(out), tenant, project, id, old.Version)
		if e != nil {
			return old, e
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return old, expertError("expert_team_changed", expertPlanChanged)
		}
	}
	if out.State == "completed" {
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_expert_team_plan_activations SET state='completed' WHERE tenant_id=? AND project_id=? AND plan_id=? AND generation=?`), tenant, project, id, out.ActivationGeneration); err != nil {
			return old, err
		}
	}
	return out, tx.Commit()
}
