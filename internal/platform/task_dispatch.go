package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func (s *Service) InitializeTaskDispatch(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_task_dispatch_plans(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(64) NOT NULL UNIQUE,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,created_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`))
	if err != nil {
		return err
	}
	if err = s.initializeTaskDispatchRoutes(ctx); err != nil {
		return err
	}
	if err = s.InitializeTaskRunProvenance(ctx); err != nil {
		return err
	}
	if err = s.InitializeExpertTeams(ctx); err != nil {
		return err
	}
	return s.InitializeCandidateCompositions(ctx)
}

func taskContextRecord(r Record) TaskContextRecord {
	criteria := append([]string{}, r.AcceptanceCriteria...)
	return TaskContextRecord{ID: r.ID, Version: r.Version, Title: r.Title, Description: r.Description, AcceptanceCriteria: criteria}
}

func (s *Service) taskContextTx(ctx context.Context, tx *sql.Tx, p Principal, project, taskID string) (TaskContext, error) {
	var out TaskContext
	task, err := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, taskID))
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	out.Task = taskContextRecord(task)
	quality, err := s.taskTestPlanContextTx(ctx, tx, p.TenantID, project, taskID)
	if err != nil {
		return out, err
	}
	if quality != nil {
		out.TestPlan = &TaskTestPlanContext{Revision: quality.Revision, SHA256: quality.SHA256, SourceSHA256: quality.Source.SHA256, CaseCount: len(quality.Cases), Title: quality.Title}
	}
	if task.Status == "accepted" {
		return out, invalid("任务已经验收；请通过变更需求创建明确的新开发范围")
	}
	if task.RequirementID != "" {
		r, e := scanRecord(tx.QueryRowContext(ctx, s.q(`SELECT `+recordColumns+` FROM suma_requirements WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, task.RequirementID))
		if errors.Is(e, sql.ErrNoRows) {
			return out, missing()
		}
		if e != nil {
			return out, e
		}
		c := taskContextRecord(r)
		out.Requirement = &c
	}
	baseline, _, err := s.taskCodeBaselineTx(ctx, tx, p.TenantID, project, taskID)
	if err != nil {
		return out, err
	}
	if baseline != nil {
		if baseline.TaskVersion != task.Version {
			return out, compositionError(409, "baseline_changed", compositionBaseline)
		}
		if err = s.checkTaskCodeBaselineTx(ctx, tx, p.TenantID, project, *baseline); err != nil {
			return out, err
		}
		out.CodeBaseline = baseline
	}
	return out, nil
}

