package platform

import (
	"context"
	"database/sql"
	"errors"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const (
	acpExecutionInvalid     = "Invalid fixed ACP execution configuration"
	acpExecutionChanged     = "ACP configuration changed; prepare a new dispatch preview"
	acpExecutionUnavailable = "The fixed ACP configuration is not in the current authorized worker inventory"
	acpExecutionBudget      = "ACP supports observed usage only; enforced budgets are unavailable"
)

func supportsACPExecution(p ToolProfile) bool { return tc.SupportsACPExecutionProfile(p.Profile) }

func checkACPObserveOnlyPolicies(p ToolProfile, policies ...UsagePolicy) error {
	if supportsACPExecution(p) {
		for _, policy := range policies {
			if policy.Mode == "enforce" {
				return invalid(acpExecutionBudget)
			}
		}
	}
	return nil
}

func (s *Service) checkACPBudgetTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile) error {
	if !supportsACPExecution(p) {
		return nil
	}
	for _, scope := range []string{"organization", "project"} {
		policy, err := s.usagePolicyTx(ctx, tx, tenant, project, scope)
		if err != nil {
			return err
		}
		if err = checkACPObserveOnlyPolicies(p, policy); err != nil {
			return err
		}
	}
	return nil
}

func acpNodeConfiguration(n dag.Node) (*ac.Capability, error) {
	fixed, err := tc.DecodeACPNodeConfiguration(n.Config)
	if err != nil {
		return nil, invalid(acpExecutionInvalid)
	}
	return fixed, nil
}

func acpNodeMatchesProfile(n dag.Node, p ToolProfile) bool {
	fixed, err := acpNodeConfiguration(n)
	if err != nil {
		return false
	}
	if !supportsACPExecution(p) {
		return fixed == nil
	}
	return fixed != nil && *fixed == *p.Profile.ACPExecution && n.Type == "agent" && n.MaxRetries == 0 && len(n.Command) == 1 && n.Command[0] == p.Profile.CommandID
}

func (s *Service) validateACPExecutionPlanTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, plan TaskDispatchPlan) error {
	var node *dag.Node
	for i := range plan.Workflow.Nodes {
		if plan.Workflow.Nodes[i].ID == plan.AgentNodeID {
			node = &plan.Workflow.Nodes[i]
			break
		}
	}
	if node == nil || !acpNodeMatchesProfile(*node, p) {
		return conflict(acpExecutionChanged)
	}
	if !supportsACPExecution(p) {
		if plan.ACPExecution != nil {
			return conflict(acpExecutionChanged)
		}
		return nil
	}
	if plan.ACPExecution == nil || *plan.ACPExecution != *p.Profile.ACPExecution || plan.ModelExecution != nil || plan.CodexExecution != nil || plan.OpenCodeExecution != nil || plan.ClaudeExecution != nil {
		return conflict(acpExecutionChanged)
	}
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	if !current.Available {
		return conflict(acpExecutionUnavailable)
	}
	return s.checkACPBudgetTx(ctx, tx, tenant, project, current)
}

func (s *Service) acpExecutionClaimEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, n dag.Node) (bool, error) {
	if !acpNodeMatchesProfile(n, p) {
		return false, nil
	}
	if err := s.checkACPBudgetTx(ctx, tx, tenant, project, p); err != nil {
		var api *APIError
		if errors.As(err, &api) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
