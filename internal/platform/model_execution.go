package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/modelcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	modelExecutionInvalid     = "Invalid native model execution selection"
	modelExecutionUnsupported = "Independent model selection requires an approved native workspace tool"
	modelExecutionChanged     = "Model deployment, parameters or rate changed; prepare a new dispatch preview"
	modelExecutionUnavailable = "The selected model deployment is not in the current authorized worker inventory"
)

func supportsModelExecution(p ToolProfile) bool {
	if supportsSingleCallReadOnlyModel(p) {
		return true
	}
	return p.Profile.ModelExecutionProtocol == modelcontract.Protocol && !p.Profile.ReadOnly && p.Profile.WorkspaceToolsVersion == "attempt-workspace-v1" && (p.Profile.Adapter == "suma" && p.Profile.Protocol == "suma-jsonl-v1" || supportsOctopExecution(p))
}

// A recommendation is a separate read-only purpose: no workspace capability,
// one bounded model request, and the same Go-owned provider/budget authority.
func supportsDispatchRecommendationModel(p ToolProfile) bool {
	return tc.SupportsDispatchRecommendationProfile(p.Profile)
}

// Single-call read-only purposes share the model/budget authority, without
// inheriting workspace capabilities or a second model configuration path.
func supportsSingleCallReadOnlyModel(p ToolProfile) bool {
	return supportsDispatchRecommendationModel(p) || tc.SupportsTaskConversationReplyProfile(p.Profile)
}
func matchingModelRate(rate *UsageRateRevision, profile ToolProfile, d modelcontract.Deployment) bool {
	return rate != nil && rate.ToolProfileID == profile.ID && rate.ProfileSHA256 == profile.Profile.ProfileSHA256 && rate.ModelDeploymentID == d.ID && rate.ModelDeploymentRevision == d.Revision && rate.ModelDeploymentSHA256 == d.SHA256 && rate.Model == d.Model && rate.EndpointOrigin == d.EndpointOrigin
}
func (s *Service) currentModelDeploymentsTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile) (ToolProfile, []modelcontract.Deployment, error) {
	if !supportsModelExecution(p) {
		return p, nil, invalid(modelExecutionUnsupported)
	}
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if err != nil {
		return current, nil, err
	}
	if !current.Available || current.Observation == nil {
		return current, nil, conflict(modelExecutionUnavailable)
	}
	return current, current.Observation.ModelDeployments, nil
}
func (s *Service) GetModelExecutionOptions(ctx context.Context, actor Actor, project string, req ModelExecutionOptionsRequest) (out ModelExecutionOptions, err error) {
	out.Items = []ModelExecutionOption{}
	if !taskConversationActor(actor) {
		return out, forbidden()
	}
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(req.ToolProfileID, false) || req.ExpectedToolVersion < 1 {
		return out, invalid(modelExecutionInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeProjectRead, func(auth *authorizedCommand) error {
		p, e := s.toolProfileTx(auth.ctx, auth.tx, auth.currentPrincipal, project, req.ToolProfileID)
		if e != nil {
			return e
		}
		if p.Version != req.ExpectedToolVersion {
			return conflict(modelExecutionChanged)
		}
		clock, e := s.automationClock(auth.ctx, auth.tx)
		if e != nil {
			return e
		}
		out = ModelExecutionOptions{ProjectID: project, ToolProfileID: p.ID, ToolProfileVersion: p.Version, ToolProfileSHA256: p.Profile.ProfileSHA256, ObservedAt: automationStamp(clock), Items: []ModelExecutionOption{}}
		out.CodexReason = codexExecutionUnsupported
		if supportsACPExecution(p) {
			current, err := s.toolProfileObservationLockedTx(auth.ctx, auth.tx, auth.target.tenantID, project, p)
			if err != nil {
				return err
			}
			if !current.Available {
				return conflict(acpExecutionUnavailable)
			}
			out.FixedACP = tc.CloneProfile(current.Profile).ACPExecution
			out.Reason = "fixed_by_tool"
			return nil
		}
		if supportsClaudeExecution(p) {
			current, err := s.toolProfileObservationLockedTx(auth.ctx, auth.tx, auth.target.tenantID, project, p)
			if err != nil {
				return err
			}
			if !current.Available {
				return conflict(claudeExecutionUnavailable)
			}
			out.FixedClaude = tc.CloneProfile(current.Profile).ClaudeExecution
			out.Reason = "fixed_by_tool"
			return nil
		}
		if supportsClaudeSDKExecution(p) {
			current, err := s.toolProfileObservationLockedTx(auth.ctx, auth.tx, auth.target.tenantID, project, p)
			if err != nil {
				return err
			}
			if !current.Available {
				return conflict(claudeSDKExecutionUnavailable)
			}
			out.FixedClaudeSDK = tc.CloneProfile(current.Profile).ClaudeSDKExecution
			out.Reason = "fixed_by_tool"
			return nil
		}
		if supportsOpenCodeExecution(p) {
			current, err := s.toolProfileObservationLockedTx(auth.ctx, auth.tx, auth.target.tenantID, project, p)
			if err != nil {
				return err
			}
			if !current.Available {
				return conflict(opencodeExecutionUnavailable)
			}
			out.FixedOpenCode = tc.CloneProfile(current.Profile).OpenCodeExecution
			out.Reason = "fixed_by_tool"
			return nil
		}
		if supportsCodexExecution(p) {
			out.Reason = modelExecutionUnsupported
			out.CodexCatalog, e = s.currentCodexCatalogTx(auth.ctx, auth.tx, auth.target.tenantID, project, p)
			if e != nil {
				return e
			}
			if e = s.checkCodexBudgetTx(auth.ctx, auth.tx, auth.target.tenantID, project); e != nil {
				var api *APIError
				if errors.As(e, &api) {
					out.CodexReason = api.Message
					return nil
				}
				return e
			}
			out.CodexSupported, out.CodexReason = true, ""
			return nil
		}
		if !supportsModelExecution(p) {
			out.Reason = modelExecutionUnsupported
			return nil
		}
		current, deployments, e := s.currentModelDeploymentsTx(auth.ctx, auth.tx, auth.target.tenantID, project, p)
		if e != nil {
			return e
		}
		rate, e := s.usageRateTx(auth.ctx, auth.tx, auth.target.tenantID, project, p.ID)
		if e != nil {
			return e
		}
		out.Supported = true
		out.ParameterLimits = &modelcontract.Parameters{MaxSteps: modelcontract.MaxSteps, MaxOutputTokens: modelcontract.MaxOutputTokens}
		if supportsSingleCallReadOnlyModel(p) {
			out.ParameterLimits = &modelcontract.Parameters{MaxSteps: 1, MaxOutputTokens: 8192}
		}
		for _, d := range deployments {
			if e = modelcontract.ValidateDeployment(d); e != nil {
				return invalid(modelExecutionInvalid)
			}
			option := ModelExecutionOption{Deployment: d, CostState: "unpriced", DefaultParameters: modelcontract.Parameters{MaxSteps: 8, MaxOutputTokens: 4096}, InputTokenReservation: d.ContextWindow * 8, OutputTokenReservation: 4096}
			if supportsSingleCallReadOnlyModel(p) {
				option.DefaultParameters.MaxSteps = 1
				option.InputTokenReservation = d.ContextWindow
			}
			if matchingModelRate(rate, current, d) {
				copy := *rate
				option.Rate = &copy
				option.CostState = "priced"
			}
			out.Items = append(out.Items, option)
		}
		return nil
	})
	return out, err
}

func (s *Service) buildModelExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project string, profile ToolProfile, selection *modelcontract.Selection) (*modelcontract.Binding, error) {
	if selection == nil {
		if supportsModelExecution(profile) {
			return nil, invalid(modelExecutionInvalid)
		}
		return nil, nil
	}
	if modelcontract.ValidateSelection(*selection) != nil {
		return nil, invalid(modelExecutionInvalid)
	}
	if supportsSingleCallReadOnlyModel(profile) && (selection.Parameters.MaxSteps != 1 || selection.Parameters.MaxOutputTokens > 8192) {
		return nil, invalid(modelExecutionInvalid)
	}
	_, deployments, err := s.currentModelDeploymentsTx(ctx, tx, tenant, project, profile)
	if err != nil {
		return nil, err
	}
	for _, d := range deployments {
		if d.ID != selection.DeploymentID || d.Revision != selection.DeploymentRevision || d.SHA256 != selection.DeploymentSHA256 {
			continue
		}
		rate, err := s.usageRateTx(ctx, tx, tenant, project, profile.ID)
		if err != nil {
			return nil, err
		}
		var snapshot *modelcontract.RateSnapshot
		if matchingModelRate(rate, profile, d) {
			snapshot = &modelcontract.RateSnapshot{ID: rate.ID, Version: rate.Version, Currency: rate.Currency, InputRateMicrosPerMillion: rate.InputRateMicrosPerMillion, OutputRateMicrosPerMillion: rate.OutputRateMicrosPerMillion}
		}
		binding, err := modelcontract.NewBinding(d, selection.Parameters, selection.Budget, snapshot)
		if err != nil {
			return nil, invalid(modelExecutionInvalid)
		}
		for _, scope := range []string{"organization", "project"} {
			policy, e := s.usagePolicyTx(ctx, tx, tenant, project, scope)
			if e != nil {
				return nil, e
			}
			if policy.Mode == "enforce" && (snapshot == nil || snapshot.Currency != policy.Currency) {
				return nil, invalid(usageUnavailable)
			}
		}
		return &binding, nil
	}
	return nil, conflict(modelExecutionUnavailable)
}

