package platform

import (
	"context"
	"database/sql"
	"encoding/json"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	drc "github.com/ai-mada/suma/internal/dispatchrecommendationcontract"
	mc "github.com/ai-mada/suma/internal/modelcontract"
)

// The model and human editor may select only an existing bounded alternative.
// Deployment IDs, endpoint, budget, rates and catalogs always come from source.
func dispatchRecommendationSelection(source DispatchRecommendationSource, choice DispatchRecommendationChoice) (TaskDispatchPreviewRequest, error) {
	if !contentDigest.MatchString(choice.CandidateID) || !testPlanText(choice.Reason, 2000, true) {
		return TaskDispatchPreviewRequest{}, invalid(dispatchRecommendationOutput)
	}
	var candidate *DispatchRecommendationCandidate
	for i := range source.Candidates {
		if source.Candidates[i].ID == choice.CandidateID {
			candidate = &source.Candidates[i]
			break
		}
	}
	if candidate == nil {
		return TaskDispatchPreviewRequest{}, invalid(dispatchRecommendationOutput)
	}
	request := TaskDispatchPreviewRequest{ProjectID: "", TaskID: source.Context.Task.ID, ExpectedTaskVersion: source.Context.Task.Version, WorkflowID: source.Workflow.ID, WorkflowVersion: source.Workflow.Version, AgentNodeID: source.AgentNodeID, ToolProfileID: candidate.ToolProfileID, ExpectedToolVersion: candidate.ToolProfileVersion, Knowledge: knowledgeReferences(source.Context.Knowledge)}
	switch candidate.Kind {
	case "model":
		if candidate.ModelSelection == nil || choice.MaxSteps == nil || choice.MaxOutputTokens == nil || choice.ReasoningEffort != nil || *choice.MaxSteps < 1 || *choice.MaxSteps > source.Constraints.MaxSteps || *choice.MaxOutputTokens < 1 || *choice.MaxOutputTokens > source.Constraints.MaxOutputTokens {
			return request, invalid(dispatchRecommendationOutput)
		}
		var selection mc.Selection
		if json.Unmarshal([]byte(stringify(candidate.ModelSelection)), &selection) != nil {
			return request, invalid(dispatchRecommendationOutput)
		}
		selection.Parameters = mc.Parameters{MaxSteps: *choice.MaxSteps, MaxOutputTokens: *choice.MaxOutputTokens}
		if mc.ValidateSelection(selection) != nil {
			return request, invalid(dispatchRecommendationOutput)
		}
		request.ModelExecutionSelection = &selection
	case "codex":
		if candidate.CodexSelection == nil || choice.MaxSteps != nil || choice.MaxOutputTokens != nil || choice.ReasoningEffort == nil || source.Constraints.RequireHardBudget || source.Constraints.Budget != nil {
			return request, invalid(dispatchRecommendationOutput)
		}
		found := false
		for _, effort := range candidate.ReasoningEfforts {
			if effort == *choice.ReasoningEffort {
				found = true
				break
			}
		}
		if !found {
			return request, invalid(dispatchRecommendationOutput)
		}
		var selection cc.Selection
		if json.Unmarshal([]byte(stringify(candidate.CodexSelection)), &selection) != nil {
			return request, invalid(dispatchRecommendationOutput)
		}
		selection.ReasoningEffort = *choice.ReasoningEffort
		request.CodexExecutionSelection = &selection
	case "fixed":
		if candidate.ModelSelection != nil || candidate.CodexSelection != nil || choice.MaxSteps != nil || choice.MaxOutputTokens != nil || choice.ReasoningEffort != nil || source.Constraints.RequireHardBudget || source.Constraints.Budget != nil {
			return request, invalid(dispatchRecommendationOutput)
		}
	default:
		return request, invalid(dispatchRecommendationOutput)
	}
	return request, nil
}

