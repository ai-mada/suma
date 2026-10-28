package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
)

const dispatchRecommendationMaxProfiles = 32
const dispatchRecommendationMaxCandidates = 24

// Source construction is a read inside the caller's current authority
// transaction. It neither reserves execution resources nor probes/calls a tool.
// The same implementation is used by options, analysis admission and adoption.
func (s *Service) dispatchRecommendationSourceTx(ctx context.Context, engine *dag.Engine, auth *authorizedCommand, project string, request DispatchRecommendationOptionsRequest) (out DispatchRecommendationSource, err error) {
	req, err := normalizeDispatchRecommendationOptions(request)
	if err != nil {
		return out, err
	}
	if auth == nil || auth.tx == nil || auth.target.projectID != project || req.ProjectID != project {
		return out, forbidden()
	}
	tenant := auth.target.tenantID
	if err = s.rejectReservedCandidateRepairTaskTx(ctx, auth.tx, tenant, project, req.TaskID, ""); err != nil {
		return out, err
	}
	contextSnapshot, err := s.taskContextForDispatchTx(ctx, auth, project, req.TaskID, req.Knowledge)
	if err != nil {
		return out, err
	}
	if contextSnapshot.Task.Version != req.ExpectedTaskVersion {
		return out, conflict(dispatchRecommendationSourceChanged)
	}
	var raw string
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT w.spec FROM dag_workflows w WHERE w.tenant_id=? AND w.project_id=? AND w.id=? AND w.version=? AND NOT EXISTS(SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=w.tenant_id AND m.project_id=w.project_id AND m.workflow_id=w.id)`), tenant, project, req.WorkflowID, req.WorkflowVersion).Scan(&raw)
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
	var route dag.ProjectExecutionRoute
	if engine != nil {
		route, err = engine.ReadProjectExecutionRouteTx(ctx, auth.tx, tenant, project)
	} else {
		route, err = dag.ReadProjectExecutionRouteTx(ctx, auth.tx, s.Dialect, tenant, project)
	}
	if err != nil {
		return out, err
	}
	profiles, err := s.listToolProfilesForProjectTx(ctx, auth, project, dispatchRecommendationMaxProfiles, "")
	if err != nil {
		return out, err
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	policies := []UsagePolicy{}
	policyBasis := []any{}
	hard := req.Constraints.RequireHardBudget || req.Constraints.Budget != nil
	for _, scope := range []string{"organization", "project"} {
		policy, e := s.usagePolicyTx(ctx, auth.tx, tenant, project, scope)
		if e != nil {
			return out, e
		}
		policies = append(policies, policy)
		// Configuration, not volatile balances or display permissions. Quotas
		// stay private; their digest still invalidates a changed policy basis.
		policyBasis = append(policyBasis, []any{scope, policy.ID, policy.Version, policy.Mode, policy.Currency, policy.TokenLimit, policy.CostLimitMicros})
		if policy.Mode == "enforce" {
			hard = true
		}
	}
	out = DispatchRecommendationSource{ObservedAt: automationStamp(clock), Context: contextSnapshot, Workflow: workflow, AgentNodeID: req.AgentNodeID, ExecutionRoute: route, PolicySHA256: digest(stringify(policyBasis)), Constraints: req.Constraints, Candidates: []DispatchRecommendationCandidate{}, Coverage: DispatchRecommendationCoverage{ProfileTotal: profiles.Total, ProfilesExamined: len(profiles.Items), MaxProfiles: dispatchRecommendationMaxProfiles, MaxCandidates: dispatchRecommendationMaxCandidates, Complete: profiles.NextCursor == "", Reason: "complete"}}
	// Round-robin across real profiles prevents the first tool's large model
	// catalogue from consuming the entire bounded semantic-choice context.
	groups := [][]DispatchRecommendationCandidate{}
	for _, profile := range profiles.Items {
		if !profile.Available {
			out.Coverage.ExcludedCount++
			continue
		}
		if e := s.checkDispatchProfileRouteTx(ctx, auth.tx, tenant, project, profile, route); e != nil {
			var api *APIError
			if !errors.As(e, &api) {
				return out, e
			}
			out.Coverage.ExcludedCount++
			continue
		}
		if _, e := buildTaskDispatchWorkflow(workflow, req.AgentNodeID, profile.ID, profile.Profile, contextSnapshot); e != nil {
			var api *APIError
			if !errors.As(e, &api) {
				return out, e
			}
			out.Coverage.ExcludedCount++
			continue
		}
		base := DispatchRecommendationCandidate{ToolProfileID: profile.ID, ToolProfileVersion: profile.Version, ToolProfileSHA256: profile.Profile.ProfileSHA256, WorkerID: profile.WorkerID, ToolName: profile.Name, Adapter: profile.Profile.Adapter, Protocol: profile.Profile.Protocol, InventoryObservedAt: profile.LastSeenAt, ModelState: "fixed_by_tool", CostState: "unknown", PerformanceState: "unknown", BudgetMode: "observe_only", ReasoningEfforts: []string{}}
		group := []DispatchRecommendationCandidate{}
		switch {
		case supportsModelExecution(profile):
			if profile.Observation == nil {
				out.Coverage.ExcludedCount++
				continue
			}
			deployments := append([]mc.Deployment{}, profile.Observation.ModelDeployments...)
			sort.Slice(deployments, func(i, j int) bool { return deployments[i].ID < deployments[j].ID })
			rate, e := s.usageRateTx(ctx, auth.tx, tenant, project, profile.ID)
			if e != nil {
				return out, e
			}
			for _, deployment := range deployments {
				if mc.ValidateDeployment(deployment) != nil {
					out.Coverage.ExcludedCount++
					continue
				}
				var quote *mc.RateSnapshot
				if matchingModelRate(rate, profile, deployment) {
					quote = &mc.RateSnapshot{ID: rate.ID, Version: rate.Version, Currency: rate.Currency, InputRateMicrosPerMillion: rate.InputRateMicrosPerMillion, OutputRateMicrosPerMillion: rate.OutputRateMicrosPerMillion}
				}
				priced := true
				for _, policy := range policies {
					if policy.Mode == "enforce" && (quote == nil || quote.Currency != policy.Currency) {
						priced = false
					}
				}
				if !priced {
					out.Coverage.ExcludedCount++
					continue
				}
				parameters, ok := dispatchRecommendationFeasibleParameters(deployment, req.Constraints, quote)
				if !ok {
					out.Coverage.ExcludedCount++
					continue
				}
				candidate := base
				candidate.Kind = "model"
				candidate.ModelState = "selectable"
				candidate.BudgetMode = "platform_enforced"
				candidate.Model = deployment.Model
				candidate.ContextWindow = deployment.ContextWindow
				candidate.CostState = "unpriced"
				candidate.ModelRate = quote
				if quote != nil {
					candidate.CostState = "quoted"
				}
				candidate.ModelSelection = &mc.Selection{DeploymentID: deployment.ID, DeploymentRevision: deployment.Revision, DeploymentSHA256: deployment.SHA256, Parameters: parameters, Budget: req.Constraints.Budget}
				group = append(group, candidate)
			}
		case supportsCodexExecution(profile):
			if hard {
				out.Coverage.ExcludedCount++
				continue
			}
			if e := s.checkCodexBudgetTx(ctx, auth.tx, tenant, project); e != nil {
				var api *APIError
				if !errors.As(e, &api) {
					return out, e
				}
				out.Coverage.ExcludedCount++
				continue
			}
			catalog, e := s.currentCodexCatalogTx(ctx, auth.tx, tenant, project, profile)
			if e != nil {
				var api *APIError
				if !errors.As(e, &api) {
					return out, e
				}
				out.Coverage.ExcludedCount++
				continue
			}
			models := append([]cc.ModelOption{}, catalog.Models...)
			sort.Slice(models, func(i, j int) bool { return models[i].Model < models[j].Model })
			for _, model := range models {
				candidate := base
				candidate.Kind = "codex"
				candidate.ModelState = "selectable_configuration"
				candidate.Model = model.Model
				candidate.ReasoningEfforts = append([]string{}, model.ReasoningEfforts...)
				candidate.FixedConfigurationSHA256 = catalog.ConfigurationSHA256
				candidate.CodexSelection = &cc.Selection{CatalogSHA256: catalog.CatalogSHA256, Model: model.Model, ReasoningEffort: model.DefaultReasoningEffort}
				// An empty official effort list means use the app-server default,
				// represented explicitly without inventing another reasoning mode.
				if len(candidate.ReasoningEfforts) == 0 && model.DefaultReasoningEffort == "" {
					candidate.ReasoningEfforts = []string{""}
				}
				group = append(group, candidate)
			}
		default:
			if hard {
				out.Coverage.ExcludedCount++
				continue
			}
			candidate := base
			candidate.Kind = "fixed"
			candidate.FixedConfigurationSHA256 = profile.Profile.ProfileSHA256
			if supportsOpenCodeExecution(profile) {
				candidate.Model = profile.Profile.OpenCodeExecution.Model
				candidate.FixedConfigurationSHA256 = profile.Profile.OpenCodeExecution.ConfigurationSHA256
			}
			group = append(group, candidate)
		}
		for i := range group {
			group[i].ID = dispatchRecommendationCandidateID(req, group[i])
		}
		if len(group) > 0 {
			groups = append(groups, group)
		}
	}
	for round := 0; ; round++ {
		more := false
		for _, group := range groups {
			if round >= len(group) {
				continue
			}
			more = true
			if len(out.Candidates) < dispatchRecommendationMaxCandidates {
				out.Candidates = append(out.Candidates, group[round])
			} else {
				out.Coverage.Complete = false
			}
		}
		if !more {
			break
		}
	}
	out.Coverage.CandidateCount = len(out.Candidates)
	if !out.Coverage.Complete {
		out.Coverage.Reason = "bounded_candidates"
	}
	// Heartbeat timestamps remain visible provenance. A renewed, identical
	// inventory must not force another model call; freshness is rechecked above.
	basis := out
	basis.SHA256 = ""
	basis.ObservedAt = ""
	basis.Candidates = append([]DispatchRecommendationCandidate{}, out.Candidates...)
	for i := range basis.Candidates {
		basis.Candidates[i].InventoryObservedAt = ""
	}
	out.SHA256 = digest(stringify(basis))
	return out, nil
}

func dispatchRecommendationCandidateID(req DispatchRecommendationOptionsRequest, candidate DispatchRecommendationCandidate) string {
	candidate.ID = ""
	candidate.InventoryObservedAt = ""
	return digest(stringify([]any{"suma-dispatch-candidate-v1", req.TaskID, req.ExpectedTaskVersion, req.WorkflowID, req.WorkflowVersion, req.AgentNodeID, candidate}))
}

// Choose a valid initial configuration within the reviewed ceilings, without
// rejecting a cheaper feasible setting merely because defaults were larger.
// The model/user may lower or adjust it later; adoption revalidates NewBinding.
func dispatchRecommendationFeasibleParameters(deployment mc.Deployment, constraints DispatchRecommendationConstraints, quote *mc.RateSnapshot) (mc.Parameters, bool) {
	for steps := min(8, constraints.MaxSteps); steps >= 1; steps-- {
		hi := min(int64(4096), constraints.MaxOutputTokens)
		if constraints.Budget != nil && constraints.Budget.TokenLimit != nil {
			hi = min(hi, *constraints.Budget.TokenLimit-deployment.ContextWindow*int64(steps))
		}
		if hi < 1 {
			continue
		}
		valid := func(tokens int64) bool {
			_, err := mc.NewBinding(deployment, mc.Parameters{MaxSteps: steps, MaxOutputTokens: tokens}, constraints.Budget, quote)
			return err == nil
		}
		if valid(hi) {
			return mc.Parameters{MaxSteps: steps, MaxOutputTokens: hi}, true
		}
		if !valid(1) {
			continue
		}
		lo := int64(1)
		for lo < hi {
			mid := lo + (hi-lo+1)/2
			if valid(mid) {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return mc.Parameters{MaxSteps: steps, MaxOutputTokens: lo}, true
	}
	return mc.Parameters{}, false
}
