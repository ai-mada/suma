package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	cc "github.com/ai-mada/suma/internal/codexcontract"
	"github.com/ai-mada/suma/internal/dag"
)

const (
	codexExecutionInvalid     = "Invalid Codex model execution selection"
	codexExecutionUnsupported = "Independent Codex model selection requires an approved app-server tool"
	codexExecutionChanged     = "Codex model catalog or local configuration changed; prepare a new dispatch preview"
	codexExecutionUnavailable = "The selected Codex model is not in the current authorized worker catalog"
	codexExecutionBudget      = "Codex app-server supports observed usage only; enforced budgets are unavailable"
)

func supportsCodexExecution(p ToolProfile) bool {
	return p.Profile.Adapter == "codex" && p.Profile.Protocol == cc.Protocol && !p.Profile.ReadOnly && p.Profile.WorkspaceToolsVersion == "" && p.Profile.ModelExecutionProtocol == ""
}

func (s *Service) currentCodexCatalogTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile) (*cc.Catalog, error) {
	if !supportsCodexExecution(p) {
		return nil, invalid(codexExecutionUnsupported)
	}
	current, err := s.toolProfileObservationLockedTx(ctx, tx, tenant, project, p)
	if err != nil {
		return nil, err
	}
	if !current.Available || current.Observation == nil || current.Observation.CodexCatalog == nil {
		return nil, conflict(codexExecutionUnavailable)
	}
	catalog := cc.CloneCatalog(*current.Observation.CodexCatalog)
	if cc.ValidateCatalog(catalog) != nil {
		return nil, invalid(codexExecutionInvalid)
	}
	return &catalog, nil
}

// Observe-mode policy limits are notices. Enforce-mode policies require a
// bounded execution capability which this external configuration ACK lacks.
func (s *Service) checkCodexBudgetTx(ctx context.Context, tx *sql.Tx, tenant, project string) error {
	for _, scope := range []string{"organization", "project"} {
		policy, err := s.usagePolicyTx(ctx, tx, tenant, project, scope)
		if err != nil {
			return err
		}
		if policy.Mode == "enforce" {
			return invalid(codexExecutionBudget)
		}
	}
	return nil
}

func (s *Service) taskDispatchSourceCodexTx(ctx context.Context, tx *sql.Tx, tenant, project string, profile ToolProfile, request TaskDispatchPreviewRequest, source *TaskDispatchPlan) (*cc.Binding, error) {
	if request.CodexExecutionSelection != nil && request.ModelExecutionSelection != nil {
		return nil, invalid(codexExecutionInvalid)
	}
	if source != nil {
		if err := s.validateCodexExecutionPlanTx(ctx, tx, tenant, project, profile, *source); err != nil {
			return nil, err
		}
		return source.CodexExecution, nil
	}
	selection := request.CodexExecutionSelection
	if selection == nil {
		if supportsCodexExecution(profile) {
			return nil, invalid(codexExecutionInvalid)
		}
		return nil, nil
	}
	if cc.ValidateSelection(*selection) != nil {
		return nil, invalid(codexExecutionInvalid)
	}
	catalog, err := s.currentCodexCatalogTx(ctx, tx, tenant, project, profile)
	if err != nil {
		return nil, err
	}
	binding, err := cc.NewBinding(*catalog, *selection)
	if err != nil {
		return nil, conflict(codexExecutionUnavailable)
	}
	if err = s.checkCodexBudgetTx(ctx, tx, tenant, project); err != nil {
		return nil, err
	}
	return &binding, nil
}

func bindCodexExecutionWorkflow(w *dag.Workflow, node string, b *cc.Binding) error {
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if n.ID != node {
			continue
		}
		delete(n.Config, "codex_execution")
		if b != nil {
			n.MaxRetries = 0
			var raw map[string]any
			if err := json.Unmarshal([]byte(stringify(b)), &raw); err != nil {
				return err
			}
			n.Config["codex_execution"] = raw
			if _, err := dag.CodexBinding(*n); err != nil {
				return invalid(codexExecutionInvalid)
			}
		}
		return dag.Compile(*w)
	}
	return invalid(codexExecutionInvalid)
}

func codexBindingInCatalog(b cc.Binding, catalog cc.Catalog) bool {
	if b.ConfigurationSHA256 != catalog.ConfigurationSHA256 || b.CatalogSHA256 != catalog.CatalogSHA256 {
		return false
	}
	current, err := cc.NewBinding(catalog, cc.Selection{CatalogSHA256: b.CatalogSHA256, Model: b.Model, ReasoningEffort: b.ReasoningEffort, TokenNoticeLimit: b.TokenNoticeLimit})
	return err == nil && stringify(current) == stringify(b)
}

func (s *Service) validateCodexExecutionPlanTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, plan TaskDispatchPlan) error {
	if err := validateTaskExecutionSkills(plan, p.Profile); err != nil {
		return err
	}
	if plan.ModelExecution != nil && plan.CodexExecution != nil {
		return invalid(codexExecutionInvalid)
	}
	var node *dag.Node
	for i := range plan.Workflow.Nodes {
		if plan.Workflow.Nodes[i].ID == plan.AgentNodeID {
			node = &plan.Workflow.Nodes[i]
			break
		}
	}
	if node == nil {
		return invalid(codexExecutionInvalid)
	}
	b, err := dag.CodexBinding(*node)
	if err != nil {
		return invalid(codexExecutionInvalid)
	}
	if stringify(b) != stringify(plan.CodexExecution) {
		return conflict(codexExecutionChanged)
	}
	if b == nil {
		if supportsCodexExecution(p) {
			return conflict(codexExecutionChanged)
		}
		return nil
	}
	catalog, err := s.currentCodexCatalogTx(ctx, tx, tenant, project, p)
	if err != nil {
		return err
	}
	if !codexBindingInCatalog(*b, *catalog) {
		return conflict(codexExecutionChanged)
	}
	return s.checkCodexBudgetTx(ctx, tx, tenant, project)
}

func (s *Service) codexExecutionClaimEligibleTx(ctx context.Context, tx *sql.Tx, tenant, project string, p ToolProfile, n dag.Node) (bool, error) {
	b, err := dag.CodexBinding(n)
	if err != nil {
		return false, nil
	}
	if b == nil {
		return !supportsCodexExecution(p), nil
	}
	catalog, err := s.currentCodexCatalogTx(ctx, tx, tenant, project, p)
	if err == nil && codexBindingInCatalog(*b, *catalog) {
		err = s.checkCodexBudgetTx(ctx, tx, tenant, project)
		if err == nil {
			return true, nil
		}
	}
	var api *APIError
	if errors.As(err, &api) {
		return false, nil
	}
	return false, err
}