func validateDispatchRecommendationProposal(source DispatchRecommendationSource, proposal DispatchRecommendationProposal) error {
	if drc.ValidateProposal(proposal) != nil {
		return invalid(dispatchRecommendationOutput)
	}
	for _, choice := range proposal.Recommendations {
		if _, err := dispatchRecommendationSelection(source, choice); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) AdoptDispatchRecommendation(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req DispatchRecommendationAdoptRequest) (out DispatchRecommendationAdoption, err error) {
	if engine == nil || !handoffID(project, false) || !handoffID(id, false) || req.ProjectID != project || req.ExpectedVersion < 1 || !contentDigest.MatchString(req.SourceSHA256) || !testPlanText(req.Reason, 2000, true) || !planningKey(req.IdempotencyKey) {
		return out, invalid(dispatchRecommendationInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectWrite)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"dispatch_recommendation_adopt": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return DispatchRecommendationAdoption{}, err
		}
		if err = s.dispatchRecommendationContextReadTx(ctx, auth, project, out.Recommendation.Source.Context); err != nil {
			return DispatchRecommendationAdoption{}, err
		}
		if err = s.dispatchRecommendationContextReadTx(ctx, auth, project, out.Plan.Context); err != nil {
			return DispatchRecommendationAdoption{}, err
		}
		return out, nil
	}
	record, err := s.dispatchRecommendationTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	a := record.Recommendation
	if err = s.dispatchRecommendationContextReadTx(ctx, auth, project, a.Source.Context); err != nil {
		return out, err
	}
	if a.Version != req.ExpectedVersion || a.State != "ready" || a.Proposal == nil || a.PlanID != "" || a.Source.SHA256 != req.SourceSHA256 {
		return out, conflict(dispatchRecommendationVersion)
	}
	if err = s.checkDispatchRecommendationBasisTx(ctx, engine, auth, record); err != nil {
		return out, err
	}
	request, err := dispatchRecommendationSelection(a.Source, req.Choice)
	if err != nil {
		return out, err
	}
	request.ProjectID = project
	// Test authority is the human's explicit adoption input, never part of
	// the model's closed five-field Choice or the paid analysis source.
	request.TestBindings = req.TestBindings
	plan, err := s.buildTaskDispatchSnapshotTx(ctx, engine, auth, project, request)
	if err != nil {
		return out, err
	}
	plan.ID, plan.RunID = newID(), newID()
	plan.Recommendation = &DispatchRecommendationProvenance{RecommendationID: id, RecommendationVersion: a.Version, SourceSHA256: a.Source.SHA256, OriginalOutputSHA256: a.OriginalOutputSHA256, CandidateID: req.Choice.CandidateID, ChoiceSHA256: digest(stringify(req.Choice)), AdoptedBy: auth.currentPrincipal.ID, AdoptedAt: now(), Reason: req.Reason}
	if err = s.persistTaskDispatchSnapshotTx(ctx, auth, &plan); err != nil {
		return out, err
	}
	previous := a.Version
	a.Version++
	a.State = "adopted"
	a.PlanID = plan.ID
	a.UpdatedAt = now()
	a.AllowedActions = []string{}
	var adoptedChoice DispatchRecommendationChoice
	if err = json.Unmarshal([]byte(stringify(req.Choice)), &adoptedChoice); err != nil {
		return out, err
	}
	a.AdoptedChoice = &adoptedChoice
	a.AdoptedTestBindings = append([]TaskDispatchTestBinding(nil), plan.TestBindings...)
	record.Recommendation = a
	if err = s.saveDispatchRecommendationTx(ctx, auth.tx, auth.target.tenantID, record, previous); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "task.dispatch_recommendation_adopted", id, req.Reason); err != nil {
		return out, err
	}
	out = DispatchRecommendationAdoption{Recommendation: a, Plan: plan}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

// Ordinary preview/start remains the dispatch authority. This only verifies
// that its optional recommendation provenance came from the exact protected
// adoption, rather than editable task metadata or an arbitrary analysis ID.
func (s *Service) validateDispatchRecommendationProvenanceTx(ctx context.Context, tx *sql.Tx, tenant, project string, plan TaskDispatchPlan) error {
	p := plan.Recommendation
	if p == nil {
		return nil
	}
	record, err := s.dispatchRecommendationTx(ctx, tx, tenant, project, p.RecommendationID)
	if err != nil {
		return err
	}
	a := record.Recommendation
	if a.State != "adopted" || a.PlanID != plan.ID || a.Version != p.RecommendationVersion+1 || a.Source.SHA256 != p.SourceSHA256 || a.OriginalOutputSHA256 != p.OriginalOutputSHA256 || digest(a.OriginalOutput) != p.OriginalOutputSHA256 || a.AdoptedChoice == nil || a.AdoptedChoice.CandidateID != p.CandidateID || digest(stringify(a.AdoptedChoice)) != p.ChoiceSHA256 || !taskDispatchTestBindingsConsistent(plan) || !sameTaskDispatchTestBindings(a.AdoptedTestBindings, plan.TestBindings) {
		return conflict(dispatchRecommendationVersion)
	}
	request, err := dispatchRecommendationSelection(a.Source, *a.AdoptedChoice)
	if err != nil {
		return err
	}
	if plan.TaskID != request.TaskID || plan.TaskVersion != request.ExpectedTaskVersion || plan.WorkflowID != request.WorkflowID || plan.WorkflowVersion != request.WorkflowVersion || plan.AgentNodeID != request.AgentNodeID || plan.ToolProfileID != request.ToolProfileID || plan.ToolProfileVersion != request.ExpectedToolVersion {
		return conflict(dispatchRecommendationVersion)
	}
	if request.ModelExecutionSelection != nil {
		s := request.ModelExecutionSelection
		b := plan.ModelExecution
		if b == nil || b.Deployment.ID != s.DeploymentID || b.Deployment.Revision != s.DeploymentRevision || b.Deployment.SHA256 != s.DeploymentSHA256 || b.Parameters != s.Parameters || stringify(b.Budget) != stringify(s.Budget) {
			return conflict(dispatchRecommendationVersion)
		}
	} else if plan.ModelExecution != nil {
		return conflict(dispatchRecommendationVersion)
	}
	if request.CodexExecutionSelection != nil {
		s := request.CodexExecutionSelection
		b := plan.CodexExecution
		if b == nil || b.CatalogSHA256 != s.CatalogSHA256 || b.Model != s.Model || b.ReasoningEffort != s.ReasoningEffort || stringify(b.TokenNoticeLimit) != stringify(s.TokenNoticeLimit) {
			return conflict(dispatchRecommendationVersion)
		}
	} else if plan.CodexExecution != nil {
		return conflict(dispatchRecommendationVersion)
	}
	return nil
}
