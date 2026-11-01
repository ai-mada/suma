package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	claudeSDKExecutionInvalid     = "Invalid Claude SDK execution configuration"
	claudeSDKExecutionChanged     = "Claude SDK configuration changed; prepare a new dispatch preview"
	claudeSDKExecutionUnavailable = "The fixed Claude SDK configuration is not in the current authorized worker inventory"
	claudeSDKExecutionBudget      = "Claude SDK supports observed usage only; enforced budgets are unavailable"
)

func supportsClaudeSDKExecution(p ToolProfile) bool {
	return tc.SupportsClaudeSDKExecutionProfile(p.Profile)
}
func checkClaudeSDKObserveOnlyPolicies(p ToolProfile, policies ...UsagePolicy) error {
	if !supportsClaudeSDKExecution(p) {
		return nil
	}
	for _, policy := range policies {
		if policy.Mode == "enforce" {
			return invalid(claudeSDKExecutionBudget)
		}
	}
	return nil
}
func (s *Service) checkClaudeSDKBudgetTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile) error {
	if !supportsClaudeSDKExecution(p) {
		return nil
	}
	for _, scope := range []string{"organization", "project"} {
		policy, err := s.usagePolicyTx(ctx, tx, tenant, project, scope)
		if err != nil {
			return err
		}
		if err = checkClaudeSDKObserveOnlyPolicies(p, policy); err != nil {
			return err
		}
	}
	return nil
}
func claudeSDKNodeConfiguration(n dag.Node) (*tc.ClaudeSDKExecutionCapability, error) {
	fixed, err := tc.DecodeClaudeSDKNodeConfiguration(n.Config)
	if err != nil {
		return nil, invalid(claudeSDKExecutionInvalid)
	}
	return fixed, nil
}
func claudeSDKNodeMatchesProfile(n dag.Node, p ToolProfile) bool {
	fixed, err := claudeSDKNodeConfiguration(n)
	if err != nil {
		return false
	}
	if !supportsClaudeSDKExecution(p) {
		return fixed == nil
	}
	return fixed != nil && *fixed == *p.Profile.ClaudeSDKExecution && n.Type == "agent" && n.MaxRetries == 0 && len(n.Command) == 1 && n.Command[0] == p.Profile.CommandID
}
func (s *Service) validateClaudeSDKExecutionPlanTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, plan TaskDispatchPlan) error {
	var node *dag.Node
	for i := range plan.Workflow.Nodes {
		if plan.Workflow.Nodes[i].ID == plan.AgentNodeID {
			node = &plan.Workflow.Nodes[i]
			break
		}
	}
	if node == nil || !claudeSDKNodeMatchesProfile(*node, p) {
		return conflict(claudeSDKExecutionChanged)
	}
	if !supportsClaudeSDKExecution(p) {
		if plan.ClaudeSDKExecution != nil {
			return conflict(claudeSDKExecutionChanged)
		}
		return nil
	}
	if plan.ClaudeSDKExecution == nil || *plan.ClaudeSDKExecution != *p.Profile.ClaudeSDKExecution || plan.ModelExecution != nil || plan.CodexExecution != nil || plan.OpenCodeExecution != nil || plan.ClaudeExecution != nil || plan.ACPExecution != nil {
		return conflict(claudeSDKExecutionChanged)
	}
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	if !current.Available {
		return conflict(claudeSDKExecutionUnavailable)
	}
	return s.checkClaudeSDKBudgetTx(ctx, tx, tenant, project, current)
}
func (s *Service) claudeSDKExecutionClaimEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, n dag.Node) (bool, error) {
	if !claudeSDKNodeMatchesProfile(n, p) {
		return false, nil
	}
	if err := s.checkClaudeSDKBudgetTx(ctx, tx, tenant, project, p); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