func bindModelExecutionWorkflow(w *dag.Workflow, node string, b *modelcontract.Binding) error {
	for i := range w.Nodes {
		if w.Nodes[i].ID != node {
			continue
		}
		delete(w.Nodes[i].Config, "model_execution")
		if b != nil {
			// Disable inherited template retries only in this reviewed copy. An
			// unknown provider result must retain its original attempt and hold.
			w.Nodes[i].MaxRetries = 0
			var raw map[string]any
			if err := json.Unmarshal([]byte(stringify(b)), &raw); err != nil {
				return err
			}
			w.Nodes[i].Config["model_execution"] = raw
			w.Nodes[i].Config["max_steps"] = b.Parameters.MaxSteps
			w.Nodes[i].Config["max_output_tokens"] = b.Parameters.MaxOutputTokens
			if b.Parameters.MaxSteps > modelcontract.MaxSteps || b.Parameters.MaxOutputTokens > modelcontract.MaxOutputTokens {
				return invalid(modelExecutionInvalid)
			}
		}
		return dag.Compile(*w)
	}
	return invalid(modelExecutionInvalid)
}

func (s *Service) validateModelExecutionPlanTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, plan TaskDispatchPlan) error {
	if err := s.validateACPExecutionPlanTx(ctx, tx, tenant, project, p, plan); err != nil {
		return err
	}
	if err := s.validateClaudeSDKExecutionPlanTx(ctx, tx, tenant, project, p, plan); err != nil {
		return err
	}
	if err := s.validateClaudeExecutionPlanTx(ctx, tx, tenant, project, p, plan); err != nil {
		return err
	}
	if err := s.validateOpenCodeExecutionPlanTx(ctx, tx, tenant, project, p, plan); err != nil {
		return err
	}
	var node *dag.Node
	for i := range plan.Workflow.Nodes {
		if plan.Workflow.Nodes[i].ID == plan.AgentNodeID {
			node = &plan.Workflow.Nodes[i]
			break
		}
	}
	if node == nil {
		return invalid(modelExecutionInvalid)
	}
	if supportsOctopExecution(p) && tc.ValidateOctopNodeConfiguration(node.Config) != nil {
		return conflict(modelExecutionChanged)
	}
	b, err := dag.ModelBinding(*node)
	if err != nil {
		return invalid(modelExecutionInvalid)
	}
	if stringify(b) != stringify(plan.ModelExecution) {
		return conflict(modelExecutionChanged)
	}
	if b == nil {
		if supportsModelExecution(p) {
			return conflict(modelExecutionChanged)
		}
		return nil
	}
	_, deployments, err := s.currentModelDeploymentsTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	found := false
	for _, d := range deployments {
		if d == b.Deployment {
			found = true
			break
		}
	}
	if !found {
		return conflict(modelExecutionUnavailable)
	}
	if b.Rate != nil {
		rate, err := s.modelExecutionExactRateTx(ctx, tx, tenant, project, p.ID, *b)
		if err != nil {
			return err
		}
		if rate == nil || rate.ProfileSHA256 != p.Profile.ProfileSHA256 {
			return conflict(modelExecutionChanged)
		}
	}
	return nil
}

