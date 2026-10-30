package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	claudeExecutionInvalid     = "Invalid Claude CLI execution configuration"
	claudeExecutionChanged     = "Claude CLI configuration changed; prepare a new dispatch preview"
	claudeExecutionUnavailable = "The fixed Claude CLI configuration is not in the current authorized worker inventory"
	claudeExecutionBudget      = "Claude CLI supports observed usage only; enforced budgets are unavailable"
)

func supportsClaudeExecution(p ToolProfile) bool { return tc.SupportsClaudeExecutionProfile(p.Profile) }
func checkClaudeObserveOnlyPolicies(p ToolProfile, policies ...UsagePolicy) error {
	if !supportsClaudeExecution(p) {
		return nil
	}
	for _, policy := range policies {
		if policy.Mode == "enforce" {
			return invalid(claudeExecutionBudget)
		}
	}
	return nil
}
func (s *Service) checkClaudeBudgetTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile) error {
	if !supportsClaudeExecution(p) {
		return nil
	}
	for _, scope := range []string{"organization", "project"} {
		policy, err := s.usagePolicyTx(ctx, tx, tenant, project, scope)
		if err != nil {
			return err
		}
		if err = checkClaudeObserveOnlyPolicies(p, policy); err != nil {
			return err
		}
	}
	return nil
}
func claudeNodeConfiguration(n dag.Node) (*tc.ClaudeExecutionCapability, error) {
	fixed, err := tc.DecodeClaudeNodeConfiguration(n.Config)
	if err != nil {
		return nil, invalid(claudeExecutionInvalid)
	}
	return fixed, nil
}
func claudeNodeMatchesProfile(n dag.Node, p ToolProfile) bool {
	fixed, err := claudeNodeConfiguration(n)
	if err != nil {
		return false
	}
	if !supportsClaudeExecution(p) {
		return fixed == nil
	}
	return fixed != nil && *fixed == *p.Profile.ClaudeExecution && n.Type == "agent" && n.MaxRetries == 0 && len(n.Command) == 1 && n.Command[0] == p.Profile.CommandID
}
func (s *Service) validateClaudeExecutionPlanTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, plan TaskDispatchPlan) error {
	var node *dag.Node
	for i := range plan.Workflow.Nodes {
		if plan.Workflow.Nodes[i].ID == plan.AgentNodeID {
			node = &plan.Workflow.Nodes[i]
			break
		}
	}
	if node == nil || !claudeNodeMatchesProfile(*node, p) {
		return conflict(claudeExecutionChanged)
	}
	if !supportsClaudeExecution(p) {
		if plan.ClaudeExecution != nil {
			return conflict(claudeExecutionChanged)
		}
		return nil
	}
	if plan.ClaudeExecution == nil || *plan.ClaudeExecution != *p.Profile.ClaudeExecution || plan.ModelExecution != nil || plan.CodexExecution != nil || plan.OpenCodeExecution != nil {
		return conflict(claudeExecutionChanged)
	}
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	if !current.Available {
		return conflict(claudeExecutionUnavailable)
	}
	return s.checkClaudeBudgetTx(ctx, tx, tenant, project, current)
}
func (s *Service) claudeExecutionClaimEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, n dag.Node) (bool, error) {
	if !claudeNodeMatchesProfile(n, p) {
		return false, nil
	}
	if err := s.checkClaudeBudgetTx(ctx, tx, tenant, project, p); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