func (s *Service) taskDispatchPlanTx(ctx context.Context, tx governanceQuery, p Principal, project, id string) (TaskDispatchPlan, error) {
	var out TaskDispatchPlan
	var raw string
	var version int
	var state string
	var expires string
	err := tx.QueryRowContext(ctx, s.q(`SELECT version,state,expires_at,payload FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, id).Scan(&version, &state, &expires, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	out.Version, out.State, out.ExpiresAt = version, state, expires
	return out, nil
}

func (s *Service) validateTaskDispatchPlanTx(ctx context.Context, tx *sql.Tx, engine *dag.Engine, auth *authorizedCommand, project string, plan TaskDispatchPlan) error {
	if auth == nil {
		return forbidden()
	}
	if err := auth.assertTx(tx, auth.target, auth.purpose); err != nil {
		return err
	}
	if auth.target.projectID != project {
		return forbidden()
	}
	if err := s.validateDispatchRecommendationProvenanceTx(ctx, tx, auth.target.tenantID, project, plan); err != nil {
		return err
	}
	p := auth.currentPrincipal
	if plan.State != "ready" {
		return conflict("计划已派发，不能创建另一运行；请打开原运行或恢复原操作")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= plan.ExpiresAt {
		return conflict("派发预览已过期，请根据当前任务重新预览")
	}
	current, err := s.taskContextForDispatchTx(ctx, auth, project, plan.TaskID, knowledgeReferences(plan.Context.Knowledge))
	if err != nil {
		return err
	}
	if current.Task.Version != plan.TaskVersion {
		return conflict("任务内容或排期已变化，请重新派发预览")
	}
	if plan.RequirementID != "" {
		if current.Requirement == nil || current.Requirement.ID != plan.RequirementID || current.Requirement.Version != plan.RequirementVersion {
			return conflict("关联需求依据已变化，请重新派发预览")
		}
	} else if current.Requirement != nil {
		return conflict("任务的需求关联已变化，请重新派发预览")
	}
	if digest(stringify(current.Knowledge)) != digest(stringify(plan.Context.Knowledge)) {
		return conflict("所选项目知识依据已变化，请重新预览派发")
	}
	if digest(stringify(current.Design)) != digest(stringify(plan.Context.Design)) {
		return conflict("需求设计采用依据已变化，请重新预览派发")
	}
	if stringify(current.TestPlan) != stringify(plan.Context.TestPlan) {
		return conflict("The published test plan changed; preview dispatch again")
	}
	if stringify(current.CodeBaseline) != stringify(plan.CodeBaseline) || stringify(plan.Context.CodeBaseline) != stringify(plan.CodeBaseline) {
		return compositionError(409, "baseline_changed", compositionBaseline)
	}
	profile, err := s.validateToolDispatchTx(ctx, tx, auth, project, plan.ToolProfileID, plan.ToolProfileVersion, plan.ToolProfileSHA256)
	if err != nil {
		return err
	}
	if err = s.validateModelExecutionPlanTx(ctx, tx, p.TenantID, project, profile, plan); err != nil {
		return err
	}
	if err = s.validateCodexExecutionPlanTx(ctx, tx, p.TenantID, project, profile, plan); err != nil {
		return err
	}
	return s.checkTaskDispatchCurrentRouteTx(ctx, tx, engine, p, project, plan, profile)
}

func (s *Service) CreateTaskDispatchPlan(ctx context.Context, engine *dag.Engine, actor Actor, project string, request TaskDispatchPreviewRequest) (TaskDispatchPlan, error) {
	var out TaskDispatchPlan
	if engine == nil {
		return out, invalid("任务调度器未配置")
	}
	if request.ProjectID != project || request.ExpectedTaskVersion < 1 || request.WorkflowVersion < 1 || request.ExpectedToolVersion < 1 || request.TaskID == "" || request.WorkflowID == "" || request.AgentNodeID == "" || request.ToolProfileID == "" {
		return out, invalid("派发预览需要项目、任务版本、固定流程、开发节点及工具版本")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: request.TaskID}, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"task_dispatch_preview": request})
	if err != nil {
		return out, err
	}
	if replay {
		var receipt TaskDispatchPlan
		if err = json.Unmarshal([]byte(hash), &receipt); err != nil {
			return out, err
		}
		out, err = s.taskDispatchPlanTx(ctx, tx, p, project, receipt.ID)
		if err != nil {
			return out, err
		}
		out.Valid, out.Reason = true, ""
		if check := s.validateTaskDispatchPlanTx(ctx, tx, engine, auth, project, out); check != nil {
			out.Valid, out.Reason = false, check.Error()
		}
		return out, tx.Commit()
	}
	out, err = s.createTaskDispatchPlanTx(ctx, engine, auth, project, request)
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// Shared by ordinary and coordinated previews. The caller owns authority,
// transaction and operation receipt; no nested transaction or run is created.
func (s *Service) createTaskDispatchPlanTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, project string, request TaskDispatchPreviewRequest) (out TaskDispatchPlan, err error) {
	out, err = s.buildTaskDispatchSnapshotTx(ctx, engine, auth, project, request)
	if err != nil {
		return out, err
	}
	out.ID, out.RunID = newID(), newID()
	err = s.persistTaskDispatchSnapshotTx(ctx, auth, &out)
	return out, err
}

// A source snapshot neither reserves a machine nor starts its 30 minute
// dispatch lifetime. The final admission persists a fresh plan in its own tx.
func (s *Service) buildTaskDispatchSnapshotTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, project string, request TaskDispatchPreviewRequest) (out TaskDispatchPlan, err error) {
	return s.buildTaskDispatchSnapshotWithSourceTx(ctx, engine, auth, project, request, nil)
}

// source is a previously persisted, reviewed snapshot used only for current
// semantic revalidation. Public requests cannot supply a frozen binding.
func (s *Service) buildTaskDispatchSnapshotWithSourceTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, project string, request TaskDispatchPreviewRequest, source *TaskDispatchPlan) (out TaskDispatchPlan, err error) {
	if engine == nil || auth == nil {
		return out, invalid("任务调度器未配置")
	}
	if err = assertProjectCommand(auth.tx, auth, project); err != nil {
		return out, err
	}
	tx, p := auth.tx, auth.currentPrincipal
	if err = s.rejectReservedCandidateRepairTaskTx(ctx, tx, p.TenantID, project, request.TaskID, ""); err != nil {
		return out, err
	}
	context, err := s.taskContextForDispatchTx(ctx, auth, project, request.TaskID, request.Knowledge)
	if err != nil {
		return out, err
	}
	if context.Task.Version != request.ExpectedTaskVersion {
		return out, conflict("任务已经变更，请刷新后重新预览")
	}
	profile, err := s.toolProfileTx(ctx, tx, p, project, request.ToolProfileID)
	if err != nil {
		return out, err
	}
	if _, err = s.validateToolDispatchTx(ctx, tx, auth, project, profile.ID, request.ExpectedToolVersion, profile.Profile.ProfileSHA256); err != nil {
		return out, err
	}
	route, err := engine.ReadProjectExecutionRouteTx(ctx, tx, p.TenantID, project)
	if err != nil {
		return out, err
	}
	if err = s.checkDispatchProfileRouteTx(ctx, tx, p.TenantID, project, profile, route); err != nil {
		return out, err
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT w.spec FROM dag_workflows w WHERE w.tenant_id=? AND w.project_id=? AND w.id=? AND w.version=? AND NOT EXISTS(SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=w.tenant_id AND m.project_id=w.project_id AND m.workflow_id=w.id)`), p.TenantID, project, request.WorkflowID, request.WorkflowVersion).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	var workflow dag.Workflow
	if err = json.Unmarshal([]byte(raw), &workflow); err != nil {
		return out, err
	}
	derived, err := buildTaskDispatchWorkflow(workflow, request.AgentNodeID, profile.ID, profile.Profile, context)
	if err != nil {
		return out, err
	}
	modelBinding, err := s.taskDispatchSourceModelTx(ctx, tx, p.TenantID, project, profile, request, source)
	if err != nil {
		return out, err
	}
	if err = bindModelExecutionWorkflow(&derived, request.AgentNodeID, modelBinding); err != nil {
		return out, err
	}
	codexBinding, err := s.taskDispatchSourceCodexTx(ctx, tx, p.TenantID, project, profile, request, source)
	if err != nil {
		return out, err
	}
	if err = bindCodexExecutionWorkflow(&derived, request.AgentNodeID, codexBinding); err != nil {
		return out, err
	}
	out = TaskDispatchPlan{ProjectID: project, Version: 1, State: "ready", TaskID: context.Task.ID, TaskVersion: context.Task.Version, WorkflowID: request.WorkflowID, WorkflowVersion: request.WorkflowVersion, AgentNodeID: request.AgentNodeID, ToolProfileID: profile.ID, ToolProfileVersion: profile.Version, ToolProfileSHA256: profile.Profile.ProfileSHA256, Context: context, CodeBaseline: context.CodeBaseline, Workflow: derived, ExecutionRoute: &route, Valid: true}
	out.ModelExecution = modelBinding
	out.CodexExecution = codexBinding
	out.OpenCodeExecution = toolcontract.CloneProfile(profile.Profile).OpenCodeExecution
	if context.Requirement != nil {
		out.RequirementID, out.RequirementVersion = context.Requirement.ID, context.Requirement.Version
	}
	return out, nil
}

