package platform

import (
	"context"
	"encoding/json"
	"errors"
)

func (s *Service) intelligentOperationPreviewTx(ctx context.Context, auth *authorizedCommand, analysis IntelligentOperationAnalysis) (out IntelligentOperationPreview, err error) {
	out = IntelligentOperationPreview{ID: analysis.ID, ProjectID: analysis.ProjectID, Version: analysis.Version, SourceSHA256: analysis.Source.SHA256, Policy: operationPolicy, Questions: []string{}, Defaults: []IntelligentOperationDefault{}, Steps: []IntelligentOperationImpact{}, BasisCurrent: true}
	if analysis.Proposal == nil {
		return out, conflict(operationVersion)
	}
	proposal, err := parseIntelligentOperationOutput(stringify(analysis.Proposal), analysis.Source)
	if err != nil {
		return out, err
	}
	out.ProposalSHA256 = digest(stringify(proposal))
	out.Questions = append([]string{}, proposal.Questions...)
	if err = s.checkIntelligentOperationSourceTx(ctx, auth.tx, auth.target.tenantID, analysis.ProjectID, analysis.Source); err != nil {
		var api *APIError
		if !errors.As(err, &api) {
			return out, err
		}
		out.BasisCurrent = false
	}
	ordered, err := operationOrderedSteps(proposal)
	if err != nil {
		return out, err
	}
	for _, step := range ordered {
		impact := IntelligentOperationImpact{Key: step.Key, Action: step.Action, DependsOn: append([]string{}, step.DependsOn...), RequiredPermission: "project.write", Risk: "draft_only"}
		switch step.Action {
		case "requirement.create_draft":
			impact.TargetKind, impact.Title = "requirements", step.Requirement.Title
			out.Defaults = append(out.Defaults, IntelligentOperationDefault{StepKey: step.Key, Field: "status", Value: "draft", Source: operationPolicy})
			if step.Requirement.Priority == "" {
				out.Defaults = append(out.Defaults, IntelligentOperationDefault{StepKey: step.Key, Field: "priority", Value: "medium", Source: "requirement.default"})
			}
		case "requirement.design.save_draft":
			impact.TargetKind, impact.TargetID, impact.TargetStepKey = "requirement_designs", step.Design.RequirementID, step.Design.RequirementStepKey
			// Preview holds a read proof. Resolve referenced owners without
			// manufacturing the write proof required by the actual domain save.
			for _, task := range step.Design.Tasks {
				if task.OwnerID == "" {
					continue
				}
				owner, e := s.getRecordTx(ctx, auth, analysis.ProjectID, "members", task.OwnerID)
				if e != nil {
					return out, e
				}
				if owner.Status == "archived" || owner.Status == "deleted" {
					return out, conflict(operationSource)
				}
				if owner.Kind == "human" {
					user, _ := owner.Data["user_id"].(string)
					if _, e = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, user, analysis.ProjectID, false, false); e != nil {
						return out, e
					}
				}
			}
			if impact.TargetID != "" {
				requirement, e := s.getRecordTx(ctx, auth, analysis.ProjectID, "requirements", impact.TargetID)
				if e != nil {
					return out, e
				}
				if requirement.Status == "archived" || requirement.Status == "deleted" {
					return out, conflict(operationSource)
				}
				aggregate, e := s.designAggregateTx(ctx, auth, analysis.ProjectID, impact.TargetID)
				if e != nil {
					return out, e
				}
				impact.CurrentVersion, impact.RequirementVersion, impact.Title = aggregate.Version, requirement.Version, requirement.Title
			} else {
				for _, target := range proposal.Steps {
					if target.Key == impact.TargetStepKey {
						impact.Title = target.Requirement.Title
					}
				}
				impact.RequirementVersion = 1
			}
			out.Defaults = append(out.Defaults, IntelligentOperationDefault{StepKey: step.Key, Field: "revision.state", Value: "draft", Source: operationPolicy})
		case "knowledge.create_draft":
			impact.TargetKind, impact.Title = "project_knowledge", step.Knowledge.Title
			out.Defaults = append(out.Defaults, IntelligentOperationDefault{StepKey: step.Key, Field: "state", Value: "draft", Source: operationPolicy})
		}
		out.Steps = append(out.Steps, impact)
	}
	out.CanApply = analysis.State == "ready" && out.BasisCurrent && len(out.Questions) == 0 && len(out.Steps) > 0
	if out.CanApply {
		_, err = s.currentProjectMembershipLockedTx(ctx, auth.tx, auth.target.tenantID, auth.currentPrincipal.ID, analysis.ProjectID, true, false)
		if err != nil {
			var api *APIError
			if !errors.As(err, &api) {
				return out, err
			}
			out.CanApply = false
		}
	}
	out.PreviewSHA256 = digest(stringify(map[string]any{"id": out.ID, "project_id": out.ProjectID, "version": out.Version, "source": analysis.Source, "proposal": proposal, "policy": out.Policy, "steps": out.Steps, "defaults": out.Defaults, "questions": out.Questions}))
	return out, nil
}

