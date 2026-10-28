package platform

import "context"

const applicationQualityEvidenceLimit = 3
const applicationQualitySourceLimit = 16
const applicationQualityCursor = "The case cursor does not belong to the selected run's frozen test plan"

// Application quality tools expose existing read models. Their declarations do
// not grant execution, test, approval or issue-management authority to a client.
func applicationQualityDefinitions(makeDefinition func(string, string, string, map[string]any, bool, ...string) ApplicationToolDefinition) []ApplicationToolDefinition {
	limit := applicationInteger(1, 20)
	limit["default"] = 10
	return []ApplicationToolDefinition{
		makeDefinition("project.task_delivery", "读取当前任务的交付、测试与人工验收事实，不派发或修改状态", "application_tool_project_task_delivery", applicationObject(map[string]any{"task_id": applicationString(64)}, "task_id"), true, "api"),
		makeDefinition("project.quality_execution", "分页读取固定任务运行的逐例质量与紧凑证据引用，不读取报告原件或批准验收", "application_tool_project_quality_execution", applicationObject(map[string]any{
			"task_id": applicationString(64), "run_id": applicationString(100), "limit": limit,
			"after_case": map[string]any{"type": "string", "minLength": 1, "maxLength": 64, "pattern": "^[A-Za-z0-9_-]+$"},
		}, "task_id", "run_id"), true, "api"),
	}
}

// The application transport already holds a current project-read transaction.
// Reuse that exact transaction rather than calling public services that would
// open another connection and detach authority (or deadlock single-pool SQLite).
func (s *Service) invokeApplicationQualityRead(ctx context.Context, auth *authorizedCommand, call ApplicationToolCall) (any, error) {
	if auth == nil || auth.purpose != purposeProjectRead || auth.target.projectID != call.ProjectID {
		return nil, forbidden()
	}
	if err := auth.assertTx(auth.tx, auth.target, purposeProjectRead); err != nil {
		return nil, err
	}
	switch call.Tool {
	case "project.task_delivery":
		var input ApplicationToolTaskInput
		if err := strictApplicationInput(call.Arguments, &input, "task_id"); err != nil {
			return nil, err
		}
		if !applicationID(input.TaskID) {
			return nil, applicationInputError()
		}
		return s.taskDeliveryTx(ctx, auth, call.ProjectID, input.TaskID)
	case "project.quality_execution":
		var input ApplicationQualityExecutionInput
		if err := strictApplicationInput(call.Arguments, &input, "task_id", "run_id"); err != nil {
			return nil, err
		}
		if !applicationID(input.TaskID) || !testPlanRunID(input.RunID) || input.Limit < 0 || input.Limit > 20 || applicationHasField(call.Arguments, "limit") && input.Limit == 0 || applicationHasField(call.Arguments, "after_case") && !designKey(input.AfterCase) {
			return nil, applicationInputError()
		}
		if input.Limit == 0 {
			input.Limit = 10
		}
		return s.applicationQualityExecutionTx(ctx, auth, call.ProjectID, input)
	}
	return nil, &APIError{422, "tool_not_allowed", "该工具尚未授权或未实现", nil}
}

func (s *Service) applicationQualityExecutionTx(ctx context.Context, auth *authorizedCommand, project string, input ApplicationQualityExecutionInput) (out ApplicationQualityExecutionPage, err error) {
	if _, err = s.getRecordTx(ctx, auth, project, "tasks", input.TaskID); err != nil {
		return out, err
	}
	tenant := auth.target.tenantID
	current, found, err := s.testPlanByTaskTx(ctx, auth.tx, tenant, project, input.TaskID)
	if err != nil {
		return out, err
	}
	if !found {
		return out, missing()
	}
	fixed, found, err := s.testPlanRunTx(ctx, auth.tx, tenant, project, input.RunID)
	if err != nil {
		return out, err
	}
	// A same-project run is insufficient: it must carry the protected binding
	// to this exact task and plan. A later plan cannot adopt an earlier nil ID.
	if !found || fixed.TaskID != input.TaskID || fixed.PlanID != current.ID {
		return out, missing()
	}
	execution, err := s.testPlanExecutionTx(ctx, auth.tx, tenant, project, current, fixed, input.RunID)
	if err != nil {
		return out, err
	}
	out = ApplicationQualityExecutionPage{
		ProjectID: project, TaskID: execution.TaskID, PlanID: execution.PlanID, RunID: execution.RunID,
		Revision: execution.Revision, PlanSHA256: execution.PlanSHA256, SourceCommits: []string{},
		SourceCommitTotal: len(execution.SourceCommits), SourceCommitsComplete: len(execution.SourceCommits) <= applicationQualitySourceLimit,
		RunState: execution.RunState, State: execution.State, BasisCurrent: execution.BasisCurrent,
		Total: execution.Total, Passed: execution.Passed, Failed: execution.Failed, Errors: execution.Errors,
		Skipped: execution.Skipped, NotRun: execution.NotRun, Limit: input.Limit, Cases: []ApplicationQualityCase{},
	}
	for index, commit := range execution.SourceCommits {
		if index == applicationQualitySourceLimit {
			break
		}
		out.SourceCommits = append(out.SourceCommits, commit)
	}
	if fixed.Revision != nil {
		out.SourceSHA256 = fixed.Revision.Source.SHA256
	}
	// The domain projection already preserves the immutable revision order.
	// Reject invented cursors rather than silently restarting at the first case.
	start := 0
	if input.AfterCase != "" {
		start = -1
		for index, item := range execution.Cases {
			if item.CaseKey == input.AfterCase {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return ApplicationQualityExecutionPage{}, &APIError{422, "invalid_tool_arguments", applicationQualityCursor, nil}
		}
	}
	end := start + input.Limit
	if end > len(execution.Cases) {
		end = len(execution.Cases)
	}
	for _, item := range execution.Cases[start:end] {
		compact := ApplicationQualityCase{CaseKey: item.CaseKey, Title: item.Title, Status: item.Status,
			Covers: append([]int{}, item.Covers...), IssueID: item.IssueID, Evidence: []ApplicationQualityEvidenceReference{},
			EvidenceTotal: len(item.Evidence), EvidenceComplete: len(item.Evidence) <= applicationQualityEvidenceLimit}
		for index, fact := range item.Evidence {
			if index == applicationQualityEvidenceLimit {
				break
			}
			compact.Evidence = append(compact.Evidence, ApplicationQualityEvidenceReference{
				ReportID: fact.ReportID, SourceCommit: fact.SourceCommit, NodeID: fact.NodeID,
				Attempt: fact.Attempt, Fence: fact.Fence, WorkerID: fact.WorkerID, Status: fact.Status,
			})
		}
		out.Cases = append(out.Cases, compact)
	}
	out.Complete = end == len(execution.Cases)
	if !out.Complete && len(out.Cases) > 0 {
		out.NextCase = out.Cases[len(out.Cases)-1].CaseKey
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}
