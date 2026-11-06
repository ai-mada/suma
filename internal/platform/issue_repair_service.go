package platform

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// Commands recover the original operation before checking mutable versions.
// Current credentials and the same project guard are still checked first.
func (s *Service) issueRepairCommand(ctx context.Context, actor Actor, project, issue, key string, purpose commandPurpose, request any, change func(context.Context, *authorizedCommand) error) (out IssueRepairView, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(issue, false) || !planningKey(key) {
		return out, invalid(issueRepairInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, issue, purpose)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, key, request)
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		out.restrictAutomationSource(auth.actor)
		return out, err
	}
	if err = change(ctx, auth); err != nil {
		return out, err
	}
	out, err = s.issueRepairViewTx(ctx, auth, project, issue)
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, key, hash, out); err != nil {
		return IssueRepairView{}, err
	}
	return out, auth.tx.Commit()
}

func issueRepairRequest(project, requested string, version int, reason string) bool {
	return project == requested && designVersion(version) && version >= 1 && testPlanText(reason, 2000, true)
}

func (s *Service) GetIssueRepair(ctx context.Context, actor Actor, project, issue string) (out IssueRepairView, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(issue, false) {
		return out, invalid(issueRepairInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, issue, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	return s.issueRepairViewTx(ctx, auth, project, issue)
}

func (s *Service) PrepareIssueRepair(ctx context.Context, actor Actor, project, issueID string, request IssueRepairPrepareRequest) (IssueRepairView, error) {
	if !issueRepairRequest(project, request.ProjectID, request.ExpectedIssueVersion, request.Reason) || requireTitle(strings.TrimSpace(request.Title)) != nil || !testPlanText(request.Title, 500, true) || !testPlanText(request.Description, 200000, false) || !handoffID(request.OwnerID, true) || len(request.AcceptanceCriteria) == 0 || len(request.AcceptanceCriteria) > 100 {
		return IssueRepairView{}, invalid(issueRepairInvalid)
	}
	for _, criterion := range request.AcceptanceCriteria {
		if !testPlanText(criterion, 2000, true) {
			return IssueRepairView{}, invalid(issueRepairInvalid)
		}
	}
	return s.issueRepairCommand(ctx, actor, project, issueID, request.IdempotencyKey, purposeProjectWrite, map[string]any{"issue_repair_prepare": issueID, "request": request}, func(ctx context.Context, auth *authorizedCommand) error {
		issue, err := s.getRecordTx(ctx, auth, project, "issues", issueID)
		if err != nil {
			return err
		}
		if issue.ID != issueID || issue.TenantID != auth.target.tenantID || issue.ProjectID != project {
			return missing()
		}
		if issue.Version != request.ExpectedIssueVersion {
			return conflict(issueRepairVersion)
		}
		previous, err := s.issueRepairLatestTx(ctx, auth, project, issueID)
		if err != nil {
			return err
		}
		if issue.Status != "open" || previous != nil && previous.ReopenedAt == "" {
			return conflict(issueRepairState)
		}
		if _, err = s.issueRepairOriginalCaseTx(ctx, auth, project, issueID); err != nil {
			return err
		}
		automationSource, err := s.issueRepairAutomationSourceTx(ctx, auth)
		if err != nil {
			return err
		}
		criteria := append([]string{}, request.AcceptanceCriteria...)
		criterionInput := make([]any, len(criteria))
		for i, criterion := range criteria {
			criterionInput[i] = criterion
		}
		requirement, err := s.saveSharedIssueRequirementTx(ctx, auth, project, issueID, map[string]any{"title": issue.Title, "description": issue.Description, "priority": issue.Priority, "acceptance_criteria": criterionInput})
		if err != nil {
			return err
		}
		if requirement.Status == "archived" || requirement.Status == "deleted" {
			return conflict(issueRepairState)
		}
		// This is an ordinary editable task. Only the separate protected row below
		// identifies it as the repair, and preparing never starts an execution.
		task := recordFrom(auth.currentPrincipal, project, map[string]any{"title": strings.TrimSpace(request.Title), "description": request.Description, "owner_id": request.OwnerID, "requirement_id": requirement.ID, "acceptance_criteria": criterionInput, "priority": issue.Priority})
		if err = s.validateRecord(ctx, auth.tx, auth, task, "tasks"); err != nil {
			return err
		}
		if err = s.insertCommandRecord(ctx, auth.tx, auth, task, "tasks"); err != nil {
			return err
		}
		cycle := 1
		if previous != nil {
			cycle = previous.Cycle + 1
		}
		repair := IssueRepair{ID: newID(), TenantID: auth.target.tenantID, ProjectID: project, IssueID: issueID, Cycle: cycle, Version: 1, RequirementID: requirement.ID, TaskID: task.ID, PreparedBy: auth.currentPrincipal.ID, CreatedAt: now(), UpdatedAt: now()}
		repair.AutomationSource = automationSource
		if err = s.saveIssueRepairTx(ctx, auth, repair, 0); err != nil {
			return err
		}
		if err = s.updateIssueRepairStateTx(ctx, auth, &issue, "in_progress"); err != nil {
			return err
		}
		return s.commandActivity(ctx, auth, "issue.repair_prepared", issueID, request.Reason)
	})
}

func (s *Service) issueRepairAutomationSourceTx(ctx context.Context, auth *authorizedCommand) (*IssueRepairAutomationSource, error) {
	run, err := s.failureDiagnosisIssueRunTx(ctx, auth)
	if err != nil {
		var apiError *APIError
		if errors.As(err, &apiError) && apiError.Status == http.StatusConflict {
			return nil, nil
		}
		return nil, err
	}
	if run == "" {
		return nil, nil
	}
	origin, err := s.verifiedIssueAutomationOriginTx(ctx, auth, run)
	if err != nil || origin == nil {
		return nil, err
	}
	return &IssueRepairAutomationSource{RunID: run, Origin: *origin}, nil
}

func (s *Service) BindIssueRegression(ctx context.Context, actor Actor, project, issueID string, request IssueRepairBindRequest) (IssueRepairView, error) {
	if !issueRepairRequest(project, request.ProjectID, request.ExpectedIssueVersion, request.Reason) || !designVersion(request.ExpectedRepairVersion) || request.ExpectedRepairVersion < 1 || !designVersion(request.ExpectedPlanVersion) || request.ExpectedPlanVersion < 1 || !handoffID(request.PlanID, false) || len(request.CaseKeys) < 1 || len(request.CaseKeys) > 100 {
		return IssueRepairView{}, invalid(issueRepairInvalid)
	}
	seen := map[string]bool{}
	for _, key := range request.CaseKeys {
		if !designKey(key) || seen[key] {
			return IssueRepairView{}, invalid(issueRepairInvalid)
		}
		seen[key] = true
	}
	return s.issueRepairCommand(ctx, actor, project, issueID, request.IdempotencyKey, purposeProjectWrite, map[string]any{"issue_repair_regression": issueID, "request": request}, func(ctx context.Context, auth *authorizedCommand) error {
		issue, repair, err := s.currentIssueRepairTx(ctx, auth, project, issueID, request.ExpectedIssueVersion, request.ExpectedRepairVersion)
		if err != nil {
			return err
		}
		if issue.Status != "in_progress" || repair.Closure != nil {
			return conflict(issueRepairState)
		}
		plan, err := s.testPlanTx(ctx, auth.tx, auth.target.tenantID, project, request.PlanID)
		if err != nil {
			return err
		}
		if plan.TaskID != repair.TaskID || plan.Version != request.ExpectedPlanVersion || plan.Published < 1 || plan.State == "retired" {
			return conflict(issueRepairRegression)
		}
		revision, err := s.testPlanRevisionTx(ctx, auth.tx, auth.target.tenantID, project, plan.ID, plan.Published)
		if err != nil {
			return err
		}
		repair.Regression = &IssueRegressionBinding{PlanID: plan.ID, PlanRevision: revision.Revision, PlanSHA256: revision.SHA256, CaseKeys: append([]string{}, request.CaseKeys...), BoundBy: auth.currentPrincipal.ID, BoundAt: now()}
		if _, _, err = s.issueRepairRegressionTx(ctx, auth, project, *repair); err != nil {
			return err
		}
		if err = s.issueRepairSelectedCasesTx(ctx, auth, project, issueID, *repair, revision); err != nil {
			return err
		}
		original := repair.Version
		repair.Version++
		repair.UpdatedAt = now()
		if err = s.saveIssueRepairTx(ctx, auth, *repair, original); err != nil {
			return err
		}
		if err = s.updateIssueRepairStateTx(ctx, auth, &issue, issue.Status); err != nil {
			return err
		}
		return s.commandActivity(ctx, auth, "issue.regression_bound", issueID, request.Reason)
	})
}

func (s *Service) currentIssueRepairTx(ctx context.Context, auth *authorizedCommand, project, issueID string, issueVersion, repairVersion int) (Record, *IssueRepair, error) {
	issue, err := s.getRecordTx(ctx, auth, project, "issues", issueID)
	if err != nil {
		return issue, nil, err
	}
	repair, err := s.issueRepairLatestTx(ctx, auth, project, issueID)
	if err != nil {
		return issue, nil, err
	}
	if issue.Version != issueVersion || repair == nil || repair.Version != repairVersion {
		return issue, repair, conflict(issueRepairVersion)
	}
	if repair.ReopenedAt != "" {
		return issue, repair, conflict(issueRepairState)
	}
	return issue, repair, nil
}

func (s *Service) CloseIssueRepair(ctx context.Context, actor Actor, project, issueID string, request IssueRepairCloseRequest) (IssueRepairView, error) {
	if !issueRepairRequest(project, request.ProjectID, request.ExpectedIssueVersion, request.Reason) || !designVersion(request.ExpectedRepairVersion) || request.ExpectedRepairVersion < 1 || !testPlanRunID(request.RunID) {
		return IssueRepairView{}, invalid(issueRepairInvalid)
	}
	return s.issueRepairCommand(ctx, actor, project, issueID, request.IdempotencyKey, purposeBrowserProjectApprove, map[string]any{"issue_repair_close": issueID, "request": request}, func(ctx context.Context, auth *authorizedCommand) error {
		issue, repair, err := s.currentIssueRepairTx(ctx, auth, project, issueID, request.ExpectedIssueVersion, request.ExpectedRepairVersion)
		if err != nil {
			return err
		}
		if issue.Status != "in_progress" || repair.Closure != nil {
			return conflict(issueRepairState)
		}
		packageOrigin, err := s.issueRepairOriginalPackageTx(ctx, auth, project, issueID)
		if err != nil {
			return err
		}
		closure, blockers, err := s.issueRepairClosureTx(ctx, auth, project, *repair, request.RunID, packageOrigin)
		if err != nil {
			return err
		}
		if len(blockers) != 0 || closure == nil {
			return conflict(issueRepairGate)
		}
		closure.ClosedBy, closure.ClosedAt, closure.Reason = auth.currentPrincipal.ID, now(), request.Reason
		repair.Closure = closure
		original := repair.Version
		repair.Version++
		repair.UpdatedAt = now()
		if err = s.saveIssueRepairTx(ctx, auth, *repair, original); err != nil {
			return err
		}
		if err = s.updateIssueRepairStateTx(ctx, auth, &issue, "closed"); err != nil {
			return err
		}
		return s.commandActivity(ctx, auth, "issue.closed", issueID, request.Reason)
	})
}

func (s *Service) ReopenIssueRepair(ctx context.Context, actor Actor, project, issueID string, request IssueRepairReopenRequest) (IssueRepairView, error) {
	if !issueRepairRequest(project, request.ProjectID, request.ExpectedIssueVersion, request.Reason) || !designVersion(request.ExpectedRepairVersion) || request.ExpectedRepairVersion < 1 {
		return IssueRepairView{}, invalid(issueRepairInvalid)
	}
	return s.issueRepairCommand(ctx, actor, project, issueID, request.IdempotencyKey, purposeBrowserProjectApprove, map[string]any{"issue_repair_reopen": issueID, "request": request}, func(ctx context.Context, auth *authorizedCommand) error {
		issue, repair, err := s.currentIssueRepairTx(ctx, auth, project, issueID, request.ExpectedIssueVersion, request.ExpectedRepairVersion)
		if err != nil {
			return err
		}
		if issue.Status != "closed" || repair.Closure == nil {
			return conflict(issueRepairState)
		}
		original := repair.Version
		repair.Version++
		repair.ReopenedAt, repair.ReopenedBy, repair.ReopenReason, repair.UpdatedAt = now(), auth.currentPrincipal.ID, request.Reason, now()
		if err = s.saveIssueRepairTx(ctx, auth, *repair, original); err != nil {
			return err
		}
		if err = s.updateIssueRepairStateTx(ctx, auth, &issue, "open"); err != nil {
			return err
		}
		return s.commandActivity(ctx, auth, "issue.reopened", issueID, request.Reason)
	})
}