func (s *Service) PreviewIntelligentOperation(ctx context.Context, actor Actor, project, id string, req IntelligentOperationPreviewRequest) (out IntelligentOperationPreview, err error) {
	if !operationActor(actor) {
		return out, forbidden()
	}
	if req.ProjectID != project || req.ExpectedVersion < 1 || !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(operationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	r, err := s.intelligentOperationTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if r.Analysis.Version != req.ExpectedVersion {
		return out, conflict(operationVersion)
	}
	return s.intelligentOperationPreviewTx(ctx, auth, r.Analysis)
}

func operationRequirementBody(requirement IntelligentOperationRequirement) map[string]any {
	priority := requirement.Priority
	if priority == "" {
		priority = "medium"
	}
	return map[string]any{"title": requirement.Title, "description": requirement.Description, "priority": priority, "acceptance_criteria": requirement.AcceptanceCriteria, "status": "draft"}
}

func (s *Service) applyIntelligentOperationTx(ctx context.Context, auth *authorizedCommand, analysis IntelligentOperationAnalysis, preview IntelligentOperationPreview) ([]IntelligentOperationStepResult, error) {
	if err := assertProjectCommand(auth.tx, auth, analysis.ProjectID); err != nil {
		return nil, err
	}
	ordered, err := operationOrderedSteps(*analysis.Proposal)
	if err != nil {
		return nil, err
	}
	results := []IntelligentOperationStepResult{}
	byKey := map[string]IntelligentOperationStepResult{}
	impacts := map[string]IntelligentOperationImpact{}
	for _, impact := range preview.Steps {
		impacts[impact.Key] = impact
	}
	for _, step := range ordered {
		result := IntelligentOperationStepResult{Key: step.Key, Action: step.Action, SourceSHA256: analysis.Source.SHA256, ProposalSHA256: preview.ProposalSHA256, OriginalOutputSHA256: analysis.OriginalOutputSHA256, RunID: analysis.RunID}
		switch step.Action {
		case "requirement.create_draft":
			// Use the ordinary record domain validators and protected writer. Only
			// the whole batch has an operation key, so public record keys cannot
			// preplant an internal per-step receipt.
			record := recordFrom(auth.currentPrincipal, analysis.ProjectID, operationRequirementBody(*step.Requirement))
			if err = s.validateRecord(ctx, auth.tx, auth, record, "requirements"); err != nil {
				return nil, err
			}
			if err = s.insertCommandRecord(ctx, auth.tx, auth, record, "requirements"); err != nil {
				return nil, err
			}
			if err = s.commandActivity(ctx, auth, "requirements.created", record.ID, record.Title); err != nil {
				return nil, err
			}
			result.ResourceKind, result.ResourceID, result.Version, result.SHA256 = "requirements", record.ID, record.Version, digest(stringify(record))
		case "requirement.design.save_draft":
			impact := impacts[step.Key]
			requirement := step.Design.RequirementID
			if requirement == "" {
				requirement = byKey[step.Design.RequirementStepKey].ResourceID
			}
			if requirement == "" {
				return nil, invalid(operationOutput)
			}
			view, e := s.saveRequirementDesignDraftTx(ctx, auth, analysis.ProjectID, requirement, RequirementDesignSaveRequest{ProjectID: analysis.ProjectID, ExpectedVersion: impact.CurrentVersion, ExpectedRequirementVersion: impact.RequirementVersion, Content: step.Design.Content, Tasks: step.Design.Tasks})
			if e != nil {
				return nil, e
			}
			result.ResourceKind, result.ResourceID, result.Version, result.Revision, result.SHA256 = "requirement_designs", requirement, view.Version, view.LatestRevision, view.Latest.SHA256
		case "knowledge.create_draft":
			proposal := step.Knowledge
			sources := []ProjectKnowledgeSource{}
			for _, index := range proposal.SourceIndices {
				sources = append(sources, analysis.Source.Sources[index])
			}
			request := ProjectKnowledgeWriteRequest{ProjectID: analysis.ProjectID, Kind: proposal.Kind, Title: proposal.Title, Summary: proposal.Summary, Content: proposal.Content, Sources: []ProjectKnowledgeSourceInput{}}
			if err = validateKnowledgeDraft(request); err != nil {
				return nil, err
			}
			knowledge, e := s.knowledgeCreateTx(ctx, auth, analysis.ProjectID, request, sources, "model_assisted")
			if e != nil {
				return nil, e
			}
			result.ResourceKind, result.ResourceID, result.Version, result.Revision, result.SHA256 = "project_knowledge", knowledge.ID, knowledge.Version, knowledge.LatestRevision, knowledge.Latest.SHA256
		default:
			return nil, invalid(operationOutput)
		}
		results = append(results, result)
		byKey[step.Key] = result
	}
	return results, nil
}

func (s *Service) CommandIntelligentOperation(ctx context.Context, actor Actor, project, id string, req IntelligentOperationCommandRequest) (out IntelligentOperationAnalysis, err error) {
	if !operationActor(actor) {
		return out, forbidden()
	}
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || req.ExpectedVersion > 2147483646 || !planningText(req.Reason, 2000) || !planningKey(req.IdempotencyKey) || req.Action != "revise" && req.Action != "apply" && req.Action != "dismiss" {
		return out, invalid(operationInvalid)
	}
	if req.Action != "revise" && req.Proposal != nil || req.Action != "apply" && (req.PreviewSHA256 != "" || req.AcknowledgeDrafts) {
		return out, invalid(operationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"intelligent_operation_command": id, "body": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return s.intelligentOperationProjectionTx(ctx, auth, out)
	}
	record, err := s.intelligentOperationTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	out = record.Analysis
	if out.Version != req.ExpectedVersion || req.Action == "dismiss" && out.State != "ready" && out.State != "failed" || req.Action != "dismiss" && out.State != "ready" {
		return out, conflict(operationVersion)
	}
	switch req.Action {
	case "revise":
		if req.Proposal == nil {
			return out, invalid(operationOutput)
		}
		proposal, e := parseIntelligentOperationOutput(stringify(req.Proposal), out.Source)
		if e != nil {
			return out, e
		}
		out.Proposal = &proposal
	case "dismiss":
		out.State = "dismissed"
	case "apply":
		if !req.AcknowledgeDrafts || len(req.PreviewSHA256) != 64 {
			return out, invalid(operationDrafts)
		}
		if out.Proposal == nil {
			return out, invalid(operationOutput)
		}
		if len(out.Proposal.Questions) > 0 || len(out.Proposal.Steps) == 0 {
			return out, invalid(operationQuestions)
		}
		if err = s.checkIntelligentOperationSourceTx(ctx, auth.tx, auth.target.tenantID, project, out.Source); err != nil {
			return out, err
		}
		preview, e := s.intelligentOperationPreviewTx(ctx, auth, out)
		if e != nil {
			return out, e
		}
		if !preview.CanApply || preview.PreviewSHA256 != req.PreviewSHA256 {
			return out, conflict(operationDrafts)
		}
		out.Results, err = s.applyIntelligentOperationTx(ctx, auth, out, preview)
		if err != nil {
			return out, err
		}
		out.State, out.AppliedBy, out.AppliedAt, out.AppliedPreviewSHA256 = "applied", auth.currentPrincipal.ID, now(), preview.PreviewSHA256
	}
	old := out.Version
	out.Version++
	out.UpdatedAt = now()
	out.Reason = req.Reason
	record.Analysis = out
	record.SystemReason = false
	if err = s.saveIntelligentOperationTx(ctx, auth.tx, auth.target.tenantID, record, old); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "intelligent_operation."+req.Action, id, req.Reason); err != nil {
		return out, err
	}
	out, err = s.intelligentOperationProjectionTx(ctx, auth, out)
	if err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = auth.tx.Commit(); err != nil {
		return IntelligentOperationAnalysis{}, err
	}
	return out, nil
}
