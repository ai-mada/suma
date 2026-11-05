package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

const (
	repositorySourceProducerNode = "repository-source"
	repositorySourceTestNode     = "repository-test"
	repositorySourcePlanChanged  = "Approved repository source or original task basis changed; preview again"
)

// TaskDeliveryRepositoryDispatchPreviewRequest selects two existing tool
// profiles. Control derives the fixed workflow; no command, source object,
// arbitrary branch, or model selection is supplied by the browser.
type TaskDeliveryRepositoryDispatchPreviewRequest struct {
	ProjectID                      string `json:"project_id"`
	ExpectedTaskVersion            int    `json:"expected_task_version"`
	ProducerProfileID              string `json:"producer_profile_id"`
	ExpectedProducerProfileVersion int    `json:"expected_producer_profile_version"`
	TestProfileID                  string `json:"test_profile_id"`
	ExpectedTestProfileVersion     int    `json:"expected_test_profile_version"`
	IdempotencyKey                 string `json:"idempotency_key"`
}

func (s *Service) initializeRepositorySourceDispatch(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl("CREATE TABLE IF NOT EXISTS suma_repository_source_dispatch_plans(plan_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,run_id VARCHAR(64) NOT NULL UNIQUE,node_id VARCHAR(150) NOT NULL,source TEXT NOT NULL)"))
	return err
}

func repositorySourceFromIntake(planID string, record repositorySourceIntakeRecord) ri.RepositorySourceExecution {
	in := record.Receipt
	return ri.RepositorySourceExecution{
		IntakeID: in.ID, PlanID: planID, TaskID: in.TaskID, TaskVersion: in.TaskVersion,
		BaseCommit: in.BaseCommit, HeadCommit: in.HeadCommit, BundleSHA256: in.BundleSHA256, SizeBytes: in.SizeBytes,
	}
}

func (s *Service) repositorySourceDispatchPlanTx(ctx context.Context, tx *sql.Tx, tenant, project, plan string) (*ri.RepositorySourceExecution, error) {
	var raw string
	err := tx.QueryRowContext(ctx, s.q("SELECT source FROM suma_repository_source_dispatch_plans WHERE tenant_id=? AND project_id=? AND plan_id=?"), tenant, project, plan).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var source ri.RepositorySourceExecution
	if json.Unmarshal([]byte(raw), &source) != nil || !source.Valid() || source.PlanID != plan {
		return nil, conflict(repositorySourcePlanChanged)
	}
	return &source, nil
}

// An ordinary workflow with a copied marker has no corresponding started
// platform plan and cannot gain repository intake access through this lookup.
func (s *Service) repositorySourceExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (*ri.RepositorySourceExecution, error) {
	var plan, raw string
	err := tx.QueryRowContext(ctx, s.q("SELECT c.plan_id,c.source FROM suma_repository_source_dispatch_plans c JOIN suma_task_dispatch_plans p ON p.id=c.plan_id AND p.tenant_id=c.tenant_id AND p.project_id=c.project_id AND p.run_id=c.run_id WHERE c.tenant_id=? AND c.project_id=? AND c.run_id=? AND c.node_id=? AND p.state='started'"), tenant, project, run, node).Scan(&plan, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var source ri.RepositorySourceExecution
	if json.Unmarshal([]byte(raw), &source) != nil || !source.Valid() || source.PlanID != plan {
		return nil, conflict(repositorySourcePlanChanged)
	}
	return &source, nil
}

func repositorySourceDispatchMarked(plan TaskDispatchPlan) bool {
	for _, node := range plan.Workflow.Nodes {
		if _, exists := node.Config[ri.RepositorySourceExecutionKey]; exists {
			return true
		}
	}
	return false
}

func (s *Service) checkRepositorySourceDispatchModeTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, plan TaskDispatchPlan) (*ri.RepositorySourceExecution, error) {
	source, err := s.repositorySourceDispatchPlanTx(ctx, tx, auth.target.tenantID, auth.target.projectID, plan.ID)
	if err != nil {
		return nil, err
	}
	var declared *ri.RepositorySourceExecution
	for _, node := range plan.Workflow.Nodes {
		raw, exists := node.Config[ri.RepositorySourceExecutionKey]
		if !exists {
			continue
		}
		if declared != nil || node.ID != plan.AgentNodeID || node.ID != repositorySourceProducerNode || node.Type != "command" {
			return nil, forbidden()
		}
		declared, err = ri.DecodeRepositorySourceExecution(raw)
		if err != nil || declared == nil {
			return nil, forbidden()
		}
	}
	if source == nil && declared == nil {
		return nil, nil
	}
	if source == nil || declared == nil || *source != *declared || source.TaskID != plan.TaskID || source.TaskVersion != plan.TaskVersion ||
		plan.Context.TestPlan == nil || plan.Context.TestPlan.CaseCount < 1 || plan.CodeBaseline != nil || len(plan.TestBindings) != 1 ||
		plan.TestBindings[0].NodeID != repositorySourceTestNode || len(plan.Workflow.Nodes) != 2 {
		return nil, forbidden()
	}
	if auth.purpose != purposeBrowserProjectApprove && auth.purpose != purposeProjectRead {
		return nil, forbidden()
	}
	return source, nil
}

