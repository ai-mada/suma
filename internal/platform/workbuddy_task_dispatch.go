package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/ai-mada/suma/internal/dag"
	"time"
)

func (w *WorkBuddyConnections) CreateTaskDispatchPlan(ctx context.Context, engine *dag.Engine, actor Actor, project string, req WorkBuddyTaskDispatchPreviewRequest) (WorkBuddyTaskDispatchPlan, error) {
	var out WorkBuddyTaskDispatchPlan
	if w == nil || w.s == nil || engine == nil || !validWorkBuddyTaskDispatchPreview(project, req) {
		return out, invalid(workBuddyTaskDispatchInvalid)
	}
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, req.TaskID, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := w.s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"workbuddy_task_dispatch_preview": req})
	if err != nil {
		return out, err
	}
	if replay {
		var id string
		if json.Unmarshal([]byte(hash), &id) != nil {
			return out, conflict(workBuddyTaskDispatchChanged)
		}
		record, e := w.s.workBuddyTaskDispatchPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		if record.Plan.OwnerID != auth.currentPrincipal.ID {
			return out, missing()
		}
		out = record.Plan
		out.Valid, out.Reason = true, ""
		if e = w.validateTaskPlanTx(ctx, engine, auth, record); e != nil {
			out.Valid, out.Reason = false, e.Error()
		}
		return out, auth.assertTx(auth.tx, auth.target, purposeBrowserProjectWrite)
	}
	if err = w.s.rejectReservedCandidateRepairTaskTx(ctx, auth.tx, auth.target.tenantID, project, req.TaskID, ""); err != nil {
		return out, err
	}
	fixed, err := w.s.taskContextForDispatchTx(ctx, auth, project, req.TaskID, req.Knowledge)
	if err != nil {
		return out, err
	}
	if fixed.Task.Version != req.ExpectedTaskVersion {
		return out, conflict(workBuddyTaskDispatchChanged)
	}
	if fixed.CodeBaseline != nil {
		return out, invalid(workBuddyTaskDispatchBaseline)
	}
	connection, err := w.cloudGrantMetadataTx(ctx, auth.tx, auth.target.tenantID, project, auth.currentPrincipal.ID, req.ConnectionID, req.GrantGeneration, "", true)
	if err != nil {
		return out, err
	}
	if connection.Version != req.ExpectedConnectionVersion {
		return out, conflict(workBuddyTaskDispatchChanged)
	}
	if err = w.cloudBudgetTx(ctx, auth.tx, auth.target.tenantID, project); err != nil {
		return out, err
	}
	route, err := engine.ReadProjectExecutionRouteTx(ctx, auth.tx, auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	if err = w.s.checkWorkBuddyTaskDispatchRouteTx(ctx, auth.tx, auth.target.tenantID, project, route); err != nil {
		return out, err
	}
	clock, err := w.s.authorityNow(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	out = WorkBuddyTaskDispatchPlan{ID: newID(), ProjectID: project, OwnerID: auth.currentPrincipal.ID, Version: 1, State: "ready", TaskID: fixed.Task.ID, TaskVersion: fixed.Task.Version, ConnectionID: connection.ID, ConnectionVersion: connection.Version, GrantGeneration: connection.GrantGeneration, RunID: newID(), NodeID: "workbuddy-cloud", DelegationID: newID(), CreatedAt: automationStamp(clock), ExpiresAt: automationStamp(clock.Add(30 * time.Minute)), Context: fixed, ExecutionRoute: route, Valid: true}
	out.Workflow, out.NodeSpecSHA256, out.RequestSHA256, err = workBuddyTaskWorkflow(fixed, connection.ID, connection.GrantGeneration, out.RunID)
	if err != nil {
		return WorkBuddyTaskDispatchPlan{}, err
	}
	record := workBuddyTaskDispatchSnapshot{Plan: out, ClientBindingSHA256: cloudClientBinding(connection)}
	if _, err = auth.tx.ExecContext(ctx, w.s.q(`INSERT INTO suma_workbuddy_task_dispatch_plans(id,tenant_id,project_id,owner_id,run_id,version,state,created_at,expires_at,payload)VALUES(?,?,?,?,?,1,'ready',?,?,?)`), out.ID, auth.target.tenantID, project, out.OwnerID, out.RunID, out.CreatedAt, out.ExpiresAt, stringify(record)); err != nil {
		return WorkBuddyTaskDispatchPlan{}, err
	}
	if err = w.s.saveTaskDispatchRouteBasisTx(ctx, auth.tx, out.ID, route); err != nil {
		return WorkBuddyTaskDispatchPlan{}, err
	}
	if err = w.s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out.ID); err != nil {
		return WorkBuddyTaskDispatchPlan{}, err
	}
	if err = w.s.commandActivity(ctx, auth, "workbuddy_task.dispatch_preview", out.ID, out.Context.Task.Title); err != nil {
		return WorkBuddyTaskDispatchPlan{}, err
	}
	if err = auth.tx.Commit(); err != nil {
		return WorkBuddyTaskDispatchPlan{}, err
	}
	return out, nil
}
func (w *WorkBuddyConnections) GetTaskDispatchPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (WorkBuddyTaskDispatchPlan, error) {
	var out WorkBuddyTaskDispatchPlan
	if w == nil || w.s == nil || engine == nil || !taskSubscriptionID(project) || !taskSubscriptionID(id) {
		return out, invalid(workBuddyTaskDispatchInvalid)
	}
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	ctx, auth, close, err := w.s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	record, err := w.s.workBuddyTaskDispatchPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if record.Plan.OwnerID != auth.currentPrincipal.ID {
		return out, missing()
	}
	out = record.Plan
	out.Valid, out.Reason = true, ""
	if err = w.validateTaskPlanTx(ctx, engine, auth, record); err != nil {
		out.Valid, out.Reason = false, err.Error()
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeBrowserProjectRead)
}
func (w *WorkBuddyConnections) StartTaskDispatchPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req WorkBuddyTaskDispatchStartRequest) (dag.Run, error) {
	var out dag.Run
	if w == nil || w.s == nil || engine == nil || req.ProjectID != project || !taskSubscriptionID(project) || !taskSubscriptionID(id) || req.ExpectedVersion < 1 || req.ExpectedVersion >= 2147483647 || !workBuddyTaskDispatchKey(req.IdempotencyKey) {
		return out, invalid(workBuddyTaskDispatchInvalid)
	}
	if !actor.IsBrowser() {
		return out, forbidden()
	}
	plan, err := w.GetTaskDispatchPlan(ctx, engine, actor, project, id)
	if err != nil {
		return out, err
	}
	p := actor.Projection()
	authority := w.s.engineAuthority(ctx, actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: id}, purposeBrowserProjectWrite)
	defer authority.close()
	ctx = authority.ctx
	var hash string
	var record workBuddyTaskDispatchSnapshot
	return engine.CreateManagedRunBound(ctx, p.TenantID, project, plan.RunID, "workbuddy-task-"+digest(p.ID+":"+req.IdempotencyKey), plan.Workflow, func(tx *sql.Tx) error {
		if e := authority.before(tx); e != nil {
			return e
		}
		var replay bool
		hash, replay, err = w.s.commandReceipt(ctx, authority.proof, req.IdempotencyKey, map[string]any{"workbuddy_task_dispatch_start": id, "request": req})
		if err != nil {
			return err
		}
		if replay {
			var receipt struct {
				PlanID       string `json:"plan_id"`
				RunID        string `json:"run_id"`
				DelegationID string `json:"delegation_id"`
			}
			if json.Unmarshal([]byte(hash), &receipt) != nil || receipt.PlanID != id || receipt.RunID != plan.RunID || receipt.DelegationID != plan.DelegationID {
				return conflict(workBuddyTaskDispatchChanged)
			}
			return nil
		}
		record, err = w.s.workBuddyTaskDispatchPlanTx(ctx, tx, p.TenantID, project, id)
		if err != nil {
			return err
		}
		if record.Plan.Version != req.ExpectedVersion {
			return conflict(workBuddyTaskDispatchChanged)
		}
		return w.validateTaskPlanTx(ctx, engine, authority.proof, record)
	}, func(tx *sql.Tx, runID string) error {
		p = authority.proof.currentPrincipal
		route, e := dag.ReadRunExecutionRouteTx(ctx, tx, w.s.Dialect, p.TenantID, project, runID)
		if e != nil {
			return e
		}
		if e = w.s.checkTaskDispatchRouteBasisTx(ctx, tx, p.TenantID, project, id, route); e != nil {
			return e
		}
		if e = w.s.checkWorkBuddyTaskDispatchRouteTx(ctx, tx, p.TenantID, project, route); e != nil {
			return e
		}
		if e = w.s.BindRunTx(ctx, tx, authority.proof, project, record.Plan.TaskID, runID, record.Plan.TaskVersion); e != nil {
			return e
		}
		if _, e = w.issueCloudTaskDispatchDelegationTx(ctx, authority.proof, record); e != nil {
			return e
		}
		result, e := tx.ExecContext(ctx, w.s.q(`UPDATE suma_workbuddy_task_dispatch_plans SET version=2,state='started' WHERE id=? AND tenant_id=? AND project_id=? AND owner_id=? AND version=? AND state='ready'`), id, p.TenantID, project, p.ID, req.ExpectedVersion)
		if e != nil {
			return e
		}
		count, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if count != 1 {
			return conflict(workBuddyTaskDispatchChanged)
		}
		if e = w.s.saveCommandReceipt(ctx, authority.proof, req.IdempotencyKey, hash, map[string]string{"plan_id": id, "run_id": runID, "delegation_id": record.Plan.DelegationID}); e != nil {
			return e
		}
		return w.s.commandActivity(ctx, authority.proof, "workbuddy_task.dispatched", record.Plan.TaskID, record.Plan.Context.Task.Title)
	})
}
