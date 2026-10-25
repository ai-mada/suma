package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
