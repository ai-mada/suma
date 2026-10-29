package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

const workspaceTerminalCandidateChanged = "The stopped terminal candidate basis changed; inspect the original session and preview again"

func (s *Service) InitializeWorkspaceTerminalCandidates(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_workspace_terminal_candidate_plans(plan_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,session_id VARCHAR(64) NOT NULL,run_id VARCHAR(64) NOT NULL UNIQUE,node_id VARCHAR(150) NOT NULL,source TEXT NOT NULL)`))
	return err
}

func workspaceTerminalCandidateProfile(p ToolProfile) bool {
	q := p.Profile
	return p.State == "enabled" && q.Adapter == "command" && q.Protocol == "text-v1" && !q.ReadOnly && q.TerminalSessionProtocol == "" && q.TestReportProtocol == "" && q.WorkspaceToolsVersion == "" && q.ModelExecutionProtocol == "" && q.ConflictRepairProtocol == ""
}

func (s *Service) workspaceTerminalCandidateSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out *term.TerminalCandidateSource, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT source FROM suma_workspace_terminal_candidate_plans WHERE tenant_id=? AND project_id=? AND plan_id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var source term.TerminalCandidateSource
	if json.Unmarshal([]byte(raw), &source) != nil || !source.Valid() || source.PlanID != id {
		return nil, conflict(workspaceTerminalCandidateChanged)
	}
	return &source, nil
}

func (s *Service) workspaceTerminalCandidateStoppedTx(ctx context.Context, tx *sql.Tx, tenant, project string, source term.TerminalCandidateSource) error {
	r, err := s.workspaceTerminalTx(ctx, tx, tenant, project, source.SessionID)
	if err != nil {
		return err
	}
	if r.Session.Version != source.SessionVersion || !r.TreeStopped || r.Session.State != "stopped" && r.Session.State != "exited" || r.Session.SourceSnapshotSHA256 != source.SnapshotSHA256 || r.Session.BaseCommit != source.BaseCommit || r.Attempt != source.Original.Attempt || r.Fence != source.Original.Fence || r.Session.WorkerID != source.Original.WorkerID || r.Session.RunID != source.Original.RunID {
		return conflict(workspaceTerminalCandidateChanged)
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_terminal_active WHERE tenant_id=? AND project_id=? AND task_id=?`), tenant, project, r.Session.TaskID).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return conflict(workspaceTerminalActive)
	}
	var raw, state string
	if r.Route.PoolID == "" {
		err = tx.QueryRowContext(ctx, s.q(`SELECT assignment FROM dag_dispatches WHERE id=? AND tenant_id=? AND project_id=? AND run_id=? AND node_id='terminal' AND attempt=? AND fence=? AND worker_id=?`), r.DispatchID, tenant, project, r.Session.RunID, r.Attempt, r.Fence, r.Session.WorkerID).Scan(&raw)
	} else {
		err = tx.QueryRowContext(ctx, s.q(`SELECT assignment FROM dag_execution_pool_dispatches WHERE dispatch_id=? AND pool_id=?`), r.DispatchID, r.Route.PoolID).Scan(&raw)
	}
	var original dag.Assignment
	if err != nil || json.Unmarshal([]byte(raw), &original) != nil || codeInputAssignmentRef(original) != source.Original {
		return conflict(workspaceTerminalCandidateChanged)
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT state FROM dag_nodes WHERE run_id=? AND id='terminal' AND attempt=? AND fence=? AND worker_id=?`), source.Original.RunID, source.Original.Attempt, source.Original.Fence, source.Original.WorkerID).Scan(&state); err != nil {
		return err
	}
	if state != "succeeded" && state != "failed" && state != "cancelled" {
		return conflict(workspaceTerminalStopped)
	}
	return nil
}

func (s *Service) buildWorkspaceTerminalCandidateWorkflowTx(ctx context.Context, auth *authorizedCommand, project string, workflow dag.Workflow, producer string, profile ToolProfile, source term.TerminalCandidateSource) (dag.Workflow, error) {
	var derived dag.Workflow
	if json.Unmarshal([]byte(stringify(workflow)), &derived) != nil || len(derived.Nodes) != 2 {
		return derived, invalid("Terminal candidate workflow requires one fixed command and one independent candidate test")
	}
	chosen := -1
	test := -1
	for i, n := range derived.Nodes {
		if n.ID == producer && n.Type == "command" {
			chosen = i
		} else if n.Type == "test" {
			test = i
		}
	}
	if chosen < 0 || test < 0 || derived.Nodes[test].Config["candidate_from"] != producer || len(derived.Nodes[chosen].Command) != 1 || derived.Nodes[chosen].Command[0] != profile.Profile.CommandID {
		return derived, invalid("Select the fixed command producer and its explicit candidate test")
	}
	for _, n := range derived.Nodes {
		for _, key := range []string{term.CandidateConfigKey, "purpose", "candidate_composition", "candidate_conflict_repair", "code_baseline", "model_execution", "codex_execution", "opencode_execution", "octop_execution", "artifact_from", "test_from"} {
			if _, present := n.Config[key]; present {
				return derived, invalid("Terminal candidate workflow cannot mix protected input or model modes")
			}
		}
	}
	if _, present := derived.Nodes[chosen].Config["candidate_from"]; present {
		return derived, invalid("Terminal source cannot inherit another candidate")
	}
	testNode := derived.Nodes[test]
	testID, testSHA, bound, err := dag.ToolBinding(testNode)
	if err != nil || !bound || len(testNode.Command) != 1 {
		return derived, invalid("Independent test requires its exact current tool binding")
	}
	testProfile, err := s.toolProfileTx(ctx, auth.tx, auth.currentPrincipal, project, testID)
	if err != nil {
		return derived, err
	}
	testProfile, err = s.validateToolDispatchTx(ctx, auth.tx, auth, project, testID, testProfile.Version, testSHA)
	if err != nil {
		return derived, err
	}
	if testProfile.WorkerID == source.Original.WorkerID || testProfile.Profile.CommandID != testNode.Command[0] || testProfile.Profile.Adapter != "command" || testProfile.Profile.Protocol != "text-v1" || testProfile.Profile.ReadOnly || testProfile.Profile.TestReportProtocol == "" {
		return derived, invalid("Select a different Worker with a fixed isolated JUnit test profile")
	}
	n := &derived.Nodes[chosen]
	if n.Config == nil {
		n.Config = map[string]any{}
	}
	n.Config["tool_profile_id"], n.Config["tool_profile_sha256"] = profile.ID, profile.Profile.ProfileSHA256
	n.Config[term.CandidateConfigKey] = source
	n.MaxRetries, n.Idempotent = 0, false
	derived.Name = "Stopped terminal candidate"
	if err = dag.Compile(derived); err != nil {
		return derived, err
	}
	return derived, nil
}

func (s *Service) CreateWorkspaceTerminalCandidatePlan(ctx context.Context, engine *dag.Engine, actor Actor, project, session string, req WorkspaceTerminalCandidatePreviewRequest) (out WorkspaceTerminalCandidatePlan, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if engine == nil || project != req.ProjectID || !handoffID(project, false) || !handoffID(session, false) || !handoffID(req.WorkflowID, false) || !handoffID(req.ProducerNodeID, false) || !handoffID(req.ToolProfileID, false) || req.ExpectedSessionVersion < 1 || req.ExpectedTaskVersion < 1 || req.WorkflowVersion < 1 || req.ExpectedToolVersion < 1 || !planningKey(req.IdempotencyKey) || !req.AcknowledgeStoppedChanges {
		return out, invalid("Confirm the stopped terminal changes and exact current candidate workflow")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: session}, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, auth.target.tenantID, project); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"workspace_terminal_candidate_preview": session, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		var receipt WorkspaceTerminalCandidatePlan
		if json.Unmarshal([]byte(hash), &receipt) != nil {
			return out, conflict(workspaceTerminalCandidateChanged)
		}
		out, err = s.workspaceTerminalCandidatePlanTx(ctx, engine, auth, project, receipt.ID)
		if err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	r, err := s.workspaceTerminalTx(ctx, tx, auth.target.tenantID, project, session)
	if err != nil {
		return out, err
	}
	if r.Session.Version != req.ExpectedSessionVersion || r.Task.Version != req.ExpectedTaskVersion {
		return out, conflict(workspaceTerminalCandidateChanged)
	}
	if err = s.workspaceTerminalBasisTx(ctx, auth, r); err != nil {
		return out, err
	}
	if err = s.rejectReservedCandidateRepairTaskTx(ctx, tx, auth.target.tenantID, project, r.Session.TaskID, ""); err != nil {
		return out, err
	}
	context, err := s.taskContextForDispatchTx(ctx, auth, project, r.Session.TaskID, nil)
	if err != nil {
		return out, err
	}
	if context.CodeBaseline != nil {
		return out, conflict("Terminal candidate cannot replace a task's adopted code baseline")
	}
	profile, err := s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, req.ToolProfileID)
	if err != nil {
		return out, err
	}
	profile, err = s.validateToolDispatchTx(ctx, tx, auth, project, profile.ID, req.ExpectedToolVersion, profile.Profile.ProfileSHA256)
	if err != nil {
		return out, err
	}
	if !workspaceTerminalCandidateProfile(profile) || profile.WorkerID != r.Session.WorkerID {
		return out, invalid("Select the original Worker and a fixed writable command profile")
	}
	var originalRaw string
	if r.Route.PoolID == "" {
		err = tx.QueryRowContext(ctx, s.q(`SELECT assignment FROM dag_dispatches WHERE id=? AND tenant_id=? AND project_id=?`), r.DispatchID, auth.target.tenantID, project).Scan(&originalRaw)
	} else {
		err = tx.QueryRowContext(ctx, s.q(`SELECT assignment FROM dag_execution_pool_dispatches WHERE dispatch_id=? AND pool_id=?`), r.DispatchID, r.Route.PoolID).Scan(&originalRaw)
	}
	var original dag.Assignment
	if err != nil || json.Unmarshal([]byte(originalRaw), &original) != nil {
		return out, conflict(workspaceTerminalCandidateChanged)
	}
	plan := TaskDispatchPlan{ID: newID(), RunID: newID(), ProjectID: project, Version: 1, State: "ready", TaskID: r.Session.TaskID, TaskVersion: context.Task.Version, WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, AgentNodeID: req.ProducerNodeID, ToolProfileID: profile.ID, ToolProfileVersion: profile.Version, ToolProfileSHA256: profile.Profile.ProfileSHA256, Context: context, Valid: true}
	source := term.TerminalCandidateSource{SessionID: session, SessionVersion: r.Session.Version, Original: codeInputAssignmentRef(original), BaseCommit: r.Session.BaseCommit, SnapshotSHA256: r.Session.SourceSnapshotSHA256, PlanID: plan.ID}
	if !source.Valid() {
		return out, conflict("The original terminal has no safe stopped source snapshot")
	}
	if err = s.workspaceTerminalCandidateStoppedTx(ctx, tx, auth.target.tenantID, project, source); err != nil {
		return out, err
	}
	route, err := engine.ReadProjectExecutionRouteTx(ctx, tx, auth.target.tenantID, project)
	if err != nil {
		return out, err
	}
	if route != r.Route {
		return out, conflict(workspaceTerminalCandidateChanged)
	}
	if err = s.checkDispatchProfileRouteTx(ctx, tx, auth.target.tenantID, project, profile, route); err != nil {
		return out, err
	}
	plan.ExecutionRoute = &route
	if context.Requirement != nil {
		plan.RequirementID, plan.RequirementVersion = context.Requirement.ID, context.Requirement.Version
	}
	var workflowRaw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT spec FROM dag_workflows w WHERE tenant_id=? AND project_id=? AND id=? AND version=? AND NOT EXISTS(SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=w.tenant_id AND m.project_id=w.project_id AND m.workflow_id=w.id)`), auth.target.tenantID, project, req.WorkflowID, req.WorkflowVersion).Scan(&workflowRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	var workflow dag.Workflow
	if json.Unmarshal([]byte(workflowRaw), &workflow) != nil {
		return out, conflict(workspaceTerminalCandidateChanged)
	}
	plan.Workflow, err = s.buildWorkspaceTerminalCandidateWorkflowTx(ctx, auth, project, workflow, req.ProducerNodeID, profile, source)
	if err != nil {
		return out, err
	}
	if err = s.persistTaskDispatchSnapshotTx(ctx, auth, &plan); err != nil {
		return out, err
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_terminal_candidate_plans(plan_id,tenant_id,project_id,session_id,run_id,node_id,source)VALUES(?,?,?,?,?,?,?)`), plan.ID, auth.target.tenantID, project, session, plan.RunID, plan.AgentNodeID, stringify(source))
	if err != nil {
		return out, err
	}
	out = workspaceTerminalCandidatePublic(plan, source)
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func workspaceTerminalCandidatePublic(plan TaskDispatchPlan, source term.TerminalCandidateSource) WorkspaceTerminalCandidatePlan {
	return WorkspaceTerminalCandidatePlan{ID: plan.ID, ProjectID: plan.ProjectID, Version: plan.Version, State: plan.State, SessionID: source.SessionID, SessionVersion: source.SessionVersion, TaskID: plan.TaskID, TaskVersion: plan.TaskVersion, RunID: plan.RunID, CreatedAt: plan.CreatedAt, ExpiresAt: plan.ExpiresAt, ToolProfileID: plan.ToolProfileID, ToolProfileVersion: plan.ToolProfileVersion, ToolProfileSHA256: plan.ToolProfileSHA256, WorkerID: source.Original.WorkerID, WorkflowID: plan.WorkflowID, WorkflowVersion: plan.WorkflowVersion, ProducerNodeID: plan.AgentNodeID, Context: plan.Context, Workflow: plan.Workflow, Source: source, Valid: plan.Valid, Reason: plan.Reason}
}

func (s *Service) workspaceTerminalCandidatePlanTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, project, id string) (out WorkspaceTerminalCandidatePlan, err error) {
	source, err := s.workspaceTerminalCandidateSourceTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if source == nil {
		return out, missing()
	}
	plan, err := s.taskDispatchPlanTx(ctx, auth.tx, auth.currentPrincipal, project, id)
	if err != nil {
		return out, err
	}
	plan.Valid, plan.Reason = true, ""
	if plan.State == "ready" {
		if check := s.validateTaskDispatchPlanTx(ctx, auth.tx, engine, auth, project, plan); check != nil {
			plan.Valid, plan.Reason = false, check.Error()
		}
	}
	return workspaceTerminalCandidatePublic(plan, *source), nil
}

func (s *Service) GetWorkspaceTerminalCandidatePlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (out WorkspaceTerminalCandidatePlan, err error) {
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(workspaceTerminalInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	return s.workspaceTerminalCandidatePlanTx(ctx, engine, auth, project, id)
}

func (s *Service) ListWorkspaceTerminalCandidatePlans(ctx context.Context, engine *dag.Engine, actor Actor, project, session string) (out WorkspaceTerminalCandidatePlanPage, err error) {
	out.Items = []WorkspaceTerminalCandidatePlan{}
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(session, false) {
		return out, invalid(workspaceTerminalInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, session, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	original, err := s.workspaceTerminalTx(ctx, auth.tx, auth.target.tenantID, project, session)
	if err != nil {
		return out, err
	}
	task, err := s.getRecordTx(ctx, auth, project, "tasks", original.Session.TaskID)
	if err != nil {
		return out, err
	}
	out.TaskVersion = task.Version
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT c.plan_id FROM suma_workspace_terminal_candidate_plans c JOIN suma_task_dispatch_plans p ON p.id=c.plan_id AND p.tenant_id=c.tenant_id AND p.project_id=c.project_id WHERE c.tenant_id=? AND c.project_id=? AND c.session_id=? ORDER BY p.created_at DESC,c.plan_id DESC LIMIT 20`), auth.target.tenantID, project, session)
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		plan, e := s.workspaceTerminalCandidatePlanTx(ctx, engine, auth, project, id)
		if e != nil {
			return out, e
		}
		if plan.Source.SessionID != session || plan.TaskID != task.ID {
			return out, conflict(workspaceTerminalCandidateChanged)
		}
		out.Items = append(out.Items, plan)
	}
	return out, nil
}

func (s *Service) StartWorkspaceTerminalCandidatePlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req TaskDispatchStartRequest) (dag.Run, error) {
	if _, err := s.GetWorkspaceTerminalCandidatePlan(ctx, engine, actor, project, id); err != nil {
		return dag.Run{}, err
	}
	return s.StartTaskDispatchPlan(ctx, engine, actor, project, id, req)
}

func (s *Service) validateWorkspaceTerminalCandidatePlanTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project string, plan TaskDispatchPlan) error {
	source, err := s.workspaceTerminalCandidateSourceTx(ctx, tx, auth.target.tenantID, project, plan.ID)
	if err != nil {
		return err
	}
	declared := false
	for _, node := range plan.Workflow.Nodes {
		if _, present := node.Config[term.CandidateConfigKey]; present {
			declared = true
		}
	}
	if source == nil {
		if declared {
			return forbidden()
		}
		return nil
	}
	if !declared {
		return forbidden()
	}
	return s.workspaceTerminalCandidateStoppedTx(ctx, tx, auth.target.tenantID, project, *source)
}
