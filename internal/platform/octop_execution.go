package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	octopExecutionInvalid     = "Invalid Octop ACP execution configuration"
	octopExecutionChanged     = "Octop ACP configuration changed; prepare a new dispatch preview"
	octopExecutionUnavailable = "The fixed Octop ACP configuration is not in the current authorized worker inventory"
	octopExecutionBudget      = "Octop ACP supports observed usage only; enforced budgets are unavailable"
)

func supportsOctopExecution(p ToolProfile) bool { return tc.SupportsOctopExecutionProfile(p.Profile) }

func checkOctopObserveOnlyPolicies(p ToolProfile, policies ...UsagePolicy) error {
	if !supportsOctopExecution(p) {
		return nil
	}
	for _, policy := range policies {
		if policy.Mode == "enforce" {
			return invalid(octopExecutionBudget)
		}
	}
	return nil
}

// All preview, dispatch, claim and accounting checks use their existing locked
// policy facts. A generic rate cannot manufacture a hard allowance or pricing.
func (s *Service) checkOctopBudgetTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile) error {
	if !supportsOctopExecution(p) {
		return nil
	}
	for _, scope := range []string{"organization", "project"} {
		policy, err := s.usagePolicyTx(ctx, tx, tenant, project, scope)
		if err != nil {
			return err
		}
		if err = checkOctopObserveOnlyPolicies(p, policy); err != nil {
			return err
		}
	}
	return nil
}

func octopNodeConfiguration(n dag.Node) (*tc.OctopExecutionCapability, error) {
	fixed, err := tc.DecodeOctopNodeConfiguration(n.Config)
	if err != nil {
		return nil, invalid(octopExecutionInvalid)
	}
	return fixed, nil
}

func octopNodeMatchesProfile(n dag.Node, p ToolProfile) bool {
	fixed, err := octopNodeConfiguration(n)
	if err != nil {
		return false
	}
	if !supportsOctopExecution(p) {
		return fixed == nil
	}
	return fixed != nil && *fixed == *p.Profile.OctopExecution && n.Type == "agent" && n.MaxRetries == 0 && len(n.Command) == 1 && n.Command[0] == p.Profile.CommandID
}

func (s *Service) validateOctopExecutionPlanTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, plan TaskDispatchPlan) error {
	var n *dag.Node
	for i := range plan.Workflow.Nodes {
		if plan.Workflow.Nodes[i].ID == plan.AgentNodeID {
			n = &plan.Workflow.Nodes[i]
			break
		}
	}
	if n == nil || !octopNodeMatchesProfile(*n, p) {
		return conflict(octopExecutionChanged)
	}
	if !supportsOctopExecution(p) {
		if plan.OctopExecution != nil {
			return conflict(octopExecutionChanged)
		}
		return nil
	}
	if plan.OctopExecution == nil || *plan.OctopExecution != *p.Profile.OctopExecution || plan.ModelExecution != nil || plan.CodexExecution != nil || plan.OpenCodeExecution != nil {
		return conflict(octopExecutionChanged)
	}
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	if !current.Available {
		return conflict(octopExecutionUnavailable)
	}
	return s.checkOctopBudgetTx(ctx, tx, tenant, project, current)
}

func (s *Service) octopExecutionClaimEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, n dag.Node) (bool, error) {
	if !octopNodeMatchesProfile(n, p) {
		return false, nil
	}
	if err := s.checkOctopBudgetTx(ctx, tx, tenant, project, p); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