func (s *Service) modelExecutionExactRateTx(ctx context.Context, tx *sql.Tx, tenant, project, profile string, b modelcontract.Binding) (*UsageRateRevision, error) {
	if b.Rate == nil {
		return nil, nil
	}
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT payload FROM suma_usage_rates WHERE tenant_id=? AND project_id=? AND tool_profile_id=? AND id=? AND version=?`), tenant, project, profile, b.Rate.ID, b.Rate.Version).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var rate UsageRateRevision
	if err = json.Unmarshal([]byte(raw), &rate); err != nil {
		return nil, err
	}
	if rate.ModelDeploymentID != b.Deployment.ID || rate.ModelDeploymentRevision != b.Deployment.Revision || rate.ModelDeploymentSHA256 != b.Deployment.SHA256 || rate.Model != b.Deployment.Model || rate.EndpointOrigin != b.Deployment.EndpointOrigin || rate.Currency != b.Rate.Currency || rate.InputRateMicrosPerMillion != b.Rate.InputRateMicrosPerMillion || rate.OutputRateMicrosPerMillion != b.Rate.OutputRateMicrosPerMillion {
		return nil, nil
	}
	return &rate, nil
}

func (s *Service) modelExecutionClaimEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, n dag.Node) (bool, error) {
	if eligible, err := s.acpExecutionClaimEligibleTx(ctx, tx, tenant, project, p, n); err != nil || !eligible {
		return false, err
	}
	if eligible, err := s.claudeSDKExecutionClaimEligibleTx(ctx, tx, tenant, project, p, n); err != nil || !eligible {
		return eligible, err
	}
	if eligible, err := s.claudeExecutionClaimEligibleTx(ctx, tx, tenant, project, p, n); err != nil || !eligible {
		return false, err
	}
	if supportsOctopExecution(p) && tc.ValidateOctopNodeConfiguration(n.Config) != nil {
		return false, nil
	}
	if eligible, err := s.openCodeExecutionClaimEligibleTx(ctx, tx, tenant, project, p, n); err != nil || !eligible {
		return false, err
	}
	if eligible, err := s.codexExecutionClaimEligibleTx(ctx, tx, tenant, project, p, n); err != nil || !eligible {
		return false, err
	}
	b, err := dag.ModelBinding(n)
	if err != nil {
		return false, nil
	}
	if b == nil {
		return !supportsModelExecution(p), nil
	}
	if !supportsModelExecution(p) {
		return false, nil
	}
	_, deployments, err := s.currentModelDeploymentsTx(ctx, tx, tenant, project, p)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	for _, d := range deployments {
		if d == b.Deployment {
			return true, nil
		}
	}
	return false, nil
}
