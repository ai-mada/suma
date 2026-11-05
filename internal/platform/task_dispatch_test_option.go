package platform

import (
	"context"

	"github.com/ai-mada/suma/internal/dag"
)

const (
	taskDispatchTestOptionInvalid    = "Task dispatch test option request is invalid"
	taskDispatchTestOptionMaxVersion = 2147483646
)

type TaskDispatchTestOptionRequest struct {
	ProjectID                     string `json:"project_id"`
	TaskID                        string `json:"task_id"`
	ExpectedTaskVersion           int    `json:"expected_task_version"`
	WorkflowID                    string `json:"workflow_id"`
	WorkflowVersion               int    `json:"workflow_version"`
	AgentNodeID                   string `json:"agent_node_id"`
	DevelopmentToolProfileID      string `json:"development_tool_profile_id"`
	DevelopmentToolProfileVersion int    `json:"development_tool_profile_version"`
	TestNodeID                    string `json:"test_node_id"`
	TestToolProfileID             string `json:"test_tool_profile_id"`
	TestToolProfileVersion        int    `json:"test_tool_profile_version"`
}

// TaskDispatchTestOption is a checked configuration for one proposed test
// override. A current plan and Start must still resolve their own inputs.
type TaskDispatchTestOption struct {
	ProjectID                     string                    `json:"project_id"`
	TaskID                        string                    `json:"task_id"`
	TaskVersion                   int                       `json:"task_version"`
	WorkflowID                    string                    `json:"workflow_id"`
	WorkflowVersion               int                       `json:"workflow_version"`
	AgentNodeID                   string                    `json:"agent_node_id"`
	DevelopmentToolProfileID      string                    `json:"development_tool_profile_id"`
	DevelopmentToolProfileVersion int                       `json:"development_tool_profile_version"`
	DevelopmentToolProfileSHA256  string                    `json:"development_tool_profile_sha256"`
	CheckedAt                     string                    `json:"checked_at"`
	ExecutionRoute                dag.ProjectExecutionRoute `json:"execution_route"`
	Binding                       TaskDispatchTestBinding   `json:"binding"`
}

func (s *Service) GetTaskDispatchTestOption(ctx context.Context, engine *dag.Engine, actor Actor, project, taskID string, req TaskDispatchTestOptionRequest) (out TaskDispatchTestOption, err error) {
	if engine == nil || req.ProjectID != project || req.TaskID != taskID ||
		!handoffID(project, false) || !handoffID(taskID, false) ||
		!diagnosisID(req.WorkflowID, 100) || !diagnosisID(req.AgentNodeID, 150) || !diagnosisID(req.TestNodeID, 150) ||
		!handoffID(req.DevelopmentToolProfileID, false) || !handoffID(req.TestToolProfileID, false) {
		return out, invalid(taskDispatchTestOptionInvalid)
	}
	for _, version := range []int{req.ExpectedTaskVersion, req.WorkflowVersion, req.DevelopmentToolProfileVersion, req.TestToolProfileVersion} {
		if version < 1 || version > taskDispatchTestOptionMaxVersion {
			return out, invalid(taskDispatchTestOptionInvalid)
		}
	}

	// The write proof is the eligibility required by the exact override
	// validators. Only the in-memory graph is changed during this read.
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: project}
	err = s.withCurrentRead(ctx, actor, target, purposeBrowserProjectWrite, func(auth *authorizedCommand) error {
		ctx, tx, p := auth.ctx, auth.tx, auth.currentPrincipal
		if e := s.rejectReservedCandidateRepairTaskTx(ctx, tx, p.TenantID, project, taskID, ""); e != nil {
			return e
		}
		task, e := s.taskContextForDispatchTx(ctx, auth, project, taskID, nil)
		if e != nil {
			return e
		}
		if task.Task.Version != req.ExpectedTaskVersion {
			return conflict("任务已经变更，请刷新后重新预览")
		}
		profile, e := s.toolProfileTx(ctx, tx, p, project, req.DevelopmentToolProfileID)
		if e != nil {
			return e
		}
		profile, e = s.validateToolDispatchTx(ctx, tx, auth, project, profile.ID, req.DevelopmentToolProfileVersion, profile.Profile.ProfileSHA256)
		if e != nil {
			return e
		}
		route, e := engine.ReadProjectExecutionRouteTx(ctx, tx, p.TenantID, project)
		if e != nil {
			return e
		}
		if e = s.checkDispatchProfileRouteTx(ctx, tx, p.TenantID, project, profile, route); e != nil {
			return e
		}
		workflow, e := s.readPublishedTaskDispatchWorkflowTx(ctx, tx, p.TenantID, project, req.WorkflowID, req.WorkflowVersion)
		if e != nil {
			return e
		}
		derived, e := buildTaskDispatchWorkflow(workflow, req.AgentNodeID, profile.ID, profile.Profile, task)
		if e != nil {
			return e
		}
		bindings, e := s.freezeTaskDispatchTestBindingsTx(ctx, auth, project, &derived, req.AgentNodeID, profile, route,
			[]TaskDispatchTestSelection{{NodeID: req.TestNodeID, ToolProfileID: req.TestToolProfileID, ExpectedToolVersion: req.TestToolProfileVersion}})
		if e != nil {
			return e
		}
		out = TaskDispatchTestOption{
			ProjectID: project, TaskID: taskID, TaskVersion: task.Task.Version,
			WorkflowID: req.WorkflowID, WorkflowVersion: req.WorkflowVersion, AgentNodeID: req.AgentNodeID,
			DevelopmentToolProfileID: profile.ID, DevelopmentToolProfileVersion: profile.Version, DevelopmentToolProfileSHA256: profile.Profile.ProfileSHA256,
			CheckedAt: automationStamp(auth.checkedAt), ExecutionRoute: route, Binding: bindings[0],
		}
		return nil
	})
	if err != nil {
		return TaskDispatchTestOption{}, err
	}
	return out, nil
}
