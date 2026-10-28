package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	opencodeExecutionInvalid     = "Invalid OpenCode SDK execution configuration"
	opencodeExecutionChanged     = "OpenCode SDK configuration changed; prepare a new dispatch preview"
	opencodeExecutionUnavailable = "The fixed OpenCode SDK configuration is not in the current authorized worker inventory"
	opencodeExecutionBudget      = "OpenCode SDK supports observed usage only; enforced budgets are unavailable"
)

func supportsOpenCodeExecution(p ToolProfile) bool {
	return tc.SupportsOpenCodeExecutionProfile(p.Profile)
}

// All admission/accounting paths consume their own already locked policy
// facts through the same capability rule. No configured rate creates a cap.
func checkOpenCodeObserveOnlyPolicies(p ToolProfile, policies ...UsagePolicy) error {
	if !supportsOpenCodeExecution(p) {
		return nil
	}
	for _, policy := range policies {
		if policy.Mode == "enforce" {
			return invalid(opencodeExecutionBudget)
		}
	}
	return nil
}

func (s *Service) checkOpenCodeBudgetTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile) error {
	if !supportsOpenCodeExecution(p) {
		return nil
	}
	for _, scope := range []string{"organization", "project"} {
		policy, err := s.usagePolicyTx(ctx, tx, tenant, project, scope)
		if err != nil {
			return err
		}
		if err := checkOpenCodeObserveOnlyPolicies(p, policy); err != nil {
			return err
		}
	}
	return nil
}

func openCodeNodeConfiguration(n dag.Node) (*tc.OpenCodeExecutionCapability, error) {
	fixed, err := tc.DecodeOpenCodeNodeConfiguration(n.Config)
	if err != nil {
		return nil, invalid(opencodeExecutionInvalid)
	}
	return fixed, nil
}

func openCodeNodeMatchesProfile(n dag.Node, p ToolProfile) bool {
	fixed, err := openCodeNodeConfiguration(n)
	if err != nil {
		return false
	}
	if !supportsOpenCodeExecution(p) {
		return fixed == nil
	}
	_, native := n.Config["model_execution"]
	_, codex := n.Config["codex_execution"]
	return fixed != nil && *fixed == *p.Profile.OpenCodeExecution && n.Type == "agent" && n.MaxRetries == 0 && len(n.Command) == 1 && n.Command[0] == p.Profile.CommandID && !native && !codex
}

func (s *Service) validateOpenCodeExecutionPlanTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, plan TaskDispatchPlan) error {
	var n *dag.Node
	for i := range plan.Workflow.Nodes {
		if plan.Workflow.Nodes[i].ID == plan.AgentNodeID {
			n = &plan.Workflow.Nodes[i]
			break
		}
	}
	if n == nil || !openCodeNodeMatchesProfile(*n, p) {
		return conflict(opencodeExecutionChanged)
	}
	if !supportsOpenCodeExecution(p) {
		if plan.OpenCodeExecution != nil {
			return conflict(opencodeExecutionChanged)
		}
		return nil
	}
	if plan.OpenCodeExecution == nil || *plan.OpenCodeExecution != *p.Profile.OpenCodeExecution || plan.ModelExecution != nil || plan.CodexExecution != nil {
		return conflict(opencodeExecutionChanged)
	}
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	if !current.Available {
		return conflict(opencodeExecutionUnavailable)
	}
	return s.checkOpenCodeBudgetTx(ctx, tx, tenant, project, current)
}

func (s *Service) openCodeExecutionClaimEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, n dag.Node) (bool, error) {
	if !openCodeNodeMatchesProfile(n, p) {
		return false, nil
	}
	if err := s.checkOpenCodeBudgetTx(ctx, tx, tenant, project, p); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
