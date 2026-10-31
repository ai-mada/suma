package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
)

// A design-governed task must execute the same design recorded in the formal
// dispatch plan. A generic run or editable Record.Data cannot prove adoption.
// The caller owns the current-authorized tx and (for acceptance) project guard.
func (s *Service) checkTaskRequirementDesignRunTx(ctx context.Context, auth *authorizedCommand, project, taskID, runID string, requireStarted bool) error {
	if err := assertProjectRead(auth, project); err != nil {
		return err
	}
	current, err := s.resolveTaskRequirementDesignTx(auth.ctx, auth, project, taskID)
	if err != nil || current == nil {
		return err
	}
	var raw, state string
	if err = auth.tx.QueryRowContext(auth.ctx, s.q(`SELECT payload,state FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND run_id=?`), auth.target.tenantID, project, runID).Scan(&raw, &state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			cloud, found, e := s.workBuddyTaskDispatchRunSourceTx(ctx, auth.tx, auth.target.tenantID, project, runID)
			if e != nil {
				return e
			}
			if found {
				allowed := cloud.State == "started" || !requireStarted && cloud.State == "ready"
				if cloud.TaskID != taskID || cloud.Context.Design == nil || digest(stringify(current)) != digest(stringify(cloud.Context.Design)) || !allowed {
					return conflict("执行的需求设计依据已失效，需要重新派发")
				}
				return nil
			}
			// A trusted composition is also a real code-producing task run. Its
			// immutable domain binding is written in the same Core transaction
			// before BindRunTx; arbitrary workflow config cannot supply this proof.
			r, found, e := s.candidateCompositionExecutionRecordTx(ctx, auth.tx, auth.target.tenantID, project, runID)
			if e != nil {
				return e
			}
			if found && r.Execution != nil && ccmp.ValidateCandidateCompositionExecution(*r.Execution) == nil && r.Plan.TaskID == taskID && r.Plan.RunID == runID && r.Execution.PlanID == r.Plan.ID && r.Execution.PlanVersion == r.Plan.Version && r.Plan.Context.Design != nil && digest(stringify(current)) == digest(stringify(r.Plan.Context.Design)) {
				if requireStarted {
					var actual string
					if e = auth.tx.QueryRowContext(ctx, s.q(`SELECT run_id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), auth.target.tenantID, project, taskID).Scan(&actual); e != nil {
						return e
					}
					if actual != runID {
						return conflict("执行的需求设计依据已失效，需要重新派发")
					}
				}
				return nil
			}
			return conflict("设计任务需要关联含同一设计依据的正式派发运行")
		}
		return err
	}
	var plan TaskDispatchPlan
	if err = json.Unmarshal([]byte(raw), &plan); err != nil {
		return err
	}
	allowedState := state == "started" || !requireStarted && state == "ready"
	if plan.TaskID != taskID || plan.RunID != runID || plan.Context.Design == nil || digest(stringify(current)) != digest(stringify(plan.Context.Design)) || !allowedState {
		return conflict("执行的需求设计依据已失效，需要重新派发")
	}
	return nil
}