func (s *Service) checkRepositorySourceDispatchBasisTx(ctx context.Context, auth *authorizedCommand, record repositorySourceIntakeRecord) error {
	in := record.Receipt
	query := taskDeliveryRepositoryQuery{project: in.ProjectID, task: in.TaskID, run: in.RunID, target: in.TargetID, taskVersion: in.TaskVersion, targetVersion: in.TargetVersion}
	current, _, err := s.taskDeliveryRepositoryBasisTx(ctx, auth, query)
	if err != nil {
		return err
	}
	if !repositorySourceIntakeMatchesBasis(record, current) {
		return conflict(repositorySourcePlanChanged)
	}
	var active int
	if err = auth.tx.QueryRowContext(ctx, s.q("SELECT COUNT(*) FROM suma_workspace_terminal_active WHERE tenant_id=? AND project_id=? AND task_id=?"), auth.target.tenantID, in.ProjectID, in.TaskID).Scan(&active); err != nil {
		return err
	}
	if active != 0 {
		return conflict(workspaceTerminalActive)
	}
	return nil
}

// Called before first start and again under the scheduler project guard just
// before BindRunTx. A stale cloud origin rolls back run creation and task CAS.
func (s *Service) validateRepositorySourceDispatchTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, plan TaskDispatchPlan) error {
	source, err := s.checkRepositorySourceDispatchModeTx(ctx, tx, auth, plan)
	if err != nil || source == nil {
		return err
	}
	record, _, err := s.loadRepositorySourceIntakeTx(ctx, tx, auth.target.tenantID, plan.ProjectID, plan.TaskID, source.IntakeID)
	if err != nil {
		return err
	}
	if repositorySourceFromIntake(plan.ID, record) != *source {
		return conflict(repositorySourcePlanChanged)
	}
	return s.checkRepositorySourceDispatchBasisTx(ctx, auth, record)
}