func (s *Service) persistTaskDispatchSnapshotTx(ctx context.Context, auth *authorizedCommand, out *TaskDispatchPlan) error {
	if out == nil || out.ID == "" || out.RunID == "" || out.ExecutionRoute == nil {
		return expertError("expert_team_invalid", expertInvalid)
	}
	if err := assertProjectCommand(auth.tx, auth, out.ProjectID); err != nil {
		return err
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return err
	}
	out.CreatedAt, out.ExpiresAt = automationStamp(clock), automationStamp(clock.Add(30*time.Minute))
	_, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_dispatch_plans(id,tenant_id,project_id,run_id,version,state,created_by,created_at,expires_at,payload)VALUES(?,?,?,?,1,'ready',?,?,?,?)`), out.ID, auth.target.tenantID, out.ProjectID, out.RunID, auth.currentPrincipal.ID, out.CreatedAt, out.ExpiresAt, stringify(out))
	if err != nil {
		return err
	}
	if err = s.saveTaskDispatchRouteBasisTx(ctx, auth.tx, out.ID, *out.ExecutionRoute); err != nil {
		return err
	}
	return s.commandActivity(ctx, auth, "task.dispatch_preview", out.ID, out.Context.Task.Title)
}

func (s *Service) GetTaskDispatchPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (TaskDispatchPlan, error) {
	var out TaskDispatchPlan
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectRead)
	if err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	out, err = s.taskDispatchPlanTx(ctx, tx, p, project, id)
	if err != nil {
		return out, err
	}
	out.Valid, out.Reason = true, ""
	if check := s.validateTaskDispatchPlanTx(ctx, tx, engine, auth, project, out); check != nil {
		out.Valid, out.Reason = false, check.Error()
	}
	return out, tx.Commit()
}

func (s *Service) StartTaskDispatchPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, request TaskDispatchStartRequest) (dag.Run, error) {
	var out dag.Run
	if request.ProjectID != project || request.ExpectedVersion < 1 {
		return out, invalid("请提供当前派发计划版本")
	}
	// The preview is a current-authorized read; admission and every replay
	// independently recheck the same credential in the Engine transaction.
	plan, err := s.GetTaskDispatchPlan(ctx, engine, actor, project, id)
	if err != nil {
		return out, err
	}
	p := actor.Projection()
	authority := s.engineAuthority(ctx, actor, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: id}, purposeProjectWrite)
	defer authority.close()
	ctx = authority.ctx
	var hash string
	return engine.CreateManagedRunBound(ctx, p.TenantID, project, plan.RunID, "task-"+digest(p.ID+":"+request.IdempotencyKey), plan.Workflow, func(tx *sql.Tx) error {
		if e := authority.before(tx); e != nil {
			return e
		}
		p = authority.proof.currentPrincipal
		if e := s.rejectExpertTeamOwnedPlanTx(ctx, tx, p.TenantID, project, id); e != nil {
			return e
		}
		var replay bool
		hash, replay, err = s.commandReceipt(ctx, authority.proof, request.IdempotencyKey, map[string]any{"task_dispatch_start": id, "request": request})
		if err != nil || replay {
			return err
		}
		current, e := s.taskDispatchPlanTx(ctx, tx, p, project, id)
		if e != nil {
			return e
		}
		if current.Version != request.ExpectedVersion {
			return conflict("派发计划已变化，请刷新并核对")
		}
		return s.validateTaskDispatchPlanTx(ctx, tx, engine, authority.proof, project, current)
	}, func(tx *sql.Tx, runID string) error {
		if e := s.checkTaskDispatchBoundRouteTx(ctx, tx, p, project, plan, runID); e != nil {
			return e
		}
		if e := s.BindRunTx(ctx, tx, authority.proof, project, plan.TaskID, runID, plan.TaskVersion); e != nil {
			return e
		}
		if plan.CodeBaseline != nil {
			if e := s.bindCodeBaselineExecutionTx(ctx, tx, authority.proof, project, runID, plan.AgentNodeID, *plan.CodeBaseline); e != nil {
				return e
			}
		}
		res, e := tx.ExecContext(ctx, s.q(`UPDATE suma_task_dispatch_plans SET version=version+1,state='started' WHERE id=? AND tenant_id=? AND project_id=? AND version=? AND state='ready'`), id, p.TenantID, project, request.ExpectedVersion)
		if e != nil {
			return e
		}
		if count, e := res.RowsAffected(); e != nil || count != 1 {
			return conflict("派发计划已被处理")
		}
		if e = s.saveCommandReceipt(ctx, authority.proof, request.IdempotencyKey, hash, map[string]string{"plan_id": id, "run_id": runID}); e != nil {
			return e
		}
		return s.commandActivity(ctx, authority.proof, "task.dispatched", plan.TaskID, plan.Context.Task.Title)
	})
}

// Managed task workflows cannot be started as public reusable templates, but
// their run lifecycle is separate from privileged managed deployments.
func (s *Service) TaskDispatchRunTx(ctx context.Context, tx governanceQuery, tenant, project, runID string) (bool, error) {
	var count int
	err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_task_dispatch_plans WHERE tenant_id=? AND project_id=? AND run_id=? AND state='started'`), tenant, project, runID).Scan(&count)
	return count == 1, err
}