func (s *Service) CreateTaskDeliveryRepositoryDispatchPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, task, intake string, req TaskDeliveryRepositoryDispatchPreviewRequest) (out TaskDispatchPlan, err error) {
	if engine == nil || project != req.ProjectID || !handoffID(project, false) || !handoffID(task, false) || !handoffID(intake, false) ||
		!handoffID(req.ProducerProfileID, false) || !handoffID(req.TestProfileID, false) || !ingressVersion(req.ExpectedTaskVersion) ||
		!ingressVersion(req.ExpectedProducerProfileVersion) || !ingressVersion(req.ExpectedTestProfileVersion) || !planningKey(req.IdempotencyKey) {
		return out, invalid("Select exact repository intake, producer and independent test profiles")
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, task, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	tx, p := auth.tx, auth.currentPrincipal
	fingerprint, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{
		"repository_source_dispatch_preview": intake, "task_id": task, "request": req,
	})
	if err != nil {
		return out, err
	}
	if replay {
		var receipt TaskDispatchPlan
		if json.Unmarshal([]byte(fingerprint), &receipt) != nil {
			return out, conflict(repositorySourcePlanChanged)
		}
		out, err = s.taskDispatchPlanTx(ctx, tx, p, project, receipt.ID)
		if err != nil {
			return out, err
		}
		out.Valid, out.Reason = true, ""
		if out.State == "ready" {
			if check := s.validateTaskDispatchPlanTx(ctx, tx, engine, auth, project, out); check != nil {
				out.Valid, out.Reason = false, check.Error()
			} else if check := s.validateRepositorySourceDispatchTx(ctx, tx, auth, out); check != nil {
				out.Valid, out.Reason = false, check.Error()
			}
		}
		return out, tx.Commit()
	}

	record, _, err := s.loadRepositorySourceIntakeTx(ctx, tx, p.TenantID, project, task, intake)
	if err != nil {
		return out, err
	}
	if record.Receipt.TaskVersion != req.ExpectedTaskVersion {
		return out, conflict(repositorySourcePlanChanged)
	}
	if err = s.checkRepositorySourceDispatchBasisTx(ctx, auth, record); err != nil {
		return out, err
	}
	if err = s.rejectReservedCandidateRepairTaskTx(ctx, tx, p.TenantID, project, task, ""); err != nil {
		return out, err
	}
	taskContext, err := s.taskContextForDispatchTx(ctx, auth, project, task, nil)
	if err != nil {
		return out, err
	}
	if taskContext.Task.Version != req.ExpectedTaskVersion || taskContext.CodeBaseline != nil {
		return out, conflict(repositorySourcePlanChanged)
	}
	if taskContext.TestPlan == nil || taskContext.TestPlan.CaseCount < 1 {
		return out, invalid("Publish the task's test cases before dispatching the repository source")
	}
	producer, err := s.toolProfileTx(ctx, tx, p, project, req.ProducerProfileID)
	if err != nil {
		return out, err
	}
	producer, err = s.validateToolDispatchTx(ctx, tx, auth, project, producer.ID, req.ExpectedProducerProfileVersion, producer.Profile.ProfileSHA256)
	if err != nil {
		return out, err
	}
	if !workspaceTerminalCandidateProfile(producer) {
		return out, invalid("Select a fixed writable command profile for repository source import")
	}
	route, err := engine.ReadProjectExecutionRouteTx(ctx, tx, p.TenantID, project)
	if err != nil {
		return out, err
	}
	if err = s.checkDispatchProfileRouteTx(ctx, tx, p.TenantID, project, producer, route); err != nil {
		return out, err
	}
	test, err := s.toolProfileTx(ctx, tx, p, project, req.TestProfileID)
	if err != nil {
		return out, err
	}
	id, run := newID(), newID()
	source := repositorySourceFromIntake(id, record)
	if !source.Valid() {
		return out, conflict(repositorySourcePlanChanged)
	}
	workflow := dag.Workflow{ID: "managed-" + run, Version: 1, Name: "Approved repository candidate",
		Nodes: []dag.Node{
			{ID: repositorySourceProducerNode, Name: "Import approved repository commit", Type: "command", Command: []string{producer.Profile.CommandID}, TimeoutSeconds: 300, Config: map[string]any{
				"tool_profile_id": producer.ID, "tool_profile_sha256": producer.Profile.ProfileSHA256, ri.RepositorySourceExecutionKey: source,
			}},
			{ID: repositorySourceTestNode, Name: "Independent test of imported commit", Type: "test", Command: []string{test.Profile.CommandID}, TimeoutSeconds: 300, Config: map[string]any{
				"candidate_from": repositorySourceProducerNode,
			}},
		}, Edges: []dag.Edge{{From: repositorySourceProducerNode, To: repositorySourceTestNode}},
	}
	testBindings, err := s.freezeTaskDispatchTestBindingsTx(ctx, auth, project, &workflow, repositorySourceProducerNode, producer, route,
		[]TaskDispatchTestSelection{{NodeID: repositorySourceTestNode, ToolProfileID: req.TestProfileID, ExpectedToolVersion: req.ExpectedTestProfileVersion}})
	if err != nil {
		return out, err
	}
	out = TaskDispatchPlan{ID: id, RunID: run, ProjectID: project, Version: 1, State: "ready", TaskID: task, TaskVersion: req.ExpectedTaskVersion,
		WorkflowID: workflow.ID, WorkflowVersion: workflow.Version, AgentNodeID: repositorySourceProducerNode,
		ToolProfileID: producer.ID, ToolProfileVersion: producer.Version, ToolProfileSHA256: producer.Profile.ProfileSHA256,
		Context: taskContext, Workflow: workflow, ExecutionRoute: &route, TestBindings: testBindings, Valid: true,
	}
	if taskContext.Requirement != nil {
		out.RequirementID, out.RequirementVersion = taskContext.Requirement.ID, taskContext.Requirement.Version
	}
	if err = s.persistTaskDispatchSnapshotTx(ctx, auth, &out); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q("INSERT INTO suma_repository_source_dispatch_plans(plan_id,tenant_id,project_id,run_id,node_id,source)VALUES(?,?,?,?,?,?)"),
		out.ID, p.TenantID, project, out.RunID, out.AgentNodeID, stringify(source)); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, fingerprint, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
